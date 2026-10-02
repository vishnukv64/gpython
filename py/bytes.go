// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Bytes objects

package py

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

var BytesType = ObjectType.NewType("bytes",
	`bytes(iterable_of_ints) -> bytes
bytes(string, encoding[, errors]) -> bytes
bytes(bytes_or_buffer) -> immutable copy of bytes_or_buffer
bytes(int) -> bytes object of size given by the parameter initialized with null bytes
bytes() -> empty bytes object

Construct an immutable array of bytes from:
  - an iterable yielding integers in range(256)
  - a text string encoded using the specified encoding
  - any object implementing the buffer API.
  - an integer`, BytesNew, nil)

type Bytes []byte

// Type of this Bytes object
func (o Bytes) Type() *Type {
	return BytesType
}

// BytesNew
func BytesNew(metatype *Type, args Tuple, kwargs StringDict) (res Object, err error) {
	var x Object
	var encoding Object
	var errors Object
	var New Object
	kwlist := []string{"source", "encoding", "errors"}

	err = ParseTupleAndKeywords(args, kwargs, "|Oss:bytes", kwlist, &x, &encoding, &errors)
	if err != nil {
		return nil, err
	}
	if x == nil {
		if encoding != nil || errors != nil {
			return nil, ExceptionNewf(TypeError, "encoding without a string argument")
		}
		return Bytes{}, nil
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
		return Bytes(raw), nil
	}

	// A bytes-like source is copied verbatim before anything else: bytearray
	// and Bytes both reach here and neither should be reinterpreted as an
	// iterable of ints.
	if data, ok := bytesLikeData(x); ok {
		if encoding != nil || errors != nil {
			if encoding == nil {
				return nil, ExceptionNewf(TypeError, "errors without a string argument")
			}
			return nil, ExceptionNewf(TypeError, "encoding without a string argument")
		}
		return Bytes(append([]byte(nil), data...)), nil
	}

	// We'd like to call PyObject_Bytes here, but we need to check for an
	// integer argument before deferring to PyBytes_FromObject, something
	// PyObject_Bytes doesn't do.
	var ok bool
	if I, ok := x.(I__bytes__); ok {
		New, err = I.M__bytes__()
		if err != nil {
			return nil, err
		}
	} else if New, ok, err = TypeCall0(x, "__bytes__"); ok {
		if err != nil {
			return nil, err
		}
	} else {
		goto no_bytes_method
	}
	if _, ok = New.(Bytes); !ok {
		return nil, ExceptionNewf(TypeError, "__bytes__ returned non-bytes (type %s)", New.Type().Name)
	}
no_bytes_method:

	// Is it an integer?
	if isPlainInt(x) {
		size, err := MakeGoInt(x)
		if err != nil {
			return nil, err
		}
		if size < 0 {
			return nil, ExceptionNewf(ValueError, "negative count")
		}
		return make(Bytes, size), nil
	}

	// If it's not unicode, there can't be encoding or errors
	if encoding != nil || errors != nil {
		return nil, ExceptionNewf(TypeError, "encoding or errors without a string argument")
	}

	return BytesFromObject(x)
}

// Converts an object into bytes
func BytesFromObject(x Object) (Bytes, error) {
	// Look for special cases
	// FIXME implement converting from any object implementing the buffer API.
	switch z := x.(type) {
	case Bytes:
		// Immutable type so just return what was passed in
		return z, nil
	case String:
		return nil, ExceptionNewf(TypeError, "cannot convert unicode object to bytes")
	}
	// Otherwise iterate through the whatever converting it into ints
	b := Bytes{}
	var loopErr error
	iterErr := Iterate(x, func(item Object) bool {
		var value int
		value, loopErr = IndexInt(item)
		if loopErr != nil {
			return true
		}
		if value < 0 || value >= 256 {
			loopErr = ExceptionNewf(ValueError, "bytes must be in range(0, 256)")
			return true
		}
		b = append(b, byte(value))
		return false
	})
	if iterErr != nil {
		return nil, iterErr
	}
	if loopErr != nil {
		return nil, loopErr
	}
	return b, nil
}

func (a Bytes) M__str__() (Object, error) {
	return a.M__repr__()
}

func (a Bytes) M__repr__() (Object, error) {
	return String(bytesRepr(a)), nil
}

// bytesRepr formats b exactly as CPython's bytes.__repr__ does, including the
// choice of quote character: single quotes unless the value holds a ' and no ".
func bytesRepr(b []byte) string {
	return bytesReprWith(b, false)
}

// bytearrayRepr formats b the way bytearray.__repr__ does.  It differs from
// bytes.__repr__ in one observable way: a single quote is always escaped, even
// when the doubled-quote form is chosen for the outer quotes.
func bytearrayRepr(b []byte) string {
	return bytesReprWith(b, true)
}

func bytesReprWith(b []byte, escapeAlways bool) string {
	var out bytes.Buffer
	quote := '\''
	if bytes.IndexByte(b, byte('\'')) >= 0 && !(bytes.IndexByte(b, byte('"')) >= 0) {
		quote = '"'
	}
	out.WriteRune('b')
	out.WriteRune(quote)
	for _, c := range b {
		switch {
		case c < 0x20:
			switch c {
			case '\t':
				out.WriteString(`\t`)
			case '\n':
				out.WriteString(`\n`)
			case '\r':
				out.WriteString(`\r`)
			default:
				fmt.Fprintf(&out, `\x%02x`, c)
			}
		case c < 0x7F:
			escape := c == '\\' || c == byte(quote)
			if escapeAlways && c == '\'' {
				// bytearray escapes a lone ' even when the outer quotes are
				// " instead of ', which is the one observable difference from
				// bytes.__repr__.
				escape = true
			}
			if escape {
				out.WriteRune('\\')
			}
			out.WriteByte(c)
		default:
			fmt.Fprintf(&out, "\\x%02x", c)
		}
	}
	out.WriteRune(quote)
	return out.String()
}

// Convert an Object to an Bytes
//
// Returns ok as to whether the conversion worked or not
func convertToBytes(other Object) (Bytes, bool) {
	switch b := other.(type) {
	case Bytes:
		return b, true
	case *ByteArray:
		return Bytes(b.b), true
	}
	return []byte(nil), false
}

// bytesLikeData returns the raw bytes behind any bytes-like object: Bytes and
// ByteArray.  It is what every method that accepts "bytes or bytearray" uses,
// so the two types share one implementation.
func bytesLikeData(other Object) ([]byte, bool) {
	switch b := other.(type) {
	case Bytes:
		return b, true
	case *ByteArray:
		return b.b, true
	}
	return nil, false
}

// Rich comparison

func (a Bytes) M__lt__(other Object) (Object, error) {
	if b, ok := convertToBytes(other); ok {
		return NewBool(bytes.Compare(a, b) < 0), nil
	}
	return NotImplemented, nil
}

func (a Bytes) M__le__(other Object) (Object, error) {
	if b, ok := convertToBytes(other); ok {
		return NewBool(bytes.Compare(a, b) <= 0), nil
	}
	return NotImplemented, nil
}

func (a Bytes) M__eq__(other Object) (Object, error) {
	if b, ok := convertToBytes(other); ok {
		return NewBool(bytes.Equal(a, b)), nil
	}
	return NotImplemented, nil
}

func (a Bytes) M__ne__(other Object) (Object, error) {
	if b, ok := convertToBytes(other); ok {
		return NewBool(!bytes.Equal(a, b)), nil
	}
	return NotImplemented, nil
}

func (a Bytes) M__gt__(other Object) (Object, error) {
	if b, ok := convertToBytes(other); ok {
		return NewBool(bytes.Compare(a, b) > 0), nil
	}
	return NotImplemented, nil
}

