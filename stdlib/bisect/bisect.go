// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bisect provides the implementation of python's 'bisect' module.
//
// The functions are the binary searches over a sorted sequence: the search
// half of the module (bisect_left, bisect_right and their aliases) returns
// the insertion point, and the "insort" half inserts there.  Both honour the
// lo and hi arguments.
//
// The comparison is the interpreter's own "<" and the insertion uses the
// sequence's own __setitem__/insert, so the module works on any object that
// supports them rather than only on a list.
package bisect

import (
	"github.com/vishnukv64/gpython/py"
)

const bisect_doc = `Bisection algorithms.

This module provides support for maintaining a list in sorted order without
having to sort the list after each insertion.  For long lists of items with
expensive comparison operations, this can be an improvement over the more
common approach.  The module is called bisect because it uses a basic
bisection algorithm to do its work.`

var (
	bisectRight_doc = `bisect_right(a, x, lo=0, hi=None, *, key=None)

Return the index where to insert item x in list a, assuming a is sorted.

The return value i is such that all e in a[:i] have e <= x, and all e in
a[i:] have e > x.  So if x already appears in the list, a.insert(i, x) will
insert just after the rightmost x already there.

Optional args lo (default 0) and hi (default len(a)) bound the slice of a to
be searched.`

	bisectLeft_doc = `bisect_left(a, x, lo=0, hi=None, *, key=None)

Return the index where to insert item x in list a, assuming a is sorted.

The return value i is such that all e in a[:i] have e < x, and all e in
a[i:] have e >= x.  So if x already appears in the list, a.insert(i, x) will
insert just before the leftmost x already there.

Optional args lo (default 0) and hi (default len(a)) bound the slice of a to
be searched.`

	insortRight_doc = `insort_right(a, x, lo=0, hi=None, *, key=None)

Insert item x in list a, and keep it sorted assuming a is sorted.

If x is already in a, insert it to the right of the rightmost x.

Optional args lo (default 0) and hi (default len(a)) bound the slice of a to
be searched.`

	insortLeft_doc = `insort_left(a, x, lo=0, hi=None, *, key=None)

Insert item x in list a, and keep it sorted assuming a is sorted.

If x is already in a, insert it to the left of the leftmost x.

Optional args lo (default 0) and hi (default len(a)) bound the slice of a to
be searched.`
)

