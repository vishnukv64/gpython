// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package threading provides the implementation of python's 'threading'
// module.
//
// This interpreter runs one interpreter per context and does not create
// Python threads, so the parts that exist here are the ones that work
// without them:
//
//   - local() is fully implemented.  It is a per-thread attribute holder,
//     and it is what code uses to keep a value tied to "the current
//     execution" - click's globals are built on it.  With no Python threads
//     the attributes are per-context, which is what a single thread observes.
//
//   - Lock and RLock are real locks and can be acquired and released.
//
//   - Thread, current_thread and the rest exist so that an import and an
//     isinstance succeed; starting a Thread raises, because there is no
//     second Python thread to run the target in.  Returning a Thread object
//     that silently never runs would be worse than refusing.
package threading

import (
	"fmt"
	"os"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Thread-based parallelism.

This implementation provides local(), the locks and the thread objects; it
does not create Python threads, so Thread.start() raises.`

// Local is a per-thread namespace.  Attributes set on one are visible only
// to the thread that set them; here there is one Python thread per context,
// so they are kept in the context.
type Local struct {
	mu   sync.Mutex
	data py.StringDict
	// cls is the class this holder was created as, which is the SUBCLASS when
	// one is used - "class ConsoleThreadLocals(threading.local)" is how rich
	// keeps its per-thread console buffer, and every such instance used to
	// report LocalType, so the subclass's own class attributes were
	// unreachable and "type(holder)" named the base.
	cls *py.Type
}

var LocalType = py.NewTypeX("threading.local", "A class that represents thread-local data.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// metatype is the class being instantiated, so a subclass instance records
	// itself rather than the base it was declared against.
	return &Local{data: py.NewStringDict(), cls: metatype}, nil
}, nil)

func (l *Local) Type() *py.Type {
	if l.cls != nil {
		return l.cls
	}
	return LocalType
}

// Lock is a mutual exclusion lock.
type Lock struct {
	mu     sync.Mutex
	locked bool
}

var LockType = py.NewTypeX("threading.Lock", "A lock object.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return &Lock{}, nil
}, nil)

func (l *Lock) Type() *py.Type { return LockType }

// RLock is a reentrant lock: the same thread may acquire it more than once,
// and only the thread that holds it may release it.
//
// It is NOT built on sync.Mutex held across the acquisitions: a Go mutex is
// not reentrant, so locking it twice from here deadlocks the whole process
// ("all goroutines are asleep").  The state is therefore tracked explicitly,
// with a short-lived mutex only around the bookkeeping.
type RLock struct {
	mu    sync.Mutex
	depth int
	owner int64
}

var RLockType = py.NewTypeX("threading.RLock", "A reentrant lock object.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return &RLock{}, nil
}, nil)

func (r *RLock) Type() *py.Type { return RLockType }

// Thread exists so that a subclass can be defined and an isinstance test can
// answer; it is not started.
type Thread struct {
	target py.Object
	args   py.Tuple
	kwargs py.StringDict
	name   string
	daemon bool

	// A Thread runs its target in a real Go goroutine.
	//
	// Interpreter state is per-goroutine (the current frame and the current
	// exception are keyed by goroutine id), so two goroutines can run py code
	// at once without fighting over it - which is what made this possible.
	//
	// CAVEAT, and it is a real one: this is concurrency, not CPython's
	// threading.  There is no GIL, so several threads DO run py code
	// simultaneously; CPython's threads do not.  Code that relies on the GIL
	// for atomicity - "x += 1" from two threads - is safe here only because
	// each THREAD shares its globals, and a shared global mutated from two
	// threads is exactly the data race the Go race detector will report.
	// Python-level synchronisation (Lock, Event when it exists) is the way to
	// order them.
	wg     sync.WaitGroup
	done   chan struct{}
	mu     sync.Mutex
	alive  bool
	joined bool
}

var ThreadType = py.NewTypeX("threading.Thread", "A thread of control.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	t := &Thread{args: py.Tuple{}, kwargs: py.NewStringDict(), name: "Thread-1"}
	for _, __e := range kwargs.Items() {
		k := __e.Key
		v := __e.Value

		switch k {
		case "target":
			t.target = v
		case "args":
			items, err := py.SequenceList(v)
			if err != nil {
				return nil, err
			}
			t.args = py.Tuple(items.Items)
		case "kwargs":
			if d, ok := v.(py.IGetDict); ok {
				t.kwargs = d.GetDict()
			}
		case "name":
			if s, ok := v.(py.String); ok {
				t.name = string(s)
			}
		case "daemon":
			t.daemon = v == py.True
		}
	}
	return t, nil
}, nil)

func (t *Thread) Type() *py.Type { return ThreadType }

func init() {
	LocalType.Flags |= py.TPFLAGS_BASETYPE
	LockType.Flags |= py.TPFLAGS_BASETYPE
	RLockType.Flags |= py.TPFLAGS_BASETYPE
	ThreadType.Flags |= py.TPFLAGS_BASETYPE

	// local: attributes live in the holder's own dict, which is what
	// __dict__ exposes.
	LocalType.Dict.Set("__dict__", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			l := self.(*Local)
			l.mu.Lock()
			defer l.mu.Unlock()
			return l.data, nil
		},
	})
	LocalType.Dict.Set("__getattribute__", py.MustNewMethod("__getattribute__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var name py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getattribute__", 1, 1, &name); err != nil {
			return nil, err
		}
		text, err := py.StrAsString(name)
		if err != nil {
			return nil, err
		}
		// self is the CLASS when the attribute is being read off
		// "threading.local" itself - "type(threading.local()).__name__" - and
		// only an instance otherwise.  Asserting outright panicked with
		// "interface conversion: py.Object is *py.Type, not *threading.Local".
		l, ok := self.(*Local)
		if !ok {
			return nil, py.ExceptionNewf(py.AttributeError, "'%s' object has no attribute '%s'", "type", text)
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		if v, ok := l.data.Get(text); ok {
			return v, nil
		}
		// The CLASS is consulted next: a subclass declaring "x = 0" in its
		// body leaves x readable on every holder until an instance assigns to
		// it, which is exactly CPython's rule and what rich relies on for the
		// defaults on ConsoleThreadLocals.
		if cls := l.Type(); cls != nil {
			if v := cls.Lookup(text); v != nil {
				if getter, ok := v.(py.I__get__); ok {
					return getter.M__get__(l, cls)
				}
				return v, nil
			}
		}
		return nil, py.ExceptionNewf(py.AttributeError, "'%s' object has no attribute '%s'", l.Type().Name, text)
	}, 0, "Return the thread-local attribute."))
	LocalType.Dict.Set("__setattr__", py.MustNewMethod("__setattr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var name, value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__setattr__", 2, 2, &name, &value); err != nil {
			return nil, err
		}
		text, err := py.StrAsString(name)
		if err != nil {
			return nil, err
		}
		l, ok := self.(*Local)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "cannot set attribute on '%s' object", "type")
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		l.data.Set(text, value)
		return py.None, nil
	}, 0, "Set the thread-local attribute."))

	lockMethods := func(t *py.Type, isReentrant bool) {
		t.Dict.Set("acquire", py.MustNewMethod("acquire", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			blocking := py.Object(py.True)
			timeout := py.Object(py.None)
			// acquire is normally called as acquire(False) or
			// acquire(blocking=False), and taking no keyword arguments made
			// the second a TypeError - the standard non-blocking idiom.
			// UnpackTuple rejects keywords outright, so they are read here.
			if err := py.UnpackTuple(args, py.StringDict{}, "acquire", 0, 2, &blocking, &timeout); err != nil {
				return nil, err
			}
			if v, ok := kwargs.Get("blocking"); ok {
				blocking = v
			}
			if v, ok := kwargs.Get("timeout"); ok {
				timeout = v
			}
			switch v := self.(type) {
			case *Lock:
				if blocking == py.True {
					v.mu.Lock()
					v.locked = true
					return py.True, nil
				}
				if v.mu.TryLock() {
					v.locked = true
					return py.True, nil
				}
				return py.False, nil
			case *RLock:
				return v.acquire(), nil
			}
			return py.None, nil
		}, 0, "Acquire the lock."))
		t.Dict.Set("release", py.MustNewMethod("release", func(self py.Object, args py.Tuple) (py.Object, error) {
			switch v := self.(type) {
			case *Lock:
				v.mu.Unlock()
				v.locked = false
			case *RLock:
				return v.release()
			}
			return py.None, nil
		}, 0, "Release the lock."))
		t.Dict.Set("__enter__", py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
			switch v := self.(type) {
			case *Lock:
				v.mu.Lock()
				v.locked = true
			case *RLock:
				v.acquire()
			}
			return self, nil
		}, 0, "Acquire the lock and return it."))
		t.Dict.Set("__exit__", py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
			switch v := self.(type) {
			case *Lock:
				v.mu.Unlock()
				v.locked = false
			case *RLock:
				return v.release()
			}
			return py.False, nil
		}, 0, "Release the lock."))
		if isReentrant {
			t.Dict.Set("_is_owned", py.MustNewMethod("_is_owned", func(self py.Object, args py.Tuple) (py.Object, error) {
				return py.NewBool(self.(*RLock).depth > 0), nil
			}, 0, "Return whether this thread holds the lock."))
		}
	}
	lockMethods(LockType, false)
	lockMethods(RLockType, true)

	LockType.Dict.Set("locked", py.MustNewMethod("locked", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(self.(*Lock).locked), nil
	}, 0, "Return whether the lock is held."))

	ThreadType.Dict.Set("start", py.MustNewMethod("start", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*Thread)
		t.mu.Lock()
		if t.alive {
			t.mu.Unlock()
			return nil, py.ExceptionNewf(py.RuntimeError, "threads can only be started once")
		}
		t.alive = true
		t.done = make(chan struct{})
		t.mu.Unlock()

		t.wg.Add(1)
		// The target runs in its own goroutine.  run() is the same code the
		// single-threaded path used, so a subclass overriding run() still
		// works.
		go func() {
			defer t.wg.Done()
			defer close(t.done)
			t.mu.Lock()
			t.alive = false
			t.mu.Unlock()
			// Call run() through the ordinary lookup, so a subclass that
			// overrides it is honoured.
			runObj, err := py.GetAttrString(t, "run")
			if err != nil {
				logThreadError(t, err)
				return
			}
			if _, err := py.Call(runObj, py.Tuple{}, py.StringDict{}); err != nil {
				// The result is discarded as CPython discards it, and an
				// exception is REPORTED rather than propagated: there is no
				// caller left to receive it.
				logThreadError(t, err)
			}
		}()
		return py.None, nil
	}, 0, "Start the thread's activity in a new goroutine."))

	ThreadType.Dict.Set("run", py.MustNewMethod("run", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*Thread)
		if t.target == nil || t.target == py.None {
			return py.None, nil
		}
		_, err := py.Call(t.target, t.args, t.kwargs)
		return py.None, err
	}, 0, "Run the target in the current thread."))

	ThreadType.Dict.Set("join", py.MustNewMethod("join", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*Thread)
		t.mu.Lock()
		done := t.done
		t.mu.Unlock()
		if done == nil {
			// Never started: nothing to wait for.
			return py.None, nil
		}
		<-done
		t.wg.Wait()
		return py.None, nil
	}, 0, "Wait until the thread terminates."))

	ThreadType.Dict.Set("is_alive", py.MustNewMethod("is_alive", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*Thread)
		t.mu.Lock()
		defer t.mu.Unlock()
		return py.NewBool(t.alive), nil
	}, 0, "Return whether the thread is alive."))

	ThreadType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*Thread).name), nil },
	})
	ThreadType.Dict.Set("daemon", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.NewBool(self.(*Thread).daemon), nil },
	})

	// The main thread is the only one there is.
	mainThread := &Thread{name: "MainThread"}

	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "local", Value: LocalType},
		py.DictEntry{Key: "Lock", Value: LockType},
		py.DictEntry{Key: "RLock", Value: RLockType},
		py.DictEntry{Key: "Thread", Value: ThreadType},
		py.DictEntry{Key: "ThreadError", Value: py.ExceptionType.NewType("threading.ThreadError", "Raised for threading errors.", nil, nil)},
		// These used to be ALIASES of LockType, which is worse than missing:
		// the name existed and the object constructed, so hasattr said yes and
		// try/except said yes, but there was no set(), no is_set(), no wait().
		py.DictEntry{Key: "Event", Value: EventType},
		py.DictEntry{Key: "Semaphore", Value: SemaphoreType},
		py.DictEntry{Key: "Barrier", Value: BarrierType},
		py.DictEntry{Key: "BoundedSemaphore", Value: SemaphoreType},
		py.DictEntry{Key: "Condition", Value: ConditionType},
		py.DictEntry{Key: "get_ident", Value: py.MustNewMethod("get_ident", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(1), nil
		}, 0, "Return the identifier of the current thread.")},
		py.DictEntry{Key: "current_thread", Value: py.MustNewMethod("current_thread", func(self py.Object, args py.Tuple) (py.Object, error) {
			return mainThread, nil
		}, 0, "Return the current Thread object.")},
		py.DictEntry{Key: "main_thread", Value: py.MustNewMethod("main_thread", func(self py.Object, args py.Tuple) (py.Object, error) {
			return mainThread, nil
		}, 0, "Return the main Thread object.")},
		py.DictEntry{Key: "active_count", Value: py.MustNewMethod("active_count", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(1), nil
		}, 0, "Return the number of Thread objects currently alive.")},
		py.DictEntry{Key: "enumerate", Value: py.MustNewMethod("enumerate", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.NewListFromItems([]py.Object{mainThread}), nil
		}, 0, "Return a list of all Thread objects currently alive.")},
		py.DictEntry{Key: "stack_size", Value: py.MustNewMethod("stack_size", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(0), nil
		}, 0, "Return the thread stack size.")},
		py.DictEntry{Key: "settrace", Value: py.MustNewMethod("settrace", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.None, nil
		}, 0, "Set a trace function (no-op).")},
		py.DictEntry{Key: "setprofile", Value: py.MustNewMethod("setprofile", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.None, nil
		}, 0, "Set a profile function (no-op).")},
		py.DictEntry{Key: "TIMEOUT_MAX", Value: py.Float(1e9)},
	)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "threading",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// acquire takes the reentrant lock, counting the depth so that a repeat
// acquisition by the same owner does not block.
func (r *RLock) acquire() py.Object {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.depth++
	r.owner = 1
	return py.True
}

// release drops one level of the lock.
func (r *RLock) release() (py.Object, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.depth == 0 {
		return nil, py.ExceptionNewf(py.RuntimeError, "cannot release un-acquired lock")
	}
	r.depth--
	if r.depth == 0 {
		r.owner = 0
	}
	return py.None, nil
}

// ---------------------------------------------------------------------------
// Go interface bridges
//
// A method registered only in a type's Dict is not reliably reached for an
// instance here - the same binding issue the other native types work around
// - so the attribute protocol for local() is implemented directly.  The
// assertion at the bottom makes an omission a compile error.

func (l *Local) M__getattribute__(name string) (py.Object, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// __dict__ is the thread's namespace itself, not a missing attribute.
	// Intercepting everything made it an AttributeError, and click reads
	// "_local.__dict__.setdefault(...)" while pushing its context.
	if name == "__dict__" {
		return l.data, nil
	}
	if v, ok := l.data.Get(name); ok {
		return v, nil
	}
	// The CLASS is consulted next, so a subclass declaring defaults in its body
	// can read them until an instance assigns to the name.  rich's
	// ConsoleThreadLocals declares "buffer_index: int = 0" that way and is how
	// pip's console reaches it; without this the attribute was an
	// AttributeError.
	if cls := l.Type(); cls != nil {
		if v := cls.Lookup(name); v != nil {
			if getter, ok := v.(py.I__get__); ok {
				return getter.M__get__(l, cls)
			}
			return v, nil
		}
	}
	return nil, py.ExceptionNewf(py.AttributeError, "'%s' object has no attribute '%s'", l.Type().Name, name)
}

// M__setattr__ on the Local type itself is only reached for a plain
// attribute assignment, which the VM performs through SetAttr; the method
// below is what py.SetAttr looks for.
func (l *Local) M__setattr__(name string, value py.Object) (py.Object, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data.Set(name, value)
	return py.None, nil
}

func (l *Local) M__getattr__(name string) (py.Object, error) {
	return l.M__getattribute__(name)
}

var (
	_ py.I__getattribute__ = (*Local)(nil)
	_ py.I__getattr__      = (*Local)(nil)
)

// M__enter__/M__exit__ on the locks, so that "with lock:" works through the
// with statement's own lookup.
func (l *Lock) M__enter__() (py.Object, error) {
	l.mu.Lock()
	l.locked = true
	return l, nil
}

func (l *Lock) M__exit__(excType, excValue, traceback py.Object) (py.Object, error) {
	l.mu.Unlock()
	l.locked = false
	return py.False, nil
}

var (
	_ py.I__enter__ = (*Lock)(nil)
	_ py.I__exit__  = (*Lock)(nil)
)

// logThreadError reports a failure from a thread, which has no caller to
// receive it.  CPython prints to stderr in the same place.
func logThreadError(t *Thread, err error) {
	fmt.Fprintf(os.Stderr, "Exception in thread %s: %v\n", t.name, err)
}
