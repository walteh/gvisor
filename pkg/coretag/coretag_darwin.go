//go:build darwin

package coretag

import "golang.org/x/sys/unix"

// Enable core tagging.
// Core tagging is a Linux-specific feature (prctl PR_SCHED_CORE) and is not
// supported on Darwin.
func Enable() error {
	return unix.EOPNOTSUPP
}

// GetAllCoreTags returns the core tag of all the threads in the thread group.
// Core tagging is a Linux-specific feature and is not supported on Darwin.
// PID 0 means the current pid.
func GetAllCoreTags(pid int) ([]uint64, error) {
	return nil, unix.EOPNOTSUPP
}
