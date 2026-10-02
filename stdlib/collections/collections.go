// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package collections provides the implementation of python's 'collections'
// module.
//
// The types here are implemented natively rather than as Python subclasses
// of dict and tuple, because this interpreter does not allow dict or tuple
// to be subclassed ("not an acceptable base type"), and Python's own
// implementations are all subclasses.  They therefore behave like the
// CPython types rather than literally sharing their storage.
package collections

import (
	"sort"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `This module implements specialized container datatypes providing
alternatives to Python's general purpose built-in containers.

* namedtuple   factory function for creating tuple subclasses with named fields
* deque        list-like container with fast appends and pops on either end
* ChainMap     dict-like class for creating a single view of multiple mappings
* Counter      dict subclass for counting hashable objects
* OrderedDict  dict subclass that remembers the order entries were added
* defaultdict  dict subclass that calls a factory function to supply missing values
* UserDict     wrapper around dictionary objects for easier dict subclassing
* UserList     wrapper around list objects for easier list subclassing
* UserString   wrapper around string objects for easier string subclassing`

// newIteratorFromItems returns an iterator over a copy of the items.
func newIteratorFromItems(items []py.Object) *py.Iterator {
	tuple := make(py.Tuple, len(items))
	copy(tuple, items)
	return py.NewIterator(tuple)
}

// ---------------------------------------------------------------------------
// deque

const deque_doc = `deque([iterable[, maxlen]]) --> deque object

A list-like sequence optimized for data accesses near its endpoints.`

// Deque is a double-ended queue.
//
// This is the Go container/list style doubly linked list rather than a
// circular buffer: the access patterns that matter here are the ends, and a
// linked list gives O(1) for both without the resize logic.
type Deque struct {
	items  []py.Object
	maxlen int // -1 when unbounded
}

var DequeType = py.NewTypeX("collections.deque", deque_doc, DequeNew, nil)

func (d *Deque) Type() *py.Type { return DequeType }

func (d *Deque) M__len__() (py.Object, error) { return py.Int(len(d.items)), nil }

// append and trim in one place so maxlen is enforced everywhere.
func (d *Deque) add(item py.Object, front bool) {
	if front {
		d.items = append([]py.Object{item}, d.items...)
	} else {
		d.items = append(d.items, item)
	}
	if d.maxlen >= 0 {
		for len(d.items) > d.maxlen {
			if front {
				d.items = d.items[:len(d.items)-1]
			} else {
				d.items = d.items[1:]
			}
		}
	}
}

func DequeNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		iterable py.Object = py.None
		maxlen   py.Object = py.None
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "|Oi:deque", []string{"iterable", "maxlen"}, &iterable, &maxlen)
	if err != nil {
		return nil, err
	}
	d := &Deque{maxlen: -1}
	if maxlen != py.None {
		n, err := py.IndexInt(maxlen)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			n = -1 // deque(maxlen=-1) is unbounded, as in CPython
		}
		d.maxlen = n
	}
	if iterable != py.None {
		items, err := py.SequenceList(iterable)
		if err != nil {
			return nil, err
		}
		for _, item := range items.Items {
			d.add(item, false)
		}
	}
	return d, nil
}

func init() {
	DequeType.Dict.Set("append", py.MustNewMethod("append", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		var item py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "append", 1, 1, &item); err != nil {
			return nil, err
		}
		d.add(item, false)
		return py.None, nil
	}, 0, "Add an element to the right side of the deque."))

	DequeType.Dict.Set("appendleft", py.MustNewMethod("appendleft", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		var item py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "appendleft", 1, 1, &item); err != nil {
			return nil, err
		}
		d.add(item, true)
		return py.None, nil
	}, 0, "Add an element to the left side of the deque."))

	DequeType.Dict.Set("pop", py.MustNewMethod("pop", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		if err := py.UnpackTuple(args, py.StringDict{}, "pop", 0, 0); err != nil {
			return nil, err
		}
		if len(d.items) == 0 {
			return nil, py.ExceptionNewf(py.IndexError, "pop from an empty deque")
		}
		item := d.items[len(d.items)-1]
		d.items = d.items[:len(d.items)-1]
		return item, nil
	}, 0, "Remove and return the rightmost element."))

	DequeType.Dict.Set("popleft", py.MustNewMethod("popleft", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		if err := py.UnpackTuple(args, py.StringDict{}, "popleft", 0, 0); err != nil {
			return nil, err
		}
		if len(d.items) == 0 {
			return nil, py.ExceptionNewf(py.IndexError, "pop from an empty deque")
		}
		item := d.items[0]
		d.items = d.items[1:]
		return item, nil
	}, 0, "Remove and return the leftmost element."))

	DequeType.Dict.Set("extend", py.MustNewMethod("extend", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		var iterable py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "extend", 1, 1, &iterable); err != nil {
			return nil, err
		}
		items, err := py.SequenceList(iterable)
		if err != nil {
			return nil, err
		}
		for _, item := range items.Items {
			d.add(item, false)
		}
		return py.None, nil
	}, 0, "Extend the right side of the deque with elements from the iterable."))

	DequeType.Dict.Set("extendleft", py.MustNewMethod("extendleft", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		var iterable py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "extendleft", 1, 1, &iterable); err != nil {
			return nil, err
		}
		items, err := py.SequenceList(iterable)
		if err != nil {
			return nil, err
		}
		for _, item := range items.Items {
			d.add(item, true)
		}
		return py.None, nil
	}, 0, "Extend the left side of the deque with elements from the iterable."))

	DequeType.Dict.Set("clear", py.MustNewMethod("clear", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		if err := py.UnpackTuple(args, py.StringDict{}, "clear", 0, 0); err != nil {
			return nil, err
		}
		d.items = nil
		return py.None, nil
	}, 0, "Remove all elements from the deque."))

	DequeType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		if err := py.UnpackTuple(args, py.StringDict{}, "__iter__", 0, 0); err != nil {
			return nil, err
		}
		return newIteratorFromItems(d.items), nil
	}, 0, "Implement iter(self)."))

	DequeType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		i, err := py.IndexInt(key)
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "sequence index must be integer, not '%s'", key.Type().Name)
		}
		if i < 0 {
			i += len(d.items)
		}
		if i < 0 || i >= len(d.items) {
			return nil, py.ExceptionNewf(py.IndexError, "deque index out of range")
		}
		return d.items[i], nil
	}, 0, "Return self[index]."))

	DequeType.Dict.Set("__setitem__", py.MustNewMethod("__setitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		var key, value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__setitem__", 2, 2, &key, &value); err != nil {
			return nil, err
		}
		i, err := py.IndexInt(key)
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "sequence index must be integer, not '%s'", key.Type().Name)
		}
		if i < 0 {
			i += len(d.items)
		}
		if i < 0 || i >= len(d.items) {
			return nil, py.ExceptionNewf(py.IndexError, "deque index out of range")
		}
		d.items[i] = value
		return py.None, nil
	}, 0, "Set self[index] to value."))

	DequeType.Dict.Set("__contains__", py.MustNewMethod("__contains__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		var item py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__contains__", 1, 1, &item); err != nil {
			return nil, err
		}
		for _, have := range d.items {
			eq, err := py.Eq(have, item)
			if err != nil {
				return nil, err
			}
			if eq == py.True {
				return py.True, nil
			}
		}
		return py.False, nil
	}, 0, "Implement 'in'."))

	DequeType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*Deque)
		if err := py.UnpackTuple(args, py.StringDict{}, "__repr__", 0, 0); err != nil {
			return nil, err
		}
		parts, err := reprItems(d.items)
		if err != nil {
			return nil, err
		}
		out := "deque([" + strings.Join(parts, ", ") + "])"
		if d.maxlen >= 0 {
			out = "deque([" + strings.Join(parts, ", ") + "], maxlen=" + itoa(d.maxlen) + ")"
		}
		return py.String(out), nil
	}, 0, "Return repr(self)."))
}