func (a Bytes) M__ge__(other Object) (Object, error) {
	if b, ok := convertToBytes(other); ok {
		return NewBool(bytes.Compare(a, b) >= 0), nil
	}
	return NotImplemented, nil
}

func (a Bytes) M__add__(other Object) (Object, error) {
	if b, ok := convertToBytes(other); ok {
		o := make([]byte, len(a)+len(b))
		copy(o[:len(a)], a)
		copy(o[len(a):], b)
		return Bytes(o), nil
	}
	return NotImplemented, nil
}

func (a Bytes) M__iadd__(other Object) (Object, error) {
	if b, ok := convertToBytes(other); ok {
		a = append(a, b...)
		return a, nil
	}
	return NotImplemented, nil
}

func (a Bytes) M__mul__(other Object) (Object, error) {
	n, err := bytesRepeatCount(other)
	if err != nil {
		return nil, err
	}
	return Bytes(bytesRepeat(a, n)), nil
}

func (a Bytes) M__rmul__(other Object) (Object, error) {
	return a.M__mul__(other)
}

func (a Bytes) M__contains__(item Object) (Object, error) {
	return bytesContainsItem(a, item)
}

// Shared implementation helpers for bytes and bytearray

// bytesCompare is bytes.Compare over two plain slices.
func bytesCompare(a, b []byte) int {
	return bytes.Compare(a, b)
}

// bytesEqual is bytes.Equal over two plain slices.
func bytesEqual(a, b []byte) bool {
	return bytes.Equal(a, b)
}

// bytesRepeatCount turns a repetition operand into an int, raising CPython's
// message for a non-integer.  A negative count yields 0 (the slice is empty).
func bytesRepeatCount(other Object) (int, error) {
	switch v := other.(type) {
	case Bool:
		if v {
			return 1, nil
		}
		return 0, nil
	}
	n, err := IndexInt(other)
	if err != nil {
		return 0, ExceptionNewf(TypeError, "can't multiply sequence by non-int of type '%s'", other.Type().Name)
	}
	if n < 0 {
		n = 0
	}
	return n, nil
}

// bytesRepeat returns b repeated n times as a fresh slice.
func bytesRepeat(b []byte, n int) []byte {
	if n <= 0 || len(b) == 0 {
		return []byte{}
	}
	out := make([]byte, 0, len(b)*n)
	for i := 0; i < n; i++ {
		out = append(out, b...)
	}
	return out
}

// bytesIntArg converts an argument that must be an integer, raising CPython's
// argument-clinic wording rather than the generic index message.
func bytesIntArg(o Object) (int, error) {
	i, err := IndexInt(o)
	if err != nil {
		if IsException(TypeError, err) {
			return 0, ExceptionNewf(TypeError, "'%s' object cannot be interpreted as an integer", o.Type().Name)
		}
		return 0, err
	}
	return i, nil
}

// bytesIndexItem converts an iterable item or a single subscript value into a
// byte, raising the ValueError CPython raises for an out of range int.
func bytesIndexItem(item Object) (byte, error) {
	// An int/bool is the byte value itself.
	switch v := item.(type) {
	case Bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case Int, *BigInt:
		i, err := bytesIntArg(item)
		if err != nil {
			return 0, err
		}
		if i < 0 || i > 255 {
			return 0, ExceptionNewf(ValueError, "byte must be in range(0, 256)")
		}
		return byte(i), nil
	}
	// Any other object must itself be an integer.
	i, err := bytesIntArg(item)
	if err != nil {
		return 0, err
	}
	if i < 0 || i > 255 {
		return 0, ExceptionNewf(ValueError, "byte must be in range(0, 256)")
	}
	return byte(i), nil
}

// bytesContainsItem implements "x in b" for b a bytes or bytearray.  An int in
// range is a byte test; any other integer-like object is rejected as an int;
// everything else must be bytes-like and is searched as a subsequence.
func bytesContainsItem(b []byte, item Object) (Object, error) {
	switch item.(type) {
	case Int, Bool, *BigInt:
		c, err := bytesIndexItem(item)
		if err != nil {
			return nil, err
		}
		return NewBool(indexByte(b, c) >= 0), nil
	}
	needle, ok := bytesLikeData(item)
	if !ok {
		return nil, ExceptionNewf(TypeError, "a bytes-like object is required, not '%s'", item.Type().Name)
	}
	return NewBool(bytes.Contains(b, needle)), nil
}

// bytesAssignValue converts the right hand side of a bytearray slice
// assignment into a byte slice.
func bytesAssignValue(value Object) ([]byte, error) {
	if data, ok := bytesLikeData(value); ok {
		return data, nil
	}
	if value == None {
		return nil, ExceptionNewf(TypeError, "cannot convert 'NoneType' object to bytearray")
	}
	// A str is rejected wholesale rather than iterated, matching the message
	// CPython gives for any non-bytes, non-iterable-of-ints value.
	if _, ok := value.(String); ok {
		return nil, ExceptionNewf(TypeError, "can assign only bytes, buffers, or iterables of ints in range(0, 256)")
	}
	// An iterable is read as a sequence of ints; a dict reaches here too and
	// fails on its keys, exactly as CPython does.
	if objectIsIterable(value) {
		return byteArrayFromIterable(value)
	}
	return nil, ExceptionNewf(TypeError, "can assign only bytes, buffers, or iterables of ints in range(0, 256)")
}

// bytesNeedle resolves the "sub" argument of find/count/index: either a single
// int in range, or a bytes-like subsequence.
func bytesNeedle(sub Object) ([]byte, error) {
	if _, ok := bytesLikeData(sub); ok {
		data, _ := bytesLikeData(sub)
		if len(data) == 0 {
			return data, nil
		}
		return data, nil
	}
	switch sub.(type) {
	case Int, *BigInt, Bool:
		c, err := bytesIndexItem(sub)
		if err != nil {
			return nil, err
		}
		return []byte{c}, nil
	}
	// Anything else that is not bytes-like is rejected, naming its type the
	// way CPython's argument clinic does.
	return nil, ExceptionNewf(TypeError, "argument should be integer or bytes-like object, not '%s'", sub.Type().Name)
}

// bytesClampBounds normalises a start/end pair the way CPython's slice
// handling does for the bytes search methods: a negative bound counts from the
// end and saturates at 0, an end past the length saturates at the length, but a
// start past the length is left alone so that the caller can report "not found".
func bytesClampBounds(length int, start, end Object) (int, int, error) {
	s := 0
	e := length
	if start != nil && start != None {
		var err error
		s, err = IndexInt(start)
		if err != nil {
			return 0, 0, err
		}
		if s < 0 {
			s += length
			if s < 0 {
				s = 0
			}
		}
	}
	if end != nil && end != None {
		var err error
		e, err = IndexInt(end)
		if err != nil {
			return 0, 0, err
		}
		if e < 0 {
			e += length
			if e < 0 {
				e = 0
			}
		} else if e > length {
			e = length
		}
	}
	return s, e, nil
}

// bytesFindIn finds the first (or, when reverse is set, last) occurrence of
// needle in b[start:end] and returns its offset, or -1.
//
// An empty needle is found at every position from start to end inclusive, so
// the returned index can be start, start+1, ..., end; a needle longer than the
// window cannot match.
func bytesFindIn(b, needle []byte, start, end int, reverse bool) int {
	if start > end {
		return -1
	}
	if len(needle) == 0 {
		if reverse {
			return end
		}
		return start
	}
	if len(needle) > end-start {
		return -1
	}
	// The window may lie past the end of b when start was beyond the length,
	// so the found offset has to be checked before it is translated back; a
	// bare -1 + start would report a bogus hit.
	if reverse {
		i := bytes.LastIndex(b[start:end], needle)
		if i < 0 {
			return -1
		}
		return i + start
	}
	i := bytes.Index(b[start:end], needle)
	if i < 0 {
		return -1
	}
	return i + start
}

