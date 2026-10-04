// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Dict and StringDict type
//
// The idea is that most dicts just have strings for keys so we use
// the simpler StringDict and promote it into a Dict when necessary

package py

import (
	"bytes"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const dictDoc = `dict() -> new empty dictionary
dict(mapping) -> new dictionary initialized from a mapping object's
    (key, value) pairs
dict(iterable) -> new dictionary initialized as if via:
    d = {}
    for k, v in iterable:
        d[k] = v
dict(**kwargs) -> new dictionary initialized with the name=value pairs
    in the keyword argument list.  For example:  dict(one=1, two=2)`

var (
	StringDictType = NewTypeX("dict", dictDoc, DictNew, nil)
	DictType       = NewType("dict", dictDoc)
	expectingDict  = ExceptionNewf(TypeError, "a dict is required")
)

// mappingStorage reports whether an object is a mapping and, if so, its
// storage.  It is the non-erroring counterpart of dictStorage: a caller asking
// "is this a dict-like thing?" wants a bool, not an error.
func mappingStorage(o Object) (StringDict, bool) {
	switch v := o.(type) {
	case StringDict:
		return v, true
	case *Type:
		// gStringDictType rather than StringDictType: this is reached from
		// DictNew, which StringDictType is built from, so naming the type
		// directly is an initialisation cycle.
		if gStringDictType != nil && v.IsSubtype(gStringDictType) {
			return v.GetDict(), true
		}
	}
	return StringDict{}, false
}

// dictStorage returns the mapping behind a dict or a dict SUBCLASS instance.
//
// A plain dict IS a StringDict; an instance of a python-level class is a *Type
// whose namespace is its storage.  Both are a StringDict, so the methods below
// share one implementation - and returning an error rather than asserting
// means a misuse reports instead of panicking.
func dictStorage(self Object) (StringDict, error) {
	switch o := self.(type) {
	case StringDict:
		return o, nil
	case *Type:
		return o.GetDict(), nil
	}
	return StringDict{}, ExceptionNewf(TypeError, "'%s' object is not a dict", self.Type().Name)
}

func init() {
	// The holder DictNew reads, set here where StringDictType is available.
	gStringDictType = StringDictType

	// Subclassable, as in CPython: requests derives LookupDict(dict),
	// collections derives OrderedDict(dict) and deque-adjacent types, and
	// ordinary code subclasses list and set.  See the note by the
	// subscript dunders below for what a subclass must inherit.
	StringDictType.Flags |= TPFLAGS_BASETYPE
	// The subscript operators as METHODS, so a subclass can inherit them.
	//
	// dict implemented __getitem__/__setitem__/__delitem__ as methods on the
	// Go value, which the interpreter reaches through the I__setitem__
	// interface - fine for a plain dict, but a SUBCLASS cannot be given a Go
	// interface, so "class D(dict): pass; d['a'] = 1" raised "'D' object does
	// not support item assignment" once dict became subclassable.
	//
	// self is either a StringDict - a plain dict - or a *Type, which is how
	// this interpreter represents an instance of a python-level class.  A
	// *Type has its own Dict, so the same code serves both; asDictOf picks
	// the storage and never asserts.
	asDictOf := func(self Object) (StringDict, error) {
		switch o := self.(type) {
		case StringDict:
			return o, nil
		case *Type:
			// An instance of a python-level class: its namespace is its
			// storage, which is exactly a dict.
			return o.GetDict(), nil
		}
		return StringDict{}, ExceptionNewf(TypeError,
			"'%s' object is not a dict", self.Type().Name)
	}
	StringDictType.Dict.Set("__getitem__", MustNewMethod("__getitem__", func(self Object, args Tuple) (Object, error) {
		var key Object
		if err := UnpackTuple(args, StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		d, err := asDictOf(self)
		if err != nil {
			return nil, err
		}
		return d.M__getitem__(key)
	}, 0, "Return self[key]."))
	StringDictType.Dict.Set("__setitem__", MustNewMethod("__setitem__", func(self Object, args Tuple) (Object, error) {
		var key, value Object
		if err := UnpackTuple(args, StringDict{}, "__setitem__", 2, 2, &key, &value); err != nil {
			return nil, err
		}
		d, err := asDictOf(self)
		if err != nil {
			return nil, err
		}
		return d.M__setitem__(key, value)
	}, 0, "Set self[key] to value."))
	StringDictType.Dict.Set("__delitem__", MustNewMethod("__delitem__", func(self Object, args Tuple) (Object, error) {
		var key Object
		if err := UnpackTuple(args, StringDict{}, "__delitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		d, err := asDictOf(self)
		if err != nil {
			return nil, err
		}
		return d.M__delitem__(key)
	}, 0, "Delete self[key]."))
	StringDictType.Dict.Set("__len__", MustNewMethod("__len__", func(self Object, args Tuple) (Object, error) {
		d, err := asDictOf(self)
		if err != nil {
			return Int(0), nil
		}
		return Int(d.Len()), nil
	}, 0, "Return len(self)."))
	StringDictType.Dict.Set("__contains__", MustNewMethod("__contains__", func(self Object, args Tuple) (Object, error) {
		var key Object
		if err := UnpackTuple(args, StringDict{}, "__contains__", 1, 1, &key); err != nil {
			return nil, err
		}
		d, err := asDictOf(self)
		if err != nil {
			return False, nil
		}
		return d.M__contains__(key)
	}, 0, "Return key in self."))
	// repr/str as METHODS too, so an instance of a dict subclass prints as a
	// dict rather than falling through to the generic "<D object at 0x...>".
	StringDictType.Dict.Set("__repr__", MustNewMethod("__repr__", func(self Object, args Tuple) (Object, error) {
		d, err := dictStorage(self)
		if err != nil {
			return nil, err
		}
		return d.M__repr__()
	}, 0, "Return repr(self)."))
	StringDictType.Dict.Set("__str__", MustNewMethod("__str__", func(self Object, args Tuple) (Object, error) {
		d, err := dictStorage(self)
		if err != nil {
			return nil, err
		}
		return d.M__repr__()
	}, 0, "Return str(self)."))

	StringDictType.Dict.Set("__iter__", MustNewMethod("__iter__", func(self Object, args Tuple) (Object, error) {
		d, err := asDictOf(self)
		if err != nil {
			return nil, err
		}
		return d.M__iter__()
	}, 0, "Implement iter(self)."))

	StringDictType.Dict.Set("items", MustNewMethod("items", func(self Object, args Tuple) (Object, error) {
		err := UnpackTuple(args, NewStringDict(), "items", 0, 0)
		if err != nil {
			return nil, err
		}
		sMap, _ := dictStorage(self)
		o := make(Tuple, 0, sMap.Len())
		var itemsErr error
		sMap.Range(func(k string, v Object) bool {
			key, err := sMap.DecodeKey(k)
			if err != nil {
				itemsErr = err
				return true
			}
			o = append(o, Tuple{key, v})
			return false
		})
		if itemsErr != nil {
			return nil, itemsErr
		}
		return NewIterator(o), nil
	}, 0, "items() -> list of D's (key, value) pairs, as 2-tuples"))

	StringDictType.Dict.Set("keys", MustNewMethod("keys", func(self Object, args Tuple) (Object, error) {
		err := UnpackTuple(args, NewStringDict(), "keys", 0, 0)
		if err != nil {
			return nil, err
		}
		sMap, _ := dictStorage(self)
		o := make(Tuple, 0, sMap.Len())
		for _, k := range sMap.Keys() {
			key, err := sMap.DecodeKey(k)
			if err != nil {
				return nil, err
			}
			o = append(o, key)
		}
		return NewIterator(o), nil
	}, 0, "keys() -> list of D's keys, as a list"))

	StringDictType.Dict.Set("values", MustNewMethod("values", func(self Object, args Tuple) (Object, error) {
		err := UnpackTuple(args, NewStringDict(), "values", 0, 0)
		if err != nil {
			return nil, err
		}
		sMap, _ := dictStorage(self)
		o := make(Tuple, 0, sMap.Len())
		for _, v := range sMap.Values() {
			o = append(o, v)
		}
		return NewIterator(o), nil
	}, 0, "values() -> list of D's values, as a list"))

	StringDictType.Dict.Set("get", MustNewMethod("get", func(self Object, args Tuple) (Object, error) {
		sMap, _ := dictStorage(self)
		if len(args) < 1 {
			return nil, ExceptionNewf(TypeError, "get expected at least 1 argument, got %d", len(args))
		}
		if len(args) > 2 {
			return nil, ExceptionNewf(TypeError, "get expected at most 2 arguments, got %d", len(args))
		}
		encoded, ok, err := sMap.keyCode(args[0])
		if err != nil {
			return nil, err
		}
		if ok {
			return sMap.GetOrNil(encoded), nil
		}
		if len(args) == 2 {
			return args[1], nil
		}
		return None, nil
	}, 0, "get(key[, default]) -> value for key if key is in the dictionary, else default (None by default)."))

	StringDictType.Dict.Set("pop", MustNewMethod("pop", func(self Object, args Tuple) (Object, error) {
		d, _ := dictStorage(self)
		if len(args) < 1 {
			return nil, ExceptionNewf(TypeError, "pop expected at least 1 argument, got %d", len(args))
		}
		if len(args) > 2 {
			return nil, ExceptionNewf(TypeError, "pop expected at most 2 arguments, got %d", len(args))
		}
		// CPython answers an empty dict before it ever hashes the key, so
		// the default is returned (and an unhashable key is not complained
		// about) when there is nothing to pop.
		if d.Len() == 0 {
			if len(args) == 2 {
				return args[1], nil
			}
			return nil, ExceptionNewf(KeyError, "%v", args[0])
		}
		encoded, ok, err := d.keyCode(args[0])
		if err != nil {
			return nil, err
		}
		if ok {
			res := d.GetOrNil(encoded)
			d.Del(encoded)
			return res, nil
		}
		if len(args) == 2 {
			return args[1], nil
		}
		return nil, ExceptionNewf(KeyError, "%v", args[0])
	}, 0, "pop(key[, default]) -> value -- remove specified key and return the corresponding value."))

	StringDictType.Dict.Set("popitem", MustNewMethod("popitem", func(self Object, args Tuple) (Object, error) {
		d, _ := dictStorage(self)
		if err := methodNoArgs("dict.popitem", args); err != nil {
			return nil, err
		}
		// dict.popitem() removes and returns the most recently inserted pair,
		// which is the LAST one in insertion order.
		keys := d.Keys()
		if len(keys) == 0 {
			return nil, ExceptionNewf(KeyError, "%v", "popitem(): dictionary is empty")
		}
		k := keys[len(keys)-1]
		v, _ := d.Get(k)
		// Decode BEFORE removing: for a key defining __hash__, the entry's
		// object lives in the table Del is about to forget.
		key, err := d.DecodeKey(k)
		if err != nil {
			return nil, err
		}
		d.Del(k)
		return Tuple{key, v}, nil
	}, 0, "popitem() -> (k, v) -- remove and return some (key, value) pair as a 2-tuple."))

	StringDictType.Dict.Set("setdefault", MustNewMethod("setdefault", func(self Object, args Tuple) (Object, error) {
		d, _ := dictStorage(self)
		if len(args) < 1 {
			return nil, ExceptionNewf(TypeError, "setdefault expected at least 1 argument, got %d", len(args))
		}
		if len(args) > 2 {
			return nil, ExceptionNewf(TypeError, "setdefault expected at most 2 arguments, got %d", len(args))
		}
		encoded, ok, err := d.keyCode(args[0])
		if err != nil {
			return nil, err
		}
		if ok {
			return d.GetOrNil(encoded), nil
		}
		var deflt Object = None
		if len(args) == 2 {
			deflt = args[1]
		}
		if err := d.setItem(args[0], deflt); err != nil {
			return nil, err
		}
		return deflt, nil
	}, 0, "setdefault(key[, default]) -> value -- return value if key is in the dictionary, else insert and return default."))

	StringDictType.Dict.Set("update", MustNewMethod("update", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		d, _ := dictStorage(self)
		if len(args) > 1 {
			return nil, ExceptionNewf(TypeError, "update expected at most 1 argument, got %d", len(args))
		}
		if len(args) == 1 {
			if err := dictUpdateFrom(d, args[0]); err != nil {
				return nil, err
			}
		}
		// Keyword arguments are name=value pairs; the names are Python
		// identifiers, so they are stored verbatim, exactly as dict()
		// stores its own keyword arguments.
		kwargs.Range(func(k string, v Object) bool {
			d.Set(k, v)
			return false
		})
		return None, nil
	}, 0, "update([other]) -> None.  Update D from a dict/iterable of key/value pairs and keywords."))

	StringDictType.Dict.Set("clear", MustNewMethod("clear", func(self Object, args Tuple) (Object, error) {
		d, _ := dictStorage(self)
		if err := methodNoArgs("dict.clear", args); err != nil {
			return nil, err
		}
		// The entries are removed in place rather than the dict being replaced,
		// because a StringDict is copied by value and every copy shares this
		// same storage.
		d.Clear()
		return None, nil
	}, 0, "clear() -> None.  Remove all items from the dictionary."))

	StringDictType.Dict.Set("copy", MustNewMethod("copy", func(self Object, args Tuple) (Object, error) {
		d, _ := dictStorage(self)
		if err := methodNoArgs("dict.copy", args); err != nil {
			return nil, err
		}
		return d.Copy(), nil
	}, 0, "copy() -> a shallow copy of the dictionary."))

	// dict.fromkeys is a class method in CPython: it is looked up on the
	// type (dict.fromkeys) as well as on an instance ({}.fromkeys).  In both
	// forms the arguments arrive unshifted - self is the dict for the bound
	// form and the module for the unbound one - so unlike the other methods
	// this one ignores self entirely.
	StringDictType.Dict.Set("fromkeys", MustNewMethod("fromkeys", func(self Object, args Tuple) (Object, error) {
		if len(args) < 1 {
			return nil, ExceptionNewf(TypeError, "fromkeys expected at least 1 argument, got %d", len(args))
		}
		if len(args) > 2 {
			return nil, ExceptionNewf(TypeError, "fromkeys expected at most 2 arguments, got %d", len(args))
		}
		var value Object = None
		if len(args) == 2 {
			value = args[1]
		}
		out := NewStringDict()
		var loopErr error
		err := Iterate(args[0], func(key Object) bool {
			if err := out.setItem(key, value); err != nil {
				loopErr = err
				return true
			}
			return false
		})
		if err != nil {
			return nil, err
		}
		if loopErr != nil {
			return nil, loopErr
		}
		return out, nil
	}, METH_CLASS, "fromkeys(iterable, value=None, /) -> New dict with keys from iterable and values equal to value."))
}

// dictUpdateFrom implements the body shared by dict.update() and
// dict.__ior__().
//
// other may be a mapping - anything with a keys() method, which is how
// CPython tells the two forms apart - or an iterable of key/value pairs.
func dictUpdateFrom(d StringDict, other Object) error {
	if src, ok := other.(StringDict); ok {
		// Range snapshots the keys, so "d.update(d)" is safe.  A key with a
		// __hash__ is re-filed rather than copied verbatim: its code belongs
		// to the source's table.
		var copyErr error
		src.Range(func(k string, v Object) bool {
			if err := d.copyEntryFrom(src, k, v); err != nil {
				copyErr = err
				return true
			}
			return false
		})
		return copyErr
	}
	if keysFn, err := GetAttrString(other, "keys"); err == nil {
		keys, err := Call(keysFn, nil, NewStringDict())
		if err != nil {
			return err
		}
		var loopErr error
		err = Iterate(keys, func(key Object) bool {
			value, err := GetItem(other, key)
			if err != nil {
				loopErr = err
				return true
			}
			if err := d.setItem(key, value); err != nil {
				loopErr = err
				return true
			}
			return false
		})
		if err == nil {
			err = loopErr
		}
		return err
	}
	index := 0
	var loopErr error
	err := Iterate(other, func(item Object) bool {
		var pair []Object
		err := Iterate(item, func(o Object) bool {
			pair = append(pair, o)
			return false
		})
		if err != nil {
			loopErr = ExceptionNewf(TypeError, "object is not iterable")
			return true
		}
		if len(pair) != 2 {
			loopErr = ExceptionNewf(ValueError, "dictionary update sequence element #%d has length %d; 2 is required", index, len(pair))
			return true
		}
		if err := d.setItem(pair[0], pair[1]); err != nil {
			loopErr = err
			return true
		}
		index++
		return false
	})
	if err == nil {
		err = loopErr
	}
	return err
}

// String to object dictionary
//
// Used for variables etc where the keys can only be strings.
//
// Python has guaranteed that a dict keeps its insertion order since 3.7, so a
// Go map - whose iteration order is randomised - is not enough on its own.
// The entries are therefore kept twice: in the map, for lookup, and in a list
// of their encoded keys in the order they were first inserted.
//
// The order list is shared through a pointer because a StringDict is copied
// by value everywhere (Go maps are references, so the map itself needs no such
// care): every copy has to see the one order, including appends made through
// a copy.  The table of keys that define __hash__ is mutable state of the same
// kind, so it lives behind the same pointer instead of a field of its own.  A
// StringDict built by NewStringDict always has it set; the zero value reads as
// an empty dict, as the bare nil map did.
type dictShared struct {
	// order holds the encoded keys in the order they were first inserted,
	// which is what gives a Python dict its guaranteed insertion order.
	order []string
	// ht resolves a key whose type defines __hash__ to the one slot holding
	// it.  A dict of plain string keys - nearly all of them - never touches
	// it, and its two maps stay nil.
	ht hashTable
}

type StringDict struct {
	m      map[string]Object
	shared *dictShared
}

// Ptr returns a value identifying this dict's storage, so that tooling such as
// pprint can tell one dict from another.  reflect.Value.Pointer cannot be
// called on a struct, which is what a StringDict now is - it could only be
// called on the map before.
func (d StringDict) Ptr() uintptr {
	return reflect.ValueOf(d.m).Pointer()
}

// Get returns the value stored under an encoded key and whether it is present.
func (d StringDict) Get(key string) (Object, bool) {
	v, ok := d.m[key]
	return v, ok
}

// SameAs reports whether two dicts are the SAME dict, not merely equal.
//
// It exists so a frame can skip a redundant lookup: at module scope Locals and
// Globals are one namespace, so a miss in Locals has already searched Globals
// and searching it again costs a hash and a string compare per name read.
//
// The storage pointer is the identity - two StringDict values that share an
// order slice share the same map - and comparing it is one word.
func (d StringDict) SameAs(other StringDict) bool {
	return d.shared == other.shared
}

// GetOrNil returns the value stored under an encoded key, or Go nil when the
// key is absent.  It is the exact equivalent of a Go map read d[key] on the
// old map-typed StringDict, for the few places that relied on that - notably
// comparing the result against nil to test for a key's presence.
func (d StringDict) GetOrNil(key string) Object {
	return d.m[key]
}

// IsNil reports whether the dict has never been given storage, which is what
// a nil map used to mean: NewStringDict always allocates, so only the zero
// value or an explicit NewStringDict() is nil here.  Several places used
// "dict == nil" to mean "not set up yet", and this is its replacement.
func (d StringDict) IsNil() bool {
	return d.m == nil
}

// Set stores value under an encoded key, keeping the position of a key that is
// already present.  A key that was deleted and is set again moves to the end,
// which is what CPython does.
//
// It has a pointer receiver so that the zero StringDict - whose map is nil -
// allocates its storage on first write instead of the write being lost.  A
// dict built by NewStringDict already has both fields set, so the common path
// only writes the shared map.
func (d *StringDict) Set(key string, value Object) {
	if d.m == nil {
		d.m = make(map[string]Object)
	}
	shared := d.shared
	if shared == nil {
		shared = &dictShared{}
		d.shared = shared
	}
	// One hash, not two.
	//
	// This used to read the map to test for the key and then write it, which
	// hashes and compares the name twice on EVERY assignment.  STORE_NAME is
	// one of the hottest opcodes there is - "i = i + 1" in a loop is a store
	// per iteration - and a profile of a module-level loop put
	// mapassign_faststr at 9% of samples for exactly this reason.
	//
	// Assigning first and testing the old value for nil gives the same answer
	// with a single hash.  A nil Object can never have been stored as a
	// value - the interpreter has no nil value, None is a real type - so the
	// test is unambiguous.
	prev := d.m[key]
	d.m[key] = value
	if prev == nil {
		// The key is new, so it takes its place at the END of the order.
		// Re-assigning an existing key must keep its original position, which
		// is why this is not unconditional.
		shared.order = append(shared.order, key)
	}
}

// Del removes an encoded key, reporting whether it was present.
func (d *StringDict) Del(key string) bool {
	if _, ok := d.m[key]; !ok {
		return false
	}
	delete(d.m, key)
	keys := d.shared.order
	for i, k := range keys {
		if k == key {
			d.shared.order = append(keys[:i], keys[i+1:]...)
			break
		}
	}
	// A key whose type defines __hash__ also owns a slot in the bucket table.
	d.shared.ht.forget(key)
	return true
}

// sharedState returns the dict's shared bookkeeping, allocating it for the
// zero StringDict so that a write through a copy is still visible to the
// original.
func (d *StringDict) sharedState() *dictShared {
	if d.shared == nil {
		d.shared = &dictShared{}
	}
	return d.shared
}

// hashSet stores value under a key whose type defines __hash__.  bucket is the
// key's encoded form, which groups together every key sharing its type and
// hash; the entry is stored under a code of its own, so that two different
// objects with the same hash are two entries.  __eq__ decides which entry - if
// any - the key is, exactly as CPython's bucket walk does.
func (d *StringDict) hashSet(bucket string, key, value Object) error {
	shared := d.sharedState()
	code, found, err := shared.ht.find(bucket, key)
	if err != nil {
		return err
	}
	if !found {
		code = shared.ht.newCode(bucket)
		shared.ht.objs[code] = key
	}
	d.Set(code, value)
	return nil
}

// keyCode returns the code the dict stores an object under, and whether it is
// already present.  For everything but a key defining __hash__ the code is the
// key's encoded form; for such a key it is the code of the entry it is equal
// to, which only the bucket walk can find.
func (d StringDict) keyCode(key Object) (string, bool, error) {
	encoded, err := dictKey(key)
	if err != nil {
		return "", false, err
	}
	if !isHashKey(encoded) {
		_, ok := d.m[encoded]
		return encoded, ok, nil
	}
	if d.shared == nil {
		return "", false, nil
	}
	code, ok, err := d.shared.ht.find(encoded, key)
	if err != nil || !ok {
		return "", false, err
	}
	_, ok = d.m[code]
	return code, ok, nil
}

// setItem stores value under key.  It is dict.__setitem__'s whole body and the
// one place that has to tell a hash bucket from an ordinary encoding.
func (d StringDict) setItem(key, value Object) error {
	encoded, err := dictKey(key)
	if err != nil {
		return err
	}
	if isHashKey(encoded) {
		return d.hashSet(encoded, key, value)
	}
	d.Set(encoded, value)
	return nil
}

// DecodeKey recovers the object an encoded key stands for.
//
// For everything but a key defining __hash__ the encoding is reversible on its
// own and DictKeyDecode would do.  A hash key is stored under a code whose
// object lives in this dict's table - hashing is not reversible - so it takes
// the dict to decode it.  See hashTable.
func (d StringDict) DecodeKey(encoded string) (Object, error) {
	if isHashKey(encoded) && d.shared != nil {
		if obj, ok := d.shared.ht.objs[encoded]; ok {
			return obj, nil
		}
	}
	return dictKeyDecode(encoded)
}

// copyEntryFrom copies one of src's entries - given as its code and value -
// into d.  A builtin key's code is canonical and can be stored as it stands; a
// hash key's code belongs to src's table, so the key object has to be recovered
// and filed anew in d.
func (d *StringDict) copyEntryFrom(src StringDict, code string, value Object) error {
	if !isHashKey(code) {
		d.Set(code, value)
		return nil
	}
	key, err := src.DecodeKey(code)
	if err != nil {
		return err
	}
	return d.setItem(key, value)
}

// Has reports whether an encoded key is present.
func (d StringDict) Has(key string) bool {
	_, ok := d.m[key]
	return ok
}

// Len returns the number of entries.
func (d StringDict) Len() int {
	return len(d.m)
}

// Keys returns the encoded keys in insertion order.
func (d StringDict) Keys() []string {
	if d.shared == nil {
		return nil
	}
	out := make([]string, len(d.shared.order))
	copy(out, d.shared.order)
	return out
}

// Values returns the values in insertion order.
func (d StringDict) Values() []Object {
	if d.shared == nil {
		return nil
	}
	order := d.shared.order
	out := make([]Object, len(order))
	for i, k := range order {
		out[i] = d.m[k]
	}
	return out
}

// Items returns the entries in insertion order as a slice, so callers can
// write an ordinary range loop - with continue, return and goto - instead of a
// Range callback.
func (d StringDict) Items() []DictEntry {
	if d.shared == nil {
		return nil
	}
	out := make([]DictEntry, 0, len(d.shared.order))
	for _, k := range d.shared.order {
		out = append(out, DictEntry{Key: k, Value: d.m[k]})
	}
	return out
}

// Range calls fn for each entry in insertion order, stopping early when fn
// returns true.  The keys are snapshotted first, so fn may delete entries.
func (d StringDict) Range(fn func(key string, value Object) bool) {
	for _, k := range d.Keys() {
		v, ok := d.m[k]
		if !ok {
			continue // deleted by fn
		}
		if fn(k, v) {
			return
		}
	}
}

// Clear removes every entry, keeping the dict object itself in place.
func (d *StringDict) Clear() {
	if d.m == nil {
		return
	}
	for k := range d.m {
		delete(d.m, k)
	}
	d.shared.order = d.shared.order[:0]
	d.shared.ht = hashTable{}
}

// DictEntry is one key/value pair for NewStringDictFrom.
type DictEntry struct {
	Key   string
	Value Object
}

// NewStringDictFrom builds a StringDict from the listed pairs, in the order
// they are given.  The keys are already-encoded dict keys, which for the
// module globals and method tables that use this builder are plain strings.
// It exists because a composite literal cannot reach the unexported order
// field, so a literal such as a Go map literal with two entries - whose order
// the map does not preserve - has no ordered composite equivalent.
func NewStringDictFrom(entries ...DictEntry) StringDict {
	d := NewStringDictSized(len(entries))
	for _, e := range entries {
		d.Set(e.Key, e.Value)
	}
	return d
}

// Python dicts accept any hashable key, but the storage here is keyed by
// string.  Keys are therefore encoded into a reversible string form.  A
// String key that does not begin with keyTag is stored as itself, which
// keeps the module globals, keyword arguments and every existing literal
// dictionary working with no encoding at all; everything else is stored
// with keyTag and a one byte type marker.
const keyTag = "\x00"

// hashSep separates an entry's own code from the hash bucket it was derived
// from.  It cannot occur in a bucket, whose type name is an identifier.
const hashSep = "\x01"

// isHashKey reports whether an encoded key is a hash bucket rather than a key
// that identifies its own entry.  The bucket is what a key whose type defines
// __hash__ encodes to.
func isHashKey(encoded string) bool {
	return len(encoded) >= 2 && encoded[0] == keyTag[0] && encoded[1] == 'H'
}

// hashTable is a dict's bookkeeping for the keys whose type defines __hash__.
//
// Such a key cannot be encoded into a string that both finds an equal object
// and tells two different objects with the same hash apart: the first needs
// the encoding to depend only on the hash, the second needs it not to.  The
// hash plus the type name is therefore stored as a BUCKET - which is what
// CPython has too, where equal hashes share a bucket and __eq__ decides - and
// each entry the dict actually holds is given a code of its own, the bucket
// followed by that entry's number.  The object the code stands for is kept
// here, because nothing else can recover it: that is what hashing does.
//
// A dict of plain keys - nearly all of them - never allocates any of this.
type hashTable struct {
	// buckets holds, per bucket, the codes of the entries in it.
	buckets map[string][]string
	// objs maps a code to the object it stands for, so that keys(), items(),
	// repr(), iteration and popitem can hand the original back.
	objs map[string]Object
	// next numbers the codes.  It numbers them per dict rather than by object
	// identity, which the encoding is not allowed to depend on.
	next int
}

// newCode gives an entry in bucket a code of its own, and records it there.
func (h *hashTable) newCode(bucket string) string {
	if h.buckets == nil {
		h.buckets = make(map[string][]string)
		h.objs = make(map[string]Object)
	}
	code := bucket + hashSep + strconv.Itoa(h.next)
	h.next++
	h.buckets[bucket] = append(h.buckets[bucket], code)
	return code
}

// find returns the code of the entry equal to obj among the entries that
// share obj's bucket, and whether there is one.  Equal hashes are only a
// bucket: __eq__ is what says whether it is the same key.
func (h *hashTable) find(bucket string, obj Object) (string, bool, error) {
	for _, code := range h.buckets[bucket] {
		other := h.objs[code]
		if other == nil {
			continue
		}
		eq, err := objEq(obj, other)
		if err != nil {
			return "", false, err
		}
		if eq {
			return code, true, nil
		}
	}
	return "", false, nil
}

// objEq reports whether two key objects are equal, calling each side's Python
// __eq__ in turn.
//
// Eq does not do that: it dispatches only methods implemented in Go, so a
// class written in Python - the ordinary case for a custom __hash__ - compared
// by IDENTITY, and two equal K(1)s were never found equal.  A hash bucket walk
// is exactly where that matters, so the Python-level __eq__ is tried here, the
// same way callPyOrdering tries the ordering operators.
func objEq(a, b Object) (bool, error) {
	if res, ok, err := callPyOrdering(a, "__eq__", b); err != nil {
		return false, err
	} else if ok {
		return ObjectIsTrue(res)
	}
	if res, ok, err := callPyOrdering(b, "__eq__", a); err != nil {
		return false, err
	} else if ok {
		return ObjectIsTrue(res)
	}
	res, err := Eq(a, b)
	if err != nil {
		return false, err
	}
	return ObjectIsTrue(res)
}

// forget drops the bookkeeping of the entry with this code.  The bucket is
// recovered from the code itself, so a caller that only has the code - Del,
// which is handed one by pop and popitem - need not carry it along.
func (h *hashTable) forget(code string) {
	if h.objs == nil {
		return
	}
	delete(h.objs, code)
	i := strings.LastIndexByte(code, hashSep[0])
	if i < 0 {
		return
	}
	bucket := code[:i]
	codes := h.buckets[bucket]
	for j, c := range codes {
		if c == code {
			h.buckets[bucket] = append(codes[:j], codes[j+1:]...)
			break
		}
	}
}

// clone copies the table and the maps in it, so that the copy can be mutated
// without touching the original.
func (h hashTable) clone() hashTable {
	if h.objs == nil {
		return hashTable{}
	}
	c := hashTable{
		buckets: make(map[string][]string, len(h.buckets)),
		objs:    make(map[string]Object, len(h.objs)),
		next:    h.next,
	}
	for b, codes := range h.buckets {
		c.buckets[b] = append([]string(nil), codes...)
	}
	for code, obj := range h.objs {
		c.objs[code] = obj
	}
	return c
}

// Encode an object as a dict key.
//
// Structurally encoded types (str, int, float, bool, None, bytes, tuple,
// frozenset) are reversible, so keys() and items() can return the original
// objects.  Types with identity hashing are not reversible and raise, which
// is the honest answer: it is also what Python does for the unhashable
// containers.
func dictKey(key Object) (string, error) {
	var b []byte
	if err := appendKey(&b, key); err != nil {
		return "", err
	}
	return string(b), nil
}

func appendKey(b *[]byte, key Object) error {
	// A payload-carrying instance encodes as the container it carries, so
	// "d[T((1,2))]" finds the entry "d[(1, 2)]" stored - the two are equal and
	// hash alike, and a key must encode alike to be found.
	if payload, ok := payloadOf(key); ok {
		key = payload
	}
	switch k := key.(type) {
	case String:
		s := string(k)
		if !strings.HasPrefix(s, keyTag) {
			// The common case: stored verbatim, no encoding.
			*b = append(*b, s...)
			return nil
		}
		*b = append(*b, keyTag...)
		*b = append(*b, 's')
		*b = append(*b, s...)
	case Int:
		*b = append(*b, keyTag...)
		*b = append(*b, 'i')
		*b = strconv.AppendInt(*b, int64(k), 10)
	case *BigInt:
		*b = append(*b, keyTag...)
		*b = append(*b, 'I')
		*b = append(*b, []byte((*big.Int)(k).String())...)
	case Bool:
		// A bool IS an int for hashing: True == 1 and hash(True) == hash(1),
		// so "d = {True: 'a'}; d[1] = 'b'" must leave ONE entry and
		// "{1: 'x'}.get(True)" must find it.  Encoding bool under a tag of its
		// own made them different keys, which gave two entries and a None.
		//
		// It is encoded as the int it equals, so the two collapse.
		*b = append(*b, keyTag...)
		*b = append(*b, 'i')
		if k {
			*b = append(*b, '1')
		} else {
			*b = append(*b, '0')
		}
	case Float:
		*b = append(*b, keyTag...)
		*b = append(*b, 'f')
		*b = strconv.AppendUint(*b, math.Float64bits(float64(k)), 16)
	case NoneType:
		*b = append(*b, keyTag...)
		*b = append(*b, 'n')
	case Bytes:
		*b = append(*b, keyTag...)
		*b = append(*b, 'y')
		*b = strconv.AppendInt(*b, int64(len(k)), 10)
		*b = append(*b, ':')
		*b = append(*b, []byte(k)...)
	case Tuple:
		*b = append(*b, keyTag...)
		*b = append(*b, 't')
		*b = strconv.AppendInt(*b, int64(len(k)), 10)
		*b = append(*b, ':')
		for _, item := range k {
			var sub []byte
			if err := appendKey(&sub, item); err != nil {
				return err
			}
			*b = strconv.AppendInt(*b, int64(len(sub)), 10)
			*b = append(*b, ':')
			*b = append(*b, sub...)
		}
	case *Module:
		// A module encodes as its identity - the address of its own globals -
		// which is how it HASHES, so the stored key and the looked-up key agree.
		// Leaving it to the __hash__ fallback made the two disagree, because
		// the encoding there prefixes the type name to the hash and the two
		// forms did not match: "d[sys] = 1; d[sys]" raised KeyError.
		*b = append(*b, keyTag...)
		*b = append(*b, 'm')
		*b = strconv.AppendUint(*b, uint64(k.Globals.Ptr()), 16)
	case *Function, *Method:
		// A function and a method encode as their identity, for the same
		// reason: two distinct functions are two distinct keys.
		*b = append(*b, keyTag...)
		*b = append(*b, 'F')
		*b = strconv.AppendUint(*b, uint64(identityHash(k)), 16)
	case *FrozenSet:
		// Sets have no order, so sort the member encodings to give equal
		// sets the same key.
		// k.items is already keyed by the encoded form, so the keys ARE the
		// encoded members; re-encoding them would double-encode.  They are
		// sorted, because Go map iteration is random and the encoding must be
		// stable for two equal frozensets to hash alike.
		//
		// A member whose type defines __hash__ is stored under a code of its
		// own, and that code is private to the set that made it - so it would
		// make two equal frozensets encode differently.  The member's BUCKET
		// (type and hash) is used instead, which is the same in every set.
		// Two members sharing a hash therefore collapse within one frozenset,
		// which is the pre-existing limit of an encoding that cannot hold an
		// object; a frozenset of such members is not a working dict key either
		// way, because decoding one is not reversible.
		members := make([]string, 0, len(k.items))
		for item := range k.items {
			if isHashKey(item) {
				if i := strings.LastIndexByte(item, hashSep[0]); i >= 0 {
					item = item[:i]
				}
			}
			members = append(members, item)
		}
		sort.Strings(members)
		*b = append(*b, keyTag...)
		*b = append(*b, 'S')
		*b = strconv.AppendInt(*b, int64(len(members)), 10)
		*b = append(*b, ':')
		for _, m := range members {
			*b = strconv.AppendInt(*b, int64(len(m)), 10)
			*b = append(*b, ':')
			*b = append(*b, m...)
		}
	default:
		// Anything else is hashable only if it says so.  Rejecting outright meant
		// that EVERY type defining __hash__ was unusable as a dict key or set
		// member: pathlib.Path hashes to a string and is used as a key all over
		// ordinary code, and "{Path('/a'): 1}" raised "unhashable type:
		// 'pathlib.PosixPath'".
		//
		// The value of __hash__ is stored alongside the type name, so two objects
		// of the same type with the same hash share a bucket - which is exactly
		// what CPython's hash/eq protocol does.  A type with no __hash__, or one
		// whose __hash__ is None (explicitly unhashable, as a list is), raises.
		if h, ok := hashOf(key); ok {
			*b = append(*b, keyTag...)
			*b = append(*b, 'H')
			*b = append(*b, key.Type().Name...)
			*b = append(*b, ':')
			*b = append(*b, h...)
			return nil
		}
		return ExceptionNewf(TypeError, "unhashable type: '%s'", key.Type().Name)
	}
	return nil
}

// identityKeyPlaceholder is what a key stored by IDENTITY decodes to.
//
// It exists so that iteration and repr of such a dict WORK rather than raising,
// and it says plainly what it stands for - a program reading it back gets
// something it can test rather than a silently wrong object.
func identityKeyPlaceholder(marker byte, addr string) Object {
	kind := "object"
	switch marker {
	case 'm':
		kind = "module"
	case 'F':
		kind = "function"
	}
	return String("<" + kind + " key " + addr + ">")
}

// hashKeyPlaceholder is what a key stored through its own __hash__ decodes to.
func hashKeyPlaceholder(typeName, hash string) Object {
	return String("<" + typeName + " key " + hash + ">")
}

// hashOf renders an object's __hash__ as a string, or reports that it has
// none.  A __hash__ that is None means explicitly unhashable, which is how a
// class says "do not use me as a key" - a list is the builtin example.
func hashOf(key Object) (string, bool) {
	// A payload-carrying instance, and every type with a natural hash, is hashed
	// from its VALUE.  HashValue has no case that calls back here - its fallback
	// performs the __hash__ lookup itself - so this cannot recurse.
	if n, ok := HashValue(key); ok {
		return strconv.FormatInt(n, 10), true
	}
	// hash(x) is type(x).__hash__(x), and for a CLASS that is the METATYPE's
	// __hash__ - the identity hash on "type".  Going through the class's own
	// __hash__ instead picked up the method meant for its INSTANCES: h11's
	// events are frozen dataclasses, so "Request.__hash__" is the generated
	// field hash, and calling it with the class itself reported
	// "Request.__hash__() missing self argument" - the class could not be a
	// dict key while every plain class could, by accident, because only a
	// plain class lacks a __hash__ of its own to be found first.
	target := key.Type()
	if t, isType := key.(*Type); isType && t.ObjectType != nil {
		target = t.ObjectType
	}
	res, found, err := target.CallMethod("__hash__", Tuple{key}, NewStringDict())
	if err != nil || !found {
		return "", false
	}
	if _, isNone := res.(NoneType); isNone {
		return "", false
	}
	n, err := Index(res)
	if err != nil {
		return "", false
	}
	return strconv.FormatInt(int64(n), 10), true
}

// identityHash hashes an object by its address, which is what CPython's
// default object hash does: two distinct functions are two distinct keys, and
// a function is the same key as long as it is alive.
func identityHash(o Object) int64 {
	return int64(uintptr(reflect.ValueOf(o).Pointer()))
}

// HashValue returns an object's hash as an int64, the way hash() reports it.
//
// This is the value hash() returns for the concrete types that have a natural
// hash.  A tuple's is computed from its ELEMENTS, because a tuple has no
// __hash__ of its own to look up; anything else falls back to its own
// __hash__, which is what makes a user type's hash respected.
func HashValue(o Object) (int64, bool) {
	// A payload-carrying instance hashes as the container it carries, so
	// "hash(T((1,2))) == hash((1,2))" and the two key the same dict entry.
	if payload, ok := payloadOf(o); ok {
		return HashValue(payload)
	}
	switch v := o.(type) {
	case NoneType:
		return 0, true
	case Bool:
		if v {
			return 1, true
		}
		return 0, true
	case Int:
		return int64(v), true
	case Float:
		return int64(float64(v)), true
	case String:
		return MemoryHash([]byte(v)), true
	case Bytes:
		return MemoryHash([]byte(v)), true
	case Tuple:
		// CPython's sequence hash: combine the elements in order, so the same
		// items in a different order hash differently.
		var h int64 = 0x345678
		for _, item := range v {
			ih, ok := HashValue(item)
			if !ok {
				return 0, false
			}
			h = (h ^ ih) * 1000003
		}
		return h ^ int64(len(v)), true
	case *Function, *Method:
		// A function and a method hash by IDENTITY, which makes them usable as
		// dict keys and set members.
		//
		// Without this a function was UNHASHABLE, and the import system keys a
		// table by one: pkg_resources' "register_loader_type(loaders.FileLoader,
		// ...)" reported "unhashable type: 'function'", which is how pip's
		// metadata backend stopped loading.
		return identityHash(o), true
	case *Module:
		// A module hashes by identity.  Its Globals holds the module's own
		// storage, and Ptr reports that storage's address - reflecting into the
		// field directly panicked, because the map is unexported.
		return int64(uintptr(v.Globals.Ptr())), true
	}
	// Anything else answers through its own __hash__, looked up on its TYPE - the
	// metatype for a class, the class for an instance.
	//
	// This deliberately does NOT call hashOf: hashOf routes back here, and the
	// two together recursed until the process died.
	target := o.Type()
	if t, isType := o.(*Type); isType && t.ObjectType != nil {
		target = t.ObjectType
	}
	res, found, err := target.CallMethod("__hash__", Tuple{o}, NewStringDict())
	if err != nil || !found {
		return 0, false
	}
	if _, isNone := res.(NoneType); isNone {
		return 0, false
	}
	n, err := Index(res)
	if err != nil {
		return 0, false
	}
	return int64(n), true
}

// DictKey encodes an object into the string form used to store dict keys.
//
// It is exported so that other mapping-like types (collections.Counter, for
// instance) can share one key encoding with dict rather than inventing a
// second one.
//
// A key whose type defines __hash__ encodes to a BUCKET - its type name and
// hash - which is what makes lookup by an equal but distinct object work.  It
// does not identify one entry: the dict's own table maps a bucket to the
// entries in it.  DictKeyDecode therefore cannot recover such a key on its
// own; a dict hands its table to StringDict.DecodeKey, which can.
func DictKey(key Object) (string, error) {
	return dictKey(key)
}

// DictKeyDecode is the inverse of DictKey for every key whose encoding is
// reversible - that is, every key whose type does not define __hash__.
//
// A hash key is not reversible, because hashing loses the object: use
// StringDict.DecodeKey, which consults the dict that stored it.
func DictKeyDecode(encoded string) (Object, error) {
	return dictKeyDecode(encoded)
}

// dictKeyDecode recovers the original key object from its encoded form.
func dictKeyDecode(encoded string) (Object, error) {
	key, rest, err := readKey(encoded)
	if err != nil {
		return nil, err
	}
	_ = rest
	return key, nil
}

func readKey(s string) (Object, string, error) {
	if !strings.HasPrefix(s, keyTag) {
		// An unencoded string key, which runs to the end.
		return String(s), "", nil
	}
	if len(s) < 2 {
		return nil, "", ExceptionNewf(KeyError, "corrupt dict key")
	}
	marker, rest := s[1], s[2:]
	switch marker {
	case 's':
		return String(rest), "", nil
	case 'i':
		i, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			return nil, "", ExceptionNewf(KeyError, "corrupt dict key")
		}
		return Int(i), "", nil
	case 'I':
		i, err := IntFromString(rest, 10)
		if err != nil {
			return nil, "", err
		}
		return i, "", nil
	case 'b':
		return Bool(rest == "1"), "", nil
	case 'f':
		bits, err := strconv.ParseUint(rest, 16, 64)
		if err != nil {
			return nil, "", ExceptionNewf(KeyError, "corrupt dict key")
		}
		return Float(math.Float64frombits(bits)), "", nil
	case 'n':
		return None, "", nil
	case 'y':
		n, rest2, err := readPrefixInt(rest)
		if err != nil {
			return nil, "", err
		}
		return Bytes(rest2[:n]), rest2[n:], nil
	case 't':
		n, rest2, err := readPrefixInt(rest)
		if err != nil {
			return nil, "", err
		}
		items := make(Tuple, n)
		for i := 0; i < n; i++ {
			size, rest3, err := readPrefixInt(rest2)
			if err != nil {
				return nil, "", err
			}
			item, _, err := readKey(rest3[:size])
			if err != nil {
				return nil, "", err
			}
			items[i] = item
			rest2 = rest3[size:]
		}
		return items, rest2, nil
	case 'm', 'F':
		// A marker for an object stored by IDENTITY: a module, a function or a
		// method.  It cannot be decoded back to the object from the encoding
		// alone - the address is all that is stored - so it decodes to a
		// placeholder that reports what it is.  Iteration over a dict holding
		// one therefore yields this object, where CPython yields the module
		// itself; the KEY LOOKUP is unaffected, because a lookup encodes the
		// key it was given and never decodes.
		//
		// Without the branch, keys()/items()/repr() of such a dict raised
		// KeyError: 'corrupt dict key' - which is how this was found: pkg_resources
		// keys a table by a function, and listing its keys failed.
		return identityKeyPlaceholder(marker, rest), "", nil
	case 'H':
		// An object keyed through its own __hash__: the encoding holds the
		// TYPE NAME and the hash, not the object.  It decodes to a placeholder
		// for the same reason, and the lookup is unaffected.
		colon := strings.LastIndexByte(rest, ':')
		if colon < 0 {
			return nil, "", ExceptionNewf(KeyError, "corrupt dict key")
		}
		return hashKeyPlaceholder(rest[:colon], rest[colon+1:]), "", nil
	case 'S':
		n, rest2, err := readPrefixInt(rest)
		if err != nil {
			return nil, "", err
		}
		items := make([]Object, 0, n)
		for i := 0; i < n; i++ {
			size, rest3, err := readPrefixInt(rest2)
			if err != nil {
				return nil, "", err
			}
			item, _, err := readKey(rest3[:size])
			if err != nil {
				return nil, "", err
			}
			items = append(items, item)
			rest2 = rest3[size:]
		}
		return NewFrozenSetFromItems(items), rest2, nil
	}
	return nil, "", ExceptionNewf(KeyError, "corrupt dict key")
}

// readPrefixInt reads a leading decimal count followed by ':'.
func readPrefixInt(s string) (int, string, error) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return 0, "", ExceptionNewf(KeyError, "corrupt dict key")
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil || n < 0 {
		return 0, "", ExceptionNewf(KeyError, "corrupt dict key")
	}
	return n, s[i+1:], nil
}

