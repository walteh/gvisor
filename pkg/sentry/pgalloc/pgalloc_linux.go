//go:build linux
// +build linux

package pgalloc

import (
	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/hostarch"
	"gvisor.dev/gvisor/pkg/log"
	"gvisor.dev/gvisor/pkg/safemem"
	"gvisor.dev/gvisor/pkg/sentry/memmap"
)

func (f *MemoryFile) madviseChunkMapping(addr, len uintptr, huge bool) {
	if huge {
		if f.opts.AdviseHugepage {
			_, _, errno := unix.Syscall(unix.SYS_MADVISE, addr, len, unix.MADV_HUGEPAGE)
			if errno != 0 {
				// Log this failure but continue.
				log.Warningf("madvise(%#x, %d, MADV_HUGEPAGE) failed: %s", addr, len, errno)
			}
		}
	} else {
		if f.opts.AdviseNoHugepage {
			_, _, errno := unix.Syscall(unix.SYS_MADVISE, addr, len, unix.MADV_NOHUGEPAGE)
			if errno != 0 {
				// Log this failure but continue.
				log.Warningf("madvise(%#x, %d, MADV_NOHUGEPAGE) failed: %s", addr, len, errno)
			}
		}
	}
}

func tryPopulateMadv(b safemem.Block) bool {
	if madvPopulateWriteDisabled.Load() != 0 {
		return false
	}
	// Only call madvise(MADV_POPULATE_WRITE) if >=2 pages are being populated.
	// 1 syscall overhead >= 1 page fault overhead. This is because syscalls are
	// susceptible to additional overheads like seccomp-bpf filters and auditing.
	if b.Len() <= hostarch.PageSize {
		return true
	}
	_, _, errno := unix.Syscall(unix.SYS_MADVISE, b.Addr(), uintptr(b.Len()), unix.MADV_POPULATE_WRITE)
	if errno != 0 {
		if errno == unix.EINVAL {
			// EINVAL is expected if MADV_POPULATE_WRITE is not supported (Linux <5.14).
			log.Infof("Disabling pgalloc.MemoryFile.AllocateAndFill pre-population: madvise failed: %s", errno)
		} else {
			log.Warningf("Disabling pgalloc.MemoryFile.AllocateAndFill pre-population: madvise failed: %s", errno)
		}
		madvPopulateWriteDisabled.Store(1)
		return false
	}
	return true
}

func (f *MemoryFile) commitFile(fr memmap.FileRange) error {
	// "The default operation (i.e., mode is zero) of fallocate() allocates the
	// disk space within the range specified by offset and len." - fallocate(2)
	return unix.Fallocate(
		int(f.file.Fd()),
		0, // mode
		int64(fr.Start),
		int64(fr.Length()))
}

func (f *MemoryFile) decommitFile(fr memmap.FileRange) error {
	// "After a successful call, subsequent reads from this range will
	// return zeroes. The FALLOC_FL_PUNCH_HOLE flag must be ORed with
	// FALLOC_FL_KEEP_SIZE in mode ..." - fallocate(2)
	return unix.Fallocate(
		int(f.file.Fd()),
		unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE,
		int64(fr.Start),
		int64(fr.Length()))
}
