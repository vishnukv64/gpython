// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(darwin || freebsd || netbsd || openbsd || linux || windows)

package os

import (
	"os"

	"github.com/vishnukv64/gpython/py"
)

// sysStatFields reports false here: there is no syscall.Stat_t, so
// NewStatResult falls back to the portable os.FileInfo.
func sysStatFields(fi os.FileInfo) (statFields, bool) { return statFields{}, false }

// fstatResult is not supported here.
func fstatResult(fd int) (py.Object, error) {
	return nil, py.ExceptionNewf(py.NotImplementedError, "os.fstat is not supported on this platform")
}
