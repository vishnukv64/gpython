// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The format-spec mini-language.
//
// It lives in this package rather than in the builtin module because BOTH
// "format(value, spec)" and "str.format" need it, and because a value's own
// __format__ has to be able to reach it.  The builtin was the only caller
// before, which is why ""{:5}".format(42)" failed with "unsupported format
// string passed to int.__format__" while "format(42, "5")" worked.

package py

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Format renders value according to a format spec, the way format() does.
func Format(value Object, spec string) (Object, error) {
	switch v := value.(type) {
	case String:
		return formatString(string(v), spec)
	case Int, Bool:
		return formatInt(v, spec)
	case Float:
		return formatFloat(v, spec)
	case NoneType:
		if spec == "" {
			return String("None"), nil
		}
		return nil, ExceptionNewf(TypeError, "unsupported format string passed to NoneType.__format__")
	}
	if spec == "" {
		return Str(value)
	}
	return nil, ExceptionNewf(TypeError, "unsupported format string passed to %s.__format__", value.Type().Name)
}

type formatSpec struct {
	fill      rune
	align     byte
	width     int
	comma     bool
	precision int // -1 when absent
	verb      byte
}

func parseFormatSpec(spec string) (formatSpec, error) {
	out := formatSpec{fill: ' ', precision: -1}
	rest := spec

	if len(rest) >= 2 {
		switch rest[1] {
		case '<', '>', '^':
			out.fill = rune(rest[0])
			out.align = rest[1]
			rest = rest[2:]
		}
	}
	if out.align == 0 && len(rest) >= 1 {
		switch rest[0] {
		case '<', '>', '^':
			out.align = rest[0]
			rest = rest[1:]
		}
	}
	if strings.HasPrefix(rest, ",") {
		out.comma = true
		rest = rest[1:]
	}
	if dot := strings.IndexByte(rest, '.'); dot >= 0 {
		prec := rest[dot+1:]
		rest = rest[:dot]
		i := 0
		for i < len(prec) && prec[i] >= '0' && prec[i] <= '9' {
			i++
		}
		if i == 0 {
			return out, ExceptionNewf(ValueError, "Format specifier missing precision")
		}
		out.precision = 0
		for _, r := range prec[:i] {
			out.precision = out.precision*10 + int(r-'0')
		}
		rest += prec[i:]
	}
	// A leading '0' before the width requests zero padding.
	if out.align == 0 && len(rest) >= 2 && rest[0] == '0' && rest[1] >= '0' && rest[1] <= '9' {
		out.fill = '0'
		rest = rest[1:]
	}
	// width: leading digits
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		out.width = out.width*10 + int(rest[i]-'0')
		i++
	}
	rest = rest[i:]
	if strings.Contains(rest, ",") {
		out.comma = true
		rest = strings.ReplaceAll(rest, ",", "")
	}
	if rest != "" {
		out.verb = rest[len(rest)-1]
		if len(rest) != 1 {
			return out, ExceptionNewf(ValueError, "Invalid format specifier %q", spec)
		}
	}
	return out, nil
}

func formatString(value, spec string) (Object, error) {
	if spec == "" {
		return String(value), nil
	}
	f, err := parseFormatSpec(spec)
	if err != nil {
		return nil, err
	}
	if f.verb != 0 && f.verb != 's' {
		return nil, ExceptionNewf(ValueError, "Unknown format code %q for object of type 'str'", string(f.verb))
	}
	if f.align == 0 {
		f.align = '<' // strings are left aligned by default
	}
	return String(pad(value, f)), nil
}

func formatInt(value Object, spec string) (Object, error) {
	i, err := Index(value)
	if err != nil {
		return nil, err
	}

	if spec != "" {
		f, err := parseFormatSpec(spec)
		if err != nil {
			return nil, err
		}
		switch f.verb {
		case 'f', 'F', 'e', 'E', 'g', 'G', '%':
			// CPython lets an int be formatted with the float types.
			return formatFloat(Float(i), spec)
		}
		body, err := intBody(Int(i), f)
		if err != nil {
			return nil, err
		}
		if f.align == 0 {
			f.align = '>'
		}
		return String(pad(body, f)), nil
	}

	return Str(value)
}

// intBody renders an integer in the requested base, with separators applied.
func intBody(value Object, f formatSpec) (string, error) {
	verb := f.verb
	if verb == 0 {
		verb = 'd'
	}

	var text string
	switch verb {
	case 'd':
		text = fmt.Sprintf("%d", int64(value.(Int)))
	case 'b', 'o', 'x', 'X':
		base := 2
		switch verb {
		case 'o':
			base = 8
		case 'x', 'X':
			base = 16
		}
		n, ok := toBigInt(value)
		if !ok {
			return "", ExceptionNewf(ValueError, "Unknown format code %q for object of type 'int'", string(verb))
		}
		text = n.Text(base)
		if verb == 'X' {
			text = strings.ToUpper(text)
		}
	default:
		return "", ExceptionNewf(ValueError, "Unknown format code %q for object of type 'int'", string(verb))
	}

	if f.comma && verb == 'd' {
		text = groupThousands(text)
	}
	return text, nil
}

func toBigInt(value Object) (*big.Int, bool) {
	switch v := value.(type) {
	case Int:
		return big.NewInt(int64(v)), true
	case *BigInt:
		return (*big.Int)(v), true
	}
	return nil, false
}

// groupThousands inserts the "," separator every three digits of the integer
// part.
func groupThousands(text string) string {
	neg := strings.HasPrefix(text, "-")
	if neg {
		text = text[1:]
	}
	var out strings.Builder
	for i, r := range text {
		if i > 0 && (len(text)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(r)
	}
	if neg {
		return "-" + out.String()
	}
	return out.String()
}

func formatFloat(value Float, spec string) (Object, error) {
	if spec == "" {
		return Str(value)
	}
	f, err := parseFormatSpec(spec)
	if err != nil {
		return nil, err
	}

	verb := f.verb
	if verb == 0 {
		verb = 'g'
	}

	format := "%"
	if f.precision >= 0 {
		format += "." + itoa(f.precision)
	}
	switch verb {
	case 'f', 'F', 'e', 'E', 'g', 'G':
		format += string(verb)
	case '%':
		format += "%"
	default:
		return nil, ExceptionNewf(ValueError, "Unknown format code %q for object of type 'float'", string(verb))
	}

	body := fmt.Sprintf(format, float64(value))
	if f.comma {
		if dot := strings.IndexByte(body, '.'); dot >= 0 {
			body = groupThousands(body[:dot]) + body[dot:]
		} else {
			body = groupThousands(body)
		}
	}
	if f.align == 0 {
		f.align = '>'
	}
	return String(pad(body, f)), nil
}

// pad applies filling, alignment and width to an already rendered value.
func pad(body string, f formatSpec) string {
	width := len([]rune(body))
	if f.width <= width {
		return body
	}
	gap := f.width - width
	fill := string(f.fill)
	switch f.align {
	case '<':
		return body + strings.Repeat(fill, gap)
	case '^':
		left := gap / 2
		return strings.Repeat(fill, left) + body + strings.Repeat(fill, gap-left)
	default:
		return strings.Repeat(fill, gap) + body
	}
}

// itoa renders an int the way CPython's int formatting does.
func itoa(i int) string { return strconv.Itoa(i) }