// DictNew
func DictNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	if len(args) > 1 {
		return nil, ExceptionNewf(TypeError, "dict expects at most one argument")
	}
	out := NewStringDict()
	if len(args) == 1 {
		arg := args[0]
		// A MAPPING first: "dict({'a': 1})" is the commonest form of all, and
		// it was rejected with "non-tuple sequence" because only a sequence of
		// pairs was accepted.  A mapping is copied through its keys.
		if src, isMapping := mappingStorage(arg); isMapping {
			for _, k := range src.Keys() {
				if v, ok := src.Get(k); ok {
					if err := out.copyEntryFrom(src, k, v); err != nil {
						return nil, err
					}
				}
			}
		} else {
			seq, err := SequenceList(arg)
			if err != nil {
				return nil, err
			}
			for _, i := range seq.Items {
				switch z := i.(type) {
				case Tuple:
					if zStr, ok := z[0].(String); ok {
						out.Set(string(zStr), z[1])
					}
				default:
					return nil, ExceptionNewf(TypeError, "non-tuple sequence")
				}
			}
		}
	}
	if kwargs.Len() > 0 {
		kwargs.Range(func(k string, v Object) bool {
			out.Set(k, v)
			return false
		})
	}
	// An instance of a dict SUBCLASS is a *Type whose namespace IS its storage,
	// so the contents built above have to be moved onto it - otherwise
	// "class D(dict): pass; D({'a': 1})" produced an EMPTY dict, while plain
	// "dict({'a': 1})" worked because it returns the dict built here.
	//
	// The comparison goes through a holder rather than naming StringDictType
	// directly, which would be an initialisation cycle: this function is one
	// of the values StringDictType is built from.
	if metatype != nil && metatype != gStringDictType {
		inst, err := ObjectNew(metatype, args, kwargs)
		if err != nil {
			return nil, err
		}
		if t, ok := inst.(*Type); ok {
			d := t.GetDict()
			for _, k := range out.Keys() {
				if v, ok := out.Get(k); ok {
					if err := d.copyEntryFrom(out, k, v); err != nil {
						return nil, err
					}
				}
			}
		}
		return inst, nil
	}
	return out, nil
}

