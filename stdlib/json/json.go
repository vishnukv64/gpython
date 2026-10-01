// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package json provides the implementation of python's 'json' module.
//
// The parsing and formatting are done natively rather than by delegating to
// encoding/json, because the mapping to Python values is not the same as the
// mapping to Go values: a JSON object must become a dict that preserves key
// order and is a real Python dict, an array a list, and - the part that is
// easy to get wrong - a number with no fraction or exponent must become an
// int, not a float.  {"count": 3} must give an int, because code adds to it,
// uses it as a dict key, and prints it without a .0.
package json

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `JSON (JavaScript Object Notation) <https://json.org> is a subset of
JavaScript syntax (ECMA-262 3rd edition) used as a lightweight data
interchange format.

This module exposes an API familiar to users of the standard library
marshal and pickle modules.`

const (
	INFINITY     = "Infinity"
	NEG_INFINITY = "-Infinity"
	NAN          = "NaN"
)

var JSONDecodeErrorType = py.ExceptionType.NewType("json.JSONDecodeError", "JSON decoding error.", nil, nil)

func init() {
	globals := py.StringDict{}
	globals["dumps"] = py.MustNewMethod("dumps", jsonDumps, 0, "Serialize obj to a JSON formatted str.")
	globals["dump"] = py.MustNewMethod("dump", jsonDump, 0, "Serialize obj as a JSON formatted stream to a file.")
	globals["loads"] = py.MustNewMethod("loads", jsonLoads, 0, "Deserialize a str or bytes to a Python object.")
	globals["load"] = py.MustNewMethod("load", jsonLoad, 0, "Deserialize a file to a Python object.")
	globals["JSONDecodeError"] = JSONDecodeErrorType

	// JSONDecodeError exposes msg, doc, pos, lineno and colno as attributes,
	// which is what code catching it reads.  They live in the exception's
	// Dict, set when the error is built.
	msgAttr := func(name string, index int) *py.Property {
		return &py.Property{Fget: func(self py.Object) (py.Object, error) {
			if e, ok := self.(*py.Exception); ok {
				if v, ok := e.Dict[name]; ok {
					return v, nil
				}
				if args, ok := e.Args.(py.Tuple); ok && index < len(args) {
					return args[index], nil
				}
			}
			return py.None, nil
		}}
	}
	JSONDecodeErrorType.Dict["msg"] = msgAttr("msg", 0)
	JSONDecodeErrorType.Dict["doc"] = msgAttr("doc", 1)
	JSONDecodeErrorType.Dict["pos"] = msgAttr("pos", 2)
	JSONDecodeErrorType.Dict["lineno"] = msgAttr("lineno", 3)
	JSONDecodeErrorType.Dict["colno"] = msgAttr("colno", 4)
	globals["JSONEncoder"] = EncoderType
	globals["JSONDecoder"] = DecoderType
	globals["encoder"] = EncoderType
	globals["decoder"] = DecoderType

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "json",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// Encoder holds the formatting options.  It is a real class so that code
// subclassing json.JSONEncoder, which is common, keeps working.
type Encoder struct {
	skipkeys  bool
	indent    string
	hasIndent bool
	sortKeys  bool
	itemSep   string
	keySep    string
	defaultFn py.Object
}

var EncoderType = py.NewTypeX("json.JSONEncoder", "Extensible JSON encoder for Python data structures.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	e := &Encoder{itemSep: ", ", keySep: ": "}
	if err := e.configure(args, kwargs); err != nil {
		return nil, err
	}
	return e, nil
}, nil)

func (e *Encoder) Type() *py.Type { return EncoderType }

