//go:build darwin

package fsutil

// --- Darwin Porting Status ---
//
// PROBLEM:
//   Uses Linux-specific directory reading (Getdents) and time setting
//   (SYS_UTIMENSAT) syscalls, and potentially incompatible Dirent structure access.
//
// LINUX_SPECIFIC:
//   - unix.Getdents (syscall)
//   - unix.SYS_UTIMENSAT (syscall number)
//   - Access to unix.Dirent.Off (offset field might differ/not exist)
//
// DARWIN_EQUIVALENT:
//   - unix.Getdirentries / getdents64 (syscalls for reading directories)
//   - unix.UtimesNanoAt / utimensat syscall
//   - Darwin's specific unix.Dirent structure
//
// REQUIRED_ACTION:
//   1. [IMPLEMENT]: Create `fsutil_darwin.go`. Implement directory reading
//      using `unix.Getdirentries` and handle the Darwin `unix.Dirent` struct carefully.
//   2. [REFACTOR/SYSCALL]: Modify time setting logic. Prefer `unix.UtimesNanoAt`.
//      If direct syscall needed, use build tags and Darwin's `utimensat` number.
//   3. [BUILD_TAG]: Add `//go:build linux` to original files (`fsutil.go`,
//      `fsutil_unsafe.go`) if they become Linux-only after refactoring.
//
// IMPACT_IF_STUBBED:
//   Directory listing and setting file timestamps would fail.
//
// PRIORITY:
//   - CRITICAL (Essential for filesystem operations).
//   - BLOCKER (as it currently fails the build).
//
// --- End Darwin Porting Status ---
