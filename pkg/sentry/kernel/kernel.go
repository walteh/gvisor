// Copyright 2018 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package kernel provides an emulation of the Linux kernel.
//
// See README.md for a detailed overview.
//
// Lock order (outermost locks must be taken first):
//
//	Kernel.extMu
//	  TTY.mu
//	  timekeeperTcpipTimer.mu
//	  ThreadGroup.timerMu
//	    Locks acquired by ktime.Timer methods
//	      TaskSet.mu
//	        SignalHandlers.mu
//	          Task.mu
//	      runningTasksMu
//
// Locking SignalHandlers.mu in multiple SignalHandlers requires locking
// TaskSet.mu exclusively first. Locking Task.mu in multiple Tasks at the same
// time requires locking all of their signal mutexes first.
package kernel

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"gvisor.dev/gvisor/pkg/abi/linux"
	"gvisor.dev/gvisor/pkg/atomicbitops"
	"gvisor.dev/gvisor/pkg/cleanup"
	"gvisor.dev/gvisor/pkg/context"
	"gvisor.dev/gvisor/pkg/cpuid"
	"gvisor.dev/gvisor/pkg/devutil"
	"gvisor.dev/gvisor/pkg/errors/linuxerr"
	"gvisor.dev/gvisor/pkg/eventchannel"
	"gvisor.dev/gvisor/pkg/fd"
	"gvisor.dev/gvisor/pkg/fspath"
	"gvisor.dev/gvisor/pkg/log"
	"gvisor.dev/gvisor/pkg/refs"
	"gvisor.dev/gvisor/pkg/sentry/arch"
	"gvisor.dev/gvisor/pkg/sentry/devices/nvproxy/nvconf"
	"gvisor.dev/gvisor/pkg/sentry/fsimpl/nsfs"
	"gvisor.dev/gvisor/pkg/sentry/fsimpl/pipefs"
	"gvisor.dev/gvisor/pkg/sentry/fsimpl/sockfs"
	"gvisor.dev/gvisor/pkg/sentry/fsimpl/timerfd"
	"gvisor.dev/gvisor/pkg/sentry/fsimpl/tmpfs"
	"gvisor.dev/gvisor/pkg/sentry/hostcpu"
	"gvisor.dev/gvisor/pkg/sentry/inet"
	"gvisor.dev/gvisor/pkg/sentry/kernel/auth"
	"gvisor.dev/gvisor/pkg/sentry/kernel/futex"
	"gvisor.dev/gvisor/pkg/sentry/kernel/ipc"
	"gvisor.dev/gvisor/pkg/sentry/kernel/sched"
	"gvisor.dev/gvisor/pkg/sentry/ktime"
	"gvisor.dev/gvisor/pkg/sentry/limits"
	"gvisor.dev/gvisor/pkg/sentry/loader"
	"gvisor.dev/gvisor/pkg/sentry/mm"
	"gvisor.dev/gvisor/pkg/sentry/pgalloc"
	"gvisor.dev/gvisor/pkg/sentry/platform"
	"gvisor.dev/gvisor/pkg/sentry/socket/netlink/port"
	"gvisor.dev/gvisor/pkg/sentry/socket/unix/transport"
	sentrytime "gvisor.dev/gvisor/pkg/sentry/time"
	"gvisor.dev/gvisor/pkg/sentry/unimpl"
	uspb "gvisor.dev/gvisor/pkg/sentry/unimpl/unimplemented_syscall_go_proto"
	"gvisor.dev/gvisor/pkg/sentry/uniqueid"
	"gvisor.dev/gvisor/pkg/sentry/vfs"
	"gvisor.dev/gvisor/pkg/state"
	"gvisor.dev/gvisor/pkg/sync"
)

// IOUringEnabled is set to true when IO_URING is enabled. Added as a global to
// allow easy access everywhere.
var IOUringEnabled = false

// UserCounters is a set of user counters.
//
// +stateify savable
type UserCounters struct {
	uid auth.KUID

	rlimitNProc atomicbitops.Uint64
}

// incRLimitNProc increments the rlimitNProc counter.
func (uc *UserCounters) incRLimitNProc(ctx context.Context) error {
	lim := limits.FromContext(ctx).Get(limits.ProcessCount)
	creds := auth.CredentialsFromContext(ctx)
	nproc := uc.rlimitNProc.Add(1)
	if nproc > lim.Cur &&
		!creds.HasCapability(linux.CAP_SYS_ADMIN) &&
		!creds.HasCapability(linux.CAP_SYS_RESOURCE) {
		uc.rlimitNProc.Add(^uint64(0))
		return linuxerr.EAGAIN
	}
	return nil
}

// decRLimitNProc decrements the rlimitNProc counter.
func (uc *UserCounters) decRLimitNProc() {
	uc.rlimitNProc.Add(^uint64(0))
}

// CgroupMount contains the cgroup mount. These mounts are created for the root
// container by default and are stored in the kernel.
//
// +stateify savable
type CgroupMount struct {
	Fs    *vfs.Filesystem
	Root  *vfs.Dentry
	Mount *vfs.Mount
}

// SaveRestoreExecConfig contains the configuration for the save/restore binary.
//
// +stateify savable
type SaveRestoreExecConfig struct {
	// Argv is the argv to the save/restore binary. The binary path is expected to
	// be argv[0]. The specified binary is executed with an environment variable
	// (GVISOR_SAVE_RESTORE_AUTO_EXEC_MODE) set to "save" before the kernel is
	// saved, "restore" after the kernel is restored and restarted, and "resume"
	// after the kernel is saved and resumed.
	Argv []string
	// Timeout is the timeout for the save/restore binary. If the binary fails to
	// exit within this timeout the save/restore operation will fail.
	Timeout time.Duration
	// LeaderTask is the task in the kernel that the save/restore binary will run
	// under.
	LeaderTask *Task
}

