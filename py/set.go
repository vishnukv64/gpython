// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Set and FrozenSet types
//
// FIXME preliminary implementation only - doesn't work properly!

package py

import "bytes"

var SetType = NewTypeX("set", "set() -> new empty set object\nset(iterable) -> new set object\n\nBuild an unordered collection of unique elements.", SetNew, nil)

type SetValue struct{}

type Set struct {
	// items is keyed by the ENCODED form of each member (see DictKey), not by
	// the Object itself.
	//
	// Keying by Object used GO equality, so two objects equal in Python but
	// distinct Go values were different members: len({Path("/a"), Path("/a")})
	// was 2. It also PANICKED for a non-comparable Object.  The encoded key is
	// what dict already uses, and it is what makes the Python hash/eq protocol
	// apply - including raising for a genuinely unhashable member.
	items map[string]SetValue
}

// Type of this Set object
func (o *Set) Type() *Type {
	return SetType
}

// Make a new empty set
func NewSet() *Set {
	return &Set{
		items: make(map[string]SetValue),
	}
}

// Make a new empty set with capacity for n items
func NewSetWithCapacity(n int) *Set {
	return &Set{
		items: make(map[string]SetValue, n),
	}
}

// Make a new set with the items passed in
//
// An unhashable item is DROPPED, which is wrong but is what this signature can
// express; use NewSetFromItemsErr where the error can be reported.  The two
// callers that must not drop are set(iterable) and a set display, which is why
// that variant exists.
func NewSetFromItems(items []Object) *Set {
	s := NewSetWithCapacity(len(items))
	for _, item := range items {
		_ = s.setAdd(item)
	}
	return s
}

// NewSetFromItemsErr builds a set, reporting an unhashable item.
//
// "set([[1]])" must raise "unhashable type: 'list'" rather than quietly
// producing an empty set, which is what discarding the error did.
func NewSetFromItemsErr(items []Object) (*Set, error) {
	s := NewSetWithCapacity(len(items))
	for _, item := range items {
		if err := s.setAdd(item); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func init() {
	SetType.Dict.Set("add", MustNewMethod("add", func(self Object, args Tuple) (Object, error) {
		setSelf := self.(*Set)
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "add() takes exactly one argument (%d given)", len(args))
		}
		// The erroring form: "s.add([1])" must raise "unhashable type:
		// 'list'", not quietly do nothing.
		if err := setSelf.AddErr(args[0]); err != nil {
			return nil, err
		}
		return NoneType{}, nil
	}, 0, "add(value)"))
}

// Add an item to the set
// setKey encodes a member the way dict encodes a key, so that membership uses
// the Python hash/eq protocol rather than Go equality.
func setKey(item Object) (string, error) {
	return DictKey(item)
}

// setAdd inserts a member, reporting an unhashable one.
func (s *Set) setAdd(item Object) error {
	k, err := setKey(item)
	if err != nil {
		return err
	}
	s.items[k] = SetValue{}
	return nil
}

// setHas reports whether a member is present.
func (s *Set) setHas(item Object) bool {
	k, err := setKey(item)
	if err != nil {
		return false
	}
	_, ok := s.items[k]
	return ok
}

// setDelete removes a member, reporting whether it was there.
func (s *Set) setDelete(item Object) bool {
	k, err := setKey(item)
	if err != nil {
		return false
	}
	if _, ok := s.items[k]; !ok {
		return false
	}
	delete(s.items, k)
	return true
}

// setItems returns the members as Objects, decoding each key.
func (s *Set) setItems() []Object {
	out := make([]Object, 0, len(s.items))
	for k := range s.items {
		if item, err := DictKeyDecode(k); err == nil {
			out = append(out, item)
		}
	}
	return out
}

func (s *Set) Add(item Object) {
	_ = s.setAdd(item)
}

// Copy returns a shallow copy of the set.  A new items map, so the copy and
// the original are independent.
func (s *Set) Copy() *Set {
	ret := NewSetWithCapacity(len(s.items))
	for k := range s.items {
		ret.items[k] = SetValue{}
	}
	return ret
}