func reprItems(items []py.Object) ([]string, error) {
	out := make([]string, len(items))
	for i, item := range items {
		s, err := py.ReprAsString(item)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

func itoa(i int) string {
	s, _ := py.StrAsString(py.Int(i))
	return s
}

// ---------------------------------------------------------------------------
// Counter

const counter_doc = `Dict subclass for counting hashable items.  Sometimes called a bag
or multiset.  Elements are stored as dictionary keys and their counts
are stored as dictionary values.`

// Counter maps elements to counts, keeping the insertion order of the
// elements as CPython's does.
type Counter struct {
	keys   []string // encoded keys, in insertion order
	values []py.Object
	index  map[string]int // encoded key -> position in keys/values
}

var CounterType = py.NewTypeX("collections.Counter", counter_doc, counterNew, nil)

func (c *Counter) Type() *py.Type { return CounterType }

func newCounter() *Counter {
	return &Counter{index: map[string]int{}}
}

func (c *Counter) get(key py.Object) (py.Object, bool, error) {
	encoded, err := py.DictKey(key)
	if err != nil {
		return nil, false, nil // unhashable keys simply are not present
	}
	i, ok := c.index[encoded]
	if !ok {
		return nil, false, nil
	}
	return c.values[i], true, nil
}

func (c *Counter) set(key, value py.Object) error {
	encoded, err := py.DictKey(key)
	if err != nil {
		return err
	}
	if i, ok := c.index[encoded]; ok {
		c.values[i] = value
		return nil
	}
	c.index[encoded] = len(c.keys)
	c.keys = append(c.keys, encoded)
	c.values = append(c.values, value)
	return nil
}

func (c *Counter) order() []py.Object {
	out := make([]py.Object, 0, len(c.keys))
	for _, encoded := range c.keys {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			continue
		}
		out = append(out, key)
	}
	return out
}

// count returns the integer count for a key, treating a missing key as zero.
func (c *Counter) count(key py.Object) (int64, error) {
	value, ok, err := c.get(key)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	n, err := py.IndexInt(value)
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}

func (c *Counter) add(key py.Object, delta int64) error {
	n, err := c.count(key)
	if err != nil {
		return err
	}
	return c.set(key, py.Int(n+delta))
}

func counterNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var iterable py.Object = py.None
	err := py.ParseTupleAndKeywords(args, kwargs, "|O:Counter", []string{"iterable"}, &iterable)
	if err != nil {
		return nil, err
	}
	c := newCounter()
	if iterable != py.None {
		if err := counterUpdate(c, iterable); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// counterUpdate counts the elements of an iterable, or, when given another
// Counter or a mapping, adds its values.
func counterUpdate(c *Counter, arg py.Object) error {
	if other, ok := arg.(*Counter); ok {
		for _, key := range other.order() {
			n, err := other.count(key)
			if err != nil {
				return err
			}
			if err := c.add(key, n); err != nil {
				return err
			}
		}
		return nil
	}
	items, err := py.SequenceList(arg)
	if err != nil {
		return err
	}
	for _, item := range items.Items {
		if err := c.add(item, 1); err != nil {
			return err
		}
	}
	return nil
}

// counterGetItem implements Counter[key].  A missing element counts zero,
// unlike a dict.
func counterGetItem(self py.Object, key py.Object) (py.Object, error) {
	c := self.(*Counter)
	value, ok, err := c.get(key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return py.Int(0), nil
	}
	return value, nil
}

func init() {
	CounterType.Dict.Set("most_common", py.MustNewMethod("most_common", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		var n py.Object = py.None
		if err := py.UnpackTuple(args, py.StringDict{}, "most_common", 0, 1, &n); err != nil {
			return nil, err
		}
		type pair struct {
			key   py.Object
			count int64
		}
		pairs := make([]pair, 0, len(c.keys))
		for _, key := range c.order() {
			count, err := c.count(key)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, pair{key, count})
		}
		// most_common sorts by descending count, and equal counts keep the
		// order they were first seen in, which is what CPython does.
		sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].count > pairs[j].count })
		if n != py.None {
			limit, err := py.IndexInt(n)
			if err != nil {
				return nil, err
			}
			if limit < len(pairs) {
				pairs = pairs[:limit]
			}
		}
		out := make([]py.Object, len(pairs))
		for i, p := range pairs {
			out[i] = py.Tuple{p.key, py.Int(p.count)}
		}
		return py.NewListFromItems(out), nil
	}, 0, "List the n most common elements and their counts."))

	CounterType.Dict.Set("elements", py.MustNewMethod("elements", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		if err := py.UnpackTuple(args, py.StringDict{}, "elements", 0, 0); err != nil {
			return nil, err
		}
		items := []py.Object{}
		for _, key := range c.order() {
			count, err := c.count(key)
			if err != nil {
				return nil, err
			}
			for i := int64(0); i < count; i++ {
				items = append(items, key)
			}
		}
		return newIteratorFromItems(items), nil
	}, 0, "Iterator over elements repeating each as many times as its count."))

	CounterType.Dict.Set("total", py.MustNewMethod("total", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		if err := py.UnpackTuple(args, py.StringDict{}, "total", 0, 0); err != nil {
			return nil, err
		}
		total := int64(0)
		for _, key := range c.order() {
			count, err := c.count(key)
			if err != nil {
				return nil, err
			}
			total += count
		}
		return py.Int(total), nil
	}, 0, "Sum of the counts."))

	CounterType.Dict.Set("update", py.MustNewMethod("update", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		var arg py.Object = py.None
		if err := py.UnpackTuple(args, py.StringDict{}, "update", 0, 1, &arg); err != nil {
			return nil, err
		}
		if arg != py.None {
			if err := counterUpdate(c, arg); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}, 0, "Like dict.update() but adds counts instead of replacing them."))

	CounterType.Dict.Set("get", py.MustNewMethod("get", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		var key py.Object
		var def py.Object = py.None
		if err := py.UnpackTuple(args, py.StringDict{}, "get", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		value, ok, err := c.get(key)
		if err != nil {
			return nil, err
		}
		if !ok {
			return def, nil
		}
		return value, nil
	}, 0, "Return the count for key, or the default."))

	CounterType.Dict.Set("keys", py.MustNewMethod("keys", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		if err := py.UnpackTuple(args, py.StringDict{}, "keys", 0, 0); err != nil {
			return nil, err
		}
		return py.NewListFromItems(c.order()), nil
	}, 0, "Return the element list."))

	CounterType.Dict.Set("values", py.MustNewMethod("values", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		if err := py.UnpackTuple(args, py.StringDict{}, "values", 0, 0); err != nil {
			return nil, err
		}
		out := make([]py.Object, len(c.values))
		copy(out, c.values)
		return py.NewListFromItems(out), nil
	}, 0, "Return the count list."))

	CounterType.Dict.Set("items", py.MustNewMethod("items", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		if err := py.UnpackTuple(args, py.StringDict{}, "items", 0, 0); err != nil {
			return nil, err
		}
		out := make([]py.Object, 0, len(c.keys))
		for i, encoded := range c.keys {
			key, err := py.DictKeyDecode(encoded)
			if err != nil {
				return nil, err
			}
			out = append(out, py.Tuple{key, c.values[i]})
		}
		return py.NewListFromItems(out), nil
	}, 0, "Return the (key, count) list."))

	CounterType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(len(self.(*Counter).keys)), nil
	}, 0, "Number of distinct elements."))

	CounterType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return newIteratorFromItems(self.(*Counter).order()), nil
	}, 0, "Implement iter(self)."))

	CounterType.Dict.Set("__contains__", py.MustNewMethod("__contains__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__contains__", 1, 1, &key); err != nil {
			return nil, err
		}
		_, ok, err := c.get(key)
		if err != nil {
			return nil, err
		}
		if ok {
			return py.True, nil
		}
		return py.False, nil
	}, 0, "Implement 'in'."))

	CounterType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", counterGetItem, 0, "Return the count for key, zero if absent."))

	CounterType.Dict.Set("__setitem__", py.MustNewMethod("__setitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		var key, value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__setitem__", 2, 2, &key, &value); err != nil {
			return nil, err
		}
		if err := c.set(key, value); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Set the count for key."))

	CounterType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Counter)
		if err := py.UnpackTuple(args, py.StringDict{}, "__repr__", 0, 0); err != nil {
			return nil, err
		}
		parts := make([]string, 0, len(c.keys))
		for i, encoded := range c.keys {
			key, err := py.DictKeyDecode(encoded)
			if err != nil {
				return nil, err
			}
			keyStr, err := py.ReprAsString(key)
			if err != nil {
				return nil, err
			}
			valueStr, err := py.ReprAsString(c.values[i])
			if err != nil {
				return nil, err
			}
			parts = append(parts, keyStr+": "+valueStr)
		}
		return py.String("Counter({" + strings.Join(parts, ", ") + "})"), nil
	}, 0, "Return repr(self)."))
}

