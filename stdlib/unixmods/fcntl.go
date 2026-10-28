// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || darwin

package unixmods

import (
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/vishnukv64/gpython/py"
)

type namedConst struct {
	name string
	val  int64
}

// fcntlConsts are the constants every supported Unix has under the same name;
// the per-OS files add the rest.
var fcntlConsts = []namedConst{
	{"FASYNC", unix.O_ASYNC}, {"FD_CLOEXEC", unix.FD_CLOEXEC},
	{"F_DUPFD", unix.F_DUPFD}, {"F_DUPFD_CLOEXEC", unix.F_DUPFD_CLOEXEC},
	{"F_GETFD", unix.F_GETFD}, {"F_GETFL", unix.F_GETFL}, {"F_GETLK", unix.F_GETLK},
	{"F_GETOWN", unix.F_GETOWN}, {"F_RDLCK", unix.F_RDLCK}, {"F_SETFD", unix.F_SETFD},
	{"F_SETFL", unix.F_SETFL}, {"F_SETLK", unix.F_SETLK}, {"F_SETLKW", unix.F_SETLKW},
	{"F_SETOWN", unix.F_SETOWN}, {"F_UNLCK", unix.F_UNLCK}, {"F_WRLCK", unix.F_WRLCK},
	{"LOCK_EX", unix.LOCK_EX}, {"LOCK_NB", unix.LOCK_NB}, {"LOCK_SH", unix.LOCK_SH},
	{"LOCK_UN", unix.LOCK_UN},
}

func intArg(o py.Object) int { n, _ := o.(py.Int); return int(n) }

// bufCall runs a syscall that takes a pointer to a byte buffer and returns the
// result and the buffer afterwards.  An empty buffer still gets one byte of
// backing so the pointer is valid.
func bufCall(trap uintptr, fd int, req uintptr, b []byte) (int, []byte, error) {
	buf := make([]byte, len(b)+1)
	copy(buf, b)
	r, err := rawSyscall(trap, uintptr(fd), req, uintptr(unsafe.Pointer(&buf[0])))
	if err != nil {
		return 0, nil, err
	}
	return int(r), buf[:len(b)], nil
}

const fcntlSrc = `
def _fd(fd):
    if isinstance(fd, int):
        if fd < 0:
            raise ValueError("file descriptor cannot be a negative integer (%d)" % fd)
        return fd
    try:
        f = fd.fileno
    except AttributeError:
        raise TypeError("argument must be an int, or have a fileno() method.") from None
    fd = f()
    if not isinstance(fd, int):
        raise TypeError("fileno() returned a non-integer")
    return fd
def _num(v, what):
    import operator
    try:
        return operator.index(v)
    except TypeError:
        raise TypeError("'%s' object cannot be interpreted as an integer" % type(v).__name__) from None
def fcntl(fd, cmd, arg=0, /):
    "Perform the operation cmd on file descriptor fd.\n\nThe argument may be an int, or a bytes-like object (in which case the result is bytes)."
    fd = _fd(fd); cmd = _num(cmd, 'cmd')
    if isinstance(arg, (bytes, bytearray, memoryview)):
        b = bytes(arg)
        if len(b) > 1024:
            raise ValueError("fcntl argument 3 is too long")
        return _fcntl_buf(fd, cmd, b)
    if isinstance(arg, str):
        raise TypeError("fcntl() argument 3 must be an int or a bytes-like object, not str")
    return _fcntl_int(fd, cmd, _num(arg, 'arg'))
def ioctl(fd, request, arg=0, mutate_flag=True, /):
    "Perform the operation request on file descriptor fd.\n\nint arg returns the ioctl result; a bytes-like arg is passed by pointer (a bytearray is updated in place unless mutate_flag is false)."
    fd = _fd(fd); request = _num(request, 'request')
    if isinstance(arg, bytearray):
        if len(arg) > 1024:
            raise ValueError("ioctl buffer too long")
        r, out = _ioctl_buf(fd, request, bytes(arg))
        if mutate_flag:
            arg[:] = out
            return r
        return out
    if isinstance(arg, (bytes, memoryview)):
        b = bytes(arg)
        if len(b) > 1024:
            raise ValueError("ioctl string arg too long")
        return _ioctl_buf(fd, request, b)[1]
    if isinstance(arg, str):
        raise TypeError("ioctl() argument 3 must be an int or a bytes-like object, not str")
    return _ioctl_int(fd, request, _num(arg, 'arg'))
def flock(fd, operation, /):
    "Apply, or remove, an advisory lock on the open file identified by fd."
    _flock(_fd(fd), _num(operation, 'operation'))
def lockf(fd, cmd, len=0, start=0, whence=0, /):
    "A wrapper around the fcntl() locking calls."
    fd = _fd(fd); cmd = _num(cmd, 'cmd')
    _lockf(fd, cmd, _num(len, 'len'), _num(start, 'start'), _num(whence, 'whence'))
`

