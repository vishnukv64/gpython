// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package py

import "syscall"

// sysErrno gives the OSError errno, winerror (-1: none) and message.
func sysErrno(en syscall.Errno) (errno, winerror int, strerror string) {
	return int(en), -1, en.Error()
}

// errnoClass is the PEP 3151 subclass for an errno that the fs.Err* checks in
// osErrorClass did not already settle.
func errnoClass(errno int) *Type {
	switch syscall.Errno(errno) {
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
