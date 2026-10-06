// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socket

// SO_REUSEPORT is 15 on linux (asm-generic/socket.h, and CPython's value
// there); Go's syscall package does not define it.
var SO_REUSEPORT = 0xf
