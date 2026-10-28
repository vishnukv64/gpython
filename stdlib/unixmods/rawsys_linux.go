//go:build linux

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unixmods

import "golang.org/x/sys/unix"

const (
	sysIoctl = unix.SYS_IOCTL
	sysFcntl = unix.SYS_FCNTL
)

func rawSyscall(trap, a1, a2, a3 uintptr) (uintptr, error) {
	r, _, e := unix.Syscall(trap, a1, a2, a3)
	if e != 0 {
		return 0, e
	}
	return r, nil
}
