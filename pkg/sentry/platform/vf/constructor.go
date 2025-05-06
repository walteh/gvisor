package vf

import (
	"errors"

	"gvisor.dev/gvisor/pkg/fd"
	"gvisor.dev/gvisor/pkg/sentry/platform"
)

type vfConstructor struct{}

var _ platform.Constructor = &vfConstructor{}

func init() {
	// Phase 1.2: Register the platform constructor.
	platform.Register("vf", &vfConstructor{})
}

// New implements platform.Constructor.New.
func (c *vfConstructor) New(_ *fd.FD) (platform.Platform, error) {
	// For now, we ignore the device FD.
	return &vf{}, nil
}

// OpenDevice implements platform.Constructor.OpenDevice.
// TODO: Determine if VF needs a device path/FD. Return error for now.
func (c *vfConstructor) OpenDevice(devicePath string) (*fd.FD, error) {
	// Virtualization.framework doesn't use a character device like KVM.
	return nil, errors.New("vf platform does not use a device path")
}

// Requirements implements platform.Constructor.Requirements.
// TODO: Determine the actual requirements for VF.
func (c *vfConstructor) Requirements() platform.Requirements {
	// Return empty requirements for now.
	return platform.Requirements{}
}

// PrecompiledSeccompInfo implements platform.Constructor.PrecompiledSeccompInfo.
// TODO: Determine if VF needs precompiled seccomp rules.
func (c *vfConstructor) PrecompiledSeccompInfo() []platform.SeccompInfo {
	// Return empty info for now.
	return nil
}
