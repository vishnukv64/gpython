//go:build !windows && !plan9 && !js && !wasip1

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mpcore

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/thread"
)

const (
	recursiveMutex = 0
	semaphoreKind  = 1
	semValueMax    = 32767
)

// semPath maps a POSIX semaphore name ("/mp-abc") to its backing file.
func semPath(name string) string {
	return filepath.Join(os.TempDir(), "gpy-sem"+strings.ReplaceAll(name, "/", "_"))
}

// SemLock is _multiprocessing.SemLock.  count and lastTid are per process, as
// in CPython: they say whether THIS process's thread holds a recursive mutex.
type SemLock struct {
	mu       sync.Mutex // serialises this process's threads around the flock
	f        *os.File
	kind     int
	maxvalue int
	name     string
	handle   int
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
	name, err := py.StrAsString(oname)
	if err != nil {
		return nil, err
	}
	if kind != recursiveMutex && kind != semaphoreKind {
		return nil, py.ExceptionNewf(py.ValueError, "unrecognized kind")
	}
	path := semPath(name)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, py.OSErrorFrom(err, path)
	}
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(value))
	if _, err := f.WriteAt(b[:], 0); err != nil {
		f.Close()
		os.Remove(path)
		return nil, py.OSErrorFrom(err, path)
	}
	if u, _ := py.ObjectIsTrue(unlink); u {
		os.Remove(path)
	}
	return newSem(f, kind, maxvalue, name), nil
}, nil)

var semHandles atomic.Int64

func newSem(f *os.File, kind, maxvalue int, name string) *SemLock {
	return &SemLock{f: f, kind: kind, maxvalue: maxvalue, name: name, handle: int(semHandles.Add(1))}
}

func (s *SemLock) Type() *py.Type { return SemLockType }

// locked runs fn with the counter's file locked against every process.
func (s *SemLock) locked(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fd := int(s.f.Fd())
	for {
		err := syscall.Flock(fd, syscall.LOCK_EX)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return py.OSErrorFrom(err, s.name)
		}
		break
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	return fn()
}

func (s *SemLock) get() int {
	var b [8]byte
	s.f.ReadAt(b[:], 0)
	return int(binary.LittleEndian.Uint64(b[:]))
}

func (s *SemLock) set(v int) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	s.f.WriteAt(b[:], 0)
}

func (s *SemLock) value() (v int, err error) {
	err = s.locked(func() error { v = s.get(); return nil })
	return
}

func (s *SemLock) isMine() bool { return s.count > 0 && s.lastTid == thread.Ident() }

// tryWait takes one unit if the count is positive.
func (s *SemLock) tryWait() (ok bool, err error) {
	err = s.locked(func() error {
		if v := s.get(); v > 0 {
			s.set(v - 1)
			ok = true
		}
		return nil
	})
	return
}

// acquire waits up to d (negative = forever).
func (s *SemLock) acquire(d time.Duration) (bool, error) {
	if s.kind == recursiveMutex && s.isMine() {
		s.count++
		return true, nil
	}
	var deadline time.Time
	if d >= 0 {
		deadline = time.Now().Add(d)
	}
	backoff := 20 * time.Microsecond
	for {
		ok, err := s.tryWait()
		if err != nil {
			return false, err
		}
		if ok {
			s.count++
			s.lastTid = thread.Ident()
			return true, nil
		}
		if d >= 0 {
			left := time.Until(deadline)
			if left <= 0 {
				return false, nil
			}
			if backoff > left {
				backoff = left
			}
		}
		time.Sleep(backoff)
		if backoff < 2*time.Millisecond {
			backoff *= 2
		}
	}
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
	err := s.locked(func() error {
		v := s.get()
		if v >= s.maxvalue {
			return py.ExceptionNewf(py.ValueError, "semaphore or lock released too many times")
		}
		s.set(v + 1)
		return nil
	})
	if err == nil {
		s.count--
	}
	return err
}

func (s *SemLock) M__enter__() (py.Object, error) {
	_, err := s.acquire(-1)
	return py.True, err
}

func (s *SemLock) M__exit__(a, b, c py.Object) (py.Object, error) {
	return py.None, s.release()
}

