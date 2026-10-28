//go:build !windows && !plan9 && !js && !wasip1

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"os"
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// File descriptors here are raw OS descriptors, as in CPython, so a number
// can be handed to a child process and mean the same thing there.
func unixMethods() []*py.Method {
	fdErr := func(err error) error { return py.OSErrorFrom(err, "") }
	return []*py.Method{
		py.MustNewMethod("openpty", func(self py.Object, args py.Tuple) (py.Object, error) {
			m, s, err := openptyFds()
			if err != nil {
				return nil, fdErr(err)
			}
			return py.Tuple{py.Int(m), py.Int(s)}, nil
		}, 0, "Open a pseudo-terminal; return (master_fd, slave_fd)."),
		py.MustNewMethod("pipe", func(self py.Object, args py.Tuple) (py.Object, error) {
			var p [2]int
			if err := syscall.Pipe(p[:]); err != nil {
				return nil, fdErr(err)
			}
			syscall.CloseOnExec(p[0])
			syscall.CloseOnExec(p[1])
			return py.Tuple{py.Int(p[0]), py.Int(p[1])}, nil
		}, 0, "Create a pipe; return (read_fd, write_fd)."),
		py.MustNewMethod("read", func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "read() takes exactly 2 arguments")
			}
			fd, err := py.IndexInt(args[0])
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
			for {
				m, err := syscall.Read(fd, b)
				if err == syscall.EINTR {
					continue
				}
				if err != nil {
					return nil, fdErr(err)
				}
				return py.Bytes(b[:m]), nil
			}
		}, 0, "Read at most n bytes from fd."),
		py.MustNewMethod("write", func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "write() takes exactly 2 arguments")
			}
			fd, err := py.IndexInt(args[0])
			if err != nil {
				return nil, err
			}
			var data []byte
			switch v := args[1].(type) {
			case py.Bytes:
				data = v
			default:
				b, err := py.BytesFromObject(args[1])
				if err != nil {
					return nil, err
				}
				data = b
			}
			for {
				m, err := syscall.Write(fd, data)
				if err == syscall.EINTR {
					continue
				}
				if err != nil {
					return nil, fdErr(err)
				}
				return py.Int(m), nil
			}
		}, 0, "Write bytes to fd; return the number written."),
		py.MustNewMethod("dup", func(self py.Object, args py.Tuple) (py.Object, error) {
			fd, err := oneInt(args, "dup")
			if err != nil {
				return nil, err
			}
			n, err := syscall.Dup(fd)
			if err != nil {
				return nil, fdErr(err)
			}
			syscall.CloseOnExec(n)
			return py.Int(n), nil
		}, 0, "Return a duplicate of a file descriptor."),
		py.MustNewMethod("ftruncate", func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "ftruncate() takes exactly 2 arguments")
			}
			fd, err := py.IndexInt(args[0])
			if err != nil {
				return nil, err
			}
			n, err := py.IndexInt(args[1])
			if err != nil {
				return nil, err
			}
			if err := syscall.Ftruncate(fd, int64(n)); err != nil {
				return nil, fdErr(err)
			}
			return py.None, nil
		}, 0, "Truncate a file, specified by file descriptor, to a specific length."),
		py.MustNewMethod("waitpid", func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "waitpid() takes exactly 2 arguments")
			}
			pid, err := py.IndexInt(args[0])
			if err != nil {
				return nil, err
			}
			opts, err := py.IndexInt(args[1])
			if err != nil {
				return nil, err
			}
			var ws syscall.WaitStatus
			for {
				p, err := syscall.Wait4(pid, &ws, opts, nil)
				if err == syscall.EINTR {
					continue
				}
				if err != nil {
					return nil, fdErr(err)
				}
				return py.Tuple{py.Int(p), py.Int(int(ws))}, nil
			}
		}, 0, "Wait for completion of a child process; return (pid, status)."),
		py.MustNewMethod("waitstatus_to_exitcode", func(self py.Object, args py.Tuple) (py.Object, error) {
			n, err := oneInt(args, "waitstatus_to_exitcode")
			if err != nil {
				return nil, err
			}
			ws := syscall.WaitStatus(n)
			switch {
			case ws.Exited():
				return py.Int(ws.ExitStatus()), nil
			case ws.Signaled():
				return py.Int(-int(ws.Signal())), nil
			}
			return nil, py.ExceptionNewf(py.ValueError, "invalid wait status: %d", n)
		}, 0, "Convert a wait status to an exit code."),
	}
}

// procConsts are the wait flags.
func procConsts() map[string]int {
	return map[string]int{"WNOHANG": syscall.WNOHANG, "O_RDONLY": os.O_RDONLY, "O_WRONLY": os.O_WRONLY, "O_RDWR": os.O_RDWR,
		"O_CREAT": os.O_CREATE, "O_EXCL": os.O_EXCL, "O_TRUNC": os.O_TRUNC, "O_APPEND": os.O_APPEND,
		"O_ACCMODE": syscall.O_ACCMODE, "O_NONBLOCK": syscall.O_NONBLOCK, "O_NOCTTY": syscall.O_NOCTTY,
		"O_CLOEXEC": syscall.O_CLOEXEC, "O_SYNC": syscall.O_SYNC, "O_NDELAY": syscall.O_NONBLOCK,
		"O_DIRECTORY": syscall.O_DIRECTORY, "O_NOFOLLOW": syscall.O_NOFOLLOW}
}
