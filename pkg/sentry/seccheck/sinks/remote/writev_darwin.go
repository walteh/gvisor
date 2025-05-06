//go:build darwin

package remote

import (
	"golang.org/x/sys/unix"
)

// Writev implements a Darwin-specific version of the unix.Writev function,
// which is not available on Darwin.
func Writev(fd int, iovecs [][]byte) (int, error) {
	// Calculate total length of all vectors
	var totalLen int
	for _, iov := range iovecs {
		totalLen += len(iov)
	}

	if totalLen == 0 {
		return 0, nil
	}

	// Create a buffer large enough to hold all the vectors
	buf := make([]byte, totalLen)
	offset := 0

	// Copy all the vectors into the buffer
	for _, iov := range iovecs {
		copy(buf[offset:], iov)
		offset += len(iov)
	}

	// Perform a single write
	return unix.Write(fd, buf)
}
