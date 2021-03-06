// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd

package poll

import (
	"io"
	"syscall"
	"unsafe"
)

// Writev wraps the writev system call.
func (fd *FD) Writev(v *[][]byte) (int64, error) {
	if err := fd.writeLock(); err != nil {
		return 0, err
	}
	defer fd.writeUnlock()
	if err := fd.pd.prepareWrite(); err != nil {
		return 0, err
	}

	var iovecs []syscall.Iovec
	if fd.iovecs != nil {
		iovecs = *fd.iovecs
	}
	// TODO: read from sysconf(_SC_IOV_MAX)? The Linux default is
	// 1024 and this seems conservative enough for now. Darwin's
	// UIO_MAXIOV also seems to be 1024.
	maxVec := 1024

	var n int64
	var err error
	for len(*v) > 0 {
		iovecs = iovecs[:0]
		for _, chunk := range *v {
			if len(chunk) == 0 {
				continue
			}
			iovecs = append(iovecs, syscall.Iovec{Base: &chunk[0]})
			if fd.IsStream && len(chunk) > 1<<30 {
				iovecs[len(iovecs)-1].SetLen(1 << 30)
				break // continue chunk on next writev
			}
			iovecs[len(iovecs)-1].SetLen(len(chunk))
			if len(iovecs) == maxVec {
				break
			}
		}
		if len(iovecs) == 0 {
			break
		}
		fd.iovecs = &iovecs // cache

		wrote, _, e0 := syscall.Syscall(syscall.SYS_WRITEV,
			uintptr(fd.Sysfd),
			uintptr(unsafe.Pointer(&iovecs[0])),
			uintptr(len(iovecs)))
		if wrote == ^uintptr(0) {
			wrote = 0
		}
		TestHookDidWritev(int(wrote))
		n += int64(wrote)
		consume(v, int64(wrote))
		if e0 == syscall.EAGAIN {
			if err = fd.pd.waitWrite(); err == nil {
				continue
			}
		} else if e0 != 0 {
			err = syscall.Errno(e0)
		}
		if err != nil {
			break
		}
		if n == 0 {
			err = io.ErrUnexpectedEOF
			break
		}
	}
	return n, err
}
