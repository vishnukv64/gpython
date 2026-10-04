// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// List objects

package py

import (
	"sort"
	"sync"
)

var ListType = ObjectType.NewType("list", "list() -> new empty list\nlist(iterable) -> new list initialized from iterable's items", ListNew, nil)

// FIXME lists are mutable so this should probably be struct { Tuple } then can use the sub methods on Tuple
type List struct {
	Items []Object

	// mu serialises the mutating methods.
	//
	// CPython's list.append is atomic in practice because of the GIL, and
	// real code leans on that: several threads appending to one list is an
	// ordinary pattern.  Without a lock here the interpreter loses data -
	// measured, 8 threads appending 200 items each lost between 14 and 404
	// of 1600 in EVERY one of 20 runs, because "Items = append(Items, x)"
	// reads the length, appends and writes it back, and two goroutines
	// interleave those steps.
	//
	// A per-list lock restores the observable behaviour.  It is not a GIL:
	// two threads still run py code simultaneously, so a compound update
	// ("x = x + 1" on a shared global, or two DIFFERENT containers) is still
	// racy in the way any unsynchronised code is.  This makes the container
	// operations themselves safe, which is what CPython gives you.
	mu sync.Mutex
}

func init() {
	// list REFUSES subclasses, deliberately, because a subclass cannot work.
	//
	// In CPython "class L(list): pass" is ordinary.  Here an instance of a
	// python-level class is represented as a *Type whose namespace is a DICT -
	// which is exactly what a dict subclass needs, so dict subclassing works -
	// list IS subclassable.  It used to be refused, because an instance of a
	// python-level class is a *Type whose namespace is a dict and a sequence has
	// positional items with nowhere to put them: "class L(list): pass; L([1,2])"
	// produced an object whose len() raised.  Type.Payload now carries the value,
	// and the container protocols read it, so the refusal is gone.

	// FIXME: all methods should be callable using list.method([], *args, **kwargs) or [].method(*args, **kwargs)
	ListType.Dict.Set("append", MustNewMethod("append", func(self Object, args Tuple) (Object, error) {
		listSelf := self.(*List)
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "append() takes exactly one argument (%d given)", len(args))
		}
		listSelf.mu.Lock()
		defer listSelf.mu.Unlock()
		listSelf.Items = append(listSelf.Items, args[0])
		return NoneType{}, nil
	}, 0, "append(item)"))

	ListType.Dict.Set("extend", MustNewMethod("extend", func(self Object, args Tuple) (Object, error) {
		listSelf := self.(*List)
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "extend() takes exactly one argument (%d given)", len(args))
		}
		// ANY iterable, not just a list.  The fast path copies a list's items
		// directly; everything else - a tuple, a generator, a python-level
		// object with __iter__ - is walked with the iterator protocol.
		//
		// Before this, anything that was not a *List was SILENTLY IGNORED, so
		// "lines.extend(other_lines)" added nothing and returned None.  That
		// broke rich's text wrapping, which is extend over a Lines object.
		if err := appendIterable(listSelf, args[0]); err != nil {
			return nil, err
		}
		return NoneType{}, nil
	}, 0, "extend([item])"))

	ListType.Dict.Set("sort", MustNewMethod("sort", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		const funcName = "sort"
		l, isList := self.(*List)
		if !isList {
			// method called using `list.sort([], **kwargs)`
			var o Object
			err := UnpackTuple(args, NewStringDict(), funcName, 1, 1, &o)
			if err != nil {
				return nil, err
			}
			var ok bool
			l, ok = o.(*List)
			if !ok {
				return nil, ExceptionNewf(TypeError, "descriptor 'sort' requires a 'list' object but received a '%s'", o.Type())
			}
		} else {
			// method called using `[].sort(**kargs)`
			err := UnpackTuple(args, NewStringDict(), funcName, 0, 0)
			if err != nil {
				return nil, err
			}
		}
		err := SortInPlace(l, kwargs, funcName)
		if err != nil {
			return nil, err
		}
		return NoneType{}, nil
	}, 0, "sort(key=None, reverse=False)"))

	ListType.Dict.Set("pop", MustNewMethod("pop", func(self Object, args Tuple) (Object, error) {
		l := self.(*List)
		if len(args) > 1 {
			return nil, ExceptionNewf(TypeError, "pop expected at most 1 argument, got %d", len(args))
		}
		// The index is converted before the list is inspected, so an empty
		// list with a bad index reports the index error, as CPython does.
		i := len(l.Items) - 1
		if len(args) == 1 {
			var err error
			i, err = toIndex(args[0])
			if err != nil {
				return nil, err
			}
		}
		n := len(l.Items)
		if n == 0 {
			return nil, ExceptionNewf(IndexError, "pop from empty list")
		}
		if i < 0 {
			i += n
		}
		if i < 0 || i >= n {
			return nil, ExceptionNewf(IndexError, "pop index out of range")
		}
		item := l.Items[i]
		l.DelItem(i)
		return item, nil
	}, 0, "pop([index]) -> item -- remove and return item at index (default last)."))

	ListType.Dict.Set("remove", MustNewMethod("remove", func(self Object, args Tuple) (Object, error) {
		l := self.(*List)
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "list.remove() takes exactly one argument (%d given)", len(args))
		}
		for i, item := range l.Items {
			eq, err := Eq(item, args[0])
			if err != nil {
				return nil, err
			}
			if eq == True {
				l.DelItem(i)
				return None, nil
			}
		}
		return nil, ExceptionNewf(ValueError, "list.remove(x): x not in list")
	}, 0, "remove(value) -- remove first occurrence of value."))

	ListType.Dict.Set("insert", MustNewMethod("insert", func(self Object, args Tuple) (Object, error) {
		l := self.(*List)
		if len(args) != 2 {
			return nil, ExceptionNewf(TypeError, "insert expected 2 arguments, got %d", len(args))
		}
		i, err := toIndex(args[0])
		if err != nil {
			return nil, err
		}
		n := len(l.Items)
		// Clamp like CPython: a negative index counts from the end and is
		// still clamped at 0, an index past the end appends.
		if i < 0 {
			i += n
			if i < 0 {
				i = 0
			}
		} else if i > n {
			i = n
		}
		l.Items = append(l.Items, nil)
		copy(l.Items[i+1:], l.Items[i:])
		l.Items[i] = args[1]
		return None, nil
	}, 0, "insert(index, object) -- insert object before index."))

	ListType.Dict.Set("index", MustNewMethod("index", func(self Object, args Tuple) (Object, error) {
		l := self.(*List)
		if len(args) < 1 {
			return nil, ExceptionNewf(TypeError, "index expected at least 1 argument, got %d", len(args))
		}
		if len(args) > 3 {
			return nil, ExceptionNewf(TypeError, "index expected at most 3 arguments, got %d", len(args))
		}
		n := len(l.Items)
		start, stop := 0, n
		if len(args) >= 2 {
			var err error
			start, err = sliceIndex(args[1], n, 0)
			if err != nil {
				return nil, err
			}
		}
		if len(args) >= 3 {
			var err error
			stop, err = sliceIndex(args[2], n, n)
			if err != nil {
				return nil, err
			}
		}
		for i := start; i < stop; i++ {
			eq, err := Eq(l.Items[i], args[0])
			if err != nil {
				return nil, err
			}
			if eq == True {
				return Int(i), nil
			}
		}
		return nil, ExceptionNewf(ValueError, "list.index(x): x not in list")
	}, 0, "index(value, [start, [stop]]) -> integer -- return first index of value."))

	ListType.Dict.Set("count", MustNewMethod("count", func(self Object, args Tuple) (Object, error) {
		l := self.(*List)
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "list.count() takes exactly one argument (%d given)", len(args))
		}
		count := 0
		for _, item := range l.Items {
			eq, err := Eq(item, args[0])
			if err != nil {
				return nil, err
			}
			if eq == True {
				count++
			}
		}
		return Int(count), nil
	}, 0, "count(value) -> integer -- return number of occurrences of value."))

	ListType.Dict.Set("reverse", MustNewMethod("reverse", func(self Object, args Tuple) (Object, error) {
		l := self.(*List)
		if err := methodNoArgs("list.reverse", args); err != nil {
			return nil, err
		}
		for i, j := 0, len(l.Items)-1; i < j; i, j = i+1, j-1 {
			l.Items[i], l.Items[j] = l.Items[j], l.Items[i]
		}
		return None, nil
	}, 0, "reverse() -- reverse *IN PLACE*."))

	ListType.Dict.Set("clear", MustNewMethod("clear", func(self Object, args Tuple) (Object, error) {
		l := self.(*List)
		if err := methodNoArgs("list.clear", args); err != nil {
			return nil, err
		}
		l.Items = nil
		return None, nil
	}, 0, "clear() -- remove all items from list."))

	ListType.Dict.Set("copy", MustNewMethod("copy", func(self Object, args Tuple) (Object, error) {
		l := self.(*List)
		if err := methodNoArgs("list.copy", args); err != nil {
			return nil, err
		}
		return l.Copy(), nil
	}, 0, "copy() -> a shallow copy of the list."))

}

