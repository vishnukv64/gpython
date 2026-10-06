// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package socket

import (
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// platformGlobals are the module entries only some systems define; CPython has
// SO_REUSEPORT on darwin and linux but not on windows.
var platformGlobals = []py.DictEntry{{Key: "SO_REUSEPORT", Value: py.Int(SO_REUSEPORT)}}

func setReuseAddr(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}