// ---------------------------------------------------------------------------
// OrderedDict

const ordereddict_doc = `Dictionary that remembers insertion order`

// OrderedDict is a dict that preserves insertion order (which the plain
// dict here does not promise).
type OrderedDict struct {
	keys   []string
	values py.StringDict
}

var OrderedDictType = py.NewTypeX("collections.OrderedDict", ordereddict_doc, orderedDictNew, nil)

func (o *OrderedDict) Type() *py.Type { return OrderedDictType }

func orderedDictNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	o := &OrderedDict{values: py.NewStringDict()}
	if len(args) > 1 {
		return nil, py.ExceptionNewf(py.TypeError, "expected at most 1 argument, got %d", len(args))
	}
	if len(args) == 1 {
		pairs, err := py.SequenceList(args[0])
		if err != nil {
			return nil, err
		}
		for _, item := range pairs.Items {
			pair, ok := item.(py.Tuple)
			if !ok || len(pair) != 2 {
				return nil, py.ExceptionNewf(py.ValueError, "dictionary update sequence element is not a pair")
			}
			if err := o.set(pair[0], pair[1]); err != nil {
				return nil, err
			}
		}
	}
	for _, __e := range kwargs.Items() {
		k := __e.Key
		v := __e.Value

		if err := o.set(py.String(k), v); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func (o *OrderedDict) set(key, value py.Object) error {
	encoded, err := py.DictKey(key)
	if err != nil {
		return err
	}
	if _, ok := o.values.Get(encoded); !ok {
		o.keys = append(o.keys, encoded)
	}
	o.values.Set(encoded, value)
	return nil
}

func (o *OrderedDict) order() []py.Object {
	out := make([]py.Object, 0, len(o.keys))
	for _, encoded := range o.keys {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			continue
		}
		out = append(out, key)
	}
	return out
}

func init() {
	OrderedDictType.Dict.Set("keys", py.MustNewMethod("keys", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewListFromItems(self.(*OrderedDict).order()), nil
	}, 0, "Return the key list, in insertion order."))

	OrderedDictType.Dict.Set("values", py.MustNewMethod("values", func(self py.Object, args py.Tuple) (py.Object, error) {
		o := self.(*OrderedDict)
		out := make([]py.Object, 0, len(o.keys))
		for _, encoded := range o.keys {
			out = append(out, o.values.GetOrNil(encoded))
		}
		return py.NewListFromItems(out), nil
	}, 0, "Return the value list, in insertion order."))

	OrderedDictType.Dict.Set("items", py.MustNewMethod("items", func(self py.Object, args py.Tuple) (py.Object, error) {
		o := self.(*OrderedDict)
		out := make([]py.Object, 0, len(o.keys))
		for _, encoded := range o.keys {
			key, err := py.DictKeyDecode(encoded)
			if err != nil {
				return nil, err
			}
			out = append(out, py.Tuple{key, o.values.GetOrNil(encoded)})
		}
		return py.NewListFromItems(out), nil
	}, 0, "Return the (key, value) list, in insertion order."))

	OrderedDictType.Dict.Set("get", py.MustNewMethod("get", func(self py.Object, args py.Tuple) (py.Object, error) {
		o := self.(*OrderedDict)
		var key py.Object
		var def py.Object = py.None
		if err := py.UnpackTuple(args, py.StringDict{}, "get", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		encoded, err := py.DictKey(key)
		if err == nil {
			if v, ok := o.values.Get(encoded); ok {
				return v, nil
			}
		}
		return def, nil
	}, 0, "Return the value for key, or the default."))

	OrderedDictType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(len(self.(*OrderedDict).keys)), nil
	}, 0, "Number of entries."))

	OrderedDictType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return newIteratorFromItems(self.(*OrderedDict).order()), nil
	}, 0, "Implement iter(self)."))

	OrderedDictType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		o := self.(*OrderedDict)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		encoded, err := py.DictKey(key)
		if err == nil {
			if v, ok := o.values.Get(encoded); ok {
				return v, nil
			}
		}
		return nil, py.ExceptionNewf(py.KeyError, "%v", key)
	}, 0, "Return self[key]."))

	OrderedDictType.Dict.Set("__setitem__", py.MustNewMethod("__setitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key, value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__setitem__", 2, 2, &key, &value); err != nil {
			return nil, err
		}
		if err := self.(*OrderedDict).set(key, value); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Set self[key] to value."))

	OrderedDictType.Dict.Set("__delitem__", py.MustNewMethod("__delitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		o := self.(*OrderedDict)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__delitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		encoded, err := py.DictKey(key)
		if err != nil {
			return nil, py.ExceptionNewf(py.KeyError, "%v", key)
		}
		if _, ok := o.values.Get(encoded); !ok {
			return nil, py.ExceptionNewf(py.KeyError, "%v", key)
		}
		o.values.Del(encoded)
		for i, k := range o.keys {
			if k == encoded {
				o.keys = append(o.keys[:i], o.keys[i+1:]...)
				break
			}
		}
		return py.None, nil
	}, 0, "Delete self[key]."))

	OrderedDictType.Dict.Set("__contains__", py.MustNewMethod("__contains__", func(self py.Object, args py.Tuple) (py.Object, error) {
		o := self.(*OrderedDict)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__contains__", 1, 1, &key); err != nil {
			return nil, err
		}
		encoded, err := py.DictKey(key)
		if err != nil {
			return py.False, nil
		}
		if _, ok := o.values.Get(encoded); ok {
			return py.True, nil
		}
		return py.False, nil
	}, 0, "Implement 'in'."))

	OrderedDictType.Dict.Set("popitem", py.MustNewMethod("popitem", func(self py.Object, args py.Tuple) (py.Object, error) {
		o := self.(*OrderedDict)
		var last py.Object = py.True
		if err := py.UnpackTuple(args, py.StringDict{}, "popitem", 0, 1, &last); err != nil {
			return nil, err
		}
		if len(o.keys) == 0 {
			return nil, py.ExceptionNewf(py.KeyError, "dictionary is empty")
		}
		i := 0
		if last == py.True {
			i = len(o.keys) - 1
		}
		encoded := o.keys[i]
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			return nil, err
		}
		value := o.values.GetOrNil(encoded)
		o.values.Del(encoded)
		o.keys = append(o.keys[:i], o.keys[i+1:]...)
		return py.Tuple{key, value}, nil
	}, 0, "Remove and return a (key, value) pair."))

	OrderedDictType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		o := self.(*OrderedDict)
		parts := make([]string, 0, len(o.keys))
		for _, encoded := range o.keys {
			key, err := py.DictKeyDecode(encoded)
			if err != nil {
				return nil, err
			}
			keyStr, err := py.ReprAsString(key)
			if err != nil {
				return nil, err
			}
			valueStr, err := py.ReprAsString(o.values.GetOrNil(encoded))
			if err != nil {
				return nil, err
			}
			parts = append(parts, keyStr+": "+valueStr)
		}
		return py.String("OrderedDict({" + strings.Join(parts, ", ") + "})"), nil
	}, 0, "Return repr(self)."))
}

// ---------------------------------------------------------------------------
// defaultdict

const defaultdict_doc = `defaultdict(default_factory[, ...]) --> dict with default factory

The default factory is called without arguments to produce a new value when
a key is not present, in __getitem__ only.`

// DefaultDict supplies a value for a missing key by calling a factory.
type DefaultDict struct {
	defaultFactory py.Object
	keys           []string
	values         py.StringDict
}

var DefaultDictType = py.NewTypeX("collections.defaultdict", defaultdict_doc, defaultDictNew, nil)

func (d *DefaultDict) Type() *py.Type { return DefaultDictType }

func defaultDictNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d := &DefaultDict{values: py.NewStringDict()}
	if len(args) > 0 {
		d.defaultFactory = args[0]
	} else {
		d.defaultFactory = py.None
	}
	if len(args) > 1 {
		return nil, py.ExceptionNewf(py.TypeError, "first argument must be callable or None")
	}
	if len(args) == 2 {
		pairs, err := py.SequenceList(args[1])
		if err != nil {
			return nil, err
		}
		for _, item := range pairs.Items {
			pair, ok := item.(py.Tuple)
			if !ok || len(pair) != 2 {
				return nil, py.ExceptionNewf(py.ValueError, "dictionary update sequence element is not a pair")
			}
			if err := d.set(pair[0], pair[1]); err != nil {
				return nil, err
			}
		}
	}
	for _, __e := range kwargs.Items() {
		k := __e.Key
		v := __e.Value

		if err := d.set(py.String(k), v); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (d *DefaultDict) set(key, value py.Object) error {
	encoded, err := py.DictKey(key)
	if err != nil {
		return err
	}
	if _, ok := d.values.Get(encoded); !ok {
		d.keys = append(d.keys, encoded)
	}
	d.values.Set(encoded, value)
	return nil
}

// missing produces the default for a key, storing it as CPython does.
func (d *DefaultDict) missing(key py.Object) (py.Object, error) {
	if d.defaultFactory == py.None || d.defaultFactory == nil {
		return nil, py.ExceptionNewf(py.KeyError, "%v", key)
	}
	value, err := py.Call(d.defaultFactory, py.Tuple{}, py.StringDict{})
	if err != nil {
		return nil, err
	}
	if err := d.set(key, value); err != nil {
		return nil, err
	}
	return value, nil
}

func init() {
	DefaultDictType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DefaultDict)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		if encoded, err := py.DictKey(key); err == nil {
			if v, ok := d.values.Get(encoded); ok {
				return v, nil
			}
		}
		return d.missing(key)
	}, 0, "Return self[key], creating a default when absent."))

	DefaultDictType.Dict.Set("__setitem__", py.MustNewMethod("__setitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key, value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__setitem__", 2, 2, &key, &value); err != nil {
			return nil, err
		}
		if err := self.(*DefaultDict).set(key, value); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Set self[key] to value."))

	DefaultDictType.Dict.Set("get", py.MustNewMethod("get", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DefaultDict)
		var key py.Object
		var def py.Object = py.None
		if err := py.UnpackTuple(args, py.StringDict{}, "get", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		if encoded, err := py.DictKey(key); err == nil {
			if v, ok := d.values.Get(encoded); ok {
				return v, nil
			}
		}
		return def, nil
	}, 0, "Return the value for key, or the default (no factory call)."))

	DefaultDictType.Dict.Set("__contains__", py.MustNewMethod("__contains__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DefaultDict)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__contains__", 1, 1, &key); err != nil {
			return nil, err
		}
		encoded, err := py.DictKey(key)
		if err != nil {
			return py.False, nil
		}
		if _, ok := d.values.Get(encoded); ok {
			return py.True, nil
		}
		return py.False, nil
	}, 0, "Implement 'in'."))

	DefaultDictType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(len(self.(*DefaultDict).keys)), nil
	}, 0, "Number of entries."))

	DefaultDictType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DefaultDict)
		items := make([]py.Object, 0, len(d.keys))
		for _, encoded := range d.keys {
			key, err := py.DictKeyDecode(encoded)
			if err != nil {
				return nil, err
			}
			items = append(items, key)
		}
		return newIteratorFromItems(items), nil
	}, 0, "Implement iter(self)."))

	DefaultDictType.Dict.Set("keys", py.MustNewMethod("keys", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DefaultDict)
		items := make([]py.Object, 0, len(d.keys))
		for _, encoded := range d.keys {
			key, err := py.DictKeyDecode(encoded)
			if err != nil {
				return nil, err
			}
			items = append(items, key)
		}
		return py.NewListFromItems(items), nil
	}, 0, "Return the key list."))

	DefaultDictType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*DefaultDict)
		parts := make([]string, 0, len(d.keys))
		for _, encoded := range d.keys {
			key, err := py.DictKeyDecode(encoded)
			if err != nil {
				return nil, err
			}
			keyStr, err := py.ReprAsString(key)
			if err != nil {
				return nil, err
			}
			valueStr, err := py.ReprAsString(d.values.GetOrNil(encoded))
			if err != nil {
				return nil, err
			}
			parts = append(parts, keyStr+": "+valueStr)
		}
		factoryStr, err := py.ReprAsString(d.defaultFactory)
		if err != nil {
			return nil, err
		}
		return py.String("defaultdict(" + factoryStr + ", {" + strings.Join(parts, ", ") + "})"), nil
	}, 0, "Return repr(self)."))
}

// ---------------------------------------------------------------------------
// ChainMap

const chainmap_doc = `A ChainMap groups multiple dicts (or other mappings) together
to create a single, updateable view.`

// ChainMap searches a list of mappings in order.
type ChainMap struct {
	maps []py.Object
}

var ChainMapType = py.NewTypeX("collections.ChainMap", chainmap_doc, chainMapNew, nil)

func (c *ChainMap) Type() *py.Type { return ChainMapType }

func chainMapNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := &ChainMap{}
	for _, arg := range args {
		c.maps = append(c.maps, arg)
	}
	if kwargs.Len() > 0 {
		d := py.NewStringDict()
		kwargs.Range(func(k string, v py.Object) bool {
			d.Set(k, v)
			return false
		})
		c.maps = append(c.maps, d)
	}
	if len(c.maps) == 0 {
		c.maps = append(c.maps, py.NewStringDict())
	}
	return c, nil
}

func (c *ChainMap) lookup(key py.Object) (py.Object, bool, error) {
	for _, m := range c.maps {
		if _, ok := m.(py.IGetDict); ok {
			res, err := py.GetAttr(m, key)
			if err == nil && res != nil {
				return res, true, nil
			}
		}
		if d, ok := m.(py.StringDict); ok {
			if encoded, err := py.DictKey(key); err == nil {
				if v, ok := d.Get(encoded); ok {
					return v, true, nil
				}
			}
		}
	}
	return nil, false, nil
}

func init() {
	ChainMapType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*ChainMap)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		v, ok, err := c.lookup(key)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, py.ExceptionNewf(py.KeyError, "%v", key)
		}
		return v, nil
	}, 0, "Return the value from the first mapping that has it."))

	ChainMapType.Dict.Set("get", py.MustNewMethod("get", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*ChainMap)
		var key py.Object
		var def py.Object = py.None
		if err := py.UnpackTuple(args, py.StringDict{}, "get", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		v, ok, err := c.lookup(key)
		if err != nil {
			return nil, err
		}
		if !ok {
			return def, nil
		}
		return v, nil
	}, 0, "Return the value for key, or the default."))

	ChainMapType.Dict.Set("__contains__", py.MustNewMethod("__contains__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*ChainMap)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__contains__", 1, 1, &key); err != nil {
			return nil, err
		}
		_, ok, err := c.lookup(key)
		if err != nil {
			return nil, err
		}
		if ok {
			return py.True, nil
		}
		return py.False, nil
	}, 0, "Implement 'in'."))

	ChainMapType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*ChainMap)
		seen := map[string]bool{}
		for _, m := range c.maps {
			d, ok := m.(py.IGetDict)
			if !ok {
				continue
			}
			for _, k := range d.GetDict().Keys() {
				seen[k] = true
			}
		}
		return py.Int(len(seen)), nil
	}, 0, "Number of distinct keys across the mappings."))

	ChainMapType.Dict.Set("maps", py.MustNewMethod("maps", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*ChainMap)
		items := make([]py.Object, len(c.maps))
		copy(items, c.maps)
		return py.NewListFromItems(items), nil
	}, 0, "Return the list of mappings."))

	ChainMapType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*ChainMap)
		parts, err := reprItems(c.maps)
		if err != nil {
			return nil, err
		}
		return py.String("ChainMap(" + strings.Join(parts, ", ") + ")"), nil
	}, 0, "Return repr(self)."))
}

// ---------------------------------------------------------------------------
// UserDict, UserList, UserString

const userdict_doc = `Dictionary wrapper for easier subclassing.`

// UserDict is a plain wrapper that Python code can subclass and override.
type UserDict struct {
	data py.Object
}

var UserDictType = py.NewTypeX("collections.UserDict", userdict_doc, func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	u := &UserDict{data: py.NewStringDict()}
	if len(args) > 1 {
		return nil, py.ExceptionNewf(py.TypeError, "expected at most 1 argument, got %d", len(args))
	}
	if len(args) == 1 {
		if err := updateFrom(u.data, args[0]); err != nil {
			return nil, err
		}
	}
	if kwargs.Len() > 0 {
		for _, __e := range kwargs.Items() {
			k := __e.Key
			v := __e.Value

			if _, err := py.SetItem(u.data, py.String(k), v); err != nil {
				return nil, err
			}
		}
	}
	return u, nil
}, nil)

