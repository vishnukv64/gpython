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
	"strings"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/collections"
)

const module_doc = `Abstract Base Classes for Containers.  (PEP 3119)`

// Container, Hashable, Sized, Callable, Iterable, Iterator, Reversible,
// Collection, Sequence, MutableSequence, Set, MutableSet, Mapping,
// MutableMapping, MappingView, KeysView, ItemsView, ValuesView, Awaitable,
// Coroutine, AsyncIterable, AsyncIterator, AsyncGenerator, Generator.

// abcType makes one abstract base class, deriving it from base so that the
// isinstance base-chain walk recognises the hierarchy.
func abcType(name string, base *py.Type) *py.Type {
	t := py.NewType("collections.abc."+name, name+" abstract base class")
	if base != nil {
		t.Base = base
	}
	return t
}

// subscriptable gives a class the "Class[params]" form used in annotations.
// The parameters carry no runtime meaning here, so the class itself is
// returned, which is all that evaluating an annotation needs.
func subscriptable(t *py.Type) {
	getitem := func(self py.Object, args py.Tuple) (py.Object, error) {
		// The parameters carry no runtime meaning here, so the class itself
		// is returned, which is all that evaluating an annotation needs.
		return self, nil
	}
	t.Dict.Set("__class_getitem__", py.MustNewMethod("__class_getitem__", getitem, 0, "Return the class, ignoring the subscription parameters."))
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
func isStringOrBytes(obj py.Object) bool {
	switch obj.(type) {
	case py.String, py.Bytes:
		return true
	}
	return false
}

var (
	ContainerType       = abcType("Container", nil)
	HashableType        = abcType("Hashable", nil)
	SizedType           = abcType("Sized", nil)
	CallableType        = abcType("Callable", nil)
	IterableType        = abcType("Iterable", nil)
	IteratorType        = abcType("Iterator", IterableType)
	ReversibleType      = abcType("Reversible", IterableType)
	GeneratorType       = abcType("Generator", IteratorType)
	CollectionType      = abcType("Collection", nil)
	SequenceType        = abcType("Sequence", nil)
	MutableSequenceType = abcType("MutableSequence", SequenceType)
	SetType             = abcType("Set", nil)
	MutableSetType      = abcType("MutableSet", SetType)
	MappingType         = abcType("Mapping", nil)
	MutableMappingType  = abcType("MutableMapping", MappingType)
	MappingViewType     = abcType("MappingView", nil)
	KeysViewType        = abcType("KeysView", MappingViewType)
	ItemsViewType       = abcType("ItemsView", MappingViewType)
	ValuesViewType      = abcType("ValuesView", MappingViewType)
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

func init() {
	for _, t := range all {
		subscriptable(t)
		// The abstract base classes exist to be derived from.
		t.Flags |= py.TPFLAGS_BASETYPE
	}

	// Structural conformance.  Every test is a method presence check, which
	// is how Python's own ABCs behave for the methods they declare abstract.
	py.ABCHooks = append(py.ABCHooks, func(obj py.Object, class *py.Type) bool {
		// Types are not instances of the ABCs; only their instances are.
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