// AddErr inserts a member, reporting an unhashable one.  The VM uses this so
// that "{[1]}" and "s.add([1])" raise rather than silently doing nothing.
func (s *Set) AddErr(item Object) error {
	return s.setAdd(item)
}

// SetNew
func SetNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	var iterable Object
	err := UnpackTuple(args, kwargs, "set", 0, 1, &iterable)
	if err != nil {
		return nil, err
	}
	if iterable != nil {
		return SequenceSet(iterable)
	}
	return NewSet(), nil
}

// FrozenSetNew implements frozenset([iterable]).
//
// FrozenSetType was created with NewType, which leaves New nil, so
// "frozenset(...)" raised "cannot create 'frozenset' instances" and a
// frozenset literal - which is how idna writes its bidi category tables -
// could not be built at all.  The elements are collected through the same
// path set() uses, so the two agree on duplicate and ordering behaviour.
func FrozenSetNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	var iterable Object
	err := UnpackTuple(args, kwargs, "frozenset", 0, 1, &iterable)
	if err != nil {
		return nil, err
	}
	if iterable == nil {
		return NewFrozenSet(), nil
	}
	s, err := SequenceSet(iterable)
	if err != nil {
		return nil, err
	}
	return NewFrozenSetFromItems(s.setItems()), nil
}

var FrozenSetType = NewTypeX("frozenset", "frozenset() -> empty frozenset object\nfrozenset(iterable) -> frozenset object\n\nBuild an immutable unordered collection of unique elements.", FrozenSetNew, nil)

type FrozenSet struct {
	Set
}

// Type of this FrozenSet object
func (o *FrozenSet) Type() *Type {
	return FrozenSetType
}

// M__repr__ renders "frozenset({...})".
//
// FrozenSet embeds a Set, so it would otherwise inherit Set.M__repr__ - whose
// receiver is the EMBEDDED field, which cannot tell what encloses it.  This
// shadows that with the frozen spelling.
func (o *FrozenSet) M__repr__() (Object, error) {
	rep, err := o.Set.M__repr__()
	if err != nil {
		return nil, err
	}
	// The empty frozenset is "frozenset()", not "frozenset({})".
	if len(o.items) == 0 {
		return String("frozenset()"), nil
	}
	return String("frozenset(" + string(rep.(String)) + ")"), nil
}

// Make a new empty frozen set
func NewFrozenSet() *FrozenSet {
	return &FrozenSet{
		Set: *NewSet(),
	}
}

// Make a new set with the items passed in
func NewFrozenSetFromItems(items []Object) *FrozenSet {
	return &FrozenSet{
		Set: *NewSetFromItems(items),
	}
}

// Extend the set with items
func (s *Set) Update(items []Object) {
	for _, item := range items {
		_ = s.setAdd(item)
	}
}

func (s *Set) M__len__() (Object, error) {
	return Int(len(s.items)), nil
}

func (s *Set) M__bool__() (Object, error) {
	return NewBool(len(s.items) > 0), nil
}

func (s *Set) M__repr__() (Object, error) {
	// The EMPTY set is "set()", not "{}" - which is an empty DICT.
	if len(s.items) == 0 {
		return String("set()"), nil
	}
	var out bytes.Buffer
	out.WriteRune('{')
	spacer := false
	for _, item := range s.setItems() {
		if spacer {
			out.WriteString(", ")
		}
		str, err := ReprAsString(item)
		if err != nil {
			return nil, err
		}
		out.WriteString(str)
		spacer = true
	}
	out.WriteRune('}')
	return String(out.String()), nil
}

func (s *Set) M__iter__() (Object, error) {
	objs := s.setItems()
	items := make(Tuple, 0, len(objs))
	items = append(items, objs...)
	return NewIterator(items), nil
}

func (s *Set) M__and__(other Object) (Object, error) {
	ret := NewSet()
	b, ok := other.(*Set)
	if !ok {
		return nil, ExceptionNewf(TypeError, "unsupported operand type(s) for &: '%s' and '%s'", s.Type().Name, other.Type().Name)
	}
	for k := range b.items {
		if _, ok := s.items[k]; ok {
			ret.items[k] = SetValue{}
		}
	}
	return ret, nil
}

