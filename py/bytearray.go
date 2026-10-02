// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Bytearray objects
//
// A bytearray is the mutable counterpart of bytes: the same sequence of
// 0..255 integers, but resizable in place.  The value is held in a []byte so
// that append/extend/insert do not have to reallocate the whole object.
//
// Nearly all of the read-only behaviour is shared with bytes through the
// helpers in bytes.go, which take the raw byte slice; only the methods that
// mutate (and the ones that must return a bytearray rather than a bytes) are
// special to this file.

package py

var ByteArrayType = ObjectType.NewType("bytearray",
	`bytearray(iterable_of_ints) -> bytearray
bytearray(string, encoding[, errors]) -> bytearray
bytearray(bytes_or_buffer) -> mutable copy of bytes_or_buffer
bytearray(int) -> bytes array of size given by the parameter initialized with null bytes
bytearray() -> empty bytes array

Construct a mutable bytearray object from:
  - an iterable yielding integers in range(256)
  - a text string encoded using the specified encoding
  - a bytes or a buffer object
  - any object implementing the buffer API.
  - an integer`, ByteArrayNew, nil)

// ByteArray is a mutable sequence of bytes.  It is a struct rather than a
// plain []byte because the slice has to be replaceable in place: a method
// that resizes the array assigns to o.b, and every method reaches the storage
// through the pointer.
type ByteArray struct {
	b []byte
}

// Type of this ByteArray object
func (o *ByteArray) Type() *Type {
	return ByteArrayType
}

// NewByteArray returns a ByteArray over b.  The caller must not retain b.
func NewByteArray(b []byte) *ByteArray {
	return &ByteArray{b: b}
}

// ByteArrayNew implements the bytearray() constructor.
//
// The argument checks follow CPython's order: a non-str encoding is rejected
// before anything else, then encoding/errors supplied without a str source,
// then the str source is encoded, and only then is a non-str source converted.
func ByteArrayNew(metatype *Type, args Tuple, kwargs StringDict) (res Object, err error) {
	var (
		x        Object
		encoding Object
		errors   Object
		kwlist   = []string{"source", "encoding", "errors"}
	)

	err = ParseTupleAndKeywords(args, kwargs, "|OOO:bytearray", kwlist, &x, &encoding, &errors)
	if err != nil {
		return nil, err
	}

	if encoding != nil {
		if _, ok := encoding.(String); !ok {
			return nil, ExceptionNewf(TypeError, "bytearray() argument 'encoding' must be str, not %s", encoding.Type().Name)
		}
	}

	if x == nil {
		// No source: the only legal call is bytearray() itself.
		if encoding != nil {
			return nil, ExceptionNewf(TypeError, "encoding without a string argument")
		}
		if errors != nil {
			return nil, ExceptionNewf(TypeError, "errors without a string argument")
		}
		return &ByteArray{}, nil
	}

	if s, ok := x.(String); ok {
		if encoding == nil {
			return nil, ExceptionNewf(TypeError, "string argument without an encoding")
		}
		handler := "strict"
		if errors != nil {
			handler = string(errors.(String))
		}
		raw, err := encodeStringWith(string(encoding.(String)), string(s), handler)
		if err != nil {
			return nil, err
		}
		return &ByteArray{b: raw}, nil
	}

	if encoding != nil || errors != nil {
		if encoding == nil {
			return nil, ExceptionNewf(TypeError, "errors without a string argument")
		}
		return nil, ExceptionNewf(TypeError, "encoding without a string argument")
	}

	// Any other object: an int/bool gives a zero filled array of that size,
	// a bytes-like object is copied, and anything iterable is read as a
	// sequence of ints.
	if I, ok := x.(I__index__); ok && isPlainInt(x) {
		size, err := I.M__index__()
		if err != nil {
			return nil, err
		}
		n := int(size)
		if n < 0 {
			return nil, ExceptionNewf(ValueError, "negative count")
		}
		return &ByteArray{b: make([]byte, n)}, nil
	}

	if data, ok := bytesLikeData(x); ok {
		return &ByteArray{b: append([]byte(nil), data...)}, nil
	}

	if !objectIsIterable(x) {
		return nil, ExceptionNewf(TypeError, "cannot convert '%s' object to bytearray", x.Type().Name)
	}

	raw, err := byteArrayFromIterable(x)
	if err != nil {
		return nil, err
	}
	return &ByteArray{b: raw}, nil
}

