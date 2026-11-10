// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import "golang.org/x/sys/windows"

// A descriptor here is the OS handle itself (see OwnFD).

// IsTerminal reports whether fd is a console, as CPython's isatty does there.
func IsTerminal(fd uintptr) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(fd), &mode) == nil
}

// TerminalSize is CPython's os.get_terminal_size on Windows: the visible
// window of the console screen buffer.  A handle that is not a console fails
// with ERROR_INVALID_HANDLE, which OSError reports as errno EBADF (9), as
// CPython does.
func TerminalSize(fd uintptr) (columns, lines int, err error) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info); err != nil {
		return 0, 0, err
	}
	w := info.Window
	return int(w.Right-w.Left) + 1, int(w.Bottom-w.Top) + 1, nil
}
