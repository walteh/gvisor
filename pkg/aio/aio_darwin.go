//go:build darwin

package aio

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

func (q *GoQueue) workerMain() {
	defer q.workers.Done()
	for {
		select {
		case <-q.shutdown:
			return
		case r := <-q.requests:
			var (
				n   int
				err error
			)
			// Convert unsafe.Pointer and length to []byte slice.
			buf := unsafe.Slice((*byte)(r.Buf), r.Len)
			switch r.Op {
			case OpRead:
				n, err = unix.Pread(int(r.FD), buf, r.Off)
			case OpWrite:
				n, err = unix.Pwrite(int(r.FD), buf, r.Off)
			case OpReadv:
				// TODO(gvisor.dev/issue/10398): Implement Readv support. Currently Preadv is not implemented in x/sys/unix.
				err = unix.EOPNOTSUPP
			case OpWritev:
				// TODO(gvisor.dev/issue/10398): Implement Writev support. Currently Pwritev is not implemented in x/sys/unix.
				err = unix.EOPNOTSUPP
			default:
				panic(fmt.Sprintf("unknown op %v", r.Op))
			}

			c := Completion{
				ID:     r.ID,
				Result: int64(n),
			}
			if err != nil {
				errno, ok := err.(unix.Errno)
				if !ok {
					// Not an errno? Should not happen with unix package calls.
					// Return EIO to be safe.
					c.Result = -int64(unix.EIO)
				} else {
					c.Result = -int64(errno)
				}
			}
			q.completions <- c
		}
	}
}