// methodNoArgs checks that a method that takes no arguments was given none,
// raising the TypeError CPython raises for it.
func methodNoArgs(qualname string, args Tuple) error {
	if len(args) != 0 {
		return ExceptionNewf(TypeError, "%s() takes no arguments (%d given)", qualname, len(args))
	}
	return nil
}

// toIndex converts an object used as an index or a repeat count into a Go int.
//
// An object with no __index__ is rejected with CPython's message rather than
// the generic one Index itself produces; an object that has __index__ but
// returns a non-integer keeps Index's more specific complaint.
func toIndex(v Object) (int, error) {
	if _, ok := v.(I__index__); !ok && v.Type().GetAttrOrNil("__index__") == nil {
		return 0, ExceptionNewf(TypeError, "'%s' object cannot be interpreted as an integer", v.Type().Name)
	}
	return IndexInt(v)
}

// sliceIndex clamps a start or stop argument of index() the way CPython
// clips the bounds of a slice against a sequence of length n.
func sliceIndex(v Object, n int, deflt int) (int, error) {
	if v == nil {
		return deflt, nil
	}
	i, err := toIndex(v)
	if err != nil {
		return 0, ExceptionNewf(TypeError, "slice indices must be integers or have an __index__ method")
	}
	if i < 0 {
		i += n
		if i < 0 {
			i = 0
		}
	}
	if i > n {
		i = n
	}
	return i, nil
}

