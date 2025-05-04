//go:build darwin

package hostfd

// Darwin Plan:
// This package currently fails to build on Darwin due to:
// 1. Missing Linux-specific syscall constants:
//    - unix.SYS_PREAD64
//    - unix.SYS_PREADV2
//    - unix.SYS_PWRITE64
//    - unix.SYS_PWRITEV2
// 2. Undefined constant: MaxReadWriteIov.
//    This is likely a Linux-specific limit for iovec arrays.
//
// Required Changes:
// 1. Similar to pkg/aio, use standard Go functions (unix.Pread, Pwrite,
//    Preadv, Pwritev) if possible.
// 2. If direct syscalls are needed, conditionalize using build tags or runtime
//    checks and use the correct Darwin syscall numbers.
// 3. Define an appropriate MaxReadWriteIov constant for Darwin, potentially
//    using unix.IOV_MAX or deriving it from system limits if necessary.
//    Place this definition in a darwin-specific file or use build tags.
