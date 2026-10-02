// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pickle

import (
	"math"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// globalRef is a module/name pair pushed by the c opcode and consumed by the R
// that follows.  It is a py.Object so that it can live on the value stack.
type globalRef struct {
	module string
	name   string
}

// GlobalRefType is the type of the interim GLOBAL marker.
var GlobalRefType = py.NewType("pickle._global", "The module/name pair a protocol-0 GLOBAL opcode names.")

func (g globalRef) Type() *py.Type { return GlobalRefType }

// decoder reads the protocol-0 opcode stream.
//
// Protocol 0 is a stack machine.  MARK opens a group that TUPLE, LIST or DICT
// closes by collecting everything pushed since; REDUCE calls the GLOBAL pushed
// before the group with that group as *arguments.  The p and g opcodes index an
// unbounded memo, which is what preserves a shared sub-object.
type decoder struct {
	s     string
	i     int
	stack []py.Object
	mark  []int
	memo  []py.Object
	// last is the most recently pushed value, which is what a memo PUT stores.
	last py.Object
}

func (d *decoder) decode() (py.Object, error) {
	for {
		if d.i >= len(d.s) {
			return nil, py.ExceptionNewf(UnpicklingError, "pickle data was truncated")
		}
		c := d.s[d.i]
		d.i++
		switch c {
		case '.':
			return d.result()
		case 'N':
			d.push(py.None)
		case '(':
			d.mark = append(d.mark, len(d.stack))
		case 'I', 'L', 'F', 'V', 'S':
			if err := d.readScalar(c); err != nil {
				return nil, err
			}
		case 'c':
			if err := d.readGlobal(); err != nil {
				return nil, err
			}
		case 'p':
			if err := d.memoPut(); err != nil {
				return nil, err
			}
		case 'g':
			if err := d.memoGet(); err != nil {
				return nil, err
			}
		case 't':
			d.push(py.Tuple(d.popToMark()))
		case 'l':
			d.push(py.NewListFromItems(d.popToMark()))
		case 'd':
			if err := d.buildDict(); err != nil {
				return nil, err
			}
		case 'a':
			if err := d.appendItem(); err != nil {
				return nil, err
			}
		case 's':
			if err := d.setItem(); err != nil {
				return nil, err
			}
		case 'R':
			if err := d.reduce(); err != nil {
				return nil, err
			}
		default:
			return nil, py.ExceptionNewf(UnpicklingError, "unsupported pickle opcode: %q", string(c))
		}
	}
}

// result is the value the STOP leaves: the single stack entry, or the last
// value pushed when nothing remains.
func (d *decoder) result() (py.Object, error) {
	switch len(d.stack) {
	case 0:
		if d.last == nil {
			return nil, py.ExceptionNewf(UnpicklingError, "pickle data was truncated")
		}
		return d.last, nil
	case 1:
		return d.stack[0], nil
	}
	return nil, py.ExceptionNewf(UnpicklingError, "unexpected data after pickle value")
}

func (d *decoder) push(v py.Object) {
	d.stack = append(d.stack, v)
	d.last = v
}

func (d *decoder) pop() (py.Object, error) {
	if len(d.stack) == 0 {
		return nil, py.ExceptionNewf(UnpicklingError, "pickle stack underflow")
	}
	v := d.stack[len(d.stack)-1]
	d.stack = d.stack[:len(d.stack)-1]
	return v, nil
}

// popToMark pops everything pushed since the innermost MARK.
func (d *decoder) popToMark() []py.Object {
	if len(d.mark) == 0 {
		return nil
	}
	at := d.mark[len(d.mark)-1]
	d.mark = d.mark[:len(d.mark)-1]
	items := append([]py.Object(nil), d.stack[at:]...)
	d.stack = d.stack[:at]
	return items
}

// readLine reads to the next newline and returns the text without it.
func (d *decoder) readLine() (string, bool) {
	nl := strings.IndexByte(d.s[d.i:], '\n')
	if nl < 0 {
		return "", false
	}
	line := d.s[d.i : d.i+nl]
	d.i += nl + 1
	return line, true
}

func (d *decoder) readScalar(op byte) error {
	switch op {
	case 'I':
		line, ok := d.readLine()
		if !ok {
			return py.ExceptionNewf(UnpicklingError, "pickle data was truncated")
		}
		switch line {
		case "01":
			d.push(py.True)
			return nil
		case "00":
			d.push(py.False)
			return nil
		}
		n, err := strconv.ParseInt(strings.TrimSpace(line), 10, 64)
		if err != nil {
			return py.ExceptionNewf(UnpicklingError, "invalid integer literal: %q", line)
		}
		d.push(py.Int(n))
	case 'L':
		// CPython reads the L opcode as a whole line and strips a trailing
		// "L", so L<decimal>L\n and L<decimal>\n are both valid.
		line, ok := d.readLine()
		if !ok {
			return py.ExceptionNewf(UnpicklingError, "pickle data was truncated")
		}
		text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "L"))
		// The L form carries values that may not fit a machine int.
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			d.push(py.Int(n))
			return nil
		}
		v, err := py.IntFromString(text, 10)
		if err != nil {
			return err
		}
		d.push(v)
	case 'F':
		line, ok := d.readLine()
		if !ok {
			return py.ExceptionNewf(UnpicklingError, "pickle data was truncated")
		}
		f, err := parseFloatLine(line)
		if err != nil {
			return err
		}
		d.push(py.Float(f))
	case 'V', 'S':
		s, err := d.readString(op)
		if err != nil {
			return err
		}
		d.push(s)
	}
	return nil
}

