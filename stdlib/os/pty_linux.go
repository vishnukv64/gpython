//go:build linux

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"strconv"

	"golang.org/x/sys/unix"
)

// openptyFds opens a pseudo-terminal pair through /dev/ptmx, as glibc's
// openpty does; both descriptors are close-on-exec.
func openptyFds() (master, slave int, err error) {
	master, err = unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return
	}
	if err = unix.IoctlSetPointerInt(master, unix.TIOCSPTLCK, 0); err == nil {
		var n int
		if n, err = unix.IoctlGetInt(master, unix.TIOCGPTN); err == nil {
			slave, err = unix.Open("/dev/pts/"+strconv.Itoa(n), unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
		}
	}
	if err != nil {
		unix.Close(master)
	}
	return
}
