//go:build linux

package fsutil

import "golang.org/x/sys/unix"

const MAP_FIXED_NOREPLACE = unix.MAP_FIXED_NOREPLACE