// Kernel represents an emulated Linux kernel. It must be initialized by calling
// Init() or LoadFrom().
//
// +stateify savable
type Kernel struct {
	// extMu serializes external changes to the Kernel with calls to
	// Kernel.SaveTo. (Kernel.SaveTo requires that the state of the Kernel
	// remains frozen for the duration of the call; it requires that the Kernel
	// is paused as a precondition, which ensures that none of the tasks
	// running within the Kernel can affect its state, but extMu is required to
	// ensure that concurrent users of the Kernel *outside* the Kernel's
	// control cannot affect its state by calling e.g.
	// Kernel.SendExternalSignal.)
	extMu sync.Mutex `state:"nosave"`

	// started is true if Start has been called. Unless otherwise specified,
	// all Kernel fields become immutable once started becomes true.
	started bool `state:"nosave"`

	// All of the following fields are immutable unless otherwise specified.

	// Platform is the platform that is used to execute tasks in the created
	// Kernel.
	platform.Platform `state:"nosave"`

	// mf provides application memory.
	mf *pgalloc.MemoryFile `state:"nosave"`

	// See InitKernelArgs for the meaning of these fields.
	featureSet           cpuid.FeatureSet
	timekeeper           *Timekeeper
	tasks                *TaskSet
	rootUserNamespace    *auth.UserNamespace
	rootNetworkNamespace *inet.Namespace
	applicationCores     uint
	useHostCores         bool
	extraAuxv            []arch.AuxEntry
	vdso                 *loader.VDSO
	vdsoParams           *VDSOParamPage
	rootUTSNamespace     *UTSNamespace
	rootIPCNamespace     *IPCNamespace

	// futexes is the "root" futex.Manager, from which all others are forked.
	// This is necessary to ensure that shared futexes are coherent across all
	// tasks, including those created by CreateProcess.
	futexes *futex.Manager

	// globalInit is the thread group whose leader has ID 1 in the root PID
	// namespace. globalInit is stored separately so that it is accessible even
	// after all tasks in the thread group have exited, such that ID 1 is no
	// longer mapped.
	//
	// globalInit is mutable until it is assigned by the first successful call
	// to CreateProcess, and is protected by extMu.
	globalInit *ThreadGroup

	// syslog is the kernel log.
	syslog syslog

	runningTasksMu runningTasksMutex `state:"nosave"`

	// runningTasks is the total count of tasks currently in
	// TaskGoroutineRunningSys or TaskGoroutineRunningApp. i.e., they are
	// not blocked or stopped.
	//
	// runningTasks must be accessed atomically. Increments from 0 to 1 are
	// further protected by runningTasksMu (see incRunningTasks).
	runningTasks atomicbitops.Int64

	// runningTasksCond is signaled when runningTasks is incremented from 0 to 1.
	//
	// Invariant: runningTasksCond.L == &runningTasksMu.
	runningTasksCond sync.Cond `state:"nosave"`

	// cpuClockTickTimer drives increments of cpuClock.
	cpuClockTickTimer *time.Timer `state:"nosave"`

	// cpuClockTickerRunning is true if the goroutine that increments cpuClock is
	// running, and false if it is blocked in runningTasksCond.Wait() or if it
	// never started.
	//
	// cpuClockTickerRunning is protected by runningTasksMu.
	cpuClockTickerRunning bool

	// cpuClockTickerWakeCh is sent to to wake the goroutine that increments
	// cpuClock if it's sleeping between ticks.
	cpuClockTickerWakeCh chan struct{} `state:"nosave"`

	// cpuClockTickerStopCond is broadcast when cpuClockTickerRunning transitions
	// from true to false.
	//
	// Invariant: cpuClockTickerStopCond.L == &runningTasksMu.
	cpuClockTickerStopCond sync.Cond `state:"nosave"`

	// cpuClock is a coarse monotonic clock that is advanced by the CPU clock
	// ticker and thus approximates wall time when tasks are running (but is
	// strictly slower due to CPU clock ticker goroutine wakeup latency). This
	// does not use ktime.SyntheticClock since this clock currently does not
	// need to support timers.
	cpuClock atomicbitops.Int64

	// uniqueID is used to generate unique identifiers.
	//
	// uniqueID is mutable, and is accessed using atomic memory operations.
	uniqueID atomicbitops.Uint64

	// nextInotifyCookie is a monotonically increasing counter used for
	// generating unique inotify event cookies.
	//
	// nextInotifyCookie is mutable.
	nextInotifyCookie atomicbitops.Uint32

	// netlinkPorts manages allocation of netlink socket port IDs.
	netlinkPorts *port.Manager

	// saveStatus is nil if the sandbox has not been saved, errSaved or
	// errAutoSaved if it has been saved successfully, or the error causing the
	// sandbox to exit during save.
	// It is protected by extMu.
	saveStatus error `state:"nosave"`

	// danglingEndpoints is used to save / restore tcpip.DanglingEndpoints.
	danglingEndpoints struct{} `state:".([]tcpip.Endpoint)"`

	// sockets records all network sockets in the system. Protected by extMu.
	sockets map[*vfs.FileDescription]*SocketRecord

	// nextSocketRecord is the next entry number to use in sockets. Protected
	// by extMu.
	nextSocketRecord uint64

	// unimplementedSyscallEmitterOnce is used in the initialization of
	// unimplementedSyscallEmitter.
	unimplementedSyscallEmitterOnce sync.Once `state:"nosave"`

	// unimplementedSyscallEmitter is used to emit unimplemented syscall
	// events. This is initialized lazily on the first unimplemented
	// syscall.
	unimplementedSyscallEmitter eventchannel.Emitter `state:"nosave"`

	// SpecialOpts contains special kernel options.
	SpecialOpts

	// vfs keeps the filesystem state used across the kernel.
	vfs vfs.VirtualFilesystem

	// hostMount is the Mount used for file descriptors that were imported
	// from the host.
	hostMount *vfs.Mount

	// pipeMount is the Mount used for pipes created by the pipe() and pipe2()
	// syscalls (as opposed to named pipes created by mknod()).
	pipeMount *vfs.Mount

	// nsfsMount is the Mount used for namespaces.
	nsfsMount *vfs.Mount

	// shmMount is the Mount used for anonymous files created by the
	// memfd_create() syscalls. It is analogous to Linux's shm_mnt.
	shmMount *vfs.Mount

	// socketMount is the Mount used for sockets created by the socket() and
	// socketpair() syscalls. There are several cases where a socket dentry will
	// not be contained in socketMount:
	// 1. Socket files created by mknod()
	// 2. Socket fds imported from the host (Kernel.hostMount is used for these)
	// 3. Socket files created by binding Unix sockets to a file path
	socketMount *vfs.Mount

	// sysVShmDevID is the device number used by SysV shm segments. In Linux,
	// SysV shm uses shmem_file_setup() and thus uses shm_mnt's device number.
	// In gVisor, the shm implementation does not use shmMount, extracting
	// shmMount's device number is inconvenient, applications accept a
	// different device number in practice, and using a distinct device number
	// avoids the possibility of inode number collisions due to the hack
	// described in shm.Shm.InodeID().
	sysVShmDevID uint32

	// If set to true, report address space activation waits as if the task is in
	// external wait so that the watchdog doesn't report the task stuck.
	SleepForAddressSpaceActivation bool

	// Exceptions to YAMA ptrace restrictions. Each key-value pair represents a
	// tracee-tracer relationship. The key is a process (technically, the thread
	// group leader) that can be traced by any thread that is a descendant of the
	// value. If the value is nil, then anyone can trace the process represented by
	// the key.
	//
	// ptraceExceptions is protected by the TaskSet mutex.
	ptraceExceptions map[*Task]*Task

	// YAMAPtraceScope is the current level of YAMA ptrace restrictions.
	YAMAPtraceScope atomicbitops.Int32

	// cgroupRegistry contains the set of active cgroup controllers on the
	// system. It is controller by cgroupfs. Nil if cgroupfs is unavailable on
	// the system.
	cgroupRegistry *CgroupRegistry

	// cgroupMountsMap maps the cgroup controller names to the cgroup mounts
	// created for the root container. These mounts are then bind mounted
	// for other application containers by creating their own container
	// directories.
	cgroupMountsMap   map[string]*CgroupMount
	cgroupMountsMapMu cgroupMountsMutex `state:"nosave"`

	// userCountersMap maps auth.KUID into a set of user counters.
	userCountersMap   map[auth.KUID]*UserCounters
	userCountersMapMu userCountersMutex `state:"nosave"`

	// MaxFDLimit specifies the maximum file descriptor number that can be
	// used by processes.
	MaxFDLimit atomicbitops.Int32

	// devGofers maps containers (using its name) to its device gofer client.
	devGofers   map[string]*devutil.GoferClient `state:"nosave"`
	devGofersMu sync.Mutex                      `state:"nosave"`

	// containerNames store the container name based on their container ID.
	// Names are preserved between save/restore session, while IDs can change.
	//
	// Mapping: cid -> name.
	// It's protected by extMu.
	containerNames map[string]string

	// checkpointMu is used to protect the checkpointing related fields below.
	checkpointMu sync.Mutex `state:"nosave"`

	// additionalCheckpointState stores additional state that needs
	// to be checkpointed. It's protected by checkpointMu.
	additionalCheckpointState map[any]any

	// saver implements the Saver interface, which (as of writing) supports
	// asynchronous checkpointing. It's protected by checkpointMu.
	saver Saver `state:"nosave"`

	// CheckpointWait is used to wait for a checkpoint to complete.
	CheckpointWait CheckpointWaitable

	// checkpointGen aims to track the number of times the kernel has been
	// successfully checkpointed. Callers of checkpoint must notify the kernel
	// when checkpoint/restore are done. It's protected by checkpointMu.
	checkpointGen CheckpointGeneration

	// UnixSocketOpts stores configuration options for management of unix sockets.
	UnixSocketOpts transport.UnixSocketOpts

	// SaveRestoreExecConfig stores configuration options for the save/restore
	// exec binary.
	SaveRestoreExecConfig *SaveRestoreExecConfig

	// NvidiaDriverVersion is the NVIDIA driver version configured for this
	// sandbox.
	NvidiaDriverVersion nvconf.DriverVersion
}

// InitKernelArgs holds arguments to Init.
type InitKernelArgs struct {
	// FeatureSet is the emulated CPU feature set.
	FeatureSet cpuid.FeatureSet

	// Timekeeper manages time for all tasks in the system.
	Timekeeper *Timekeeper

	// RootUserNamespace is the root user namespace.
	RootUserNamespace *auth.UserNamespace

	// RootNetworkNamespace is the root network namespace. If nil, no networking
	// will be available.
	RootNetworkNamespace *inet.Namespace

	// ApplicationCores is the number of logical CPUs visible to sandboxed
	// applications. The set of logical CPU IDs is [0, ApplicationCores); thus
	// ApplicationCores is analogous to Linux's nr_cpu_ids, the index of the
	// most significant bit in cpu_possible_mask + 1.
	ApplicationCores uint

	// If UseHostCores is true, Task.CPU() returns the task goroutine's CPU
	// instead of a virtualized CPU number, and Task.CopyToCPUMask() is a
	// no-op. If ApplicationCores is less than hostcpu.MaxPossibleCPU(), it
	// will be overridden.
	UseHostCores bool

	// ExtraAuxv contains additional auxiliary vector entries that are added to
	// each process by the ELF loader.
	ExtraAuxv []arch.AuxEntry

	// Vdso holds the VDSO and its parameter page.
	Vdso *loader.VDSO

	// VdsoParams is the VDSO parameter page manager.
	VdsoParams *VDSOParamPage

	// RootUTSNamespace is the root UTS namespace.
	RootUTSNamespace *UTSNamespace

	// RootIPCNamespace is the root IPC namespace.
	RootIPCNamespace *IPCNamespace

	// RootPIDNamespace is the root PID namespace.
	RootPIDNamespace *PIDNamespace

	// MaxFDLimit specifies the maximum file descriptor number that can be
	// used by processes.  If it is zero, the limit will be set to
	// unlimited.
	MaxFDLimit int32

	// UnixSocketOpts contains configuration options for unix sockets.
	UnixSocketOpts transport.UnixSocketOpts
}

