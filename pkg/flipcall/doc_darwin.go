//go:build darwin

package flipcall

// --- Darwin Porting Status ---
//
// PROBLEM:
//   Control logic implementation (`ctrl_futex.go`) relies exclusively on
//   Linux futex(2) operations, causing undefined methods on Endpoint.
//
// LINUX_SPECIFIC:
//   - futex(2) syscall (used for efficient cross-process/thread wakeups).
//   - `ctrl_futex.go` file.
//
// DARWIN_EQUIVALENT:
//   - POSIX semaphores (via syscalls or cgo).
//   - Mach semaphores/ports (lower-level macOS IPC).
//   - Other primitives (pipes, kqueue, pthread mutex/cond via cgo).
//
// REQUIRED_ACTION:
//   1. [IMPLEMENT]: Create `ctrl_darwin.go` implementing the `controller`
//      interface using a suitable Darwin synchronization mechanism.
//   2. [BUILD_TAG]: Add `//go:build linux` to `ctrl_futex.go`.
//   3. [MODIFY?]: The `Endpoint` struct might need Darwin-specific fields,
//      added conditionally using build tags.
//
// IMPACT_IF_STUBBED:
//   Flipcall mechanism will not function, breaking anything relying on it
//   (potentially RPC, device communication).
//
// PRIORITY:
//   - CRITICAL (If flipcall is fundamental to VF platform communication).
//   - BLOCKER (as it currently fails the build).
//
// --- End Darwin Porting Status ---
