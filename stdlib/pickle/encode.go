// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pickle

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// encoder accumulates the protocol-0 opcodes.
//
// The memo (PUT p<n> / GET g<n>) is what keeps a shared sub-object one object
// in the unpickled graph, and is what makes a self-referential container
// expressible at all: the container is PUT before its items are written, so an
// item can GET it back.  CPython PUTs exactly the objects it memoises - the
// mutable containers and the tuple - and leaves the immutable scalars
// unmemoised, and this follows that.
type encoder struct {
	buf  strings.Builder
	memo map[uintptr]int
	next int
}

func newEncoder() *encoder {
	return &encoder{memo: map[uintptr]int{}}
}

// key identifies a memoable object.  StringDict reports its storage pointer,
// which is its identity; the other containers are Go pointers.
func key(v py.Object) uintptr {
	switch t := v.(type) {
	case py.StringDict:
		return t.Ptr()
	case *py.List:
		return pointerOf(t)
	case py.Tuple:
		if len(t) == 0 {
			return 0 // the empty tuple is a singleton CPython does not memoise
		}
		return pointerOf(t)
	case *py.Set:
		return pointerOf(t)
	case *py.FrozenSet:
		return pointerOf(t)
	}
	return 0
}

func (e *encoder) put(v py.Object) (int, bool) {
	k := key(v)
	if k == 0 {
		return 0, false
	}
	if idx, ok := e.memo[k]; ok {
		return idx, true
	}
	return 0, false
}

// memo records v and writes its PUT, before the body, so a self-reference in
// the body can GET it.
func (e *encoder) memoPut(v py.Object) int {
	idx := e.next
	e.next++
	e.memo[key(v)] = idx
	fmt.Fprintf(&e.buf, "p%d\n", idx)
	return idx
}

func (e *encoder) encode(v py.Object, depth int) error {
	if depth > 1000 {
		return py.ExceptionNewf(PicklingError, "maximum recursion depth exceeded")
	}
	// A reference to something already written is a GET.
	if idx, ok := e.put(v); ok {
		fmt.Fprintf(&e.buf, "g%d\n", idx)
		return nil
	}
	switch t := v.(type) {
	case py.NoneType:
		e.buf.WriteString("N")
	case py.Bool:
		if t {
			e.buf.WriteString("I01\n")
		} else {
			e.buf.WriteString("I00\n")
		}
	case py.Int:
		fmt.Fprintf(&e.buf, "I%d\n", int64(t))
	case *py.BigInt:
		dec, err := bigIntDecimal(t)
		if err != nil {
			return err
		}
		fmt.Fprintf(&e.buf, "L%sL\n", dec)
	case py.Float:
		e.encodeFloat(float64(t))
	case py.Complex:
		e.encodeComplex(t)
	case py.String:
		fmt.Fprintf(&e.buf, "V%s\n", escapeUnicode(string(t)))
	case py.Bytes:
		e.encodeBytes([]byte(t))
	case *py.ByteArray:
		raw, err := bytesOf(t)
		if err != nil {
			return err
		}
		e.encodeByteArray(raw)
	case py.Tuple:
		return e.encodeTuple(t, depth)
	case *py.List:
		return e.encodeList(t, depth)
	case py.StringDict:
		return e.encodeDict(t, depth)
	case *py.Set:
		return e.encodeSet(t, false, depth)
	case *py.FrozenSet:
		return e.encodeSet(t, true, depth)
	default:
		return py.ExceptionNewf(PicklingError, "cannot pickle '%s' object", v.Type().Name)
	}
	return nil
}

func (e *encoder) encodeFloat(f float64) {
	switch {
	case math.IsInf(f, 1):
		e.buf.WriteString("F1e999\n")
	case math.IsInf(f, -1):
		e.buf.WriteString("F-1e999\n")
	case math.IsNaN(f):
		e.buf.WriteString("Fnan\n")
	default:
		// Go's shortest round-tripping form is also what Python's float()
		// reads back exactly.
		fmt.Fprintf(&e.buf, "F%s\n", strconv.FormatFloat(f, 'g', -1, 64))
	}
}

// encodeComplex writes the __builtin__.complex reduce form.
func (e *encoder) encodeComplex(c py.Complex) {
	e.buf.WriteString("c__builtin__\ncomplex\n")
	e.buf.WriteString("(")
	e.encodeFloat(real(c))
	e.encodeFloat(imag(c))
	e.buf.WriteString("tR")
}

