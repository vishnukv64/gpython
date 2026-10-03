// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builtin

import (
	"reflect"
	"sort"

	"github.com/vishnukv64/gpython/py"
)

const callable_doc = `Return True if the object argument appears callable, False if not.

If this returns True, it is still possible that a call fails, but if it is
False, calling object will never succeed.`

func builtin_callable(self, obj py.Object) (py.Object, error) {
	// A type is callable (it constructs instances), as is anything with a
	// __call__ method or one supplied by its type.
	if _, ok := obj.(*py.Type); ok {
		return py.True, nil
	}
	if _, ok := obj.(py.I__call__); ok {
		return py.True, nil
	}
	if obj.Type().Lookup("__call__") != nil {
		return py.True, nil
	}
	return py.False, nil
}

const id_doc = `Return the identity of an object.

This is guaranteed to be unique among simultaneously existing objects.  It is
derived from the object's address where it has one, and from a counter for the
value types (int, str, float) that have no address of their own.`

// lastID backs `id` for the object shapes that are not pointers.
var lastID int64

func builtin_id(self, obj py.Object) (py.Object, error) {
	if obj == nil {
		return py.Int(0), nil
	}
	v := reflect.ValueOf(obj)
	switch v.Kind() {
	case reflect.Ptr, reflect.UnsafePointer, reflect.Chan, reflect.Map, reflect.Func, reflect.Slice:
		if !v.IsNil() {
			return py.Int(int64(v.Pointer())), nil
		}
	}
	lastID++
	return py.Int(lastID), nil
}

const hash_doc = `Return the hash value of the object (if it has one).

Hash values are integers.  Two objects that compare equal must have the same
hash value.`

func builtin_hash(self, obj py.Object) (py.Object, error) {
	// The concrete types with a natural hash - a tuple, an int, a string, and
	// an instance of a subclass carrying one - are hashed from their VALUE,
	// through the same encoder a dict key takes.  This comes FIRST: a tuple
	// defines no __hash__ of its own, so the lookup below found object's
	// identity descriptor and raised "descriptor '__hash__' requires a 'type'
	// object".
	if n, ok := py.HashValue(obj); ok {
		return py.Int(n), nil
	}
	// An explicit __hash__ wins, and a __hash__ of None makes the object
	// unhashable.
	if h, ok := obj.(py.I__hash__); ok {
		return h.M__hash__()
	}

	// The concrete types that have a natural hash, before consulting the
	// type's namespace: the value types do not register __hash__ itself.
	switch v := obj.(type) {
	case py.NoneType:
		return py.Int(0), nil
	case py.Bool:
		if v {
			return py.Int(1), nil
		}
		return py.Int(0), nil
	case py.Int:
		return py.Int(int64(v)), nil
	case py.Float:
		return py.Int(int64(float64(v))), nil
	case py.String:
		return py.Int(hashBytes([]byte(v))), nil
	case py.Bytes:
		// bytes is hashable and "hash(b'a')" must work -- it raised
		// "descriptor '__hash__' requires a 'type' object", because bytes has
		// no __hash__ and no case here either.  Equal bytes hash equal, which
		// is what put them in a set or dict key.
		return py.Int(hashBytes([]byte(v))), nil
	case py.Tuple:
		// A TUPLE is hashable and "hash((1, 2))" must work.  It had no case
		// here, so it fell through to the __hash__ lookup, which found object's
		// identity descriptor and raised "descriptor '__hash__' requires a 'type'
		// object" -- while "{(1, 2): 'v'}" worked, because dict keys go through a
		// different encoder.  CPython's rule is order-sensitive: tuples with the
		// same items in a different order hash differently.
		return py.Int(hashObjects(v)), nil
	}

	if obj.Type().Lookup("__hash__") == nil {
		return nil, py.ExceptionNewf(py.TypeError, "unhashable type: '%s'", obj.Type().Name)
	}
	// A __hash__ supplied by the object's own type is invoked through the
	// normal attribute protocol.
	h, err := py.GetAttrString(obj, "__hash__")
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "unhashable type: '%s'", obj.Type().Name)
	}
	return py.Call(h, py.Tuple{}, py.StringDict{})
}

