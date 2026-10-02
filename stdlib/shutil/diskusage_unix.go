// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package shutil

import (
	"syscall"
)

// diskUsageQuery asks statfs about path and answers (total, used, free) in
// bytes, which is what CPython's os.statvfs call computes.
//
// used is total minus the blocks available to an unprivileged process - the
// same arithmetic as CPython's shutil.disk_usage, so the numbers agree with
// "df" as far as the reserved-blocks convention goes.
func diskUsageQuery(path string) (total, used, free int64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0, oserr(err)
	}
	bsize := int64(st.Bsize)
	total = int64(st.Blocks) * bsize
	free = int64(st.Bavail) * bsize
	used = total - free
	return total, used, free, nil
}