func (s *Set) M__or__(other Object) (Object, error) {
	ret := NewSet()
	b, ok := other.(*Set)
	if !ok {
		return nil, ExceptionNewf(TypeError, "unsupported operand type(s) for &: '%s' and '%s'", s.Type().Name, other.Type().Name)
	}
	for k := range s.items {
		ret.items[k] = SetValue{}
	}
	for k := range b.items {
		if _, ok := s.items[k]; !ok {
			ret.items[k] = SetValue{}
		}
	}
	return ret, nil
}

func (s *Set) M__sub__(other Object) (Object, error) {
	ret := NewSet()
	b, ok := other.(*Set)
	if !ok {
		return nil, ExceptionNewf(TypeError, "unsupported operand type(s) for &: '%s' and '%s'", s.Type().Name, other.Type().Name)
	}
	for k := range s.items {
		ret.items[k] = SetValue{}
	}
	for k := range b.items {
		delete(ret.items, k)
	}
	return ret, nil
}

func (s *Set) M__xor__(other Object) (Object, error) {
	ret := NewSet()
	b, ok := other.(*Set)
	if !ok {
		return nil, ExceptionNewf(TypeError, "unsupported operand type(s) for &: '%s' and '%s'", s.Type().Name, other.Type().Name)
	}
	for k := range s.items {
		ret.items[k] = SetValue{}
	}
	for k := range b.items {
		if _, ok := s.items[k]; ok {
			delete(ret.items, k)
		} else {
			ret.items[k] = SetValue{}
		}
	}
	return ret, nil
}

// Check interface is satisfied
var _ I__len__ = (*Set)(nil)
var _ I__bool__ = (*Set)(nil)
var _ I__iter__ = (*Set)(nil)

// var _ richComparison = (*Set)(nil)

func (a *Set) M__eq__(other Object) (Object, error) {
	// A frozenset compares equal to a set with the same members, so the
	// other side is accepted in either form.  Asserting *Set made
	// "frozenset([1,2]) == frozenset([2,1])" raise "unsupported operand
	// type(s) for ==: 'frozenset' and 'frozenset'".
	var b *Set
	switch o := other.(type) {
	case *Set:
		b = o
	case *FrozenSet:
		b = &o.Set
	default:
		return NotImplemented, nil
	}
	if len(a.items) != len(b.items) {
		return False, nil
	}
	// O(n) now, not O(n**2).
	//
	// This was a nested loop with a "FIXME waiting for proper hashing",
	// calling Eq on every pair.  Members are now stored under their ENCODED
	// form (DictKey), so two members are equal exactly when their keys are -
	// which makes equality a direct key-set comparison, and it is the same
	// hash/eq protocol CPython uses.
	for k := range a.items {
		if _, ok := b.items[k]; !ok {
			return False, nil
		}
	}
	return True, nil
}

func (a *Set) M__ne__(other Object) (Object, error) {
	eq, err := a.M__eq__(other)
	if err != nil {
		return nil, err
	}
	if eq == NotImplemented {
		return eq, nil
	}
	if eq == True {
		return False, nil
	}
	return True, nil
}

