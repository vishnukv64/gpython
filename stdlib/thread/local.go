// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package thread

import (
	"sync"

	"github.com/vishnukv64/gpython/py"
)

// Local is _thread._local: attributes set by one thread are invisible to the
// others.  A subclass's class-body defaults are visible to every thread.
//
// ponytail: a subclass __init__ is not re-run in each new thread (CPython
// does); add when something relies on per-thread initialisation.
type Local struct {
	mu   sync.Mutex
	data map[uint64]py.StringDict
	cls  *py.Type
}

var LocalType = py.NewTypeX("_thread._local", "Thread-local data.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return &Local{data: map[uint64]py.StringDict{}, cls: metatype}, nil
}, nil)

func init() { LocalType.Flags |= py.TPFLAGS_BASETYPE }

func (l *Local) Type() *py.Type {
	if l.cls != nil {
		return l.cls
	}
	return LocalType
}

// dict is the calling thread's namespace.
func (l *Local) dict() py.StringDict {
	l.mu.Lock()
	defer l.mu.Unlock()
	id := goid()
	d, ok := l.data[id]
	if !ok {
		d = py.NewStringDict()
		l.data[id] = d
	}
	return d
}

func (l *Local) M__getattribute__(name string) (py.Object, error) {
	d := l.dict()
	if name == "__dict__" {
		return d, nil
	}
	if v, ok := d.Get(name); ok {
		return v, nil
	}
	// The class supplies defaults, as a subclass body declaring "x = 0" expects.
	if v := l.Type().Lookup(name); v != nil {
		if g, ok := v.(py.I__get__); ok {
			return g.M__get__(l, l.Type())
		}
		return v, nil
	}
	return nil, py.ExceptionNewf(py.AttributeError, "'%s' object has no attribute '%s'", l.Type().Name, name)
}

func (l *Local) M__getattr__(name string) (py.Object, error) { return l.M__getattribute__(name) }

func (l *Local) M__setattr__(name string, value py.Object) (py.Object, error) {
	d := l.dict()
	d.Set(name, value)
	return py.None, nil
}

var (
	_ py.I__getattribute__ = (*Local)(nil)
	_ py.I__getattr__      = (*Local)(nil)
)
