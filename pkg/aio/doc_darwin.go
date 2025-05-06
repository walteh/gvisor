//go:build darwin

package aio

// --- Darwin Porting Status ---
//
// PROBLEM:
//   Direct use of Linux-specific syscall numbers for async I/O operations.
//
// LINUX_SPECIFIC:
//   - unix.SYS_PREAD64
//   - unix.SYS_PWRITE64
//   - unix.SYS_PREADV2
//   - unix.SYS_PWRITEV2
//
// DARWIN_EQUIVALENT:
//   - Standard POSIX pread/pwrite/preadv/pwritev (via C library or direct syscalls
//     with different numbers).
//   - Potentially AIO via kqueue (though API is different from Linux AIO).
//
// REQUIRED_ACTION:
//   1. [REFACTOR]: Modify aio.go to use `unix.Pread`, `unix.Pwrite`, `unix.Preadv`,
//      `unix.Pwritev` from x/sys/unix if semantics match.
//   2. [ALTERNATIVE]: If direct syscalls are needed, add `//go:build linux` to
//      aio.go and create `aio_darwin.go` using Darwin syscall numbers or
//      stubbing with errors.
//
// IMPACT_IF_STUBBED:
//   Asynchronous I/O operations via this package would fail at runtime.
//
// PRIORITY:
//   - IMPORTANT (if sentry relies heavily on Linux AIO syscalls via this pkg).
//   - BLOCKER (as it currently fails the build).
//
// --- End Darwin Porting Status ---
