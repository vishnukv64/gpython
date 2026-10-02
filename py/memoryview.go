// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"encoding/hex"
	"fmt"
)

// MemoryView is a view over the bytes of ANOTHER object, without copying them.
//
// The point of a memoryview is that it is a VIEW: writing through it changes
// the buffer it was made from, and a slice of it is another view over the same
// storage rather than a copy.  So it holds the backing slice itself, not a
// copy of its contents.
//
// What is modelled: a one-dimensional view of bytes (format "B", itemsize 1),
// which is what the overwhelming majority of code uses - reading and writing
// bytes, slicing, and converting with bytes()/tobytes()/tolist().  A
// multi-dimensional view, a non-byte format string ("i", "d") and the
// cast()/release() protocol are NOT implemented; the first two raise rather
// than returning a plausible wrong answer, and release() is a no-op because
// there is no lifetime to manage here.
type MemoryView struct {
	// b is the storage this view covers.  It is a sub-slice of the source
	// object's bytes, so a view of a view, or a slice of a view, all share one
	// array and agree about every change.
	b []byte
	// source is the object the view was made from, kept so that the view does
	// not outlive what it points at and so that __repr__ and .obj can name it.
	source Object
	// readonly is true for a view over immutable bytes.
	readonly bool
}

// MemoryViewType is the type of a memoryview.
var MemoryViewType = NewTypeX("memoryview", "memoryview(object)\n\nCreate a new memoryview object which references the given object.", MemoryViewNew, nil)

func (m *MemoryView) Type() *Type { return MemoryViewType }

// MemoryViewNew implements memoryview(object).
func MemoryViewNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	var obj Object
	if err := UnpackTuple(args, kwargs, "memoryview", 1, 1, &obj); err != nil {
		return nil, err
	}
	return NewMemoryView(obj)
}

// NewMemoryView returns a view over obj's bytes, sharing its storage.
//
// bytes and bytearray are the two sources that can be viewed: the first gives
// a readonly view, the second a writable one that aliases the bytearray, which
// is the behaviour the whole object exists for.
func NewMemoryView(obj Object) (Object, error) {
	switch o := obj.(type) {
	case *MemoryView:
		// A view of a view is another view of the same storage, carrying the
		// same readonly-ness.
		return &MemoryView{b: o.b, source: o.source, readonly: o.readonly}, nil
	case Bytes:
		return &MemoryView{b: []byte(o), source: obj, readonly: true}, nil
	case *ByteArray:
		return &MemoryView{b: o.b, source: obj, readonly: false}, nil
	}
	return nil, ExceptionNewf(TypeError, "memoryview: a bytes-like object is required, not '%s'", obj.Type().Name)
}

// M__len__ is the number of ITEMS, which for a byte view is the byte count.
func (m *MemoryView) M__len__() (Object, error) { return Int(len(m.b)), nil }

// M__getitem__ returns the byte at an index, or a new view over a slice.
func (m *MemoryView) M__getitem__(key Object) (Object, error) {
	if k, ok := key.(*Slice); ok {
		// A slice is another VIEW over the same storage, so writing through it
		// is visible in the original.
		lo, hi, _, _, err := k.GetIndices(len(m.b))
		if err != nil {
			return nil, err
		}
		return &MemoryView{b: m.b[lo:hi], source: m.source, readonly: m.readonly}, nil
	}
	i, err := Index(key)
	if err != nil {
		return nil, err
	}
	n := int(i)
	if n < 0 {
		n += len(m.b)
	}
	if n < 0 || n >= len(m.b) {
		return nil, ExceptionNewf(IndexError, "index out of bounds on dimension 1")
	}
	return Int(m.b[n]), nil
}

// M__setitem__ writes through the view, which is what changes the source.
func (m *MemoryView) M__setitem__(key, value Object) (Object, error) {
	if m.readonly {
		return nil, ExceptionNewf(TypeError, "cannot modify read-only memory")
	}
	i, err := Index(key)
	if err != nil {
		return nil, err
	}
	n := int(i)
	if n < 0 {
		n += len(m.b)
	}
	if n < 0 || n >= len(m.b) {
		return nil, ExceptionNewf(IndexError, "index out of bounds on dimension 1")
	}
	v, err := MakeGoInt(value)
	if err != nil {
		return nil, err
	}
	if v < 0 || v > 255 {
		return nil, ExceptionNewf(ValueError, "memoryview: cannot cast to unsigned byte")
	}
	m.b[n] = byte(v)
	return None, nil
}

// M__iter__ walks the view's bytes.
func (m *MemoryView) M__iter__() (Object, error) {
	objs := make(Tuple, 0, len(m.b))
	for _, c := range m.b {
		objs = append(objs, Int(c))
	}
	return NewIterator(objs), nil
}

// M__contains__ is "in" over the byte values.
func (m *MemoryView) M__contains__(item Object) (Object, error) {
	v, err := MakeGoInt(item)
	if err != nil {
		return nil, err
	}
	for _, c := range m.b {
		if int64(c) == int64(v) {
			return True, nil
		}
	}
	return False, nil
}

// M__eq__ compares the bytes, so a view of the same bytes is equal.
func (m *MemoryView) M__eq__(other Object) (Object, error) {
	switch o := other.(type) {
	case *MemoryView:
		if len(m.b) != len(o.b) {
			return False, nil
		}
		for i := range m.b {
			if m.b[i] != o.b[i] {
				return False, nil
			}
		}
		return True, nil
	}
	return NotImplemented, nil
}

