// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socket

import (
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// windows has no SO_REUSEPORT, so the module does not define it, as in
// CPython.  -1 matches no option a caller can pass to setsockopt.
var SO_REUSEPORT = -1

var platformGlobals []py.DictEntry

// windows' SetsockoptInt takes a Handle, not an int.
func setReuseAddr(fd uintptr) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}
