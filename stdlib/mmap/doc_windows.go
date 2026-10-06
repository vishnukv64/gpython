// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mmap is empty on Windows: the implementation is built on
// syscall.Mmap, which Windows does not have.  The package still has to exist,
// because stdlib imports it on every platform.
package mmap