// isPlainInt reports whether x is an int or bool rather than some other object
// that merely implements __index__.  bytearray(size) only treats the plain
// integer types as a length.
func isPlainInt(x Object) bool {
	switch x.(type) {
	case Int, *BigInt, Bool:
		return true
	}
	return false
}

// byteArrayFromIterable reads an iterable of ints into a fresh byte slice,
// reporting the bytearray flavour of the range error.
func byteArrayFromIterable(x Object) ([]byte, error) {
	out := []byte{}
	var loopErr error
	err := Iterate(x, func(item Object) bool {
		c, err := bytesIndexItem(item)
		if err != nil {
			loopErr = err
			return true
		}
		out = append(out, c)
		return false
	})
	if err != nil {
		return nil, err
	}
	if loopErr != nil {
		return nil, loopErr
	}
	return out, nil
}

// objectIsIterable reports whether Iter can make an iterator for x.  It is
// used to tell "cannot convert 'X' object to bytearray" from a failed
// element conversion.
func objectIsIterable(x Object) bool {
	if _, ok := x.(I__iter__); ok {
		return true
	}
	if ObjectIsSequence(x) {
		return true
	}
	if x.Type().Lookup("__iter__") != nil {
		return true
	}
	return false
}

func (o *ByteArray) M__len__() (Object, error) {
	return Int(len(o.b)), nil
}

func (o *ByteArray) M__contains__(item Object) (Object, error) {
	return bytesContainsItem(o.b, item)
}

func (o *ByteArray) M__iter__() (Object, error) {
	return NewIterator(o), nil
}

func (o *ByteArray) M__bytes__() (Object, error) {
	// bytes(bytearray) must copy: the bytearray can change afterwards.
	return Bytes(append([]byte(nil), o.b...)), nil
}

func (o *ByteArray) M__repr__() (Object, error) {
	return String("bytearray(" + bytearrayRepr(o.b) + ")"), nil
}

func (o *ByteArray) M__str__() (Object, error) {
	return o.M__repr__()
}

// M__hash__ exists so that hash() reports the CPython message and so that
// "bytearray.__hash__" is an attribute at all.  A bytearray is mutable and
// therefore unhashable.
func (o *ByteArray) M__hash__() (Object, error) {
	return nil, ExceptionNewf(TypeError, "unhashable type: 'bytearray'")
}

// Subscript

func (o *ByteArray) M__getitem__(key Object) (Object, error) {
	if sl, ok := key.(*Slice); ok {
		data, err := byteArraySlice(o.b, sl)
		if err != nil {
			return nil, err
		}
		return &ByteArray{b: data}, nil
	}
	i, err := byteArrayIndex(key, len(o.b))
	if err != nil {
		return nil, err
	}
	return Int(o.b[i]), nil
}

// byteArrayIndex resolves a single subscript, raising the bytearray flavour
// of the TypeError/IndexError CPython raises.
func byteArrayIndex(key Object, length int) (int, error) {
	switch key.(type) {
	case Int, Bool, *BigInt:
	default:
		if _, ok := key.(I__index__); !ok {
			return 0, ExceptionNewf(TypeError, "bytearray indices must be integers or slices, not %s", key.Type().Name)
		}
	}
	i, err := Index(key)
	if err != nil {
		if IsException(TypeError, err) {
			return 0, ExceptionNewf(TypeError, "bytearray indices must be integers or slices, not %s", key.Type().Name)
		}
		return 0, err
	}
	n := int(i)
	if i < 0 {
		n += length
	}
	if n < 0 || n >= length {
		return 0, ExceptionNewf(IndexError, "bytearray index out of range")
	}
	return n, nil
}

// byteArraySlice extracts the slice's span of b as a fresh slice.
func byteArraySlice(b []byte, sl *Slice) ([]byte, error) {
	start, _, step, slicelength, err := sl.GetIndices(len(b))
	if err != nil {
		return nil, err
	}
	out := make([]byte, slicelength)
	i := start
	for k := 0; k < slicelength; k++ {
		out[k] = b[i]
		i += step
	}
	return out, nil
}