// configure reads the encoder's keyword options, shared by the constructor
// and by dumps().
func (e *Encoder) configure(args py.Tuple, kwargs py.StringDict) error {
	var (
		skipkeys py.Object = py.False
		indent   py.Object = py.None
		sortKeys py.Object = py.False
		seps     py.Object = py.None
	)
	for k, v := range kwargs {
		switch k {
		case "skipkeys":
			skipkeys = v
		case "indent":
			indent = v
		case "sort_keys":
			sortKeys = v
		case "separators":
			seps = v
		case "default":
			e.defaultFn = v
		case "ensure_ascii", "check_circular", "allow_nan":
			// Accepted and ignored: this encoder always writes ASCII-safe
			// output, always checks for cycles, and always allows the
			// non-finite values.
		default:
			return py.ExceptionNewf(py.TypeError, "unexpected keyword argument %q", k)
		}
	}
	e.skipkeys = skipkeys == py.True
	e.sortKeys = sortKeys == py.True

	if indent != py.None {
		// CPython: a string indent is used as it is, a number is that many
		// spaces, and None or "" means no newlines.
		switch v := indent.(type) {
		case py.String:
			if v != "" {
				e.indent = string(v)
				e.hasIndent = true
			}
		default:
			n, err := py.IndexInt(indent)
			if err != nil {
				return py.ExceptionNewf(py.TypeError, "indent must be an int or a string")
			}
			if n > 0 {
				e.indent = strings.Repeat(" ", n)
				e.hasIndent = true
			}
		}
	}

	if seps != py.None {
		t, ok := seps.(py.Tuple)
		if !ok || len(t) != 2 {
			return py.ExceptionNewf(py.ValueError, "separators must be a (item_separator, key_separator) tuple")
		}
		is, err := py.StrAsString(t[0])
		if err != nil {
			return err
		}
		ks, err := py.StrAsString(t[1])
		if err != nil {
			return err
		}
		e.itemSep, e.keySep = is, ks
	} else if e.hasIndent {
		// CPython's default changes with indent: no trailing spaces.
		e.itemSep, e.keySep = ",", ": "
	}
	return nil
}

// Encoder method, so that a subclass overriding default() is honoured.
func (e *Encoder) encode(obj py.Object) (py.Object, error) {
	s, err := e.encodeTo(nil, obj, 0, map[uintptr]bool{})
	if err != nil {
		return nil, err
	}
	return py.String(s), nil
}

// encodeTo writes obj, returning the text.
func (e *Encoder) encodeTo(b *strings.Builder, obj py.Object, depth int, seen map[uintptr]bool) (string, error) {
	var out strings.Builder
	if b != nil {
		out = *b
	}
	text, err := e.encodeValue(obj, depth, seen)
	if err != nil {
		return "", err
	}
	out.WriteString(text)
	return out.String(), nil
}

func (e *Encoder) encodeValue(obj py.Object, depth int, seen map[uintptr]bool) (string, error) {
	switch v := obj.(type) {
	case py.NoneType:
		return "null", nil
	case py.Bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case py.Int:
		return strconv.FormatInt(int64(v), 10), nil
	case *py.BigInt:
		return bigIntText(v), nil
	case py.Float:
		return formatFloat(float64(v)), nil
	case py.String:
		return encodeString(string(v)), nil
	}

	// The containers are checked by protocol as well as by type, so a
	// subclass of dict or a collections.OrderedDict serialises too.
	if d, ok := obj.(py.IGetDict); ok {
		return e.encodeDict(obj, d.GetDict(), depth, seen)
	}
	if obj.Type() == py.ListType || obj.Type() == py.TupleType {
		return e.encodeList(obj, depth, seen)
	}
	if items, err := sequenceItems(obj); err == nil && items != nil {
		return e.encodeItems(items, depth, seen)
	}

	// A custom default() may handle it, which is how a decorated class or a
	// datetime is serialised by real code.
	if e.defaultFn != nil {
		converted, err := py.Call(e.defaultFn, py.Tuple{obj}, nil)
		if err != nil {
			return "", err
		}
		return e.encodeValue(converted, depth, seen)
	}

	return "", py.ExceptionNewf(py.TypeError, "Object of type %s is not JSON serializable", obj.Type().Name)
}