// M__hash__ is deliberately absent in the type's dict: a writable view is
// unhashable in CPython, and a readonly one is hashable.  Rather than model two
// cases, the type declines and raises, which is what a caller can rely on.
func (m *MemoryView) M__hash__() (Object, error) {
	if !m.readonly {
		return nil, ExceptionNewf(TypeError, "unhashable type: 'memoryview'")
	}
	// A readonly view hashes like the BYTES it covers, so a view and the bytes
	// it was made from hash alike - which is what CPython does.
	//
	// The hash goes through the same encoder that DictKey uses, because py's
	// scalar types carry no M__hash__ of their own; that encoder is the one
	// place a Python-level hash is computed here.
	// The hash is exposed through a package variable because it is defined by
	// the builtin module, which this package cannot import.  When it is unset
	// the view declines rather than inventing a hash that would disagree with
	// the bytes it covers.
	if MemoryHash == nil {
		return nil, ExceptionNewf(TypeError, "unhashable type: 'memoryview'")
	}
	return Int(MemoryHash(m.b)), nil
}

func (m *MemoryView) M__repr__() (Object, error) {
	return String(fmt.Sprintf("<memory at %p>", m)), nil
}

// M__str__ is the same as repr, as CPython has it: a memoryview prints as
// "<memory at 0x...>", not as its bytes.
func (m *MemoryView) M__str__() (Object, error) { return m.M__repr__() }

func init() {
	MemoryViewType.Dict.Set("format", &Property{
		Fget: func(self Object) (Object, error) { return String("B"), nil },
	})
	MemoryViewType.Dict.Set("itemsize", &Property{
		Fget: func(self Object) (Object, error) { return Int(1), nil },
	})
	MemoryViewType.Dict.Set("ndim", &Property{
		Fget: func(self Object) (Object, error) { return Int(1), nil },
	})
	MemoryViewType.Dict.Set("shape", &Property{
		Fget: func(self Object) (Object, error) {
			return Tuple{Int(len(self.(*MemoryView).b))}, nil
		},
	})
	MemoryViewType.Dict.Set("strides", &Property{
		Fget: func(self Object) (Object, error) { return Tuple{Int(1)}, nil },
	})
	// nbytes is itemsize * the number of items, so for a byte view it is the
	// length.
	MemoryViewType.Dict.Set("nbytes", &Property{
		Fget: func(self Object) (Object, error) { return Int(len(self.(*MemoryView).b)), nil },
	})
	MemoryViewType.Dict.Set("readonly", &Property{
		Fget: func(self Object) (Object, error) { return NewBool(self.(*MemoryView).readonly), nil },
	})
	// obj is the object the view was made from.
	MemoryViewType.Dict.Set("obj", &Property{
		Fget: func(self Object) (Object, error) { return self.(*MemoryView).source, nil },
	})

	MemoryViewType.Dict.Set("tobytes", MustNewMethod("tobytes", func(self Object, args Tuple) (Object, error) {
		m := self.(*MemoryView)
		// A copy, which is the whole point of the name.
		out := make([]byte, len(m.b))
		copy(out, m.b)
		return Bytes(out), nil
	}, 0, "Return the data as a bytes object, copied."))

	MemoryViewType.Dict.Set("tolist", MustNewMethod("tolist", func(self Object, args Tuple) (Object, error) {
		m := self.(*MemoryView)
		out := make([]Object, 0, len(m.b))
		for _, c := range m.b {
			out = append(out, Int(c))
		}
		return NewListFromItems(out), nil
	}, 0, "Return the data as a list of elements."))

	MemoryViewType.Dict.Set("hex", MustNewMethod("hex", func(self Object, args Tuple) (Object, error) {
		m := self.(*MemoryView)
		return String(hex.EncodeToString(m.b)), nil
	}, 0, "Return the data as a string of hexadecimal digits."))

	MemoryViewType.Dict.Set("cast", MustNewMethod("cast", func(self Object, args Tuple) (Object, error) {
		return nil, ExceptionNewf(NotImplementedError,
			"memoryview.cast() is not implemented: only a 1-dimensional byte view is modelled")
	}, 0, "Not implemented; only a 1-dimensional byte view is modelled."))

	MemoryViewType.Dict.Set("release", MustNewMethod("release", func(self Object, args Tuple) (Object, error) {
		// A no-op, deliberately: there is no reference count to drop and no
		// exporting object to unblock, so there is nothing for this to do.  It
		// is here because code calls it in a finally block.
		return None, nil
	}, 0, "Release the underlying buffer exposed by the memoryview object."))
}

// MemoryHash computes the hash of a byte string, matching what hash() gives for
// bytes.  It is set by the builtin module at init, because the hash lives
// there and this package cannot import it.
var MemoryHash func([]byte) int64

// Check the interfaces are satisfied.
var _ I__len__ = (*MemoryView)(nil)
var _ I__getitem__ = (*MemoryView)(nil)
var _ I__setitem__ = (*MemoryView)(nil)
var _ I__iter__ = (*MemoryView)(nil)
var _ I__contains__ = (*MemoryView)(nil)
var _ I__eq__ = (*MemoryView)(nil)
