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
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Format renders value according to a format spec, the way format() does.
func Format(value Object, spec string) (Object, error) {
	switch v := value.(type) {
	case String:
		return formatString(string(v), spec)
	case Int:
		return formatInt(v, spec)
	case Float:
		return formatFloat(v, spec)
	}
	// str.__format__ / int.__format__ on a SUBCLASS: an empty spec is str(value),
	// which honours a Python __str__; any other spec formats the builtin value.
	// A __format__ written in Python wins over both.
	if p, ok := scalarOf(value); ok && value.(*Type).LookupPython("__format__") == nil {
		if spec == "" {
			return Str(value)
		}
		return Format(p, spec)
	}
	switch v := value.(type) {
	case Bool:
		return formatInt(v, spec)
	case *BigInt:
		return formatInt(v, spec)
	case NoneType:
		if spec == "" {
			return String("None"), nil
		}
		return nil, ExceptionNewf(TypeError, "unsupported format string passed to NoneType.__format__")
	}
	// A __format__ written in Python lives in the class Dict.
	if res, found, err := value.Type().CallMethod("__format__", Tuple{value, String(spec)}, NewStringDict()); err != nil {
		return nil, err
	} else if found && res != NotImplemented {
		return res, nil
	}
	if spec == "" {
		return Str(value)
	}
	return nil, ExceptionNewf(TypeError, "unsupported format string passed to %s.__format__", value.Type().Name)
}

type formatSpec struct {
	fill      rune
	align     byte // 0 when not given
	sign      byte // '+', '-', ' '
	alt       bool
	zeroCoerc bool // 'z'
	width     int
	group     byte // ',' or '_'
	precision int  // -1 when absent
	verb      byte // 0 when absent
}

func formatSpecErr(spec, typ string) error {
	return ExceptionNewf(ValueError, "Invalid format specifier '%s' for object of type '%s'", spec, typ)
}

func isAlign(c byte) bool { return c == '<' || c == '>' || c == '^' || c == '=' }

func parseNum(s string, pos *int, spec, typ string) (int, bool, error) {
	start := *pos
	n := 0
	for *pos < len(s) && s[*pos] >= '0' && s[*pos] <= '9' {
		if n > (math.MaxInt32-9)/10 {
			return 0, false, ExceptionNewf(ValueError, "Too many decimal digits in format string")
		}
		n = n*10 + int(s[*pos]-'0')
		*pos++
	}
	return n, *pos > start, nil
}

// parseFormatSpec follows CPython's format-spec mini-language:
// [[fill]align][sign]["z"]["#"]["0"][width][grouping]["." precision][type]
func parseFormatSpec(spec, typ string, defAlign byte) (formatSpec, error) {
	f := formatSpec{fill: ' ', precision: -1, align: defAlign}
	pos := 0
	fillGiven := false
	alignGiven := false
	if len(spec) > 0 {
		r, sz := rune(spec[0]), 1
		if r >= utf8.RuneSelf {
			r, sz = utf8.DecodeRuneInString(spec)
		}
		if sz < len(spec) && isAlign(spec[sz]) {
			f.fill, f.align, pos = r, spec[sz], sz+1
			fillGiven, alignGiven = true, true
		} else if isAlign(spec[0]) {
			f.align, pos = spec[0], 1
			alignGiven = true
		}
	}
	if pos < len(spec) && (spec[pos] == '+' || spec[pos] == '-' || spec[pos] == ' ') {
		f.sign = spec[pos]
		pos++
	}
	if pos < len(spec) && spec[pos] == 'z' {
		f.zeroCoerc = true
		pos++
	}
	if pos < len(spec) && spec[pos] == '#' {
		f.alt = true
		pos++
	}
	if !fillGiven && pos < len(spec) && spec[pos] == '0' {
		f.fill = '0'
		if !alignGiven && defAlign == '>' {
			f.align = '='
		}
		pos++
	}
	var err error
	if f.width, _, err = parseNum(spec, &pos, spec, typ); err != nil {
		return f, err
	}
	if pos < len(spec) && (spec[pos] == ',' || spec[pos] == '_') {
		f.group = spec[pos]
		pos++
	}
	if pos < len(spec) && (spec[pos] == ',' || spec[pos] == '_') {
		if spec[pos] == f.group {
			return f, formatSpecErr(spec, typ)
		}
		return f, ExceptionNewf(ValueError, "Cannot specify both ',' and '_'.")
	}
	if pos < len(spec) && spec[pos] == '.' {
		pos++
		var ok bool
		if f.precision, ok, err = parseNum(spec, &pos, spec, typ); err != nil {
			return f, err
		}
		if !ok {
			return f, ExceptionNewf(ValueError, "Format specifier missing precision")
		}
	}
	if len(spec)-pos > 1 {
		return f, formatSpecErr(spec, typ)
	}
	if pos < len(spec) {
		f.verb = spec[pos]
	}
	if f.group != 0 {
		ok := false
		switch f.verb {
		case 0, 'd', 'e', 'E', 'f', 'F', 'g', 'G', '%':
			ok = true
		case 'b', 'o', 'x', 'X':
			ok = f.group == '_'
		}
		if !ok && f.verb != 0 && !(f.verb == 's' && typ == "str") {
			return f, ExceptionNewf(ValueError, "Cannot specify '%c' with '%c'.", f.group, f.verb)
		}
	}
	return f, nil
}