func (e *Encoder) encodeDict(obj py.Object, d py.StringDict, depth int, seen map[uintptr]bool) (string, error) {
	id := identity(obj)
	if id != 0 && seen[id] {
		return "", py.ExceptionNewf(py.ValueError, "Circular reference detected")
	}
	if id != 0 {
		seen[id] = true
		defer delete(seen, id)
	}

	// Keys become strings, as JSON requires.  An int key is stringified the
	// way Python does; anything else that is not a string needs skipkeys.
	type pair struct{ key, value string }
	pairs := make([]pair, 0, len(d))

	for encoded, value := range d {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			return "", err
		}
		var keyText string
		switch k := key.(type) {
		case py.String:
			keyText = encodeString(string(k))
		case py.Int:
			// An int key is written as a quoted number.
			keyText = encodeString(strconv.FormatInt(int64(k), 10))
		case py.Bool, py.Float, py.NoneType:
			if text, err := py.ReprAsString(key); err == nil {
				keyText = encodeString(text)
			}
		default:
			if e.skipkeys {
				continue
			}
			return "", py.ExceptionNewf(py.TypeError, "keys must be str, int, float, bool or None, not %s", key.Type().Name)
		}
		valueText, err := e.encodeValue(value, depth+1, seen)
		if err != nil {
			return "", err
		}
		pairs = append(pairs, pair{keyText, valueText})
	}
	if e.sortKeys {
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
	}

	if len(pairs) == 0 {
		return "{}", nil
	}
	if !e.hasIndent {
		parts := make([]string, len(pairs))
		for i, p := range pairs {
			parts[i] = p.key + e.keySep + p.value
		}
		return "{" + strings.Join(parts, e.itemSep) + "}", nil
	}
	inner := e.indent + e.indent
	pad := strings.Repeat(e.indent, depth+1)
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = pad + p.key + e.keySep + p.value
	}
	padEnd := strings.Repeat(e.indent, depth)
	_ = inner
	return "{\n" + strings.Join(parts, e.itemSep+"\n") + "\n" + padEnd + "}", nil
}

func (e *Encoder) encodeList(obj py.Object, depth int, seen map[uintptr]bool) (string, error) {
	id := identity(obj)
	if id != 0 && seen[id] {
		return "", py.ExceptionNewf(py.ValueError, "Circular reference detected")
	}
	if id != 0 {
		seen[id] = true
		defer delete(seen, id)
	}

	if list, ok := obj.(*py.List); ok {
		return e.encodeItems(list.Items, depth, seen)
	}
	if tuple, ok := obj.(py.Tuple); ok {
		return e.encodeItems([]py.Object(tuple), depth, seen)
	}
	return "[]", nil
}

func (e *Encoder) encodeItems(items []py.Object, depth int, seen map[uintptr]bool) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}
	parts := make([]string, len(items))
	for i, item := range items {
		text, err := e.encodeValue(item, depth+1, seen)
		if err != nil {
			return "", err
		}
		parts[i] = text
	}
	if !e.hasIndent {
		return "[" + strings.Join(parts, e.itemSep) + "]", nil
	}
	pad := strings.Repeat(e.indent, depth+1)
	for i := range parts {
		parts[i] = pad + parts[i]
	}
	padEnd := strings.Repeat(e.indent, depth)
	return "[\n" + strings.Join(parts, e.itemSep+"\n") + "\n" + padEnd + "]", nil
}

// sequenceItems returns the items of any other sequence, so that a list
// subclass or a collections.deque serialises as an array.
func sequenceItems(obj py.Object) ([]py.Object, error) {
	iterObj, err := py.Iter(obj)
	if err != nil {
		return nil, err
	}
	items := []py.Object{}
	for {
		item, err := py.Next(iterObj)
		if err != nil {
			if py.IsException(py.StopIteration, err) {
				return items, nil
			}
			return nil, err
		}
		items = append(items, item)
	}
}

// formatFloat matches Python's repr of a float, with the JSON spellings of
// the non-finite values.
func formatFloat(f float64) string {
	switch {
	case f != f:
		return NAN
	case f > 1.7976931348623157e308:
		return INFINITY
	case f < -1.7976931348623157e308:
		return NEG_INFINITY
	}
	text := strconv.FormatFloat(f, 'g', -1, 64)
	// Go writes exponents as 1e+10; Python omits the plus.
	text = strings.Replace(text, "e+", "e+", 1)
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return text
}

// encodeString writes a JSON string, escaping what must be escaped and
// keeping everything else as printable ASCII, as CPython does by default.
func encodeString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else if r < 0x7f {
				b.WriteRune(r)
			} else {
				// Non-ASCII is written as \uXXXX, with surrogate pairs above
				// the BMP, which is what ensure_ascii=True produces.
				if r > 0xFFFF {
					hi, lo := utf16.EncodeRune(r)
					fmt.Fprintf(&b, `\u%04x\u%04x`, hi, lo)
				} else {
					fmt.Fprintf(&b, `\u%04x`, r)
				}
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func jsonDumps(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var obj py.Object
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "dumps() missing required argument 'obj'")
	}
	obj = args[0]

	e := &Encoder{itemSep: ", ", keySep: ": "}
	if err := e.configure(args[1:], kwargs); err != nil {
		return nil, err
	}
	return e.encode(obj)
}

