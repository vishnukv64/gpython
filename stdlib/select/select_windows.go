//go:build windows

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package selectmod provides select.select on Windows over winsock's
// select(), which like CPython's only accepts sockets (fileno() returns the
// SOCKET handle).  There is no poll/epoll, as in CPython on Windows.
package selectmod

import (
	"syscall"
	"time"
	"unsafe"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Wait for I/O completion on multiple sockets.

select(rlist, wlist, xlist[, timeout]) -> (rlist, wlist, xlist)
`

// fdSetSize is winsock's default FD_SETSIZE; CPython uses the same limit.
const fdSetSize = 64

type fdSet struct {
	count uint32
	array [fdSetSize]uintptr
}

type timeval struct{ sec, usec int32 }

var procSelect = syscall.NewLazyDLL("ws2_32.dll").NewProc("select")

func (s *fdSet) has(h uintptr) bool {
	for i := uint32(0); i < s.count; i++ {
		if s.array[i] == h {
			return true
		}
	}
	return false
}

func handleOf(obj py.Object) (uintptr, error) {
	v := obj
	if _, ok := obj.(py.Int); !ok {
		fn, err := py.GetAttrString(obj, "fileno")
		if err != nil {
			return 0, py.ExceptionNewf(py.TypeError, "argument must be an int, or have a fileno() method")
		}
		if v, err = py.Call(fn, py.Tuple{}, py.NewStringDict()); err != nil {
			return 0, err
		}
	}
	n, err := py.IndexInt(v)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, py.ExceptionNewf(py.ValueError, "file descriptor cannot be a negative integer (%d)", n)
	}
	return uintptr(n), nil
}

// build converts a Python sequence into an fd_set plus its handles.
func build(list py.Object) (*fdSet, []py.Object, []uintptr, error) {
	l, err := py.SequenceList(list)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(l.Items) > fdSetSize {
		return nil, nil, nil, py.ExceptionNewf(py.ValueError, "too many file descriptors in select()")
	}
	set := &fdSet{}
	hs := make([]uintptr, len(l.Items))
	for i, it := range l.Items {
		h, err := handleOf(it)
		if err != nil {
			return nil, nil, nil, err
		}
		hs[i] = h
		set.array[i] = h
	}
	set.count = uint32(len(l.Items))
	return set, l.Items, hs, nil
}

func ready(set *fdSet, objs []py.Object, hs []uintptr) py.Object {
	out := make([]py.Object, 0, len(objs))
	for i, o := range objs {
		if set.has(hs[i]) {
			out = append(out, o)
		}
	}
	return py.NewListFromItems(out)
}

func selectSelect(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var rl, wl, xl py.Object
	var timeout py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "OOO|O:select",
		[]string{"rlist", "wlist", "xlist", "timeout"}, &rl, &wl, &xl, &timeout); err != nil {
		return nil, err
	}
	var tv *timeval
	var secs float64
	if timeout != py.None {
		switch x := timeout.(type) {
		case py.Int:
			secs = float64(x)
		case py.Float:
			secs = float64(x)
		default:
			return nil, py.ExceptionNewf(py.TypeError, "timeout must be a float or None")
		}
		if secs < 0 {
			return nil, py.ExceptionNewf(py.ValueError, "timeout must be non-negative")
		}
		tv = &timeval{int32(secs), int32((secs - float64(int32(secs))) * 1e6)}
	}
	rs, ro, rh, err := build(rl)
	if err != nil {
		return nil, err
	}
	ws, wo, wh, err := build(wl)
	if err != nil {
		return nil, err
	}
	xs, xo, xh, err := build(xl)
	if err != nil {
		return nil, err
	}
	if rs.count+ws.count+xs.count == 0 {
		// winsock rejects empty sets (WSAEINVAL); CPython's callers expect a sleep.
		if tv == nil {
			select {} // ponytail: blocks forever like CPython would on a valid-but-idle select
		}
		time.Sleep(time.Duration(secs * float64(time.Second)))
		return py.Tuple{py.NewListFromItems(nil), py.NewListFromItems(nil), py.NewListFromItems(nil)}, nil
	}
	var tvp uintptr
	if tv != nil {
		tvp = uintptr(unsafe.Pointer(tv))
	}
	r, _, e := procSelect.Call(0, uintptr(unsafe.Pointer(rs)), uintptr(unsafe.Pointer(ws)),
		uintptr(unsafe.Pointer(xs)), tvp)
	if int32(r) == -1 {
		return nil, py.ExceptionNewf(py.OSError, "[WinError %d] %s", uintptr(e.(syscall.Errno)), e)
	}
	return py.Tuple{ready(rs, ro, rh), ready(ws, wo, wh), ready(xs, xo, xh)}, nil
}

func init() {
	globals := py.NewStringDict()
	globals.Set("select", py.MustNewMethod("select", selectSelect, 0,
		"select(rlist, wlist, xlist[, timeout]) -> (rlist, wlist, xlist)"))
	globals.Set("error", py.OSError)
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "select", Doc: module_doc},
		Globals: globals,
	})
}
