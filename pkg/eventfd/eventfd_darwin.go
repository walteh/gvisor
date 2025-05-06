//go:build darwin

package eventfd

import (
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

// We use pipes to simulate eventfd on Darwin
var (
	pipeWriteFds      = make(map[int]int)
	pipeWriteFdsMutex sync.Mutex
)

// Create returns an initialized eventfd using pipes for Darwin.
func Create() (Eventfd, error) {
	// Create a pipe to simulate eventfd
	r := make([]int, 2)
	if err := unix.Pipe(r); err != nil {
		return Eventfd{}, fmt.Errorf("failed to create pipe for eventfd simulation: %v", err)
	}

	// Set both ends to non-blocking
	if err := unix.SetNonblock(r[0], true); err != nil {
		unix.Close(r[0])
		unix.Close(r[1])
		return Eventfd{}, err
	}

	if err := unix.SetNonblock(r[1], true); err != nil {
		unix.Close(r[0])
		unix.Close(r[1])
		return Eventfd{}, err
	}

	// Store the write end of the pipe for later use
	pipeWriteFdsMutex.Lock()
	pipeWriteFds[r[0]] = r[1]
	pipeWriteFdsMutex.Unlock()

	return Eventfd{r[0]}, nil
}

// Close overrides the standard Close to handle the pipe-based implementation.
func (ev Eventfd) Close() error {
	pipeWriteFdsMutex.Lock()
	writeFd, ok := pipeWriteFds[ev.fd]
	if ok {
		delete(pipeWriteFds, ev.fd)
	}
	pipeWriteFdsMutex.Unlock()

	err1 := unix.Close(ev.fd)

	if ok {
		err2 := unix.Close(writeFd)
		if err1 != nil {
			return err1
		}
		return err2
	}

	return err1
}

// nonBlockingWrite writes to the write end of the pipe for Darwin.
func nonBlockingWrite(fd int, buf []byte) (int, error) {
	pipeWriteFdsMutex.Lock()
	writeFd, ok := pipeWriteFds[fd]
	pipeWriteFdsMutex.Unlock()

	if !ok {
		// Try the fd directly (it might be the write end already)
		writeFd = fd
	}

	n, err := unix.Write(writeFd, buf)
	if err != nil {
		return 0, err
	}
	return n, nil
}