func jsonDump(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "dump() missing required argument 'fp'")
	}
	fp := args[1]
	text, err := jsonDumps(self, py.Tuple{args[0]}, kwargs)
	if err != nil {
		return nil, err
	}
	write, err := py.GetAttrString(fp, "write")
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "fp must have a write() method")
	}
	if _, err := py.Call(write, py.Tuple{text}, nil); err != nil {
		return nil, err
	}
	return py.None, nil
}

func jsonLoads(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "loads() missing required argument 's'")
	}
	s = args[0]

	var text string
	switch v := s.(type) {
	case py.String:
		text = string(v)
	case py.Bytes:
		text = string(v)
	default:
		return nil, py.ExceptionNewf(py.TypeError, "the JSON object must be str, bytes or bytearray, not %s", s.Type().Name)
	}

	// The module-level loads() accepts the same hook keywords as a Decoder,
	// so build a decoder when any were given.
	dec := &Decoder{}
	for k, v := range kwargs {
		switch k {
		case "object_hook":
			dec.objectHook = v
		case "object_pairs_hook":
			dec.objectPairsHook = v
		case "parse_float":
			dec.parseFloat = v
		case "parse_int":
			dec.parseInt = v
		case "parse_constant":
			dec.parseConstant = v
		case "strict", "cls":
			// Accepted: cls is not used to pick a different decoder class.
		default:
			return nil, py.ExceptionNewf(py.TypeError, "unexpected keyword argument %q", k)
		}
	}
	return dec.decodeText(text)
}

func jsonLoad(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "load() missing required argument 'fp'")
	}
	read, err := py.GetAttrString(args[0], "read")
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "fp must have a read() method")
	}
	text, err := py.Call(read, py.Tuple{}, nil)
	if err != nil {
		return nil, err
	}
	return jsonLoads(self, py.Tuple{text}, kwargs)
}

// decoder holds the parsing position and the hooks that transform what is
// produced.
type decoder struct {
	src             string
	pos             int
	objectHook      py.Object
	objectPairsHook py.Object
	parseFloat      py.Object
	parseInt        py.Object
	parseConstant   py.Object
}

// error builds a JSONDecodeError describing the failure, as CPython does.
func (d *decoder) error(msg string) error {
	line, col := 1, 1
	for i := 0; i < d.pos && i < len(d.src); i++ {
		if d.src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	e, err := py.ExceptionNew(JSONDecodeErrorType, py.Tuple{
		py.String(msg), py.String(d.src), py.Int(d.pos),
	}, nil)
	if err != nil {
		return err
	}
	exc, ok := e.(*py.Exception)
	if !ok {
		return err
	}
	if exc.Dict == nil {
		exc.Dict = py.NewStringDict()
	}
	// The Dict keys go through a plain map write, which is fine here: these
	// are string keys and a string key is stored verbatim.
	exc.Dict["msg"] = py.String(msg)
	exc.Dict["doc"] = py.String(d.src)
	exc.Dict["pos"] = py.Int(d.pos)
	exc.Dict["lineno"] = py.Int(line)
	exc.Dict["colno"] = py.Int(col)
	return exc
}

// identity is the address of a container, used to spot a cycle.  It is a
// pointer rather than the object itself because a dict here is a Go map,
// which cannot be a map key.
func identity(obj py.Object) uintptr {
	v := reflect.ValueOf(obj)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice:
		if !v.IsNil() {
			return v.Pointer()
		}
	}
	return 0
}

// bigIntText renders a bigint the way str() would.
func bigIntText(v *py.BigInt) string {
	text, err := py.StrAsString(v)
	if err != nil {
		return "0"
	}
	return text
}

func (d *decoder) skipSpace() {
	for d.pos < len(d.src) {
		switch d.src[d.pos] {
		case ' ', '\t', '\n', '\r':
			d.pos++
		default:
			return
		}
	}
}

func (d *decoder) parse() (py.Object, error) {
	d.skipSpace()
	if d.pos >= len(d.src) {
		return nil, d.error("Expecting value")
	}
	switch c := d.src[d.pos]; {
	case c == '{':
		return d.parseObject()
	case c == '[':
		return d.parseArray()
	case c == '"':
		return d.parseString()
	case c == 't':
		return d.parseLiteral("true", py.True)
	case c == 'f':
		return d.parseLiteral("false", py.False)
	case c == 'n':
		return d.parseLiteral("null", py.None)
	case c == '-' || (c >= '0' && c <= '9'):
		return d.parseNumber()
	default:
		return nil, d.error("Expecting value")
	}
}

