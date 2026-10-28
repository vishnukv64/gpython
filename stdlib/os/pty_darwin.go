//go:build darwin

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openptyFds opens a pseudo-terminal pair through /dev/ptmx, granting and
// unlocking the slave as libutil's openpty does; both descriptors are
// close-on-exec.
func openptyFds() (master, slave int, err error) {
	master, err = unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return
	}
	var name [128]byte
	if err = unix.IoctlSetInt(master, unix.TIOCPTYGRANT, 0); err == nil {
		if err = unix.IoctlSetInt(master, unix.TIOCPTYUNLK, 0); err == nil {
			if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(master), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); e != 0 {
				err = e
			} else {
				n := 0
				for name[n] != 0 {
					n++
				}
				slave, err = unix.Open(string(name[:n]), unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
			}
		}
	}
	if err != nil {
		unix.Close(master)
	}
	return
}
