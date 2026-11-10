// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(darwin || freebsd || netbsd || openbsd || linux || windows)

package py

import "errors"

// IsTerminal answers False where no termios ioctl is wired up, which is the
// safe answer: a caller that sees False does not prompt.
func IsTerminal(fd uintptr) bool { return false }

// TerminalSize is not available here.
func TerminalSize(fd uintptr) (columns, lines int, err error) {
	return 0, 0, errors.ErrUnsupported
}