// Type of this List object
func (o *List) Type() *Type {
	return ListType
}

// ListNew
func ListNew(metatype *Type, args Tuple, kwargs StringDict) (res Object, err error) {
	var iterable Object
	err = UnpackTuple(args, kwargs, "list", 0, 1, &iterable)
	if err != nil {
		return nil, err
	}
	if iterable != nil {
		return SequenceList(iterable)
	}
	return NewList(), nil
}

// Make a new empty list
func NewList() *List {
	return &List{}
}

// Make a new empty list with given capacity
func NewListWithCapacity(n int) *List {
	l := &List{}
	if n != 0 {
		l.Items = make([]Object, 0, n)
	}
	return l
}

// Make a list with n nil elements
func NewListSized(n int) *List {
	l := &List{}
	if n != 0 {
		l.Items = make([]Object, n)
	}
	return l
}

// Make a new list from an []Object
//
// The []Object is copied into the list
func NewListFromItems(items []Object) *List {
	l := NewListSized(len(items))
	copy(l.Items, items)
	return l
}

// Makes an argv into a tuple
func NewListFromStrings(items []string) *List {
	l := NewListSized(len(items))
	for i, v := range items {
		l.Items[i] = String(v)
	}
	return l
}

// Copy a list object
func (l *List) Copy() *List {
	return NewListFromItems(l.Items)
}

// Append an item
func (l *List) Append(item Object) {
	l.Items = append(l.Items, item)
}

// Resize the list
func (l *List) Resize(newSize int) {
	l.Items = l.Items[:newSize]
}

// Extend the list with items
func (l *List) Extend(items []Object) {
	l.Items = append(l.Items, items...)
}

// Extend the list with strings
func (l *List) ExtendWithStrings(items []string) {
	for _, item := range items {
		l.Items = append(l.Items, Object(String(item)))
	}
}