// bytesCountIn counts non-overlapping occurrences of needle in b[start:end].
func bytesCountIn(b, needle []byte, start, end int) int {
	if start > end {
		return 0
	}
	if len(needle) == 0 {
		return end - start + 1
	}
	return bytes.Count(b[start:end], needle)
}

// bytesParseSubAndBounds parses the optional start/end arguments shared by
// count/find/index/rfind/startswith/endswith.  A nil or None bound means the
// default; the caller supplies defaultStart/defaultEnd.
func bytesParseSubAndBounds(args Tuple, name string, nargs int) (start, end Object, err error) {
	if nargs < 1 {
		return nil, nil, ExceptionNewf(TypeError, "%s expected at least 1 argument, got %d", name, nargs)
	}
	if nargs > 3 {
		return nil, nil, ExceptionNewf(TypeError, "%s expected at most 3 arguments, got %d", name, nargs)
	}
	if nargs >= 2 {
		start = args[1]
	}
	if nargs >= 3 {
		end = args[2]
	}
	return start, end, nil
}

// Method bodies.  Each takes the raw subject slice so that bytes and
// bytearray share one implementation; wantArray selects the bytearray flavour
// of the result (and of the error messages).

func bytesDecodeMethod(b []byte, args Tuple, kwargs StringDict) (Object, error) {
	var (
		encoding Object = String("utf-8")
		errors   Object = String("strict")
	)
	if err := ParseTupleAndKeywords(args, kwargs, "|OO:decode", []string{"encoding", "errors"}, &encoding, &errors); err != nil {
		return nil, err
	}
	enc, ok := encoding.(String)
	if !ok {
		return nil, ExceptionNewf(TypeError, "decode() argument 'encoding' must be str, not %s", encoding.Type().Name)
	}
	errs, ok := errors.(String)
	if !ok {
		return nil, ExceptionNewf(TypeError, "decode() argument 'errors' must be str, not %s", errors.Type().Name)
	}
	return decodeBytesWith(string(enc), b, string(errs))
}

func bytesHexMethod(b []byte, args Tuple, kwargs StringDict) (Object, error) {
	var (
		sep    Object
		perSep Object = Int(1)
		kwlist        = []string{"sep", "bytes_per_sep"}
	)
	if err := ParseTupleAndKeywords(args, kwargs, "|OO:hex", kwlist, &sep, &perSep); err != nil {
		return nil, err
	}
	digits := "0123456789abcdef"
	sepStr := ""
	if sep != nil && sep != None {
		sepBytes, ok := bytesLikeData(sep)
		if ok {
			sepStr = string(sepBytes)
		} else if s, ok := sep.(String); ok {
			sepStr = string(s)
		} else {
			return nil, ExceptionNewf(TypeError, "object of type '%s' has no len()", sep.Type().Name)
		}
		if len(sepStr) != 1 {
			return nil, ExceptionNewf(ValueError, "sep must be length 1.")
		}
	}
	per := 1
	if perSep != nil && perSep != None {
		var err error
		per, err = IndexInt(perSep)
		if err != nil {
			return nil, ExceptionNewf(TypeError, "'%s' object cannot be interpreted as an integer", perSep.Type().Name)
		}
	}
	if sepStr == "" || per == 0 {
		var out strings.Builder
		for _, c := range b {
			out.WriteByte(digits[c>>4])
			out.WriteByte(digits[c&0xf])
		}
		return String(out.String()), nil
	}
	abs := per
	if abs < 0 {
		abs = -abs
	}
	var out strings.Builder
	for i, c := range b {
		if i > 0 {
			if per > 0 && (len(b)-i)%per == 0 {
				out.WriteString(sepStr)
			} else if per < 0 && i%abs == 0 {
				out.WriteString(sepStr)
			}
		}
		out.WriteByte(digits[c>>4])
		out.WriteByte(digits[c&0xf])
	}
	return String(out.String()), nil
}

func bytesCountMethod(b []byte, args Tuple, kwargs StringDict) (Object, error) {
	if kwargs.Len() != 0 {
		return nil, ExceptionNewf(TypeError, "count() takes no keyword arguments")
	}
	start, end, err := bytesParseSubAndBounds(args, "count", len(args))
	if err != nil {
		return nil, err
	}
	needle, err := bytesNeedle(args[0])
	if err != nil {
		return nil, err
	}
	s, e, err := bytesClampBounds(len(b), start, end)
	if err != nil {
		return nil, err
	}
	return Int(bytesCountIn(b, needle, s, e)), nil
}

func bytesFindMethod(b []byte, args Tuple, kwargs StringDict, name string, reverse, raise bool) (Object, error) {
	if kwargs.Len() != 0 {
		return nil, ExceptionNewf(TypeError, "%s() takes no keyword arguments", name)
	}
	start, end, err := bytesParseSubAndBounds(args, name, len(args))
	if err != nil {
		return nil, err
	}
	needle, err := bytesNeedle(args[0])
	if err != nil {
		return nil, err
	}
	s, e, err := bytesClampBounds(len(b), start, end)
	if err != nil {
		return nil, err
	}
	i := bytesFindIn(b, needle, s, e, reverse)
	if i < 0 && raise {
		return nil, ExceptionNewf(ValueError, "subsection not found")
	}
	return Int(i), nil
}

// bytesSplitMethod implements split and rsplit.
//
// With sep == None the subject is split on ASCII whitespace, runs of which
// are one separator and leading/trailing runs produce no empty fields; with a
// sep each occurrence delimits a field, and a maxsplit of 0 means "no splits".
func bytesSplitMethod(b []byte, wantArray bool, args Tuple, kwargs StringDict, reverse bool) (Object, error) {
	name := "split"
	if reverse {
		name = "rsplit"
	}
	if len(args) > 2 {
		return nil, ExceptionNewf(TypeError, "%s() takes at most 2 arguments (%d given)", name, len(args))
	}
	var (
		sep Object
		max Object = Int(-1)
	)
	kwlist := []string{"sep", "maxsplit"}
	if err := ParseTupleAndKeywords(args, kwargs, "|OO:"+name, kwlist, &sep, &max); err != nil {
		return nil, err
	}
	maxSplit := -1
	if max != nil && max != None {
		var err error
		maxSplit, err = IndexInt(max)
		if err != nil {
			return nil, err
		}
	}

	mk := func(p []byte) Object {
		if wantArray {
			return &ByteArray{b: append([]byte(nil), p...)}
		}
		return Bytes(append([]byte(nil), p...))
	}

	fields := [][]byte{}

	if sep == nil || sep == None {
		// Whitespace split.
		isWS := func(c byte) bool {
			return c == ' ' || (c >= 0x09 && c <= 0x0d)
		}
		if reverse {
			i := len(b)
			for i > 0 && isWS(b[i-1]) {
				i--
			}
			count := 0
			for i > 0 {
				if maxSplit >= 0 && count >= maxSplit {
					break
				}
				j := i
				for j > 0 && !isWS(b[j-1]) {
					j--
				}
				fields = append(fields, b[j:i])
				count++
				for j > 0 && isWS(b[j-1]) {
					j--
				}
				i = j
			}
			if i > 0 {
				fields = append(fields, b[:i])
			}
			for l, r := 0, len(fields)-1; l < r; l, r = l+1, r-1 {
				fields[l], fields[r] = fields[r], fields[l]
			}
		} else {
			i := 0
			for i < len(b) && isWS(b[i]) {
				i++
			}
			count := 0
			for i < len(b) {
				if maxSplit >= 0 && count >= maxSplit {
					break
				}
				j := i
				for j < len(b) && !isWS(b[j]) {
					j++
				}
				fields = append(fields, b[i:j])
				count++
				for j < len(b) && isWS(b[j]) {
					j++
				}
				i = j
			}
			if i < len(b) {
				fields = append(fields, b[i:])
			}
		}
	} else {
		sepBytes, ok := bytesLikeData(sep)
		if !ok {
			return nil, ExceptionNewf(TypeError, "a bytes-like object is required, not '%s'", sep.Type().Name)
		}
		if len(sepBytes) == 0 {
			return nil, ExceptionNewf(ValueError, "empty separator")
		}
		if maxSplit < 0 {
			maxSplit = -1
		}
		if !reverse {
			start := 0
			count := 0
			for {
				if maxSplit >= 0 && count >= maxSplit {
					break
				}
				i := bytes.Index(b[start:], sepBytes)
				if i < 0 {
					break
				}
				fields = append(fields, b[start:start+i])
				start += i + len(sepBytes)
				count++
			}
			fields = append(fields, b[start:])
		} else {
			end := len(b)
			count := 0
			for {
				if maxSplit >= 0 && count >= maxSplit {
					break
				}
				i := bytes.LastIndex(b[:end], sepBytes)
				if i < 0 {
					break
				}
				fields = append(fields, b[i+len(sepBytes):end])
				end = i
				count++
			}
			fields = append(fields, b[:end])
			for l, r := 0, len(fields)-1; l < r; l, r = l+1, r-1 {
				fields[l], fields[r] = fields[r], fields[l]
			}
		}
	}

	out := make([]Object, len(fields))
	for i, f := range fields {
		out[i] = mk(f)
	}
	return NewListFromItems(out), nil
}

