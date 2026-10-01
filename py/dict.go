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
		var length = len(args)
		switch {
		case length == 0:
			return nil, ExceptionNewf(TypeError, "%s expected at least 1 arguments, got %d", "items()", length)
		case length > 2:
			return nil, ExceptionNewf(TypeError, "%s expected at most 2 arguments, got %d", "items()", length)
		}
		sMap := self.(StringDict)
		if str, ok := args[0].(String); ok {
			if res, ok := sMap[string(str)]; ok {
				return res, nil
			}

			switch length {
			case 2:
				return args[1], nil
			default:
				return None, nil
			}
		}
		return nil, ExceptionNewf(KeyError, "%v", args[0])
	}, 0, "gets(key, default) -> If there is a val corresponding to key, return val, otherwise default")
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
