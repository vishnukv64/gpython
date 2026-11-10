// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import "strings"

// latin1 decodes bytes to a String of the same code points (U+0000..U+00FF).
func latin1(b []byte) String {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return String(r)
}

// M__mod__ is printf-style formatting of bytes (PEP 461): the format is
// treated as Latin-1 text and run through str's "%" with bytes arguments
// turned into the same Latin-1 text, so %s/%b insert raw bytes; %r and %a insert the
// repr (positional arguments only).  Differences from CPython, none of which change a valid format's
// result: %s accepts a non-bytes argument by str() where CPython raises
// TypeError, and a result character above U+00FF (an int-valued %c is the one
// way) raises ValueError.
func (a Bytes) M__mod__(other Object) (Object, error) {
	conv := func(o Object) Object {
		switch v := o.(type) {
		case Bytes:
			return latin1(v)
		case *ByteArray:
			return latin1(v.b)
		}
		return o
	}
	var arg Object
	switch v := other.(type) {
	case Tuple:
		t := make(Tuple, len(v))
		for i, o := range v {
			t[i] = conv(o)
		}
		arg = t
	case StringDict:
		arg = other
	default:
		arg = conv(other)
	}
	// Positional conversions that need the original object (%r, %a): their
	// argument is replaced by its repr text and the conversion becomes %s.
	var reprAt []int
	nconv := 0
	// %b is %s for bytes; str's formatter knows only %s.  Skip "%%" pairs.
	f := []byte(a)
	var sb strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			sb.WriteRune(rune(f[i]))
			continue
		}
		sb.WriteByte('%')
		j := i + 1
		for j < len(f) && strings.IndexByte("0123456789.-+ #*()abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_%", f[j]) >= 0 {
			if f[j] == '%' {
				break
			}
			// A mapping key is copied whole.
			if f[j] == '(' {
				for j < len(f) && f[j] != ')' {
					sb.WriteRune(rune(f[j]))
					j++
				}
				continue
			}
			if strings.IndexByte("diouxXeEfFgGcrsab", f[j]) >= 0 {
				nconv++
				if f[j] == 'b' {
					sb.WriteByte('s')
				} else if f[j] == 'r' || f[j] == 'a' {
					reprAt = append(reprAt, nconv-1)
					sb.WriteByte('s')
				} else {
					sb.WriteByte(f[j])
				}
				j++
				break
			}
			sb.WriteRune(rune(f[j]))
			j++
		}
		if j < len(f) && f[j] == '%' && j == i+1 {
			sb.WriteByte('%')
			j++
		}
		i = j - 1
	}
	if len(reprAt) > 0 {
		orig := Tuple{other}
		if t, ok := other.(Tuple); ok {
			orig = t
		}
		t := make(Tuple, len(orig))
		for i, o := range orig {
			t[i] = conv(o)
		}
		for _, k := range reprAt {
			if k < len(orig) {
				r, err := Repr(orig[k])
				if err != nil {
					return nil, err
				}
				t[k] = r
			}
		}
		arg = t
	}
	res, err := String(sb.String()).M__mod__(arg)
	if err != nil {
		return nil, err
	}
	s := res.(String)
	out := make([]byte, 0, len(s))
	for _, r := range string(s) {
		if r > 0xff {
			return nil, ExceptionNewf(ValueError, "%%c arg not in range(256)")
		}
		out = append(out, byte(r))
	}
	return Bytes(out), nil
}
