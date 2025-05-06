//go:build aix || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build aix dragonfly freebsd linux netbsd openbsd solaris

package fdchannel

import "golang.org/x/sys/unix"

// SOCK_CLOEXEC is set to the system's value (unix.SOCK_CLOEXEC) on Linux
// and other POSIX systems where it's expected to work directly in the
// socket(2) or socketpair(2) call.
var (
	SOCK_CLOEXEC = unix.SOCK_CLOEXEC
)

// manualSetCloexec is a no-op on Linux and other POSIX systems where
// SOCK_CLOEXEC is assumed to work reliably when passed to socketpair(2).
// The actual implementation for platforms needing manual setting (like Darwin)
// is in socket_darwin.go.
func manualSetCloexec([2]int) error { return nil }
