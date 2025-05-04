//go:build !darwin

package cgroup

import "golang.org/x/sys/unix"

const CGROUP2_SUPER_MAGIC = unix.CGROUP2_SUPER_MAGIC
