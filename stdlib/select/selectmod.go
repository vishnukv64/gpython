//go:build darwin || linux

// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package selectmod provides the implementation of python's 'select' module.
//
// select.select and select.poll are implemented over the interpreter's own
// sockets.  A socket object exposes fileno(), which yields the underlying file
// descriptor, and the wait is the real syscall.select on those descriptors, so
// the readiness reported is what the kernel reports and a blocking select
// genuinely blocks.  Objects without a file descriptor cannot be waited on and
// are reported as not-ready immediately, which is what CPython does for an
// object whose fileno() is -1.
//
// It is built for darwin and linux, whose select(2) syscall.Select wraps; on
// Windows syscall has no Select and the package registers nothing.
package selectmod

import (
	"syscall"
	"time"
	"unsafe"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Wait for I/O completion on multiple streams.

select(rlist, wlist, xlist[, timeout]) -> (rlist, wlist, xlist)
poll() -> a polling object

This module provides access to the select() and poll() functions.
`

// Poll constants, from system headers.
const (
	POLLIN   = 0x0001
	POLLOUT  = 0x0004
	POLLERR  = 0x0008
	POLLHUP  = 0x0010
	POLLNVAL = 0x0020
)

// pollType is the Python-visible poll object type.
var pollType = py.NewTypeX("select.poll",
	"A polling object for multiple file descriptors: register/unregister/poll.", pollNew, nil)

// pollFd is one registered descriptor.
type pollFd struct {
	fd     int
	events int16
}

// pollObject holds the registered descriptors.
type pollObject struct {
	fds  map[int]*pollFd
	Dict py.StringDict
}

func (p *pollObject) Type() *py.Type         { return pollType }
func (p *pollObject) GetDict() py.StringDict { return p.Dict }

func pollNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return &pollObject{fds: map[int]*pollFd{}, Dict: py.NewStringDict()}, nil
}

// fdOf extracts the file descriptor of a python object: an int is itself, and
// anything with fileno() is that.
func fdOf(obj py.Object) (int, error) {
	if n, ok := obj.(py.Int); ok {
		return int(n), nil
	}
	fn, err := py.GetAttrString(obj, "fileno")
	if err != nil {
		return -1, py.ExceptionNewf(py.TypeError,
			"argument must be an int, or have a fileno() method")
	}
	res, err := py.Call(fn, py.Tuple{}, py.NewStringDict())
	if err != nil {
		return -1, err
	}
	n, err := py.IndexInt(res)
	if err != nil {
		return -1, err
	}
	return n, nil
}

// pySelect performs the underlying select(2) wait and returns the ready sets.
//
// File descriptors must be below 1024 (FD_SETSIZE).  Readiness is read back
// from the FdSets rather than from select's count, which darwin's
// syscall.Select does not return.
func pySelect(rlist, wlist, xlist []int, timeout float64) (readyR, readyW, readyX map[int]bool, err error) {
	readyR = map[int]bool{}
	readyW = map[int]bool{}
	readyX = map[int]bool{}
	const fdSetsize = 1024
	var rfds, wfds, xfds syscall.FdSet
	maxfd := -1
	for _, fd := range rlist {
		if fd < 0 || fd >= fdSetsize {
			return nil, nil, nil, py.ExceptionNewf(py.ValueError,
				"filedescriptor out of range in select()")
		}
		setFd(&rfds, fd)
		if fd > maxfd {
			maxfd = fd
		}
	}
	for _, fd := range wlist {
		if fd < 0 || fd >= fdSetsize {
			return nil, nil, nil, py.ExceptionNewf(py.ValueError,
				"filedescriptor out of range in select()")
		}
		setFd(&wfds, fd)
		if fd > maxfd {
			maxfd = fd
		}
	}
	for _, fd := range xlist {
		if fd < 0 || fd >= fdSetsize {
			return nil, nil, nil, py.ExceptionNewf(py.ValueError,
				"filedescriptor out of range in select()")
		}
		setFd(&xfds, fd)
		if fd > maxfd {
			maxfd = fd
		}
	}
	if maxfd < 0 {
		// Nothing to wait on: CPython sleeps for the timeout, or returns
		// immediately for a zero timeout.
		if timeout > 0 {
			time.Sleep(time.Duration(timeout * float64(time.Second)))
		}
		return readyR, readyW, readyX, nil
	}
	var tv *syscall.Timeval
	if timeout >= 0 {
		// NsecToTimeval, because Timeval's field types differ by platform:
		// a hand-built literal compiled on darwin and failed on linux.
		t := syscall.NsecToTimeval(int64(timeout * 1e9))
		tv = &t
	}
	if err := doSelect(maxfd+1, &rfds, &wfds, &xfds, tv); err != nil {
		if err == syscall.EINTR {
			return readyR, readyW, readyX, nil
		}
		return nil, nil, nil, py.ExceptionNewf(py.OSError, "%s", err)
	}
	for _, fd := range rlist {
		if fdIsSet(&rfds, fd) {
			readyR[fd] = true
		}
	}
	for _, fd := range wlist {
		if fdIsSet(&wfds, fd) {
			readyW[fd] = true
		}
	}
	for _, fd := range xlist {
		if fdIsSet(&xfds, fd) {
			readyX[fd] = true
		}
	}
	return readyR, readyW, readyX, nil
}

// nfdbits is the width of one FdSet word, which is the platform's choice:
// int32 on darwin and linux/386, int64 on linux/amd64.  Hardcoding 32 put fd 40
// in the wrong bit on amd64 linux.
const nfdbits = int(unsafe.Sizeof(syscall.FdSet{}.Bits[0])) * 8

// setFd and fdIsSet manipulate a syscall.FdSet; the shifted 1 takes the
// word's own type from the expression it is used in.
func setFd(set *syscall.FdSet, fd int) {
	set.Bits[fd/nfdbits] |= 1 << uint(fd%nfdbits)
}

func fdIsSet(set *syscall.FdSet, fd int) bool {
	return set.Bits[fd/nfdbits]&(1<<uint(fd%nfdbits)) != 0
}

func selectSelect(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var rlist, wlist, xlist py.Object
	var timeout py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "OOO|O:select",
		[]string{"rlist", "wlist", "xlist", "timeout"},
		&rlist, &wlist, &xlist, &timeout); err != nil {
		return nil, err
	}
	to := -1.0
	if timeout != py.None {
		f, err := floatFromObject(timeout)
		if err != nil {
			return nil, err
		}
		to = f
	}
	// Map each input object to its fd, keeping the objects for the result.
	rIn, err := objectsWithFd(rlist)
	if err != nil {
		return nil, err
	}
	wIn, err := objectsWithFd(wlist)
	if err != nil {
		return nil, err
	}
	xIn, err := objectsWithFd(xlist)
	if err != nil {
		return nil, err
	}
	readyR, readyW, readyX, err := pySelect(rIn.fds(), wIn.fds(), xIn.fds(), to)
	if err != nil {
		return nil, err
	}
	return py.Tuple{
		sliceOfReady(rIn, readyR),
		sliceOfReady(wIn, readyW),
		sliceOfReady(xIn, readyX),
	}, nil
}

// fdObjects pairs each requested object with its file descriptor.
type fdObjects struct {
	objs []py.Object
	fd   []int
}

func (f *fdObjects) fds() []int { return f.fd }

func objectsWithFd(list py.Object) (*fdObjects, error) {
	l, err := py.SequenceList(list)
	if err != nil {
		return nil, err
	}
	out := &fdObjects{}
	for _, it := range l.Items {
		fd, err := fdOf(it)
		if err != nil {
			return nil, err
		}
		out.objs = append(out.objs, it)
		out.fd = append(out.fd, fd)
	}
	return out, nil
}

func sliceOfReady(in *fdObjects, ready map[int]bool) py.Object {
	items := make([]py.Object, 0, len(in.objs))
	for i, obj := range in.objs {
		if ready[in.fd[i]] {
			items = append(items, obj)
		}
	}
	return py.NewListFromItems(items)
}

func floatFromObject(v py.Object) (float64, error) {
	switch x := v.(type) {
	case py.Int:
		return float64(x), nil
	case py.Float:
		return float64(x), nil
	}
	return 0, py.ExceptionNewf(py.TypeError, "a number is required")
}

func pollRegister(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p := self.(*pollObject)
	var fdObj py.Object
	var events py.Object = py.Int(POLLIN | POLLPRI)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:register",
		[]string{"fd", "eventmask"}, &fdObj, &events); err != nil {
		return nil, err
	}
	fd, err := fdOf(fdObj)
	if err != nil {
		return nil, err
	}
	ev, err := py.IndexInt(events)
	if err != nil {
		return nil, err
	}
	p.fds[fd] = &pollFd{fd: fd, events: int16(ev)}
	return py.None, nil
}

// POLLPRI is not defined as a named constant in CPython's select, but poll
// accepts the numeric mask; define it locally.
const POLLPRI = 0x0002

func pollUnregister(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*pollObject)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "unregister() takes exactly one argument")
	}
	fd, err := fdOf(args[0])
	if err != nil {
		return nil, err
	}
	if _, ok := p.fds[fd]; !ok {
		return nil, py.ExceptionNewf(py.KeyError, "fd %d not registered", fd)
	}
	delete(p.fds, fd)
	return py.None, nil
}

func pollModify(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p := self.(*pollObject)
	var fdObj, events py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO:modify",
		[]string{"fd", "eventmask"}, &fdObj, &events); err != nil {
		return nil, err
	}
	fd, err := fdOf(fdObj)
	if err != nil {
		return nil, err
	}
	if _, ok := p.fds[fd]; !ok {
		return nil, py.ExceptionNewf(py.KeyError, "fd %d not registered", fd)
	}
	ev, err := py.IndexInt(events)
	if err != nil {
		return nil, err
	}
	p.fds[fd].events = int16(ev)
	return py.None, nil
}

func pollPoll(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p := self.(*pollObject)
	var timeout py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:poll",
		[]string{"timeout"}, &timeout); err != nil {
		return nil, err
	}
	ms := -1
	if timeout != py.None {
		f, err := floatFromObject(timeout)
		if err != nil {
			return nil, err
		}
		ms = int(f)
	}
	// Split the registered fds by the events they want.
	var rlist, wlist, xlist []int
	for _, pf := range p.fds {
		if pf.events&(POLLIN|POLLPRI) != 0 {
			rlist = append(rlist, pf.fd)
		}
		if pf.events&POLLOUT != 0 {
			wlist = append(wlist, pf.fd)
		}
	}
	var to float64 = -1
	if ms >= 0 {
		to = float64(ms) / 1000.0
	}
	readyR, readyW, readyX, err := pySelect(rlist, wlist, xlist, to)
	if err != nil {
		return nil, err
	}
	var out []py.Object
	for _, pf := range p.fds {
		var revents int
		if readyR[pf.fd] {
			revents |= int(pf.events) & (POLLIN | POLLPRI)
		}
		if readyW[pf.fd] {
			revents |= POLLOUT
		}
		if readyX[pf.fd] {
			revents |= POLLERR
		}
		if revents != 0 {
			out = append(out, py.Tuple{py.Int(pf.fd), py.Int(revents)})
		}
	}
	return py.NewListFromItems(out), nil
}

func init() {
	for _, m := range []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"register", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return pollRegister(self, args, kw)
		}, "Register a file descriptor with the polling object."},
		{"unregister", pollUnregister, "Remove a file descriptor being tracked."},
		{"modify", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return pollModify(self, args, kw)
		}, "Change the events a registered file descriptor is polled for."},
		{"poll", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return pollPoll(self, args, kw)
		}, "Poll the registered file descriptors."},
	} {
		pollType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}

	globals := py.NewStringDict()
	globals.Set("select", py.MustNewMethod("select", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return selectSelect(self, args, kw)
	}, 0, "select(rlist, wlist, xlist[, timeout]) -> (rlist, wlist, xlist)"))
	globals.Set("poll", py.MustNewMethod("poll", func(self py.Object, args py.Tuple) (py.Object, error) {
		return pollNew(pollType, args, py.NewStringDict())
	}, 0, "poll() -> a polling object"))
	globals.Set("error", py.OSError)
	globals.Set("POLLIN", py.Int(POLLIN))
	globals.Set("POLLPRI", py.Int(POLLPRI))
	globals.Set("POLLOUT", py.Int(POLLOUT))
	globals.Set("POLLERR", py.Int(POLLERR))
	globals.Set("POLLHUP", py.Int(POLLHUP))
	globals.Set("POLLNVAL", py.Int(POLLNVAL))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "select",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