// gStringDictType is StringDictType, set once it exists.
//
// DictNew is one of the values StringDictType is built from, so it cannot name
// StringDictType without an initialisation cycle; the holder breaks it.
var gStringDictType *Type

// Type of this StringDict object
func (o StringDict) Type() *Type {
	return StringDictType
}

// Make a new dictionary
func NewStringDict() StringDict {
	return StringDict{m: make(map[string]Object), shared: &dictShared{}}
}

// Make a new dictionary with reservation for n entries
func NewStringDictSized(n int) StringDict {
	return StringDict{m: make(map[string]Object, n), shared: &dictShared{}}
}

// Checks that obj is exactly a dictionary and returns an error if not
func DictCheckExact(obj Object) (StringDict, error) {
	dict, ok := obj.(StringDict)
	if !ok {
		return NewStringDict(), expectingDict
	}
	return dict, nil
}

// Checks that obj is exactly a dictionary and returns an error if not
func DictCheck(obj Object) (StringDict, error) {
	// FIXME should be checking subclasses
	return DictCheckExact(obj)
}

// Copy a dictionary
func (d StringDict) Copy() StringDict {
	e := NewStringDictSized(d.Len())
	var copyErr error
	d.Range(func(k string, v Object) bool {
		if err := e.copyEntryFrom(d, k, v); err != nil {
			copyErr = err
			return true
		}
		return false
	})
	// Copy has no error channel, and its only failure is a corrupt entry,
	// which cannot arise for a dict built by this package.
	_ = copyErr
	return e
}

