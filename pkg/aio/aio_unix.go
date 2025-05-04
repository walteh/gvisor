//go:build !darwin

package aio

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func (q *GoQueue) workerMain() {
	defer q.workers.Done()
	for {
		select {
		case <-q.shutdown:
			return
		case r := <-q.requests:
			var sysno uintptr
			switch r.Op {
			case OpRead:
				sysno = unix.SYS_PREAD64
			case OpWrite:
				sysno = unix.SYS_PWRITE64
			case OpReadv:
				sysno = unix.SYS_PREADV2
			case OpWritev:
				sysno = unix.SYS_PWRITEV2
			default:
				panic(fmt.Sprintf("unknown op %v", r.Op))
			}
			n, _, e := unix.Syscall6(sysno, uintptr(r.FD), uintptr(r.Buf), uintptr(r.Len), uintptr(r.Off), 0 /* pos_h */, 0 /* flags/unused */)
			c := Completion{
				ID:     r.ID,
				Result: int64(n),
			}
			if e != 0 {
				c.Result = -int64(e)
			}
			q.completions <- c
		}
	}
}
