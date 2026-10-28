// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || darwin

package unixmods

import (
	"golang.org/x/sys/unix"

	"github.com/vishnukv64/gpython/py"
)

const termiosSrc = `
class error(Exception):
    pass
error.__module__ = 'termios'
def _fd(fd):
    if isinstance(fd, int):
        if fd < 0:
            raise ValueError("file descriptor cannot be a negative integer (%d)" % fd)
        return fd
    try:
        f = fd.fileno
    except AttributeError:
        raise TypeError("argument must be an int, or have a fileno() method.") from None
    return f()
def _call(f, *a):
    try:
        return f(*a)
    except OSError as e:
        raise error(e.errno, e.strerror) from None
def tcgetattr(fd, /):
    "Get the tty attributes for file descriptor fd.\n\nReturns a list [iflag, oflag, cflag, lflag, ispeed, ospeed, cc]."
    iflag, oflag, cflag, lflag, ispeed, ospeed, cc = _call(_tcget, _fd(fd))
    cc = [bytes([c]) for c in cc]
    if not lflag & ICANON:
        for i in (VMIN, VTIME):
            cc[i] = cc[i][0]
    return [iflag, oflag, cflag, lflag, ispeed, ospeed, cc]
def tcsetattr(fd, when, attributes, /):
    "Set the tty attributes for file descriptor fd."
    fd = _fd(fd)
    if not isinstance(attributes, list) or len(attributes) != 7:
        raise TypeError("tcsetattr, arg 3: must be 7 element list")
    iflag, oflag, cflag, lflag, ispeed, ospeed, cc = attributes
    if not isinstance(cc, list) or len(cc) != NCCS:
        raise TypeError("tcsetattr: attributes[6] must be %d element list" % NCCS)
    raw = bytearray()
    for c in cc:
        if isinstance(c, bytes) and len(c) == 1:
            raw.append(c[0])
        elif isinstance(c, int):
            raw.append(c & 0xff)
        else:
            raise TypeError("tcsetattr: elements of attributes must be characters or integers")
    _call(_tcset, fd, when, iflag, oflag, cflag, lflag, ispeed, ospeed, bytes(raw))
def tcsendbreak(fd, duration, /):
    "Send a break on file descriptor fd."
    _call(_tcbreak, _fd(fd), duration)
def tcdrain(fd, /):
    "Wait until all output written to file descriptor fd has been transmitted."
    _call(_tcdrain, _fd(fd))
def tcflush(fd, queue, /):
    "Discard queued data on file descriptor fd."
    _call(_tcflush, _fd(fd), queue)
def tcflow(fd, action, /):
    "Suspend or resume input or output on file descriptor fd."
    _call(_tcflow, _fd(fd), action)
def tcgetwinsize(fd, /):
    "Get the tty winsize for file descriptor fd.\n\nReturns a tuple (ws_row, ws_col)."
    return _call(_tcgetwinsize, _fd(fd))
def tcsetwinsize(fd, winsize, /):
    "Set the tty winsize for file descriptor fd.\n\nThe winsize to be set is taken from the winsize argument, which is a two-item tuple (ws_row, ws_col)."
    try:
        row, col = winsize
    except (TypeError, ValueError):
        raise TypeError("tcsetwinsize, arg 2 must be a tuple or list of 2 integers") from None
    _call(_tcsetwinsize, _fd(fd), row, col)
`

func errTuple(err error) error { return py.OSErrorFrom(err, "") }

func init() {
	g := py.NewStringDict()
	for _, c := range append(append([]namedConst{}, termiosConsts...), osTermiosConsts...) {
		g.Set(c.name, py.Int(c.val))
	}
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "termios", Doc: "This module provides an interface to the Posix calls for tty I/O control.\nFor a complete description of these calls, see the Posix or Unix manual\npages."},
		Globals: g,
		Methods: []*py.Method{
			py.MustNewMethod("_tcget", func(self py.Object, args py.Tuple) (py.Object, error) {
				s, err := tcGet(intArg(args[0]))
				if err != nil {
					return nil, errTuple(err)
				}
				return py.Tuple{py.Int(s.iflag), py.Int(s.oflag), py.Int(s.cflag), py.Int(s.lflag),
					py.Int(s.ispeed), py.Int(s.ospeed), py.Bytes(s.cc[:])}, nil
			}, 0, ""),
			py.MustNewMethod("_tcset", func(self py.Object, args py.Tuple) (py.Object, error) {
				when := intArg(args[1])
				if when < 0 || when > 2 {
					return nil, py.ExceptionNewf(py.OSError, "[Errno 22] Invalid argument")
				}
				s := &termiosState{iflag: uint(intArg(args[2])), oflag: uint(intArg(args[3])), cflag: uint(intArg(args[4])),
					lflag: uint(intArg(args[5])), ispeed: uint(intArg(args[6])), ospeed: uint(intArg(args[7]))}
				copy(s.cc[:], args[8].(py.Bytes))
				if err := tcSet(intArg(args[0]), when, s); err != nil {
					return nil, errTuple(err)
				}
				return py.None, nil
			}, 0, ""),
			intCall("_tcbreak", tcBreak), intCall("_tcflush", tcFlush), intCall("_tcflow", tcFlow),
			py.MustNewMethod("_tcdrain", func(self py.Object, args py.Tuple) (py.Object, error) {
				if err := tcDrain(intArg(args[0])); err != nil {
					return nil, errTuple(err)
				}
				return py.None, nil
			}, 0, ""),
			py.MustNewMethod("_tcgetwinsize", func(self py.Object, args py.Tuple) (py.Object, error) {
				w, err := unix.IoctlGetWinsize(intArg(args[0]), unix.TIOCGWINSZ)
				if err != nil {
					return nil, errTuple(err)
				}
				return py.Tuple{py.Int(w.Row), py.Int(w.Col)}, nil
			}, 0, ""),
			py.MustNewMethod("_tcsetwinsize", func(self py.Object, args py.Tuple) (py.Object, error) {
				w := &unix.Winsize{Row: uint16(intArg(args[1])), Col: uint16(intArg(args[2]))}
				if err := unix.IoctlSetWinsize(intArg(args[0]), unix.TIOCSWINSZ, w); err != nil {
					return nil, errTuple(err)
				}
				return py.None, nil
			}, 0, ""),
		},
		CodeSrc: termiosSrc,
	})
}

func intCall(name string, f func(fd, n int) error) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		if err := f(intArg(args[0]), intArg(args[1])); err != nil {
			return nil, errTuple(err)
		}
		return py.None, nil
	}, 0, "")
}