func (a StringDict) M__str__() (Object, error) {
	return a.M__repr__()
}

func (a StringDict) M__len__() (Object, error) {
	return Int(a.Len()), nil
}

func (a StringDict) M__repr__() (Object, error) {
	var out bytes.Buffer
	out.WriteRune('{')
	spacer := false
	var reprErr error
	a.Range(func(key string, value Object) bool {

		if spacer {
			out.WriteString(", ")
		}
		k, err := a.DecodeKey(key)
		if err != nil {
			reprErr = err
			return true
		}
		keyStr, err := ReprAsString(k)
		if err != nil {
			reprErr = err
			return true
		}
		valueStr, err := ReprAsString(value)
		if err != nil {
			reprErr = err
			return true
		}
		out.WriteString(keyStr)
		out.WriteString(": ")
		out.WriteString(valueStr)
		spacer = true
		return false
	})
	if reprErr != nil {
		return nil, reprErr
	}
	out.WriteRune('}')
	return String(out.String()), nil
}

// Returns a list of keys from the dict
func (d StringDict) M__iter__() (Object, error) {
	o := make(Tuple, 0, d.Len())
	for _, k := range d.Keys() {
		key, err := d.DecodeKey(k)
		if err != nil {
			return nil, err
		}
		o = append(o, key)
	}
	return NewIterator(o), nil
}

