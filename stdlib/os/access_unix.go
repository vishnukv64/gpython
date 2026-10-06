// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package os

import "syscall"

// unixAccess asks the operating system whether the real uid may use a path with
// the given mask.  It is a syscall rather than a stat, because the mode bits on
// their own do not account for group membership or a read-only filesystem.
func unixAccess(path string, mask uint32) error {
	return syscall.Access(path, mask)
}
