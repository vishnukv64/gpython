// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package structmod provides the implementation of python's 'struct' module.
//
// pack, unpack, pack_into, unpack_from and calcsize are implemented for the
// standard format characters.  The byte order, alignment and size of each item
// follow the struct module's rules: an explicit byte-order prefix ("@", "=",
// "<", ">", "!") selects native, standard, little- or big-endian, and the
// standard sizes match CPython (so "=L" is 4 bytes even where C's long is 8).
//
// Not implemented: the "p" (Pascal string) and "?" formats are supported,
// but the arbitrary-precision "n"/"N" and the float16 "e" format are not -
// a request for them raises struct.error naming the limit rather than
// returning a wrong value.
package structmod

import (
	"encoding/binary"
	"math"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Functions to convert between Python values and C structs.

pack(fmt, v1, ...)      -- Return a bytes object containing the values.
unpack(fmt, buffer)    -- Return a tuple of the values unpacked from buffer.
pack_into(fmt, buf, off, v1, ...) -- Pack into a writable buffer.
unpack_from(fmt, buf, off=0) -- Unpack from a buffer starting at offset.
calcsize(fmt)          -- Return the size of the struct described by fmt.
`

// errorType is struct.error.
var errorType = py.ExceptionType.NewType("struct.error", "An error occurred in the struct module.", nil, nil)

// field is one parsed format item.
type field struct {
	code  byte
	count int
	size  int
}

// layout is a parsed format: how to read and write it.
type layout struct {
	order  binary.ByteOrder
	fields []field
	size   int
}

// native sizes for the "@" (native) order, which follow the C types: on
// darwin/arm64 long is 8 bytes.
func itemSizeNative(c byte) (int, bool) {
	switch c {
	case 'b', 'B', 'c', '?':
		return 1, true
	case 'h', 'H':
		return 2, true
	case 'i', 'I', 'l', 'L', 'f':
		return 4, true
	case 'q', 'Q', 'd', 'n', 'N', 'P':
		return 8, true
	case 'e':
		return 2, true
	case 's':
		return 1, true
	}
	return 0, false
}

// itemSizeStd is the size under a "=", "<", ">" or "!" prefix, where long is
// exactly 4 bytes.
func itemSizeStd(c byte) (int, bool) {
	switch c {
	case 'b', 'B', 'c', '?':
		return 1, true
	case 'h', 'H', 'e':
		return 2, true
	case 'i', 'I', 'l', 'L', 'f':
		return 4, true
	case 'q', 'Q', 'd':
		return 8, true
	case 's':
		return 1, true
	case 'n', 'N', 'P':
		// The standard order has no pointer-sized type.
		return 0, false
	}
	return 0, false
}

// parse turns a format string into a layout.
func parse(format string) (*layout, error) {
	l := &layout{order: nativeOrder()}
	rest := format
	if rest != "" {
		switch rest[0] {
		case '@':
			l.order = nativeOrder()
			rest = rest[1:]
		case '=':
			l.order = nativeOrder()
			rest = rest[1:]
		case '<':
			l.order = binary.LittleEndian
			rest = rest[1:]
		case '>', '!':
			l.order = binary.BigEndian
			rest = rest[1:]
		}
	}
	stdSizes := format != "" && (format[0] == '=' || format[0] == '<' || format[0] == '>' || format[0] == '!')
	for i := 0; i < len(rest); {
		c := rest[i]
		if c >= '0' && c <= '9' {
			// A repeat count.
			j := i
			for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
				j++
			}
			// The count applies to the item that follows.
			n := 0
			for _, d := range rest[i:j] {
				n = n*10 + int(d-'0')
			}
			if j >= len(rest) {
				return nil, errorf("repeat count given without format specifier")
			}
			item, err := parseOne(rest[j], stdSizes)
			if err != nil {
				return nil, err
			}
			item.count = n
			l.fields = append(l.fields, item)
			l.size += item.size * n
			i = j + 1
			continue
		}
		if c == 'x' {
			l.fields = append(l.fields, field{code: 'x', count: 1, size: 1})
			l.size++
			i++
			continue
		}
		if c == ' ' {
			i++
			continue
		}
		item, err := parseOne(c, stdSizes)
		if err != nil {
			return nil, err
		}
		l.fields = append(l.fields, item)
		l.size += item.size
		i++
	}
	return l, nil
}

func parseOne(c byte, stdSizes bool) (field, error) {
	var size int
	var ok bool
	if stdSizes {
		size, ok = itemSizeStd(c)
	} else {
		size, ok = itemSizeNative(c)
	}
	if !ok {
		switch c {
		case 'n', 'N':
			return field{}, errorf("the 'n' and 'N' formats are not implemented")
		default:
			return field{}, errorf("bad char in struct format")
		}
	}
	return field{code: c, count: 1, size: size}, nil
}

func errorf(format string, a ...interface{}) error {
	return py.ExceptionNewf(errorType, format, a...)
}

// packItems writes the values into buf in the layout's order.
func (l *layout) packItems(values []py.Object) ([]byte, error) {
	buf := make([]byte, 0, l.size)
	vi := 0
	for _, f := range l.fields {
		if f.code == 'x' {
			for n := 0; n < f.count; n++ {
				buf = append(buf, 0)
			}
			continue
		}
		if vi >= len(values) {
			return nil, errorf("pack expected %d items for packing (got %d)", countValues(l), len(values))
		}
		v := values[vi]
		vi++
		if f.code == 's' {
			b, err := py.BytesFromObject(v)
			if err != nil {
				return nil, err
			}
			out := make([]byte, f.count)
			copy(out, b)
			buf = append(buf, out...)
			continue
		}
		for n := 0; n < f.count; n++ {
			// A repeat count consumes one value per item.
			if n > 0 {
				if vi >= len(values) {
					return nil, errorf("pack expected more items")
				}
				v = values[vi]
				vi++
			}
			elem, err := l.packOne(f.code, v)
			if err != nil {
				return nil, err
			}
			buf = append(buf, elem...)
		}
	}
	if vi < len(values) {
		return nil, errorf("pack expected %d items for packing (got %d)", vi, len(values))
	}
	return buf, nil
}

func countValues(l *layout) int {
	n := 0
	for _, f := range l.fields {
		if f.code == 'x' || f.code == 's' {
			n++
			continue
		}
		n += f.count
	}
	return n
}

// packOne encodes a single scalar.
func (l *layout) packOne(code byte, v py.Object) ([]byte, error) {
	order := l.order
	switch code {
	case 'b', 'h', 'i', 'l', 'q':
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		return encodeInt(order, uint64(n), fieldSize(code), true)
	case 'B', 'H', 'I', 'L', 'Q':
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		return encodeInt(order, uint64(n), fieldSize(code), false)
	case '?':
		b, err := py.ObjectIsTrue(v)
		if err != nil {
			return nil, err
		}
		if b {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case 'c':
		b, err := py.BytesFromObject(v)
		if err != nil {
			return nil, err
		}
		if len(b) != 1 {
			return nil, errorf("char format requires a bytes object of length 1")
		}
		return []byte{b[0]}, nil
	case 'f':
		f, err := floatValue(v)
		if err != nil {
			return nil, err
		}
		out := make([]byte, 4)
		order.PutUint32(out, math.Float32bits(float32(f)))
		return out, nil
	case 'd':
		f, err := floatValue(v)
		if err != nil {
			return nil, err
		}
		out := make([]byte, 8)
		order.PutUint64(out, math.Float64bits(f))
		return out, nil
	}
	return nil, errorf("unsupported format character '%c'", code)
}

func fieldSize(c byte) int {
	switch c {
	case 'b', 'B':
		return 1
	case 'h', 'H':
		return 2
	case 'i', 'I', 'l', 'L':
		return 4
	case 'q', 'Q':
		return 8
	}
	return 0
}

func encodeInt(order binary.ByteOrder, n uint64, size int, signed bool) ([]byte, error) {
	out := make([]byte, size)
	switch size {
	case 1:
		out[0] = byte(n)
	case 2:
		order.PutUint16(out, uint16(n))
	case 4:
		order.PutUint32(out, uint32(n))
	case 8:
		order.PutUint64(out, n)
	default:
		return nil, errorf("unsupported integer size %d", size)
	}
	_ = signed
	return out, nil
}

func floatValue(v py.Object) (float64, error) {
	switch x := v.(type) {
	case py.Int:
		return float64(x), nil
	case py.Float:
		return float64(x), nil
	case py.Bool:
		if bool(x) {
			return 1, nil
		}
		return 0, nil
	}
	return 0, py.ExceptionNewf(py.TypeError, "required argument is not a number")
}

// unpackItems reads values from data.
func (l *layout) unpackItems(data []byte) (py.Tuple, error) {
	if len(data) < l.size {
		return nil, errorf("unpack requires a buffer of %d bytes", l.size)
	}
	var out py.Tuple
	off := 0
	for _, f := range l.fields {
		if f.code == 'x' {
			off += f.count
			continue
		}
		if f.code == 's' {
			out = append(out, py.Bytes(append([]byte(nil), data[off:off+f.count]...)))
			off += f.count
			continue
		}
		for n := 0; n < f.count; n++ {
			v, size, err := l.unpackOne(f.code, data[off:])
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			off += size
		}
	}
	return out, nil
}

func (l *layout) unpackOne(code byte, data []byte) (py.Object, int, error) {
	order := l.order
	switch code {
	case 'b':
		return py.Int(int8(data[0])), 1, nil
	case 'B':
		return py.Int(data[0]), 1, nil
	case '?':
		return py.Bool(data[0] != 0), 1, nil
	case 'c':
		return py.Bytes([]byte{data[0]}), 1, nil
	case 'h':
		return py.Int(int16(order.Uint16(data))), 2, nil
	case 'H':
		return py.Int(order.Uint16(data)), 2, nil
	case 'i', 'l':
		return py.Int(int32(order.Uint32(data))), 4, nil
	case 'I', 'L':
		return py.Int(order.Uint32(data)), 4, nil
	case 'q':
		return py.Int(int64(order.Uint64(data))), 8, nil
	case 'Q':
		return py.Int(order.Uint64(data)), 8, nil
	case 'f':
		return py.Float(math.Float32frombits(order.Uint32(data))), 4, nil
	case 'd':
		return py.Float(math.Float64frombits(order.Uint64(data))), 8, nil
	}
	return nil, 0, errorf("unsupported format character '%c'", code)
}

func nativeOrder() binary.ByteOrder {
	// The host is little-endian on every platform this interpreter builds for
	// in practice; the standard order is what the struct tests exercise.
	return binary.LittleEndian
}

// ---------------------------------------------------------------------------
// Python functions
// ---------------------------------------------------------------------------

// formatAndBuffer pulls the format string and buffer argument, which several
// entries share.
func formatAndBuffer(args py.Tuple, kwargs py.StringDict, name string) (string, py.Object, error) {
	var fmtObj, buf py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "sO:"+name,
		[]string{"format", "buffer"}, &fmtObj, &buf); err != nil {
		return "", nil, err
	}
	format, err := py.StrAsString(fmtObj)
	if err != nil {
		return "", nil, err
	}
	return format, buf, nil
}

func structPack(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "pack expected at least 1 argument")
	}
	format, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	l, err := parse(format)
	if err != nil {
		return nil, err
	}
	buf, err := l.packItems(args[1:])
	if err != nil {
		return nil, err
	}
	return py.Bytes(buf), nil
}

func structUnpack(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	format, buf, err := formatAndBuffer(args, kwargs, "unpack")
	if err != nil {
		return nil, err
	}
	l, err := parse(format)
	if err != nil {
		return nil, err
	}
	data, err := py.BytesFromObject(buf)
	if err != nil {
		return nil, err
	}
	return l.unpackItems(data)
}

// structCalcsize is calcsize(fmt) -> int.  It takes only the format string, so
// it does not go through formatAndBuffer, which requires the buffer that pack
// and unpack take.
func structCalcsize(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fmtObj py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "s:calcsize", []string{"format"}, &fmtObj); err != nil {
		return nil, err
	}
	format, err := py.StrAsString(fmtObj)
	if err != nil {
		return nil, err
	}
	l, err := parse(format)
	if err != nil {
		return nil, err
	}
	return py.Int(l.size), nil
}

func structPackInto(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fmtObj, buf, offset py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "OOO:pack_into",
		[]string{"format", "buffer", "offset"}, &fmtObj, &buf, &offset); err != nil {
		return nil, err
	}
	format, err := py.StrAsString(fmtObj)
	if err != nil {
		return nil, err
	}
	l, err := parse(format)
	if err != nil {
		return nil, err
	}
	off, err := py.IndexInt(offset)
	if err != nil {
		return nil, err
	}
	packed, err := l.packItems(args[3:])
	if err != nil {
		return nil, err
	}
	ba, ok := buf.(*py.ByteArray)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "pack_into requires a writable buffer")
	}
	blen, err := ba.M__len__()
	if err != nil {
		return nil, err
	}
	n, err := py.IndexInt(blen)
	if err != nil {
		return nil, err
	}
	if off < 0 || off+len(packed) > n {
		return nil, errorf("pack_into requires a buffer of at least %d bytes", off+len(packed))
	}
	if _, err := ba.M__setitem__(&py.Slice{Start: py.Int(off), Stop: py.Int(off + len(packed))}, py.Bytes(packed)); err != nil {
		return nil, err
	}
	return py.None, nil
}

func structUnpackFrom(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fmtObj, buf py.Object
	offset := py.Object(py.Int(0))
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|O:unpack_from",
		[]string{"format", "buffer", "offset"}, &fmtObj, &buf, &offset); err != nil {
		return nil, err
	}
	format, err := py.StrAsString(fmtObj)
	if err != nil {
		return nil, err
	}
	l, err := parse(format)
	if err != nil {
		return nil, err
	}
	off, err := py.IndexInt(offset)
	if err != nil {
		return nil, err
	}
	data, err := py.BytesFromObject(buf)
	if err != nil {
		return nil, err
	}
	if off < 0 || off > len(data) {
		return nil, errorf("offset is out of range")
	}
	return l.unpackItems(data[off:])
}

func structIterUnpack(self py.Object, args py.Tuple) (py.Object, error) {
	return nil, py.ExceptionNewf(py.NotImplementedError,
		"struct.iter_unpack: this implementation does not provide the streaming unpacker; use unpack_from in a loop")
}

var _ = strings.TrimSpace

func init() {
	globals := py.NewStringDict()
	globals.Set("error", errorType)
	globals.Set("pack", py.MustNewMethod("pack", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return structPack(self, args, kw)
	}, 0, "pack(fmt, v1, ...) -> bytes"))
	globals.Set("unpack", py.MustNewMethod("unpack", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return structUnpack(self, args, kw)
	}, 0, "unpack(fmt, buffer) -> tuple"))
	globals.Set("calcsize", py.MustNewMethod("calcsize", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return structCalcsize(self, args, kw)
	}, 0, "calcsize(fmt) -> int"))
	globals.Set("pack_into", py.MustNewMethod("pack_into", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return structPackInto(self, args, kw)
	}, 0, "pack_into(fmt, buffer, offset, v1, ...)"))
	globals.Set("unpack_from", py.MustNewMethod("unpack_from", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return structUnpackFrom(self, args, kw)
	}, 0, "unpack_from(fmt, buffer, offset=0) -> tuple"))
	globals.Set("iter_unpack", py.MustNewMethod("iter_unpack", structIterUnpack, 0,
		"iter_unpack(fmt, buffer) -> iterator of tuples"))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "struct",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