// Extends the list with the sequence passed in
func (l *List) ExtendSequence(seq Object) error {
	return Iterate(seq, func(item Object) bool {
		l.Append(item)
		return false
	})
}

// Len of list
func (l *List) Len() int {
	return len(l.Items)
}

func (l *List) M__str__() (Object, error) {
	return l.M__repr__()
}

// listOrder compares two lists elementwise, returning -1, 0 or 1.
//
// Same rule as tupleOrder, and CPython has the same rule for both: the first
// pair that differs decides, comparing with < and > so a user type's own
// ordering methods are consulted; if one is a prefix of the other, the shorter
// is less.  Tuples had this and lists did not, so "[1, 2] < [1, 3]" raised
// "unsupported operand type(s) for <" while the tuple spelling worked.
func listOrder(a, b *List) (int, error) {
	n := len(a.Items)
	if len(b.Items) < n {
		n = len(b.Items)
	}
	for i := 0; i < n; i++ {
		lt, err := Lt(a.Items[i], b.Items[i])
		if err != nil {
			return 0, err
		}
		if lt == True {
			return -1, nil
		}
		gt, err := Gt(a.Items[i], b.Items[i])
		if err != nil {
			return 0, err
		}
		if gt == True {
			return 1, nil
		}
	}
	switch {
	case len(a.Items) < len(b.Items):
		return -1, nil
	case len(a.Items) > len(b.Items):
		return 1, nil
	}
	return 0, nil
}

func (l *List) M__lt__(other Object) (Object, error) {
	b, ok := other.(*List)
	if !ok {
		return NotImplemented, nil
	}
	ord, err := listOrder(l, b)
	if err != nil {
		return nil, err
	}
	return Bool(ord < 0), nil
}

func (l *List) M__le__(other Object) (Object, error) {
	b, ok := other.(*List)
	if !ok {
		return NotImplemented, nil
	}
	ord, err := listOrder(l, b)
	if err != nil {
		return nil, err
	}
	return Bool(ord <= 0), nil
}

func (l *List) M__gt__(other Object) (Object, error) {
	b, ok := other.(*List)
	if !ok {
		return NotImplemented, nil
	}
	ord, err := listOrder(l, b)
	if err != nil {
		return nil, err
	}
	return Bool(ord > 0), nil
}

func (l *List) M__ge__(other Object) (Object, error) {
	b, ok := other.(*List)
	if !ok {
		return NotImplemented, nil
	}
	ord, err := listOrder(l, b)
	if err != nil {
		return nil, err
	}
	return Bool(ord >= 0), nil
}

func (l *List) M__repr__() (Object, error) {
	return Tuple(l.Items).repr("[", "]")
}

func (l *List) M__len__() (Object, error) {
	return Int(len(l.Items)), nil
}

func (l *List) M__bool__() (Object, error) {
	return NewBool(len(l.Items) > 0), nil
}

func (l *List) M__iter__() (Object, error) {
	return NewIterator(Tuple(l.Items)), nil
}

// appendIterable appends every item of an arbitrary iterable to a list.
//
// The iterator protocol is the definition of "iterable" - an object is one
// because it has __iter__, not because it is a particular Go type - so a
// generator and a python-level sequence both work here.
func appendIterable(l *List, source Object) error {
	if oList, ok := source.(*List); ok {
		// A list is the common case and its items need no iteration.
		l.Items = append(l.Items, oList.Items...)
		return nil
	}
	iter, err := Iter(source)
	if err != nil {
		return err
	}
	for {
		item, err := Next(iter)
		if err != nil {
			if IsException(StopIteration, err) {
				return nil
			}
			return err
		}
		l.Items = append(l.Items, item)
	}
}

func (l *List) M__getitem__(key Object) (Object, error) {
	if slice, ok := key.(*Slice); ok {
		start, _, step, slicelength, err := slice.GetIndices(len(l.Items))
		if err != nil {
			return nil, err
		}
		newList := NewListSized(slicelength)
		for i, j := start, 0; j < slicelength; i, j = i+step, j+1 {
			newList.Items[j] = l.Items[i]
		}
		return newList, nil
	}
	i, err := IndexIntCheckNamed(key, len(l.Items), "list")
	if err != nil {
		return nil, err
	}
	return l.Items[i], nil
}