// Init initialize the Kernel with no tasks.
//
// Callers must manually set Kernel.Platform and call Kernel.SetMemoryFile
// before calling Init.
func (k *Kernel) Init(args InitKernelArgs) error {
	if args.Timekeeper == nil {
		return fmt.Errorf("args.Timekeeper is nil")
	}
	if args.Timekeeper.clocks == nil {
		return fmt.Errorf("must call Timekeeper.SetClocks() before Kernel.Init()")
	}
	if args.RootUserNamespace == nil {
		return fmt.Errorf("args.RootUserNamespace is nil")
	}
	if args.ApplicationCores == 0 {
		return fmt.Errorf("args.ApplicationCores is 0")
	}

	k.featureSet = args.FeatureSet
	k.timekeeper = args.Timekeeper
	k.tasks = newTaskSet(args.RootPIDNamespace)
	k.rootUserNamespace = args.RootUserNamespace
	k.rootUTSNamespace = args.RootUTSNamespace
	k.rootIPCNamespace = args.RootIPCNamespace
	k.rootNetworkNamespace = args.RootNetworkNamespace
	if k.rootNetworkNamespace == nil {
		k.rootNetworkNamespace = inet.NewRootNamespace(nil, nil, args.RootUserNamespace)
	}
	k.runningTasksCond.L = &k.runningTasksMu
	k.cpuClockTickerWakeCh = make(chan struct{}, 1)
	k.cpuClockTickerStopCond.L = &k.runningTasksMu
	k.applicationCores = args.ApplicationCores
	if args.UseHostCores {
		k.useHostCores = true
		maxCPU, err := hostcpu.MaxPossibleCPU()
		if err != nil {
			return fmt.Errorf("failed to get maximum CPU number: %v", err)
		}
		minAppCores := uint(maxCPU) + 1
		if k.applicationCores < minAppCores {
			log.Infof("UseHostCores enabled: increasing ApplicationCores from %d to %d", k.applicationCores, minAppCores)
			k.applicationCores = minAppCores
		}
	}
	k.extraAuxv = args.ExtraAuxv
	k.vdso = args.Vdso
	k.vdsoParams = args.VdsoParams
	k.futexes = futex.NewManager()
	k.netlinkPorts = port.New()
	k.ptraceExceptions = make(map[*Task]*Task)
	k.YAMAPtraceScope = atomicbitops.FromInt32(linux.YAMA_SCOPE_RELATIONAL)
	k.userCountersMap = make(map[auth.KUID]*UserCounters)
	if args.MaxFDLimit == 0 {
		args.MaxFDLimit = MaxFdLimit
	}
	k.MaxFDLimit.Store(args.MaxFDLimit)
	k.containerNames = make(map[string]string)
	k.CheckpointWait.k = k

	ctx := k.SupervisorContext()
	if err := k.vfs.Init(ctx); err != nil {
		return fmt.Errorf("failed to initialize VFS: %v", err)
	}

	err := k.rootIPCNamespace.InitPosixQueues(ctx, &k.vfs, auth.CredentialsFromContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to create mqfs filesystem: %v", err)
	}

	pipeFilesystem, err := pipefs.NewFilesystem(&k.vfs)
	if err != nil {
		return fmt.Errorf("failed to create pipefs filesystem: %v", err)
	}
	defer pipeFilesystem.DecRef(ctx)
	pipeMount := k.vfs.NewDisconnectedMount(pipeFilesystem, nil, &vfs.MountOptions{})
	k.pipeMount = pipeMount

	nsfsFilesystem, err := nsfs.NewFilesystem(&k.vfs)
	if err != nil {
		return fmt.Errorf("failed to create nsfs filesystem: %v", err)
	}
	defer nsfsFilesystem.DecRef(ctx)
	k.nsfsMount = k.vfs.NewDisconnectedMount(nsfsFilesystem, nil, &vfs.MountOptions{})
	k.rootNetworkNamespace.SetInode(nsfs.NewInode(ctx, k.nsfsMount, k.rootNetworkNamespace))
	k.rootIPCNamespace.SetInode(nsfs.NewInode(ctx, k.nsfsMount, k.rootIPCNamespace))
	k.rootUTSNamespace.SetInode(nsfs.NewInode(ctx, k.nsfsMount, k.rootUTSNamespace))

	args.RootPIDNamespace.InitInode(ctx, k)

	tmpfsOpts := vfs.GetFilesystemOptions{
		InternalData: tmpfs.FilesystemOpts{
			// See mm/shmem.c:shmem_init() => vfs_kern_mount(flags=SB_KERNMOUNT).
			// Note how mm/shmem.c:shmem_fill_super() does not provide a default
			// value for sbinfo->max_blocks when SB_KERNMOUNT is set.
			DisableDefaultSizeLimit: true,
		},
		InternalMount: true,
	}
	tmpfsFilesystem, tmpfsRoot, err := tmpfs.FilesystemType{}.GetFilesystem(ctx, &k.vfs, auth.NewRootCredentials(k.rootUserNamespace), "", tmpfsOpts)
	if err != nil {
		return fmt.Errorf("failed to create tmpfs filesystem: %v", err)
	}
	defer tmpfsFilesystem.DecRef(ctx)
	defer tmpfsRoot.DecRef(ctx)
	k.shmMount = k.vfs.NewDisconnectedMount(tmpfsFilesystem, tmpfsRoot, &vfs.MountOptions{})

	socketFilesystem, err := sockfs.NewFilesystem(&k.vfs)
	if err != nil {
		return fmt.Errorf("failed to create sockfs filesystem: %v", err)
	}
	defer socketFilesystem.DecRef(ctx)
	k.socketMount = k.vfs.NewDisconnectedMount(socketFilesystem, nil, &vfs.MountOptions{})

	sysVShmDevMinor, err := k.vfs.GetAnonBlockDevMinor()
	if err != nil {
		return fmt.Errorf("failed to get device number for SysV shm: %v", err)
	}
	k.sysVShmDevID = linux.MakeDeviceID(linux.UNNAMED_MAJOR, sysVShmDevMinor)

	k.sockets = make(map[*vfs.FileDescription]*SocketRecord)

	k.cgroupRegistry = newCgroupRegistry()
	k.UnixSocketOpts = args.UnixSocketOpts
	return nil
}

// +stateify savable
type privateMemoryFileMetadata struct {
	owners []string
}

func savePrivateMFs(ctx context.Context, w io.Writer, pw io.Writer, mfsToSave map[string]*pgalloc.MemoryFile, mfOpts pgalloc.SaveOpts) error {
	// mfOpts.ExcludeCommittedZeroPages is expected to reflect application
	// memory usage behavior, but not necessarily usage of private MemoryFiles.
	mfOpts.ExcludeCommittedZeroPages = false

	var meta privateMemoryFileMetadata
	// Generate the order in which private memory files are saved.
	for fsID := range mfsToSave {
		meta.owners = append(meta.owners, fsID)
	}
	// Save the metadata.
	if _, err := state.Save(ctx, w, &meta); err != nil {
		return err
	}
	// Followed by the private memory files in order.
	for _, fsID := range meta.owners {
		if err := mfsToSave[fsID].SaveTo(ctx, w, pw, mfOpts); err != nil {
			return err
		}
	}
	return nil
}

// SaveTo saves the state of k to w.
//
// Preconditions: The kernel must be paused throughout the call to SaveTo.
func (k *Kernel) SaveTo(ctx context.Context, w, pagesMetadata io.Writer, pagesFile *fd.FD, mfOpts pgalloc.SaveOpts) error {
	saveStart := time.Now()

	// Do not allow other Kernel methods to affect it while it's being saved.
	k.extMu.Lock()
	defer k.extMu.Unlock()

	// Stop time.
	k.pauseTimeLocked(ctx)
	defer k.resumeTimeLocked(ctx)

	// Evict all evictable MemoryFile allocations.
	k.mf.StartEvictions()
	k.mf.WaitForEvictions()

	// Discard unsavable mappings, such as those for host file descriptors.
	if err := k.invalidateUnsavableMappings(ctx); err != nil {
		return fmt.Errorf("failed to invalidate unsavable mappings: %v", err)
	}

	// Capture all private memory files.
	mfsToSave := make(map[string]*pgalloc.MemoryFile)
	vfsCtx := context.WithValue(ctx, pgalloc.CtxMemoryFileMap, mfsToSave)
	// Prepare filesystems for saving. This must be done after
	// invalidateUnsavableMappings(), since dropping memory mappings may
	// affect filesystem state (e.g. page cache reference counts).
	if err := k.vfs.PrepareSave(vfsCtx); err != nil {
		return err
	}
	// Mark all to-be-saved MemoryFiles as savable to inform kernel save below.
	k.mf.MarkSavable()
	for _, mf := range mfsToSave {
		mf.MarkSavable()
	}

	var (
		mfSaveWg  sync.WaitGroup
		mfSaveErr error
	)
	parallelMfSave := pagesMetadata != nil && pagesFile != nil
	if parallelMfSave {
		// Parallelize MemoryFile save and kernel save. Both are independent.
		mfSaveWg.Add(1)
		go func() {
			defer mfSaveWg.Done()
			mfSaveErr = k.saveMemoryFiles(ctx, w, pagesMetadata, pagesFile, mfsToSave, mfOpts)
		}()
		// Defer a Wait() so we wait for k.saveMemoryFiles() to complete even if we
		// error out without reaching the other Wait() below.
		defer mfSaveWg.Wait()
	}

	// Save the CPUID FeatureSet before the rest of the kernel so we can
	// verify its compatibility on restore before attempting to restore the
	// entire kernel, which may fail on an incompatible machine.
	//
	// N.B. This will also be saved along with the full kernel save below.
	cpuidStart := time.Now()
	if _, err := state.Save(ctx, w, &k.featureSet); err != nil {
		return err
	}
	log.Infof("CPUID save took [%s].", time.Since(cpuidStart))

	// Save the timekeeper's state.

	if rootNS := k.rootNetworkNamespace; rootNS != nil && rootNS.Stack() != nil {
		// Pause the network stack.
		netstackPauseStart := time.Now()
		log.Infof("Pausing root network namespace")
		k.rootNetworkNamespace.Stack().Pause()
		defer k.rootNetworkNamespace.Stack().Resume()
		log.Infof("Pausing root network namespace took [%s].", time.Since(netstackPauseStart))
	}

	// Save the kernel state.
	kernelStart := time.Now()
	stats, err := state.Save(ctx, w, k)
	if err != nil {
		return err
	}
	log.Infof("Kernel save stats: %s", stats.String())
	log.Infof("Kernel save took [%s].", time.Since(kernelStart))

	if parallelMfSave {
		mfSaveWg.Wait()
	} else {
		mfSaveErr = k.saveMemoryFiles(ctx, w, pagesMetadata, pagesFile, mfsToSave, mfOpts)
	}
	if mfSaveErr != nil {
		return mfSaveErr
	}

	log.Infof("Overall save took [%s].", time.Since(saveStart))
	return nil
}

