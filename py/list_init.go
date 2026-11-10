// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

// list.__init__(self, iterable) replaces the contents of an existing list -
// including an instance of a Python subclass, whose storage is its payload.
// A subclass's own __init__ calls it to fill itself after construction
// (tomlkit's Array).
//
// The method is marked Unbound so that constructing a list does not run it a
// second time after the native constructor: the type-call path calls only
// bound __init__ methods.
func init() {
	m, err := NewMethod("__init__", func(self Object, args Tuple) (Object, error) {
		if p, ok := payloadOf(self); ok {
			self = p
		}
		l, ok := self.(*List)
		if !ok {
			return nil, ExceptionNewf(TypeError, "descriptor '__init__' requires a 'list' object but received a '%s'", self.Type().Name)
		}
		if len(args) > 1 {
			return nil, ExceptionNewf(TypeError, "list expected at most 1 argument, got %d", len(args))
		}
		var items []Object
		if len(args) == 1 {
			// ponytail: the native constructor already drained a single-use
			// iterator passed to list subclass construction (CPython's list.__new__
			// ignores its arguments), so refilling from it would empty the list.
			// Keep the contents; explicit re-init from an iterator is the ceiling.
			if it, err := Iter(args[0]); err == nil && it == args[0] {
				return None, nil
			}
			tmp := NewListFromItems(nil)
			if err := tmp.ExtendSequence(args[0]); err != nil {
				return nil, err
			}
			items = tmp.Items
		}
		l.Items = items
		return None, nil
	}, 0, "Initialize self.  See help(type(self)) for accurate signature.")
	if err != nil {
		panic(err)
	}
	m.Unbound = true
	ListType.Dict.Set("__init__", m)
}

// attachListPayload gives an instance of a class that derives from list AND
// from Python classes - class Array(Item, MutableSequence, list), tomlkit's -
// the list storage its native constructor never made: New came from the first
// base, a Python class, and returned a plain instance.  With no Python
// __init__ the construction arguments fill it as list.__init__ would.
func attachListPayload(t *Type, obj Object, args Tuple) error {
	inst, ok := obj.(*Type)
	if !ok || inst.Payload != nil || inst.ObjectType != t || t.Name == "" || !t.IsSubtype(ListType) {
		return nil
	}
	l := NewListFromItems(nil)
	if _, hasPyInit := t.Lookup("__init__").(*Function); !hasPyInit && len(args) == 1 {
		if err := l.ExtendSequence(args[0]); err != nil {
			return err
		}
	}
	inst.Payload = l
	return nil
}
