// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package abc provides the implementation of python's 'collections.abc'
// module: the abstract base classes for containers.
//
// There is no metaclass machinery here to install a subclass hook on, so
// structural conformance is checked through py.MatchesABC, which isinstance
// consults.  That is what makes "isinstance([], Iterable)" true even though
// a list does not derive from Iterable.
//
// The classes are subscriptable - "Sequence[int]" - because that is how they
// are written in annotations, which are evaluated unless the module uses
// from __future__ import annotations.
package abc

import (
	_ "embed"
	"strings"
	"sync"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/collections"
)

//go:embed mixins.py
var mixinsSrc string

var installOnce sync.Once

// install copies the Python-defined concrete methods onto the Go ABC types,
// once per process: the types are shared by every context, and their dicts
// are plain maps.
func install(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "_install takes one argument")
	}
	pairs, ok := args[0].(*py.List)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "_install takes a list")
	}
	var err error
	installOnce.Do(func() {
		for _, pr := range pairs.Items {
			kv := pr.(py.Tuple)
			t, ok := kv[0].(*py.Type)
			methods, ok2 := kv[1].(py.Tuple)
			if !ok || !ok2 {
				err = py.ExceptionNewf(py.TypeError, "_install: bad entry")
				return
			}
			for _, m := range methods {
				name := string(m.(py.Tuple)[0].(py.String))
				if _, has := t.Dict.Get(name); !has {
					t.Dict.Set(name, m.(py.Tuple)[1])
				}
			}
		}
	})
	return py.None, err
}

const module_doc = `Abstract Base Classes for Containers.  (PEP 3119)`

// Container, Hashable, Sized, Callable, Iterable, Iterator, Reversible,
// Collection, Sequence, MutableSequence, Set, MutableSet, Mapping,
// MutableMapping, MappingView, KeysView, ItemsView, ValuesView, Awaitable,
// Coroutine, AsyncIterable, AsyncIterator, AsyncGenerator, Generator.

// abcType makes one abstract base class, deriving it from base so that the
// isinstance base-chain walk recognises the hierarchy.
func abcType(name string, bases ...*py.Type) *py.Type {
	t := py.NewType("collections.abc."+name, name+" abstract base class")
	for _, b := range bases {
		if b != nil {
			t.Bases = append(t.Bases, b)
		}
	}
	if len(t.Bases) > 0 {
		t.Base = t.Bases[0].(*py.Type)
	}
	return t
}

// subscriptable gives a class the "Class[params]" form used in annotations.
// The parameters carry no runtime meaning here, so the class itself is
// returned, which is all that evaluating an annotation needs.
func subscriptable(t *py.Type) {
	t.Dict.Set("__class_getitem__", py.ClassGetItemAlias)
}

// has reports whether the object satisfies the named protocol.
//
// The native types in this standard library implement their protocols as Go
// interfaces rather than as entries in the type's Dict, so a Dict lookup
// alone would say that a list is not Iterable.  Both are consulted.
func has(obj py.Object, name string) bool {
	for t := obj.Type(); t != nil; t = t.Base {
		if t.Lookup(name) != nil {
			return true
		}
	}
	switch name {
	case "__len__":
		_, ok := obj.(py.I__len__)
		return ok
	case "__iter__":
		_, ok := obj.(py.I__iter__)
		return ok
	case "__next__":
		_, ok := obj.(py.I__next__)
		return ok
	case "__getitem__":
		_, ok := obj.(py.I__getitem__)
		return ok
	case "__setitem__":
		_, ok := obj.(py.I__setitem__)
		return ok
	case "__contains__":
		_, ok := obj.(py.I__contains__)
		return ok
	case "__hash__":
		_, ok := obj.(py.I__hash__)
		return ok
	case "__call__":
		_, ok := obj.(py.I__call__)
		return ok
	case "__reversed__":
		_, ok := obj.(py.I__reversed__)
		return ok
	case "__le__":
		_, ok := obj.(py.I__le__)
		return ok
	}
	return false
}

// hasIter and hasNext cover the protocol types.
func hasIter(obj py.Object) bool { return has(obj, "__iter__") }

func hasNext(obj py.Object) bool { return has(obj, "__next__") }

