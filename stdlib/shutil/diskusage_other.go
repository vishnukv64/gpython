// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !unix && !windows

package shutil

import (
	"github.com/vishnukv64/gpython/py"
)

// diskUsageQuery is the fallback for the remaining targets (js/wasm and
// plan9), where syscall has no Statfs.  It refuses rather than guessing.
func diskUsageQuery(path string) (total, used, free int64, err error) {
	return 0, 0, 0, py.ExceptionNewf(py.NotImplementedError,
		"shutil.disk_usage is not implemented on this platform")
}