func (l *List) M__setitem__(key, value Object) (Object, error) {
	if slice, ok := key.(*Slice); ok {
		start, stop, step, slicelength, err := slice.GetIndices(len(l.Items))
		if err != nil {
			return nil, err
		}
		if step == 1 {
			// Make a copy of the tail
			tailSlice := l.Items[stop:]
			tail := make([]Object, len(tailSlice))
			copy(tail, tailSlice)
			l.Items = l.Items[:start]
			err = l.ExtendSequence(value)
			if err != nil {
				return nil, err
			}
			l.Items = append(l.Items, tail...)
		} else {
			newItems, err := SequenceTuple(value)
			if err != nil {
				return nil, err
			}
			if len(newItems) != slicelength {
				return nil, ExceptionNewf(ValueError, "attempt to assign sequence of size %d to extended slice of size %d", len(newItems), slicelength)
			}
			j := 0
			for i := start; i < stop; i += step {
				l.Items[i] = newItems[j]
				j++
			}
		}
	} else {
		i, err := IndexIntCheckNamed(key, len(l.Items), "list")
		if err != nil {
			return nil, err
		}
		l.Items[i] = value
	}
	return None, nil
}

// Removes the item at i
func (a *List) DelItem(i int) {
	a.Items = append(a.Items[:i], a.Items[i+1:]...)
}

// Removes items from a list
func (a *List) M__delitem__(key Object) (Object, error) {
	if slice, ok := key.(*Slice); ok {
		start, stop, step, _, err := slice.GetIndices(len(a.Items))
		if err != nil {
			return nil, err
		}
		if step == 1 {
			a.Items = append(a.Items[:start], a.Items[stop:]...)
		} else {
			j := 0
			for i := start; i < stop; i += step {
				a.DelItem(i - j)
				j++
			}
		}
	} else {
		i, err := IndexIntCheck(key, len(a.Items))
		if err != nil {
			return nil, err
		}
		a.DelItem(i)
	}
	return None, nil
}

func (a *List) M__add__(other Object) (Object, error) {
	if b, ok := other.(*List); ok {
		newList := NewListSized(len(a.Items) + len(b.Items))
		copy(newList.Items, a.Items)
		copy(newList.Items[len(a.Items):], b.Items)
		return newList, nil
	}
	return NotImplemented, nil
}

func (a *List) M__radd__(other Object) (Object, error) {
	if b, ok := other.(*List); ok {
		return b.M__add__(a)
	}
	return NotImplemented, nil
}

func (a *List) M__iadd__(other Object) (Object, error) {
	if b, ok := other.(*List); ok {
		a.Extend(b.Items)
		return a, nil
	}
	return NotImplemented, nil
}

func (l *List) M__mul__(other Object) (Object, error) {
	if b, ok := convertToInt(other); ok {
		m := len(l.Items)
		n := int(b) * m
		if n < 0 {
			n = 0
		}
		newList := NewListSized(n)
		for i := 0; i < n; i += m {
			copy(newList.Items[i:i+m], l.Items)
		}
		return newList, nil
	}
	return NotImplemented, nil
}

func (a *List) M__rmul__(other Object) (Object, error) {
	return a.M__mul__(other)
}

func (a *List) M__contains__(item Object) (Object, error) {
	for _, x := range a.Items {
		eq, err := Eq(x, item)
		if err != nil {
			return nil, err
		}
		if eq == True {
			return True, nil
		}
	}
	return False, nil
}

func (a *List) M__imul__(other Object) (Object, error) {
	return a.M__mul__(other)
}

// Check interface is satisfied
var _ sequenceArithmetic = (*List)(nil)
var _ I__str__ = (*List)(nil)
var _ I__repr__ = (*List)(nil)
var _ I__len__ = (*List)(nil)
var _ I__len__ = (*List)(nil)
var _ I__bool__ = (*List)(nil)
var _ I__iter__ = (*List)(nil)
var _ I__getitem__ = (*List)(nil)
var _ I__setitem__ = (*List)(nil)
var _ I__contains__ = (*List)(nil)

