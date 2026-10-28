// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import "syscall"

func fsyncFD(fd int) error { return syscall.FlushFileBuffers(syscall.Handle(fd)) }

// umask is the process-wide file creation mask; Windows file creation does not
// consult it, but os.umask keeps CPython's set-and-return-previous contract.
var umask int

func setUmask(mask int) int {
	prev := umask
	umask = mask
	return prev
}
