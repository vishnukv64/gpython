// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package unixmods holds the Unix-only extension modules of the standard
// library that CPython builds from C: pwd, grp, fcntl, termios, resource and
// syslog.  None of them exists on Windows, as in CPython, so on Windows (and on
// Unixes the CI does not target) the package is empty and importing one of them
// raises ModuleNotFoundError.
//
// Each module is a thin Go layer of raw system calls plus a Python-source body
// (CodeSrc) that does the argument checking, error text and struct sequences,
// which is much shorter to keep identical to CPython than the same in Go.
package unixmods
