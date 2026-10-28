// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package thread provides the _thread module: real locks, thread handles and
// thread-local data over goroutines.  CPython's own threading.py (embedded
// with the rest of Lib) is built on it, so Thread, Condition, Event,
// Semaphore, Barrier and Timer are the reference implementations.
//
// There is no GIL: Python code in two threads runs in parallel, as it does in
// a free-threaded CPython build.
package thread

import (
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/vishnukv64/gpython/py"
)

// Ident is the calling thread's identifier, as threading.get_ident() reports.
func Ident() uint64 { return goid() }

func goid() uint64 { return py.GoroutineID() }

// timeoutDur converts (blocking, timeout) into a wait: block forever (-1),
// do not wait (0), or wait up to d.
func timeoutDur(blocking py.Object, timeout py.Object) (time.Duration, error) {
	b, err := py.ObjectIsTrue(blocking)
	if err != nil {
		return 0, err
	}
	t := -1.0
	if timeout != nil {
		f, err := py.MakeFloat(timeout)
		if err != nil {
			return 0, err
		}
		t = float64(f.(py.Float))
	}
	if !b {
		if t != -1 {
			return 0, py.ExceptionNewf(py.ValueError, "can't specify a timeout for a non-blocking call")
		}
		return 0, nil
	}
	if t < 0 && t != -1 {
		return 0, py.ExceptionNewf(py.ValueError, "timeout value must be a non-negative number")
	}
	if t == -1 {
		return -1, nil
	}
	if t > 9223372036 {
		return 0, py.ExceptionNewf(py.OverflowError, "timeout value is too large")
	}
	return time.Duration(t * float64(time.Second)), nil
}

// take waits on ch for a token for up to d (-1 = forever, 0 = poll).
func take(ch chan struct{}, d time.Duration) bool {
	if d == 0 {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
	if d < 0 {
		<-ch
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ch:
		return true
	case <-t.C:
		return false
	}
}

func acquireArgs(args py.Tuple, kwargs py.StringDict) (time.Duration, error) {
	blocking, timeout := py.Object(py.True), py.Object(nil)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:acquire", []string{"blocking", "timeout"}, &blocking, &timeout); err != nil {
		return 0, err
	}
	return timeoutDur(blocking, timeout)
}

// ---------------------------------------------------------------------------
// lock

// Lock is _thread.lock.  A token in ch means unlocked.
type Lock struct{ ch chan struct{} }

var LockType = py.NewTypeX("_thread.lock", "A lock object.", func(*py.Type, py.Tuple, py.StringDict) (py.Object, error) {
	return newLock(), nil
}, nil)

func newLock() *Lock {
	l := &Lock{ch: make(chan struct{}, 1)}
	l.ch <- struct{}{}
	return l
}

func (l *Lock) Type() *py.Type { return LockType }

func (l *Lock) release() error {
	select {
	case l.ch <- struct{}{}:
		return nil
	default:
		return py.ExceptionNewf(py.RuntimeError, "release unlocked lock")
	}
}

func (l *Lock) M__enter__() (py.Object, error) {
	<-l.ch
	return py.True, nil
}

func (l *Lock) M__exit__(a, b, c py.Object) (py.Object, error) {
	return py.False, l.release()
}

// ---------------------------------------------------------------------------
// RLock

// RLock is _thread.RLock: reentrant, owned by a goroutine.  Use NewRLock.
type RLock struct {
	lk    *Lock
	mu    sync.Mutex // guards owner and count
	owner uint64
	count int
}

var RLockType = py.NewTypeX("_thread.RLock", "A reentrant lock object.", func(*py.Type, py.Tuple, py.StringDict) (py.Object, error) {
	return NewRLock(), nil
}, nil)

// NewRLock returns an unlocked reentrant lock.
func NewRLock() *RLock { return &RLock{lk: newLock()} }

func (r *RLock) Type() *py.Type { return RLockType }

func (r *RLock) owned() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count > 0 && r.owner == goid()
}

func (r *RLock) acquire(d time.Duration) bool {
	if r.owned() {
		r.mu.Lock()
		r.count++
		r.mu.Unlock()
		return true
	}
	if !take(r.lk.ch, d) {
		return false
	}
	r.mu.Lock()
	r.owner, r.count = goid(), 1
	r.mu.Unlock()
	return true
}

