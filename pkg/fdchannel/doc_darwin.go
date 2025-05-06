//go:build darwin

package fdchannel

// Darwin Plan:
// This package currently fails to build on Darwin because it uses the
// Linux-specific socket flag unix.SOCK_CLOEXEC.
//
// Required Changes:
// 1. Replace the use of unix.SOCK_CLOEXEC.
// 2. On Darwin, the equivalent behavior is achieved by creating a standard
//    socket and then using fcntl with FD_CLOEXEC to set the close-on-exec flag.
//    The relevant Go functions are likely unix.Socket() followed by unix.FcntlInt(..., unix.F_SETFD, unix.FD_CLOEXEC).
