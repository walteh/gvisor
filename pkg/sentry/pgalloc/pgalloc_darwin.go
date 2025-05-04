//go:build darwin

package pgalloc

import (
	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/log"
	"gvisor.dev/gvisor/pkg/safemem"
	"gvisor.dev/gvisor/pkg/sentry/memmap"
)

func (f *MemoryFile) madviseChunkMapping(addr, len uintptr, huge bool) {
	// Darwin doesn't support MADV_HUGEPAGE or MADV_NOHUGEPAGE
	// macOS has VM_FLAGS_SUPERPAGE_SIZE_ANY for superpages,
	// but it must be set at allocation time via vm_allocate,
	// not via madvise after allocation.

	// However, we can still use MADV_FREE to mark pages as
	// free-able if they're not touched, which is a reasonable
	// substitute behavior for non-huge pages.
	if !huge && f.opts.AdviseNoHugepage {
		_, _, errno := unix.Syscall(unix.SYS_MADVISE, addr, len, unix.MADV_FREE)
		if errno != 0 {
			// Log this failure but continue.
			log.Warningf("madvise(%#x, %d, MADV_FREE) failed: %s", addr, len, errno)
		}
	}
}
func tryPopulateMadv(b safemem.Block) bool {
	// Darwin doesn't have MADV_POPULATE_WRITE.
	// Return true to indicate the caller should fall back to
	// manual page population.
	return true
}

func (f *MemoryFile) commitFile(fr memmap.FileRange) error {
	// Darwin doesn't have fallocate, but ftruncate can extend a file.
	// This approach doesn't pre-allocate blocks, but extends the logical size.
	// Note: This doesn't actually reserve disk space like fallocate does.
	end := int64(fr.End)

	// Get current size
	current, err := unix.Seek(int(f.file.Fd()), 0, unix.SEEK_END)
	if err != nil {
		return err
	}

	// Only extend if needed
	if current < end {
		return unix.Ftruncate(int(f.file.Fd()), end)
	}

	return nil
}

func (f *MemoryFile) decommitFile(fr memmap.FileRange) error {
	// Darwin doesn't have FALLOC_FL_PUNCH_HOLE.
	// We'll use a simpler approach of writing zeros to the file region.
	// This is less efficient but will work on Darwin.

	// On Darwin, we have fewer options for space-efficient file hole punching.
	// Let's use a simple solution that's compatible with the Darwin interface:
	// just write zeros to the specified range.

	// Create a moderate-sized zero buffer (1MB) to avoid excessive memory usage
	const bufSize = 1 << 20 // 1MB
	zeroBuf := make([]byte, bufSize)

	// Seek to the start position
	_, err := unix.Seek(int(f.file.Fd()), int64(fr.Start), unix.SEEK_SET)
	if err != nil {
		return err
	}

	// Write zeros in chunks
	remaining := int64(fr.Length())
	for remaining > 0 {
		writeSize := remaining
		if writeSize > bufSize {
			writeSize = bufSize
		}

		n, err := unix.Write(int(f.file.Fd()), zeroBuf[:writeSize])
		if err != nil {
			return err
		}
		if n <= 0 {
			break
		}

		remaining -= int64(n)
	}

	return nil
}
