// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import "syscall"

// Linux's statfs(2) carries every statvfs field; f_favail is f_ffree, as glibc reports it.
func statfsToVfs(st *syscall.Statfs_t) ([10]int64, int64) {
	return [10]int64{int64(st.Bsize), int64(st.Frsize), int64(st.Blocks), int64(st.Bfree),
		int64(st.Bavail), int64(st.Files), int64(st.Ffree), int64(st.Ffree),
		int64(st.Flags), int64(st.Namelen)}, int64(uint32(st.Fsid.X__val[0])) | int64(uint32(st.Fsid.X__val[1]))<<32
}

func statvfsFields(path string) ([10]int64, int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return [10]int64{}, 0, err
	}
	f, fsid := statfsToVfs(&st)
	return f, fsid, nil
}

func fstatvfsFields(fd int) ([10]int64, int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Fstatfs(fd, &st); err != nil {
		return [10]int64{}, 0, err
	}
	f, fsid := statfsToVfs(&st)
	return f, fsid, nil
}
