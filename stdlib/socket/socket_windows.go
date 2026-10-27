// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socket

import (
	"net"
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// windows has no SO_REUSEPORT, so the module does not define it, as in
// CPython.  -1 matches no option a caller can pass to setsockopt.
var SO_REUSEPORT = -1

var platformGlobals []py.DictEntry

// windows' SetsockoptInt takes a Handle, not an int.
func setReuseAddr(fd uintptr) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}

// Without a raw non-blocking path the operations fall back to Go's blocking
// ones (ponytail: asyncio over sockets is POSIX-only for now).
func rawRecv(c net.Conn, buf []byte) (int, error) { return c.Read(buf) }
func rawSend(c net.Conn, b []byte) (int, error)   { return c.Write(b) }
func rawAccept(l net.Listener) (net.Conn, error)  { return l.Accept() }

func placeholderFD(family, typ int) (int, error) { return -1, syscall.EOPNOTSUPP }
func closeFD(fd int)                             {}