func bytesJoinMethod(sep []byte, wantArray bool, args Tuple) (Object, error) {
	flavour := "bytes"
	if wantArray {
		flavour = "bytearray"
	}
	if len(args) != 1 {
		return nil, ExceptionNewf(TypeError, "%s.join() takes exactly one argument (%d given)", flavour, len(args))
	}
	items, err := SequenceList(args[0])
	if err != nil {
		return nil, err
	}
	var out []byte
	for i, item := range items.Items {
		data, ok := bytesLikeData(item)
		if !ok {
			return nil, ExceptionNewf(TypeError, "sequence item %d: expected a bytes-like object, %s found", i, item.Type().Name)
		}
		if i > 0 {
			out = append(out, sep...)
		}
		out = append(out, data...)
	}
	return bytesWrap(out, wantArray), nil
}

func bytesReplaceMethod(b []byte, wantArray bool, args Tuple) (Object, error) {
	if len(args) < 2 {
		return nil, ExceptionNewf(TypeError, "replace expected at least 2 arguments, got %d", len(args))
	}
	if len(args) > 3 {
		return nil, ExceptionNewf(TypeError, "replace expected at most 3 arguments, got %d", len(args))
	}
	old, err := bytesNeedleStrict(args[0])
	if err != nil {
		return nil, err
	}
	newB, err := bytesNeedleStrict(args[1])
	if err != nil {
		return nil, err
	}
	count := -1
	if len(args) == 3 {
		count, err = IndexInt(args[2])
		if err != nil {
			return nil, err
		}
	}
	out := bytesReplace(b, old, newB, count)
	if wantArray {
		return &ByteArray{b: out}, nil
	}
	return Bytes(out), nil
}

// bytesNeedleStrict converts an argument that must be bytes-like (not an int),
// as replace, strip and the split-family separators require.
func bytesNeedleStrict(item Object) ([]byte, error) {
	data, ok := bytesLikeData(item)
	if !ok {
		return nil, ExceptionNewf(TypeError, "a bytes-like object is required, not '%s'", item.Type().Name)
	}
	return data, nil
}

// bytesMakeTransMethod implements bytes.maketrans(from, to): a 256 byte table
// mapping each byte of from onto the byte at the same offset of to.
func bytesMakeTransMethod(self Object, args Tuple, kwargs StringDict) (Object, error) {
	if kwargs.Len() != 0 {
		return nil, ExceptionNewf(TypeError, "%s.maketrans() takes no keyword arguments", self.Type().Name)
	}
	if len(args) != 2 {
		return nil, ExceptionNewf(TypeError, "maketrans expected 2 arguments, got %d", len(args))
	}
	from, err := bytesNeedleStrict(args[0])
	if err != nil {
		return nil, err
	}
	to, err := bytesNeedleStrict(args[1])
	if err != nil {
		return nil, err
	}
	if len(from) != len(to) {
		return nil, ExceptionNewf(ValueError, "maketrans arguments must have same length")
	}
	table := make([]byte, 256)
	for i := range table {
		table[i] = byte(i)
	}
	for i, c := range from {
		table[c] = to[i]
	}
	return Bytes(table), nil
}

// bytesReplace is bytes.Replace but with CPython's handling of a count of 0
// (no replacements) and of an empty old (insert between every byte).
func bytesReplace(b, old, newB []byte, count int) []byte {
	if count == 0 {
		return append([]byte(nil), b...)
	}
	if len(old) == 0 {
		var out []byte
		done := 0
		out = append(out, newB...)
		done++
		for i := 0; i < len(b); i++ {
			out = append(out, b[i])
			if count < 0 || done < count {
				out = append(out, newB...)
				done++
			}
		}
		return out
	}
	return bytes.Replace(b, old, newB, count)
}

func bytesStripMethod(b []byte, wantArray bool, args Tuple, kwargs StringDict, left, right bool) (Object, error) {
	if len(args) > 1 {
		return nil, ExceptionNewf(TypeError, "strip expected at most 1 argument, got %d", len(args))
	}
	if kwargs.Len() != 0 {
		return nil, ExceptionNewf(TypeError, "strip() takes no keyword arguments")
	}
	var cut func(byte) bool
	if len(args) == 0 || args[0] == None {
		cut = func(c byte) bool { return c == ' ' || (c >= 0x09 && c <= 0x0d) }
	} else {
		chars, ok := bytesLikeData(args[0])
		if !ok {
			return nil, ExceptionNewf(TypeError, "a bytes-like object is required, not '%s'", args[0].Type().Name)
		}
		set := [256]bool{}
		for _, c := range chars {
			set[c] = true
		}
		cut = func(c byte) bool { return set[c] }
	}
	start, end := 0, len(b)
	if left {
		for start < end && cut(b[start]) {
			start++
		}
	}
	if right {
		for end > start && cut(b[end-1]) {
			end--
		}
	}
	out := append([]byte(nil), b[start:end]...)
	if wantArray {
		return &ByteArray{b: out}, nil
	}
	return Bytes(out), nil
}

func bytesCaseMethod(b []byte, wantArray bool, args Tuple, upper bool) Object {
	out := make([]byte, len(b))
	for i, c := range b {
		if upper {
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
		} else {
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
		}
		out[i] = c
	}
	if wantArray {
		return &ByteArray{b: out}
	}
	return Bytes(out)
}