func (d StringDict) M__getitem__(key Object) (Object, error) {
	encoded, ok, err := d.keyCode(key)
	if err == nil && ok {
		return d.GetOrNil(encoded), nil
	}
	return nil, ExceptionNewf(KeyError, "%v", key)
}

func (d StringDict) M__delitem__(key Object) (Object, error) {
	encoded, ok, err := d.keyCode(key)
	if err != nil {
		return nil, ExceptionNewf(KeyError, "%v", key)
	}
	if !ok {
		return nil, ExceptionNewf(KeyError, "%v", key)
	}
	d.Del(encoded)
	return None, nil
}

func (d StringDict) M__setitem__(key, value Object) (Object, error) {
	if err := d.setItem(key, value); err != nil {
		return nil, err
	}
	return None, nil
}

func (a StringDict) M__eq__(other Object) (Object, error) {
	b, ok := other.(StringDict)
	if !ok {
		return NotImplemented, nil
	}
	if a.Len() != b.Len() {
		return False, nil
	}
	var eqErr error
	same := true
	for _, entry := range a.Items() {
		key, err := a.DecodeKey(entry.Key)
		if err != nil {
			eqErr = err
			break
		}
		// Through the key OBJECT, not its encoded form: a key defining
		// __hash__ is stored under a code private to this dict, so the two
		// dicts' codes for the same key differ.
		bv, ok, err := b.keyCode(key)
		if err != nil {
			eqErr = err
			break
		}
		if !ok {
			same = false
			break
		}
		res, err := Eq(entry.Value, b.GetOrNil(bv))
		if err != nil {
			eqErr = err
			break
		}
		if res == False {
			same = false
			break
		}
	}
	if eqErr != nil {
		return nil, eqErr
	}
	if !same {
		return False, nil
	}
	return True, nil
}