func unknownCode(verb byte, typ string) error {
	if verb > 32 && verb < 128 {
		return ExceptionNewf(ValueError, "Unknown format code '%c' for object of type '%s'", verb, typ)
	}
	return ExceptionNewf(ValueError, "Unknown format code '\\x%x' for object of type '%s'", verb, typ)
}

func formatString(value, spec string) (Object, error) {
	if spec == "" {
		return String(value), nil
	}
	f, err := parseFormatSpec(spec, "str", '<')
	if err != nil {
		return nil, err
	}
	if f.verb != 0 && f.verb != 's' {
		return nil, unknownCode(f.verb, "str")
	}
	switch {
	case f.sign != 0:
		return nil, ExceptionNewf(ValueError, "Sign not allowed in string format specifier")
	case f.zeroCoerc:
		return nil, ExceptionNewf(ValueError, "Negative zero coercion (z) not allowed in format specifier")
	case f.alt:
		return nil, ExceptionNewf(ValueError, "Alternate form (#) not allowed in string format specifier")
	case f.align == '=':
		return nil, ExceptionNewf(ValueError, "'=' alignment not allowed in string format specifier")
	case f.group != 0:
		return nil, ExceptionNewf(ValueError, "Cannot specify '%c' with 's'.", f.group)
	}
	n := len(value)
	if !isASCII(value) {
		n = utf8.RuneCountInString(value)
	}
	if f.precision >= 0 && f.precision < n {
		if n == len(value) {
			value = value[:f.precision]
		} else {
			i := 0
			for k := 0; k < f.precision; k++ {
				_, sz := utf8.DecodeRuneInString(value[i:])
				i += sz
			}
			value = value[:i]
		}
		n = f.precision
	}
	return String(padText("", value, n, f)), nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// padText lays out prefix+body (n runes of body) in f.width; '=' puts the
// fill between prefix and body.
func padText(prefix, body string, n int, f formatSpec) string {
	return padParts(prefix, nil, body, n, f)
}

// padParts is padText with the body in two pieces, digits then tail, so that
// callers can pass digits from a stack buffer without converting to string.
func padParts(prefix string, digits []byte, tail string, n int, f formatSpec) string {
	gap := f.width - len(prefix) - n
	if gap < 0 {
		gap = 0
	}
	var left, right, mid int
	switch f.align {
	case '<':
		right = gap
	case '^':
		left = gap / 2
		right = gap - left
	case '=':
		mid = gap
	default:
		left = gap
	}
	var b strings.Builder
	b.Grow(len(prefix) + len(digits) + len(tail) + gap*utf8.RuneLen(f.fill))
	writeFill(&b, f.fill, left)
	b.WriteString(prefix)
	writeFill(&b, f.fill, mid)
	b.Write(digits)
	b.WriteString(tail)
	writeFill(&b, f.fill, right)
	return b.String()
}

func writeFill(b *strings.Builder, r rune, n int) {
	if r < utf8.RuneSelf {
		for ; n > 0; n-- {
			b.WriteByte(byte(r))
		}
		return
	}
	for ; n > 0; n-- {
		b.WriteRune(r)
	}
}

// groupDigits inserts sep every size digits of digits (no leading sep),
// left-padding with zeros until the result is at least minLen long.
func groupDigits(digits string, sep byte, size, minLen int) string {
	n := len(digits)
	pad := 0
	for {
		total := n + pad
		l := total + (total-1)/size
		if l >= minLen {
			break
		}
		pad++
	}
	total := n + pad
	out := make([]byte, 0, total+(total-1)/size)
	for i := 0; i < total; i++ {
		if i > 0 && (total-i)%size == 0 {
			out = append(out, sep)
		}
		if k := i - pad; k >= 0 {
			out = append(out, digits[k])
		} else {
			out = append(out, '0')
		}
	}
	return string(out)
}

func signPrefix(neg bool, f formatSpec) string {
	switch {
	case neg:
		return "-"
	case f.sign == '+':
		return "+"
	case f.sign == ' ':
		return " "
	}
	return ""
}

// numLayout groups the integer digits (when asked) and pads.  tail is what
// follows the integer digits (fraction, exponent, '%').
func numLayout(prefix, digits, tail string, f formatSpec, size int) string {
	if f.group == 0 {
		return padText(prefix, digits+tail, len(digits)+len(tail), f)
	}
	min := 0
	if f.align == '=' && f.fill == '0' {
		min = f.width - len(prefix) - len(tail)
	}
	g := groupDigits(digits, f.group, size, min)
	return padText(prefix, g+tail, len(g)+len(tail), f)
}

func formatInt(value Object, spec string) (Object, error) {
	if spec == "" {
		return Str(value)
	}
	typ := "int"
	if _, isBool := value.(Bool); isBool {
		typ = "bool"
	}
	f, err := parseFormatSpec(spec, typ, '>')
	if err != nil {
		return nil, err
	}
	var neg bool
	var small uint64
	var big_ *big.Int
	switch v := value.(type) {
	case Int:
		if v < 0 {
			neg, small = true, uint64(-(v+1))+1
		} else {
			small = uint64(v)
		}
	case Bool:
		if v {
			small = 1
		}
	case *BigInt:
		b := (*big.Int)(v)
		neg = b.Sign() < 0
		big_ = new(big.Int).Abs(b)
	}
	switch f.verb {
	case 'e', 'E', 'f', 'F', 'g', 'G', '%':
		var x float64
		if big_ != nil {
			x, _ = new(big.Float).SetInt((*big.Int)(value.(*BigInt))).Float64()
			if math.IsInf(x, 0) {
				return nil, ExceptionNewf(OverflowError, "int too large to convert to float")
			}
		} else {
			x = float64(Index0(value))
		}
		return formatFloat(Float(x), spec)
	case 0, 'd', 'n', 'b', 'o', 'x', 'X', 'c':
	default:
		return nil, unknownCode(f.verb, typ)
	}
	if f.precision >= 0 {
		return nil, ExceptionNewf(ValueError, "Precision not allowed in integer format specifier")
	}
	if f.zeroCoerc {
		return nil, ExceptionNewf(ValueError, "Negative zero coercion (z) not allowed in integer format specifier")
	}
	base := 10
	prefix := ""
	switch f.verb {
	case 'b':
		base, prefix = 2, "0b"
	case 'o':
		base, prefix = 8, "0o"
	case 'x':
		base, prefix = 16, "0x"
	case 'X':
		base, prefix = 16, "0X"
	}
	if f.verb == 'c' {
		if f.sign != 0 {
			return nil, ExceptionNewf(ValueError, "Sign not allowed with integer format specifier 'c'")
		}
		if f.alt {
			return nil, ExceptionNewf(ValueError, "Alternate form (#) not allowed with integer format specifier 'c'")
		}
		if f.group != 0 {
			return nil, ExceptionNewf(ValueError, "Cannot specify '%c' with 'c'.", f.group)
		}
		if big_ != nil {
			return nil, ExceptionNewf(OverflowError, "Python int too large to convert to C long")
		}
		if neg || small > 0x10ffff {
			return nil, ExceptionNewf(OverflowError, "%%c arg not in range(0x110000)")
		}
		return String(padText("", string(rune(small)), 1, f)), nil
	}
	if f.group == ',' && base != 10 || f.group != 0 && f.verb == 'n' {
		return nil, ExceptionNewf(ValueError, "Cannot specify '%c' with '%c'.", f.group, f.verb)
	}
	if !f.alt {
		prefix = ""
	}
	if big_ == nil && f.group == 0 && f.verb != 'X' {
		var buf [64]byte
		d := strconv.AppendUint(buf[:0], small, base)
		return String(padParts(signPrefix(neg, f)+prefix, d, "", len(d), f)), nil
	}
	var digits string
	if big_ != nil {
		digits = big_.Text(base)
	} else {
		digits = strconv.FormatUint(small, base)
	}
	if f.verb == 'X' {
		digits = strings.ToUpper(digits)
	}
	size := 3
	if base != 10 {
		size = 4
	}
	return String(numLayout(signPrefix(neg, f)+prefix, digits, "", f, size)), nil
}

// Index0 is the int64 of an Int or Bool.
func Index0(v Object) int64 {
	switch v := v.(type) {
	case Int:
		return int64(v)
	case Bool:
		if v {
			return 1
		}
	}
	return 0
}

func formatFloat(value Float, spec string) (Object, error) {
	if spec == "" {
		return Str(value)
	}
	f, err := parseFormatSpec(spec, "float", '>')
	if err != nil {
		return nil, err
	}
	x := float64(value)
	verb := f.verb
	switch verb {
	case 0, 'e', 'E', 'f', 'F', 'g', 'G', 'n', '%':
	default:
		return nil, unknownCode(verb, "float")
	}
	if f.group == ',' && verb == 'n' || f.group == '_' && verb == 'n' {
		return nil, ExceptionNewf(ValueError, "Cannot specify '%c' with 'n'.", f.group)
	}
	neg := math.Signbit(x)
	if math.IsNaN(x) {
		neg = false
	}
	ax := math.Abs(x)
	if verb == '%' {
		ax *= 100
	}
	var body string // digits, maybe '.', fraction, exponent
	switch {
	case math.IsNaN(x) || math.IsInf(x, 0):
		body = "inf"
		if math.IsNaN(x) {
			body = "nan"
		}
		if verb == 'E' || verb == 'F' || verb == 'G' {
			body = strings.ToUpper(body)
		}
		if verb == '%' {
			body += "%"
		}
		pre := signPrefix(neg, f)
		return String(padText(pre, body, len(body), f)), nil
	default:
		body = floatBody(ax, verb, f)
	}
	// split body into integer digits and the rest
	end := 0
	for end < len(body) && body[end] >= '0' && body[end] <= '9' {
		end++
	}
	digits, tail := body[:end], body[end:]
	if f.zeroCoerc && neg && isZeroBody(digits+tail) {
		neg = false
	}
	if verb == '%' {
		tail += "%"
	}
	return String(numLayout(signPrefix(neg, f), digits, tail, f, 3)), nil
}

func isZeroBody(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '1' && c <= '9' {
			return false
		}
		if c == 'e' || c == 'E' || c == '%' {
			break
		}
	}
	return true
}

