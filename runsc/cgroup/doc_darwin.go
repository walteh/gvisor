//go:build darwin

package cgroup

// --- Darwin Porting Status ---
//
// PROBLEM:
//   1. Uses Linux-specific cgroup constants (unix.CGROUP2_SUPER_MAGIC).
//   2. Relies entirely on the Linux cgroup filesystem structure and files for
//      resource control.
//   3. Type definitions required by stub function signatures (Cgroup, controller
//      interfaces, etc.) are in Linux-only files (cgroup.go, cgroup_v2.go).
//
// LINUX_SPECIFIC:
//   - Cgroups v1 & v2 filesystem layout (/sys/fs/cgroup/...)
//   - unix.CGROUP2_SUPER_MAGIC
//   - All controller logic files (cpu.go, memory.go, etc.)
//
// DARWIN_EQUIVALENT:
//   - N/A for cgroups.
//   - Resource limits set via `setrlimit(2)`.
//   - Potential process properties via `sysctl`.
//
// REQUIRED_ACTION:
//   1. [MOVE_TYPE]: Relocate common interfaces/types (`Cgroup` interface,
//      potentially `controller` interface, helper structs/funcs if needed by
//      stubs or future impl) from Linux files to `cgroup_defs.go`.
//   2. [STUB]: Create `cgroup_darwin.go` with `darwinCgroup` struct implementing
//      `Cgroup` with no-op/error methods (returning `unix.EOPNOTSUPP` for getters).
//      Define `CGROUP2_SUPER_MAGIC=0`, implement `IsOnlyV2` returning false.
//      Implement `NewFromPath`, `NewFromPid` returning the stub.
//   3. [BUILD_TAG]: Add `//go:build linux` to `cgroup.go`, `cgroup_v2.go`, etc.
//      AFTER types are moved.
//
// IMPACT_IF_STUBBED:
//   All cgroup-based resource limiting specified in OCI spec is ignored.
//   Runsc may fail later if it strictly expects cgroups to be functional.
//
// PRIORITY:
//   - IMPORTANT (Resource limiting is a key container feature).
//   - BLOCKER (Due to type definition issues causing build failures).
//
// --- End Darwin Porting Status ---
