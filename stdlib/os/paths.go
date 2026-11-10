// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"os"
	"runtime"

	"github.com/vishnukv64/gpython/py"
)

// hostSplit is os.path.split for the host's flavour.
func hostSplit(p string) (string, string) {
	if runtime.GOOS == "windows" {
		return ntSplit(p)
	}
	return posixSplit(p)
}

// truncate is os.truncate(path, length); a descriptor makes it ftruncate.
func truncate(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var path, length py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO:truncate", []string{"path", "length"}, &path, &length); err != nil {
		return nil, err
	}
	p, err := getPath("truncate", path, pathOrFdKinds)
	if err != nil {
		return nil, err
	}
	n, err := py.IndexInt(length)
	if err != nil {
		return nil, err
	}
	if p.isFd {
		err = withFD(p.fd, func(f *os.File) error { return f.Truncate(int64(n)) })
		if err != nil {
			return nil, py.OSErrorFromObj(err, nil)
		}
		return py.None, nil
	}
	if err := os.Truncate(p.s, int64(n)); err != nil {
		return nil, p.fail(err)
	}
	return py.None, nil
}

// chownAs is chown (follow) and lchown (no follow).
func chownAs(fn string, kinds string, follow bool) func(py.Object, py.Tuple, py.StringDict) (py.Object, error) {
	return func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var path, uid, gid, follow2 py.Object
		format := "OOO:" + fn
		names := []string{"path", "uid", "gid"}
		dst := []*py.Object{&path, &uid, &gid}
		if follow {
			format = "OOO|$OO:" + fn
			names = append(names, "dir_fd", "follow_symlinks")
			dst = append(dst, new(py.Object), &follow2)
		}
		if err := py.ParseTupleAndKeywords(args, kwargs, format, names, dst...); err != nil {
			return nil, err
		}
		p, err := getPath(fn, path, kinds)
		if err != nil {
			return nil, err
		}
		u, err := py.IndexInt(uid)
		if err != nil {
			return nil, err
		}
		g, err := py.IndexInt(gid)
		if err != nil {
			return nil, err
		}
		switch {
		case p.isFd:
			err = withFD(p.fd, func(f *os.File) error { return f.Chown(u, g) })
		case follow && (follow2 == nil || follow2 != py.Object(py.False)):
			err = os.Chown(p.s, u, g)
		default:
			err = os.Lchown(p.s, u, g)
		}
		if err != nil {
			return nil, p.fail(err)
		}
		return py.None, nil
	}
}

// mkfifo is os.mkfifo(path, mode=0o666, *, dir_fd=None).
func mkfifo(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var path py.Object
	var mode py.Object = py.Int(0o666)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O$O:mkfifo", []string{"path", "mode", "dir_fd"}, &path, &mode, new(py.Object)); err != nil {
		return nil, err
	}
	p, err := getPath("mkfifo", path, pathKinds)
	if err != nil {
		return nil, err
	}
	m, err := py.IndexInt(mode)
	if err != nil {
		return nil, err
	}
	if err := mkfifoSys(p.s, uint32(m)); err != nil {
		return nil, p.fail(err)
	}
	return py.None, nil
}

// terminalSize is the Go half of os.get_terminal_size.
func terminalSize(self py.Object, args py.Tuple) (py.Object, error) {
	fd, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	c, l, err := py.TerminalSize(uintptr(fd))
	if err != nil {
		return nil, py.OSErrorFrom(err, "")
	}
	return py.Tuple{py.Int(c), py.Int(l)}, nil
}

// pathMethods are the os functions that take paths and are not in os.go.
func pathMethods() []*py.Method {
	ms := []*py.Method{
		py.MustNewMethod("_terminal_size", terminalSize, 0, "_terminal_size(fd) -> (columns, lines)"),
		py.MustNewMethod("truncate", truncate, 0, "truncate(path, length) -> Truncate a file, specified by path, to a specific length."),
	}
	if hasMkfifo {
		ms = append(ms, py.MustNewMethod("mkfifo", mkfifo, 0, "mkfifo(path, mode=0o666, *, dir_fd=None) -> Create a fifo (a POSIX named pipe)."))
	}
	if runtime.GOOS != "windows" {
		ms = append(ms,
			py.MustNewMethod("chown", chownAs("chown", pathOrFdKinds, true), 0, "chown(path, uid, gid, *, dir_fd=None, follow_symlinks=True) -> Change the owner and group id of path."),
			py.MustNewMethod("lchown", chownAs("lchown", pathKinds, false), 0, "lchown(path, uid, gid) -> Change the owner and group id of path, not following a symlink."),
		)
	}
	return ms
}
