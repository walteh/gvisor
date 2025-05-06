//go:build darwin

package transport

import (
	"golang.org/x/sys/unix"
)

// Darwin doesn't define this constant, stub it with an unused value
const SO_DOMAIN = 0x1029 // Arbitrary unused value

// MSGHDRToLinux converts a Darwin msghdr struct to Linux-compatible representation.
func MSGHDRToLinux(msg *unix.Msghdr) {
	// Darwin's Msghdr has Controllen as uint32 and Iovlen as int32
	// We don't need to do anything as we're directly manipulating the fields
}

// Writev is a stub for unix.Writev which doesn't exist on Darwin
func Writev(fd int, iovecs [][]byte) (int, error) {
	// Simulate writev by copying bytes to a buffer and doing a single write
	var totalLen int
	for _, vec := range iovecs {
		totalLen += len(vec)
	}

	if totalLen == 0 {
		return 0, nil
	}

	buf := make([]byte, totalLen)
	offset := 0
	for _, vec := range iovecs {
		copy(buf[offset:], vec)
		offset += len(vec)
	}

	return unix.Write(fd, buf)
}
