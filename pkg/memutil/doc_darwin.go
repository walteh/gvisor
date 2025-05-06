//go:build darwin

package memutil

// --- Darwin Porting Status ---
//
// PROBLEM:
//   Uses Linux-specific memfd_create(2) syscall via CreateMemFD function.
//
// LINUX_SPECIFIC:
//   - memfd_create(2) syscall.
//   - `memutil.CreateMemFD` function wrapping the syscall.
//
// DARWIN_EQUIVALENT:
//   - N/A for anonymous memory file descriptor.
//   - Alternatives: Anonymous mmap, POSIX shm_open, temporary files.
//
// REQUIRED_ACTION:
//   1. [STUB]: Create `memutil_darwin.go` with a stub for `CreateMemFD`
//      returning `unix.EOPNOTSUPP`.
//   2. [BUILD_TAG]: Add `//go:build linux` to the file defining `CreateMemFD`
//      for Linux (likely `memutil_linux.go` or `memutil.go`).
//
// IMPACT_IF_STUBBED:
//   Code relying on creating anonymous memory FDs (e.g., `pkg/sentry/usage`)
//   will fail at runtime.
//
// PRIORITY:
//   - IMPORTANT (If needed for memory accounting or other core features).
//   - BLOCKER (as it currently fails the build).
//
// --- End Darwin Porting Status ---
