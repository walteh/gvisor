//go:build !darwin
// +build !darwin

package fd

import "golang.org/x/sys/unix"

// O_LARGEFILE is set to the system's value (unix.O_LARGEFILE) on non-Darwin
// POSIX systems (primarily Linux). This ensures that code calling open(2)
// via this package uses the correct flag if required by the platform (e.g.,
// 32-bit Linux) while being a no-op if the platform ignores it (e.g., 64-bit Linux).
const O_LARGEFILE = unix.O_LARGEFILE
