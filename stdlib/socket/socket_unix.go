// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package socket

import (
	"net"
	"os"
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// platformGlobals are the module entries only some systems define; CPython has
// SO_REUSEPORT on darwin and linux but not on windows.
var platformGlobals = []py.DictEntry{{Key: "SO_REUSEPORT", Value: py.Int(SO_REUSEPORT)}}

func setReuseAddr(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}

// Non-blocking primitives.  Go's net sockets are non-blocking underneath, so a
// raw syscall inside RawConn.Read/Write returns EAGAIN instead of parking the
// goroutine - which is what a selector-driven event loop needs.

func rawConn(c interface{}) (syscall.RawConn, error) {
	sc, ok := c.(syscall.Conn)
	if !ok {
		return nil, syscall.EOPNOTSUPP
	}
	return sc.SyscallConn()
}

func rawRecv(c net.Conn, buf []byte) (n int, err error) {
	rc, err := rawConn(c)
	if err != nil {
		return 0, err
	}
	if cerr := rc.Read(func(fd uintptr) bool {
		n, err = syscall.Read(int(fd), buf)
		return true
	}); cerr != nil {
		return 0, cerr
	}
	return n, err
}

func rawSend(c net.Conn, b []byte) (n int, err error) {
	rc, err := rawConn(c)
	if err != nil {
		return 0, err
	}
	if cerr := rc.Write(func(fd uintptr) bool {
		n, err = syscall.Write(int(fd), b)
		return true
	}); cerr != nil {
		return 0, cerr
	}
	return n, err
}

func rawAccept(l net.Listener) (net.Conn, error) {
	rc, err := rawConn(l)
	if err != nil {
		return nil, err
	}
	nfd := -1
	// A listener's RawConn supports Control only; its fd is non-blocking, so
	// accept returns EAGAIN rather than waiting.
	if cerr := rc.Control(func(fd uintptr) {
		nfd, _, err = syscall.Accept(int(fd))
	}); cerr != nil {
		return nil, cerr
	}
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(nfd), "accepted")
	defer f.Close() // FileConn dups the descriptor
	return net.FileConn(f)
}

// A socket with no connection yet still needs a descriptor to answer fileno()
// (asyncio asks before connect); a real, unused socket stands in for it.
func placeholderFD(family, typ int) (int, error) { return syscall.Socket(family, typ, 0) }
func closeFD(fd int)                             { _ = syscall.Close(fd) }