func (a StringDict) M__ne__(other Object) (Object, error) {
	res, err := a.M__eq__(other)
	if err != nil {
		return nil, err
	}
	if res == NotImplemented {
		return res, nil
	}
	if res == True {
		return False, nil
	}
	return True, nil
}

// M__or__ merges two dicts into a new one, which is the PEP 584 "|" that
// CPython 3.9 added.  It only accepts another dict, as CPython does, so a
// list of pairs yields NotImplemented and the caller reports the operand
// type error.
func (a StringDict) M__or__(other Object) (Object, error) {
	b, ok := other.(StringDict)
	if !ok {
		return NotImplemented, nil
	}
	out := a.Copy()
	var mergeErr error
	b.Range(func(k string, v Object) bool {
		if err := out.copyEntryFrom(b, k, v); err != nil {
			mergeErr = err
			return true
		}
		return false
	})
	if mergeErr != nil {
		return nil, mergeErr
	}
	return out, nil
}

// M__ior__ is the in-place form of M__or__ (PEP 584 "|=").  It updates the
// receiver and returns it; like dict.update() it also accepts an iterable of
// key/value pairs.
func (a StringDict) M__ior__(other Object) (Object, error) {
	if err := dictUpdateFrom(a, other); err != nil {
		return nil, err
	}
	return a, nil
}

func (a StringDict) M__contains__(other Object) (Object, error) {
	_, ok, err := a.keyCode(other)
	if err != nil {
		return False, nil
	}
	return NewBool(ok), nil
}

func (d StringDict) GetDict() StringDict {
	return d
}

var _ IGetDict = (*StringDict)(nil)
var _ I__or__ = NewStringDict()
var _ I__ior__ = NewStringDict()
