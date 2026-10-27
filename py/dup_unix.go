//go:build !windows && !plan9 && !js && !wasip1

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import "syscall"

// dupFD duplicates a descriptor with close-on-exec set.
func dupFD(fd int) (int, error) {
	n, err := syscall.Dup(fd)
	if err == nil {
		syscall.CloseOnExec(n)
	}
	return n, err
}