// readString reads the V (unicode, line-terminated, backslash-escaped) and S
// (quoted) string bodies.
func (d *decoder) readString(op byte) (py.Object, error) {
	line, ok := d.readLine()
	if !ok {
		return nil, py.ExceptionNewf(UnpicklingError, "pickle data was truncated")
	}
	if op == 'S' {
		line = strings.TrimSpace(line)
		if len(line) >= 2 && (line[0] == '\'' || line[0] == '"') {
			// The quoted form is a Python literal; strconv.Unquote reads the
			// same backslash escapes, except that Python also writes \xNN in
			// single-quoted strings, which Unquote understands.
			if unq, err := strconv.Unquote(line); err == nil {
				return py.String(unq), nil
			}
			if len(line) >= 2 {
				return py.String(decodeUnicode(line[1 : len(line)-1])), nil
			}
		}
	}
	return py.String(decodeUnicode(line)), nil
}

func (d *decoder) readGlobal() error {
	module, ok := d.readLine()
	if !ok {
		return py.ExceptionNewf(UnpicklingError, "pickle data was truncated")
	}
	name, ok := d.readLine()
	if !ok {
		return py.ExceptionNewf(UnpicklingError, "pickle data was truncated")
	}
	d.push(globalRef{module: module, name: name})
	return nil
}

func (d *decoder) memoPut() error {
	idx, err := d.readIndex("memo index")
	if err != nil {
		return err
	}
	for len(d.memo) <= idx {
		d.memo = append(d.memo, nil)
	}
	d.memo[idx] = d.last
	return nil
}

func (d *decoder) memoGet() error {
	idx, err := d.readIndex("memo index")
	if err != nil {
		return err
	}
	if idx >= len(d.memo) || d.memo[idx] == nil {
		return py.ExceptionNewf(UnpicklingError, "pickle memo key %d not found", idx)
	}
	d.push(d.memo[idx])
	return nil
}

func (d *decoder) readIndex(what string) (int, error) {
	line, ok := d.readLine()
	if !ok {
		return 0, py.ExceptionNewf(UnpicklingError, "pickle data was truncated")
	}
	idx, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		return 0, py.ExceptionNewf(UnpicklingError, "invalid %s: %q", what, line)
	}
	return idx, nil
}