func (d *decoder) parseLiteral(word string, value py.Object) (py.Object, error) {
	if strings.HasPrefix(d.src[d.pos:], word) {
		d.pos += len(word)
		return value, nil
	}
	return nil, d.error("Expecting value")
}

func (d *decoder) parseObject() (py.Object, error) {
	d.pos++ // {
	result := py.NewStringDict()
	pairs := py.Tuple{}
	d.skipSpace()
	if d.pos < len(d.src) && d.src[d.pos] == '}' {
		d.pos++
		return result, nil
	}
	for {
		d.skipSpace()
		if d.pos >= len(d.src) || d.src[d.pos] != '"' {
			return nil, d.error("Expecting property name enclosed in double quotes")
		}
		keyObj, err := d.parseString()
		if err != nil {
			return nil, err
		}
		key, _ := py.StrAsString(keyObj)

		d.skipSpace()
		if d.pos >= len(d.src) || d.src[d.pos] != ':' {
			return nil, d.error("Expecting ':' delimiter")
		}
		d.pos++
		value, err := d.parse()
		if err != nil {
			return nil, err
		}
		// The key goes through the dict's own setitem so that its encoding
		// matches what a lookup later produces.
		if _, err := result.M__setitem__(py.String(key), value); err != nil {
			return nil, err
		}
		pairs = append(pairs, py.Tuple{py.String(key), value})

		d.skipSpace()
		if d.pos >= len(d.src) {
			return nil, d.error("Expecting ',' delimiter")
		}
		switch d.src[d.pos] {
		case ',':
			d.pos++
		case '}':
			d.pos++
			return d.applyObjectHooks(result, pairs)
		default:
			return nil, d.error("Expecting ',' delimiter")
		}
	}
}

// applyObjectHooks runs object_pairs_hook or object_hook over a finished
// object, which is how a caller substitutes its own type for a dict.
func (d *decoder) applyObjectHooks(result py.StringDict, pairs py.Tuple) (py.Object, error) {
	if d.objectPairsHook != nil {
		return py.Call(d.objectPairsHook, py.Tuple{pairs}, nil)
	}
	if d.objectHook != nil {
		return py.Call(d.objectHook, py.Tuple{result}, nil)
	}
	return result, nil
}

func (d *decoder) parseArray() (py.Object, error) {
	d.pos++ // [
	items := []py.Object{}
	d.skipSpace()
	if d.pos < len(d.src) && d.src[d.pos] == ']' {
		d.pos++
		return py.NewListFromItems(items), nil
	}
	for {
		value, err := d.parse()
		if err != nil {
			return nil, err
		}
		items = append(items, value)
		d.skipSpace()
		if d.pos >= len(d.src) {
			return nil, d.error("Expecting ',' delimiter")
		}
		switch d.src[d.pos] {
		case ',':
			d.pos++
		case ']':
			d.pos++
			return py.NewListFromItems(items), nil
		default:
			return nil, d.error("Expecting ',' delimiter")
		}
	}
}

func (d *decoder) parseString() (py.Object, error) {
	d.pos++ // "
	var b strings.Builder
	for d.pos < len(d.src) {
		c := d.src[d.pos]
		switch {
		case c == '"':
			d.pos++
			return py.String(b.String()), nil
		case c == '\\':
			d.pos++
			if d.pos >= len(d.src) {
				return nil, d.error("Unterminated string starting at")
			}
			switch d.src[d.pos] {
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			case '/':
				b.WriteByte('/')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				r, err := d.parseUnicodeEscape()
				if err != nil {
					return nil, err
				}
				b.WriteRune(r)
				continue
			default:
				return nil, d.error("Invalid \\escape")
			}
			d.pos++
		case c < 0x20:
			return nil, d.error("Invalid control character")
		default:
			b.WriteByte(c)
			d.pos++
		}
	}
	return nil, d.error("Unterminated string starting at")
}

