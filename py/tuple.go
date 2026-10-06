// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tuple objects

package py

import "bytes"

var TupleType = ObjectType.NewType("tuple", "tuple() -> empty tuple\ntuple(iterable) -> tuple initialized from iterable's items\n\nIf the argument is a tuple, the return value is the same object.", TupleNew, nil)

type Tuple []Object

// Type of this Tuple object
func (o Tuple) Type() *Type {
	return TupleType
}

// TupleNew
func TupleNew(metatype *Type, args Tuple, kwargs StringDict) (res Object, err error) {
	var iterable Object
	err = UnpackTuple(args, kwargs, "tuple", 0, 1, &iterable)
	if err != nil {
		return nil, err
	}
	if iterable != nil {
		return SequenceTuple(iterable)
	}
	return Tuple{}, nil
}

// Copy a tuple object
func (t Tuple) Copy() Tuple {
	newT := make(Tuple, len(t))
	copy(newT, t)
	return newT
}

// Reverses a tuple (in-place)
func (t Tuple) Reverse() {
	for i, j := 0, len(t)-1; i < j; i, j = i+1, j-1 {
		t[i], t[j] = t[j], t[i]
	}
}

// output the tuple to out, using fn to transform the tuple to out
// start and end brackets
func (t Tuple) repr(start, end string) (Object, error) {
	var out bytes.Buffer
	out.WriteString(start)
	for i, obj := range t {
		if i != 0 {
			out.WriteString(", ")
		}
		str, err := ReprAsString(obj)
		if err != nil {
			return nil, err
		}
		out.WriteString(str)
	}
	// A ONE-element tuple reprs with a trailing comma - "(1,)" - because that
	// comma is the only thing distinguishing it from a parenthesised
	// expression, and repr is what eval() reads back.  Without it, repr((1,))
	// was "(1)", which is not a tuple at all: it round-tripped to an int, and
	// it was wrong in every nested position too - inside a list, as a dict key,
	// and inside another tuple.
	//
	// This helper is shared with the list repr, which passes "[" and "]" and
	// where a single element takes NO comma - "[(1,)]" is a list holding a
	// tuple, not a one-element list.  The comma belongs to the paren form, so
	// that is what it keys on.
	if len(t) == 1 && start == "(" {
		out.WriteString(",")
	}
	out.WriteString(end)
	return String(out.String()), nil
}

func (t Tuple) M__str__() (Object, error) {
	return t.M__repr__()
}

func (t Tuple) M__repr__() (Object, error) {
	return t.repr("(", ")")
}

func (t Tuple) M__len__() (Object, error) {
	return Int(len(t)), nil
}

func (t Tuple) M__bool__() (Object, error) {
	return NewBool(len(t) > 0), nil
}

func (t Tuple) M__iter__() (Object, error) {
	return NewIterator(t), nil
}

func (t Tuple) M__getitem__(key Object) (Object, error) {
	if slice, ok := key.(*Slice); ok {
		start, stop, step, slicelength, err := slice.GetIndices(len(t))
		if err != nil {
			return nil, err
		}
		if step == 1 {
			// Return a subslice since tuples are immutable
			return t[start:stop], nil
		}
		newTuple := make(Tuple, slicelength)
		for i, j := start, 0; j < slicelength; i, j = i+step, j+1 {
			newTuple[j] = t[i]
		}
		return newTuple, nil
	}
	i, err := IndexIntCheckNamed(key, len(t), "tuple")
	if err != nil {
		return nil, err
	}
	return t[i], nil
}

func (a Tuple) M__add__(other Object) (Object, error) {
	if b, ok := other.(Tuple); ok {
		newTuple := make(Tuple, len(a)+len(b))
		copy(newTuple, a)
		// The second copy goes at len(a) - where the first one ENDS.  Writing it
		// at len(b) instead left a nil slot whenever the two lengths differed:
		// "() + ('AAA',)" produced a 1-element tuple whose single entry was nil,
		// and the nil then PANICKED the host process the moment anything looked
		// at it - repr, a loop, or a list().  "('AAA',) + ()" happened to work
		// because there len(b) is 0 and equals len(a).
		copy(newTuple[len(a):], b)
		return newTuple, nil
	}

	return NotImplemented, nil
}

func (a Tuple) M__radd__(other Object) (Object, error) {
	if b, ok := other.(Tuple); ok {
		return b.M__add__(a)
	}
	return NotImplemented, nil
}

func (a Tuple) M__iadd__(other Object) (Object, error) {
	return a.M__add__(other)
}

func (l Tuple) M__mul__(other Object) (Object, error) {
	if b, ok := convertToInt(other); ok {
		m := len(l)
		n := int(b) * m
		if n < 0 {
			n = 0
		}
		newTuple := make(Tuple, n)
		for i := 0; i < n; i += m {
			copy(newTuple[i:i+m], l)
		}
		return newTuple, nil
	}
	return NotImplemented, nil
}

