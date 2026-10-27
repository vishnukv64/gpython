// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"strings"
	"syscall"
)

// crtErrnos maps Go's syscall.E* values - invented ones on Windows - to the
// MSVC CRT errno numbers that CPython's errno module reports there.
var crtErrnos = map[syscall.Errno]int{
	syscall.EPERM: 1, syscall.ENOENT: 2, syscall.ESRCH: 3, syscall.EINTR: 4,
	syscall.EIO: 5, syscall.ENXIO: 6, syscall.E2BIG: 7, syscall.ENOEXEC: 8,
	syscall.EBADF: 9, syscall.ECHILD: 10, syscall.EAGAIN: 11, syscall.ENOMEM: 12,
	syscall.EACCES: 13, syscall.EFAULT: 14, syscall.EBUSY: 16, syscall.EEXIST: 17,
	syscall.EXDEV: 18, syscall.ENODEV: 19, syscall.ENOTDIR: 20, syscall.EISDIR: 21,
	syscall.EINVAL: 22, syscall.ENFILE: 23, syscall.EMFILE: 24, syscall.ENOTTY: 25,
	syscall.EFBIG: 27, syscall.ENOSPC: 28, syscall.ESPIPE: 29, syscall.EROFS: 30,
	syscall.EMLINK: 31, syscall.EPIPE: 32, syscall.EDOM: 33, syscall.ERANGE: 34,
	syscall.EDEADLK: 36, syscall.ENAMETOOLONG: 38, syscall.ENOLCK: 39,
	syscall.ENOSYS: 40, syscall.ENOTEMPTY: 41, syscall.EILSEQ: 42,
	syscall.EADDRINUSE: 100, syscall.EADDRNOTAVAIL: 101, syscall.EAFNOSUPPORT: 102,
	syscall.EALREADY: 103, syscall.EBADMSG: 104, syscall.ECANCELED: 105,
	syscall.ECONNABORTED: 106, syscall.ECONNREFUSED: 107, syscall.ECONNRESET: 108,
	syscall.EDESTADDRREQ: 109, syscall.EHOSTUNREACH: 110, syscall.EIDRM: 111,
	syscall.EINPROGRESS: 112, syscall.EISCONN: 113, syscall.ELOOP: 114,
	syscall.EMSGSIZE: 115, syscall.ENETDOWN: 116, syscall.ENETRESET: 117,
	syscall.ENETUNREACH: 118, syscall.ENOBUFS: 119, syscall.ENODATA: 120,
	syscall.ENOLINK: 121, syscall.ENOMSG: 122, syscall.ENOPROTOOPT: 123,
	syscall.ENOSR: 124, syscall.ENOSTR: 125, syscall.ENOTCONN: 126,
	syscall.ENOTRECOVERABLE: 127, syscall.ENOTSOCK: 128, syscall.ENOTSUP: 129,
	syscall.EOPNOTSUPP: 130, syscall.EOVERFLOW: 132, syscall.EOWNERDEAD: 133,
	syscall.EPROTO: 134, syscall.EPROTONOSUPPORT: 135, syscall.EPROTOTYPE: 136,
	syscall.ETIME: 137, syscall.ETIMEDOUT: 138, syscall.ETXTBSY: 139,
	syscall.EWOULDBLOCK: 140,
}

// CRTErrno is the CRT errno number for a syscall.E* constant, and false for
// the names Windows does not define.
func CRTErrno(en syscall.Errno) (int, bool) {
	n, ok := crtErrnos[en]
	return n, ok
}

// winErrnos is CPython's winerror_to_errno (PC/errmap.h): the CRT errno for a
// Win32 error code.
func winErrno(code syscall.Errno) int {
	switch {
	case code == 2, code == 3, code == 15, code == 18, code == 53, code == 67,
		code == 123, code == 161, code == 206:
		return 2 // ENOENT
	case code == 4:
		return 24 // EMFILE
	case code == 6, code == 130:
		return 9 // EBADF
	case code == 7, code == 8, code == 9, code == 14, code == 1450, code == 1451:
		return 12 // ENOMEM
	case code == 10:
		return 7 // E2BIG
	case code == 11, code == 193:
		return 8 // ENOEXEC
	case code == 17:
		return 18 // EXDEV
	case code == 80, code == 183:
		return 17 // EEXIST
	case code == 89:
		return 11 // EAGAIN
	case code == 109, code == 230, code == 231, code == 232, code == 233:
		return 32 // EPIPE
	case code == 112:
		return 28 // ENOSPC
	case code == 128, code == 129:
		return 10 // ECHILD
	case code == 145:
		return 41 // ENOTEMPTY
	case code == 267:
		return 20 // ENOTDIR
	case code == 5, code == 12, code == 16, code >= 19 && code <= 36,
		code == 65, code == 82, code == 83, code == 1314:
		return 13 // EACCES
	}
	return 22 // EINVAL
}

// sysErrno gives the OSError errno, winerror (-1 when none) and message for
// an error.  A real Win32 code becomes its CRT errno plus winerror, as in
// CPython; a Go-invented syscall.E* value becomes its CRT number.
func sysErrno(en syscall.Errno) (errno, winerror int, strerror string) {
	strerror = en.Error()
	if en < syscall.APPLICATION_ERROR {
		return winErrno(en), int(en), strings.TrimRight(strerror, " \r\n.")
	}
	if n, ok := crtErrnos[en]; ok {
		return n, -1, strerror
	}
	return int(en), -1, strerror
}

// errnoClass is the PEP 3151 subclass for a CRT errno that the fs.Err*
// checks in osErrorClass did not already settle.
func errnoClass(errno int) *Type {
	switch errno {
	case 20:
		return NotADirectoryError
	case 21:
		return IsADirectoryError
	case 4:
		return InterruptedError
	case 10:
		return ChildProcessError
	case 3:
		return ProcessLookupError
	case 11, 103, 112, 140:
		return BlockingIOError
	case 32:
		return BrokenPipeError
	case 106:
		return ConnectionAbortedError
	case 107:
		return ConnectionRefusedError
	case 108:
		return ConnectionResetError
	case 138:
		return TimeoutError
	}
	return OSError
}
