// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package errno

import "syscall"

// numberOf is the value Python reports for e: the host's own number.
func numberOf(e syscall.Errno) (int, bool) { return int(e), true }
