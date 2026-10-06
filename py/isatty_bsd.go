// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || freebsd || netbsd || openbsd

package py

import "syscall"

// ioctlReadTermios is the request that reads a terminal's settings; it fails
// on anything that is not a terminal, which is exactly isatty's question.
const ioctlReadTermios = syscall.TIOCGETA