func (u *UserDict) Type() *py.Type { return UserDictType }

const userlist_doc = `A more or less complete user-defined wrapper around list objects.`

// UserList wraps a list.
type UserList struct {
	data *py.List
}

var UserListType = py.NewTypeX("collections.UserList", userlist_doc, func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	u := &UserList{data: py.NewList()}
	if len(args) > 1 {
		return nil, py.ExceptionNewf(py.TypeError, "expected at most 1 argument, got %d", len(args))
	}
	if len(args) == 1 {
		items, err := py.SequenceList(args[0])
		if err != nil {
			return nil, err
		}
		u.data.Items = append(u.data.Items, items.Items...)
	}
	return u, nil
}, nil)

func (u *UserList) Type() *py.Type { return UserListType }

const userstring_doc = `A wrapper around string objects.`

// UserString wraps a string.
type UserString struct {
	data py.String
}

var UserStringType = py.NewTypeX("collections.UserString", userstring_doc, func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var seq py.Object = py.String("")
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:UserString", []string{"seq"}, &seq); err != nil {
		return nil, err
	}
	s, err := py.Str(seq)
	if err != nil {
		return nil, err
	}
	return &UserString{data: s.(py.String)}, nil
}, nil)

func (u *UserString) Type() *py.Type { return UserStringType }

func init() {
	UserDictType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Len(self.(*UserDict).data)
	}, 0, "Number of entries."))
	UserDictType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		return py.GetItem(self.(*UserDict).data, key)
	}, 0, "Return self[key]."))
	UserDictType.Dict.Set("__setitem__", py.MustNewMethod("__setitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key, value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__setitem__", 2, 2, &key, &value); err != nil {
			return nil, err
		}
		return py.SetItem(self.(*UserDict).data, key, value)
	}, 0, "Set self[key] to value."))
	UserDictType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*UserDict)
		if dd, ok := d.data.(py.IGetDict); ok {
			items := make([]py.Object, 0, dd.GetDict().Len())
			for _, k := range dd.GetDict().Keys() {
				key, err := py.DictKeyDecode(k)
				if err != nil {
					return nil, err
				}
				items = append(items, key)
			}
			return newIteratorFromItems(items), nil
		}
		return py.Iter(d.data)
	}, 0, "Implement iter(self)."))
	UserDictType.Dict.Set("keys", py.MustNewMethod("keys", func(self py.Object, args py.Tuple) (py.Object, error) {
		d := self.(*UserDict)
		if dd, ok := d.data.(py.IGetDict); ok {
			items := make([]py.Object, 0, dd.GetDict().Len())
			for _, k := range dd.GetDict().Keys() {
				key, err := py.DictKeyDecode(k)
				if err != nil {
					return nil, err
				}
				items = append(items, key)
			}
			return py.NewListFromItems(items), nil
		}
		return py.NewListFromItems(nil), nil
	}, 0, "Return the key list."))
	UserDictType.Dict.Set("get", py.MustNewMethod("get", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key py.Object
		var def py.Object = py.None
		if err := py.UnpackTuple(args, py.StringDict{}, "get", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		d := self.(*UserDict)
		if dd, ok := d.data.(py.IGetDict); ok {
			if encoded, err := py.DictKey(key); err == nil {
				if v, ok := dd.GetDict().Get(encoded); ok {
					return v, nil
				}
			}
		}
		return def, nil
	}, 0, "Return the value for key, or the default."))
	UserDictType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		s, err := py.ReprAsString(self.(*UserDict).data)
		if err != nil {
			return nil, err
		}
		return py.String("UserDict(" + s + ")"), nil
	}, 0, "Return repr(self)."))

	UserListType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(len(self.(*UserList).data.Items)), nil
	}, 0, "Number of items."))
	UserListType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		return py.GetItem(self.(*UserList).data, key)
	}, 0, "Return self[i]."))
	UserListType.Dict.Set("append", py.MustNewMethod("append", func(self py.Object, args py.Tuple) (py.Object, error) {
		var item py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "append", 1, 1, &item); err != nil {
			return nil, err
		}
		self.(*UserList).data.Append(item)
		return py.None, nil
	}, 0, "Append item to the end."))
	UserListType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return newIteratorFromItems(self.(*UserList).data.Items), nil
	}, 0, "Implement iter(self)."))
	UserListType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		s, err := py.ReprAsString(self.(*UserList).data)
		if err != nil {
			return nil, err
		}
		return py.String("UserList(" + s + ")"), nil
	}, 0, "Return repr(self)."))

	UserStringType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Len(self.(*UserString).data)
	}, 0, "Number of characters."))
	UserStringType.Dict.Set("__str__", py.MustNewMethod("__str__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*UserString).data, nil
	}, 0, "Return str(self)."))
	UserStringType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		u := self.(*UserString)
		return py.String("UserString(" + string(u.data) + ")"), nil
	}, 0, "Return repr(self)."))
}

// ---------------------------------------------------------------------------
// namedtuple

const namedtuple_doc = `namedtuple(typename, field_names, *, rename=False, defaults=None, module=None)

Returns a new tuple subclass named typename.`

// NamedTuple is a tuple with named fields.
//
// CPython builds a real tuple subclass; dict and tuple cannot be subclassed
// here, so this is its own type that carries the values and exposes both the
// field names and the integer indices.
type NamedTuple struct {
	values []py.Object
	fields []string
	name   string
}

var NamedTupleType = py.NewTypeX("collections.namedtuple", namedtuple_doc, nil, nil)

func (n *NamedTuple) Type() *py.Type { return NamedTupleType }

func init() {
	NamedTupleType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(len(self.(*NamedTuple).values)), nil
	}, 0, "Number of fields."))
	NamedTupleType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		n := self.(*NamedTuple)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		if s, ok := key.(py.String); ok {
			for i, f := range n.fields {
				if f == string(s) {
					return n.values[i], nil
				}
			}
			return nil, py.ExceptionNewf(py.IndexError, "no field named %v", key)
		}
		i, err := py.IndexInt(key)
		if err != nil {
			return nil, err
		}
		if i < 0 {
			i += len(n.values)
		}
		if i < 0 || i >= len(n.values) {
			return nil, py.ExceptionNewf(py.IndexError, "tuple index out of range")
		}
		return n.values[i], nil
	}, 0, "Return the field by name or index."))
	NamedTupleType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return newIteratorFromItems(self.(*NamedTuple).values), nil
	}, 0, "Implement iter(self)."))

	// A named tuple is a tuple, so it hashes like one and compares field by
	// field.  Without these the instance fell through to the metatype's
	// __hash__ descriptor on ObjectType, which raises "descriptor '__hash__'
	// requires a 'type' object": that is what made the namedtuple in
	// packaging._manylinux (a glibc version key) unhashable, and so
	// un-usable as a dict key or a set member.
	NamedTupleType.Dict.Set("__hash__", py.MustNewMethod("__hash__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return tupleHash(self.(*NamedTuple).values)
	}, 0, "Return hash(self)."))
	NamedTupleType.Dict.Set("__eq__", py.MustNewMethod("__eq__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__eq__", 1, 1, &other); err != nil {
			return nil, err
		}
		n := self.(*NamedTuple)
		switch t := other.(type) {
		case *NamedTuple:
			return py.NewBool(sameValues(n.values, t.values)), nil
		case py.Tuple:
			return py.NewBool(sameValues(n.values, []py.Object(t))), nil
		}
		return py.NotImplemented, nil
	}, 0, "Return self==value."))
	NamedTupleType.Dict.Set("__ne__", py.MustNewMethod("__ne__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__ne__", 1, 1, &other); err != nil {
			return nil, err
		}
		n := self.(*NamedTuple)
		switch t := other.(type) {
		case *NamedTuple:
			return py.NewBool(!sameValues(n.values, t.values)), nil
		case py.Tuple:
			return py.NewBool(!sameValues(n.values, []py.Object(t))), nil
		}
		return py.NotImplemented, nil
	}, 0, "Return self!=value."))
	NamedTupleType.Dict.Set("count", py.MustNewMethod("count", func(self py.Object, args py.Tuple) (py.Object, error) {
		var value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "count", 1, 1, &value); err != nil {
			return nil, err
		}
		n := 0
		for _, v := range self.(*NamedTuple).values {
			eq, err := py.Eq(v, value)
			if err != nil {
				return nil, err
			}
			if ok, err := py.ObjectIsTrue(eq); err == nil && ok {
				n++
			}
		}
		return py.Int(n), nil
	}, 0, "Return number of occurrences of value."))
	NamedTupleType.Dict.Set("index", py.MustNewMethod("index", func(self py.Object, args py.Tuple) (py.Object, error) {
		var value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "index", 1, 1, &value); err != nil {
			return nil, err
		}
		for i, v := range self.(*NamedTuple).values {
			eq, err := py.Eq(v, value)
			if err != nil {
				return nil, err
			}
			if ok, err := py.ObjectIsTrue(eq); err == nil && ok {
				return py.Int(i), nil
			}
		}
		return nil, py.ExceptionNewf(py.ValueError, "tuple.index(x): x not in tuple")
	}, 0, "Return first index of value."))
	NamedTupleType.Dict.Set("_fields", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			n := self.(*NamedTuple)
			items := make([]py.Object, len(n.fields))
			for i, f := range n.fields {
				items[i] = py.String(f)
			}
			return py.NewListFromItems(items), nil
		},
	})

	NamedTupleType.Dict.Set("_asdict", py.MustNewMethod("_asdict", func(self py.Object, args py.Tuple) (py.Object, error) {
		n := self.(*NamedTuple)
		out := py.NewStringDict()
		for i, f := range n.fields {
			out.Set(f, n.values[i])
		}
		return out, nil
	}, 0, "Return a dict of the fields."))
	NamedTupleType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		n := self.(*NamedTuple)
		parts := make([]string, len(n.fields))
		for i, f := range n.fields {
			s, err := py.ReprAsString(n.values[i])
			if err != nil {
				return nil, err
			}
			parts[i] = f + "=" + s
		}
		return py.String(n.name + "(" + strings.Join(parts, ", ") + ")"), nil
	}, 0, "Return repr(self)."))
}

