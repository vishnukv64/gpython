// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package os

import "syscall"

// fsyncFD flushes a descriptor without wrapping it in an *os.File, whose
// finalizer would close a descriptor the caller still owns.
func fsyncFD(fd int) error { return syscall.Fsync(fd) }

// setUmask sets the process umask and returns the previous one.
func setUmask(mask int) int { return syscall.Umask(mask) }
