//go:build darwin

package coretag

// --- Darwin Porting Status ---
//
// PROBLEM:
//   Uses Linux-specific prctl(2) syscall for Core Scheduling.
//
// LINUX_SPECIFIC:
//   - unix.SYS_PRCTL
//   - unix.PR_SCHED_CORE
//   - unix.PR_SCHED_CORE_CREATE
//   - unix.PR_SCHED_CORE_GET
//   - /proc/[pid]/task (used by GetAllCoreTags implementation)
//
// DARWIN_EQUIVALENT:
//   - N/A for Core Scheduling.
//   - Process/thread info via sysctl or other macOS APIs (if tag reading needed).
//
// REQUIRED_ACTION:
//   1. [STUB]: Create `coretag_darwin.go` with stubs for `Enable` and
//      `GetAllCoreTags` returning `unix.EOPNOTSUPP`.
//   2. [BUILD_TAG]: Add `//go:build linux` to `coretag.go` and `coretag_unsafe.go`.
//
// IMPACT_IF_STUBBED:
//   Core tagging feature (CPU side-channel mitigation) is disabled.
//
// PRIORITY:
//   - LOW (Feature is a security enhancement, stubbing allows build progress).
//   - BLOCKER (as it currently fails the build).
//
// --- End Darwin Porting Status ---