// timeout converts acquire's (block, timeout) into a wait.
func timeout(args py.Tuple, kwargs py.StringDict) (time.Duration, error) {
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
		return -1, nil
	}
	f, err := py.MakeFloat(tmo)
	if err != nil {
		return 0, err
	}
	if t := float64(f.(py.Float)); t > 0 {
		return time.Duration(t * float64(time.Second)), nil
	}
	return 0, nil
}

func init() {
	m := func(name, doc string, f func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error)) {
		SemLockType.Dict.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			return f(self.(*SemLock), args, kwargs)
		}, 0, doc))
	}
	m("acquire", "Acquire the semaphore/lock.", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		d, err := timeout(args, kwargs)
		if err != nil {
			return nil, err
		}
		ok, err := s.acquire(d)
		return py.NewBool(ok), err
	})
	m("release", "Release the semaphore/lock.", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return py.None, s.release()
	})
	m("__enter__", "Enter the semaphore/lock.", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return s.M__enter__()
	})
	m("__exit__", "Exit the semaphore/lock.", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return s.M__exit__(nil, nil, nil)
	})
	m("_count", "Num of `acquire()`s minus num of `release()`s for this process.", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return py.Int(s.count), nil
	})
	m("_is_mine", "Whether the lock is owned by this thread.", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return py.NewBool(s.isMine()), nil
	})
	m("_get_value", "Get the value of the semaphore.", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		v, err := s.value()
		return py.Int(v), err
	})
	m("_is_zero", "Return whether the semaphore has value zero.", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		v, err := s.value()
		return py.NewBool(v == 0), err
	})
	m("_after_fork", "Rezero the net acquisition count after fork().", func(s *SemLock, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		s.count = 0
		return py.None, nil
	})
	SemLockType.Dict.Set("_rebuild", py.MustNewMethod("_rebuild", func(self py.Object, args py.Tuple) (py.Object, error) {
		// (handle, kind, maxvalue, name): the handle is meaningless in another
		// process, so the semaphore is reopened by name.
		if len(args) != 4 {
			return nil, py.ExceptionNewf(py.TypeError, "_rebuild() takes 4 arguments")
		}
		kind, err := py.IndexInt(args[1])
		if err != nil {
			return nil, err
		}
		maxvalue, err := py.IndexInt(args[2])
		if err != nil {
			return nil, err
		}
		name, err := py.StrAsString(args[3])
		if err != nil {
			return nil, err
		}
		f, err := os.OpenFile(semPath(name), os.O_RDWR, 0)
		if err != nil {
			return nil, py.OSErrorFrom(err, name)
		}
		return newSem(f, kind, maxvalue, name), nil
	}, py.METH_CLASS|py.METH_STATIC, "Reopen a semaphore by name in this process."))
	for _, p := range []struct {
		n   string
		get func(s *SemLock) py.Object
	}{
		{"handle", func(s *SemLock) py.Object { return py.Int(s.handle) }},
		{"kind", func(s *SemLock) py.Object { return py.Int(s.kind) }},
		{"maxvalue", func(s *SemLock) py.Object { return py.Int(s.maxvalue) }},
		{"name", func(s *SemLock) py.Object { return py.String(s.name) }},
	} {
		p := p
		SemLockType.Dict.Set(p.n, &py.Property{Fget: func(self py.Object) (py.Object, error) { return p.get(self.(*SemLock)), nil }})
	}

	SemLockType.Dict.Set("SEM_VALUE_MAX", py.Int(semValueMax))
	register(py.NewStringDictFrom(
		py.DictEntry{Key: "SemLock", Value: SemLockType},
		py.DictEntry{Key: "SEM_VALUE_MAX", Value: py.Int(semValueMax)},
		py.DictEntry{Key: "sem_unlink", Value: py.MustNewMethod("sem_unlink", func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) != 1 {
				return nil, py.ExceptionNewf(py.TypeError, "sem_unlink() takes exactly one argument")
			}
			name, err := py.StrAsString(args[0])
			if err != nil {
				return nil, err
			}
			if err := os.Remove(semPath(name)); err != nil {
				return nil, py.OSErrorFrom(err, name)
			}
			return py.None, nil
		}, 0, "Unlink the named semaphore.")},
	))
	registerForkExec()
	registerShm()
}
