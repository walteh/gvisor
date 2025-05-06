//go:build darwin

package cgroup

// --- Darwin Porting Status ---
//
// PROBLEM:
//   Linux cgroups functionality (filesystem interactions, constants like
//   unix.CGROUP2_SUPER_MAGIC) is not available on Darwin.
//
// LINUX_SPECIFIC:
//   - Cgroups v1 & v2 filesystem layout (/sys/fs/cgroup/...)
//   - unix.CGROUP2_SUPER_MAGIC
//   - Cgroup controller logic (cpu, memory, etc.)
//
// DARWIN_EQUIVALENT:
//   - N/A for cgroups.
//   - Resource limits via `setrlimit(2)`, potential info via `sysctl`.
//
// CURRENT_APPROACH (as of review):
//   - Runtime Check: The main `new()` constructor checks `runtime.GOOS == "darwin"`.
//   - Stub Implementation: If Darwin is detected, `new()` returns an internal,
//     unexported `noopCgroup` which presumably implements the `Cgroup` interface
//     with no-op or error-returning methods.
//   - Shared Code: Linux-specific logic (structs like `cgroupV1`, methods,
//     filesystem access) remains within the common `cgroup.go` file.
//   - Build: This currently builds on Darwin, implying Linux-specific code paths
//     are either guarded, optimized out, or not causing fatal compile errors.
//
// REQUIRED_ACTION:
//   - [VERIFY]: Ensure the `noopCgroup` implementation correctly stubs all
//     `Cgroup` interface methods needed by callers.
//   - [REVIEW]: Re-evaluate if separate `_darwin.go` / `_linux.go` files with
//     build tags would be cleaner or more robust long-term, despite current
//     build success.
//   - [FUTURE]: Consider implementing resource limit mapping using Darwin APIs
//     (e.g., `setrlimit`) within the Darwin-specific logic if/when needed.
//
// IMPACT_IF_STUBBED:
//   All cgroup-based resource limiting specified in OCI spec is ignored.
//
// PRIORITY:
//   - IMPORTANT (Resource limiting feature missing).
//   - INFO (Currently builds, not an immediate blocker).
//
// --- End Darwin Porting Status ---
