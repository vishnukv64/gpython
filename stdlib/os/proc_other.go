//go:build windows || plan9 || js || wasip1

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"os"

	"github.com/vishnukv64/gpython/py"
)

// No POSIX descriptors or wait4 here: multiprocessing is POSIX-only.
func unixMethods() []*py.Method { return nil }

func procConsts() map[string]int {
	return map[string]int{"O_RDONLY": os.O_RDONLY, "O_WRONLY": os.O_WRONLY, "O_RDWR": os.O_RDWR,
		"O_CREAT": os.O_CREATE, "O_EXCL": os.O_EXCL, "O_TRUNC": os.O_TRUNC, "O_APPEND": os.O_APPEND}
}