// BeforeResume is called before the kernel is resumed after save.
func (k *Kernel) BeforeResume(ctx context.Context) {
	k.vfs.BeforeResume(ctx)
}

func (k *Kernel) saveMemoryFiles(ctx context.Context, w, pagesMetadata io.Writer, pagesFile *fd.FD, mfsToSave map[string]*pgalloc.MemoryFile, mfOpts pgalloc.SaveOpts) error {
	// Save the memory files' state.
	memoryStart := time.Now()
	pmw := w
	if pagesMetadata != nil {
		pmw = pagesMetadata
	}
	pw := w
	if pagesFile != nil {
		pw = pagesFile
	}
	if err := k.mf.SaveTo(ctx, pmw, pw, mfOpts); err != nil {
		return err
	}
	if err := savePrivateMFs(ctx, pmw, pw, mfsToSave, mfOpts); err != nil {
		return err
	}
	log.Infof("Memory files save took [%s].", time.Since(memoryStart))
	return nil
}

// Preconditions: The kernel must be paused.
func (k *Kernel) invalidateUnsavableMappings(ctx context.Context) error {
	invalidated := make(map[*mm.MemoryManager]struct{})
	k.tasks.mu.RLock()
	defer k.tasks.mu.RUnlock()
	for t := range k.tasks.Root.tids {
		// We can skip locking Task.mu here since the kernel is paused.
		if memMgr := t.image.MemoryManager; memMgr != nil {
			if _, ok := invalidated[memMgr]; !ok {
				if err := memMgr.InvalidateUnsavable(ctx); err != nil {
					return err
				}
				invalidated[memMgr] = struct{}{}
			}
		}
		// I really wish we just had a sync.Map of all MMs...
		if r, ok := t.runState.(*runSyscallAfterExecStop); ok {
			if err := r.image.MemoryManager.InvalidateUnsavable(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// LoadFrom returns a new Kernel loaded from args.
func (k *Kernel) LoadFrom(ctx context.Context, r io.Reader, asyncMFLoader *AsyncMFLoader, timeReady chan struct{}, net inet.Stack, clocks sentrytime.Clocks, vfsOpts *vfs.CompleteRestoreOptions, saveRestoreNet bool) error {
	loadStart := time.Now()

	k.runningTasksCond.L = &k.runningTasksMu
	k.cpuClockTickerWakeCh = make(chan struct{}, 1)
	k.cpuClockTickerStopCond.L = &k.runningTasksMu

	initAppCores := k.applicationCores

	// Load the pre-saved CPUID FeatureSet.
	//
	// N.B. This was also saved along with the full kernel below, so we
	// don't need to explicitly install it in the Kernel.
	cpuidStart := time.Now()
	if _, err := state.Load(ctx, r, &k.featureSet); err != nil {
		return err
	}
	log.Infof("CPUID load took [%s].", time.Since(cpuidStart))

	// Verify that the FeatureSet is usable on this host. We do this before
	// Kernel load so that the explicit CPUID mismatch error has priority
	// over floating point state restore errors that may occur on load on
	// an incompatible machine.
	if err := k.featureSet.CheckHostCompatible(); err != nil {
		return err
	}

	// Load the kernel state.
	kernelStart := time.Now()
	stats, err := state.Load(ctx, r, k)
	if err != nil {
		return err
	}
	log.Infof("Kernel load stats: %s", stats.String())
	log.Infof("Kernel load took [%s].", time.Since(kernelStart))

	if !saveRestoreNet {
		// rootNetworkNamespace and stack should be populated after
		// loading the state file. Reset the stack before restoring the
		// root network stack.
		k.rootNetworkNamespace.ResetStack()
		k.rootNetworkNamespace.RestoreRootStack(net)
	}

	if asyncMFLoader == nil {
		mfStart := time.Now()
		if err := k.loadMemoryFiles(ctx, r); err != nil {
			return fmt.Errorf("failed to load memory files: %w", err)
		}
		log.Infof("Memory files load took [%s].", time.Since(mfStart))
	} else {
		// Timekeeper restore below tries writing to the main MF. If async page
		// loading is being used, we need to make sure that the main MF loading has
		// started before we try to write to it.
		if err := asyncMFLoader.WaitMainMFStart(); err != nil {
			return fmt.Errorf("main MF start failed: %w", err)
		}
	}

	k.Timekeeper().SetClocks(clocks, k.vdsoParams)

	if timeReady != nil {
		close(timeReady)
	}

	if saveRestoreNet {
		log.Infof("netstack save restore is enabled")
		s := k.rootNetworkNamespace.Stack()
		if s == nil {
			panic("inet.Stack cannot be nil when netstack s/r is enabled")
		}
		if net != nil {
			s.ReplaceConfig(net)
		}
		s.Restore()
	} else if net != nil {
		net.Restore()
	}

	if err := k.vfs.CompleteRestore(ctx, vfsOpts); err != nil {
		return vfs.PrependErrMsg("vfs.CompleteRestore() failed", err)
	}

	log.Infof("Overall load took [%s] after async work", time.Since(loadStart))

	// Applications may size per-cpu structures based on k.applicationCores, so
	// it can't change across save/restore. When we are virtualizing CPU
	// numbers, this isn't a problem. However, when we are exposing host CPU
	// assignments, we can't tolerate an increase in the number of host CPUs,
	// which could result in getcpu(2) returning CPUs that applications expect
	// not to exist.
	if k.useHostCores && initAppCores > k.applicationCores {
		return fmt.Errorf("UseHostCores enabled: can't increase ApplicationCores from %d to %d after restore", k.applicationCores, initAppCores)
	}

	return nil
}

// UniqueID returns a unique identifier.
func (k *Kernel) UniqueID() uint64 {
	id := k.uniqueID.Add(1)
	if id == 0 {
		panic("unique identifier generator wrapped around")
	}
	return id
}

// CreateProcessArgs holds arguments to kernel.CreateProcess.
type CreateProcessArgs struct {
	// Filename is the filename to load as the init binary.
	//
	// If this is provided as "", File will be checked, then the file will be
	// guessed via Argv[0].
	Filename string

	// File is a passed host FD pointing to a file to load as the init binary.
	//
	// This is checked if and only if Filename is "".
	File *vfs.FileDescription

	// Argv is a list of arguments.
	Argv []string

	// Envv is a list of environment variables.
	Envv []string

	// WorkingDirectory is the initial working directory.
	//
	// This defaults to the root if empty.
	WorkingDirectory string

	// Credentials is the initial credentials.
	Credentials *auth.Credentials

	// FDTable is the initial set of file descriptors. If CreateProcess succeeds,
	// it takes a reference on FDTable.
	FDTable *FDTable

	// Umask is the initial umask.
	Umask uint

	// Limits are the initial resource limits.
	Limits *limits.LimitSet

	// MaxSymlinkTraversals is the maximum number of symlinks to follow
	// during resolution.
	MaxSymlinkTraversals uint

	// UTSNamespace is the initial UTS namespace.
	UTSNamespace *UTSNamespace

	// IPCNamespace is the initial IPC namespace.
	IPCNamespace *IPCNamespace

	// PIDNamespace is the initial PID Namespace.
	PIDNamespace *PIDNamespace

	// MountNamespace optionally contains the mount namespace for this
	// process. If nil, the init process's mount namespace is used.
	//
	// Anyone setting MountNamespace must donate a reference (i.e.
	// increment it).
	MountNamespace *vfs.MountNamespace

	// ContainerID is the container that the process belongs to.
	ContainerID string

	// InitialCgroups are the cgroups the container is initialized to.
	InitialCgroups map[Cgroup]struct{}

	// Origin indicates how the task was first created.
	Origin TaskOrigin

	// TTY is the optional controlling TTY to associate with this process.
	TTY *TTY
}

// NewContext returns a context.Context that represents the task that will be
// created by args.NewContext(k).
func (args *CreateProcessArgs) NewContext(k *Kernel) context.Context {
	return &createProcessContext{
		Context: context.Background(),
		kernel:  k,
		args:    args,
	}
}

// createProcessContext is a context.Context that represents the context
// associated with a task that is being created.
type createProcessContext struct {
	context.Context
	kernel *Kernel
	args   *CreateProcessArgs
}

// Value implements context.Context.Value.
func (ctx *createProcessContext) Value(key any) any {
	switch key {
	case CtxKernel:
		return ctx.kernel
	case CtxPIDNamespace:
		return ctx.args.PIDNamespace
	case CtxUTSNamespace:
		utsns := ctx.args.UTSNamespace
		utsns.IncRef()
		return utsns
	case ipc.CtxIPCNamespace:
		ipcns := ctx.args.IPCNamespace
		ipcns.IncRef()
		return ipcns
	case auth.CtxCredentials:
		return ctx.args.Credentials
	case vfs.CtxRoot:
		if ctx.args.MountNamespace == nil {
			return nil
		}
		root := ctx.args.MountNamespace.Root(ctx)
		return root
	case vfs.CtxMountNamespace:
		if ctx.kernel.globalInit == nil {
			return nil
		}
		mntns := ctx.kernel.GlobalInit().Leader().MountNamespace()
		mntns.IncRef()
		return mntns
	case devutil.CtxDevGoferClient:
		return ctx.kernel.GetDevGoferClient(ctx.kernel.ContainerName(ctx.args.ContainerID))
	case inet.CtxStack:
		return ctx.kernel.RootNetworkNamespace().Stack()
	case ktime.CtxRealtimeClock:
		return ctx.kernel.RealtimeClock()
	case limits.CtxLimits:
		return ctx.args.Limits
	case pgalloc.CtxMemoryCgroupID:
		return ctx.getMemoryCgroupID()
	case pgalloc.CtxMemoryFile:
		return ctx.kernel.mf
	case platform.CtxPlatform:
		return ctx.kernel
	case uniqueid.CtxGlobalUniqueID:
		return ctx.kernel.UniqueID()
	case uniqueid.CtxGlobalUniqueIDProvider:
		return ctx.kernel
	case uniqueid.CtxInotifyCookie:
		return ctx.kernel.GenerateInotifyCookie()
	case unimpl.CtxEvents:
		return ctx.kernel
	default:
		return nil
	}
}

func (ctx *createProcessContext) getMemoryCgroupID() uint32 {
	for cg := range ctx.args.InitialCgroups {
		for _, ctl := range cg.Controllers() {
			if ctl.Type() == CgroupControllerMemory {
				return cg.ID()
			}
		}
	}
	return InvalidCgroupID
}

// CreateProcess creates a new task in a new thread group with the given
// options. The new task has no parent and is in the root PID namespace.
//
// If k.Start() has already been called, then the created process must be
// started by calling kernel.StartProcess(tg).
//
// If k.Start() has not yet been called, then the created task will begin
// running when k.Start() is called.
//
// CreateProcess has no analogue in Linux; it is used to create the initial
// application task, as well as processes started by the control server.
func (k *Kernel) CreateProcess(args CreateProcessArgs) (*ThreadGroup, ThreadID, error) {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	log.Infof("EXEC: %v", args.Argv)

	ctx := args.NewContext(k)
	mntns := args.MountNamespace
	if mntns == nil {
		if k.globalInit == nil {
			return nil, 0, fmt.Errorf("mount namespace is nil")
		}
		// Add a reference to the namespace, which is transferred to the new process.
		mntns = k.globalInit.Leader().MountNamespace()
		mntns.IncRef()
	}
	// Get the root directory from the MountNamespace.
	root := mntns.Root(ctx)
	defer root.DecRef(ctx)

	// Grab the working directory.
	wd := root // Default.
	if args.WorkingDirectory != "" {
		pop := vfs.PathOperation{
			Root:               root,
			Start:              wd,
			Path:               fspath.Parse(args.WorkingDirectory),
			FollowFinalSymlink: true,
		}
		// NOTE(b/236028361): Do not set CheckSearchable flag to true.
		// Application is allowed to start with a working directory that it can
		// not access/search. This is consistent with Docker and VFS1. Runc
		// explicitly allows for this in 6ce2d63a5db6 ("libct/init_linux: retry
		// chdir to fix EPERM"). As described in the commit, runc unintentionally
		// allowed this behavior in a couple of releases and applications started
		// relying on it. So they decided to allow it for backward compatibility.
		var err error
		wd, err = k.VFS().GetDentryAt(ctx, args.Credentials, &pop, &vfs.GetDentryOptions{})
		if err != nil {
			return nil, 0, fmt.Errorf("failed to find initial working directory %q: %v", args.WorkingDirectory, err)
		}
		defer wd.DecRef(ctx)
	}
	fsContext := NewFSContext(root, wd, args.Umask)

	tg := k.NewThreadGroup(args.PIDNamespace, NewSignalHandlers(), linux.SIGCHLD, args.Limits)
	cu := cleanup.Make(func() {
		tg.Release(ctx)
	})
	defer cu.Clean()

	// Check which file to start from.
	switch {
	case args.Filename != "":
		// If a filename is given, take that.
		// Set File to nil so we resolve the path in LoadTaskImage.
		args.File = nil
	case args.File != nil:
		// If File is set, take the File provided directly.
		args.Filename = args.File.MappedName(ctx)
	default:
		// Otherwise look at Argv and see if the first argument is a valid path.
		if len(args.Argv) == 0 {
			return nil, 0, fmt.Errorf("no filename or command provided")
		}
		if !filepath.IsAbs(args.Argv[0]) {
			return nil, 0, fmt.Errorf("'%s' is not an absolute path", args.Argv[0])
		}
		args.Filename = args.Argv[0]
	}

	// Create a fresh task context.
	remainingTraversals := args.MaxSymlinkTraversals
	loadArgs := loader.LoadArgs{
		Root:                root,
		WorkingDir:          wd,
		RemainingTraversals: &remainingTraversals,
		ResolveFinal:        true,
		Filename:            args.Filename,
		File:                args.File,
		CloseOnExec:         false,
		Argv:                args.Argv,
		Envv:                args.Envv,
		Features:            k.featureSet,
	}

	image, se := k.LoadTaskImage(ctx, loadArgs)
	if se != nil {
		return nil, 0, errors.New(se.String())
	}
	if err := auth.UpdateCredsForNewTask(args.Credentials, image.FileCaps(), args.Filename); err != nil {
		return nil, 0, err
	}
	args.FDTable.IncRef()

	// Create the task.
	config := &TaskConfig{
		Kernel:           k,
		ThreadGroup:      tg,
		TaskImage:        image,
		FSContext:        fsContext,
		FDTable:          args.FDTable,
		Credentials:      args.Credentials,
		NetworkNamespace: k.RootNetworkNamespace(),
		AllowedCPUMask:   sched.NewFullCPUSet(k.applicationCores),
		UTSNamespace:     args.UTSNamespace,
		IPCNamespace:     args.IPCNamespace,
		MountNamespace:   mntns,
		ContainerID:      args.ContainerID,
		InitialCgroups:   args.InitialCgroups,
		UserCounters:     k.GetUserCounters(args.Credentials.RealKUID),
		Origin:           args.Origin,
		// A task with no parent starts out with no session keyring.
		SessionKeyring: nil,
	}
	config.UTSNamespace.IncRef()
	config.IPCNamespace.IncRef()
	config.NetworkNamespace.IncRef()
	t, err := k.tasks.NewTask(ctx, config)
	if err != nil {
		return nil, 0, err
	}
	t.traceExecEvent(image) // Simulate exec for tracing.

	// Set TTY if configured.
	if args.TTY != nil {
		if err := t.tg.SetControllingTTY(ctx, args.TTY, false /* steal */, true /* isReadable */); err != nil {
			return nil, 0, fmt.Errorf("setting controlling tty: %w", err)
		}
	}

	// Success.
	cu.Release()
	tgid := k.tasks.Root.IDOfThreadGroup(tg)
	if k.globalInit == nil {
		k.globalInit = tg
	}
	return tg, tgid, nil
}

// StartProcess starts running a process that was created with CreateProcess.
func (k *Kernel) StartProcess(tg *ThreadGroup) {
	t := tg.Leader()
	tid := k.tasks.Root.IDOfTask(t)
	t.Start(tid)
}

// Start starts execution of all tasks in k.
//
// Preconditions: Start may be called exactly once.
func (k *Kernel) Start() error {
	k.extMu.Lock()
	defer k.extMu.Unlock()

	if k.started {
		return fmt.Errorf("kernel already started")
	}

	k.started = true
	k.cpuClockTickTimer = time.NewTimer(linux.ClockTick)
	k.runningTasksMu.Lock()
	k.cpuClockTickerRunning = true
	k.runningTasksMu.Unlock()
	go k.runCPUClockTicker()
	// If k was created by LoadKernelFrom, timers were stopped during
	// Kernel.SaveTo and need to be resumed. If k was created by NewKernel,
	// this is a no-op.
	k.resumeTimeLocked(k.SupervisorContext())
	k.tasks.mu.RLock()
	ts := make([]*Task, 0, len(k.tasks.Root.tids))
	for t := range k.tasks.Root.tids {
		ts = append(ts, t)
	}
	k.tasks.mu.RUnlock()
	// Start task goroutines.
	// NOTE(b/235349091): We don't actually need the TaskSet mutex, we just
	// need to make sure we only call t.Start() once for each task. Holding the
	// mutex for each task start may cause a nested locking error.
	for _, t := range ts {
		t.Start(t.ThreadID())
	}
	return nil
}

// pauseTimeLocked pauses all Timers and Timekeeper updates.
//
// Preconditions:
//   - Any task goroutines running in k must be stopped.
//   - k.extMu must be locked.
func (k *Kernel) pauseTimeLocked(ctx context.Context) {
	// Since all task goroutines have been stopped by precondition, the CPU clock
	// ticker should stop on its own; wait for it to do so, waking it up from
	// sleeping between ticks if necessary.
	k.runningTasksMu.Lock()
	for k.cpuClockTickerRunning {
		select {
		case k.cpuClockTickerWakeCh <- struct{}{}:
		default:
		}
		k.cpuClockTickerStopCond.Wait()
	}
	k.runningTasksMu.Unlock()

	// By precondition, nothing else can be interacting with PIDNamespace.tids
	// or FDTable.files, so we can iterate them without synchronization. (We
	// can't hold the TaskSet mutex when pausing thread group timers because
	// thread group timers call ThreadGroup.SendSignal, which takes the TaskSet
	// mutex, while holding the Timer mutex.)
	for t := range k.tasks.Root.tids {
		if t == t.tg.leader {
			t.tg.itimerRealTimer.Pause()
			for _, it := range t.tg.timers {
				it.PauseTimer()
			}
		}
		// This means we'll iterate FDTables shared by multiple tasks repeatedly,
		// but ktime.Timer.Pause is idempotent so this is harmless.
		if t.fdTable != nil {
			t.fdTable.ForEach(ctx, func(_ int32, fd *vfs.FileDescription, _ FDFlags) bool {
				if tfd, ok := fd.Impl().(*timerfd.TimerFileDescription); ok {
					tfd.PauseTimer()
				}
				return true
			})
		}
	}
	k.timekeeper.PauseUpdates()
}

// resumeTimeLocked resumes all Timers and Timekeeper updates. If
// pauseTimeLocked has not been previously called, resumeTimeLocked has no
// effect.
//
// Preconditions:
//   - Any task goroutines running in k must be stopped.
//   - k.extMu must be locked.
func (k *Kernel) resumeTimeLocked(ctx context.Context) {
	// The CPU clock ticker will automatically resume as task goroutines resume
	// execution.

	k.timekeeper.ResumeUpdates(k.vdsoParams)
	for t := range k.tasks.Root.tids {
		if t == t.tg.leader {
			t.tg.itimerRealTimer.Resume()
			for _, it := range t.tg.timers {
				it.ResumeTimer()
			}
		}
		if t.fdTable != nil {
			t.fdTable.ForEach(ctx, func(_ int32, fd *vfs.FileDescription, _ FDFlags) bool {
				if tfd, ok := fd.Impl().(*timerfd.TimerFileDescription); ok {
					tfd.ResumeTimer()
				}
				return true
			})
		}
	}
}

func (k *Kernel) incRunningTasks() {
	for {
		tasks := k.runningTasks.Load()
		if tasks != 0 {
			// Standard case. Simply increment.
			if !k.runningTasks.CompareAndSwap(tasks, tasks+1) {
				continue
			}
			return
		}

		// Transition from 0 -> 1.
		k.runningTasksMu.Lock()
		if k.runningTasks.Load() != 0 {
			// Raced with another transition and lost.
			k.runningTasks.Add(1)
			k.runningTasksMu.Unlock()
			return
		}
		if !k.cpuClockTickerRunning {
			select {
			case tickTime := <-k.cpuClockTickTimer.C:
				// Rearm the timer since we consumed the wakeup. Estimate how much time
				// remains on the current tick so that periodic workloads interact with
				// the (periodic) CPU clock ticker in the same way that they would
				// without the optimization of putting the ticker to sleep.
				missedNS := time.Since(tickTime).Nanoseconds()
				missedTicks := missedNS / linux.ClockTick.Nanoseconds()
				thisTickNS := missedNS - missedTicks*linux.ClockTick.Nanoseconds()
				k.cpuClockTickTimer.Reset(time.Duration(linux.ClockTick.Nanoseconds() - thisTickNS))
				// Increment k.cpuClock on the CPU clock ticker goroutine's behalf.
				// (Whole missed ticks don't matter, and adding them to k.cpuClock will
				// just confuse the watchdog.) At the time the tick occurred, all task
				// goroutines were asleep, so there's nothing else to do. This ensures
				// that our caller (Task.accountTaskGoroutineLeave()) records an
				// updated k.cpuClock in Task.gosched.Timestamp, so that it's correctly
				// accounted as having resumed execution in the sentry during this tick
				// instead of at the end of the previous one.
				k.cpuClock.Add(1)
			default:
			}
			// We are transitioning from idle to active. Set k.cpuClockTickerRunning
			// = true here so that if we transition to idle and then active again
			// before the CPU clock ticker goroutine has a chance to run, the first
			// call to k.incRunningTasks() at the end of that cycle does not try to
			// steal k.cpuClockTickTimer.C again, as this would allow workloads that
			// rapidly cycle between idle and active to starve the CPU clock ticker
			// of chances to observe task goroutines in a running state and account
			// their CPU usage.
			k.cpuClockTickerRunning = true
			k.runningTasksCond.Signal()
		}
		// This store must happen after the increment of k.cpuClock above to ensure
		// that concurrent calls to Task.accountTaskGoroutineLeave() also observe
		// the updated k.cpuClock.
		k.runningTasks.Store(1)
		k.runningTasksMu.Unlock()
		return
	}
}

func (k *Kernel) decRunningTasks() {
	tasks := k.runningTasks.Add(-1)
	if tasks < 0 {
		panic(fmt.Sprintf("Invalid running count %d", tasks))
	}

	// Nothing to do. The next CPU clock tick will disable the timer if
	// there is still nothing running. This provides approximately one tick
	// of slack in which we can switch back and forth between idle and
	// active without an expensive transition.
}

// WaitExited blocks until all tasks in k have exited. No tasks can be created
// after WaitExited returns.
func (k *Kernel) WaitExited() {
	k.tasks.mu.Lock()
	defer k.tasks.mu.Unlock()
	k.tasks.noNewTasksIfZeroLive = true
	for k.tasks.liveTasks != 0 {
		k.tasks.zeroLiveTasksCond.Wait()
	}
}

// Kill requests that all tasks in k immediately exit as if group exiting with
// status ws. Kill does not wait for tasks to exit.
func (k *Kernel) Kill(ws linux.WaitStatus) {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	k.tasks.Kill(ws)
}

// Pause requests that all tasks in k temporarily stop executing, and blocks
// until all tasks and asynchronous I/O operations in k have stopped. Multiple
// calls to Pause nest and require an equal number of calls to Unpause to
// resume execution.
func (k *Kernel) Pause() {
	k.extMu.Lock()
	k.tasks.BeginExternalStop()
	k.extMu.Unlock()
	k.tasks.runningGoroutines.Wait()
	k.tasks.aioGoroutines.Wait()
}

// IsPaused returns true if the kernel is currently paused.
func (k *Kernel) IsPaused() bool {
	return k.tasks.isExternallyStopped()
}

// ReceiveTaskStates receives full states for all tasks.
func (k *Kernel) ReceiveTaskStates() {
	k.extMu.Lock()
	k.tasks.PullFullState()
	k.extMu.Unlock()
}

// Unpause ends the effect of a previous call to Pause. If Unpause is called
// without a matching preceding call to Pause, Unpause may panic.
func (k *Kernel) Unpause() {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	k.tasks.EndExternalStop()
}

// SendExternalSignal injects a signal into the kernel.
//
// context is used only for debugging to describe how the signal was received.
//
// Preconditions: Kernel must have an init process.
func (k *Kernel) SendExternalSignal(info *linux.SignalInfo, context string) {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	k.sendExternalSignal(info, context)
}

// SendExternalSignalThreadGroup injects a signal into an specific ThreadGroup.
//
// This function doesn't skip signals like SendExternalSignal does.
func (k *Kernel) SendExternalSignalThreadGroup(tg *ThreadGroup, info *linux.SignalInfo) error {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	return tg.SendSignal(info)
}

// SendExternalSignalProcessGroup sends a signal to all ThreadGroups in the
// given process group.
//
// This function doesn't skip signals like SendExternalSignal does.
func (k *Kernel) SendExternalSignalProcessGroup(pg *ProcessGroup, info *linux.SignalInfo) error {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	// If anything goes wrong, we'll return the error, but still try our
	// best to deliver to other processes in the group.
	var firstErr error
	for _, tg := range k.TaskSet().Root.ThreadGroups() {
		if tg.ProcessGroup() != pg {
			continue
		}
		if err := tg.SendSignal(info); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SendContainerSignal sends the given signal to all processes inside the
// namespace that match the given container ID.
func (k *Kernel) SendContainerSignal(cid string, info *linux.SignalInfo) error {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	k.tasks.mu.RLock()
	defer k.tasks.mu.RUnlock()

	var lastErr error
	for tg := range k.tasks.Root.tgids {
		if tg.leader.ContainerID() == cid {
			tg.signalHandlers.mu.Lock()
			infoCopy := *info
			if err := tg.leader.sendSignalLocked(&infoCopy, true /*group*/); err != nil {
				lastErr = err
			}
			tg.signalHandlers.mu.Unlock()
		}
	}
	return lastErr
}

// RebuildTraceContexts rebuilds the trace context for all tasks.
//
// Unfortunately, if these are built while tracing is not enabled, then we will
// not have meaningful trace data. Rebuilding here ensures that we can do so
// after tracing has been enabled.
func (k *Kernel) RebuildTraceContexts() {
	// We need to pause all task goroutines because Task.rebuildTraceContext()
	// replaces Task.traceContext and Task.traceTask, which are
	// task-goroutine-exclusive (i.e. the task goroutine assumes that it can
	// access them without synchronization) for performance.
	k.Pause()
	defer k.Unpause()

	k.extMu.Lock()
	defer k.extMu.Unlock()
	k.tasks.mu.RLock()
	defer k.tasks.mu.RUnlock()

	for t, tid := range k.tasks.Root.tids {
		t.rebuildTraceContext(tid)
	}
}

// FeatureSet returns the FeatureSet.
func (k *Kernel) FeatureSet() cpuid.FeatureSet {
	return k.featureSet
}

// Timekeeper returns the Timekeeper.
func (k *Kernel) Timekeeper() *Timekeeper {
	return k.timekeeper
}

// TaskSet returns the TaskSet.
func (k *Kernel) TaskSet() *TaskSet {
	return k.tasks
}

// RootUserNamespace returns the root UserNamespace.
func (k *Kernel) RootUserNamespace() *auth.UserNamespace {
	return k.rootUserNamespace
}

// RootUTSNamespace returns the root UTSNamespace.
func (k *Kernel) RootUTSNamespace() *UTSNamespace {
	return k.rootUTSNamespace
}

// RootIPCNamespace takes a reference and returns the root IPCNamespace.
func (k *Kernel) RootIPCNamespace() *IPCNamespace {
	return k.rootIPCNamespace
}

// RootPIDNamespace returns the root PIDNamespace.
func (k *Kernel) RootPIDNamespace() *PIDNamespace {
	return k.tasks.Root
}

// RootNetworkNamespace returns the root network namespace, always non-nil.
func (k *Kernel) RootNetworkNamespace() *inet.Namespace {
	return k.rootNetworkNamespace
}

// GlobalInit returns the thread group with ID 1 in the root PID namespace, or
// nil if no such thread group exists. GlobalInit may return a thread group
// containing no tasks if the thread group has already exited.
func (k *Kernel) GlobalInit() *ThreadGroup {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	return k.globalInit
}

// TestOnlySetGlobalInit sets the thread group with ID 1 in the root PID namespace.
func (k *Kernel) TestOnlySetGlobalInit(tg *ThreadGroup) {
	k.globalInit = tg
}

// ApplicationCores returns the number of CPUs visible to sandboxed
// applications.
func (k *Kernel) ApplicationCores() uint {
	return k.applicationCores
}

// RealtimeClock returns the application CLOCK_REALTIME clock.
func (k *Kernel) RealtimeClock() ktime.SampledClock {
	return k.timekeeper.realtimeClock
}

// MonotonicClock returns the application CLOCK_MONOTONIC clock.
func (k *Kernel) MonotonicClock() ktime.SampledClock {
	return k.timekeeper.monotonicClock
}

// Syslog returns the syslog.
func (k *Kernel) Syslog() *syslog {
	return &k.syslog
}

// GenerateInotifyCookie generates a unique inotify event cookie.
//
// Returned values may overlap with previously returned values if the value
// space is exhausted. 0 is not a valid cookie value, all other values
// representable in a uint32 are allowed.
func (k *Kernel) GenerateInotifyCookie() uint32 {
	id := k.nextInotifyCookie.Add(1)
	// Wrap-around is explicitly allowed for inotify event cookies.
	if id == 0 {
		id = k.nextInotifyCookie.Add(1)
	}
	return id
}

// NetlinkPorts returns the netlink port manager.
func (k *Kernel) NetlinkPorts() *port.Manager {
	return k.netlinkPorts
}

var (
	errSaved     = errors.New("sandbox has been successfully saved")
	errAutoSaved = errors.New("sandbox has been successfully auto-saved")
)

// SaveStatus returns the sandbox save status. If it was saved successfully,
// autosaved indicates whether save was triggered by autosave. If it was not
// saved successfully, err indicates the sandbox error that caused the kernel to
// exit during save.
func (k *Kernel) SaveStatus() (saved, autosaved bool, err error) {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	switch k.saveStatus {
	case nil:
		return false, false, nil
	case errSaved:
		return true, false, nil
	case errAutoSaved:
		return true, true, nil
	default:
		return false, false, k.saveStatus
	}
}

// SetSaveSuccess sets the flag indicating that save completed successfully, if
// no status was already set.
func (k *Kernel) SetSaveSuccess(autosave bool) {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	if k.saveStatus == nil {
		if autosave {
			k.saveStatus = errAutoSaved
		} else {
			k.saveStatus = errSaved
		}
	}
}

// SetSaveError sets the sandbox error that caused the kernel to exit during
// save, if one is not already set.
func (k *Kernel) SetSaveError(err error) {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	if k.saveStatus == nil {
		k.saveStatus = err
	}
}

// SetMemoryFile sets Kernel.mf. SetMemoryFile must be called before Init or
// LoadFrom.
func (k *Kernel) SetMemoryFile(mf *pgalloc.MemoryFile) {
	k.mf = mf
}

// MemoryFile returns the MemoryFile that provides application memory.
func (k *Kernel) MemoryFile() *pgalloc.MemoryFile {
	return k.mf
}

// SupervisorContext returns a Context with maximum privileges in k. It should
// only be used by goroutines outside the control of the emulated kernel
// defined by e.
//
// Callers are responsible for ensuring that the returned Context is not used
// concurrently with changes to the Kernel.
func (k *Kernel) SupervisorContext() context.Context {
	return &supervisorContext{
		Kernel: k,
		Logger: log.Log(),
	}
}

// SocketRecord represents a socket recorded in Kernel.sockets.
//
// +stateify savable
type SocketRecord struct {
	k    *Kernel
	Sock *vfs.FileDescription
	ID   uint64 // Socket table entry number.
}

// RecordSocket adds a socket to the system-wide socket table for
// tracking.
//
// Precondition: Caller must hold a reference to sock.
//
// Note that the socket table will not hold a reference on the
// vfs.FileDescription.
func (k *Kernel) RecordSocket(sock *vfs.FileDescription) {
	k.extMu.Lock()
	if _, ok := k.sockets[sock]; ok {
		panic(fmt.Sprintf("Socket %p added twice", sock))
	}
	id := k.nextSocketRecord
	k.nextSocketRecord++
	s := &SocketRecord{
		k:    k,
		ID:   id,
		Sock: sock,
	}
	k.sockets[sock] = s
	k.extMu.Unlock()
}

// DeleteSocket removes a socket from the system-wide socket table.
func (k *Kernel) DeleteSocket(sock *vfs.FileDescription) {
	k.extMu.Lock()
	delete(k.sockets, sock)
	k.extMu.Unlock()
}

// ListSockets returns a snapshot of all sockets.
//
// Callers of ListSockets() should use SocketRecord.Sock.TryIncRef()
// to get a reference on a socket in the table.
func (k *Kernel) ListSockets() []*SocketRecord {
	k.extMu.Lock()
	var socks []*SocketRecord
	for _, s := range k.sockets {
		socks = append(socks, s)
	}
	k.extMu.Unlock()
	return socks
}

// supervisorContext is a privileged context.
type supervisorContext struct {
	context.NoTask
	log.Logger
	*Kernel
}

// Deadline implements context.Context.Deadline.
func (*Kernel) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

// Done implements context.Context.Done.
func (*Kernel) Done() <-chan struct{} {
	return nil
}

// Err implements context.Context.Err.
func (*Kernel) Err() error {
	return nil
}

// Value implements context.Context.
func (ctx *supervisorContext) Value(key any) any {
	switch key {
	case CtxCanTrace:
		// The supervisor context can trace anything. (None of
		// supervisorContext's users are expected to invoke ptrace, but ptrace
		// permissions are required for certain file accesses.)
		return func(*Task, bool) bool { return true }
	case CtxKernel:
		return ctx.Kernel
	case CtxPIDNamespace:
		return ctx.Kernel.tasks.Root
	case CtxUTSNamespace:
		utsns := ctx.Kernel.rootUTSNamespace
		utsns.IncRef()
		return utsns
	case ipc.CtxIPCNamespace:
		ipcns := ctx.Kernel.rootIPCNamespace
		ipcns.IncRef()
		return ipcns
	case auth.CtxCredentials:
		// The supervisor context is global root.
		return auth.NewRootCredentials(ctx.Kernel.rootUserNamespace)
	case vfs.CtxRoot:
		if ctx.Kernel.globalInit == nil || ctx.Kernel.globalInit.Leader() == nil {
			return vfs.VirtualDentry{}
		}
		root := ctx.Kernel.GlobalInit().Leader().MountNamespace().Root(ctx)
		return root
	case vfs.CtxMountNamespace:
		if ctx.Kernel.globalInit == nil || ctx.Kernel.globalInit.Leader() == nil {
			return nil
		}
		mntns := ctx.Kernel.GlobalInit().Leader().MountNamespace()
		mntns.IncRef()
		return mntns
	case inet.CtxStack:
		return ctx.Kernel.RootNetworkNamespace().Stack()
	case ktime.CtxRealtimeClock:
		return ctx.Kernel.RealtimeClock()
	case limits.CtxLimits:
		// No limits apply.
		return limits.NewLimitSet()
	case pgalloc.CtxMemoryFile:
		return ctx.Kernel.mf
	case platform.CtxPlatform:
		return ctx.Kernel
	case uniqueid.CtxGlobalUniqueID:
		return ctx.Kernel.UniqueID()
	case uniqueid.CtxGlobalUniqueIDProvider:
		return ctx.Kernel
	case uniqueid.CtxInotifyCookie:
		return ctx.Kernel.GenerateInotifyCookie()
	case unimpl.CtxEvents:
		return ctx.Kernel
	case cpuid.CtxFeatureSet:
		return ctx.Kernel.featureSet
	default:
		return nil
	}
}

// Rate limits for the number of unimplemented syscall events.
const (
	unimplementedSyscallsMaxRate = 100  // events per second
	unimplementedSyscallBurst    = 1000 // events
)

// EmitUnimplementedEvent emits an UnimplementedSyscall event via the event
// channel.
func (k *Kernel) EmitUnimplementedEvent(ctx context.Context, sysno uintptr) {
	k.unimplementedSyscallEmitterOnce.Do(func() {
		k.unimplementedSyscallEmitter = eventchannel.RateLimitedEmitterFrom(eventchannel.DefaultEmitter, unimplementedSyscallsMaxRate, unimplementedSyscallBurst)
	})

	t := TaskFromContext(ctx)
	IncrementUnimplementedSyscallCounter(sysno)
	_, _ = k.unimplementedSyscallEmitter.Emit(&uspb.UnimplementedSyscall{
		Tid:       int32(t.ThreadID()),
		Registers: t.Arch().StateData().Proto(),
	})
}

// VFS returns the virtual filesystem for the kernel.
func (k *Kernel) VFS() *vfs.VirtualFilesystem {
	return &k.vfs
}

// SetHostMount sets the hostfs mount.
func (k *Kernel) SetHostMount(mnt *vfs.Mount) {
	if k.hostMount != nil {
		panic("Kernel.hostMount cannot be set more than once")
	}
	k.hostMount = mnt
}

// HostMount returns the hostfs mount.
func (k *Kernel) HostMount() *vfs.Mount {
	return k.hostMount
}

// PipeMount returns the pipefs mount.
func (k *Kernel) PipeMount() *vfs.Mount {
	return k.pipeMount
}

// GetNamespaceInode returns a new nsfs inode which serves as a reference counter for the namespace.
func (k *Kernel) GetNamespaceInode(ctx context.Context, ns vfs.Namespace) refs.TryRefCounter {
	return nsfs.NewInode(ctx, k.nsfsMount, ns)
}

// ShmMount returns the tmpfs mount.
func (k *Kernel) ShmMount() *vfs.Mount {
	return k.shmMount
}

// SocketMount returns the sockfs mount.
func (k *Kernel) SocketMount() *vfs.Mount {
	return k.socketMount
}

// CgroupRegistry returns the cgroup registry.
func (k *Kernel) CgroupRegistry() *CgroupRegistry {
	return k.cgroupRegistry
}

// AddCgroupMount adds the cgroup mounts to the cgroupMountsMap. These cgroup
// mounts are created during the creation of root container process and the
// reference ownership is transferred to the kernel.
func (k *Kernel) AddCgroupMount(ctl string, mnt *CgroupMount) {
	k.cgroupMountsMapMu.Lock()
	defer k.cgroupMountsMapMu.Unlock()

	if k.cgroupMountsMap == nil {
		k.cgroupMountsMap = make(map[string]*CgroupMount)
	}
	k.cgroupMountsMap[ctl] = mnt
}

// GetCgroupMount returns the cgroup mount for the given cgroup controller.
func (k *Kernel) GetCgroupMount(ctl string) *CgroupMount {
	k.cgroupMountsMapMu.Lock()
	defer k.cgroupMountsMapMu.Unlock()

	return k.cgroupMountsMap[ctl]
}

// releaseCgroupMounts releases the cgroup mounts.
func (k *Kernel) releaseCgroupMounts(ctx context.Context) {
	k.cgroupMountsMapMu.Lock()
	defer k.cgroupMountsMapMu.Unlock()

	for _, m := range k.cgroupMountsMap {
		m.Mount.DecRef(ctx)
		m.Root.DecRef(ctx)
		m.Fs.DecRef(ctx)
	}
}

// Release releases resources owned by k.
//
// Precondition: This should only be called after the kernel is fully
// initialized, e.g. after k.Start() has been called.
func (k *Kernel) Release() {
	ctx := k.SupervisorContext()
	k.releaseCgroupMounts(ctx)
	k.hostMount.DecRef(ctx)
	k.pipeMount.DecRef(ctx)
	k.nsfsMount.DecRef(ctx)
	k.shmMount.DecRef(ctx)
	k.socketMount.DecRef(ctx)
	k.vfs.Release(ctx)
	k.timekeeper.Destroy()
	k.vdso.Release(ctx)
	k.RootNetworkNamespace().DecRef(ctx)
	k.rootIPCNamespace.DecRef(ctx)
	k.rootUTSNamespace.DecRef(ctx)
	k.cleaupDevGofers()
	k.mf.Destroy()
	k.RootPIDNamespace().DecRef(ctx)
}

// PopulateNewCgroupHierarchy moves all tasks into a newly created cgroup
// hierarchy.
//
// Precondition: root must be a new cgroup with no tasks. This implies the
// controllers for root are also new and currently manage no task, which in turn
// implies the new cgroup can be populated without migrating tasks between
// cgroups.
func (k *Kernel) PopulateNewCgroupHierarchy(root Cgroup) {
	k.tasks.mu.RLock()
	k.tasks.forEachTaskLocked(func(t *Task) {
		if t.exitStateLocked() != TaskExitNone {
			return
		}
		t.mu.Lock()
		// A task can be in the cgroup if it has been created after the
		// cgroup hierarchy was registered.
		t.enterCgroupIfNotYetLocked(root)
		t.mu.Unlock()
	})
	k.tasks.mu.RUnlock()
}

// ReleaseCgroupHierarchy moves all tasks out of all cgroups belonging to the
// hierarchy with the provided id.  This is intended for use during hierarchy
// teardown, as otherwise the tasks would be orphaned w.r.t to some controllers.
func (k *Kernel) ReleaseCgroupHierarchy(hid uint32) {
	var releasedCGs []Cgroup

	k.tasks.mu.RLock()
	// We'll have one cgroup per hierarchy per task.
	releasedCGs = make([]Cgroup, 0, len(k.tasks.Root.tids))
	k.tasks.forEachTaskLocked(func(t *Task) {
		if t.exitStateLocked() != TaskExitNone {
			return
		}
		t.mu.Lock()
		for cg := range t.cgroups {
			if cg.HierarchyID() == hid {
				cg.Leave(t)
				t.ResetMemCgIDFromCgroup(cg)
				delete(t.cgroups, cg)
				releasedCGs = append(releasedCGs, cg)
				// A task can't be part of multiple cgroups from the same
				// hierarchy, so we can skip checking the rest once we find a
				// match.
				break
			}
		}
		t.mu.Unlock()
	})
	k.tasks.mu.RUnlock()

	for _, c := range releasedCGs {
		c.decRef()
	}
}

// ReplaceFSContextRoots updates root and cwd to `newRoot` in the FSContext
// across all tasks whose old root or cwd were `oldRoot`.
func (k *Kernel) ReplaceFSContextRoots(ctx context.Context, oldRoot vfs.VirtualDentry, newRoot vfs.VirtualDentry) {
	k.tasks.mu.RLock()
	oldRootDecRefs := 0
	k.tasks.forEachTaskLocked(func(t *Task) {
		t.mu.Lock()
		defer t.mu.Unlock()
		if fsc := t.fsContext; fsc != nil {
			fsc.mu.Lock()
			defer fsc.mu.Unlock()
			if fsc.root == oldRoot {
				newRoot.IncRef()
				oldRootDecRefs++
				fsc.root = newRoot
			}
			if fsc.cwd == oldRoot {
				newRoot.IncRef()
				oldRootDecRefs++
				fsc.cwd = newRoot
			}
		}
	})
	k.tasks.mu.RUnlock()
	for i := 0; i < oldRootDecRefs; i++ {
		oldRoot.DecRef(ctx)
	}
}

// GetUserCounters returns the user counters for the given KUID.
func (k *Kernel) GetUserCounters(uid auth.KUID) *UserCounters {
	k.userCountersMapMu.Lock()
	defer k.userCountersMapMu.Unlock()

	if uc, ok := k.userCountersMap[uid]; ok {
		return uc
	}

	uc := &UserCounters{}
	k.userCountersMap[uid] = uc
	return uc
}

// AddDevGofer initializes the dev gofer connection and starts tracking it.
// It takes ownership of goferFD.
func (k *Kernel) AddDevGofer(contName string, goferFD int) error {
	client, err := devutil.NewGoferClient(k.SupervisorContext(), contName, goferFD)
	if err != nil {
		return err
	}

	k.devGofersMu.Lock()
	defer k.devGofersMu.Unlock()
	if k.devGofers == nil {
		k.devGofers = make(map[string]*devutil.GoferClient)
	}
	k.devGofers[contName] = client
	return nil
}

// RemoveDevGofer closes the dev gofer connection, if one exists, and stops
// tracking it.
func (k *Kernel) RemoveDevGofer(contName string) {
	k.devGofersMu.Lock()
	defer k.devGofersMu.Unlock()
	client, ok := k.devGofers[contName]
	if !ok {
		return
	}
	client.Close()
	delete(k.devGofers, contName)
}

// GetDevGoferClient implements
// devutil.GoferClientProviderFromContext.GetDevGoferClient.
func (k *Kernel) GetDevGoferClient(contName string) *devutil.GoferClient {
	k.devGofersMu.Lock()
	defer k.devGofersMu.Unlock()
	return k.devGofers[contName]
}

func (k *Kernel) cleaupDevGofers() {
	k.devGofersMu.Lock()
	defer k.devGofersMu.Unlock()
	for _, client := range k.devGofers {
		client.Close()
	}
	k.devGofers = nil
}

// RegisterContainerName registers a container name for a given container ID.
func (k *Kernel) RegisterContainerName(cid, containerName string) {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	k.containerNames[cid] = containerName
}

// RestoreContainerMapping remaps old container IDs to new ones after a restore.
// containerIDs maps "name -> new container ID". Note that container names remain
// constant between restore sessions.
func (k *Kernel) RestoreContainerMapping(containerIDs map[string]string) {
	k.extMu.Lock()
	defer k.extMu.Unlock()

	// Delete mapping from old session and replace with new values.
	k.containerNames = make(map[string]string)
	for name, cid := range containerIDs {
		k.containerNames[cid] = name
	}
}

// ContainerName returns the container name for a given container ID.
func (k *Kernel) ContainerName(cid string) string {
	k.extMu.Lock()
	defer k.extMu.Unlock()
	return k.containerNames[cid]
}
