//go:build darwin

package seccomp

// --- Darwin Porting Status ---
//
// PROBLEM:
//   1. Uses Linux-specific seccomp syscalls/concepts (prctl, BPF filters).
//   2. Type definitions required by stub function signatures are in Linux-only files,
//      causing build failures when those files are tagged `//go:build linux`.
//
// LINUX_SPECIFIC:
//   - seccomp(2) syscall (via SYS_SECCOMP or prctl(PR_SET_SECCOMP)).
//   - BPF program loading for syscall filtering.
//   - unix.SYS_PRCTL, unix.ENOTUNIQ.
//   - Types defined in seccomp.go, seccomp_rules.go, etc.
//
// DARWIN_EQUIVALENT:
//   - macOS Sandbox framework (sandboxd, sandbox-exec, libsandbox, SBPL profiles).
//   - N/A for direct seccomp equivalent.
//
// REQUIRED_ACTION:
//   1. [MOVE_TYPE]: Relocate essential public types (`ProgramOptions`, `RuleSet`,
//      `BuildStats`, `SyscallRules`, `SyscallRule` interface) from Linux-specific
//      files (`seccomp.go`, `seccomp_rules.go`) to a common `seccomp_defs.go`.
//      Care must be taken not to move internal implementation details.
//   2. [STUB]: Create `seccomp_darwin.go` with stub implementations for all
//      exported functions (`Install`, `DefaultAction`, `BuildProgram`, etc.)
//      returning `nil` or appropriate defaults (e.g., `SECCOMP_RET_ALLOW`).
//      These stubs will use the types from `seccomp_defs.go`.
//   3. [BUILD_TAG]: Add `//go:build linux` to all original seccomp files
//      (`seccomp.go`, `seccomp_unsafe.go`, `seccomp_rules.go`, etc.) AFTER
//      types are moved.
//
// IMPACT_IF_STUBBED:
//   Seccomp syscall filtering (a key security hardening feature) is disabled.
//   Alternative sandboxing via macOS Sandbox framework would be needed for parity.
//
// PRIORITY:
//   - SECURITY (Requires alternative implementation for hardening).
//   - BLOCKER (Due to type definition issues causing build failures).
//
// --- End Darwin Porting Status ---
