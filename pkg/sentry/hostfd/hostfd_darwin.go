//go:build darwin

package hostfd

import (
	"io"
	"unsafe"

	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/safemem"
)

// MaxReadWriteIov is the maximum permitted size of a struct iovec array in a
// readv, writev, preadv, or pwritev host syscall.
const MaxReadWriteIov = 1024 // UIO_MAXIOV

// MaxSendRecvMsgIov is the maximum permitted size of a struct iovec array in a
// sendmsg or recvmsg host syscall.
const MaxSendRecvMsgIov = 1024 // UIO_MAXIOV

// min returns the smaller of a or b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Preadv2 reads up to dsts.NumBytes() bytes from host file descriptor fd into
// dsts. offset and flags are interpreted as for preadv2(2).
//
// Preconditions: !dsts.IsEmpty().
func Preadv2(fd int32, dsts safemem.BlockSeq, offset int64, flags uint32) (uint64, error) {
	// Darwin doesn't have preadv2, so we simulate it using pread or read
	// No buffering is necessary regardless of safecopy; host syscalls will
	// return EFAULT if appropriate, instead of raising SIGBUS.

	// If we only have a single block, use simple read or pread
	if dsts.NumBlocks() == 1 {
		dst := dsts.Head()
		var n int
		var err error

		if offset == -1 {
			// Use regular read
			n, err = unix.Read(int(fd), unsafe.Slice((*byte)(unsafe.Pointer(uintptr(dst.Addr()))), dst.Len()))
		} else {
			// Use pread
			n, err = unix.Pread(int(fd), unsafe.Slice((*byte)(unsafe.Pointer(uintptr(dst.Addr()))), dst.Len()), offset)
		}

		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, io.EOF
		}
		return uint64(n), nil
	}

	// For multiple blocks, we need to handle them one by one
	// This is less efficient than a native preadv2, but Darwin doesn't have that syscall
	var totalRead uint64

	// Create a temporary buffer to do the reads
	// We'll read in chunks of reasonable size
	const chunkSize = 32 * 1024
	buf := make([]byte, min(chunkSize, int(dsts.NumBytes())))

	// Continue reading until we've read all blocks or encountered an error
	for totalRead < dsts.NumBytes() {
		var n int
		var err error

		toRead := min(len(buf), int(dsts.NumBytes()-totalRead))

		if offset == -1 {
			// Use regular read
			n, err = unix.Read(int(fd), buf[:toRead])
		} else {
			// Use pread with adjusted offset
			n, err = unix.Pread(int(fd), buf[:toRead], offset+int64(totalRead))
		}

		if err != nil {
			if totalRead > 0 {
				// We already read some data, return what we have
				break
			}
			return 0, err
		}

		if n == 0 {
			if totalRead == 0 {
				return 0, io.EOF
			}
			break
		}

		// Copy the read data to the destination blocks
		copied, err := safemem.CopySeq(dsts.DropFirst64(totalRead), safemem.BlockSeqOf(safemem.BlockFromSafeSlice(buf[:n])))
		if err != nil {
			return totalRead + copied, err
		}

		totalRead += uint64(n)

		// If we didn't read a full buffer, we're done
		if n < toRead {
			break
		}
	}

	return totalRead, nil
}

// Pwritev2 writes up to srcs.NumBytes() from srcs into host file descriptor
// fd. offset and flags are interpreted as for pwritev2(2).
//
// Preconditions: !srcs.IsEmpty().
func Pwritev2(fd int32, srcs safemem.BlockSeq, offset int64, flags uint32) (uint64, error) {
	// Darwin doesn't have pwritev2, so we simulate it using pwrite or write
	// No buffering is necessary regardless of safecopy; host syscalls will
	// return EFAULT if appropriate, instead of raising SIGBUS.

	// If we only have a single block, use simple write or pwrite
	if srcs.NumBlocks() == 1 {
		src := srcs.Head()
		var n int
		var err error

		if offset == -1 {
			// Use regular write
			n, err = unix.Write(int(fd), unsafe.Slice((*byte)(unsafe.Pointer(uintptr(src.Addr()))), src.Len()))
		} else {
			// Use pwrite
			n, err = unix.Pwrite(int(fd), unsafe.Slice((*byte)(unsafe.Pointer(uintptr(src.Addr()))), src.Len()), offset)
		}

		if err != nil {
			return 0, err
		}
		return uint64(n), nil
	}

	// For multiple blocks, we need to handle them one by one
	// This is less efficient than a native pwritev2, but Darwin doesn't have that syscall
	var totalWritten uint64

	// Create a temporary buffer to do the writes
	// We'll write in chunks of reasonable size
	const chunkSize = 32 * 1024
	buf := make([]byte, min(chunkSize, int(srcs.NumBytes())))

	// Continue writing until we've written all blocks or encountered an error
	for totalWritten < srcs.NumBytes() {
		toWrite := min(len(buf), int(srcs.NumBytes()-totalWritten))

		// Copy from source blocks to our buffer
		copied, err := safemem.CopySeq(safemem.BlockSeqOf(safemem.BlockFromSafeSlice(buf[:toWrite])), srcs.DropFirst64(totalWritten))
		if err != nil {
			if copied == 0 {
				return totalWritten, err
			}
			// We got some data, try to write it
		}

		if copied == 0 {
			// Nothing more to write
			break
		}

		var written int

		if offset == -1 {
			// Use regular write
			written, err = unix.Write(int(fd), buf[:copied])
		} else {
			// Use pwrite with adjusted offset
			written, err = unix.Pwrite(int(fd), buf[:copied], offset+int64(totalWritten))
		}

		if err != nil {
			if totalWritten > 0 || written > 0 {
				// We already wrote some data, return what we wrote
				return totalWritten + uint64(written), nil
			}
			return 0, err
		}

		totalWritten += uint64(written)

		// If we didn't write the full buffer, we're done
		if written < int(copied) {
			break
		}
	}

	return totalWritten, nil
}
