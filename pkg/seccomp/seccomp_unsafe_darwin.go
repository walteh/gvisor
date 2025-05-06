//go:build darwin

package seccomp

import (
	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/abi/linux"
	"gvisor.dev/gvisor/pkg/bpf"
)

// Define needed constants for stubs.
// Since seccomp is disabled, RET_ALLOW is the most practical default.
const (
	SECCOMP_RET_ALLOW = linux.SECCOMP_RET_ALLOW
)

// SetFilter is a no-op on Darwin.
func SetFilter(prog []bpf.Instruction) error {
	return nil
}

// SetFilterInChild is a no-op on Darwin.
func SetFilterInChild(prog []bpf.Instruction) unix.Errno {
	return 0
}

// MustInstall installs seccomp filters and panics on error.
// This is a no-op on Darwin.
func MustInstall(rules SyscallRules, denyRules SyscallRules, options ProgramOptions) {
	_ = Install(rules, denyRules, options) // Call Install to ensure consistency, ignore result.
}

func isKillProcessAvailable() (bool, error) {
	return false, nil
}