func (o *ByteArray) M__setitem__(key, value Object) (Object, error) {
	if sl, ok := key.(*Slice); ok {
		data, err := bytesAssignValue(value)
		if err != nil {
			return nil, err
		}
		if err := o.setSlice(sl, data); err != nil {
			return nil, err
		}
		return None, nil
	}
	i, err := byteArrayIndex(key, len(o.b))
	if err != nil {
		return nil, err
	}
	c, err := bytesIndexItem(value)
	if err != nil {
		return nil, err
	}
	o.b[i] = c
	return None, nil
}

// setSlice assigns value to the span of the slice.  A slice with step 1 may
// change the length of the array; an extended slice must match it exactly.
func (o *ByteArray) setSlice(sl *Slice, value []byte) error {
	start, stop, step, slicelength, err := sl.GetIndices(len(o.b))
	if err != nil {
		return err
	}
	if step == 1 {
		nb := make([]byte, 0, len(o.b)-slicelength+len(value))
		nb = append(nb, o.b[:start]...)
		nb = append(nb, value...)
		nb = append(nb, o.b[stop:]...)
		o.b = nb
		return nil
	}
	if len(value) != slicelength {
		return ExceptionNewf(ValueError, "attempt to assign bytes of size %d to extended slice of size %d", len(value), slicelength)
	}
	i := start
	for _, c := range value {
		o.b[i] = c
		i += step
	}
	return nil
}

func (o *ByteArray) M__delitem__(key Object) (Object, error) {
	if sl, ok := key.(*Slice); ok {
		start, stop, step, slicelength, err := sl.GetIndices(len(o.b))
		if err != nil {
			return nil, err
		}
		if step == 1 {
			o.b = append(o.b[:start], o.b[stop:]...)
			return None, nil
		}
		gone := make([]bool, len(o.b))
		i := start
		for k := 0; k < slicelength; k++ {
			gone[i] = true
			i += step
		}
		nb := make([]byte, 0, len(o.b)-slicelength)
		for j, c := range o.b {
			if !gone[j] {
				nb = append(nb, c)
			}
		}
		o.b = nb
		return None, nil
	}
	i, err := byteArrayIndex(key, len(o.b))
	if err != nil {
		return nil, err
	}
	o.b = append(o.b[:i], o.b[i+1:]...)
	return None, nil
}

// Arithmetic

func (o *ByteArray) M__add__(other Object) (Object, error) {
	b, ok := bytesLikeData(other)
	if !ok {
		if other == None || other == NotImplemented {
			return NotImplemented, nil
		}
		return nil, ExceptionNewf(TypeError, "can't concat %s to bytearray", other.Type().Name)
	}
	out := make([]byte, 0, len(o.b)+len(b))
	out = append(out, o.b...)
	out = append(out, b...)
	return &ByteArray{b: out}, nil
}

// M__radd__ is what makes "b'x' + bytearray(b'y')" work: bytes cannot add a
// bytearray itself, so the reflected call lands here and CPython's result is
// an immutable bytes.
func (o *ByteArray) M__radd__(other Object) (Object, error) {
	b, ok := bytesLikeData(other)
	if !ok {
		return NotImplemented, nil
	}
	out := make([]byte, 0, len(b)+len(o.b))
	out = append(out, b...)
	out = append(out, o.b...)
	return Bytes(out), nil
}

func (o *ByteArray) M__iadd__(other Object) (Object, error) {
	b, ok := bytesLikeData(other)
	if !ok {
		if other == None || other == NotImplemented {
			return NotImplemented, nil
		}
		return nil, ExceptionNewf(TypeError, "can't concat %s to bytearray", other.Type().Name)
	}
	o.b = append(o.b, b...)
	return o, nil
}

func (o *ByteArray) M__mul__(other Object) (Object, error) {
	n, err := bytesRepeatCount(other)
	if err != nil {
		return nil, err
	}
	return &ByteArray{b: bytesRepeat(o.b, n)}, nil
}

func (o *ByteArray) M__rmul__(other Object) (Object, error) {
	return o.M__mul__(other)
}

func (o *ByteArray) M__imul__(other Object) (Object, error) {
	n, err := bytesRepeatCount(other)
	if err != nil {
		return nil, err
	}
	o.b = bytesRepeat(o.b, n)
	return o, nil
}

// Rich comparisons

