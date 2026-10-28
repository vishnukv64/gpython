// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"os"
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// On Windows a gpython descriptor is the OS handle itself (see py.OwnFD), so
// os.read/os.write/os.pipe work on it directly, as CPython's do on its CRT
// descriptors.  No wait4 or fork: multiprocessing spawns instead.
func unixMethods() []*py.Method {
	return []*py.Method{
		py.MustNewMethod("pipe", func(self py.Object, args py.Tuple) (py.Object, error) {
			r, w, err := os.Pipe()
			if err != nil {
				return nil, py.OSErrorFrom(err, "")
			}
			return py.Tuple{py.Int(py.OwnFD(r)), py.Int(py.OwnFD(w))}, nil
		}, 0, "Create a pipe; return (read_fd, write_fd)."),
		py.MustNewMethod("read", func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "read() takes exactly 2 arguments")
			}
			fd, err := py.MakeGoInt64(args[0])
			if err != nil {
				return nil, err
			}
			n, err := py.IndexInt(args[1])
			if err != nil {
				return nil, err
			}
			if n < 0 {
				return nil, py.ExceptionNewf(py.ValueError, "negative length")
			}
			b := make([]byte, n)
			m, err := syscall.Read(syscall.Handle(fd), b)
			// A pipe whose writer has closed reports ERROR_BROKEN_PIPE; that
			// is end of file, which os.read answers with b"".
			if err == syscall.ERROR_BROKEN_PIPE {
				return py.Bytes{}, nil
			}
			if err != nil {
				return nil, py.OSErrorFrom(err, "")
			}
			return py.Bytes(b[:m]), nil
		}, 0, "Read at most n bytes from fd."),
		py.MustNewMethod("write", func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "write() takes exactly 2 arguments")
			}
			fd, err := py.MakeGoInt64(args[0])
			if err != nil {
				return nil, err
			}
			data, err := py.BytesFromObject(args[1])
			if err != nil {
				return nil, err
			}
			m, err := syscall.Write(syscall.Handle(fd), data)
			if err != nil {
				return nil, py.OSErrorFrom(err, "")
			}
			return py.Int(m), nil
		}, 0, "Write bytes to fd; return the number written."),
	}
}

func procConsts() map[string]int {
	return map[string]int{"O_RDONLY": os.O_RDONLY, "O_WRONLY": os.O_WRONLY, "O_RDWR": os.O_RDWR,
		"O_CREAT": os.O_CREATE, "O_EXCL": os.O_EXCL, "O_TRUNC": os.O_TRUNC, "O_APPEND": os.O_APPEND,
		"O_BINARY": 0x8000, "O_TEXT": 0x4000, "O_NOINHERIT": 0x80}
}
