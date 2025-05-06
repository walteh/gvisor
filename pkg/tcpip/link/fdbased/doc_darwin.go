//go:build darwin

package fdbased

// --- Darwin Porting Status ---
//
// PROBLEM:
//   Non-Linux code paths (`mmap_nonlinux.go`, `save_restore.go`) use undefined
//   internal types and depend on Linux-specific rawfile features (recvmmsg).
//
// LINUX_SPECIFIC:
//   - `recvmmsg(2)` syscall (used via rawfile for efficient multi-packet reads).
//   - Potentially internal types/helpers defined only in `fdbased_linux.go`.
//
// DARWIN_EQUIVALENT:
//   - `recvmsg(2)` (can be called repeatedly to simulate recvmmsg).
//   - Standard mmap via `unix.Mmap`.
//
// REQUIRED_ACTION:
//   1. [DEPENDENCY]: Implement `rawfile.MMsgHdr` and multi-message receive logic
//      (simulating recvmmsg) in `pkg/rawfile/rawfile_darwin.go`.
//   2. [STUB/IMPLEMENT]: Provide Darwin implementations or stubs for internal helpers
//      (`endpoint`, `Options`, `linkDispatcher`, `recvMMsgDispatcher`) possibly in
//      `fdbased_darwin.go` or modified non-Linux files.
//   3. [MOVE_TYPE?]: If types mentioned in (2) are defined in `fdbased_linux.go`,
//      they may need moving to a common `defs.go` file or nested package.
//   4. [VERIFY]: Ensure memory mapping logic in `mmap_nonlinux.go` uses correct
//      Darwin APIs/flags.
//
// IMPACT_IF_STUBBED:
//   Networking link endpoint will likely fail to initialize or operate correctly.
//   Performance might be lower due to lack of recvmmsg equivalent.
//
// PRIORITY:
//   - CRITICAL (Essential for networking functionality).
//   - BLOCKER (as it currently fails the build).
//
// --- End Darwin Porting Status ---
