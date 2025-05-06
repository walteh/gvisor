//go:build darwin
// +build darwin

package fdchannel

import "golang.org/x/sys/unix"

// SOCK_CLOEXEC is defined as 0 on Darwin because passing it to socketpair(2)
// is ineffective. The close-on-exec behavior must be set manually via fcntl.
// Defining it as 0 allows common code in fdchannel_unsafe.go to compile
// while effectively ignoring the flag during the socketpair call.
const (
	SOCK_CLOEXEC = 0
)

// manualSetCloexec sets the FD_CLOEXEC flag on both file descriptors in the
// provided array using fcntl. This is necessary on Darwin because the
// SOCK_CLOEXEC flag provided to socketpair(2) is ineffective.
func manualSetCloexec(fds [2]int) error {
	for _, fd := range fds {
		// Set FD_CLOEXEC manually.
		_, err := unix.FcntlInt(uintptr(fd), unix.F_SETFD, unix.FD_CLOEXEC)
		if err != nil {
			// Note: We don't attempt to close the FDs here on error, as the caller
			// (NewConnectedSockets) is responsible for cleaning up both FDs if
			// manualSetCloexec fails.
			return err
		}
	}
	return nil
}