func floatBody(ax float64, verb byte, f formatSpec) string {
	prec := f.precision
	switch verb {
	case 'f', 'F', '%':
		if prec < 0 {
			prec = 6
		}
		s := strconv.FormatFloat(ax, 'f', prec, 64)
		if f.alt && prec == 0 {
			s += "."
		}
		return s
	case 'e', 'E':
		if prec < 0 {
			prec = 6
		}
		s := strconv.FormatFloat(ax, 'e', prec, 64)
		if f.alt && prec == 0 {
			s = strings.Replace(s, "e", ".e", 1)
		}
		if verb == 'E' {
			s = strings.ToUpper(s)
		}
		return s
	case 0:
		if prec < 0 {
			s := cpythonFloatRepr(ax)
			return s
		}
	}
	// g, G, n, and the typeless form with a precision
	if prec < 0 {
		prec = 6
	}
	if prec == 0 {
		prec = 1
	}
	es := strconv.FormatFloat(ax, 'e', prec-1, 64)
	k := strings.IndexByte(es, 'e')
	exp, _ := strconv.Atoi(es[k+1:])
	var s string
	if exp >= -4 && exp < prec {
		s = strconv.FormatFloat(ax, 'f', prec-1-exp, 64)
		if !f.alt {
			s = trimFrac(s)
		}
		if verb == 0 && !strings.Contains(s, ".") {
			s += ".0"
		}
	} else {
		m := es[:k]
		if !f.alt {
			m = trimFrac(m)
		}
		if verb == 0 && !strings.Contains(m, ".") {
			m += ".0"
		}
		s = m + es[k:]
		if verb == 0 {
			// typeless: CPython drops the ".0" before an exponent
			s = strings.Replace(s, ".0e", "e", 1)
		}
	}
	if f.alt && !strings.Contains(s, ".") && !strings.Contains(s, "e") {
		s += "."
	}
	if verb == 'G' {
		s = strings.ToUpper(s)
	}
	return s
}

func trimFrac(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func itoa(i int) string { return strconv.Itoa(i) }
