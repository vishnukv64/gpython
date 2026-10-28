//go:build darwin

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package unixmods

import "syscall"

// x/sys/unix marks its darwin SYS_* numbers and Syscall deprecated (macOS
// wants libSystem); the syscall package routes these two through libSystem.
const (
	sysIoctl = syscall.SYS_IOCTL
	sysFcntl = syscall.SYS_FCNTL
)

func rawSyscall(trap, a1, a2, a3 uintptr) (uintptr, error) {
	r, _, e := syscall.Syscall(trap, a1, a2, a3)
	if e != 0 {
		return 0, e
	}
	return r, nil
}
