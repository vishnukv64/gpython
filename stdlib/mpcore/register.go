// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mpcore

import "github.com/vishnukv64/gpython/py"

func register(globals py.StringDict) {
	if winLike {
		for _, n := range []string{"closesocket", "recv", "send"} {
			globals.Set(n, sockStub(n).Value)
		}
	}
	globals.Set("flags", py.NewStringDictFrom(
		py.DictEntry{Key: "HAVE_SEM_OPEN", Value: py.Int(1)},
		py.DictEntry{Key: "HAVE_SEM_TIMEDWAIT", Value: py.Int(1)},
	))
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "_multiprocessing", Doc: "Support for POSIX-style semaphores and subprocess spawning."},
		Globals: globals,
	})
}

// sockStub: Connection objects over a detached socket handle (AF_INET
// listeners and clients) need winsock handles, which gpython's Go sockets are
// not.  Named-pipe connections (Pipe(), Process, Queue, Pool, AF_PIPE managers)
// do not use these.
func sockStub(name string) py.DictEntry {
	return py.DictEntry{Key: name, Value: py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		return nil, py.ExceptionNewf(py.NotImplementedError, "_multiprocessing.%s: socket-handle Connections are not supported by gpython on Windows; use AF_PIPE", name)
	}, 0, name)}
}