// The rest of the set API.
//
// "set" had only "add", which is why click stopped at
// "AttributeError: 'set' has no attribute 'update'" in parser.py.  Each mutator
// works on the encoded keys directly, which is what makes it agree with
// membership: two equal members are one key.
func init() {
	// asSet returns the other operand as a *Set, or reports that it is not one.
	// A member may be handed a *FrozenSet as well as a *Set, so the helpers
	// work from the decoded members rather than a concrete type.
	asIterable := func(v Object) ([]Object, error) {
		switch o := v.(type) {
		case *Set:
			return o.setItems(), nil
		case *FrozenSet:
			return o.setItems(), nil
		}
		list, err := SequenceList(v)
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	}

	// self-as-Set, working for a frozen set too.  A method promoted from
	// SetType to FrozenSetType has the EMBEDDED Set as its receiver, so
	// asserting *Set panicked for "frozenset([1]).union([2])".
	asSet := func(self Object) *Set {
		switch o := self.(type) {
		case *Set:
			return o
		case *FrozenSet:
			return &o.Set
		}
		return nil
	}

	SetType.Dict.Set("update", MustNewMethod("update", func(self Object, args Tuple) (Object, error) {
		s := asSet(self)
		for _, arg := range args {
			items, err := asIterable(arg)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if err := s.setAdd(item); err != nil {
					return nil, err
				}
			}
		}
		return NoneType{}, nil
	}, 0, "update(*others) -> add every element of each iterable"))

	SetType.Dict.Set("union", MustNewMethod("union", func(self Object, args Tuple) (Object, error) {
		ret := asSet(self).Copy()
		for _, arg := range args {
			items, err := asIterable(arg)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if err := ret.setAdd(item); err != nil {
					return nil, err
				}
			}
		}
		return ret, nil
	}, 0, "union(*others) -> a new set with elements from this set and all others"))

	SetType.Dict.Set("intersection", MustNewMethod("intersection", func(self Object, args Tuple) (Object, error) {
		s := asSet(self)
		ret := s.Copy()
		for _, arg := range args {
			items, err := asIterable(arg)
			if err != nil {
				return nil, err
			}
			// The other iterable's members as a key set.
			keep := make(map[string]bool, len(items))
			for _, item := range items {
				k, err := setKey(item)
				if err != nil {
					return nil, err
				}
				keep[k] = true
			}
			for k := range ret.items {
				if !keep[k] {
					delete(ret.items, k)
				}
			}
		}
		return ret, nil
	}, 0, "intersection(*others) -> a new set with elements common to all"))

	SetType.Dict.Set("difference", MustNewMethod("difference", func(self Object, args Tuple) (Object, error) {
		ret := asSet(self).Copy()
		for _, arg := range args {
			items, err := asIterable(arg)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if k, err := setKey(item); err == nil {
					delete(ret.items, k)
				}
			}
		}
		return ret, nil
	}, 0, "difference(*others) -> a new set with elements not in the others"))

	SetType.Dict.Set("symmetric_difference", MustNewMethod("symmetric_difference", func(self Object, args Tuple) (Object, error) {
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "symmetric_difference() takes exactly one argument (%d given)", len(args))
		}
		s := asSet(self)
		other, err := SequenceSet(args[0])
		if err != nil {
			return nil, err
		}
		ret := s.Copy()
		for k := range other.items {
			if _, ok := ret.items[k]; ok {
				delete(ret.items, k)
			} else {
				ret.items[k] = SetValue{}
			}
		}
		return ret, nil
	}, 0, "symmetric_difference(other) -> elements in exactly one of the two sets"))

	SetType.Dict.Set("intersection_update", MustNewMethod("intersection_update", func(self Object, args Tuple) (Object, error) {
		s := asSet(self)
		res, err := s.M__and__(s)
		_ = res
		_ = err
		// Reuse the non-mutating form and copy its result in.
		for _, arg := range args {
			items, err := asIterable(arg)
			if err != nil {
				return nil, err
			}
			keep := make(map[string]bool, len(items))
			for _, item := range items {
				k, err := setKey(item)
				if err != nil {
					return nil, err
				}
				keep[k] = true
			}
			for k := range s.items {
				if !keep[k] {
					delete(s.items, k)
				}
			}
		}
		return NoneType{}, nil
	}, 0, "intersection_update(*others) -> keep only elements common to all"))

	SetType.Dict.Set("difference_update", MustNewMethod("difference_update", func(self Object, args Tuple) (Object, error) {
		s := asSet(self)
		for _, arg := range args {
			items, err := asIterable(arg)
			if err != nil {
				return nil, err
			}
			for _, item := range items {
				if k, err := setKey(item); err == nil {
					delete(s.items, k)
				}
			}
		}
		return NoneType{}, nil
	}, 0, "difference_update(*others) -> remove elements found in the others"))

	SetType.Dict.Set("symmetric_difference_update", MustNewMethod("symmetric_difference_update", func(self Object, args Tuple) (Object, error) {
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "symmetric_difference_update() takes exactly one argument (%d given)", len(args))
		}
		s := asSet(self)
		other, err := SequenceSet(args[0])
		if err != nil {
			return nil, err
		}
		for k := range other.items {
			if _, ok := s.items[k]; ok {
				delete(s.items, k)
			} else {
				s.items[k] = SetValue{}
			}
		}
		return NoneType{}, nil
	}, 0, "symmetric_difference_update(other) -> keep elements in exactly one"))

	SetType.Dict.Set("discard", MustNewMethod("discard", func(self Object, args Tuple) (Object, error) {
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "discard() takes exactly one argument (%d given)", len(args))
		}
		asSet(self).setDelete(args[0]) // discard does NOT raise when absent
		return NoneType{}, nil
	}, 0, "discard(value) -> remove value if present, without raising"))

	SetType.Dict.Set("remove", MustNewMethod("remove", func(self Object, args Tuple) (Object, error) {
		s := asSet(self)
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "remove() takes exactly one argument (%d given)", len(args))
		}
		// remove DOES raise when the element is absent.
		if !s.setDelete(args[0]) {
			return nil, ExceptionNewf(KeyError, "%s", args[0])
		}
		return NoneType{}, nil
	}, 0, "remove(value) -> remove value; raise KeyError if absent"))

	SetType.Dict.Set("pop", MustNewMethod("pop", func(self Object, args Tuple) (Object, error) {
		s := asSet(self)
		if len(s.items) == 0 {
			return nil, ExceptionNewf(KeyError, "pop from an empty set")
		}
		// An ARBITRARY element, as CPython says; Go map order is as good as any.
		for k := range s.items {
			item, err := DictKeyDecode(k)
			if err != nil {
				return nil, err
			}
			delete(s.items, k)
			return item, nil
		}
		return NoneType{}, nil
	}, 0, "pop() -> remove and return an arbitrary element"))

	SetType.Dict.Set("clear", MustNewMethod("clear", func(self Object, args Tuple) (Object, error) {
		s := asSet(self)
		s.items = make(map[string]SetValue)
		return NoneType{}, nil
	}, 0, "clear() -> remove all elements"))

	SetType.Dict.Set("copy", MustNewMethod("copy", func(self Object, args Tuple) (Object, error) {
		return asSet(self).Copy(), nil
	}, 0, "copy() -> a shallow copy"))

	SetType.Dict.Set("issubset", MustNewMethod("issubset", func(self Object, args Tuple) (Object, error) {
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "issubset() takes exactly one argument (%d given)", len(args))
		}
		s := asSet(self)
		other, err := SequenceSet(args[0])
		if err != nil {
			return nil, err
		}
		for k := range s.items {
			if _, ok := other.items[k]; !ok {
				return False, nil
			}
		}
		return True, nil
	}, 0, "issubset(other) -> True if every element is in other"))

	SetType.Dict.Set("issuperset", MustNewMethod("issuperset", func(self Object, args Tuple) (Object, error) {
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "issuperset() takes exactly one argument (%d given)", len(args))
		}
		s := asSet(self)
		other, err := SequenceSet(args[0])
		if err != nil {
			return nil, err
		}
		for k := range other.items {
			if _, ok := s.items[k]; !ok {
				return False, nil
			}
		}
		return True, nil
	}, 0, "issuperset(other) -> True if every element of other is here"))

	SetType.Dict.Set("isdisjoint", MustNewMethod("isdisjoint", func(self Object, args Tuple) (Object, error) {
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "isdisjoint() takes exactly one argument (%d given)", len(args))
		}
		s := asSet(self)
		other, err := SequenceSet(args[0])
		if err != nil {
			return nil, err
		}
		for k := range other.items {
			if _, ok := s.items[k]; ok {
				return False, nil
			}
		}
		return True, nil
	}, 0, "isdisjoint(other) -> True if the two sets share no element"))

	SetType.Dict.Set("__len__", MustNewMethod("__len__", func(self Object, args Tuple) (Object, error) {
		return Int(len(asSet(self).items)), nil
	}, 0, "len(set)"))

	// The frozen set gets the non-mutating half, on FrozenSetType so the
	// promoted Set methods do not apply.
	for _, name := range []string{"union", "intersection", "difference",
		"symmetric_difference", "copy", "issubset", "issuperset", "isdisjoint", "__len__"} {
		FrozenSetType.Dict.Set(name, SetType.Dict.GetOrNil(name))
	}
}
