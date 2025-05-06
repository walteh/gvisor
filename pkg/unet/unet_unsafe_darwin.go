//go:build darwin

package unet

import (
	"io"

	"golang.org/x/sys/unix"
)

const (
	SOCK_CLOEXEC = 0
)

func manualSocketPairCloexec(fds [2]int) error {
	for _, fd := range fds {
		_, err := unix.FcntlInt(uintptr(fd), unix.F_SETFD, unix.FD_CLOEXEC)
		if err != nil {
			return err
		}
	}
	return nil
}

// wait blocks until the socket FD is ready for reading or writing, depending
// on the value of write.
//
// Returns errClosing if the Socket is in the process of closing.
func (s *Socket) wait(write bool) error {
	for {
		// Checking the FD on each loop is not strictly necessary, it
		// just avoids an extra poll call.
		fd := s.fd.Load()
		if fd < 0 {
			return errClosing
		}

		events := []unix.PollFd{
			{
				// The actual socket FD.
				Fd:     fd,
				Events: unix.POLLIN,
			},
			{
				// The eventfd, signaled when we are closing.
				Fd:     int32(s.efd.FD()),
				Events: unix.POLLIN,
			},
		}
		if write {
			events[0].Events = unix.POLLOUT
		}

		// Darwin doesn't have ppoll, use poll instead
		n, err := unix.Poll(events, -1)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			// Timeout, but we specified infinite timeout (-1), so this shouldn't happen
			continue
		}

		if events[1].Revents&unix.POLLIN == unix.POLLIN {
			// eventfd signaled, we're closing.
			return errClosing
		}

		return nil
	}
}

// ReadVec reads into the pre-allocated bufs. Returns bytes read.
//
// The pre-allocatted space used by ReadVec is based upon slice lengths.
//
// This function is not guaranteed to read all available data, it
// returns as soon as a single recvmsg call succeeds.
func (r *SocketReader) ReadVec(bufs [][]byte) (int, error) {
	iovecs, length := buildIovec(bufs, make([]unix.Iovec, 0, 2))

	var msg unix.Msghdr
	if len(r.source) != 0 {
		msg.Name = &r.source[0]
		msg.Namelen = uint32(len(r.source))
	}

	if len(r.ControlMessage) != 0 {
		msg.Control = &r.ControlMessage[0]
		msg.Controllen = uint32(len(r.ControlMessage)) // Use uint32 for Darwin
	}

	if len(iovecs) != 0 {
		msg.Iov = &iovecs[0]
		msg.Iovlen = int32(len(iovecs)) // Use int32 for Darwin
	}

	// n is the bytes received.
	var n int

	fd, ok := r.socket.enterFD()
	if !ok {
		return 0, unix.EBADF
	}
	// Leave on returns below.
	for {
		var err error

		// Try a non-blocking recv first, so we don't give up the go runtime M.
		// Using RecvFrom since Darwin's Recvmsg has a different signature
		buffer := make([]byte, length)
		n, _, _, _, err = unix.Recvmsg(fd, buffer, r.ControlMessage, unix.MSG_DONTWAIT)
		if err == nil {
			// Copy data from buffer to bufs
			copied := 0
			for i := range bufs {
				if copied >= n {
					break
				}
				toCopy := min(n-copied, len(bufs[i]))
				copy(bufs[i], buffer[copied:copied+toCopy])
				copied += toCopy
			}
			break
		}
		if err == unix.EINTR {
			continue
		}
		if !r.blocking {
			r.socket.gate.Leave()
			return 0, err
		}
		if err != unix.EAGAIN && err != unix.EWOULDBLOCK {
			r.socket.gate.Leave()
			return 0, err
		}

		// Wait for the socket to become readable.
		err = r.socket.wait(false)
		if err == errClosing {
			err = unix.EBADF
		}
		if err != nil {
			r.socket.gate.Leave()
			return 0, err
		}
	}

	r.socket.gate.Leave()

	if uint32(len(r.ControlMessage)) > msg.Controllen {
		r.ControlMessage = r.ControlMessage[:msg.Controllen]
	}

	if msg.Namelen < uint32(len(r.source)) {
		r.source = r.source[:msg.Namelen]
	}

	// All unet sockets are SOCK_STREAM or SOCK_SEQPACKET, both of which
	// indicate that the other end is closed by returning a 0 length read
	// with no error.
	if n == 0 {
		return 0, io.EOF
	}

	if r.race != nil {
		// See comments on Socket.race.
		r.race.Add(1)
	}

	if n > length {
		return length, errMessageTruncated
	}

	return n, nil
}

// WriteVec writes the bufs to the socket. Returns bytes written.
//
// This function is not guaranteed to send all data, it returns
// as soon as a single sendmsg call succeeds.
func (w *SocketWriter) WriteVec(bufs [][]byte) (int, error) {
	iovecs, _ := buildIovec(bufs, make([]unix.Iovec, 0, 2))

	if w.race != nil {
		// See comments on Socket.race.
		w.race.Add(1)
	}

	var msg unix.Msghdr
	if len(w.to) != 0 {
		msg.Name = &w.to[0]
		msg.Namelen = uint32(len(w.to))
	}

	if len(w.ControlMessage) != 0 {
		msg.Control = &w.ControlMessage[0]
		msg.Controllen = uint32(len(w.ControlMessage)) // Use uint32 for Darwin
	}

	if len(iovecs) > 0 {
		msg.Iov = &iovecs[0]
		msg.Iovlen = int32(len(iovecs)) // Use int32 for Darwin
	}

	fd, ok := w.socket.enterFD()
	if !ok {
		return 0, unix.EBADF
	}
	// Leave on returns below.
	for {
		var err error

		// Try a non-blocking send first, so we don't give up the go runtime M.
		// Darwin's Sendmsg has a different signature
		var buffer []byte
		for _, b := range bufs {
			buffer = append(buffer, b...)
		}
		err = unix.Sendmsg(fd, buffer, w.ControlMessage, nil, unix.MSG_DONTWAIT)
		if err == nil {
			w.socket.gate.Leave()
			return len(buffer), nil
		}
		if err == unix.EINTR {
			continue
		}
		if !w.blocking {
			w.socket.gate.Leave()
			return 0, err
		}
		if err != unix.EAGAIN && err != unix.EWOULDBLOCK {
			w.socket.gate.Leave()
			return 0, err
		}

		// Wait for the socket to become writeable.
		err = w.socket.wait(true)
		if err == errClosing {
			err = unix.EBADF
		}
		if err != nil {
			w.socket.gate.Leave()
			return 0, err
		}
	}
}

// min returns the smaller of x or y.
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