func bytesStartEndMethod(b []byte, args Tuple, kwargs StringDict, isEnd bool) (Object, error) {
	name := "startswith"
	if isEnd {
		name = "endswith"
	}
	if kwargs.Len() != 0 {
		return nil, ExceptionNewf(TypeError, "%s() takes no keyword arguments", name)
	}
	start, end, err := bytesParseSubAndBounds(args, name, len(args))
	if err != nil {
		return nil, err
	}

	prefixes := [][]byte{}
	switch p := args[0].(type) {
	case Tuple:
		for _, item := range p {
			data, ok := bytesLikeData(item)
			if !ok {
				return nil, ExceptionNewf(TypeError, "a bytes-like object is required, not '%s'", item.Type().Name)
			}
			prefixes = append(prefixes, data)
		}
		if len(p) == 0 {
			return False, nil
		}
	default:
		data, ok := bytesLikeData(p)
		if !ok {
			return nil, ExceptionNewf(TypeError, "%s first arg must be bytes or a tuple of bytes, not %s", name, p.Type().Name)
		}
		prefixes = append(prefixes, data)
	}

	s, e, err := bytesClampBounds(len(b), start, end)
	if err != nil {
		return nil, err
	}
	if s > e {
		return False, nil
	}
	window := b[s:e]
	for _, pre := range prefixes {
		if isEnd {
			if len(pre) <= len(window) && bytes.Equal(window[len(window)-len(pre):], pre) {
				return True, nil
			}
		} else {
			if len(pre) <= len(window) && bytes.Equal(window[:len(pre)], pre) {
				return True, nil
			}
		}
	}
	return False, nil
}

func bytesZfillMethod(b []byte, wantArray bool, args Tuple, kwargs StringDict) (Object, error) {
	if kwargs.Len() != 0 {
		return nil, ExceptionNewf(TypeError, "zfill() takes no keyword arguments")
	}
	if len(args) != 1 {
		flavour := "bytes"
		if wantArray {
			flavour = "bytearray"
		}
		return nil, ExceptionNewf(TypeError, "%s.zfill() takes exactly one argument (%d given)", flavour, len(args))
	}
	width, err := IndexInt(args[0])
	if err != nil {
		return nil, ExceptionNewf(TypeError, "'%s' object cannot be interpreted as an integer", args[0].Type().Name)
	}
	out := append([]byte(nil), b...)
	if width <= len(b) {
		return bytesWrap(out, wantArray), nil
	}
	n := width - len(b)
	sign := []byte{}
	body := out
	if len(b) > 0 && (b[0] == '+' || b[0] == '-') {
		sign = b[:1]
		body = b[1:]
	}
	padded := make([]byte, 0, width)
	padded = append(padded, sign...)
	for i := 0; i < n; i++ {
		padded = append(padded, '0')
	}
	padded = append(padded, body...)
	return bytesWrap(padded, wantArray), nil
}

const (
	justifyLeft = iota
	justifyCenter
	justifyRight
)

func bytesJustifyMethod(b []byte, wantArray bool, args Tuple, kwargs StringDict, mode int) (Object, error) {
	name := "ljust"
	if mode == justifyCenter {
		name = "center"
	} else if mode == justifyRight {
		name = "rjust"
	}
	if kwargs.Len() != 0 {
		return nil, ExceptionNewf(TypeError, "%s() takes no keyword arguments", name)
	}
	if len(args) < 1 {
		return nil, ExceptionNewf(TypeError, "%s expected at least 1 argument, got 0", name)
	}
	if len(args) > 2 {
		return nil, ExceptionNewf(TypeError, "%s expected at most 2 arguments, got %d", name, len(args))
	}
	width, err := IndexInt(args[0])
	if err != nil {
		return nil, ExceptionNewf(TypeError, "'%s' object cannot be interpreted as an integer", args[0].Type().Name)
	}
	fill := byte(' ')
	if len(args) == 2 {
		fillBytes, ok := bytesLikeData(args[1])
		if !ok {
			return nil, ExceptionNewf(TypeError, "%s(): argument 2 must be a byte string of length 1, not %s", name, args[1].Type().Name)
		}
		if len(fillBytes) != 1 {
			return nil, ExceptionNewf(TypeError, "%s(): argument 2 must be a byte string of length 1, not a bytes object of length %d", name, len(fillBytes))
		}
		fill = fillBytes[0]
	}
	if width <= len(b) {
		return bytesWrap(append([]byte(nil), b...), wantArray), nil
	}
	pad := width - len(b)
	// ljust pads on the right, rjust on the left, and center uses CPython's
	// asymmetry rule: the extra padding byte, when the margin is odd, goes on
	// the left only if both the margin and the width are odd.
	left, right := 0, 0
	switch mode {
	case justifyLeft:
		right = pad
	case justifyRight:
		left = pad
	case justifyCenter:
		left = pad/2 + (pad & width & 1)
		right = pad - left
	}
	out := make([]byte, width)
	for i := 0; i < left; i++ {
		out[i] = fill
	}
	copy(out[left:], b)
	for i := width - right; i < width; i++ {
		out[i] = fill
	}
	return bytesWrap(out, wantArray), nil
}

// bytesWrap returns out as a bytes or bytearray.
func bytesWrap(out []byte, wantArray bool) Object {
	if wantArray {
		return &ByteArray{b: out}
	}
	return Bytes(out)
}

func bytesTranslateMethod(b []byte, wantArray bool, args Tuple) (Object, error) {
	if len(args) < 1 {
		return nil, ExceptionNewf(TypeError, "translate() takes at least 1 positional argument (0 given)")
	}
	if len(args) > 2 {
		return nil, ExceptionNewf(TypeError, "translate() takes at most 2 arguments (%d given)", len(args))
	}
	var table []byte
	if args[0] != None {
		t, ok := bytesLikeData(args[0])
		if !ok {
			return nil, ExceptionNewf(TypeError, "a bytes-like object is required, not '%s'", args[0].Type().Name)
		}
		if len(t) != 256 {
			return nil, ExceptionNewf(ValueError, "translation table must be 256 characters long")
		}
		table = t
	}
	var del [256]bool
	if len(args) == 2 {
		d, ok := bytesLikeData(args[1])
		if !ok {
			return nil, ExceptionNewf(TypeError, "a bytes-like object is required, not '%s'", args[1].Type().Name)
		}
		for _, c := range d {
			del[c] = true
		}
	}
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if del[c] {
			continue
		}
		if table != nil {
			out = append(out, table[c])
		} else {
			out = append(out, c)
		}
	}
	return bytesWrap(out, wantArray), nil
}

func bytesPartitionMethod(b []byte, wantArray bool, args Tuple, reverse bool) (Object, error) {
	name := "partition"
	if reverse {
		name = "rpartition"
	}
	if len(args) != 1 {
		flavour := "bytes"
		if wantArray {
			flavour = "bytearray"
		}
		return nil, ExceptionNewf(TypeError, "%s.%s() takes exactly one argument (%d given)", flavour, name, len(args))
	}
	sep, ok := bytesLikeData(args[0])
	if !ok {
		return nil, ExceptionNewf(TypeError, "a bytes-like object is required, not '%s'", args[0].Type().Name)
	}
	if len(sep) == 0 {
		return nil, ExceptionNewf(ValueError, "empty separator")
	}
	i := bytes.Index(b, sep)
	if reverse {
		i = bytes.LastIndex(b, sep)
	}
	if i < 0 {
		if reverse {
			return Tuple{bytesWrap([]byte{}, wantArray), bytesWrap([]byte{}, wantArray), bytesWrap(append([]byte(nil), b...), wantArray)}, nil
		}
		return Tuple{bytesWrap(append([]byte(nil), b...), wantArray), bytesWrap([]byte{}, wantArray), bytesWrap([]byte{}, wantArray)}, nil
	}
	head := append([]byte(nil), b[:i]...)
	tail := append([]byte(nil), b[i+len(sep):]...)
	return Tuple{bytesWrap(head, wantArray), bytesWrap(append([]byte(nil), sep...), wantArray), bytesWrap(tail, wantArray)}, nil
}

