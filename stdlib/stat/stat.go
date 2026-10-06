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
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Constants and functions for interpreting the results of os.stat(),
os.lstat() and os.fstat().`

// The mode bits, as Lib/stat.py defines them: the same numbers on every
// platform, which is why they are literals rather than syscall's - windows'
// syscall has no S_IRWXU, and os.stat fills st_mode with these bits there too.
const (
	S_IFMT   = 0o170000
	S_IFDIR  = 0o40000
	S_IFCHR  = 0o20000
	S_IFBLK  = 0o60000
	S_IFREG  = 0o100000
	S_IFIFO  = 0o10000
	S_IFLNK  = 0o120000
	S_IFSOCK = 0o140000
	S_ISUID  = 0o4000
	S_ISGID  = 0o2000
	S_ISVTX  = 0o1000
	S_IRWXU  = 0o700
	S_IRUSR  = 0o400
	S_IWUSR  = 0o200
	S_IXUSR  = 0o100
	S_IRWXG  = 0o70
	S_IRGRP  = 0o40
	S_IWGRP  = 0o20
	S_IXGRP  = 0o10
	S_IRWXO  = 0o7
	S_IROTH  = 0o4
	S_IWOTH  = 0o2
	S_IXOTH  = 0o1
)

func init() {
	globals := py.NewStringDict()

	// File mode bits, from the host.
	for name, value := range map[string]uint32{
		"S_IFMT":   S_IFMT,
		"S_IFDIR":  S_IFDIR,
		"S_IFCHR":  S_IFCHR,
		"S_IFBLK":  S_IFBLK,
		"S_IFREG":  S_IFREG,
		"S_IFIFO":  S_IFIFO,
		"S_IFLNK":  S_IFLNK,
		"S_IFSOCK": S_IFSOCK,
		"S_ISUID":  S_ISUID,
		"S_ISGID":  S_ISGID,
		"S_ISVTX":  S_ISVTX,
		"S_IRWXU":  S_IRWXU,
		"S_IRUSR":  S_IRUSR,
		"S_IWUSR":  S_IWUSR,
		"S_IXUSR":  S_IXUSR,
		"S_IRWXG":  S_IRWXG,
		"S_IRGRP":  S_IRGRP,
		"S_IWGRP":  S_IWGRP,
		"S_IXGRP":  S_IXGRP,
		"S_IRWXO":  S_IRWXO,
		"S_IROTH":  S_IROTH,
		"S_IWOTH":  S_IWOTH,
		"S_IXOTH":  S_IXOTH,
	} {
		globals.Set(name, py.Int(int64(value)))
	}

	// The mode predicates.  They take a mode, which is the st_mode field of
	// an os.stat_result, and answer a bool.
	globals.Set("S_ISDIR", predicate("S_ISDIR", func(mode uint32) bool { return mode&S_IFMT == S_IFDIR }))
	globals.Set("S_ISREG", predicate("S_ISREG", func(mode uint32) bool { return mode&S_IFMT == S_IFREG }))
	globals.Set("S_ISCHR", predicate("S_ISCHR", func(mode uint32) bool { return mode&S_IFMT == S_IFCHR }))
	globals.Set("S_ISBLK", predicate("S_ISBLK", func(mode uint32) bool { return mode&S_IFMT == S_IFBLK }))
	globals.Set("S_ISFIFO", predicate("S_ISFIFO", func(mode uint32) bool { return mode&S_IFMT == S_IFIFO }))
	globals.Set("S_ISLNK", predicate("S_ISLNK", func(mode uint32) bool { return mode&S_IFMT == S_IFLNK }))
	globals.Set("S_ISSOCK", predicate("S_ISSOCK", func(mode uint32) bool { return mode&S_IFMT == S_IFSOCK }))

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
	switch m & S_IFMT {
	case S_IFDIR:
		kind = 'd'
	case S_IFCHR:
		kind = 'c'
	case S_IFBLK:
		kind = 'b'
	case S_IFIFO:
		kind = 'p'
	case S_IFLNK:
		kind = 'l'
	case S_IFSOCK:
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
	if m&S_ISUID != 0 {
		if out[3] == 'x' {
			out[3] = 's'
		} else {
			out[3] = 'S'
		}
	}
	if m&S_ISGID != 0 {
		if out[6] == 'x' {
			out[6] = 's'
		} else {
			out[6] = 'S'
		}
	}
	if m&S_ISVTX != 0 {
		if out[9] == 'x' {
			out[9] = 't'
		} else {
			out[9] = 'T'
		}
	}
	return py.String(string(out)), nil
}
