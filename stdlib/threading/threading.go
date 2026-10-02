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
}

var LocalType = py.NewTypeX("threading.local", "A class that represents thread-local data.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return &Local{data: py.NewStringDict()}, nil
}, nil)

func (l *Local) Type() *py.Type { return LocalType }

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
}

var ThreadType = py.NewTypeX("threading.Thread", "A thread of control.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	t := &Thread{args: py.Tuple{}, kwargs: py.NewStringDict(), name: "Thread-1"}
	for k, v := range kwargs {
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
	LocalType.Dict["__dict__"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			l := self.(*Local)
			l.mu.Lock()
			defer l.mu.Unlock()
			return l.data, nil
		},
	}
	LocalType.Dict["__getattribute__"] = py.MustNewMethod("__getattribute__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var name py.Object
		if err := py.UnpackTuple(args, nil, "__getattribute__", 1, 1, &name); err != nil {
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
		if v, ok := l.data[text]; ok {
			return v, nil
		}
		return nil, py.ExceptionNewf(py.AttributeError, "'threading.local' object has no attribute '%s'", text)
	}, 0, "Return the thread-local attribute.")
	LocalType.Dict["__setattr__"] = py.MustNewMethod("__setattr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var name, value py.Object
		if err := py.UnpackTuple(args, nil, "__setattr__", 2, 2, &name, &value); err != nil {
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
		l.data[text] = value
		return py.None, nil
	}, 0, "Set the thread-local attribute.")

	lockMethods := func(t *py.Type, isReentrant bool) {
		t.Dict["acquire"] = py.MustNewMethod("acquire", func(self py.Object, args py.Tuple) (py.Object, error) {
			blocking := py.Object(py.True)
			timeout := py.Object(py.None)
			if err := py.UnpackTuple(args, nil, "acquire", 0, 2, &blocking, &timeout); err != nil {
				return nil, err
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
		}, 0, "Acquire the lock.")
		t.Dict["release"] = py.MustNewMethod("release", func(self py.Object, args py.Tuple) (py.Object, error) {
			switch v := self.(type) {
			case *Lock:
				v.mu.Unlock()
				v.locked = false
			case *RLock:
				return v.release()
			}
			return py.None, nil
		}, 0, "Release the lock.")
		t.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
			switch v := self.(type) {
			case *Lock:
				v.mu.Lock()
				v.locked = true
			case *RLock:
				v.acquire()
			}
			return self, nil
		}, 0, "Acquire the lock and return it.")
		t.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
			switch v := self.(type) {
			case *Lock:
				v.mu.Unlock()
				v.locked = false
			case *RLock:
				return v.release()
			}
			return py.False, nil
		}, 0, "Release the lock.")
		if isReentrant {
			t.Dict["_is_owned"] = py.MustNewMethod("_is_owned", func(self py.Object, args py.Tuple) (py.Object, error) {
				return py.NewBool(self.(*RLock).depth > 0), nil
			}, 0, "Return whether this thread holds the lock.")
		}
	}
	lockMethods(LockType, false)
	lockMethods(RLockType, true)

	LockType.Dict["locked"] = py.MustNewMethod("locked", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(self.(*Lock).locked), nil
	}, 0, "Return whether the lock is held.")

	ThreadType.Dict["start"] = py.MustNewMethod("start", func(self py.Object, args py.Tuple) (py.Object, error) {
		return nil, py.ExceptionNewf(py.RuntimeError, "this interpreter does not create Python threads, so Thread.start() cannot run the target")
	}, 0, "Start the thread (not supported).")

	ThreadType.Dict["run"] = py.MustNewMethod("run", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*Thread)
		if t.target == nil || t.target == py.None {
			return py.None, nil
		}
		_, err := py.Call(t.target, t.args, t.kwargs)
		return py.None, err
	}, 0, "Run the target in the current thread.")

	ThreadType.Dict["join"] = py.MustNewMethod("join", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "Wait for the thread to finish.")

	ThreadType.Dict["is_alive"] = py.MustNewMethod("is_alive", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.False, nil
	}, 0, "Return whether the thread is alive.")

	ThreadType.Dict["name"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*Thread).name), nil },
	}
	ThreadType.Dict["daemon"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.NewBool(self.(*Thread).daemon), nil },
	}

	// The main thread is the only one there is.
	mainThread := &Thread{name: "MainThread"}

	globals := py.StringDict{
		"local":       LocalType,
		"Lock":        LockType,
		"RLock":       RLockType,
		"Thread":      ThreadType,
		"ThreadError": py.ExceptionType.NewType("threading.ThreadError", "Raised for threading errors.", nil, nil),
		"Event":       LockType,
		"Semaphore":   LockType,
		"Barrier":     LockType,
		"get_ident": py.MustNewMethod("get_ident", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(1), nil
		}, 0, "Return the identifier of the current thread."),
		"current_thread": py.MustNewMethod("current_thread", func(self py.Object, args py.Tuple) (py.Object, error) {
			return mainThread, nil
		}, 0, "Return the current Thread object."),
		"main_thread": py.MustNewMethod("main_thread", func(self py.Object, args py.Tuple) (py.Object, error) {
			return mainThread, nil
		}, 0, "Return the main Thread object."),
		"active_count": py.MustNewMethod("active_count", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(1), nil
		}, 0, "Return the number of Thread objects currently alive."),
		"enumerate": py.MustNewMethod("enumerate", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.NewListFromItems([]py.Object{mainThread}), nil
		}, 0, "Return a list of all Thread objects currently alive."),
		"stack_size": py.MustNewMethod("stack_size", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(0), nil
		}, 0, "Return the thread stack size."),
		"settrace": py.MustNewMethod("settrace", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.None, nil
		}, 0, "Set a trace function (no-op)."),
		"setprofile": py.MustNewMethod("setprofile", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.None, nil
		}, 0, "Set a profile function (no-op)."),
		"TIMEOUT_MAX": py.Float(1e9),
	}

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
	if v, ok := l.data[name]; ok {
		return v, nil
	}
	return nil, py.ExceptionNewf(py.AttributeError, "'threading.local' object has no attribute '%s'", name)
}

// M__setattr__ on the Local type itself is only reached for a plain
// attribute assignment, which the VM performs through SetAttr; the method
// below is what py.SetAttr looks for.
func (l *Local) M__setattr__(name string, value py.Object) (py.Object, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data[name] = value
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
