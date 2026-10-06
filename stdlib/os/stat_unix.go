// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || freebsd || netbsd || openbsd || linux

package os

import (
	"os"
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// statFieldsOf reads the raw stat.  Every field is converted explicitly because
// their Go types differ by platform - Dev is int32 on darwin and uint64 on
// linux, Nlink is uint16 on darwin and uint32 on linux/386.
func statFieldsOf(st *syscall.Stat_t) statFields {
	a, m, c := statTimes(st)
	return statFields{
		mode: uint32(st.Mode), ino: uint64(st.Ino), dev: uint64(st.Dev),
		nlink: uint64(st.Nlink), uid: st.Uid, gid: st.Gid, size: st.Size,
		atimeS: int64(a.Sec), atimeNs: int64(a.Nsec),
		mtimeS: int64(m.Sec), mtimeNs: int64(m.Nsec),
		ctimeS: int64(c.Sec), ctimeNs: int64(c.Nsec),
		rdev: int64(st.Rdev), blocks: int64(st.Blocks), blksize: int64(st.Blksize),
		hasBlocks: true,
	}
}

func sysStatFields(fi os.FileInfo) (statFields, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return statFields{}, false
	}
	return statFieldsOf(st), true
}

func fstatResult(fd int) (py.Object, error) {
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return nil, py.OSErrorFrom(err, "")
	}
	return newStatResult(statFieldsOf(&st)), nil
}
