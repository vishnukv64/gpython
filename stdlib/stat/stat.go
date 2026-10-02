// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stat provides the implementation of python's 'stat' module: the
// constants and helpers for interpreting os.stat() results.
//
// The mode bits are taken from the host through syscall rather than
// transcribed, so the numbers match what this machine's os.stat() reports.
package stat

import (
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Constants and functions for interpreting the results of os.stat(),
os.lstat() and os.fstat().`

func init() {
	globals := py.NewStringDict()

	// File mode bits, from the host.
	for name, value := range map[string]uint32{
		"S_IFMT":   syscall.S_IFMT,
		"S_IFDIR":  syscall.S_IFDIR,
		"S_IFCHR":  syscall.S_IFCHR,
		"S_IFBLK":  syscall.S_IFBLK,
		"S_IFREG":  syscall.S_IFREG,
		"S_IFIFO":  syscall.S_IFIFO,
		"S_IFLNK":  syscall.S_IFLNK,
		"S_IFSOCK": syscall.S_IFSOCK,
		"S_ISUID":  syscall.S_ISUID,
		"S_ISGID":  syscall.S_ISGID,
		"S_ISVTX":  syscall.S_ISVTX,
		"S_IRWXU":  syscall.S_IRWXU,
		"S_IRUSR":  syscall.S_IRUSR,
		"S_IWUSR":  syscall.S_IWUSR,
		"S_IXUSR":  syscall.S_IXUSR,
		"S_IRWXG":  syscall.S_IRWXG,
		"S_IRGRP":  syscall.S_IRGRP,
		"S_IWGRP":  syscall.S_IWGRP,
		"S_IXGRP":  syscall.S_IXGRP,
		"S_IRWXO":  syscall.S_IRWXO,
		"S_IROTH":  syscall.S_IROTH,
		"S_IWOTH":  syscall.S_IWOTH,
		"S_IXOTH":  syscall.S_IXOTH,
	} {
		globals.Set(name, py.Int(int64(value)))
	}

	// The mode predicates.  They take a mode, which is the st_mode field of
	// an os.stat_result, and answer a bool.
	globals.Set("S_ISDIR", predicate("S_ISDIR", func(mode uint32) bool { return mode&syscall.S_IFMT == syscall.S_IFDIR }))
	globals.Set("S_ISREG", predicate("S_ISREG", func(mode uint32) bool { return mode&syscall.S_IFMT == syscall.S_IFREG }))
	globals.Set("S_ISCHR", predicate("S_ISCHR", func(mode uint32) bool { return mode&syscall.S_IFMT == syscall.S_IFCHR }))
	globals.Set("S_ISBLK", predicate("S_ISBLK", func(mode uint32) bool { return mode&syscall.S_IFMT == syscall.S_IFBLK }))
	globals.Set("S_ISFIFO", predicate("S_ISFIFO", func(mode uint32) bool { return mode&syscall.S_IFMT == syscall.S_IFIFO }))
	globals.Set("S_ISLNK", predicate("S_ISLNK", func(mode uint32) bool { return mode&syscall.S_IFMT == syscall.S_IFLNK }))
	globals.Set("S_ISSOCK", predicate("S_ISSOCK", func(mode uint32) bool { return mode&syscall.S_IFMT == syscall.S_IFSOCK }))

	globals.Set("filemode", py.MustNewMethod("filemode", filemode, 0, "Convert a file mode to a string of the form '-rwxrwxrwx'."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "stat",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// predicate builds one of the S_IS* functions.
func predicate(name string, test func(uint32) bool) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		var mode py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, name, 1, 1, &mode); err != nil {
			return nil, err
		}
		n, err := py.IndexInt(mode)
		if err != nil {
			return nil, err
		}
		return py.NewBool(test(uint32(n))), nil
	}, 0, "Test a file mode.")
}

// filemode renders a mode the way "ls -l" does.
func filemode(self py.Object, args py.Tuple) (py.Object, error) {
	var modeObj py.Object
	filetype := py.Object(py.Int(0))
	if err := py.UnpackTuple(args, py.StringDict{}, "filemode", 1, 2, &modeObj, &filetype); err != nil {
		return nil, err
	}
	mode, err := py.IndexInt(modeObj)
	if err != nil {
		return nil, err
	}
	m := uint32(mode)

	kind := byte('-')
	switch m & syscall.S_IFMT {
	case syscall.S_IFDIR:
		kind = 'd'
	case syscall.S_IFCHR:
		kind = 'c'
	case syscall.S_IFBLK:
		kind = 'b'
	case syscall.S_IFIFO:
		kind = 'p'
	case syscall.S_IFLNK:
		kind = 'l'
	case syscall.S_IFSOCK:
		kind = 's'
	}

	perms := []byte("rwxrwxrwx")
	out := make([]byte, 10)
	out[0] = kind
	for i := 0; i < 9; i++ {
		if m&(1<<uint(8-i)) == 0 {
			perms[i] = '-'
		}
	}
	copy(out[1:], perms)

	// The setuid, setgid and sticky bits replace the execute characters.
	if m&syscall.S_ISUID != 0 {
		if out[3] == 'x' {
			out[3] = 's'
		} else {
			out[3] = 'S'
		}
	}
	if m&syscall.S_ISGID != 0 {
		if out[6] == 'x' {
			out[6] = 's'
		} else {
			out[6] = 'S'
		}
	}
	if m&syscall.S_ISVTX != 0 {
		if out[9] == 'x' {
			out[9] = 't'
		} else {
			out[9] = 'T'
		}
	}
	return py.String(string(out)), nil
}
