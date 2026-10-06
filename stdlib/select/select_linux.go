// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package selectmod

import "syscall"

// doSelect is select(2).  linux's syscall.Select also returns the ready count,
// which is not needed: readiness is read back from the sets.
func doSelect(nfd int, r, w, x *syscall.FdSet, tv *syscall.Timeval) error {
	_, err := syscall.Select(nfd, r, w, x, tv)
	return err
}