func (o *ByteArray) compare(other Object, op string) (Object, error) {
	b, ok := bytesLikeData(other)
	if !ok {
		return NotImplemented, nil
	}
	switch op {
	case "<":
		return NewBool(bytesCompare(o.b, b) < 0), nil
	case "<=":
		return NewBool(bytesCompare(o.b, b) <= 0), nil
	case "==":
		return NewBool(bytesEqual(o.b, b)), nil
	case "!=":
		return NewBool(!bytesEqual(o.b, b)), nil
	case ">":
		return NewBool(bytesCompare(o.b, b) > 0), nil
	case ">=":
		return NewBool(bytesCompare(o.b, b) >= 0), nil
	}
	return NotImplemented, nil
}

func (o *ByteArray) M__lt__(other Object) (Object, error) { return o.compare(other, "<") }
func (o *ByteArray) M__le__(other Object) (Object, error) { return o.compare(other, "<=") }
func (o *ByteArray) M__eq__(other Object) (Object, error) { return o.compare(other, "==") }
func (o *ByteArray) M__ne__(other Object) (Object, error) { return o.compare(other, "!=") }
func (o *ByteArray) M__gt__(other Object) (Object, error) { return o.compare(other, ">") }
func (o *ByteArray) M__ge__(other Object) (Object, error) { return o.compare(other, ">=") }

// Mutating methods

func (o *ByteArray) doAppend(args Tuple) (Object, error) {
	if len(args) != 1 {
		return nil, ExceptionNewf(TypeError, "bytearray.append() takes exactly one argument (%d given)", len(args))
	}
	c, err := bytesIndexItem(args[0])
	if err != nil {
		return nil, err
	}
	o.b = append(o.b, c)
	return None, nil
}

func (o *ByteArray) doExtend(args Tuple) (Object, error) {
	if len(args) != 1 {
		return nil, ExceptionNewf(TypeError, "bytearray.extend() takes exactly one argument (%d given)", len(args))
	}
	x := args[0]
	if b, ok := bytesLikeData(x); ok {
		o.b = append(o.b, b...)
		return None, nil
	}
	if _, ok := x.(String); ok {
		return nil, ExceptionNewf(TypeError, "expected iterable of integers; got: 'str'")
	}
	if !objectIsIterable(x) {
		return nil, ExceptionNewf(TypeError, "can't extend bytearray with %s", x.Type().Name)
	}
	raw, err := byteArrayFromIterable(x)
	if err != nil {
		return nil, err
	}
	o.b = append(o.b, raw...)
	return None, nil
}

func (o *ByteArray) doInsert(args Tuple) (Object, error) {
	if len(args) != 2 {
		return nil, ExceptionNewf(TypeError, "insert expected 2 arguments, got %d", len(args))
	}
	idx, err := bytesIntArg(args[0])
	if err != nil {
		return nil, err
	}
	c, err := bytesIndexItem(args[1])
	if err != nil {
		return nil, err
	}
	if idx < 0 {
		idx += len(o.b)
		if idx < 0 {
			idx = 0
		}
	}
	if idx > len(o.b) {
		idx = len(o.b)
	}
	o.b = append(o.b, 0)
	copy(o.b[idx+1:], o.b[idx:])
	o.b[idx] = c
	return None, nil
}

func (o *ByteArray) doRemove(args Tuple) (Object, error) {
	if len(args) != 1 {
		return nil, ExceptionNewf(TypeError, "bytearray.remove() takes exactly one argument (%d given)", len(args))
	}
	c, err := bytesIndexItem(args[0])
	if err != nil {
		return nil, err
	}
	i := indexByte(o.b, c)
	if i < 0 {
		return nil, ExceptionNewf(ValueError, "value not found in bytearray")
	}
	o.b = append(o.b[:i], o.b[i+1:]...)
	return None, nil
}

func (o *ByteArray) doPop(args Tuple, kwargs StringDict) (Object, error) {
	if kwargs.Len() != 0 {
		return nil, ExceptionNewf(TypeError, "pop() takes no keyword arguments")
	}
	if len(args) > 1 {
		return nil, ExceptionNewf(TypeError, "pop expected at most 1 argument, got %d", len(args))
	}
	idx := -1
	if len(args) == 1 {
		var err error
		idx, err = bytesIntArg(args[0])
		if err != nil {
			return nil, err
		}
	}
	n := len(o.b)
	if n == 0 {
		return nil, ExceptionNewf(IndexError, "pop from empty bytearray")
	}
	if idx < 0 {
		idx += n
	}
	if idx < 0 || idx >= n {
		return nil, ExceptionNewf(IndexError, "pop index out of range")
	}
	c := o.b[idx]
	o.b = append(o.b[:idx], o.b[idx+1:]...)
	return Int(c), nil
}