func (r *RLock) release() error {
	if !r.owned() {
		return py.ExceptionNewf(py.RuntimeError, "cannot release un-acquired lock")
	}
	r.mu.Lock()
	r.count--
	last := r.count == 0
	if last {
		r.owner = 0
	}
	r.mu.Unlock()
	if last {
		return r.lk.release()
	}
	return nil
}

func (r *RLock) M__enter__() (py.Object, error) {
	r.acquire(-1)
	return py.True, nil
}

func (r *RLock) M__exit__(a, b, c py.Object) (py.Object, error) {
	return py.False, r.release()
}

// ---------------------------------------------------------------------------
// thread handle

// Handle is _thread._ThreadHandle: what join() waits on.
type Handle struct {
	mu     sync.Mutex
	ident  uint64
	done   chan struct{}
	once   sync.Once
	daemon bool
}

var HandleType = py.NewTypeX("_thread._ThreadHandle", "Handle for a running thread.", func(*py.Type, py.Tuple, py.StringDict) (py.Object, error) {
	return newHandle(0), nil
}, nil)

func newHandle(ident uint64) *Handle { return &Handle{ident: ident, done: make(chan struct{})} }

func (h *Handle) Type() *py.Type { return HandleType }

func (h *Handle) setDone() { h.once.Do(func() { close(h.done) }) }

var (
	liveMu sync.Mutex
	live   = map[*Handle]struct{}{} // started non-daemon threads, joined by _shutdown
)

func startThread(fn py.Object, args py.Tuple, kwargs py.StringDict, h *Handle, daemon bool) {
	h.daemon = daemon
	if !daemon {
		liveMu.Lock()
		live[h] = struct{}{}
		liveMu.Unlock()
	}
	started := make(chan struct{})
	go func() {
		defer py.EnterGoroutine()()
		h.mu.Lock()
		h.ident = goid()
		h.mu.Unlock()
		close(started)
		defer func() {
			h.setDone()
			liveMu.Lock()
			delete(live, h)
			liveMu.Unlock()
		}()
		if _, err := py.Call(fn, args, kwargs); err != nil {
			fmt.Fprintf(os.Stderr, "Unhandled exception in thread started by %v: %v\n", fn, err)
		}
	}()
	<-started
}

