//go:build darwin

package memutil

import (
	"golang.org/x/sys/unix"
	// "gvisor.dev/gvisor/pkg/syserr" // No longer needed
)

// we need to keep comment tags for things we may want to or be able to implement in the user space or thorugh some other mechanism
// to differentiate the stuff that does not matter and stuff that will cause issues

// CreateMemFD creates an anonymous file descriptor analogous to memfd_create(2).
// Darwin does not support memfd_create(2).
func CreateMemFD(name string, flags uint) (fd int, err error) {
	// Return EOPNOTSUPP as there is no direct equivalent on Darwin.
	return -1, unix.EOPNOTSUPP
}