// bytesFromHexArg implements the shared fromhex() argument handling.
func bytesFromHexArg(args Tuple, kwargs StringDict, name string) ([]byte, error) {
	var (
		s      Object
		kwlist = []string{"string"}
	)
	if err := ParseTupleAndKeywords(args, kwargs, "O:"+name, kwlist, &s); err != nil {
		return nil, err
	}
	var raw []byte
	switch v := s.(type) {
	case String:
		raw = []byte(v)
	case Bytes:
		raw = []byte(v)
	case *ByteArray:
		raw = v.b
	default:
		return nil, ExceptionNewf(TypeError, "fromhex() argument must be str or bytes-like, not %s", s.Type().Name)
	}
	out := make([]byte, 0, len(raw)/2)
	hi := -1
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f' {
			continue
		}
		d := hexDigit(c)
		if d < 0 {
			return nil, ExceptionNewf(ValueError, "non-hexadecimal number found in fromhex() arg at position %d", i)
		}
		if hi < 0 {
			hi = d
			continue
		}
		out = append(out, byte(hi<<4|d))
		hi = -1
	}
	if hi >= 0 {
		return nil, ExceptionNewf(ValueError, "fromhex() arg must contain an even number of hexadecimal digits")
	}
	return out, nil
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// encodeStringWith encodes text with the named codec, honouring the error
// handler for the codecs the interpreter knows.
func encodeStringWith(name, text, handler string) ([]byte, error) {
	canonical, err := canonicalEncoding(name)
	if err != nil {
		return nil, err
	}
	switch canonical {
	case "utf-8":
		return []byte(text), nil
	case "latin-1":
		out := make([]byte, 0, len(text))
		for i, r := range text {
			if r > 255 {
				repl, err := encodeError(name, r, i, handler)
				if err != nil {
					return nil, err
				}
				out = append(out, repl...)
				continue
			}
			out = append(out, byte(r))
		}
		return out, nil
	case "ascii":
		out := make([]byte, 0, len(text))
		for i, r := range text {
			if r > 127 {
				repl, err := encodeError(name, r, i, handler)
				if err != nil {
					return nil, err
				}
				out = append(out, repl...)
				continue
			}
			out = append(out, byte(r))
		}
		return out, nil
	case "utf-16", "utf-16-le", "utf-16-be":
		var out []byte
		big := canonical == "utf-16-be"
		for _, r := range text {
			if r < 0x10000 {
				if big {
					out = append(out, byte(r>>8), byte(r))
				} else {
					out = append(out, byte(r), byte(r>>8))
				}
			} else {
				r -= 0x10000
				hi := rune(0xd800 + (r >> 10))
				lo := rune(0xdc00 + (r & 0x3ff))
				for _, h := range []rune{hi, lo} {
					if big {
						out = append(out, byte(h>>8), byte(h))
					} else {
						out = append(out, byte(h), byte(h>>8))
					}
				}
			}
		}
		return out, nil
	case "utf-32":
		var out []byte
		for _, r := range text {
			out = append(out, byte(r), byte(r>>8), byte(r>>16), byte(r>>24))
		}
		return out, nil
	}
	return nil, ExceptionNewf(LookupError, "unknown encoding: %s", name)
}

// encodeError reports a character the target codec cannot represent, honouring
// the error handler the way CPython does.
func encodeError(name string, r rune, pos int, handler string) ([]byte, error) {
	canonical, cerr := canonicalEncoding(name)
	if cerr != nil {
		canonical = name
	}
	switch handler {
	case "ignore":
		return nil, nil
	case "replace":
		return []byte{'?'}, nil
	case "backslashreplace":
		return []byte(fmt.Sprintf(`\x%02x`, r)), nil
	case "xmlcharrefreplace":
		return []byte(fmt.Sprintf("&#%d;", r)), nil
	case "surrogateescape":
		return nil, ExceptionNewf(UnicodeEncodeError, "'%s' codec can't encode character '\\x%x' in position %d: ordinal not in range(128)", canonical, r, pos)
	case "strict":
		if canonical == "ascii" {
			return nil, ExceptionNewf(UnicodeEncodeError, "'ascii' codec can't encode character '\\x%x' in position %d: ordinal not in range(128)", r, pos)
		}
		return nil, ExceptionNewf(UnicodeEncodeError, "'%s' codec can't encode character '\\x%x' in position %d: ordinal not in range(256)", canonical, r, pos)
	}
	return nil, ExceptionNewf(LookupError, "unknown error handler name '%s'", handler)
}

// decodeBytesWith decodes raw with the named codec.
func decodeBytesWith(name string, raw []byte, handler string) (Object, error) {
	canonical, err := canonicalEncoding(name)
	if err != nil {
		return nil, err
	}
	// The error handler is only consulted when an error is actually hit, so an
	// unknown name is not rejected for an input that decodes cleanly.
	switch canonical {
	case "utf-8":
		if !utf8.Valid(raw) {
			return decodeInvalid(name, raw, handler)
		}
		return String(string(raw)), nil
	case "ascii":
		runes := make([]rune, 0, len(raw))
		for i, c := range raw {
			if c > 127 {
				switch handler {
				case "ignore":
					continue
				case "replace":
					runes = append(runes, '\ufffd')
					continue
				case "backslashreplace":
					runes = append(runes, []rune(fmt.Sprintf(`\x%02x`, c))...)
					continue
				case "surrogateescape":
					runes = append(runes, rune(0xdc00+int(c)))
					continue
				case "strict":
					return nil, ExceptionNewf(UnicodeDecodeError, "'ascii' codec can't decode byte 0x%02x in position %d: ordinal not in range(128)", c, i)
				default:
					return nil, ExceptionNewf(LookupError, "unknown error handler name '%s'", handler)
				}
			}
			runes = append(runes, rune(c))
		}
		return String(string(runes)), nil
	case "latin-1":
		runes := make([]rune, len(raw))
		for i, c := range raw {
			runes[i] = rune(c)
		}
		return String(string(runes)), nil
	case "utf-16", "utf-16-le", "utf-16-be":
		// A leading BOM is consumed only by the byte-order-agnostic codec.
		big := canonical == "utf-16-be"
		if canonical == "utf-16" && len(raw) >= 2 {
			if raw[0] == 0xff && raw[1] == 0xfe {
				big = false
				raw = raw[2:]
			} else if raw[0] == 0xfe && raw[1] == 0xff {
				big = true
				raw = raw[2:]
			}
		}
		runes, err := decodeUTF16(canonical, raw, big, handler)
		if err != nil {
			return nil, err
		}
		return String(string(runes)), nil
	case "utf-32":
		runes, err := decodeUTF32(raw, handler)
		if err != nil {
			return nil, err
		}
		return String(string(runes)), nil
	}
	return nil, ExceptionNewf(LookupError, "unknown encoding: %s", name)
}

