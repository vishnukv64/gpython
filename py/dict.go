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

func init() {
	StringDictType.Dict["items"] = MustNewMethod("items", func(self Object, args Tuple) (Object, error) {
		err := UnpackTuple(args, nil, "items", 0, 0)
		if err != nil {
			return nil, err
		}
		sMap := self.(StringDict)
		o := make(Tuple, 0, len(sMap))
		for k, v := range sMap {
			key, err := dictKeyDecode(k)
			if err != nil {
				return nil, err
			}
			o = append(o, Tuple{key, v})
		}
		return NewIterator(o), nil
	}, 0, "items() -> list of D's (key, value) pairs, as 2-tuples")

	StringDictType.Dict["keys"] = MustNewMethod("keys", func(self Object, args Tuple) (Object, error) {
		err := UnpackTuple(args, nil, "keys", 0, 0)
		if err != nil {
			return nil, err
		}
		sMap := self.(StringDict)
		o := make(Tuple, 0, len(sMap))
		for k := range sMap {
			key, err := dictKeyDecode(k)
			if err != nil {
				return nil, err
			}
			o = append(o, key)
		}
		return NewIterator(o), nil
	}, 0, "keys() -> list of D's keys, as a list")

	StringDictType.Dict["values"] = MustNewMethod("values", func(self Object, args Tuple) (Object, error) {
		err := UnpackTuple(args, nil, "values", 0, 0)
		if err != nil {
			return nil, err
		}
		sMap := self.(StringDict)
		o := make(Tuple, 0, len(sMap))
		for _, v := range sMap {
			o = append(o, v)
		}
		return NewIterator(o), nil
	}, 0, "values() -> list of D's values, as a list")

	StringDictType.Dict["get"] = MustNewMethod("get", func(self Object, args Tuple) (Object, error) {
		sMap := self.(StringDict)
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
		if res, ok := sMap[encoded]; ok {
			return res, nil
		}
		if len(args) == 2 {
			return args[1], nil
		}
		return None, nil
	}, 0, "get(key[, default]) -> value for key if key is in the dictionary, else default (None by default).")

	StringDictType.Dict["pop"] = MustNewMethod("pop", func(self Object, args Tuple) (Object, error) {
		d := self.(StringDict)
		if len(args) < 1 {
			return nil, ExceptionNewf(TypeError, "pop expected at least 1 argument, got %d", len(args))
		}
		if len(args) > 2 {
			return nil, ExceptionNewf(TypeError, "pop expected at most 2 arguments, got %d", len(args))
		}
		// CPython answers an empty dict before it ever hashes the key, so
		// the default is returned (and an unhashable key is not complained
		// about) when there is nothing to pop.
		if len(d) == 0 {
			if len(args) == 2 {
				return args[1], nil
			}
			return nil, ExceptionNewf(KeyError, "%v", args[0])
		}
		encoded, err := dictKey(args[0])
		if err != nil {
			return nil, err
		}
		if res, ok := d[encoded]; ok {
			delete(d, encoded)
			return res, nil
		}
		if len(args) == 2 {
			return args[1], nil
		}
		return nil, ExceptionNewf(KeyError, "%v", args[0])
	}, 0, "pop(key[, default]) -> value -- remove specified key and return the corresponding value.")

	StringDictType.Dict["popitem"] = MustNewMethod("popitem", func(self Object, args Tuple) (Object, error) {
		d := self.(StringDict)
		if err := methodNoArgs("dict.popitem", args); err != nil {
			return nil, err
		}
		// NOTE: StringDict is a Go map, which has no insertion order, so this
		// returns an arbitrary pair rather than CPython's most recently
		// added one.  Draining a dict is unaffected; the order of the pairs
		// is not reproducible.
		for k, v := range d {
			key, err := dictKeyDecode(k)
			if err != nil {
				return nil, err
			}
			delete(d, k)
			return Tuple{key, v}, nil
		}
		return nil, ExceptionNewf(KeyError, "%v", "popitem(): dictionary is empty")
	}, 0, "popitem() -> (k, v) -- remove and return some (key, value) pair as a 2-tuple.")

	StringDictType.Dict["setdefault"] = MustNewMethod("setdefault", func(self Object, args Tuple) (Object, error) {
		d := self.(StringDict)
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
		if res, ok := d[encoded]; ok {
			return res, nil
		}
		var deflt Object = None
		if len(args) == 2 {
			deflt = args[1]
		}
		d[encoded] = deflt
		return deflt, nil
	}, 0, "setdefault(key[, default]) -> value -- return value if key is in the dictionary, else insert and return default.")

	StringDictType.Dict["update"] = MustNewMethod("update", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		d := self.(StringDict)
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
		for k, v := range kwargs {
			d[k] = v
		}
		return None, nil
	}, 0, "update([other]) -> None.  Update D from a dict/iterable of key/value pairs and keywords.")

	StringDictType.Dict["clear"] = MustNewMethod("clear", func(self Object, args Tuple) (Object, error) {
		d := self.(StringDict)
		if err := methodNoArgs("dict.clear", args); err != nil {
			return nil, err
		}
		// The map is shared with the caller, so its entries have to be
		// removed rather than the map being replaced.
		for k := range d {
			delete(d, k)
		}
		return None, nil
	}, 0, "clear() -> None.  Remove all items from the dictionary.")

	StringDictType.Dict["copy"] = MustNewMethod("copy", func(self Object, args Tuple) (Object, error) {
		d := self.(StringDict)
		if err := methodNoArgs("dict.copy", args); err != nil {
			return nil, err
		}
		return d.Copy(), nil
	}, 0, "copy() -> a shallow copy of the dictionary.")

	// dict.fromkeys is a class method in CPython: it is looked up on the
	// type (dict.fromkeys) as well as on an instance ({}.fromkeys).  In both
	// forms the arguments arrive unshifted - self is the dict for the bound
	// form and the module for the unbound one - so unlike the other methods
	// this one ignores self entirely.
	StringDictType.Dict["fromkeys"] = MustNewMethod("fromkeys", func(self Object, args Tuple) (Object, error) {
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
			out[encoded] = value
			return false
		})
		if err != nil {
			return nil, err
		}
		if loopErr != nil {
			return nil, loopErr
		}
		return out, nil
	}, 0, "fromkeys(iterable, value=None, /) -> New dict with keys from iterable and values equal to value.")
}

