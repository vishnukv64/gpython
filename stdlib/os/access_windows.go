// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"os"
	"syscall"
)

// unixAccess is CPython's windows os.access: the path must exist, and W_OK
// fails only on a file with the read-only attribute, which Go reports as a
// mode with no write bit.  Windows has no access(2), and its permissions are
// not the owner/group/other bits.
func unixAccess(path string, mask uint32) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if mask&2 != 0 && !fi.IsDir() && fi.Mode().Perm()&0o200 == 0 {
		return syscall.EACCES
	}
	return nil
}
