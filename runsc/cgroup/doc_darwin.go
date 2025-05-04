//go:build darwin

package cgroup

// Darwin Plan:
// This package currently fails to build on Darwin because it uses the
// Linux-specific constant unix.CGROUP2_SUPER_MAGIC.
//
// Cgroups (Control Groups) are a Linux kernel feature for resource limiting
// and management.
//
// Required Changes:
// 1. Darwin does not use cgroups. Resource limiting is typically handled
//    via setrlimit, POSIX APIs, or potentially macOS-specific frameworks.
// 2. Provide stub implementations for cgroup-related functions on Darwin.
//    These stubs should likely do nothing and return success, effectively
//    disabling cgroup functionality on Darwin.
// 3. Guard uses of Linux-specific constants like CGROUP2_SUPER_MAGIC
//    using build tags or runtime checks.
// 4. This means that resource limits configured via cgroups in the OCI spec
//    will likely be ignored when running on the VF platform initially.
//    Mapping OCI resource limits to Darwin mechanisms is a future task.