// namedtupleFactory is what namedtuple() returns: a class whose constructor
// builds an instance of the named tuple it describes.
//
// CPython returns a real tuple subclass.  This interpreter cannot subclass
// tuple, so the class is a distinct type carrying the field list on the class
// (in _fields) and the values on the instance; it is a genuine *py.Type, which
// is what lets "class X(namedtuple(...))" derive from it.
type namedtupleFactory struct {
	name   string
	fields []string
	cls    *py.Type
}

var namedtupleFactoryType = py.NewType("collections.namedtuple", namedtuple_doc)

func (f *namedtupleFactory) Type() *py.Type { return namedtupleFactoryType }

func (f *namedtupleFactory) M__repr__() (py.Object, error) {
	return py.String("<namedtuple " + f.name + ">"), nil
}

func (f *namedtupleFactory) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if kwargs.Len() > 0 {
		return nil, py.ExceptionNewf(py.TypeError, "%s() takes no keyword arguments", f.name)
	}
	if len(args) != len(f.fields) {
		return nil, py.ExceptionNewf(py.TypeError, "%s() takes %d arguments (%d given)", f.name, len(f.fields), len(args))
	}
	values := make([]py.Object, len(args))
	copy(values, args)
	return &NamedTuple{values: values, fields: f.fields, name: f.name}, nil
}

// namedtupleMethod is the module-level function form of namedtupleNew.
func namedtupleMethod(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return namedtupleNew(nil, args, kwargs)
}

// namedtuple(typename, field_names, *, rename=False, defaults=None, module=None)
func namedtupleNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		typename py.Object
		names    py.Object
		rename   py.Object = py.False
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "OO|O:namedtuple", []string{"typename", "field_names", "rename"}, &typename, &names, &rename)
	if err != nil {
		return nil, err
	}
	name, err := py.StrAsString(typename)
	if err != nil {
		return nil, err
	}

	// field_names is a sequence of names, or one string with names separated
	// by commas and/or spaces.
	var fields []string
	if s, ok := names.(py.String); ok {
		for _, f := range strings.FieldsFunc(string(s), func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n'
		}) {
			fields = append(fields, f)
		}
	} else {
		items, err := py.SequenceList(names)
		if err != nil {
			return nil, err
		}
		for _, item := range items.Items {
			f, err := py.StrAsString(item)
			if err != nil {
				return nil, err
			}
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 {
		return nil, py.ExceptionNewf(py.ValueError, "TypeError: namedtuple() requires at least one field name")
	}

	// A field may not be a keyword or start with an underscore, as in CPython.
	for _, f := range fields {
		if strings.HasPrefix(f, "_") {
			if rename == py.True {
				continue
			}
			return nil, py.ExceptionNewf(py.ValueError, "Field names cannot start with an underscore: %q", f)
		}
	}

	// Return a real class, not just a callable: CPython's namedtuple is a
	// class and code derives from it ("class Url(namedtuple('Url', [...]))").
	// The field list lives on the class as _fields, and the constructor builds
	// the instance carrying the values.
	factory := &namedtupleFactory{name: name, fields: fields}
	cls := py.NewTypeX(name, "", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return factory.M__call__(args, kwargs)
	}, nil)
	cls.Flags |= py.TPFLAGS_BASETYPE
	factory.cls = cls
	fieldItems := make([]py.Object, len(fields))
	for i, f := range fields {
		fieldItems[i] = py.String(f)
	}
	cls.Dict.Set("_fields", py.NewListFromItems(fieldItems))
	cls.Dict.Set("__new__", py.MustNewMethod("__new__", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return factory.M__call__(args, kwargs)
	}, 0, "Create a new named tuple instance."))
	// The named tuple behaviours are inherited from the shared NamedTupleType
	// so every generated class has the field accessors, repr and so on.
	cls.Base = NamedTupleType
	cls.Bases = py.Tuple{NamedTupleType}
	return cls, nil
}

func init() {
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "deque", Value: DequeType},
		py.DictEntry{Key: "Counter", Value: CounterType},
		py.DictEntry{Key: "OrderedDict", Value: OrderedDictType},
		py.DictEntry{Key: "defaultdict", Value: DefaultDictType},
		py.DictEntry{Key: "ChainMap", Value: ChainMapType},
		py.DictEntry{Key: "UserDict", Value: UserDictType},
		py.DictEntry{Key: "UserList", Value: UserListType},
		py.DictEntry{Key: "UserString", Value: UserStringType},
		py.DictEntry{Key: "namedtuple", Value: py.MustNewMethod("namedtuple", namedtupleMethod, 0, namedtuple_doc)},
	)
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "collections",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// ---------------------------------------------------------------------------
// Go interface bridges
//
// The rest of the interpreter reaches methods registered in a type's Dict
// inconsistently, and for these types it binds `self` incorrectly (or not at
// all), so every method that is called implicitly by the VM - subscripting,
// len, iter, repr, 'in' - is also implemented as a Go interface, which is how
// the native types in the standard library do it.

func (c *Counter) M__len__() (py.Object, error) { return py.Int(len(c.keys)), nil }

func (c *Counter) M__iter__() (py.Object, error) { return newIteratorFromItems(c.order()), nil }

func (c *Counter) M__contains__(item py.Object) (py.Object, error) {
	_, ok, err := c.get(item)
	if err != nil {
		return nil, err
	}
	if ok {
		return py.True, nil
	}
	return py.False, nil
}

func (c *Counter) M__getitem__(key py.Object) (py.Object, error) {
	return counterGetItem(c, key)
}

func (c *Counter) M__setitem__(key, value py.Object) (py.Object, error) {
	if err := c.set(key, value); err != nil {
		return nil, err
	}
	return py.None, nil
}

func (c *Counter) M__repr__() (py.Object, error) {
	parts := make([]string, 0, len(c.keys))
	for i, encoded := range c.keys {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			return nil, err
		}
		keyStr, err := py.ReprAsString(key)
		if err != nil {
			return nil, err
		}
		valueStr, err := py.ReprAsString(c.values[i])
		if err != nil {
			return nil, err
		}
		parts = append(parts, keyStr+": "+valueStr)
	}
	return py.String("Counter({" + strings.Join(parts, ", ") + "})"), nil
}

