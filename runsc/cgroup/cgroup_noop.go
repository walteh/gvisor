package cgroup

import (
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
	// "gvisor.dev/gvisor/pkg/syserr" // Use unix.EOPNOTSUPP directly
)

// noopCgroup implements the NoOp Cgroup interface provided for Darwin.
// As Darwin does not support cgroups, most methods are no-ops or return errors.
type noopCgroup struct{}

// Install is a no-op on Darwin.
func (c *noopCgroup) Install(res *specs.LinuxResources) error {
	return nil
}

// Uninstall is a no-op on Darwin.
func (c *noopCgroup) Uninstall() error {
	return nil
}

// Join is a no-op on Darwin.
func (c *noopCgroup) Join() (func(), error) {
	return func() {}, nil
}

// CPUQuota returns an error on Darwin as cgroups are not supported.
func (c *noopCgroup) CPUQuota() (float64, error) {
	return 0, unix.EOPNOTSUPP
}

// CPUUsage returns an error on Darwin as cgroups are not supported.
func (c *noopCgroup) CPUUsage() (uint64, error) {
	return 0, unix.EOPNOTSUPP
}

// NumCPU returns an error on Darwin as cgroups are not supported.
func (c *noopCgroup) NumCPU() (int, error) {
	return 0, unix.EOPNOTSUPP
}

// MemoryLimit returns an error on Darwin as cgroups are not supported.
func (c *noopCgroup) MemoryLimit() (uint64, error) {
	return 0, unix.EOPNOTSUPP
}

// MakePath returns an empty string on Darwin.
func (c *noopCgroup) MakePath(controllerName string) string {
	return ""
}