func (d *decoder) buildDict() error {
	items := d.popToMark()
	if len(items)%2 != 0 {
		return py.ExceptionNewf(UnpicklingError, "odd number of items for DICT")
	}
	dict := py.NewStringDict()
	for i := 0; i < len(items); i += 2 {
		k, err := py.StrAsString(items[i])
		if err != nil {
			return py.ExceptionNewf(UnpicklingError, "dict key is not a string: %s", items[i].Type().Name)
		}
		(&dict).Set(k, items[i+1])
	}
	d.push(dict)
	return nil
}

func (d *decoder) appendItem() error {
	item, err := d.pop()
	if err != nil {
		return err
	}
	if len(d.stack) == 0 {
		return py.ExceptionNewf(UnpicklingError, "APPEND with no list")
	}
	list, ok := d.stack[len(d.stack)-1].(*py.List)
	if !ok {
		return py.ExceptionNewf(UnpicklingError, "APPEND target is not a list")
	}
	list.Items = append(list.Items, item)
	d.last = list
	return nil
}

func (d *decoder) setItem() error {
	value, err := d.pop()
	if err != nil {
		return err
	}
	key, err := d.pop()
	if err != nil {
		return err
	}
	if len(d.stack) == 0 {
		return py.ExceptionNewf(UnpicklingError, "SETITEM with no dict")
	}
	dict, ok := d.stack[len(d.stack)-1].(py.StringDict)
	if !ok {
		return py.ExceptionNewf(UnpicklingError, "SETITEM target is not a dict")
	}
	ks, err := py.StrAsString(key)
	if err != nil {
		return py.ExceptionNewf(UnpicklingError, "dict key is not a string: %s", key.Type().Name)
	}
	(&dict).Set(ks, value)
	d.last = dict
	return nil
}

// reduce applies R: the value below the arguments is the callable, and the
// value on top (a tuple, from the TUPLE or MARK..DICT group before it) is its
// arguments.
func (d *decoder) reduce() error {
	argsObj, err := d.pop()
	if err != nil {
		return err
	}
	argv, ok := argsObj.(py.Tuple)
	if !ok {
		argv = py.Tuple{argsObj}
	}
	fnObj, err := d.pop()
	if err != nil {
		return err
	}
	fn, ok := fnObj.(globalRef)
	if !ok {
		return py.ExceptionNewf(UnpicklingError, "REDUCE without a callable")
	}
	return d.callGlobal(fn, argv)
}