func init() {
	g := py.NewStringDict()
	for _, c := range append(append([]namedConst{}, fcntlConsts...), osFcntlConsts...) {
		g.Set(c.name, py.Int(c.val))
	}
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "fcntl", Doc: "This module performs file control and I/O control on file descriptors.  It is\nan interface to the fcntl() and ioctl() Unix routines.  File descriptors can\nbe obtained with the fileno() method of a file or socket object."},
		Globals: g,
		Methods: []*py.Method{
			py.MustNewMethod("_fcntl_int", func(self py.Object, args py.Tuple) (py.Object, error) {
				r, err := unix.FcntlInt(uintptr(intArg(args[0])), intArg(args[1]), intArg(args[2]))
				if err != nil {
					return nil, py.OSErrorFrom(err, "")
				}
				return py.Int(r), nil
			}, 0, ""),
			py.MustNewMethod("_fcntl_buf", func(self py.Object, args py.Tuple) (py.Object, error) {
				_, out, err := bufCall(sysFcntl, intArg(args[0]), uintptr(intArg(args[1])), []byte(args[2].(py.Bytes)))
				if err != nil {
					return nil, py.OSErrorFrom(err, "")
				}
				return py.Bytes(out), nil
			}, 0, ""),
			py.MustNewMethod("_ioctl_int", func(self py.Object, args py.Tuple) (py.Object, error) {
				r, e := rawSyscall(sysIoctl, uintptr(intArg(args[0])), uintptr(uint32(intArg(args[1]))), uintptr(intArg(args[2])))
				if e != nil {
					return nil, py.OSErrorFrom(e, "")
				}
				return py.Int(r), nil
			}, 0, ""),
			py.MustNewMethod("_ioctl_buf", func(self py.Object, args py.Tuple) (py.Object, error) {
				r, out, err := bufCall(sysIoctl, intArg(args[0]), uintptr(uint32(intArg(args[1]))), []byte(args[2].(py.Bytes)))
				if err != nil {
					return nil, py.OSErrorFrom(err, "")
				}
				return py.Tuple{py.Int(r), py.Bytes(out)}, nil
			}, 0, ""),
			py.MustNewMethod("_flock", func(self py.Object, args py.Tuple) (py.Object, error) {
				op := intArg(args[1])
				if err := unix.Flock(intArg(args[0]), op); err != nil {
					return nil, py.OSErrorFrom(err, "")
				}
				return py.None, nil
			}, 0, ""),
			py.MustNewMethod("_lockf", func(self py.Object, args py.Tuple) (py.Object, error) {
				cmd := intArg(args[1])
				fl := unix.Flock_t{Whence: int16(intArg(args[4])), Start: int64(intArg(args[3])), Len: int64(intArg(args[2]))}
				switch {
				case cmd&unix.LOCK_UN != 0:
					fl.Type = unix.F_UNLCK
				case cmd&unix.LOCK_SH != 0:
					fl.Type = unix.F_RDLCK
				case cmd&unix.LOCK_EX != 0:
					fl.Type = unix.F_WRLCK
				default:
					return nil, py.ExceptionNewf(py.ValueError, "unrecognized lockf argument")
				}
				op := unix.F_SETLKW
				if cmd&unix.LOCK_NB != 0 {
					op = unix.F_SETLK
				}
				if err := unix.FcntlFlock(uintptr(intArg(args[0])), op, &fl); err != nil {
					return nil, py.OSErrorFrom(err, "")
				}
				return py.None, nil
			}, 0, ""),
		},
		CodeSrc: fcntlSrc,
	})
}