// parseUnicodeEscape reads \uXXXX, combining a surrogate pair into one rune.
func (d *decoder) parseUnicodeEscape() (rune, error) {
	// d.pos is already at the 'u'.
	if d.pos+4 >= len(d.src) {
		return 0, d.error("Invalid \\uXXXX escape")
	}
	value, err := strconv.ParseUint(d.src[d.pos+1:d.pos+5], 16, 32)
	if err != nil {
		return 0, d.error("Invalid \\uXXXX escape")
	}
	d.pos += 5
	r := rune(value)

	// A high surrogate is only valid as the first half of a pair.
	if r >= 0xD800 && r <= 0xDBFF && d.pos+5 < len(d.src)+1 && strings.HasPrefix(d.src[d.pos:], `\u`) {
		if low, err := strconv.ParseUint(d.src[d.pos+2:d.pos+6], 16, 32); err == nil {
			l := rune(low)
			if l >= 0xDC00 && l <= 0xDFFF {
				d.pos += 6
				return utf16.DecodeRune(r, l), nil
			}
		}
	}
	return r, nil
}

func (d *decoder) parseNumber() (py.Object, error) {
	start := d.pos
	if d.pos < len(d.src) && d.src[d.pos] == '-' {
		d.pos++
	}
	isFloat := false
	for d.pos < len(d.src) {
		c := d.src[d.pos]
		if c >= '0' && c <= '9' {
			d.pos++
			continue
		}
		if c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
			isFloat = true
			d.pos++
			continue
		}
		break
	}
	text := d.src[start:d.pos]
	if !isFloat {
		// parse_int, when given, produces the value for an integer literal.
		if d.parseInt != nil {
			return py.Call(d.parseInt, py.Tuple{py.String(text)}, nil)
		}
	} else if d.parseFloat != nil {
		return py.Call(d.parseFloat, py.Tuple{py.String(text)}, nil)
	}
	if !isFloat {
		// An integer stays an int.  A number too large for the int type
		// becomes a bigint, which is what Python does.
		n, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return py.Int(n), nil
		}
		if big, bigErr := py.IntFromString(text, 10); bigErr == nil {
			return big, nil
		}
		return nil, d.error("Expecting value")
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, d.error("Expecting value")
	}
	return py.Float(f), nil
}

// Decoder is the scanning class.  It carries the hooks that let a caller
// transform what is produced: object_hook, object_pairs_hook, parse_float,
// parse_int and parse_constant.
type Decoder struct {
	objectHook      py.Object
	objectPairsHook py.Object
	parseFloat      py.Object
	parseInt        py.Object
	parseConstant   py.Object
}

var DecoderType = py.NewTypeX("json.JSONDecoder", "Simple JSON decoder.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d := &Decoder{}
	for k, v := range kwargs {
		switch k {
		case "object_hook":
			d.objectHook = v
		case "object_pairs_hook":
			d.objectPairsHook = v
		case "parse_float":
			d.parseFloat = v
		case "parse_int":
			d.parseInt = v
		case "parse_constant":
			d.parseConstant = v
		case "strict":
			// Accepted: this decoder is always strict about control
			// characters in strings, which is what strict=True means.
		default:
			return nil, py.ExceptionNewf(py.TypeError, "unexpected keyword argument %q", k)
		}
	}
	return d, nil
}, nil)

func (d *Decoder) Type() *py.Type { return DecoderType }

// newDecoderWithHooks builds a scanner that applies the decoder's hooks.
func newDecoderWithHooks(text string, d *Decoder) *decoder {
	sc := &decoder{src: text}
	if d != nil {
		sc.parseFloat = d.parseFloat
		sc.parseInt = d.parseInt
		sc.parseConstant = d.parseConstant
		sc.objectHook = d.objectHook
		sc.objectPairsHook = d.objectPairsHook
	}
	return sc
}

// decodeText runs the decoder over text and returns the value.
func (d *Decoder) decodeText(text string) (py.Object, error) {
	sc := newDecoderWithHooks(text, d)
	value, err := sc.parse()
	if err != nil {
		return nil, err
	}
	sc.skipSpace()
	if sc.pos < len(sc.src) {
		return nil, sc.error("Extra data")
	}
	return value, nil
}

