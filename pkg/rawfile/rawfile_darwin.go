//go:build darwin

package rawfile

import (
	"golang.org/x/sys/unix"
)

// This file implements Darwin-specific stubs for rawfile functions.

// BlockingRead reads from a file descriptor. On Darwin, this is a simple wrapper
// that doesn't implement the full non-blocking behavior found in the Linux version.
func BlockingRead(fd int, b []byte) (int, unix.Errno) {
	n, err := unix.Read(fd, b)
	if err != nil {
		return 0, err.(unix.Errno)
	}
	return n, 0
}

// BlockingReadvUntilStopped is a stub for Darwin.
func BlockingReadvUntilStopped(efd int, fd int, iovecs []unix.Iovec) (int, unix.Errno) {
	// Darwin doesn't support readv in the same way as Linux
	return -1, unix.ENOSYS
}

// NonBlockingWrite writes the given buffer to a file descriptor.
func NonBlockingWrite(fd int, buf []byte) unix.Errno {
	_, err := unix.Write(fd, buf)
	if err != nil {
		return err.(unix.Errno)
	}
	return 0
}

// GetMTU determines the MTU of a network interface device.
func GetMTU(name string) (uint32, error) {
	// This is a simplified stub for Darwin
	return 1500, nil
}

// MaxMsgsPerRecv is the maximum number of messages to receive per syscall.
const MaxMsgsPerRecv = 32

// MaxReadWriteIov is the maximum number of iovec structures that can be
// passed to readv/writev/preadv/pwritev calls.
const MaxReadWriteIov = 1024

// MMsgHdr represents a stub for the mmsg_hdr structure.
type MMsgHdr struct {
	Msg unix.Msghdr
	Len uint32
	_   [4]byte
}
