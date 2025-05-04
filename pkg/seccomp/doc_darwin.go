//go:build darwin

package seccomp

// Darwin Plan:
// This package currently fails to build on Darwin due to:
// 1. Missing Linux-specific syscall: unix.SYS_PRCTL (used for PR_SET_SECCOMP).
// 2. Missing Linux-specific error constant: unix.ENOTUNIQ.
//
// Seccomp is a Linux-specific kernel feature for filtering system calls.
//
// Required Changes:
// 1. Darwin does not support seccomp. The equivalent sandboxing mechanism
//    is the Sandbox framework (sandboxd, sandbox-exec, libsandbox) which uses
//    a different profile language (SBPL - Sandbox Profile Language).
// 2. Provide stub implementations for seccomp-related functions (e.g.,
//    Install, MustInstall) on Darwin. These stubs should likely do nothing
//    and return success or indicate that seccomp is not supported.
// 3. Calls involving SYS_PRCTL for seccomp need to be stubbed out.
// 4. Replace or guard uses of unix.ENOTUNIQ.
// 5. Note: Full sandboxing on Darwin would require integrating with the
//    macOS Sandbox framework, which is a separate, significant effort.
//    The initial goal here is just to make the seccomp package *build*.
