//go:build darwin

package flipcall

// Darwin Plan:
// This package currently fails to build on Darwin because the ctrl_futex.go
// implementation relies on Linux futex operations, leading to undefined
// methods on the Endpoint type (futexSetPeerActive, futexWakePeer, futexWaitUntilActive).
//
// Required Changes:
// 1. Implement an alternative control mechanism for Darwin instead of futexes.
//    Possible options include:
//    a) POSIX semaphores (semaphore_create, semaphore_wait, semaphore_signal, semaphore_destroy
//       accessed via cgo or direct syscalls if available in x/sys/unix).
//    b) Mach semaphores (if targeting lower-level macOS interaction).
//    c) Other synchronization primitives like condition variables + mutexes.
// 2. Create a ctrl_darwin.go file (or similar) containing the Darwin-specific
//    implementation of the control logic (likely implementing the `controller` interface).
// 3. The Endpoint struct might need fields added (guarded by build tags)
//    to hold Darwin-specific synchronization state.
