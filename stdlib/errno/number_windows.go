// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package errno

import (
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// numberOf is the MSVC CRT number CPython reports on Windows, where Go's
// syscall.E* values are invented; names Windows lacks are dropped.
func numberOf(e syscall.Errno) (int, bool) { return py.CRTErrno(e) }