func (a Tuple) M__rmul__(other Object) (Object, error) {
	return a.M__mul__(other)
}

func (a Tuple) M__imul__(other Object) (Object, error) {
	return a.M__mul__(other)
}

func (a Tuple) M__eq__(other Object) (Object, error) {
	b, ok := other.(Tuple)
	if !ok {
		return NotImplemented, nil
	}
	if len(a) != len(b) {
		return False, nil
	}
	for i := range a {
		eq, err := Eq(a[i], b[i])
		if err != nil {
			return nil, err
		}
		if eq == False {
			return False, nil
		}
	}
	return True, nil
}

func (a Tuple) M__ne__(other Object) (Object, error) {
	b, ok := other.(Tuple)
	if !ok {
		return NotImplemented, nil
	}
	if len(a) != len(b) {
		return True, nil
	}
	for i := range a {
		eq, err := Eq(a[i], b[i])
		if err != nil {
			return nil, err
		}
		if eq == False {
			return True, nil
		}
	}
	return False, nil
}

// tupleOrder returns -1, 0 or 1 for a compared element-wise to b, matching
// CPython's tuple lexicographic ordering: the first non-equal element decides,
// and a proper prefix is smaller.
func tupleOrder(a, b Tuple) (int, error) {
	for i := 0; i < len(a) && i < len(b); i++ {
		lt, err := Lt(a[i], b[i])
		if err != nil {
			return 0, err
		}
		if lt == True {
			return -1, nil
		}
		gt, err := Gt(a[i], b[i])
		if err != nil {
			return 0, err
		}
		if gt == True {
			return 1, nil
		}
	}
	switch {
	case len(a) < len(b):
		return -1, nil
	case len(a) > len(b):
		return 1, nil
	}
	return 0, nil
}

func (a Tuple) M__lt__(other Object) (Object, error) {
	b, ok := other.(Tuple)
	if !ok {
		return NotImplemented, nil
	}
	ord, err := tupleOrder(a, b)
	if err != nil {
		return nil, err
	}
	return Bool(ord < 0), nil
}

func (a Tuple) M__le__(other Object) (Object, error) {
	b, ok := other.(Tuple)
	if !ok {
		return NotImplemented, nil
	}
	ord, err := tupleOrder(a, b)
	if err != nil {
		return nil, err
	}
	return Bool(ord <= 0), nil
}

func (a Tuple) M__gt__(other Object) (Object, error) {
	b, ok := other.(Tuple)
	if !ok {
		return NotImplemented, nil
	}
	ord, err := tupleOrder(a, b)
	if err != nil {
		return nil, err
	}
	return Bool(ord > 0), nil
}

func (a Tuple) M__ge__(other Object) (Object, error) {
	b, ok := other.(Tuple)
	if !ok {
		return NotImplemented, nil
	}
	ord, err := tupleOrder(a, b)
	if err != nil {
		return nil, err
	}
	return Bool(ord >= 0), nil
}

// Check interface is satisfied
var _ sequenceArithmetic = Tuple(nil)
var _ I__str__ = Tuple(nil)
var _ I__repr__ = Tuple(nil)
var _ I__len__ = Tuple(nil)
var _ I__lt__ = Tuple(nil)
var _ I__le__ = Tuple(nil)
var _ I__gt__ = Tuple(nil)
var _ I__ge__ = Tuple(nil)
var _ I__bool__ = Tuple(nil)
var _ I__iter__ = Tuple(nil)
var _ I__getitem__ = Tuple(nil)
var _ I__eq__ = Tuple(nil)
var _ I__ne__ = Tuple(nil)

// var _ richComparison = Tuple(nil)

// tupleItems is the sequence behind self: a Tuple, or the payload of an
// instance of a tuple subclass (a namedtuple record, say).
func tupleItems(self Object) ([]Object, error) {
	if t, ok := self.(Tuple); ok {
		return t, nil
	}
	return SequenceTuple(self)
}

func init() {
	TupleType.Dict.Set("index", MustNewMethod("index", func(self Object, args Tuple) (Object, error) {
		items, err := tupleItems(self)
		if err != nil {
			return nil, err
		}
		return seqIndex("tuple", items, args)
	}, 0, "index(value, [start, [stop]]) -> integer -- return first index of value."))

	TupleType.Dict.Set("count", MustNewMethod("count", func(self Object, args Tuple) (Object, error) {
		items, err := tupleItems(self)
		if err != nil {
			return nil, err
		}
		return seqCount("tuple", items, args)
	}, 0, "count(value) -> integer -- return number of occurrences of value."))
}
