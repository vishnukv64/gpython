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
			key, err := dictKeyDecode(k)
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
			key, err := dictKeyDecode(k)
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
		encoded, err := dictKey(args[0])
		if err != nil {
			return nil, err
		}
		if res, ok := sMap.Get(encoded); ok {
			return res, nil
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
		encoded, err := dictKey(args[0])
		if err != nil {
			return nil, err
		}
		if res, ok := d.Get(encoded); ok {
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
		d.Del(k)
		key, err := dictKeyDecode(k)
		if err != nil {
			return nil, err
		}
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
		encoded, err := dictKey(args[0])
		if err != nil {
			return nil, err
		}
		if res, ok := d.Get(encoded); ok {
			return res, nil
		}
		var deflt Object = None
		if len(args) == 2 {
			deflt = args[1]
		}
		d.Set(encoded, deflt)
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
			encoded, err := dictKey(key)
			if err != nil {
				loopErr = err
				return true
			}
			out.Set(encoded, value)
			return false
		})
		if err != nil {
			return nil, err
		}
		if loopErr != nil {
			return nil, loopErr
		}
		return out, nil
	}, 0, "fromkeys(iterable, value=None, /) -> New dict with keys from iterable and values equal to value."))
}

// dictUpdateFrom implements the body shared by dict.update() and
// dict.__ior__().
//
// other may be a mapping - anything with a keys() method, which is how
// CPython tells the two forms apart - or an iterable of key/value pairs.
func dictUpdateFrom(d StringDict, other Object) error {
	if src, ok := other.(StringDict); ok {
		// Range snapshots the keys, so "d.update(d)" is safe.
		src.Range(func(k string, v Object) bool {
			d.Set(k, v)
			return false
		})
		return nil
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
			encoded, err := dictKey(key)
			if err != nil {
				loopErr = err
				return true
			}
			d.Set(encoded, value)
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
		encoded, err := dictKey(pair[0])
		if err != nil {
			loopErr = err
			return true
		}
		d.Set(encoded, pair[1])
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
// a copy.  A StringDict built by NewStringDict always has both fields set; the
// zero value reads as an empty dict, as the bare nil map did.
type StringDict struct {
	m     map[string]Object
	order *[]string
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
	return d.order == other.order
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
	if d.order == nil {
		d.order = new([]string)
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
		*d.order = append(*d.order, key)
	}
}

// Del removes an encoded key, reporting whether it was present.
func (d *StringDict) Del(key string) bool {
	if _, ok := d.m[key]; !ok {
		return false
	}
	delete(d.m, key)
	keys := *d.order
	for i, k := range keys {
		if k == key {
			*d.order = append(keys[:i], keys[i+1:]...)
			break
		}
	}
	return true
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
	if d.order == nil {
		return nil
	}
	out := make([]string, len(*d.order))
	copy(out, *d.order)
	return out
}

// Values returns the values in insertion order.
func (d StringDict) Values() []Object {
	if d.order == nil {
		return nil
	}
	order := *d.order
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
	if d.order == nil {
		return nil
	}
	out := make([]DictEntry, 0, len(*d.order))
	for _, k := range *d.order {
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
	*d.order = (*d.order)[:0]
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
	case *FrozenSet:
		// Sets have no order, so sort the member encodings to give equal
		// sets the same key.
		// k.items is already keyed by the encoded form, so the keys ARE the
		// encoded members; re-encoding them would double-encode.  They are
		// sorted, because Go map iteration is random and the encoding must be
		// stable for two equal frozensets to hash alike.
		members := make([]string, 0, len(k.items))
		for item := range k.items {
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

// hashOf renders an object's __hash__ as a string, or reports that it has
// none.  A __hash__ that is None means explicitly unhashable, which is how a
// class says "do not use me as a key" - a list is the builtin example.
func hashOf(key Object) (string, bool) {
	res, ok, err := TypeCall0(key, "__hash__")
	if err != nil || !ok {
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

// DictKey encodes an object into the string form used to store dict keys,
// and DictKeyDecode recovers the original object from it.
//
// They are exported so that other mapping-like types (collections.Counter,
// for instance) can share one key encoding with dict rather than inventing
// a second one.
func DictKey(key Object) (string, error) {
	return dictKey(key)
}

// DictKeyDecode is the inverse of DictKey.
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
					out.Set(k, v)
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
					d.Set(k, v)
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
	return StringDict{m: make(map[string]Object), order: new([]string)}
}

// Make a new dictionary with reservation for n entries
func NewStringDictSized(n int) StringDict {
	return StringDict{m: make(map[string]Object, n), order: new([]string)}
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
	d.Range(func(k string, v Object) bool {
		e.Set(k, v)
		return false
	})
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
		k, err := dictKeyDecode(key)
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
		key, err := dictKeyDecode(k)
		if err != nil {
			return nil, err
		}
		o = append(o, key)
	}
	return NewIterator(o), nil
}

func (d StringDict) M__getitem__(key Object) (Object, error) {
	encoded, err := dictKey(key)
	if err == nil {
		if res, ok := d.Get(encoded); ok {
			return res, nil
		}
	}
	return nil, ExceptionNewf(KeyError, "%v", key)
}

func (d StringDict) M__delitem__(key Object) (Object, error) {
	encoded, err := dictKey(key)
	if err != nil {
		return nil, ExceptionNewf(KeyError, "%v", key)
	}
	if _, ok := d.Get(encoded); !ok {
		return nil, ExceptionNewf(KeyError, "%v", key)
	}
	d.Del(encoded)
	return None, nil
}

func (d StringDict) M__setitem__(key, value Object) (Object, error) {
	encoded, err := dictKey(key)
	if err != nil {
		return nil, err
	}
	d.Set(encoded, value)
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
	a.Range(func(k string, av Object) bool {

		bv, ok := b.Get(k)
		if !ok {
			same = false
			return true
		}
		res, err := Eq(av, bv)
		if err != nil {
			eqErr = err
			return true
		}
		if res == False {
			same = false
			return true
		}
		return false
	})
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
	b.Range(func(k string, v Object) bool {
		out.Set(k, v)
		return false
	})
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
	encoded, err := dictKey(other)
	if err != nil {
		return False, nil
	}
	if _, ok := a.Get(encoded); ok {
		return True, nil
	}
	return False, nil
}

func (d StringDict) GetDict() StringDict {
	return d
}

var _ IGetDict = (*StringDict)(nil)
var _ I__or__ = NewStringDict()
var _ I__ior__ = NewStringDict()