func (o *ByteArray) doClear(args Tuple) (Object, error) {
	if len(args) != 0 {
		return nil, ExceptionNewf(TypeError, "bytearray.clear() takes no arguments (%d given)", len(args))
	}
	o.b = o.b[:0]
	return None, nil
}

func (o *ByteArray) doCopy(args Tuple) (Object, error) {
	if len(args) != 0 {
		return nil, ExceptionNewf(TypeError, "bytearray.copy() takes no arguments (%d given)", len(args))
	}
	return &ByteArray{b: append([]byte(nil), o.b...)}, nil
}

func (o *ByteArray) doReverse(args Tuple) (Object, error) {
	if len(args) != 0 {
		return nil, ExceptionNewf(TypeError, "bytearray.reverse() takes no arguments (%d given)", len(args))
	}
	for i, j := 0, len(o.b)-1; i < j; i, j = i+1, j-1 {
		o.b[i], o.b[j] = o.b[j], o.b[i]
	}
	return None, nil
}

// indexByte is bytes.IndexByte for a plain []byte, kept local so this file does
// not need the bytes package just for one lookup.
func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

// read-only methods shared with bytes

func (o *ByteArray) doDecode(args Tuple, kwargs StringDict) (Object, error) {
	return bytesDecodeMethod(o.b, args, kwargs)
}

func (o *ByteArray) doHex(args Tuple, kwargs StringDict) (Object, error) {
	return bytesHexMethod(o.b, args, kwargs)
}

func (o *ByteArray) doFind(args Tuple, kwargs StringDict, name string, reverse, raise bool) (Object, error) {
	return bytesFindMethod(o.b, args, kwargs, name, reverse, raise)
}

func (o *ByteArray) doSplit(args Tuple, kwargs StringDict, reverse bool) (Object, error) {
	return bytesSplitMethod(o.b, true, args, kwargs, reverse)
}

func (o *ByteArray) doJoin(args Tuple) (Object, error) {
	return bytesJoinMethod(o.b, true, args)
}

func (o *ByteArray) doReplace(args Tuple) (Object, error) {
	return bytesReplaceMethod(o.b, true, args)
}

func (o *ByteArray) doStrip(args Tuple, kwargs StringDict, left, right bool) (Object, error) {
	return bytesStripMethod(o.b, true, args, kwargs, left, right)
}

func (o *ByteArray) doUpper(args Tuple) (Object, error) {
	if len(args) != 0 {
		return nil, ExceptionNewf(TypeError, "bytearray.upper() takes no arguments (%d given)", len(args))
	}
	return bytesCaseMethod(o.b, true, args, true), nil
}

func (o *ByteArray) doLower(args Tuple) (Object, error) {
	if len(args) != 0 {
		return nil, ExceptionNewf(TypeError, "bytearray.lower() takes no arguments (%d given)", len(args))
	}
	return bytesCaseMethod(o.b, true, args, false), nil
}

func (o *ByteArray) doStartEnd(args Tuple, kwargs StringDict, isEnd bool) (Object, error) {
	return bytesStartEndMethod(o.b, args, kwargs, isEnd)
}

func (o *ByteArray) doZfill(args Tuple, kwargs StringDict) (Object, error) {
	return bytesZfillMethod(o.b, true, args, kwargs)
}

func (o *ByteArray) doJustify(args Tuple, kwargs StringDict, mode int) (Object, error) {
	return bytesJustifyMethod(o.b, true, args, kwargs, mode)
}

func (o *ByteArray) doTranslate(args Tuple) (Object, error) {
	return bytesTranslateMethod(o.b, true, args)
}

func (o *ByteArray) doPartition(args Tuple, reverse bool) (Object, error) {
	return bytesPartitionMethod(o.b, true, args, reverse)
}

// fromhex is looked up on the type as well as on an instance; as with
// dict.fromkeys the arguments arrive unshifted, so self is ignored.

