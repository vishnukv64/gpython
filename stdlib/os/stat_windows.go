// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"os"

	"github.com/vishnukv64/gpython/py"
)

// sysStatFields reports false: there is no syscall.Stat_t, so NewStatResult
// falls back to the portable os.FileInfo.
func sysStatFields(fi os.FileInfo) (statFields, bool) { return statFields{}, false }

// fstatResult stats the handle behind a descriptor (Windows fds are handles).
// TakeFD/OwnFD bracket the call so the temporary *os.File is never finalized,
// which would close a handle Python still owns.
func fstatResult(fd int) (py.Object, error) {
	f := py.TakeFD(uintptr(fd), "")
	defer py.OwnFD(f)
	fi, err := f.Stat()
	if err != nil {
		return nil, py.OSErrorFrom(err, "")
	}
	return NewStatResult(fi), nil
}
