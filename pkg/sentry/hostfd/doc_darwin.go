//go:build darwin

package hostfd

// --- Darwin Porting Status ---
//
// PROBLEM:
//   Uses Linux-specific syscall numbers (pread64, etc.) and assumes Linux
//   limits (MaxReadWriteIov).
//
// LINUX_SPECIFIC:
//   - unix.SYS_PREAD64
//   - unix.SYS_PREADV2
//   - unix.SYS_PWRITE64
//   - unix.SYS_PWRITEV2
//   - MaxReadWriteIov (Linux constant for iovec array limits)
//
// DARWIN_EQUIVALENT:
//   - Standard POSIX pread/pwrite/preadv/pwritev (via C library or direct syscalls
//     with different numbers).
//   - unix.IOV_MAX (standard constant for iovec array limits).
//
// REQUIRED_ACTION:
//   1. [REFACTOR]: Modify hostfd.go/hostfd_unsafe.go to use `unix.Pread`, `unix.Pwrite`,
//      `unix.Preadv`, `unix.Pwritev` if possible.
//   2. [ALTERNATIVE_SYSCALL]: If direct syscalls are needed, use build tags and provide
//      Darwin syscall numbers in `hostfd_darwin.go`.
//   3. [DEFINE_CONST]: Define `MaxReadWriteIov = unix.IOV_MAX` in `hostfd_darwin.go`
//      (or potentially a common `hostfd_defs.go` if needed by Linux too).
//      Add `//go:build linux` to the file defining the Linux value if different.
//
// IMPACT_IF_STUBBED:
//   Direct host file descriptor operations (pread/pwrite) would fail.
//
// PRIORITY:
//   - CRITICAL (Likely needed for basic file system operations via gofers).
//   - BLOCKER (as it currently fails the build).
//
// --- End Darwin Porting Status ---
