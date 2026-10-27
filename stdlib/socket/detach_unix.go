//go:build !windows && !plan9 && !js && !wasip1

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socket

import (
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

// detach hands the socket's descriptor to the caller and closes the socket
// object, as socket.detach() does.  The Go net package owns the original
// descriptor, so the caller gets a blocking duplicate; multiprocessing's
// duplex Pipe() wraps it in a Connection and reads it with os.read.
func (s *sock) detach() (py.Object, error) {
	if s.closed {
		return py.Int(-1), nil
	}
	fdObj, err := s.fileno()
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Dup(int(fdObj.(py.Int)))
	if err != nil {
		return nil, osErr(err)
	}
	syscall.CloseOnExec(fd)
	if err := syscall.SetNonblock(fd, false); err != nil {
		syscall.Close(fd)
		return nil, osErr(err)
	}
	if _, err := s.close(); err != nil {
		return nil, err
	}
	return py.Int(fd), nil
}

func init() {
	SocketType.Dict.Set("detach", py.MustNewMethod("detach", func(self py.Object, args py.Tuple) (py.Object, error) {
		s, err := selfSock(self, "detach")
		if err != nil {
			return nil, err
		}
		return s.detach()
	}, 0, "Close the socket object without closing the underlying file descriptor; return the descriptor."))
}