func init() {
	// --- lock ---
	lockCommon := func(t *py.Type) {
		for _, n := range []string{"acquire", "acquire_lock"} {
			t.Dict.Set(n, py.MustNewMethod(n, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				d, err := acquireArgs(args, kwargs)
				if err != nil {
					return nil, err
				}
				switch l := self.(type) {
				case *Lock:
					return py.NewBool(take(l.ch, d)), nil
				case *RLock:
					return py.NewBool(l.acquire(d)), nil
				}
				return nil, py.ExceptionNewf(py.TypeError, "not a lock")
			}, 0, "Acquire the lock; return whether it was acquired."))
		}
		for _, n := range []string{"release", "release_lock"} {
			t.Dict.Set(n, py.MustNewMethod(n, func(self py.Object, args py.Tuple) (py.Object, error) {
				switch l := self.(type) {
				case *Lock:
					return py.None, l.release()
				case *RLock:
					return py.None, l.release()
				}
				return nil, py.ExceptionNewf(py.TypeError, "not a lock")
			}, 0, "Release the lock."))
		}
		for _, n := range []string{"__enter__", "__exit__"} {
			n := n
			t.Dict.Set(n, py.MustNewMethod(n, func(self py.Object, args py.Tuple) (py.Object, error) {
				if n == "__enter__" {
					return self.(interface{ M__enter__() (py.Object, error) }).M__enter__()
				}
				return self.(interface {
					M__exit__(a, b, c py.Object) (py.Object, error)
				}).M__exit__(nil, nil, nil)
			}, 0, "Context manager protocol."))
		}
		t.Flags |= py.TPFLAGS_BASETYPE
	}
	lockCommon(LockType)
	lockCommon(RLockType)
	for _, n := range []string{"locked", "locked_lock"} {
		LockType.Dict.Set(n, py.MustNewMethod(n, func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.NewBool(len(self.(*Lock).ch) == 0), nil
		}, 0, "Return whether the lock is held."))
	}
	LockType.Dict.Set("_at_fork_reinit", py.MustNewMethod("_at_fork_reinit", func(self py.Object, args py.Tuple) (py.Object, error) {
		l := self.(*Lock)
		select {
		case l.ch <- struct{}{}:
		default:
		}
		return py.None, nil
	}, 0, "Reset to unlocked."))

	// --- RLock ---
	RLockType.Dict.Set("locked", py.MustNewMethod("locked", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(len(self.(*RLock).lk.ch) == 0), nil
	}, 0, "Return whether the lock is held by any thread."))
	RLockType.Dict.Set("_is_owned", py.MustNewMethod("_is_owned", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(self.(*RLock).owned()), nil
	}, 0, "Return whether this thread holds the lock."))
	RLockType.Dict.Set("_release_save", py.MustNewMethod("_release_save", func(self py.Object, args py.Tuple) (py.Object, error) {
		r := self.(*RLock)
		if !r.owned() {
			return nil, py.ExceptionNewf(py.RuntimeError, "cannot release un-acquired lock")
		}
		r.mu.Lock()
		n, o := r.count, r.owner
		r.count, r.owner = 0, 0
		r.mu.Unlock()
		return py.Tuple{py.Int(n), py.Int(o)}, r.lk.release()
	}, 0, "Release fully, returning the state for _acquire_restore."))
	RLockType.Dict.Set("_acquire_restore", py.MustNewMethod("_acquire_restore", func(self py.Object, args py.Tuple) (py.Object, error) {
		r := self.(*RLock)
		st, ok := args[0].(py.Tuple)
		if len(args) != 1 || !ok || len(st) != 2 {
			return nil, py.ExceptionNewf(py.TypeError, "_acquire_restore() takes a (count, owner) tuple")
		}
		n, _ := py.IndexInt(st[0])
		<-r.lk.ch
		r.mu.Lock()
		r.count, r.owner = n, goid()
		r.mu.Unlock()
		return py.None, nil
	}, 0, "Reacquire the lock with the saved recursion count."))
	RLockType.Dict.Set("_recursion_count", py.MustNewMethod("_recursion_count", func(self py.Object, args py.Tuple) (py.Object, error) {
		r := self.(*RLock)
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.owner != goid() {
			return py.Int(0), nil
		}
		return py.Int(r.count), nil
	}, 0, "Return this thread's recursion depth."))
	RLockType.Dict.Set("_at_fork_reinit", py.MustNewMethod("_at_fork_reinit", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "No-op: processes are never forked."))

	// --- handle ---
	HandleType.Dict.Set("ident", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		h := self.(*Handle)
		h.mu.Lock()
		defer h.mu.Unlock()
		return py.Int(h.ident), nil
	}})
	HandleType.Dict.Set("join", py.MustNewMethod("join", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		h := self.(*Handle)
		var timeout py.Object = py.None
		if err := py.ParseTupleAndKeywords(args, kwargs, "|O:join", []string{"timeout"}, &timeout); err != nil {
			return nil, err
		}
		h.mu.Lock()
		id := h.ident
		h.mu.Unlock()
		if id == goid() && id != 0 {
			return nil, py.ExceptionNewf(py.RuntimeError, "Cannot join current thread")
		}
		if timeout == py.None {
			<-h.done
			return py.None, nil
		}
		f, err := py.MakeFloat(timeout)
		if err != nil {
			return nil, err
		}
		d := time.Duration(math.Max(0, float64(f.(py.Float))) * float64(time.Second))
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-h.done:
		case <-t.C:
		}
		return py.None, nil
	}, 0, "Wait for the thread to end, up to timeout seconds."))
	HandleType.Dict.Set("is_done", py.MustNewMethod("is_done", func(self py.Object, args py.Tuple) (py.Object, error) {
		select {
		case <-self.(*Handle).done:
			return py.True, nil
		default:
			return py.False, nil
		}
	}, 0, "Return whether the thread has ended."))
	HandleType.Dict.Set("_set_done", py.MustNewMethod("_set_done", func(self py.Object, args py.Tuple) (py.Object, error) {
		self.(*Handle).setDone()
		return py.None, nil
	}, 0, "Mark the thread as ended."))

	// --- module ---
	fn := func(name, doc string, f func(py.Tuple, py.StringDict) (py.Object, error)) py.DictEntry {
		return py.DictEntry{Key: name, Value: py.MustNewMethod(name, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			return f(args, kwargs)
		}, 0, doc)}
	}
	startNew := func(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) < 2 {
			return nil, py.ExceptionNewf(py.TypeError, "start_new_thread expected at least 2 arguments, got %d", len(args))
		}
		targs, ok := args[1].(py.Tuple)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "2nd arg must be a tuple")
		}
		kw := py.NewStringDict()
		if len(args) > 2 {
			if d, ok := args[2].(py.StringDict); ok {
				kw = d
			}
		}
		h := newHandle(0)
		startThread(args[0], targs, kw, h, true)
		return py.Int(h.ident), nil
	}
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "LockType", Value: LockType},
		py.DictEntry{Key: "lock", Value: LockType},
		py.DictEntry{Key: "RLock", Value: RLockType},
		py.DictEntry{Key: "_ThreadHandle", Value: HandleType},
		py.DictEntry{Key: "_local", Value: LocalType},
		py.DictEntry{Key: "error", Value: py.RuntimeError},
		py.DictEntry{Key: "TIMEOUT_MAX", Value: py.Float(9223372036.0)},
		fn("allocate_lock", "Create a new lock.", func(py.Tuple, py.StringDict) (py.Object, error) { return newLock(), nil }),
		fn("allocate", "Create a new lock.", func(py.Tuple, py.StringDict) (py.Object, error) { return newLock(), nil }),
		fn("get_ident", "Return the current thread's identifier.", func(py.Tuple, py.StringDict) (py.Object, error) { return py.Int(goid()), nil }),
		fn("get_native_id", "Return the current thread's identifier.", func(py.Tuple, py.StringDict) (py.Object, error) { return py.Int(goid()), nil }),
		fn("_get_main_thread_ident", "Return the main thread's identifier.", func(py.Tuple, py.StringDict) (py.Object, error) { return py.Int(1), nil }),
		fn("_is_main_interpreter", "There is one interpreter.", func(py.Tuple, py.StringDict) (py.Object, error) { return py.True, nil }),
		fn("daemon_threads_allowed", "Daemon threads are allowed.", func(py.Tuple, py.StringDict) (py.Object, error) { return py.True, nil }),
		fn("stack_size", "Goroutine stacks grow on demand; the size is not settable.", func(py.Tuple, py.StringDict) (py.Object, error) { return py.Int(0), nil }),
		fn("set_name", "Thread names are not exposed to the OS.", func(py.Tuple, py.StringDict) (py.Object, error) { return py.None, nil }),
		fn("_count", "Return the number of started non-daemon threads.", func(py.Tuple, py.StringDict) (py.Object, error) {
			liveMu.Lock()
			defer liveMu.Unlock()
			return py.Int(len(live)), nil
		}),
		fn("start_new_thread", "Start a new thread running function(*args, **kwargs); return its ident.", startNew),
		fn("start_new", "Start a new thread running function(*args, **kwargs); return its ident.", startNew),
		fn("start_joinable_thread", "Start a thread running function; return its handle.", func(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			var function, handle, daemon py.Object = nil, py.None, py.True
			if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:start_joinable_thread", []string{"function", "handle", "daemon"}, &function, &handle, &daemon); err != nil {
				return nil, err
			}
			h, ok := handle.(*Handle)
			if !ok {
				h = newHandle(0)
			}
			d, err := py.ObjectIsTrue(daemon)
			if err != nil {
				return nil, err
			}
			startThread(function, py.Tuple{}, py.NewStringDict(), h, d)
			return h, nil
		}),
		fn("_make_thread_handle", "Make a handle for an already running thread.", func(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			if len(args) != 1 {
				return nil, py.ExceptionNewf(py.TypeError, "_make_thread_handle() takes exactly one argument")
			}
			id, err := py.IndexInt(args[0])
			if err != nil {
				return nil, err
			}
			return newHandle(uint64(id)), nil
		}),
		fn("_shutdown", "Wait for every started non-daemon thread to finish.", func(py.Tuple, py.StringDict) (py.Object, error) {
			for {
				liveMu.Lock()
				var h *Handle
				for k := range live {
					h = k
					break
				}
				liveMu.Unlock()
				if h == nil {
					return py.None, nil
				}
				<-h.done
				liveMu.Lock()
				delete(live, h)
				liveMu.Unlock()
			}
		}),
	)
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "_thread", Doc: "Low-level threading primitives."},
		Globals: globals,
	})
}
