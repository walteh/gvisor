//go:build darwin

package fsutil

// Darwin Plan:
// This package currently fails to build on Darwin due to:
// 1. Missing Linux-specific syscall: unix.Getdents.
//    Darwin uses getdents64 or getdirentries64.
// 2. Missing Linux-specific syscall: unix.SYS_UTIMENSAT.
//    Darwin uses utimensat (check x/sys/unix for availability).
// 3. Accessing unix.Dirent.Off field, which may not exist or be correct
//    for Darwin's directory entry structure.
//
// Required Changes:
// 1. Replace unix.Getdents with the appropriate Darwin equivalent,
//    likely unix.Getdirentries or unix.Getdents64 from x/sys/unix,
//    adjusting for any differences in the Dirent structure and handling.
// 2. Replace direct use of unix.SYS_UTIMENSAT with unix.UtimesNanoAt or
//    similar cross-platform functions if possible. If direct syscall is
//    needed, use the correct Darwin syscall number for utimensat.
// 3. Verify the structure of unix.Dirent when built for Darwin and access
//    fields accordingly, potentially avoiding the Offset field if problematic.
// 4. Use build tags (_linux.go/_darwin.go or runtime.GOOS checks) to
//    conditionalize the implementations.
