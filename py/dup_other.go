//go:build windows || plan9 || js || wasip1

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import "errors"

func dupFD(fd int) (int, error) {
	return -1, errors.New("open(fd, closefd=False) is not supported on this platform")
}