// dictUpdateFrom implements the body shared by dict.update() and
// dict.__ior__().
//
// other may be a mapping - anything with a keys() method, which is how
// CPython tells the two forms apart - or an iterable of key/value pairs.
func dictUpdateFrom(d StringDict, other Object) error {
	if src, ok := other.(StringDict); ok {
		for k, v := range src {
			d[k] = v
		}
		return nil
	}
	if keysFn, err := GetAttrString(other, "keys"); err == nil {
		keys, err := Call(keysFn, nil, nil)
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
			d[encoded] = value
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
		d[encoded] = pair[1]
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
// Used for variables etc where the keys can only be strings
type StringDict map[string]Object

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
		*b = append(*b, keyTag...)
		*b = append(*b, 'b')
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
		members := make([]string, 0, len(k.items))
		for item := range k.items {
			ek, err := dictKey(item)
			if err != nil {
				return err
			}
			members = append(members, ek)
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
		return ExceptionNewf(TypeError, "unhashable type: '%s'", key.Type().Name)
	}
	return nil
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
		seq, err := SequenceList(arg)
		if err != nil {
			return nil, err
		}
		for _, i := range seq.Items {
			switch z := i.(type) {
			case Tuple:
				if zStr, ok := z[0].(String); ok {
					out[string(zStr)] = z[1]
				}
			default:
				return nil, ExceptionNewf(TypeError, "non-tuple sequence")
			}
		}
	}
	if len(kwargs) > 0 {
		for k, v := range kwargs {
			out[k] = v
		}
	}
	return out, nil
}

// Type of this StringDict object
func (o StringDict) Type() *Type {
	return StringDictType
}

// Make a new dictionary
func NewStringDict() StringDict {
	return make(StringDict)
}

// Make a new dictionary with reservation for n entries
func NewStringDictSized(n int) StringDict {
	return make(StringDict, n)
}

// Checks that obj is exactly a dictionary and returns an error if not
func DictCheckExact(obj Object) (StringDict, error) {
	dict, ok := obj.(StringDict)
	if !ok {
		return nil, expectingDict
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
	e := make(StringDict, len(d))
	for k, v := range d {
		e[k] = v
	}
	return e
}

func (a StringDict) M__str__() (Object, error) {
	return a.M__repr__()
}

func (a StringDict) M__len__() (Object, error) {
	return Int(len(a)), nil
}

func (a StringDict) M__repr__() (Object, error) {
	var out bytes.Buffer
	out.WriteRune('{')
	spacer := false
	for key, value := range a {
		if spacer {
			out.WriteString(", ")
		}
		key, err := dictKeyDecode(key)
		if err != nil {
			return nil, err
		}
		keyStr, err := ReprAsString(key)
		if err != nil {
			return nil, err
		}
		valueStr, err := ReprAsString(value)
		if err != nil {
			return nil, err
		}
		out.WriteString(keyStr)
		out.WriteString(": ")
		out.WriteString(valueStr)
		spacer = true
	}
	out.WriteRune('}')
	return String(out.String()), nil
}

// Returns a list of keys from the dict
func (d StringDict) M__iter__() (Object, error) {
	o := make(Tuple, 0, len(d))
	for k := range d {
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
		if res, ok := d[encoded]; ok {
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
	if _, ok := d[encoded]; !ok {
		return nil, ExceptionNewf(KeyError, "%v", key)
	}
	delete(d, encoded)
	return None, nil
}

func (d StringDict) M__setitem__(key, value Object) (Object, error) {
	encoded, err := dictKey(key)
	if err != nil {
		return nil, err
	}
	d[encoded] = value
	return None, nil
}

func (a StringDict) M__eq__(other Object) (Object, error) {
	b, ok := other.(StringDict)
	if !ok {
		return NotImplemented, nil
	}
	if len(a) != len(b) {
		return False, nil
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return False, nil
		}
		res, err := Eq(av, bv)
		if err != nil {
			return nil, err
		}
		if res == False {
			return False, nil
		}
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
	for k, v := range b {
		out[k] = v
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
	encoded, err := dictKey(other)
	if err != nil {
		return False, nil
	}
	if _, ok := a[encoded]; ok {
		return True, nil
	}
	return False, nil
}

func (d StringDict) GetDict() StringDict {
	return d
}

var _ IGetDict = (*StringDict)(nil)
var _ I__or__ = StringDict(nil)
var _ I__ior__ = StringDict(nil)