func (e *encoder) encodeBytes(raw []byte) {
	// Protocol 0 has no bytes opcode.  CPython writes the _codecs.encode
	// reduce: the bytes as a latin-1 str, then the encoding name.
	e.buf.WriteString("c_codecs\nencode\n")
	e.buf.WriteString("(")
	fmt.Fprintf(&e.buf, "V%s\n", escapeUnicode(latin1(raw)))
	fmt.Fprintf(&e.buf, "V%s\n", "latin1")
	e.buf.WriteString("tR")
}

func (e *encoder) encodeByteArray(raw []byte) {
	// bytearray(<bytes>) is the reduce CPython writes.
	e.buf.WriteString("c__builtin__\nbytearray\n")
	e.buf.WriteString("(")
	e.encodeBytes(raw)
	e.buf.WriteString("tR")
}

// encodeTuple writes the MARK..TUPLE form.  CPython PUTs the tuple after the
// TUPLE opcode, which is why the PUT follows the body here: that position is
// what lets a later reference GET it back.
func (e *encoder) encodeTuple(t py.Tuple, depth int) error {
	if len(t) == 0 {
		e.buf.WriteString("(t")
		return nil
	}
	e.buf.WriteString("(")
	if err := e.encodeItems([]py.Object(t), depth); err != nil {
		return err
	}
	e.buf.WriteString("t")
	if key(t) != 0 {
		e.memoPut(t)
	}
	return nil
}

func (e *encoder) encodeList(l *py.List, depth int) error {
	if len(l.Items) == 0 {
		e.buf.WriteString("(l")
		return nil
	}
	e.buf.WriteString("(l")
	e.memoPut(l)
	for _, item := range l.Items {
		if err := e.encode(item, depth+1); err != nil {
			return err
		}
		e.buf.WriteString("a")
	}
	return nil
}

func (e *encoder) encodeDict(d py.StringDict, depth int) error {
	if d.Len() == 0 {
		e.buf.WriteString("(d")
		return nil
	}
	e.buf.WriteString("(d")
	e.memoPut(d)
	var encErr error
	d.Range(func(k string, v py.Object) bool {
		if encErr != nil {
			return true
		}
		orig, err := py.DictKeyDecode(k)
		if err != nil {
			orig = py.String(k)
		}
		if err := e.encode(orig, depth+1); err != nil {
			encErr = err
			return true
		}
		if err := e.encode(v, depth+1); err != nil {
			encErr = err
			return true
		}
		e.buf.WriteString("s")
		return false
	})
	if encErr != nil {
		return encErr
	}
	return nil
}

// encodeSet writes a set or frozenset through its constructor: set(<list>).
// The members are read through the public iterator, since the storage is
// unexported.
func (e *encoder) encodeSet(members py.Object, frozen bool, depth int) error {
	name := "set"
	if frozen {
		name = "frozenset"
	}
	e.buf.WriteString("c__builtin__\n" + name + "\n")
	e.buf.WriteString("(")
	e.buf.WriteString("(")
	e.buf.WriteString("l")
	it, err := py.Iter(members)
	if err != nil {
		return err
	}
	for {
		item, err := py.Next(it)
		if err == py.StopIteration {
			break
		}
		if err != nil {
			return err
		}
		if err := e.encode(item, depth+1); err != nil {
			return err
		}
		e.buf.WriteString("a")
	}
	e.buf.WriteString("tR")
	return nil
}

func (e *encoder) encodeItems(items []py.Object, depth int) error {
	for _, item := range items {
		if err := e.encode(item, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// escapeUnicode escapes the characters the V opcode's line-terminated form
// cannot carry bare.  CPython writes the two- and four-hex-digit forms with
// backslash-u and backslash-U.
func escapeUnicode(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\u005c`)
		case '\n':
			b.WriteString(`\u000a`)
		case '\r':
			b.WriteString(`\u000d`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04x`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// latin1 is the str whose code points are the bytes of raw.
func latin1(raw []byte) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, c := range raw {
		b.WriteRune(rune(c))
	}
	return b.String()
}

// pointerOf identifies a Go pointer as a uintptr for the memo.
func pointerOf(v py.Object) uintptr {
	return reflect.ValueOf(v).Pointer()
}

// bytesOf is the bytes value of a bytearray, copying as bytes() does.
func bytesOf(o *py.ByteArray) ([]byte, error) {
	out, err := o.M__bytes__()
	if err != nil {
		return nil, err
	}
	return []byte(out.(py.Bytes)), nil
}

// setItemsOf is unused; members are read through the public iterator above.

// bigIntDecimal is the base-10 spelling of an arbitrary-precision int, which is
// what the L opcode carries.
func bigIntDecimal(b *py.BigInt) (string, error) {
	s, err := b.M__str__()
	if err != nil {
		return "", err
	}
	return string(s.(py.String)), nil
}
