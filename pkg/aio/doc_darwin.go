//go:build darwin

package aio

// Darwin Plan:
// This package currently fails to build on Darwin because it uses
// Linux-specific syscall constants directly:
// - unix.SYS_PREAD64
// - unix.SYS_PWRITE64
// - unix.SYS_PREADV2
// - unix.SYS_PWRITEV2
//
// Required Changes:
// 1. Use the standard Go syscall package or x/sys/unix functions like
//    unix.Pread, unix.Pwrite, unix.Preadv, unix.Pwritev if possible,
//    which should handle underlying OS differences.
// 2. If direct syscalls are truly necessary, conditionalize the syscall
//    numbers using build tags (e.g., in separate _linux.go and _darwin.go
//    files or within the function using runtime.GOOS checks) and find the
//    correct Darwin syscall numbers.
