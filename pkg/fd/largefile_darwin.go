//go:build darwin
// +build darwin

package fd

// O_LARGEFILE is a Linux-specific flag passed to open(2).
// It is generally required on 32-bit Linux systems to allow accessing files
// larger than 2GiB.
//
// On Darwin (macOS), large file support (LFS) is implicit and the O_LARGEFILE
// flag does not exist and is not needed.
//
// We define O_LARGEFILE as 0 here specifically for Darwin builds.
// This allows common code in fd.go that ORs this flag during the open(2) call
// to compile without error, while ensuring the flag has no effect on Darwin,
// matching the system's behavior.
const O_LARGEFILE = 0
