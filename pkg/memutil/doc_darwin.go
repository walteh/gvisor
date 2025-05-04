//go:build darwin

package memutil

// Darwin Plan:
// This package currently fails to build on Darwin because other packages
// (like pkg/sentry/usage) call memutil.CreateMemFD, which wraps the
// Linux-specific memfd_create(2) syscall.
//
// Required Changes:
// 1. Provide a Darwin implementation or stub for CreateMemFD.
// 2. Darwin does not have a direct equivalent to memfd_create.
//    Potential alternatives depending on the exact use case:
//    a) Use standard anonymous memory mapping (unix.Mmap with MAP_ANON).
//    b) Use POSIX shared memory (shm_open / shm_unlink) if a name/identifier
//       is acceptable.
//    c) Use a temporary file on disk if persistence isn't a major concern
//       and performance is acceptable.
// 3. The choice depends on how CreateMemFD is used by callers - specifically
//    whether they rely on the ability to get a file descriptor referring to
//    anonymous memory.
