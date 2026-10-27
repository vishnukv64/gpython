//go:build windows

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mpcore

import (
	"golang.org/x/sys/windows"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/thread"
	"github.com/vishnukv64/gpython/stdlib/winapi"
)

const (
	recursiveMutex = 0
	semValueMax    = 0x7FFFFFFF
	waitTimeout    = 0x102
)

// SemLock is _multiprocessing.SemLock over a Win32 semaphore.  The handle is
// per process; a child receives it through DuplicateHandle (see
// multiprocessing.synchronize.SemLock.__getstate__).  count and lastTid are
// per process, as in CPython.
type SemLock struct {
	h        windows.Handle
	kind     int
	maxvalue int
	count    int
	lastTid  uint64
}

var SemLockType = py.NewTypeX("_multiprocessing.SemLock", "Semaphore/Mutex type", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var okind, ovalue, omax, oname, unlink py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "OOOOO:SemLock", []string{"kind", "value", "maxvalue", "name", "unlink"}, &okind, &ovalue, &omax, &oname, &unlink); err != nil {
		return nil, err
	}
	kind, err := py.IndexInt(okind)
	if err != nil {
		return nil, err
	}
	value, err := py.IndexInt(ovalue)
	if err != nil {
		return nil, err
	}
	maxvalue, err := py.IndexInt(omax)
	if err != nil {
		return nil, err
	}
	if kind != recursiveMutex && kind != 1 {
		return nil, py.ExceptionNewf(py.ValueError, "unrecognized kind")
	}
	h, err := winapi.CreateSemaphore(value, maxvalue)
	if err != nil {
		return nil, py.WinOSError(int(err.(windows.Errno)), err.Error())
	}
	return &SemLock{h: h, kind: kind, maxvalue: maxvalue}, nil
}, nil)

func (s *SemLock) Type() *py.Type { return SemLockType }

func (s *SemLock) isMine() bool { return s.count > 0 && s.lastTid == thread.Ident() }

// acquire waits ms milliseconds (INFINITE = forever).
func (s *SemLock) acquire(ms uint32) (bool, error) {
	if s.kind == recursiveMutex && s.isMine() {
		s.count++
		return true, nil
	}
	r, err := windows.WaitForSingleObject(s.h, ms)
	if err != nil {
		return false, py.WinOSError(int(err.(windows.Errno)), err.Error())
	}
	if r == waitTimeout {
		return false, nil
	}
	s.count++
	s.lastTid = thread.Ident()
	return true, nil
}

func (s *SemLock) release() error {
	if s.kind == recursiveMutex {
		if !s.isMine() {
			return py.ExceptionNewf(py.AssertionError, "attempt to release recursive lock not owned by thread")
		}
		if s.count > 1 {
			s.count--
			return nil
		}
	}
	if _, err := winapi.ReleaseSemaphore(s.h); err != nil {
		if err == windows.ERROR_TOO_MANY_POSTS {
			return py.ExceptionNewf(py.ValueError, "semaphore or lock released too many times")
		}
		return py.WinOSError(int(err.(windows.Errno)), err.Error())
	}
	s.count--
	return nil
}

// value probes the count: a successful zero-wait takes one, ReleaseSemaphore
// returns the count before giving it back.
func (s *SemLock) value() int {
	if r, _ := windows.WaitForSingleObject(s.h, 0); r != windows.WAIT_OBJECT_0 {
		return 0
	}
	prev, _ := winapi.ReleaseSemaphore(s.h)
	return prev + 1
}

func (s *SemLock) M__enter__() (py.Object, error) {
	_, err := s.acquire(windows.INFINITE)
	return py.True, err
}

func (s *SemLock) M__exit__(a, b, c py.Object) (py.Object, error) {
	return py.None, s.release()
}

// timeoutMs converts acquire's (block, timeout) into a wait in milliseconds.
func timeoutMs(args py.Tuple, kwargs py.StringDict) (uint32, error) {
	block, tmo := py.Object(py.True), py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:acquire", []string{"block", "timeout"}, &block, &tmo); err != nil {
		return 0, err
	}
	if b, err := py.ObjectIsTrue(block); err != nil {
		return 0, err
	} else if !b {
		return 0, nil
	}
	if tmo == py.None {
		return windows.INFINITE, nil
	}
	f, err := py.MakeFloat(tmo)
	if err != nil {
		return 0, err
	}
	if t := float64(f.(py.Float)); t > 0 {
		if ms := t * 1000; ms < windows.INFINITE-1 {
			return uint32(ms + 0.5), nil
		}
		return windows.INFINITE - 1, nil
	}
	return 0, nil
}

func init() {
	m := func(name string, f func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error)) {
		SemLockType.Dict.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			return f(self.(*SemLock), args, kwargs)
		}, 0, name))
	}
	m("acquire", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		ms, err := timeoutMs(args, kwargs)
		if err != nil {
			return nil, err
		}
		ok, err := s.acquire(ms)
		return py.NewBool(ok), err
	})
	m("release", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return py.None, s.release()
	})
	m("__enter__", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) { return s.M__enter__() })
	m("__exit__", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return s.M__exit__(nil, nil, nil)
	})
	m("_count", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) { return py.Int(s.count), nil })
	m("_is_mine", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return py.NewBool(s.isMine()), nil
	})
	m("_get_value", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return py.Int(s.value()), nil
	})
	m("_is_zero", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return py.NewBool(s.value() == 0), nil
	})
	m("_after_fork", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		s.count = 0
		return py.None, nil
	})
	SemLockType.Dict.Set("_rebuild", py.MustNewMethod("_rebuild", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) != 4 {
			return nil, py.ExceptionNewf(py.TypeError, "_rebuild() takes 4 arguments")
		}
		h, err := py.IndexInt(args[0])
		if err != nil {
			return nil, err
		}
		kind, err := py.IndexInt(args[1])
		if err != nil {
			return nil, err
		}
		maxvalue, err := py.IndexInt(args[2])
		if err != nil {
			return nil, err
		}
		return &SemLock{h: windows.Handle(h), kind: kind, maxvalue: maxvalue}, nil
	}, py.METH_CLASS|py.METH_STATIC, "Rebuild a semaphore from a handle duplicated into this process."))
	for _, p := range []struct {
		n   string
		get func(s *SemLock) py.Object
	}{
		{"handle", func(s *SemLock) py.Object { return py.Int(s.h) }},
		{"kind", func(s *SemLock) py.Object { return py.Int(s.kind) }},
		{"maxvalue", func(s *SemLock) py.Object { return py.Int(s.maxvalue) }},
		{"name", func(s *SemLock) py.Object { return py.None }},
	} {
		p := p
		SemLockType.Dict.Set(p.n, &py.Property{Fget: func(self py.Object) (py.Object, error) { return p.get(self.(*SemLock)), nil }})
	}
	SemLockType.Dict.Set("SEM_VALUE_MAX", py.Int(semValueMax))
	register(py.NewStringDictFrom(
		py.DictEntry{Key: "SemLock", Value: SemLockType},
		py.DictEntry{Key: "SEM_VALUE_MAX", Value: py.Int(semValueMax)},
		py.DictEntry{Key: "sem_unlink", Value: py.MustNewMethod("sem_unlink", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.None, nil // Windows semaphores are unnamed
		}, 0, "No-op: Windows semaphores have no name to unlink.")},
	))
}
