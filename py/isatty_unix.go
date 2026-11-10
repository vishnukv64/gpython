// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || freebsd || netbsd || openbsd || linux

package py

import (
	"syscall"
	"unsafe"
)

// IsTerminal reports whether fd is a terminal, the way C's isatty(3) does: by
// asking for its terminal settings.  A character-device check alone is wrong -
// /dev/null is a character device and is not a tty.
//
// This uses the standard library rather than golang.org/x/term because adding
// that module raises go.mod's go directive to 1.25, and CI builds on 1.18.
func IsTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, ioctlReadTermios,
		uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	return errno == 0
}

// TerminalSize is the window size of the terminal on fd (TIOCGWINSZ).  The
// error is the raw errno, ENOTTY or EBADF, for the caller to wrap.
func TerminalSize(fd uintptr) (columns, lines int, err error) {
	var ws [4]uint16 // row, col, xpixel, ypixel
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0, 0, errno
	}
	return int(ws[1]), int(ws[0]), nil
}
