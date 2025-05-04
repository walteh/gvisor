//go:build darwin

package eventfd

// Darwin Plan:
// This package currently fails to build on Darwin due to:
// 1. Missing Linux-specific syscall: unix.SYS_EVENTFD2.
//    Darwin uses kqueue for event notification, not eventfd.
// 2. Undefined function call: rawfile.BlockingRead.
//    This likely needs a Darwin implementation in pkg/rawfile.
//
// Required Changes:
// 1. Implement the event notification functionality using Darwin's kqueue mechanism.
//    This will likely require a significant rewrite of the package logic for Darwin
//    and potentially defining a Darwin-specific implementation of the interfaces.
// 2. Ensure pkg/rawfile has the necessary functions implemented for Darwin,
//    including BlockingRead or its equivalent.
