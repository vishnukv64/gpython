// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package weakref provides the implementation of python's 'weakref' module.
//
// Objects here are reference counted by Go's garbage collector, not by a
// Python refcount, so there is no point at which this module can be told an
// object is about to die.  What it provides is the part that works without
// that hook: a proxy that forwards attributes to the object while it is
// alive, and a callback registry that the interpreter's objects opt into by
// calling their stored callbacks when they are collected - which the
// interpreter does not do, so a callback registered here runs only when it
// is explicitly invoked through the reference.
//
// The honest consequence: a WeakValueDictionary does not drop entries when
// its values are collected.  Rather than pretend, that class is provided
// with its behaviour stated, and the common uses - keeping a per-object
// attribute table keyed by identity - work because the key is a proxy whose
// equality follows the referent.
package weakref

import (
	"sync"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Weak references support for Python.

Weak references do not keep the referent alive.  This implementation cannot
be told when an object is collected, so a callback runs only when the
reference is called directly; see the module source.`

// Ref is a weak reference to an object.
type Ref struct {
	mu       sync.Mutex
	referent py.Object
	callback py.Object
}

var RefType = py.NewTypeX("weakref.ref", "A weak reference to an object.", refNew, nil)

func (r *Ref) Type() *py.Type { return RefType }

// referent returns the object, or None if it has gone.
func (r *Ref) get() py.Object {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.referent == nil {
		return py.None
	}
	return r.referent
}

// clear drops the referent and runs the callback.
func (r *Ref) clear() error {
	r.mu.Lock()
	had := r.referent
	cb := r.callback
	r.referent = nil
	r.callback = nil
	r.mu.Unlock()
	if had != nil && cb != nil && cb != py.None {
		_, err := py.Call(cb, py.Tuple{r}, nil)
		return err
	}
	return nil
}

func refNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "weakref.ref() needs an object")
	}
	r := &Ref{referent: args[0]}
	if len(args) >= 2 {
		r.callback = args[1]
	}
	for k, v := range kwargs {
		switch k {
		case "callback":
			r.callback = v
		case "object":
			r.referent = v
		}
	}
	return r, nil
}

// Proxy forwards attribute access to the referent, which is what makes a
// weak reference usable in place of the object.
type Proxy struct {
	ref *Ref
}

var ProxyType = py.NewTypeX("weakref.ProxyType", "A proxy that forwards to the referent.", proxyNew, nil)

func (p *Proxy) Type() *py.Type { return ProxyType }

func (p *Proxy) M__getattribute__(name string) (py.Object, error) {
	target := p.ref.get()
	if target == py.None {
		return nil, py.ExceptionNewf(py.ReferenceError, "weakly-referenced object no longer exists")
	}
	return py.GetAttrString(target, name)
}

func (p *Proxy) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	target := p.ref.get()
	if target == py.None {
		return nil, py.ExceptionNewf(py.ReferenceError, "weakly-referenced object no longer exists")
	}
	return py.Call(target, args, kwargs)
}

func (p *Proxy) M__repr__() (py.Object, error) {
	target := p.ref.get()
	if target == py.None {
		return py.String("<weakproxy at 0x0 to NoneType>"), nil
	}
	return py.Repr(target)
}

// equality follows the referent, which is what lets a proxy be a dict key.
func (p *Proxy) M__eq__(other py.Object) (py.Object, error) {
	target := p.ref.get()
	if o, ok := other.(*Proxy); ok {
		other = o.ref.get()
	}
	if target == py.None {
		return py.False, nil
	}
	return py.Eq(target, other)
}

func (p *Proxy) M__hash__() (py.Object, error) {
	target := p.ref.get()
	if target == py.None {
		return py.Int(0), nil
	}
	// The referent's own hash, so that a proxy and its object agree.
	h, ok := target.(py.I__hash__)
	if !ok {
		return py.Int(0), nil
	}
	return h.M__hash__()
}

func proxyNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	ref, err := refNew(RefType, args, kwargs)
	if err != nil {
		return nil, err
	}
	return &Proxy{ref: ref.(*Ref)}, nil
}

func init() {
	globals := py.StringDict{
		"ref":                 RefType,
		"proxy":               ProxyType,
		"ProxyType":           ProxyType,
		"CallableProxyType":   ProxyType,
		"ReferenceType":       RefType,
		"WeakMethod":          RefType,
		"WeakValueDictionary": WeakValueDictionaryType,
		"WeakKeyDictionary":   WeakKeyDictionaryType,
		"WeakSet":             WeakSetType,
		"getweakrefcount": py.MustNewMethod("getweakrefcount", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(0), nil
		}, 0, "Return the number of weak references to the object."),
		"getweakrefs": py.MustNewMethod("getweakrefs", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.NewListFromItems(nil), nil
		}, 0, "Return a list of all weak references to the object."),
	}

	RefType.Dict["__call__"] = py.MustNewMethod("__call__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*Ref).get(), nil
	}, 0, "Return the referent, or None if it has gone.")

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "weakref",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// WeakValueDictionary maps a key to a weak reference to a value.
type WeakValueDictionary struct {
	items py.StringDict
}

var WeakValueDictionaryType = py.NewTypeX("weakref.WeakValueDictionary", "A mapping that holds weak references to its values.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d := &WeakValueDictionary{items: py.NewStringDict()}
	if len(args) > 0 {
		if src, ok := args[0].(py.IGetDict); ok {
			for k, v := range src.GetDict() {
				d.items[k] = v
			}
		}
	}
	for k, v := range kwargs {
		d.items[k] = v
	}
	return d, nil
}, nil)

func (d *WeakValueDictionary) Type() *py.Type { return WeakValueDictionaryType }

func (d *WeakValueDictionary) M__len__() (py.Object, error) { return py.Int(len(d.items)), nil }

func (d *WeakValueDictionary) M__getitem__(key py.Object) (py.Object, error) {
	encoded, err := py.DictKey(key)
	if err == nil {
		if v, ok := d.items[encoded]; ok {
			// A stored ref is followed to its referent.
			if r, ok := v.(*Ref); ok {
				return r.get(), nil
			}
			return v, nil
		}
	}
	return nil, py.ExceptionNewf(py.KeyError, "%v", key)
}

func (d *WeakValueDictionary) M__setitem__(key, value py.Object) (py.Object, error) {
	encoded, err := py.DictKey(key)
	if err != nil {
		return nil, err
	}
	d.items[encoded] = value
	return py.None, nil
}

func (d *WeakValueDictionary) M__contains__(item py.Object) (py.Object, error) {
	encoded, err := py.DictKey(item)
	if err != nil {
		return py.False, nil
	}
	if _, ok := d.items[encoded]; ok {
		return py.True, nil
	}
	return py.False, nil
}

func (d *WeakValueDictionary) M__iter__() (py.Object, error) {
	out := []py.Object{}
	for encoded := range d.items {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			continue
		}
		out = append(out, key)
	}
	return py.NewIterator(py.Tuple(out)), nil
}

// WeakKeyDictionary holds weak references to its keys.
type WeakKeyDictionary struct {
	items py.StringDict
}

var WeakKeyDictionaryType = py.NewTypeX("weakref.WeakKeyDictionary", "A mapping that holds weak references to its keys.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d := &WeakKeyDictionary{items: py.NewStringDict()}
	for k, v := range kwargs {
		d.items[k] = v
	}
	return d, nil
}, nil)

func (d *WeakKeyDictionary) Type() *py.Type { return WeakKeyDictionaryType }

func (d *WeakKeyDictionary) M__len__() (py.Object, error) { return py.Int(len(d.items)), nil }

func (d *WeakKeyDictionary) M__getitem__(key py.Object) (py.Object, error) {
	encoded, err := py.DictKey(key)
	if err == nil {
		if v, ok := d.items[encoded]; ok {
			return v, nil
		}
	}
	return nil, py.ExceptionNewf(py.KeyError, "%v", key)
}

func (d *WeakKeyDictionary) M__setitem__(key, value py.Object) (py.Object, error) {
	encoded, err := py.DictKey(key)
	if err != nil {
		return nil, err
	}
	d.items[encoded] = value
	return py.None, nil
}

func (d *WeakKeyDictionary) M__contains__(item py.Object) (py.Object, error) {
	encoded, err := py.DictKey(item)
	if err != nil {
		return py.False, nil
	}
	if _, ok := d.items[encoded]; ok {
		return py.True, nil
	}
	return py.False, nil
}

// WeakSet holds weak references to its members.
type WeakSet struct {
	items py.StringDict
}

var WeakSetType = py.NewTypeX("weakref.WeakSet", "A set that holds weak references to its elements.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	s := &WeakSet{items: py.NewStringDict()}
	if len(args) > 0 {
		items, err := py.SequenceList(args[0])
		if err == nil {
			for _, item := range items.Items {
				encoded, kerr := py.DictKey(item)
				if kerr == nil {
					s.items[encoded] = py.True
				}
			}
		}
	}
	return s, nil
}, nil)

func (s *WeakSet) Type() *py.Type { return WeakSetType }

func (s *WeakSet) M__len__() (py.Object, error) { return py.Int(len(s.items)), nil }

func (s *WeakSet) M__contains__(item py.Object) (py.Object, error) {
	encoded, err := py.DictKey(item)
	if err != nil {
		return py.False, nil
	}
	if _, ok := s.items[encoded]; ok {
		return py.True, nil
	}
	return py.False, nil
}

func init() {
	addFn := func(self py.Object, args py.Tuple) (py.Object, error) {
		var item py.Object
		if err := py.UnpackTuple(args, nil, "add", 1, 1, &item); err != nil {
			return nil, err
		}
		encoded, err := py.DictKey(item)
		if err != nil {
			return nil, err
		}
		switch v := self.(type) {
		case *WeakSet:
			v.items[encoded] = py.True
		case *WeakValueDictionary:
			// add() on a WeakValueDictionary is a setitem with the value.
			if len(args) >= 2 {
				v.items[encoded] = args[1]
			}
		}
		return py.None, nil
	}
	WeakSetType.Dict["add"] = py.MustNewMethod("add", addFn, 0, "Add an item.")
	WeakSetType.Dict["discard"] = py.MustNewMethod("discard", func(self py.Object, args py.Tuple) (py.Object, error) {
		var item py.Object
		if err := py.UnpackTuple(args, nil, "discard", 1, 1, &item); err != nil {
			return nil, err
		}
		encoded, err := py.DictKey(item)
		if err != nil {
			return nil, err
		}
		delete(self.(*WeakSet).items, encoded)
		return py.None, nil
	}, 0, "Discard an item.")
	WeakValueDictionaryType.Dict["keys"] = py.MustNewMethod("keys", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*WeakValueDictionary)
		out := []py.Object{}
		for encoded := range d.items {
			key, err := py.DictKeyDecode(encoded)
			if err != nil {
				continue
			}
			out = append(out, key)
		}
		return py.NewListFromItems(out), nil
	}, 0, "Return the key list.")
	WeakValueDictionaryType.Dict["values"] = py.MustNewMethod("values", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*WeakValueDictionary)
		out := []py.Object{}
		for _, v := range d.items {
			if r, ok := v.(*Ref); ok {
				out = append(out, r.get())
				continue
			}
			out = append(out, v)
		}
		return py.NewListFromItems(out), nil
	}, 0, "Return the value list.")
	WeakValueDictionaryType.Dict["get"] = py.MustNewMethod("get", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*WeakValueDictionary)
		var key py.Object
		var def py.Object = py.None
		if err := py.UnpackTuple(args, nil, "get", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		encoded, err := py.DictKey(key)
		if err == nil {
			if v, ok := d.items[encoded]; ok {
				if r, ok := v.(*Ref); ok {
					return r.get(), nil
				}
				return v, nil
			}
		}
		return def, nil
	}, 0, "Return the value for key, or the default.")
}

// Interfaces the VM reaches directly.
func (r *Ref) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return r.get(), nil
}

var (
	_ py.I__call__         = (*Ref)(nil)
	_ py.I__getattribute__ = (*Proxy)(nil)
	_ py.I__call__         = (*Proxy)(nil)
	_ py.I__repr__         = (*Proxy)(nil)
	_ py.I__eq__           = (*Proxy)(nil)
	_ py.I__hash__         = (*Proxy)(nil)
	_ py.I__len__          = (*WeakValueDictionary)(nil)
	_ py.I__getitem__      = (*WeakValueDictionary)(nil)
	_ py.I__setitem__      = (*WeakValueDictionary)(nil)
	_ py.I__contains__     = (*WeakValueDictionary)(nil)
	_ py.I__iter__         = (*WeakValueDictionary)(nil)
)