// decodeUTF16 decodes a utf-16 byte stream into runes, honouring the handler
// for a dangling byte and for an unpaired surrogate.
func decodeUTF16(codecName string, raw []byte, big bool, handler string) ([]rune, error) {
	unit := func(i int) uint16 {
		if big {
			return uint16(raw[i])<<8 | uint16(raw[i+1])
		}
		return uint16(raw[i]) | uint16(raw[i+1])<<8
	}
	truncated := func(i int) error {
		return ExceptionNewf(UnicodeDecodeError, "'%s' codec can't decode byte 0x%02x in position %d: truncated data", codecName, raw[i], i)
	}
	bad := func(i int) error {
		return ExceptionNewf(UnicodeDecodeError, "'%s' codec can't decode bytes in position %d-%d: unexpected end of data", codecName, i-2, i-1)
	}
	runes := []rune{}
	i := 0
	for i+1 < len(raw) {
		u := unit(i)
		start := i
		i += 2
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 < len(raw) {
				lo := unit(i)
				if lo >= 0xdc00 && lo <= 0xdfff {
					runes = append(runes, rune(0x10000+(uint32(u)-0xd800)<<10+(uint32(lo)-0xdc00)))
					i += 2
					continue
				}
			}
			if handler == "ignore" {
				continue
			}
			if handler == "replace" {
				runes = append(runes, '\ufffd')
				continue
			}
			if handler != "strict" {
				return nil, ExceptionNewf(LookupError, "unknown error handler name '%s'", handler)
			}
			return nil, bad(start)
		}
		if u >= 0xdc00 && u <= 0xdfff {
			if handler == "ignore" {
				continue
			}
			if handler == "replace" {
				runes = append(runes, '\ufffd')
				continue
			}
			if handler != "strict" {
				return nil, ExceptionNewf(LookupError, "unknown error handler name '%s'", handler)
			}
			return nil, bad(start)
		}
		runes = append(runes, rune(u))
	}
	if i < len(raw) {
		switch handler {
		case "strict":
			return nil, truncated(i)
		case "replace":
			runes = append(runes, '\ufffd')
		case "ignore":
			// nothing
		default:
			return nil, ExceptionNewf(LookupError, "unknown error handler name '%s'", handler)
		}
	}
	return runes, nil
}

// decodeUTF32 decodes a little-endian utf-32 byte stream into runes.
func decodeUTF32(raw []byte, handler string) ([]rune, error) {
	runes := []rune{}
	i := 0
	for i+3 < len(raw) {
		r := rune(uint32(raw[i]) | uint32(raw[i+1])<<8 | uint32(raw[i+2])<<16 | uint32(raw[i+3])<<24)
		start := i
		i += 4
		if r > 0x10ffff || (r >= 0xd800 && r <= 0xdfff) {
			if handler == "ignore" {
				continue
			}
			if handler == "replace" {
				runes = append(runes, '\ufffd')
				continue
			}
			return nil, ExceptionNewf(UnicodeDecodeError, "'utf-32-le' codec can't decode bytes in position %d-%d: code point not in range(0x110000)", start, start+3)
		}
		runes = append(runes, r)
	}
	if i < len(raw) {
		switch handler {
		case "strict":
			return nil, ExceptionNewf(UnicodeDecodeError, "'utf-32-le' codec can't decode bytes in position %d-%d: truncated data", i, len(raw)-1)
		case "replace":
			runes = append(runes, '\ufffd')
		case "ignore":
			// nothing
		default:
			return nil, ExceptionNewf(LookupError, "unknown error handler name '%s'", handler)
		}
	}
	return runes, nil
}

// decodeInvalid reports an undecodable utf-8 sequence, honouring the error
// handler.
func decodeInvalid(name string, raw []byte, handler string) (Object, error) {
	switch handler {
	case "ignore", "replace", "backslashreplace":
		runes := []rune{}
		for i := 0; i < len(raw); {
			r, size := utf8.DecodeRune(raw[i:])
			if r == utf8.RuneError && size <= 1 {
				switch handler {
				case "replace":
					runes = append(runes, '\ufffd')
				case "backslashreplace":
					runes = append(runes, []rune(fmt.Sprintf(`\x%02x`, raw[i]))...)
				}
				i++
				continue
			}
			runes = append(runes, r)
			i += size
		}
		return String(string(runes)), nil
	case "surrogateescape":
		runes := []rune{}
		for i := 0; i < len(raw); {
			r, size := utf8.DecodeRune(raw[i:])
			if r == utf8.RuneError && size <= 1 {
				runes = append(runes, rune(0xdc00+int(raw[i])))
				i++
				continue
			}
			runes = append(runes, r)
			i += size
		}
		return String(string(runes)), nil
	case "strict":
		for i := 0; i < len(raw); {
			r, size := utf8.DecodeRune(raw[i:])
			if r == utf8.RuneError && size <= 1 {
				return nil, ExceptionNewf(UnicodeDecodeError, "'utf-8' codec can't decode byte 0x%02x in position %d: invalid start byte", raw[i], i)
			}
			i += size
		}
		return String(string(raw)), nil
	}
	return nil, ExceptionNewf(LookupError, "unknown error handler name '%s'", handler)
}

// canonicalEncoding maps the aliases a caller may use onto the canonical name
// of one of the encodings this interpreter implements, mirroring the codecs
// registry.
func canonicalEncoding(name string) (string, error) {
	n := strings.ToLower(strings.ReplaceAll(name, " ", ""))
	n = strings.ReplaceAll(n, "_", "-")
	switch n {
	case "utf-8", "utf8", "u8", "utf", "cp65001":
		return "utf-8", nil
	case "ascii", "us-ascii", "646":
		return "ascii", nil
	case "latin-1", "latin1", "iso-8859-1", "8859", "l1":
		return "latin-1", nil
	case "utf-16", "utf16":
		return "utf-16", nil
	case "utf-16-le", "utf-16le":
		return "utf-16-le", nil
	case "utf-16-be", "utf-16be":
		return "utf-16-be", nil
	case "utf-32", "utf32":
		return "utf-32", nil
	}
	return "", ExceptionNewf(LookupError, "unknown encoding: %s", name)
}

// Check interface is satisfied
var (
	_ richComparison = (Bytes)(nil)
	_ I__add__       = (Bytes)(nil)
	_ I__iadd__      = (Bytes)(nil)
	_ I__mul__       = (Bytes)(nil)
	_ I__rmul__      = (Bytes)(nil)
	_ I__len__       = (Bytes)(nil)
	_ I__contains__  = (Bytes)(nil)
)

// M__len__ implements len(b), which was missing: bytes had no __len__ at all, so
// len(b"abc") raised "object of type 'bytes' has no len()".
func (a Bytes) M__len__() (Object, error) {
	return Int(len(a)), nil
}

// Replace is kept because the parser's literal-folding path and older callers
// use it directly.
func (a Bytes) Replace(args Tuple) (Object, error) {
	return bytesReplaceMethod(a, false, args)
}

