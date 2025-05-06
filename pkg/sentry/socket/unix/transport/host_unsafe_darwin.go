//go:build darwin

package transport

import (
	"golang.org/x/sys/unix"
)

// Darwin doesn't define this constant, stub it with an unused value
const SO_DOMAIN = 0x1029 // Arbitrary unused value

// fdReadVec receives from fd to bufs.
//
// If the total length of bufs is > maxlen, fdReadVec will do a partial read
// and err will indicate why the message was truncated.
func fdReadVec(fd int, bufs [][]byte, control []byte, peek bool, maxlen int64) (readLen int64, msgLen int64, controlLen uint64, controlTrunc bool, err error) {
	flags := uintptr(unix.MSG_DONTWAIT | unix.MSG_TRUNC)
	if peek {
		flags |= unix.MSG_PEEK
	}

	// Always truncate the receive buffer. All socket types will truncate
	// received messages.
	length, iovecs, intermediate, err := buildIovec(bufs, maxlen, true)
	if err != nil && len(iovecs) == 0 {
		// No partial write to do, return error immediately.
		return 0, 0, 0, false, err
	}

	// On Darwin, unix.Msghdr uses uint32 for Controllen and int32 for Iovlen
	var msg unix.Msghdr
	if len(control) != 0 {
		msg.Control = &control[0]
		// Use uint32 for Darwin
		msg.Controllen = uint32(len(control))
	}

	if len(iovecs) != 0 {
		msg.Iov = &iovecs[0]
		// Use int32 for Darwin
		msg.Iovlen = int32(len(iovecs))
	}

	// Use unified approach for both OS
	buffer := make([]byte, length)

	// Use Recvmsg directly for Darwin
	n, oob, msgFlags, _, err := unix.Recvmsg(fd, buffer, control, int(flags))
	if err != nil {
		return 0, 0, 0, false, err
	}

	// Copy from buffer to bufs if needed
	if intermediate == nil && n > 0 {
		var copied int
		for i, buf := range bufs {
			if copied >= n {
				break
			}
			toCopy := min(len(buf), n-copied)
			copy(bufs[i][:toCopy], buffer[copied:copied+toCopy])
			copied += toCopy
		}
	}

	controlTrunc = msgFlags&unix.MSG_CTRUNC == unix.MSG_CTRUNC
	controlLength := uint64(len(oob))

	if n > int(length) {
		return length, int64(n), controlLength, controlTrunc, nil
	}

	return int64(n), int64(n), controlLength, controlTrunc, nil
}

// fdWriteVec sends from bufs to fd.
//
// If the total length of bufs is > maxlen && truncate, fdWriteVec will do a
// partial write and err will indicate why the message was truncated.
func fdWriteVec(fd int, bufs [][]byte, maxlen int64, truncate bool) (int64, int64, error) {
	length, iovecs, intermediate, err := buildIovec(bufs, maxlen, truncate)
	if err != nil && len(iovecs) == 0 {
		// No partial write to do, return error immediately.
		return 0, length, err
	}

	// Copy data to intermediate buf if needed.
	if intermediate != nil {
		copyFromMulti(intermediate, bufs)
	}

	// On Darwin, use direct write approach instead of SYS_SENDMSG
	// Join all the buffers into one
	buffer := make([]byte, length)
	offset := 0
	for _, buf := range bufs {
		if len(buf) == 0 {
			continue
		}
		// Make sure we don't exceed the buffer length
		copyLen := min(len(buf), int(length)-offset)
		if copyLen <= 0 {
			break
		}
		copy(buffer[offset:offset+copyLen], buf[:copyLen])
		offset += copyLen
	}

	// Do a single write call
	n, err := unix.Write(fd, buffer[:offset])
	if err != nil {
		return 0, length, err
	}

	return int64(n), length, nil
}

// min returns the smaller of a or b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