// var _ richComparison = (*List)(nil)

func (a *List) M__eq__(other Object) (Object, error) {
	b, ok := other.(*List)
	if !ok {
		return NotImplemented, nil
	}
	if len(a.Items) != len(b.Items) {
		return False, nil
	}
	for i := range a.Items {
		eq, err := Eq(a.Items[i], b.Items[i])
		if err != nil {
			return nil, err
		}
		if eq == False {
			return False, nil
		}
	}
	return True, nil
}

func (a *List) M__ne__(other Object) (Object, error) {
	b, ok := other.(*List)
	if !ok {
		return NotImplemented, nil
	}
	if len(a.Items) != len(b.Items) {
		return True, nil
	}
	for i := range a.Items {
		eq, err := Eq(a.Items[i], b.Items[i])
		if err != nil {
			return nil, err
		}
		if eq == False {
			return True, nil
		}
	}
	return False, nil
}

type sortable struct {
	l        *List
	keyFunc  Object
	reverse  bool
	firstErr error
}

type ptrSortable struct {
	s *sortable
}

func (s ptrSortable) Len() int {
	return s.s.l.Len()
}

func (s ptrSortable) Swap(i, j int) {
	itemI, err := s.s.l.M__getitem__(Int(i))
	if err != nil {
		if s.s.firstErr == nil {
			s.s.firstErr = err
		}
		return
	}
	itemJ, err := s.s.l.M__getitem__(Int(j))
	if err != nil {
		if s.s.firstErr == nil {
			s.s.firstErr = err
		}
		return
	}
	_, err = s.s.l.M__setitem__(Int(i), itemJ)
	if err != nil {
		if s.s.firstErr == nil {
			s.s.firstErr = err
		}
	}
	_, err = s.s.l.M__setitem__(Int(j), itemI)
	if err != nil {
		if s.s.firstErr == nil {
			s.s.firstErr = err
		}
	}
}

func (s ptrSortable) Less(i, j int) bool {
	itemI, err := s.s.l.M__getitem__(Int(i))
	if err != nil {
		if s.s.firstErr == nil {
			s.s.firstErr = err
		}
		return false
	}
	itemJ, err := s.s.l.M__getitem__(Int(j))
	if err != nil {
		if s.s.firstErr == nil {
			s.s.firstErr = err
		}
		return false
	}

	if s.s.keyFunc != None {
		itemI, err = Call(s.s.keyFunc, Tuple{itemI}, NewStringDict())
		if err != nil {
			if s.s.firstErr == nil {
				s.s.firstErr = err
			}
			return false
		}
		itemJ, err = Call(s.s.keyFunc, Tuple{itemJ}, NewStringDict())
		if err != nil {
			if s.s.firstErr == nil {
				s.s.firstErr = err
			}
			return false
		}
	}

	var cmpResult Object
	if s.s.reverse {
		cmpResult, err = Lt(itemJ, itemI)
	} else {
		cmpResult, err = Lt(itemI, itemJ)
	}

	if err != nil {
		if s.s.firstErr == nil {
			s.s.firstErr = err
		}
		return false
	}

	if boolResult, ok := cmpResult.(Bool); ok {
		return bool(boolResult)
	}

	return false
}

// SortInPlace sorts the given List in place using a stable sort.
// kwargs can have the keys "key" and "reverse".
func SortInPlace(l *List, kwargs StringDict, funcName string) error {
	var keyFunc Object
	var reverse Object
	err := ParseTupleAndKeywords(nil, kwargs, "|$OO:"+funcName, []string{"key", "reverse"}, &keyFunc, &reverse)
	if err != nil {
		return err
	}
	if keyFunc == nil {
		keyFunc = None
	}
	if reverse == nil {
		reverse = False
	}
	// FIXME: requires the same bool-check like CPython (or better "|$Op" that doesn't panic on nil).
	ok, err := ObjectIsTrue(reverse)
	if err != nil {
		return err
	}
	s := ptrSortable{&sortable{l, keyFunc, ok, nil}}
	sort.Stable(s)
	return s.s.firstErr
}