func init() {
	DecoderType.Dict["decode"] = py.MustNewMethod("decode", func(self py.Object, args py.Tuple) (py.Object, error) {
		var s py.Object
		if err := py.UnpackTuple(args, nil, "decode", 1, 1, &s); err != nil {
			return nil, err
		}
		text, ok := s.(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "the JSON object must be str, not %s", s.Type().Name)
		}
		return self.(*Decoder).decodeText(string(text))
	}, 0, "Return the Python representation of s.")

	// raw_decode stops at the end of the first JSON value rather than
	// requiring the whole string to be one, and reports where it stopped.
	DecoderType.Dict["raw_decode"] = py.MustNewMethod("raw_decode", func(self py.Object, args py.Tuple) (py.Object, error) {
		var s py.Object
		idx := py.Object(py.Int(0))
		if err := py.UnpackTuple(args, nil, "raw_decode", 1, 2, &s, &idx); err != nil {
			return nil, err
		}
		text, ok := s.(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "the JSON object must be str, not %s", s.Type().Name)
		}
		start, err := py.IndexInt(idx)
		if err != nil {
			return nil, err
		}
		sc := newDecoderWithHooks(string(text), self.(*Decoder))
		sc.pos = start
		value, err := sc.parse()
		if err != nil {
			return nil, err
		}
		return py.Tuple{value, py.Int(sc.pos)}, nil
	}, 0, "Decode a JSON document from s and return (value, end_index).")

	// The hook attributes are readable, which is how code checks whether one
	// was set before relying on its effect.
	hookAttr := func(name string, get func(*Decoder) py.Object) *py.Property {
		return &py.Property{Fget: func(self py.Object) (py.Object, error) {
			if v := get(self.(*Decoder)); v != nil {
				return v, nil
			}
			return py.None, nil
		}}
	}
	DecoderType.Dict["object_hook"] = hookAttr("object_hook", func(d *Decoder) py.Object { return d.objectHook })
	DecoderType.Dict["object_pairs_hook"] = hookAttr("object_pairs_hook", func(d *Decoder) py.Object { return d.objectPairsHook })
	DecoderType.Dict["parse_float"] = hookAttr("parse_float", func(d *Decoder) py.Object { return d.parseFloat })
	DecoderType.Dict["parse_int"] = hookAttr("parse_int", func(d *Decoder) py.Object { return d.parseInt })
	DecoderType.Dict["parse_constant"] = hookAttr("parse_constant", func(d *Decoder) py.Object { return d.parseConstant })

	EncoderType.Dict["encode"] = py.MustNewMethod("encode", func(self py.Object, args py.Tuple) (py.Object, error) {
		var obj py.Object
		if err := py.UnpackTuple(args, nil, "encode", 1, 1, &obj); err != nil {
			return nil, err
		}
		return self.(*Encoder).encode(obj)
	}, 0, "Return a JSON string representation of a Python data structure.")

	EncoderType.Dict["indent"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			e := self.(*Encoder)
			if !e.hasIndent {
				return py.None, nil
			}
			return py.String(e.indent), nil
		},
	}
	EncoderType.Dict["sort_keys"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.NewBool(self.(*Encoder).sortKeys), nil
		},
	}
	EncoderType.Dict["skipkeys"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.NewBool(self.(*Encoder).skipkeys), nil
		},
	}
	EncoderType.Dict["item_separator"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.String(self.(*Encoder).itemSep), nil
		},
	}
	EncoderType.Dict["key_separator"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.String(self.(*Encoder).keySep), nil
		},
	}

	// iterencode yields the same text as encode() in pieces.  Here it is one
	// piece, which satisfies callers that concatenate the result.
	EncoderType.Dict["iterencode"] = py.MustNewMethod("iterencode", func(self py.Object, args py.Tuple) (py.Object, error) {
		var obj py.Object
		if err := py.UnpackTuple(args, nil, "iterencode", 1, 1, &obj); err != nil {
			return nil, err
		}
		text, err := self.(*Encoder).encode(obj)
		if err != nil {
			return nil, err
		}
		return py.NewIterator(py.Tuple{text}), nil
	}, 0, "Encode the given object and yield the string representation.")

	EncoderType.Dict["default"] = py.MustNewMethod("default", func(self py.Object, args py.Tuple) (py.Object, error) {
		var obj py.Object
		if err := py.UnpackTuple(args, nil, "default", 1, 1, &obj); err != nil {
			return nil, err
		}
		return nil, py.ExceptionNewf(py.TypeError, "Object of type %s is not JSON serializable", obj.Type().Name)
	}, 0, "Implement this method to serialise otherwise unsupported objects.")
}

// keep utf8 referenced: it documents that the decoder works on bytes and the
// valid UTF-8 of the input is assumed.
var _ = utf8.RuneLen
