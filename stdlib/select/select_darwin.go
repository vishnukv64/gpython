// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package selectmod

import "syscall"

// doSelect is select(2).  darwin's syscall.Select returns only an error.
func doSelect(nfd int, r, w, x *syscall.FdSet, tv *syscall.Timeval) error {
	return syscall.Select(nfd, r, w, x, tv)
}
