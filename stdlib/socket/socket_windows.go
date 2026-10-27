// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socket

import (
	"io"
	"net"
	"syscall"
	"unsafe"

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

// Non-blocking primitives.  Go's Windows sockets complete I/O through IOCP and
// have no EAGAIN path, so a zero-timeout winsock select() decides first whether
// a read or accept would block; when it would not, the ordinary call returns at
// once.  Without this a selector loop's recv on its self-pipe parked forever.

var procSelect = syscall.NewLazyDLL("ws2_32.dll").NewProc("select")

type fdSet1 struct {
	count  uint32
	handle uintptr
}

type timeval struct{ sec, usec int32 }

// readable reports whether a read (or accept) on c's socket would not block:
// data, a pending connection, EOF or an error are all "readable" to select.
func readable(c interface{}) (bool, error) {
	sc, ok := c.(syscall.Conn)
	if !ok {
		return false, syscall.EOPNOTSUPP
	}
	rc, err := sc.SyscallConn()
	if err != nil {
		return false, err
	}
	var n uintptr
	var callErr error
	if cerr := rc.Control(func(fd uintptr) {
		set := fdSet1{count: 1, handle: fd}
		tv := timeval{}
		r, _, e := procSelect.Call(0, uintptr(unsafe.Pointer(&set)), 0, 0, uintptr(unsafe.Pointer(&tv)))
		if int32(r) < 0 {
			callErr = e
			return
		}
		n = uintptr(set.count)
	}); cerr != nil {
		return false, cerr
	}
	return n > 0, callErr
}

func rawRecv(c net.Conn, buf []byte) (int, error) {
	ok, err := readable(c)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, syscall.EAGAIN
	}
	n, err := c.Read(buf)
	if err == io.EOF {
		return 0, nil
	}
	return n, err
}

// ponytail: a send blocks until the kernel takes the bytes; fine for the small
// writes a selector loop issues, add a writability select if large ones stall.
func rawSend(c net.Conn, b []byte) (int, error) { return c.Write(b) }

func rawAccept(l net.Listener) (net.Conn, error) {
	ok, err := readable(l)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, syscall.EAGAIN
	}
	return l.Accept()
}

// A socket with no connection yet still needs a handle to answer fileno()
// (asyncio asks before connect); a real, unused socket stands in for it.
func placeholderFD(family, typ int) (int, error) {
	h, err := syscall.Socket(family, typ, 0)
	if err != nil {
		return -1, err
	}
	return int(h), nil
}

func closeFD(fd int) { _ = syscall.Closesocket(syscall.Handle(fd)) }
