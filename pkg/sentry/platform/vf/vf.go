//go:build darwin && arm64
// +build darwin,arm64

package vf

import (
	"gvisor.dev/gvisor/pkg/context"
	"gvisor.dev/gvisor/pkg/hostarch"
	"gvisor.dev/gvisor/pkg/sentry/platform"
)

// vf implements platform.Platform using Apple's Virtualization.framework.
//
// IMPLEMENTATION PLAN:
//
// Phase 1: Make runsc Compile and Select the Platform
// 1. Satisfy platform.Platform Interface: Replace panics with 'unimplemented' errors/logs. (Partially done)
// 2. Register Platform: Add init() to call platform.Register(vfPlatformInstance). (TODO)
// 3. Add Config Flag: Add "vf" option to --platform flag in runsc/config. (TODO)
// 4. Stub Other Dependencies: Create minimal *_darwin.go files for pkgs like rawfile, fdnotifier, etc., to satisfy build. (TODO)
//
// Phase 2: Attempt a Minimal Run
// 5. Minimal Switch(): Implement a stub Switch() that logs and returns a fake "exit" condition (e.g., SYS_GETPID). (TODO in NewContext which returns Context)
// 6. Build runsc: Build with GOOS=darwin GOARCH=arm64. (TODO)
// 7. Prepare Trivial Container: OCI spec for /bin/true or /bin/echo. (TODO)
// 8. Attempt runsc run: Execute runsc --platform=vf run ... (TODO)
// 9. Analyze Logs: Debug failures, identify the next blocker. (TODO)
//
// Phase 3: Incremental Implementation & Testing
// 10. Targeted Implementation: Pick the first blocker (e.g., VCPU setup, real trap handling) and implement using Virtualization.framework. (TODO)
// 11. Refine Stubs: Make other *_darwin.go stubs slightly more functional as needed. (TODO)
// 12. Repeat: Iterate build, run, analyze, implement for basic syscalls (execve, stdio, getpid, write, exit_group). (TODO)

type vf struct{}

var _ platform.Platform = &vf{}

// CooperativelySchedulesAddressSpace implements platform.Platform.
// TODO: Determine the correct behavior for VF.
func (v *vf) CooperativelySchedulesAddressSpace() bool {
	panic("unimplemented: CooperativelySchedulesAddressSpace")
}

// DetectsCPUPreemption implements platform.Platform.
// TODO: Determine the correct behavior for VF. Likely false?
func (v *vf) DetectsCPUPreemption() bool {
	panic("unimplemented: DetectsCPUPreemption")
}

// GlobalMemoryBarrier implements platform.Platform.
// TODO: Implement using appropriate VF/macOS mechanisms if needed.
func (v *vf) GlobalMemoryBarrier() error {
	panic("unimplemented: GlobalMemoryBarrier")
}

// HaveGlobalMemoryBarrier implements platform.Platform.
// TODO: Determine if VF requires/provides a global memory barrier.
func (v *vf) HaveGlobalMemoryBarrier() bool {
	panic("unimplemented: HaveGlobalMemoryBarrier")
}

// MapUnit implements platform.Platform.
// TODO: Determine the correct map unit, likely hostarch.PageSize.
func (v *vf) MapUnit() uint64 {
	panic("unimplemented: MapUnit")
}

// MaxUserAddress implements platform.Platform.
// TODO: Determine the correct max user address for arm64 macOS.
func (v *vf) MaxUserAddress() hostarch.Addr {
	panic("unimplemented: MaxUserAddress")
}

// MinUserAddress implements platform.Platform.
// TODO: Determine the correct min user address for arm64 macOS.
func (v *vf) MinUserAddress() hostarch.Addr {
	panic("unimplemented: MinUserAddress")
}

// NewAddressSpace implements platform.Platform.
// TODO: Phase 3: Implement AddressSpace using Virtualization.framework memory management.
func (v *vf) NewAddressSpace(mappingsID any) (platform.AddressSpace, <-chan struct{}, error) {
	panic("unimplemented: NewAddressSpace")
}

// NewContext implements platform.Platform.
// TODO: Phase 2.5 / Phase 3: This is where the core VM/VCPU context and Switch() logic will live.
// Return a vfContext struct that implements platform.Context.
func (v *vf) NewContext(context.Context) platform.Context {
	panic("unimplemented: NewContext")
}

// PreemptAllCPUs implements platform.Platform.
// TODO: Implement using appropriate VF/macOS mechanisms if possible/needed.
func (v *vf) PreemptAllCPUs() error {
	panic("unimplemented: PreemptAllCPUs")
}

// SeccompInfo implements platform.Platform.
// TODO: Determine how seccomp/sandboxing maps to macOS. Return basic info for now.
func (v *vf) SeccompInfo() platform.SeccompInfo {
	panic("unimplemented: SeccompInfo")
}

// SupportsAddressSpaceIO implements platform.Platform.
// TODO: Determine if VF allows direct address space I/O. Likely false initially.
func (v *vf) SupportsAddressSpaceIO() bool {
	panic("unimplemented: SupportsAddressSpaceIO")
}
