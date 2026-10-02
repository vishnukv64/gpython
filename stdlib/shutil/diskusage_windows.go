// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package shutil

import (
	"github.com/vishnukv64/gpython/py"
)

// diskUsageQuery is the Windows variant.  syscall has no Statfs there and
// this interpreter does not call GetDiskFreeSpaceEx, so the query is refused
// rather than answered with a wrong number.
func diskUsageQuery(path string) (total, used, free int64, err error) {
	return 0, 0, 0, py.ExceptionNewf(py.NotImplementedError,
		"shutil.disk_usage is not implemented on this platform")
}
