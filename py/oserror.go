// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"errors"
	"io/fs"
	"syscall"
)

// OSErrorFrom turns an error from Go's os or syscall packages into the
// exception CPython raises for the same failure: the OSError SUBCLASS chosen by
// errno, the message "[Errno 2] No such file or directory: '/x'", and the
// errno, strerror and filename attributes set.  filename may be empty; a
// two-path call (rename, link, symlink) passes its destination as filename2,
// which CPython shows as "'src' -> 'dst'".
//
// Callers that formatted err.Error() themselves raised a bare OSError (or worse)
// with Go's wording - "stat /x: no such file or directory" - which no
// "except FileNotFoundError" clause catches.
func OSErrorFrom(err error, filename string, filename2 ...string) error {
	var en syscall.Errno
	if !errors.As(err, &en) {
		return ExceptionNewf(OSError, "%s", err.Error())
	}
	strerror := en.Error()
	if strerror != "" && strerror[0] >= 'a' && strerror[0] <= 'z' {
		// Go's text is C's strerror with the first letter lowercased.
		strerror = string(strerror[0]-'a'+'A') + strerror[1:]
	}
	var e *Exception
	if filename == "" {
		e = ExceptionNewf(osErrorClass(err, en), "[Errno %d] %s", int(en), strerror)
	} else {
		quoted, rerr := ReprAsString(String(filename))
		if rerr != nil {
			return rerr
		}
		if len(filename2) > 0 {
			quoted2, rerr := ReprAsString(String(filename2[0]))
			if rerr != nil {
				return rerr
			}
			quoted += " -> " + quoted2
		}
		e = ExceptionNewf(osErrorClass(err, en), "[Errno %d] %s: %s", int(en), strerror, quoted)
		e.Dict.Set("filename", String(filename))
		if len(filename2) > 0 {
			e.Dict.Set("filename2", String(filename2[0]))
		}
	}
	SetErrno(e, int(en), strerror)
	return e
}

// osErrorClass is CPython's errno-to-subclass table (PEP 3151).  The fs
// sentinels are checked first because they also match Windows error codes,
// whose numbers are not the POSIX ones.
func osErrorClass(err error, en syscall.Errno) *Type {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return FileNotFoundError
	case errors.Is(err, fs.ErrExist):
		return FileExistsError
	case errors.Is(err, fs.ErrPermission):
		return PermissionError
	}
	switch en {
	case syscall.ENOTDIR:
		return NotADirectoryError
	case syscall.EISDIR:
		return IsADirectoryError
	case syscall.EINTR:
		return InterruptedError
	case syscall.ECHILD:
		return ChildProcessError
	case syscall.ESRCH:
		return ProcessLookupError
	case syscall.EAGAIN, syscall.EALREADY, syscall.EINPROGRESS:
		return BlockingIOError
	case syscall.EPIPE, syscall.ESHUTDOWN:
		return BrokenPipeError
	case syscall.ECONNABORTED:
		return ConnectionAbortedError
	case syscall.ECONNREFUSED:
		return ConnectionRefusedError
	case syscall.ECONNRESET:
		return ConnectionResetError
	case syscall.ETIMEDOUT:
		return TimeoutError
	}
	return OSError
}
