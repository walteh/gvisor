//go:build darwin

package fd

// Darwin Plan:
// This package currently fails to build on Darwin because it uses the
// Linux-specific open flag unix.O_LARGEFILE.
//
// Required Changes:
// 1. Remove the use of unix.O_LARGEFILE when building for Darwin.
//    Large file support (LFS) is typically implicit/standard on modern macOS,
//    so the flag is unnecessary and undefined.
// 2. This might require conditional logic (build tags or runtime checks)
//    around file opening calls.
