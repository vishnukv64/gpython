// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"fmt"
	"strconv"
	"strings"
)

// The unicode_escape and raw_unicode_escape codecs.  pickle protocol 0 writes
// every str with raw_unicode_escape, and protocols 0-2 save bytes through
// codecs.encode(..., 'latin1') - both as CPython's Lib/pickle.py does.

// encodeUnicodeEscape is str.encode('unicode_escape'): printable ASCII as is,
// \t \n \r \\ by name, other bytes below 256 as \xhh, the rest \uXXXX or
// \UXXXXXXXX.  Quotes are not escaped.
func encodeUnicodeEscape(text string) []byte {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r >= 0x20 && r < 0x7f:
			b.WriteByte(byte(r))
		case r < 0x100:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x10000:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	return []byte(b.String())
}

// encodeRawUnicodeEscape is str.encode('raw_unicode_escape'): every character
// below 256 is its latin-1 byte (backslashes included), the rest \uXXXX or
// \UXXXXXXXX.
func encodeRawUnicodeEscape(text string) []byte {
	out := make([]byte, 0, len(text))
	for _, r := range text {
		switch {
		case r < 0x100:
			out = append(out, byte(r))
		case r < 0x10000:
			out = append(out, fmt.Sprintf(`\u%04x`, r)...)
		default:
			out = append(out, fmt.Sprintf(`\U%08x`, r)...)
		}
	}
	return out
}

func escapeDecodeError(codec string, start, end int, reason string) error {
	if start == end {
		return ExceptionNewf(UnicodeDecodeError, "'%s' codec can't decode byte 0x5c in position %d: %s", codec, start, reason)
	}
	return ExceptionNewf(UnicodeDecodeError, "'%s' codec can't decode bytes in position %d-%d: %s", codec, start, end, reason)
}

// hexEscape reads n hex digits at raw[i:], the body of a \x, \u or \U escape
// that starts at start.
func hexEscape(codec string, raw []byte, start, i, n int) (rune, error) {
	what := map[int]string{2: `\xXX`, 4: `\uXXXX`, 8: `\UXXXXXXXX`}[n]
	if i+n > len(raw) {
		return 0, escapeDecodeError(codec, start, len(raw)-1, "truncated "+what+" escape")
	}
	v, err := strconv.ParseUint(string(raw[i:i+n]), 16, 32)
	if err != nil {
		end := i
		for end < i+n && strings.IndexByte("0123456789abcdefABCDEF", raw[end]) >= 0 {
			end++
		}
		return 0, escapeDecodeError(codec, start, end-1+boolInt(end == i), "truncated "+what+" escape")
	}
	if v > 0x10ffff {
		return 0, escapeDecodeError(codec, start, i+n-1, "illegal Unicode character")
	}
	return rune(v), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// decodeUnicodeEscape is bytes.decode('unicode_escape'): the bytes are latin-1
// text with Python's string-literal escapes.  An unknown escape is kept as
// written, as CPython does (it warns).
func decodeUnicodeEscape(raw []byte) (Object, error) {
	const codec = "unicodeescape"
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' {
			b.WriteRune(rune(c))
			continue
		}
		start := i
		i++
		if i >= len(raw) {
			return nil, escapeDecodeError(codec, start, start, `\ at end of string`)
		}
		switch e := raw[i]; e {
		case '\n':
		case '\\', '\'', '"':
			b.WriteByte(e)
		case 'a':
			b.WriteByte('\a')
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
		case 'v':
			b.WriteByte('\v')
		case '0', '1', '2', '3', '4', '5', '6', '7':
			v := int(e - '0')
			for k := 0; k < 2 && i+1 < len(raw) && raw[i+1] >= '0' && raw[i+1] <= '7'; k++ {
				i++
				v = v*8 + int(raw[i]-'0')
			}
			b.WriteRune(rune(v))
		case 'x', 'u', 'U':
			n := map[byte]int{'x': 2, 'u': 4, 'U': 8}[e]
			r, err := hexEscape(codec, raw, start, i+1, n)
			if err != nil {
				return nil, err
			}
			b.WriteRune(r)
			i += n
		default:
			b.WriteByte('\\')
			b.WriteRune(rune(e))
		}
	}
	return String(b.String()), nil
}

// decodeRawUnicodeEscape is bytes.decode('raw_unicode_escape'): latin-1, except
// that \uXXXX and \UXXXXXXXX after an odd run of backslashes are escapes.
func decodeRawUnicodeEscape(raw []byte) (Object, error) {
	const codec = "rawunicodeescape"
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' {
			b.WriteRune(rune(c))
			continue
		}
		run := 0
		for i+run < len(raw) && raw[i+run] == '\\' {
			run++
		}
		next := i + run
		if run%2 == 0 || next >= len(raw) || (raw[next] != 'u' && raw[next] != 'U') {
			b.WriteString(strings.Repeat(`\`, run))
			i = next - 1
			continue
		}
		b.WriteString(strings.Repeat(`\`, run-1))
		n := 4
		if raw[next] == 'U' {
			n = 8
		}
		r, err := hexEscape(codec, raw, next-1, next+1, n)
		if err != nil {
			return nil, err
		}
		b.WriteRune(r)
		i = next + n
	}
	return String(b.String()), nil
}
