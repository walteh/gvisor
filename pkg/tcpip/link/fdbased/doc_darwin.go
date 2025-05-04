//go:build darwin

package fdbased

// Darwin Plan:
// This package currently fails to build on Darwin due to multiple undefined
// types and functions in mmap_nonlinux.go and save_restore.go:
// - endpoint, Options, linkDispatcher (from mmap_nonlinux.go)
// - recvMMsgDispatcher, rawfile.MMsgHdr, rawfile.MaxMsgsPerRecv (from save_restore.go)
//
// This suggests that the non-Linux implementations rely on helpers or types
// that are either specific to Linux or haven't been implemented/stubbed for Darwin yet.
// Specifically, MMsgHdr and MaxMsgsPerRecv indicate dependencies on the
// recvmmsg syscall, which is Linux-specific.
//
// Required Changes:
// 1. Provide Darwin implementations or stubs for the missing types/functions
//    (endpoint, Options, linkDispatcher, recvMMsgDispatcher).
// 2. Implement rawfile.MMsgHdr and rawfile.MaxMsgsPerRecv for Darwin if needed,
//    or refactor the Darwin logic to avoid recvmmsg (perhaps using repeated
//    recvmsg calls).
// 3. Ensure the mmap_nonlinux.go file correctly implements memory mapping
//    using Darwin system calls if needed.
