//go:build darwin

package cgroup

// CGROUP2_SUPER_MAGIC is a Linux cgroup v2 specific magic number.
// Defined as 0 on Darwin for compatibility.
const CGROUP2_SUPER_MAGIC = 0
