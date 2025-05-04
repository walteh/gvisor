//go:build !darwin

package unet

import (
	"io"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	SOCK_CLOEXEC = unix.SOCK_CLOEXEC
)

func manualSocketPairCloexec(_ [2]int) error { return nil }

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

		_, _, e := unix.Syscall6(unix.SYS_PPOLL, uintptr(unsafe.Pointer(&events[0])), 2, 0, 0, 0, 0)
		if e == unix.EINTR {
			continue
		}
		if e != 0 {
			return e
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
		msg.Controllen = uint64(len(r.ControlMessage))
	}

	if len(iovecs) != 0 {
		msg.Iov = &iovecs[0]
		msg.Iovlen = uint64(len(iovecs))
	}

	// n is the bytes received.
	var n uintptr

	fd, ok := r.socket.enterFD()
	if !ok {
		return 0, unix.EBADF
	}
	// Leave on returns below.
	for {
		var e unix.Errno

		// Try a non-blocking recv first, so we don't give up the go runtime M.
		n, _, e = unix.RawSyscall(unix.SYS_RECVMSG, uintptr(fd), uintptr(unsafe.Pointer(&msg)), unix.MSG_DONTWAIT|unix.MSG_TRUNC)
		if e == 0 {
			break
		}
		if e == unix.EINTR {
			continue
		}
		if !r.blocking {
			r.socket.gate.Leave()
			return 0, e
		}
		if e != unix.EAGAIN && e != unix.EWOULDBLOCK {
			r.socket.gate.Leave()
			return 0, e
		}

		// Wait for the socket to become readable.
		err := r.socket.wait(false)
		if err == errClosing {
			err = unix.EBADF
		}
		if err != nil {
			r.socket.gate.Leave()
			return 0, err
		}
	}

	r.socket.gate.Leave()

	if msg.Controllen < uint64(len(r.ControlMessage)) {
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

	if int(n) > length {
		return length, errMessageTruncated
	}

	return int(n), nil
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
		msg.Controllen = uint64(len(w.ControlMessage))
	}

	if len(iovecs) > 0 {
		msg.Iov = &iovecs[0]
		msg.Iovlen = uint64(len(iovecs))
	}

	fd, ok := w.socket.enterFD()
	if !ok {
		return 0, unix.EBADF
	}
	// Leave on returns below.
	for {
		// Try a non-blocking send first, so we don't give up the go runtime M.
		n, _, e := unix.RawSyscall(unix.SYS_SENDMSG, uintptr(fd), uintptr(unsafe.Pointer(&msg)), unix.MSG_DONTWAIT|unix.MSG_NOSIGNAL)
		if e == 0 {
			w.socket.gate.Leave()
			return int(n), nil
		}
		if e == unix.EINTR {
			continue
		}
		if !w.blocking {
			w.socket.gate.Leave()
			return 0, e
		}
		if e != unix.EAGAIN && e != unix.EWOULDBLOCK {
			w.socket.gate.Leave()
			return 0, e
		}

		// Wait for the socket to become writeable.
		err := w.socket.wait(true)
		if err == errClosing {
			err = unix.EBADF
		}
		if err != nil {
			w.socket.gate.Leave()
			return 0, err
		}
	}
	// Unreachable, no s.gate.Leave needed.
}