// hashObjects hashes a sequence of objects in order, as CPython's tuple hash
// does - ("a", "b") and ("b", "a") must not collide.
//
// Each element is hashed through hashOf, the same encoder a dict key uses, so
// an element hashes here exactly as it does as a key.
func hashObjects(items py.Tuple) int64 {
	var h int64 = 0x345678
	for _, it := range items {
		k, ok := py.HashValue(it)
		if !ok {
			return 0
		}
		h = (h ^ k) * 1000003
	}
	return h ^ int64(len(items))
}

const issubclass_doc = `Return whether class is a derived class of another class or of any of
the classes in the classinfo tuple.`

func builtin_issubclass(self py.Object, args py.Tuple) (py.Object, error) {
	var cls, classinfo py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "issubclass", 2, 2, &cls, &classinfo); err != nil {
		return nil, err
	}

	class, ok := cls.(*py.Type)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "issubclass() arg 1 must be a class")
	}

	if infos, ok := classinfo.(py.Tuple); ok {
		for _, info := range infos {
			parent, ok := info.(*py.Type)
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "issubclass() arg 2 must be a class or tuple of classes")
			}
			if class.IsSubtype(parent) {
				return py.True, nil
			}
		}
		return py.False, nil
	}

	parent, ok := classinfo.(*py.Type)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "issubclass() arg 2 must be a class or tuple of classes")
	}
	if class.IsSubtype(parent) {
		return py.True, nil
	}
	return py.False, nil
}

const dir_doc = `dir([object]) -> list of strings

If called without an argument, return the names in the current scope.  Else,
return an alphabetized list of names comprising (some of) the attributes of
the given object, and of attributes reachable from it.`

// builtinDir implements dir().  It needs the interpreter frame for the
// no-argument form, so it is reached through InternalMethodDir.
func builtinDir(f *py.Frame, args py.Tuple) (py.Object, error) {
	if len(args) > 1 {
		return nil, py.ExceptionNewf(py.TypeError, "dir expected at most 1 argument, got %d", len(args))
	}

	if len(args) == 0 {
		f.FastToLocals()
		names := make([]string, 0, f.Locals.Len())
		for _, name := range f.Locals.Keys() {
			names = append(names, name)
		}
		sort.Strings(names)
		return namesToList(names), nil
	}

	obj := args[0]
	seen := map[string]bool{}

	// Attributes supplied by the type and everything it inherits.
	for t := obj.Type(); t != nil; t = t.Base {
		for _, name := range t.Dict.Keys() {
			seen[name] = true
		}
	}
	// Attributes carried by the object itself.
	if d, ok := obj.(py.IGetDict); ok {
		for _, name := range d.GetDict().Keys() {
			seen[name] = true
		}
	}
	if m, ok := obj.(*py.Module); ok {
		for _, name := range m.Globals.Keys() {
			seen[name] = true
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return namesToList(names), nil
}

func namesToList(names []string) *py.List {
	items := make([]py.Object, len(names))
	for i, name := range names {
		items[i] = py.String(name)
	}
	return py.NewListFromItems(items)
}

const format_doc = `format(value[, format_spec]) -> string

Return value.__format__(format_spec).`

func builtin_format(self py.Object, args py.Tuple) (py.Object, error) {
	var value py.Object
	spec := py.Object(py.String(""))
	if err := py.UnpackTuple(args, py.StringDict{}, "format", 1, 2, &value, &spec); err != nil {
		return nil, err
	}

	specStr, ok := spec.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "format() argument 2 must be str, not %s", spec.Type().Name)
	}

	// The mini-language itself lives in py, so that str.format and a value's
	// own __format__ share one implementation.
	if v, ok := value.(py.I__format__); ok {
		return v.M__format__(spec)
	}
	return py.Format(value, string(specStr))
}

// hashBytes is the hash of a byte string: FNV-1a, deterministic so that equal
// values hash equal, which is all a dictionary key needs.
//
// It is used for str and bytes alike, so that "hash(b'a') == hash(b'a')" holds
// and a bytes value can be a dict key or set member.
func init() {
	// The builtin module owns the hash of a byte string, and py cannot import
	// it, so memoryview reads it through this hook.
	py.MemoryHash = hashBytes
}

func hashBytes(b []byte) int64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(b); i++ {
		h ^= uint64(b[i])
		h *= 1099511628211
	}
	return int64(h & (1<<63 - 1))
}