func init() {
	BytesType.Dict.Set("__len__", MustNewMethod("__len__", func(self Object, args Tuple) (Object, error) {
		return self.(Bytes).M__len__()
	}, 0, "Return the number of bytes in the sequence."))

	// bytes.__getitem__: b[i] is the int at i, b[a:b] is a new bytes object.
	// Without this, "b'abc'[0]" raised "'bytes' object is not subscriptable",
	// which urllib3 hits in _encode_invalid_chars.
	BytesType.Dict.Set("__getitem__", MustNewMethod("__getitem__", func(self Object, args Tuple) (Object, error) {
		b := self.(Bytes)
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "bytes.__getitem__() takes exactly one argument")
		}
		if sl, ok := args[0].(*Slice); ok {
			data, err := byteArraySlice(b, sl)
			if err != nil {
				return nil, err
			}
			return Bytes(data), nil
		}
		i, err := indexForContainer(args[0], len(b), "bytes")
		if err != nil {
			return nil, err
		}
		return Int(b[i]), nil
	}, 0, "Return self[key]."))

	// bytes.__iter__ yields each byte as an int, as CPython does.
	BytesType.Dict.Set("__iter__", MustNewMethod("__iter__", func(self Object, args Tuple) (Object, error) {
		b := self.(Bytes)
		items := make([]Object, len(b))
		for i, c := range b {
			items[i] = Int(c)
		}
		return NewListFromItems(items), nil
	}, 0, "Implement iter(self)."))

	BytesType.Dict.Set("replace", MustNewMethod("replace", func(self Object, args Tuple) (Object, error) {
		return bytesReplaceMethod(self.(Bytes), false, args)
	}, 0, `replace(old, new, count=-1) -> bytes

Return a copy with all occurrences of substring old replaced by new.

  count
    Maximum number of occurrences to replace.
    -1 (the default value) means replace all occurrences.`))

	set := func(name string, fn func(self Object, args Tuple) (Object, error), doc string) {
		BytesType.Dict.Set(name, MustNewMethod(name, fn, 0, doc))
	}
	setKw := func(name string, fn func(self Object, args Tuple, kwargs StringDict) (Object, error), doc string) {
		BytesType.Dict.Set(name, MustNewMethod(name, fn, 0, doc))
	}
	self := func(fn func(b Bytes, args Tuple) (Object, error)) func(self Object, args Tuple) (Object, error) {
		return func(self Object, args Tuple) (Object, error) { return fn(self.(Bytes), args) }
	}
	selfKw := func(fn func(b Bytes, args Tuple, kwargs StringDict) (Object, error)) func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return func(self Object, args Tuple, kwargs StringDict) (Object, error) {
			return fn(self.(Bytes), args, kwargs)
		}
	}

	setKw("decode", selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
		return bytesDecodeMethod(b, args, kwargs)
	}), `decode(encoding='utf-8', errors='strict') -> str

Decode the bytes using the codec registered for encoding.`)

	setKw("hex", selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
		return bytesHexMethod(b, args, kwargs)
	}), `hex([sep[, bytes_per_sep]]) -> str

Return a string containing the hexadecimal representation of the bytes.`)

	setKw("count", selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
		return bytesCountMethod(b, args, kwargs)
	}), `count(sub[, start[, end]]) -> int

Return the number of non-overlapping occurrences of subsequence sub.`)

	findLike := func(name string, reverse, raise bool) {
		setKw(name, selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
			return bytesFindMethod(b, args, kwargs, name, reverse, raise)
		}), name+`(sub[, start[, end]]) -> int

Return the lowest (rfind: highest) index of subsequence sub.`)
	}
	findLike("find", false, false)
	findLike("rfind", true, false)
	findLike("index", false, true)

	setKw("split", selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
		return bytesSplitMethod(b, false, args, kwargs, false)
	}), `split(sep=None, maxsplit=-1) -> list of bytes

Return a list of the sections of the bytes, using sep as the delimiter.`)
	setKw("rsplit", selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
		return bytesSplitMethod(b, false, args, kwargs, true)
	}), `rsplit(sep=None, maxsplit=-1) -> list of bytes

Return a list of the sections of the bytes, starting at the end.`)

	set("join", self(func(b Bytes, args Tuple) (Object, error) {
		return bytesJoinMethod(b, false, args)
	}), "join(iterable_of_bytes) -> bytes.  Concatenate any number of bytes-like objects.")

	strip := func(name string, left, right bool) {
		setKw(name, selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
			return bytesStripMethod(b, false, args, kwargs, left, right)
		}), name+`([chars]) -> bytes

Return a copy with leading/trailing bytes removed.`)
	}
	strip("strip", true, true)
	strip("lstrip", true, false)
	strip("rstrip", false, true)

	set("upper", self(func(b Bytes, args Tuple) (Object, error) {
		if len(args) != 0 {
			return nil, ExceptionNewf(TypeError, "bytes.upper() takes no arguments (%d given)", len(args))
		}
		return bytesCaseMethod(b, false, args, true), nil
	}), "upper() -> bytes.  Return a copy with all ASCII letters uppercased.")
	set("lower", self(func(b Bytes, args Tuple) (Object, error) {
		if len(args) != 0 {
			return nil, ExceptionNewf(TypeError, "bytes.lower() takes no arguments (%d given)", len(args))
		}
		return bytesCaseMethod(b, false, args, false), nil
	}), "lower() -> bytes.  Return a copy with all ASCII letters lowercased.")

	setKw("startswith", selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
		return bytesStartEndMethod(b, args, kwargs, false)
	}), "startswith(prefix[, start[, end]]) -> bool")
	setKw("endswith", selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
		return bytesStartEndMethod(b, args, kwargs, true)
	}), "endswith(suffix[, start[, end]]) -> bool")

	setKw("zfill", selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
		// The keyword check is in the helper; the no-keyword message here is
		// the bytes flavour.
		if kwargs.Len() != 0 {
			return nil, ExceptionNewf(TypeError, "bytes.zfill() takes no keyword arguments")
		}
		return bytesZfillMethod(b, false, args, NewStringDict())
	}), "zfill(width) -> bytes.  Pad a numeric string with zeros on the left.")

	justify := func(name string, mode int) {
		setKw(name, selfKw(func(b Bytes, args Tuple, kwargs StringDict) (Object, error) {
			return bytesJustifyMethod(b, false, args, kwargs, mode)
		}), name+`(width[, fillchar]) -> bytes.  Pad the bytes to width.`)
	}
	justify("center", justifyCenter)
	justify("ljust", justifyLeft)
	justify("rjust", justifyRight)

	set("translate", self(func(b Bytes, args Tuple) (Object, error) {
		return bytesTranslateMethod(b, false, args)
	}), `translate(table, /, delete=b'') -> bytes

Return a copy with each byte mapped through the given translation table.`)

	set("partition", self(func(b Bytes, args Tuple) (Object, error) {
		return bytesPartitionMethod(b, false, args, false)
	}), "partition(sep) -> (head, sep, tail)")
	set("rpartition", self(func(b Bytes, args Tuple) (Object, error) {
		return bytesPartitionMethod(b, false, args, true)
	}), "rpartition(sep) -> (head, sep, tail)")

	// fromhex and maketrans are looked up on the type as well as on an
	// instance; as with dict.fromkeys the arguments arrive unshifted, so self is
	// only used for its type name.
	BytesType.Dict.Set("fromhex", MustNewMethod("fromhex", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		raw, err := bytesFromHexArg(args, kwargs, "bytes.fromhex")
		if err != nil {
			return nil, err
		}
		return Bytes(raw), nil
	}, METH_CLASS, `fromhex(string, /) -> bytes

Create a bytes object from a string of hexadecimal numbers.

Spaces between two numbers are accepted.
Example: bytes.fromhex('B9 01EF') -> b'\xb9\x01\xef'.`))

	BytesType.Dict.Set("maketrans", MustNewMethod("maketrans", bytesMakeTransMethod, 0, `maketrans(frm, to) -> bytes

Return a translation table (a bytes object of length 256) suitable for use in
bytes.translate.`))
}

// indexForContainer resolves a single subscript against a length, raising the
// IndexError/TypeError CPython raises for the named container.
func indexForContainer(key Object, length int, what string) (int, error) {
	switch key.(type) {
	case Int, Bool, *BigInt:
	default:
		if _, ok := key.(I__index__); !ok {
			return 0, ExceptionNewf(TypeError, "%s indices must be integers or slices, not %s", what, key.Type().Name)
		}
	}
	i, err := Index(key)
	if err != nil {
		if IsException(TypeError, err) {
			return 0, ExceptionNewf(TypeError, "%s indices must be integers or slices, not %s", what, key.Type().Name)
		}
		return 0, err
	}
	n := int(i)
	if i < 0 {
		n += length
	}
	if n < 0 || n >= length {
		return 0, ExceptionNewf(IndexError, "%s index out of range", what)
	}
	return n, nil
}