func (d *Deque) M__iter__() (py.Object, error) { return newIteratorFromItems(d.items), nil }

func (d *Deque) M__contains__(item py.Object) (py.Object, error) {
	for _, have := range d.items {
		eq, err := py.Eq(have, item)
		if err != nil {
			return nil, err
		}
		if eq == py.True {
			return py.True, nil
		}
	}
	return py.False, nil
}

func (d *Deque) M__getitem__(key py.Object) (py.Object, error) {
	i, err := py.IndexInt(key)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "sequence index must be integer, not '%s'", key.Type().Name)
	}
	if i < 0 {
		i += len(d.items)
	}
	if i < 0 || i >= len(d.items) {
		return nil, py.ExceptionNewf(py.IndexError, "deque index out of range")
	}
	return d.items[i], nil
}

func (d *Deque) M__setitem__(key, value py.Object) (py.Object, error) {
	i, err := py.IndexInt(key)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "sequence index must be integer, not '%s'", key.Type().Name)
	}
	if i < 0 {
		i += len(d.items)
	}
	if i < 0 || i >= len(d.items) {
		return nil, py.ExceptionNewf(py.IndexError, "deque index out of range")
	}
	d.items[i] = value
	return py.None, nil
}

func (d *Deque) M__repr__() (py.Object, error) {
	parts, err := reprItems(d.items)
	if err != nil {
		return nil, err
	}
	if d.maxlen >= 0 {
		return py.String("deque([" + strings.Join(parts, ", ") + "], maxlen=" + itoa(d.maxlen) + ")"), nil
	}
	return py.String("deque([" + strings.Join(parts, ", ") + "])"), nil
}

func (o *OrderedDict) M__len__() (py.Object, error) { return py.Int(len(o.keys)), nil }

func (o *OrderedDict) M__iter__() (py.Object, error) { return newIteratorFromItems(o.order()), nil }

func (o *OrderedDict) M__getitem__(key py.Object) (py.Object, error) {
	if encoded, err := py.DictKey(key); err == nil {
		if v, ok := o.values.Get(encoded); ok {
			return v, nil
		}
	}
	return nil, py.ExceptionNewf(py.KeyError, "%v", key)
}

func (o *OrderedDict) M__setitem__(key, value py.Object) (py.Object, error) {
	if err := o.set(key, value); err != nil {
		return nil, err
	}
	return py.None, nil
}

func (o *OrderedDict) M__delitem__(key py.Object) (py.Object, error) {
	encoded, err := py.DictKey(key)
	if err != nil {
		return nil, py.ExceptionNewf(py.KeyError, "%v", key)
	}
	if _, ok := o.values.Get(encoded); !ok {
		return nil, py.ExceptionNewf(py.KeyError, "%v", key)
	}
	o.values.Del(encoded)
	for i, k := range o.keys {
		if k == encoded {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
	return py.None, nil
}

func (o *OrderedDict) M__contains__(item py.Object) (py.Object, error) {
	encoded, err := py.DictKey(item)
	if err != nil {
		return py.False, nil
	}
	if _, ok := o.values.Get(encoded); ok {
		return py.True, nil
	}
	return py.False, nil
}

func (o *OrderedDict) M__repr__() (py.Object, error) {
	parts := make([]string, 0, len(o.keys))
	for _, encoded := range o.keys {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			return nil, err
		}
		keyStr, err := py.ReprAsString(key)
		if err != nil {
			return nil, err
		}
		valueStr, err := py.ReprAsString(o.values.GetOrNil(encoded))
		if err != nil {
			return nil, err
		}
		parts = append(parts, keyStr+": "+valueStr)
	}
	return py.String("OrderedDict({" + strings.Join(parts, ", ") + "})"), nil
}

func (d *DefaultDict) M__len__() (py.Object, error) { return py.Int(len(d.keys)), nil }

func (d *DefaultDict) M__getitem__(key py.Object) (py.Object, error) {
	if encoded, err := py.DictKey(key); err == nil {
		if v, ok := d.values.Get(encoded); ok {
			return v, nil
		}
	}
	return d.missing(key)
}

func (d *DefaultDict) M__setitem__(key, value py.Object) (py.Object, error) {
	if err := d.set(key, value); err != nil {
		return nil, err
	}
	return py.None, nil
}

func (d *DefaultDict) M__contains__(item py.Object) (py.Object, error) {
	encoded, err := py.DictKey(item)
	if err != nil {
		return py.False, nil
	}
	if _, ok := d.values.Get(encoded); ok {
		return py.True, nil
	}
	return py.False, nil
}

func (d *DefaultDict) M__iter__() (py.Object, error) {
	items := make([]py.Object, 0, len(d.keys))
	for _, encoded := range d.keys {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			return nil, err
		}
		items = append(items, key)
	}
	return newIteratorFromItems(items), nil
}

func (c *ChainMap) M__getitem__(key py.Object) (py.Object, error) {
	v, ok, err := c.lookup(key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, py.ExceptionNewf(py.KeyError, "%v", key)
	}
	return v, nil
}

func (c *ChainMap) M__contains__(item py.Object) (py.Object, error) {
	_, ok, err := c.lookup(item)
	if err != nil {
		return nil, err
	}
	if ok {
		return py.True, nil
	}
	return py.False, nil
}

func (n *NamedTuple) M__len__() (py.Object, error) { return py.Int(len(n.values)), nil }

func (n *NamedTuple) M__iter__() (py.Object, error) { return newIteratorFromItems(n.values), nil }

func (n *NamedTuple) M__getitem__(key py.Object) (py.Object, error) {
	if s, ok := key.(py.String); ok {
		for i, f := range n.fields {
			if f == string(s) {
				return n.values[i], nil
			}
		}
		return nil, py.ExceptionNewf(py.IndexError, "no field named %v", key)
	}
	i, err := py.IndexInt(key)
	if err != nil {
		return nil, err
	}
	if i < 0 {
		i += len(n.values)
	}
	if i < 0 || i >= len(n.values) {
		return nil, py.ExceptionNewf(py.IndexError, "tuple index out of range")
	}
	return n.values[i], nil
}

func (n *NamedTuple) M__repr__() (py.Object, error) {
	parts := make([]string, len(n.fields))
	for i, f := range n.fields {
		s, err := py.ReprAsString(n.values[i])
		if err != nil {
			return nil, err
		}
		parts[i] = f + "=" + s
	}
	return py.String(n.name + "(" + strings.Join(parts, ", ") + ")"), nil
}

// Interface assertions, so a missing bridge is a compile error rather than a
// runtime surprise.
var (
	_ py.I__len__      = (*Counter)(nil)
	_ py.I__iter__     = (*Counter)(nil)
	_ py.I__contains__ = (*Counter)(nil)
	_ py.I__getitem__  = (*Counter)(nil)
	_ py.I__setitem__  = (*Counter)(nil)
	_ py.I__repr__     = (*Counter)(nil)
	_ py.I__len__      = (*Deque)(nil)
	_ py.I__iter__     = (*Deque)(nil)
	_ py.I__contains__ = (*Deque)(nil)
	_ py.I__getitem__  = (*Deque)(nil)
	_ py.I__setitem__  = (*Deque)(nil)
	_ py.I__repr__     = (*Deque)(nil)
	_ py.I__len__      = (*OrderedDict)(nil)
	_ py.I__iter__     = (*OrderedDict)(nil)
	_ py.I__getitem__  = (*OrderedDict)(nil)
	_ py.I__setitem__  = (*OrderedDict)(nil)
	_ py.I__delitem__  = (*OrderedDict)(nil)
	_ py.I__contains__ = (*OrderedDict)(nil)
	_ py.I__repr__     = (*OrderedDict)(nil)
	_ py.I__len__      = (*DefaultDict)(nil)
	_ py.I__getitem__  = (*DefaultDict)(nil)
	_ py.I__setitem__  = (*DefaultDict)(nil)
	_ py.I__contains__ = (*DefaultDict)(nil)
	_ py.I__iter__     = (*DefaultDict)(nil)
	_ py.I__getitem__  = (*ChainMap)(nil)
	_ py.I__contains__ = (*ChainMap)(nil)
	_ py.I__len__      = (*NamedTuple)(nil)
	_ py.I__iter__     = (*NamedTuple)(nil)
	_ py.I__getitem__  = (*NamedTuple)(nil)
	_ py.I__repr__     = (*NamedTuple)(nil)
)

