//go:build darwin

package seccomp

// --- Darwin Porting Status ---
//
// PROBLEM:
//   1. Seccomp is a Linux-specific sandboxing mechanism using BPF filters
//      and prctl/seccomp syscalls, none of which exist on Darwin.
//   2. The package structure makes clean stubbing difficult because essential
//      type definitions (`ProgramOptions`, `RuleSet`, `SyscallRules`, etc.)
//      reside in Linux-specific implementation files (`seccomp.go`, `seccomp_rules.go`).
//
// LINUX_SPECIFIC:
//   - seccomp(2) syscall (via SYS_SECCOMP or prctl(PR_SET_SECCOMP)).
//   - BPF program loading for syscall filtering.
//   - unix.SYS_PRCTL, unix.ENOTUNIQ, linux.BPFAction, linux.SECCOMP_RET_*, etc.
//   - Types: ProgramOptions, RuleSet, BuildStats, SyscallRules, SyscallRule
//     interface, internal types (syscallProgram, labelSet etc.) defined in
//     seccomp.go, seccomp_rules.go.
//   - Files: seccomp.go, seccomp_unsafe.go, seccomp_rules.go contain Linux logic.
//
// DARWIN_EQUIVALENT:
//   - macOS Sandbox framework (sandboxd, sandbox-exec, libsandbox, SBPL profiles)
//     for sandboxing (requires significant separate implementation).
//   - N/A for direct seccomp equivalent.
//
// CURRENT_APPROACH:
//   - No Darwin Stubs: `seccomp_darwin.go` was created and then deleted.
//   - No Build Tags: Linux implementation files (seccomp.go, etc.) are still
//     included in the Darwin build because they lack `//go:build linux` tags.
//   - Build Status: Currently compiles on Darwin because the Linux-specific code
//     happens to not cause *compile-time* errors (e.g., constants like
//     SYS_PRCTL are likely defined as 0 or unused in paths triggered during
//     compilation). However, runtime errors or incorrect behavior are highly
//     likely if seccomp functionality is invoked.
//
// REQUIRED_ACTION:
//   1. ***[CLEANUP - RECOMMENDED]***:
//      a. [MOVE_TYPE]: Relocate essential public types (`ProgramOptions`, `RuleSet`,
//         `BuildStats`, `SyscallRules`, `SyscallRule` interface, `linux.BPFAction`
//         related constants if needed) to a common file, e.g., `seccomp_defs.go`.
//      b. [CREATE_STUB]: Re-create `seccomp_darwin.go` with stub functions
//         (returning nil/defaults/errors like ENOSYS or EOPNOTSUPP)
//         using types from the common definitions file.
//      c. [BUILD_TAG]: Add `//go:build linux` to `seccomp.go`, `seccomp_unsafe.go`,
//         `seccomp_rules.go`.
//      This provides a clean, robust separation ensuring no Linux code leaks into
//      the Darwin build and stubs function correctly.
//   2. [FUTURE - Sandboxing]: Implement Darwin sandboxing using the macOS Sandbox
//      framework. This is a large, separate feature effort.
//
// IMPACT_IF_STUBBED (Correctly via Recommended Action 1):
//   Seccomp syscall filtering is explicitly disabled. No security hardening from this package.
//
// IMPACT_IF_CURRENT_STATE:
//   Seccomp filtering *appears* enabled but will likely fail/panic at runtime.
//   Potential for unexpected behavior due to Linux code running. Unsafe.
//
// PRIORITY:
//   - HIGH (Security implications of incorrect runtime behavior).
//   - BLOCKER (The current state is fragile and needs the recommended cleanup).
//
// --- End Darwin Porting Status ---
