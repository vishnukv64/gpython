// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || darwin

package os

import (
	"fmt"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// statvfsNames are os.statvfs_result's sequence fields, in CPython's order.
var statvfsNames = [10]string{"f_bsize", "f_frsize", "f_blocks", "f_bfree", "f_bavail",
	"f_files", "f_ffree", "f_favail", "f_flag", "f_namemax"}

// StatvfsResult is os.statvfs_result: a 10-item struct sequence plus f_fsid.
type StatvfsResult struct {
	Dict py.StringDict
	seq  [10]py.Object
}

var StatvfsResultType = py.NewTypeX("statvfs_result",
	"statvfs_result: Result from statvfs or fstatvfs.", nil, nil)

func (s *StatvfsResult) Type() *py.Type               { return StatvfsResultType }
func (s *StatvfsResult) GetDict() py.StringDict       { return s.Dict }
func (s *StatvfsResult) M__len__() (py.Object, error) { return py.Int(len(s.seq)), nil }

func (s *StatvfsResult) M__iter__() (py.Object, error) {
	return py.NewListFromItems(s.seq[:]).M__iter__()
}

func (s *StatvfsResult) M__getitem__(key py.Object) (py.Object, error) {
	return py.Tuple(s.seq[:]).M__getitem__(key)
}

func (s *StatvfsResult) M__repr__() (py.Object, error) {
	var b strings.Builder
	b.WriteString("os.statvfs_result(")
	for i, name := range statvfsNames {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s=%d", name, int64(s.seq[i].(py.Int)))
	}
	b.WriteString(")")
	return py.String(b.String()), nil
}

func newStatvfsResult(f [10]int64, fsid int64) *StatvfsResult {
	s := &StatvfsResult{Dict: py.NewStringDict()}
	for i, v := range f {
		s.seq[i] = py.Int(v)
		s.Dict.Set(statvfsNames[i], s.seq[i])
	}
	s.Dict.Set("f_fsid", py.Int(fsid))
	return s
}

// osStatvfs is os.statvfs(path) and, given an int, os.fstatvfs(fd).
func osStatvfs(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "statvfs() takes exactly one argument (%d given)", len(args))
	}
	if fd, ok := args[0].(py.Int); ok {
		f, fsid, err := fstatvfsFields(int(fd))
		if err != nil {
			return nil, py.OSErrorFrom(err, "")
		}
		return newStatvfsResult(f, fsid), nil
	}
	p, err := fsPath(args[0])
	if err != nil {
		return nil, err
	}
	f, fsid, err := statvfsFields(p)
	if err != nil {
		return nil, py.OSErrorFrom(err, p)
	}
	return newStatvfsResult(f, fsid), nil
}

func statvfsMethods() []*py.Method {
	return []*py.Method{
		py.MustNewMethod("statvfs", osStatvfs, 0, "statvfs(path) -> statvfs_result for the filesystem holding path."),
		py.MustNewMethod("fstatvfs", osStatvfs, 0, "fstatvfs(fd) -> statvfs_result for the filesystem holding fd."),
	}
}

func init() {
	StatvfsResultType.Dict.Set("__module__", py.String("os"))
}