func ByteArrayFromHex(self Object, args Tuple, kwargs StringDict) (Object, error) {
	raw, err := bytesFromHexArg(args, kwargs, "bytearray.fromhex")
	if err != nil {
		return nil, err
	}
	return &ByteArray{b: raw}, nil
}

func init() {
	set := func(name string, fn func(self Object, args Tuple) (Object, error), doc string) {
		ByteArrayType.Dict.Set(name, MustNewMethod(name, fn, 0, doc))
	}
	setKw := func(name string, fn func(self Object, args Tuple, kwargs StringDict) (Object, error), doc string) {
		ByteArrayType.Dict.Set(name, MustNewMethod(name, fn, 0, doc))
	}
	self := func(fn func(o *ByteArray, args Tuple) (Object, error)) func(self Object, args Tuple) (Object, error) {
		return func(self Object, args Tuple) (Object, error) {
			return fn(self.(*ByteArray), args)
		}
	}
	selfKw := func(fn func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error)) func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return func(self Object, args Tuple, kwargs StringDict) (Object, error) {
			return fn(self.(*ByteArray), args, kwargs)
		}
	}

	ByteArrayType.Dict.Set("maketrans", MustNewMethod("maketrans", bytesMakeTransMethod, 0, `maketrans(frm, to) -> bytes

Return a translation table (a bytes object of length 256) suitable for use in
bytearray.translate.`))
	ByteArrayType.Dict.Set("fromhex", MustNewMethod("fromhex", ByteArrayFromHex, 0, `fromhex(string, /) -> bytearray

Create a bytearray from a string of hexadecimal numbers.

Spaces between two numbers are accepted.
Example: bytearray.fromhex('B9 01EF') -> bytearray(b'\xb9\x01\xef').`))

	set("append", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doAppend(args)
	}, "append(int) -> None.  Append a single byte to the bytearray.")
	set("extend", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doExtend(args)
	}, "extend(iterable_of_ints) -> None.  Append all the bytes from the iterable.")
	set("insert", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doInsert(args)
	}, "insert(index, int) -> None.  Insert a single byte before index.")
	set("remove", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doRemove(args)
	}, "remove(int) -> None.  Remove the first occurrence of the byte.")
	set("pop", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doPop(args, NewStringDict())
	}, "pop(index=-1) -> int.  Remove and return the byte at index.")
	setKw("pop", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(*ByteArray).doPop(args, kwargs)
	}, "pop(index=-1) -> int.  Remove and return the byte at index.")
	set("clear", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doClear(args)
	}, "clear() -> None.  Remove all items from the bytearray.")
	set("copy", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doCopy(args)
	}, "copy() -> a copy of the bytearray.")
	set("reverse", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doReverse(args)
	}, "reverse() -> None.  Reverse the bytearray in place.")

	setKw("decode", selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
		return o.doDecode(args, kwargs)
	}), `decode(encoding='utf-8', errors='strict') -> str

Decode the bytearray using the codec registered for encoding.`)

	setKw("hex", selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
		return o.doHex(args, kwargs)
	}), `hex([sep[, bytes_per_sep]]) -> str

Return a string containing the hexadecimal representation of the bytearray.`)

	findLike := func(name string, reverse, raise bool) {
		setKw(name, selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
			return o.doFind(args, kwargs, name, reverse, raise)
		}), name+`(sub[, start[, end]]) -> int

Return the lowest index in the bytearray where the subsequence sub is found.`)
	}
	findLike("find", false, false)
	findLike("rfind", true, false)
	findLike("index", false, true)

	setKw("count", selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
		return bytesCountMethod(o.b, args, kwargs)
	}), `count(sub[, start[, end]]) -> int

Return the number of non-overlapping occurrences of subsequence sub.`)

	setKw("split", selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
		return o.doSplit(args, kwargs, false)
	}), `split(sep=None, maxsplit=-1) -> list of bytearrays

Return a list of the sections of the bytearray, using sep as the delimiter.`)
	setKw("rsplit", selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
		return o.doSplit(args, kwargs, true)
	}), `rsplit(sep=None, maxsplit=-1) -> list of bytearrays

Return a list of the sections of the bytearray, using sep as the delimiter,
starting at the end.`)

	set("join", self(func(o *ByteArray, args Tuple) (Object, error) {
		return o.doJoin(args)
	}), "join(iterable_of_bytes) -> bytearray.  Concatenate any number of bytes-like objects.")

	set("replace", self(func(o *ByteArray, args Tuple) (Object, error) {
		return o.doReplace(args)
	}), `replace(old, new, count=-1) -> bytearray

Return a copy with all occurrences of substring old replaced by new.`)

	strip := func(name string, left, right bool) {
		setKw(name, selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
			return o.doStrip(args, kwargs, left, right)
		}), name+`([chars]) -> bytearray

Return a copy with leading/trailing bytes removed.`)
	}
	strip("strip", true, true)
	strip("lstrip", true, false)
	strip("rstrip", false, true)

	set("upper", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doUpper(args)
	}, "upper() -> bytearray.  Return a copy with all ASCII letters uppercased.")
	set("lower", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).doLower(args)
	}, "lower() -> bytearray.  Return a copy with all ASCII letters lowercased.")

	setKw("startswith", selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
		return o.doStartEnd(args, kwargs, false)
	}), "startswith(prefix[, start[, end]]) -> bool")
	setKw("endswith", selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
		return o.doStartEnd(args, kwargs, true)
	}), "endswith(suffix[, start[, end]]) -> bool")

	setKw("zfill", selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
		return o.doZfill(args, kwargs)
	}), "zfill(width) -> bytearray.  Pad a numeric string with zeros on the left.")

	justify := func(name string, mode int) {
		setKw(name, selfKw(func(o *ByteArray, args Tuple, kwargs StringDict) (Object, error) {
			return o.doJustify(args, kwargs, mode)
		}), name+`(width[, fillchar]) -> bytearray.  Pad the bytearray to width.`)
	}
	justify("center", justifyCenter)
	justify("ljust", justifyLeft)
	justify("rjust", justifyRight)

	set("translate", self(func(o *ByteArray, args Tuple) (Object, error) {
		return o.doTranslate(args)
	}), `translate(table, /, delete=b'') -> bytearray

Return a copy with each byte mapped through the given translation table.`)

	set("partition", self(func(o *ByteArray, args Tuple) (Object, error) {
		return o.doPartition(args, false)
	}), "partition(sep) -> (head, sep, tail)")
	set("rpartition", self(func(o *ByteArray, args Tuple) (Object, error) {
		return o.doPartition(args, true)
	}), "rpartition(sep) -> (head, sep, tail)")

	// The dunders are reached through the Go interfaces above, so registering
	// them is only needed to make the attributes visible, but it keeps the
	// type's namespace honest.
	// A bytearray is unhashable: M__hash__ above raises, and setting the type
	// entry to None is what CPython does so that "bytearray.__hash__" reads
	// None (the Go interface wins for hash(), so nothing else changes).
	ByteArrayType.Dict.Set("__hash__", None)
	ByteArrayType.Dict.Set("__len__", MustNewMethod("__len__", func(self Object, args Tuple) (Object, error) {
		return self.(*ByteArray).M__len__()
	}, METH_CLASS, "Return the number of bytes in the bytearray."))
	ByteArrayType.Dict.Set("__sizeof__", MustNewMethod("__sizeof__", func(self Object, args Tuple) (Object, error) {
		return Int(len(self.(*ByteArray).b)), nil
	}, 0, "Size of the bytearray object in memory."))
}

// Check interface is satisfied
var (
	_ richComparison = (*ByteArray)(nil)
	_ I__add__       = (*ByteArray)(nil)
	_ I__radd__      = (*ByteArray)(nil)
	_ I__iadd__      = (*ByteArray)(nil)
	_ I__mul__       = (*ByteArray)(nil)
	_ I__rmul__      = (*ByteArray)(nil)
	_ I__imul__      = (*ByteArray)(nil)
	_ I__len__       = (*ByteArray)(nil)
	_ I__getitem__   = (*ByteArray)(nil)
	_ I__setitem__   = (*ByteArray)(nil)
	_ I__delitem__   = (*ByteArray)(nil)
	_ I__iter__      = (*ByteArray)(nil)
	_ I__contains__  = (*ByteArray)(nil)
	_ I__hash__      = (*ByteArray)(nil)
	_ I__bytes__     = (*ByteArray)(nil)
	_ I__str__       = (*ByteArray)(nil)
	_ I__repr__      = (*ByteArray)(nil)
)
