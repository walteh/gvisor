// Copyright 2018 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package unet

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// buildIovec builds an iovec slice from the given []byte slice.
//
// iovecs is used as an initial slice, to avoid excessive allocations.
func buildIovec(bufs [][]byte, iovecs []unix.Iovec) ([]unix.Iovec, int) {
	var length int
	for i := range bufs {
		if l := len(bufs[i]); l > 0 {
			iovecs = append(iovecs, unix.Iovec{
				Base: &bufs[i][0],
				Len:  uint64(l),
			})
			length += l
		}
	}
	return iovecs, length
}

// getsockopt issues a getsockopt unix.
func getsockopt(fd int, level int, optname int, buf []byte) (uint32, error) {
	l := uint32(len(buf))
	_, _, e := unix.RawSyscall6(unix.SYS_GETSOCKOPT, uintptr(fd), uintptr(level), uintptr(optname), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&l)), 0)
	if e != 0 {
		return 0, e
	}

	return l, nil
}

// setsockopt issues a setsockopt unix.
func setsockopt(fd int, level int, optname int, buf []byte) error {
	_, _, e := unix.RawSyscall6(unix.SYS_SETSOCKOPT, uintptr(fd), uintptr(level), uintptr(optname), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if e != 0 {
		return e
	}

	return nil
}

// getsockname issues a getsockname unix.
func getsockname(fd int, buf []byte) (uint32, error) {
	l := uint32(len(buf))
	_, _, e := unix.RawSyscall(unix.SYS_GETSOCKNAME, uintptr(fd), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&l)))
	if e != 0 {
		return 0, e
	}

	return l, nil
}

// getpeername issues a getpeername unix.
func getpeername(fd int, buf []byte) (uint32, error) {
	l := uint32(len(buf))
	_, _, e := unix.RawSyscall(unix.SYS_GETPEERNAME, uintptr(fd), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&l)))
	if e != 0 {
		return 0, e
	}

	return l, nil
}