// isBuiltinHashable reports whether obj is one of the value types that the
// hash() builtin knows how to hash.  They carry no __hash__ method of their
// own, so the method check alone would call a string unhashable.
func isBuiltinHashable(obj py.Object) bool {
	switch obj.(type) {
	case py.String, py.Int, *py.BigInt, py.Float, py.Bool, py.NoneType, py.Bytes, py.Tuple, *py.FrozenSet:
		return true
	}
	return false
}

// isStringOrBytes reports whether obj is the ByteString type's idea of one.

var (
	ContainerType       = abcType("Container", nil)
	HashableType        = abcType("Hashable", nil)
	SizedType           = abcType("Sized", nil)
	CallableType        = abcType("Callable", nil)
	IterableType        = abcType("Iterable", nil)
	IteratorType        = abcType("Iterator", IterableType)
	ReversibleType      = abcType("Reversible", IterableType)
	GeneratorType       = abcType("Generator", IteratorType)
	CollectionType      = abcType("Collection", SizedType, IterableType, ContainerType)
	SequenceType        = abcType("Sequence", ReversibleType, CollectionType)
	MutableSequenceType = abcType("MutableSequence", SequenceType)
	SetType             = abcType("Set", CollectionType)
	MutableSetType      = abcType("MutableSet", SetType)
	MappingType         = abcType("Mapping", CollectionType)
	MutableMappingType  = abcType("MutableMapping", MappingType)
	MappingViewType     = abcType("MappingView", SizedType)
	KeysViewType        = abcType("KeysView", MappingViewType, SetType)
	ItemsViewType       = abcType("ItemsView", MappingViewType, SetType)
	ValuesViewType      = abcType("ValuesView", MappingViewType, CollectionType)
	AwaitableType       = abcType("Awaitable", nil)
	CoroutineType       = abcType("Coroutine", AwaitableType)
	AsyncIterableType   = abcType("AsyncIterable", nil)
	AsyncIteratorType   = abcType("AsyncIterator", AsyncIterableType)
	AsyncGeneratorType  = abcType("AsyncGenerator", AsyncIteratorType)
)

var all = map[string]*py.Type{
	"Container":       ContainerType,
	"Hashable":        HashableType,
	"Sized":           SizedType,
	"Callable":        CallableType,
	"Iterable":        IterableType,
	"Iterator":        IteratorType,
	"Reversible":      ReversibleType,
	"Generator":       GeneratorType,
	"Collection":      CollectionType,
	"Sequence":        SequenceType,
	"MutableSequence": MutableSequenceType,
	"Set":             SetType,
	"MutableSet":      MutableSetType,
	"Mapping":         MappingType,
	"MutableMapping":  MutableMappingType,
	"MappingView":     MappingViewType,
	"KeysView":        KeysViewType,
	"ItemsView":       ItemsViewType,
	"ValuesView":      ValuesViewType,
	"Awaitable":       AwaitableType,
	"Coroutine":       CoroutineType,
	"AsyncIterable":   AsyncIterableType,
	"AsyncIterator":   AsyncIteratorType,
	"AsyncGenerator":  AsyncGeneratorType,
	"ByteString":      SequenceType,
	"Buffer":          ContainerType,
}

// registered holds the classes register() has declared virtual subclasses of
// each ABC, as ABCMeta's _abc_registry does.
var (
	registeredMu sync.Mutex
	registered   = map[*py.Type][]*py.Type{}
)

// isRegistered reports whether obj's class was registered with abc, or with an
// ABC deriving from it - registering with Mapping also makes a Collection.
func isRegistered(obj py.Object, abc *py.Type) bool {
	cls := obj.Type()
	registeredMu.Lock()
	defer registeredMu.Unlock()
	for t, classes := range registered {
		if !t.IsSubtype(abc) {
			continue
		}
		for _, c := range classes {
			if cls.IsSubtype(c) {
				return true
			}
		}
	}
	return false
}