// callGlobal applies the GLOBAL reductions protocol 0 uses for the types that
// have no opcode of their own: bytes, bytearray, complex, set and frozenset.
func (d *decoder) callGlobal(fn globalRef, args py.Tuple) error {
	if fn.module == "_codecs" || fn.module == "codecs" {
		if fn.name != "encode" {
			return py.ExceptionNewf(UnpicklingError, "unsupported global: %s.%s", fn.module, fn.name)
		}
		if len(args) != 2 {
			return py.ExceptionNewf(UnpicklingError, "codecs.encode takes 2 arguments")
		}
		text, err := py.StrAsString(args[0])
		if err != nil {
			return err
		}
		encoding, err := py.StrAsString(args[1])
		if err != nil {
			return err
		}
		switch strings.ToLower(encoding) {
		case "latin1", "latin-1", "iso-8859-1", "iso8859-1":
		default:
			return py.ExceptionNewf(UnpicklingError, "unsupported codecs encoding: %s", encoding)
		}
		raw := make([]byte, 0, len(text))
		for _, r := range text {
			if r > 0xff {
				return py.ExceptionNewf(UnpicklingError, "character out of the latin-1 range")
			}
			raw = append(raw, byte(r))
		}
		d.push(py.Bytes(raw))
		return nil
	}
	if fn.module != "__builtin__" && fn.module != "builtins" {
		return py.ExceptionNewf(UnpicklingError, "unsupported global: %s.%s", fn.module, fn.name)
	}
	switch fn.name {
	case "bytearray":
		if len(args) != 1 {
			return py.ExceptionNewf(UnpicklingError, "bytearray takes 1 argument")
		}
		raw, err := bytesLike(args[0])
		if err != nil {
			return err
		}
		d.push(py.NewByteArray(raw))
	case "complex":
		if len(args) != 2 {
			return py.ExceptionNewf(UnpicklingError, "complex takes 2 arguments")
		}
		re, err := floatOf(args[0])
		if err != nil {
			return err
		}
		im, err := floatOf(args[1])
		if err != nil {
			return err
		}
		d.push(py.Complex(complex(re, im)))
	case "set", "frozenset":
		items, err := setMembers(args)
		if err != nil {
			return err
		}
		s, err := py.NewSetFromItemsErr(items)
		if err != nil {
			return err
		}
		if fn.name == "set" {
			d.push(s)
		} else {
			d.push(&py.FrozenSet{Set: *s})
		}
	default:
		return py.ExceptionNewf(UnpicklingError, "unsupported global: %s.%s", fn.module, fn.name)
	}
	return nil
}

// bytesLike accepts the bytes and bytearray a bytearray() reduce is called with.
func bytesLike(o py.Object) ([]byte, error) {
	switch v := o.(type) {
	case py.Bytes:
		return append([]byte(nil), []byte(v)...), nil
	case *py.ByteArray:
		return bytesOf(v)
	case py.Tuple:
		// A one-element tuple is what the TUPLE that carried the single
		// argument leaves; take the value it holds.
		if len(v) == 1 {
			return bytesLike(v[0])
		}
	}
	return nil, py.ExceptionNewf(UnpicklingError, "expected bytes, not %s", o.Type().Name)
}

// setMembers is the iterable argument set()/frozenset() was called with.
func setMembers(args py.Tuple) ([]py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(UnpicklingError, "set takes 1 argument")
	}
	arg := args[0]
	if t, ok := arg.(py.Tuple); ok {
		return []py.Object(t), nil
	}
	it, err := py.Iter(arg)
	if err != nil {
		return nil, err
	}
	var out []py.Object
	for {
		item, err := py.Next(it)
		if err == py.StopIteration {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func floatOf(o py.Object) (float64, error) {
	switch v := o.(type) {
	case py.Float:
		return float64(v), nil
	case py.Int:
		return float64(v), nil
	}
	return 0, py.ExceptionNewf(UnpicklingError, "expected a number, not %s", o.Type().Name)
}

// parseFloatLine reads the F opcode's payload, including the inf and nan
// spellings the format allows.
func parseFloatLine(line string) (float64, error) {
	text := strings.TrimSpace(line)
	switch strings.ToLower(text) {
	case "inf", "1e999", "1e+999", "+inf":
		return math.Inf(1), nil
	case "-inf", "-1e999", "-1e+999":
		return math.Inf(-1), nil
	case "nan":
		return math.NaN(), nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, py.ExceptionNewf(UnpicklingError, "invalid float literal: %q", line)
	}
	return f, nil
}

// decodeUnicode undoes the \u, \U and \x escapes of the V opcode.
func decodeUnicode(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		var width int
		switch s[i+1] {
		case 'u':
			width = 4
		case 'U':
			width = 8
		case 'x':
			width = 2
		case '\\', '\'':
			b.WriteByte(s[i+1])
			i += 2
			continue
		default:
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+2+width > len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		r, err := strconv.ParseUint(s[i+2:i+2+width], 16, 32)
		if err != nil {
			b.WriteByte(s[i])
			i++
			continue
		}
		b.WriteRune(rune(r))
		i += 2 + width
	}
	return b.String()
}