func (d *DefaultDict) M__repr__() (py.Object, error) {
	parts := make([]string, 0, len(d.keys))
	for _, encoded := range d.keys {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			return nil, err
		}
		keyStr, err := py.ReprAsString(key)
		if err != nil {
			return nil, err
		}
		valueStr, err := py.ReprAsString(d.values.GetOrNil(encoded))
		if err != nil {
			return nil, err
		}
		parts = append(parts, keyStr+": "+valueStr)
	}
	factoryStr, err := py.ReprAsString(d.defaultFactory)
	if err != nil {
		return nil, err
	}
	return py.String("defaultdict(" + factoryStr + ", {" + strings.Join(parts, ", ") + "})"), nil
}

func (u *UserDict) M__len__() (py.Object, error) { return py.Len(u.data) }

func (u *UserDict) M__contains__(item py.Object) (py.Object, error) {
	d, ok := u.data.(py.IGetDict)
	if !ok {
		return py.False, nil
	}
	encoded, err := py.DictKey(item)
	if err != nil {
		return py.False, nil
	}
	if _, ok := d.GetDict().Get(encoded); ok {
		return py.True, nil
	}
	return py.False, nil
}

func (u *UserList) M__len__() (py.Object, error) { return py.Int(len(u.data.Items)), nil }

func (u *UserList) M__iter__() (py.Object, error) { return newIteratorFromItems(u.data.Items), nil }

func (u *UserList) M__getitem__(key py.Object) (py.Object, error) {
	return py.GetItem(u.data, key)
}

func (u *UserList) M__repr__() (py.Object, error) {
	s, err := py.ReprAsString(u.data)
	if err != nil {
		return nil, err
	}
	return py.String("UserList(" + s + ")"), nil
}

func (u *UserString) M__len__() (py.Object, error) { return py.Len(u.data) }

func (u *UserDict) M__repr__() (py.Object, error) {
	s, err := py.ReprAsString(u.data)
	if err != nil {
		return nil, err
	}
	return py.String("UserDict(" + s + ")"), nil
}

func (u *UserString) M__str__() (py.Object, error) { return u.data, nil }

func (u *UserString) M__repr__() (py.Object, error) {
	return py.String("UserString(" + string(u.data) + ")"), nil
}

var (
	_ py.I__repr__     = (*DefaultDict)(nil)
	_ py.I__len__      = (*UserDict)(nil)
	_ py.I__contains__ = (*UserDict)(nil)
	_ py.I__len__      = (*UserList)(nil)
	_ py.I__iter__     = (*UserList)(nil)
	_ py.I__getitem__  = (*UserList)(nil)
	_ py.I__repr__     = (*UserList)(nil)
	_ py.I__len__      = (*UserString)(nil)
	_ py.I__str__      = (*UserString)(nil)
	_ py.I__repr__     = (*UserString)(nil)
)

// M__getattribute__ resolves a named tuple's field names as attributes.
// Fields come first, as in CPython, with the type's own methods after them;
// anything unknown falls through to the normal lookup so that methods and
// errors behave as usual.
func (n *NamedTuple) M__getattribute__(name string) (py.Object, error) {
	for i, f := range n.fields {
		if f == name {
			return n.values[i], nil
		}
	}
	res := NamedTupleType.NativeGetAttrOrNil(name)
	if res == nil {
		return nil, py.ExceptionNewf(py.AttributeError, "'%s' object has no attribute '%s'", n.name, name)
	}
	if I, ok := res.(py.I__get__); ok {
		return I.M__get__(n, NamedTupleType)
	}
	return res, nil
}

var _ py.I__getattribute__ = (*NamedTuple)(nil)

// updateFrom fills a mapping from either another mapping or a sequence of
// (key, value) pairs, which is what "dict(mapping)" and "dict(iterable)" both
// mean.  SequenceList alone cannot do this: a mapping is not a sequence.
func updateFrom(target py.Object, source py.Object) error {
	if d, ok := source.(py.IGetDict); ok {
		for _, __e := range d.GetDict().Items() {
			encoded := __e.Key
			value := __e.Value
			key, err := py.DictKeyDecode(encoded)
			if err != nil {
				return err
			}
			if _, err := py.SetItem(target, key, value); err != nil {
				return err
			}
		}
		return nil
	}
	pairs, err := py.SequenceList(source)
	if err != nil {
		return err
	}
	for _, item := range pairs.Items {
		pair, ok := item.(py.Tuple)
		if !ok || len(pair) != 2 {
			return py.ExceptionNewf(py.ValueError, "dictionary update sequence element is not a pair")
		}
		if _, err := py.SetItem(target, pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func (u *UserDict) M__getitem__(key py.Object) (py.Object, error) {
	return py.GetItem(u.data, key)
}

func (u *UserDict) M__setitem__(key, value py.Object) (py.Object, error) {
	return py.SetItem(u.data, key, value)
}

func (u *UserDict) M__iter__() (py.Object, error) {
	if d, ok := u.data.(py.IGetDict); ok {
		items := make([]py.Object, 0, d.GetDict().Len())
		for _, k := range d.GetDict().Keys() {
			key, err := py.DictKeyDecode(k)
			if err != nil {
				return nil, err
			}
			items = append(items, key)
		}
		return newIteratorFromItems(items), nil
	}
	return py.Iter(u.data)
}

func (u *UserString) M__getitem__(key py.Object) (py.Object, error) { return py.GetItem(u.data, key) }

func (u *UserString) M__contains__(item py.Object) (py.Object, error) {
	found, err := py.SequenceContains(u.data, item)
	if err != nil {
		return nil, err
	}
	if found {
		return py.True, nil
	}
	return py.False, nil
}

func (u *UserString) M__add__(other py.Object) (py.Object, error) {
	return py.Add(u.data, other)
}

var (
	_ py.I__getitem__  = (*UserDict)(nil)
	_ py.I__setitem__  = (*UserDict)(nil)
	_ py.I__iter__     = (*UserDict)(nil)
	_ py.I__getitem__  = (*UserString)(nil)
	_ py.I__contains__ = (*UserString)(nil)
	_ py.I__add__      = (*UserString)(nil)
)

// tupleHash hashes a sequence the way Python's tuple hash does: an order
// sensitive combination of the element hashes, so that the hash of a tuple
// equals the hash of an equal tuple and two different orders differ.
func tupleHash(values []py.Object) (py.Object, error) {
	const (
		mult = 1000003
		mod  = uint64(1) << 61
	)
	var acc uint64 = 0x345678
	length := uint64(len(values))
	for _, v := range values {
		h, err := objectHash(v)
		if err != nil {
			return nil, err
		}
		acc = (acc ^ uint64(h)) * mult % mod
		length--
	}
	acc = (acc ^ length) % mod
	return py.Int(int64(acc)), nil
}

// objectHash is the hash of one element, through its own __hash__ or through
// the value types that carry none.
func objectHash(o py.Object) (int64, error) {
	if h, ok := o.(py.I__hash__); ok {
		res, err := h.M__hash__()
		if err != nil {
			return 0, err
		}
		if n, err := py.MakeGoInt64(res); err == nil {
			return n, nil
		}
	}
	switch v := o.(type) {
	case py.NoneType:
		return 0, nil
	case py.Bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case py.Int:
		return int64(v), nil
	case py.Float:
		return int64(float64(v)), nil
	case py.String:
		var h uint64 = 14695981039346656037
		for i := 0; i < len(v); i++ {
			h ^= uint64(v[i])
			h *= 1099511628211
		}
		return int64(h & (1<<63 - 1)), nil
	}
	// A type is hashable by identity, as in CPython.
	if t, ok := o.(*py.Type); ok {
		return int64(len(t.Name)), nil
	}
	return 0, py.ExceptionNewf(py.TypeError, "unhashable type: '%s'", o.Type().Name)
}

// sameValues compares two value sequences element by element, which is the
// tuple comparison a named tuple inherits.
func sameValues(a, b []py.Object) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		eq, err := py.Eq(a[i], b[i])
		if err != nil {
			return false
		}
		if ok, _ := py.ObjectIsTrue(eq); !ok {
			return false
		}
	}
	return true
}