func init() {
	for _, t := range all {
		subscriptable(t)
		// The abstract base classes exist to be derived from.
		t.Flags |= py.TPFLAGS_BASETYPE
		// register(cls) declares a virtual subclass and returns it, so it
		// also works as a decorator.  These ABCs had none, so
		// contextvars' "_collections_abc.Mapping.register(Context)" - and
		// through it decimal - failed at import.
		abc := t
		t.Dict.Set("register", py.MustNewMethod("register", func(self py.Object, args py.Tuple) (py.Object, error) {
			// Reached through the class, the method is unbound and the class
			// to register arrives as self.
			arg := self
			if len(args) == 1 {
				arg = args[0]
			} else if len(args) > 1 {
				return nil, py.ExceptionNewf(py.TypeError, "register() takes exactly one argument (%d given)", len(args))
			}
			cls, ok := arg.(*py.Type)
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "Can only register classes")
			}
			registeredMu.Lock()
			registered[abc] = append(registered[abc], cls)
			registeredMu.Unlock()
			return cls, nil
		}, 0, "Register a virtual subclass of an ABC."))
	}

	// Structural conformance.  Every test is a method presence check, which
	// is how Python's own ABCs behave for the methods they declare abstract.
	py.ABCHooks = append(py.ABCHooks, func(obj py.Object, class *py.Type) bool {
		// Types are not instances of the ABCs; only their instances are.
		// Before the class check below: an instance of a Python class is
		// itself a *py.Type here.
		if isRegistered(obj, class) {
			return true
		}
		if _, isType := obj.(*py.Type); isType {
			return false
		}

		switch class {
		case ContainerType:
			return has(obj, "__contains__") || has(obj, "__iter__")
		case HashableType:
			return has(obj, "__hash__") || isBuiltinHashable(obj)
		case SizedType:
			return has(obj, "__len__")
		case CallableType:
			return has(obj, "__call__")
		case IterableType, AsyncIterableType:
			return hasIter(obj)
		case IteratorType, AsyncIteratorType, GeneratorType, AsyncGeneratorType:
			return hasIter(obj) && hasNext(obj)
		case ReversibleType:
			return has(obj, "__reversed__") || has(obj, "__getitem__")
		case CollectionType:
			return has(obj, "__len__") && hasIter(obj) && has(obj, "__contains__")
		case SequenceType:
			// A sequence is sized, iterable and supports integer indexing.
			return has(obj, "__len__") && hasIter(obj) && has(obj, "__getitem__")
		case MutableSequenceType:
			// A list has no __setitem__ interface; it is mutable precisely
			// because it is a list, so accept it and the UserList wrapper.
			return has(obj, "__len__") && hasIter(obj) && has(obj, "__getitem__") &&
				(has(obj, "__setitem__") || isMutableList(obj))
		case SetType:
			return has(obj, "__len__") && hasIter(obj) && has(obj, "__contains__")
		case MutableSetType:
			return has(obj, "__len__") && hasIter(obj) && has(obj, "__contains__") && !isFrozenSet(obj)
		case MappingType:
			return has(obj, "__len__") && hasIter(obj) && has(obj, "__getitem__") && has(obj, "keys")
		case MutableMappingType:
			return has(obj, "__len__") && hasIter(obj) && has(obj, "__getitem__") && has(obj, "keys") && !isFrozenSet(obj)
		case MappingViewType, KeysViewType, ItemsViewType, ValuesViewType:
			return has(obj, "__len__") && hasIter(obj)
		}
		return false
	})

	globals := py.NewStringDict()
	for name, t := range all {
		globals.Set(name, t)
	}
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "collections.abc",
			Doc:  module_doc,
		},
		Globals: globals,
		Methods: []*py.Method{py.MustNewMethod("_install", install, 0, "Install the concrete mixin methods.")},
		CodeSrc: mixinsSrc,
	})
	// _collections_abc is the same module: CPython's collections.abc is
	// "from _collections_abc import *", and contextvars imports it by name.
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "_collections_abc", Doc: module_doc},
		Globals: globals,
		Methods: []*py.Method{py.MustNewMethod("_install", install, 0, "Install the concrete mixin methods.")},
		CodeSrc: mixinsSrc,
	})
}

// isMutableList reports whether obj is a list, which is mutable without
// implementing the __setitem__ interface (the VM mutates it directly).
func isMutableList(obj py.Object) bool {
	if _, ok := obj.(*py.List); ok {
		return true
	}
	if _, ok := obj.(*collections.UserList); ok {
		return true
	}
	return false
}

// isFrozenSet reports whether obj is immutable, so that it satisfies Set
// without satisfying MutableSet.
func isFrozenSet(obj py.Object) bool {
	switch obj.(type) {
	case *py.FrozenSet:
		return true
	}
	return false
}

var _ = strings.TrimSpace

// The standard library's own containers satisfy the ABCs their CPython
// counterparts register with; see isMutableList and the Counter/Deque types
// in collections, which the checks above already recognise.
