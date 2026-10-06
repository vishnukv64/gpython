// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(darwin || freebsd || netbsd || openbsd || linux)

package py

// isTerminal answers False where no termios ioctl is wired up, which is the
// safe answer: a caller that sees False does not prompt.
func isTerminal(fd uintptr) bool { return false }
