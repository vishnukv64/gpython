// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || freebsd || netbsd || openbsd || linux

package py

import (
	"syscall"
	"unsafe"
)

// isTerminal reports whether fd is a terminal, the way C's isatty(3) does: by
// asking for its terminal settings.  A character-device check alone is wrong -
// /dev/null is a character device and is not a tty.
//
// This uses the standard library rather than golang.org/x/term because adding
// that module raises go.mod's go directive to 1.25, and CI builds on 1.18.
func isTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, ioctlReadTermios,
		uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	return errno == 0
}
