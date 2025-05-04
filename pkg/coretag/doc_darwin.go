//go:build darwin

package coretag

// Darwin Plan:
// This package currently fails to build on Darwin because it uses the
// Linux-specific prctl(2) syscall and related constants:
// - unix.SYS_PRCTL
// - unix.PR_SCHED_CORE
// - unix.PR_SCHED_CORE_CREATE
// - unix.PR_SCHED_CORE_GET
//
// This functionality is for Linux Core Scheduling, which helps mitigate
// certain CPU side-channel attacks.
//
// Required Changes:
// 1. Darwin does not have prctl or a direct equivalent for Core Scheduling.
// 2. Provide stub implementations for the relevant functions (Set, Get, etc.)
//    on Darwin that likely do nothing or return an error (e.g., syserr.ErrNotSupported).
// 3. The coretag functionality will effectively be disabled on Darwin.