// bisectRight is the C-level bisect_right; bisect is an alias of it.
func bisectRight(a py.Object, x py.Object, lo, hi int64, key py.Object) (int64, error) {
	for lo < hi {
		mid := (lo + hi) / 2
		item, err := py.GetItem(a, py.Int(mid))
		if err != nil {
			return 0, err
		}
		item, err = applyKey(key, item)
		if err != nil {
			return 0, err
		}
		cmp, err := py.Lt(x, item)
		if err != nil {
			return 0, err
		}
		less, err := py.ObjectIsTrue(cmp)
		if err != nil {
			return 0, err
		}
		if less {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo, nil
}

func bisectLeft(a py.Object, x py.Object, lo, hi int64, key py.Object) (int64, error) {
	for lo < hi {
		mid := (lo + hi) / 2
		item, err := py.GetItem(a, py.Int(mid))
		if err != nil {
			return 0, err
		}
		item, err = applyKey(key, item)
		if err != nil {
			return 0, err
		}
		cmp, err := py.Lt(item, x)
		if err != nil {
			return 0, err
		}
		less, err := py.ObjectIsTrue(cmp)
		if err != nil {
			return 0, err
		}
		if less {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, nil
}

// applyKey runs the optional key function, which the keyword-only "key"
// argument adds (Python 3.10).
func applyKey(key, item py.Object) (py.Object, error) {
	if key == nil || key == py.None {
		return item, nil
	}
	return py.Call(key, py.Tuple{item}, nil)
}

func bisect_right(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return bisectImpl("bisect_right", args, kwargs, bisectRight)
}

func bisect(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return bisectImpl("bisect", args, kwargs, bisectRight)
}

func bisect_left(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return bisectImpl("bisect_left", args, kwargs, bisectLeft)
}

func bisectImpl(name string, args py.Tuple, kwargs py.StringDict, fn func(py.Object, py.Object, int64, int64, py.Object) (int64, error)) (py.Object, error) {
	seq, x, lo, hi, key, err := parseArgs(name, args, kwargs)
	if err != nil {
		return nil, err
	}
	i, err := fn(seq, x, lo, hi, key)
	if err != nil {
		return nil, err
	}
	return py.Int(i), nil
}

// parseArgs unpacked the positional and keyword parameters shared by all
// six entry points: a, x, lo=0, hi=None and the keyword-only key=None.
func parseArgs(name string, args py.Tuple, kwargs py.StringDict) (py.Object, py.Object, int64, int64, py.Object, error) {
	var (
		a     py.Object
		x     py.Object
		loObj py.Object
		hiObj py.Object
		key   py.Object = py.None
	)
	kwlist := []string{"a", "x", "lo", "hi", "key"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|OO$O:"+name, kwlist,
		&a, &x, &loObj, &hiObj, &key); err != nil {
		return nil, nil, 0, 0, nil, err
	}

	length, err := py.GetLen(a)
	if err != nil {
		return nil, nil, 0, 0, nil, err
	}
	lo := int64(0)
	hi := int64(length)
	if loObj != nil && loObj != py.None {
		n, err := py.IndexInt(loObj)
		if err != nil {
			return nil, nil, 0, 0, nil, err
		}
		lo = int64(n)
		if lo < 0 {
			return nil, nil, 0, 0, nil, py.ExceptionNewf(py.ValueError, "lo must be non-negative")
		}
	}
	if hiObj != nil && hiObj != py.None {
		n, err := py.IndexInt(hiObj)
		if err != nil {
			return nil, nil, 0, 0, nil, err
		}
		hi = int64(n)
	}
	// CPython only clamps hi down to lo when hi is negative; a non-negative
	// hi below lo leaves lo as it is and the search finds nothing, giving
	// bisect_left's 1 for ([1,2,3], 2, 2, 1).
	if hi < 0 {
		hi += int64(length)
	}
	return a, x, lo, hi, key, nil
}

func insort_right(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return insortImpl("insort_right", args, kwargs, bisectRight)
}

func insort(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return insortImpl("insort", args, kwargs, bisectRight)
}

func insort_left(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return insortImpl("insort_left", args, kwargs, bisectLeft)
}

func insortImpl(name string, args py.Tuple, kwargs py.StringDict, fn func(py.Object, py.Object, int64, int64, py.Object) (int64, error)) (py.Object, error) {
	seq, x, lo, hi, key, err := parseArgs(name, args, kwargs)
	if err != nil {
		return nil, err
	}
	// The index is found against the key, but the item stored is x itself,
	// exactly as CPython does it.
	probe := x
	if key != nil && key != py.None {
		probe, err = applyKey(key, x)
		if err != nil {
			return nil, err
		}
	}
	i, err := fn(seq, probe, lo, hi, key)
	if err != nil {
		return nil, err
	}
	// The sequence's own insert is used when it has one.  This interpreter's
	// list has no insert method, so the equivalent slice assignment
	// "a[i:i] = [x]" is used instead; both are the documented behaviour.
	insert, err := py.GetAttrString(seq, "insert")
	if err == nil {
		if _, err := py.Call(insert, py.Tuple{py.Int(i), x}, nil); err != nil {
			return nil, err
		}
		return py.None, nil
	}
	if _, err := py.SetItem(seq, &py.Slice{Start: py.Int(i), Stop: py.Int(i), Step: py.None}, py.Tuple{x}); err != nil {
		return nil, err
	}
	return py.None, nil
}

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "bisect",
			Doc:  bisect_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("bisect_right", bisect_right, 0, bisectRight_doc),
			py.MustNewMethod("bisect", bisect, 0, bisectRight_doc),
			py.MustNewMethod("bisect_left", bisect_left, 0, bisectLeft_doc),
			py.MustNewMethod("insort_right", insort_right, 0, insortRight_doc),
			py.MustNewMethod("insort", insort, 0, insortRight_doc),
			py.MustNewMethod("insort_left", insort_left, 0, insortLeft_doc),
		},
	})
}
