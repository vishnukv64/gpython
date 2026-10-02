// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package html provides python's 'html' package.
//
// It carries escape() and unescape(), the general HTML manipulation
// functions, and hosts the html.parser and html.entities submodules.
package html

import (
	"strings"
	"unicode/utf8"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `General functions for HTML manipulation.`

func init() {
	globals := py.NewStringDict()
	globals.Set("escape", py.MustNewMethod("escape", escapeFn, 0, escape_doc))
	globals.Set("unescape", py.MustNewMethod("unescape", unescapeFn, 0, unescape_doc))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "html",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

const escape_doc = `Replace special characters "&", "<" and ">" to HTML-safe sequences.

If the optional flag quote is true (the default), the quotation mark
characters, both double quote (") and single quote (') characters are also
translated.

The result is a str, as in CPython; this differs from the HTML-escaped bytes
some older implementations returned.`

func escapeFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var sObj py.Object
	quote := py.Object(py.True)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:escape", []string{"s", "quote"}, &sObj, &quote); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(sObj)
	if err != nil {
		return nil, err
	}
	quoteValue := true
	if quote != py.None {
		b, err := py.ObjectIsTrue(quote)
		if err != nil {
			return nil, err
		}
		quoteValue = b
	}
	// The ampersand replacement must be first, or the entities introduced by
	// the later replacements would be double-escaped.
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	if quoteValue {
		s = strings.ReplaceAll(s, "\"", "&quot;")
		s = strings.ReplaceAll(s, "'", "&#x27;")
	}
	return py.String(s), nil
}

const unescape_doc = `Convert all named and numeric character references (e.g. &gt;, &#62;,
&x3e;) in the string s to the corresponding unicode characters.

This function uses the rules defined by the HTML 5 standard
for both valid and invalid character references, and the list of
HTML 5 named character references defined in html.entities.html5.`

func unescapeFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var sObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "unescape", 1, 1, &sObj); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(sObj)
	if err != nil {
		return nil, err
	}
	return py.String(unescape(s)), nil
}

// ---------------------------------------------------------------------------
// character reference handling
//
// This is CPython's html.entities/html module pair in Go.  The html5 table
// comes from the html.entities module so the two do not drift.

// invalidCharrefs is CPython's _invalid_charrefs: the Windows-1252 remapping
// the HTML5 standard prescribes for certain numeric references.
var invalidCharrefs = map[int]rune{
	0x00: '\ufffd', 0x0d: '\r', 0x80: '\u20ac', 0x81: '\x81', 0x82: '\u201a',
	0x83: '\u0192', 0x84: '\u201e', 0x85: '\u2026', 0x86: '\u2020',
	0x87: '\u2021', 0x88: '\u02c6', 0x89: '\u2030', 0x8a: '\u0160',
	0x8b: '\u2039', 0x8c: '\u0152', 0x8d: '\x8d', 0x8e: '\u017d',
	0x8f: '\x8f', 0x90: '\x90', 0x91: '\u2018', 0x92: '\u2019',
	0x93: '\u201c', 0x94: '\u201d', 0x95: '\u2022', 0x96: '\u2013',
	0x97: '\u2014', 0x98: '\u02dc', 0x99: '\u2122', 0x9a: '\u0161',
	0x9b: '\u203a', 0x9c: '\u0153', 0x9d: '\x9d', 0x9e: '\u017e',
	0x9f: '\u0178',
}

// invalidCodepoints is CPython's _invalid_codepoints: references to these
// code points expand to nothing.
var invalidCodepoints = buildInvalidCodepoints()

func buildInvalidCodepoints() map[int]bool {
	m := map[int]bool{}
	for i := 0x1; i <= 0x8; i++ {
		m[i] = true
	}
	for i := 0xe; i <= 0x1f; i++ {
		m[i] = true
	}
	for i := 0x7f; i <= 0x9f; i++ {
		m[i] = true
	}
	for i := 0xfdd0; i <= 0xfdef; i++ {
		m[i] = true
	}
	m[0xb] = true
	for _, base := range []int{0xfffe, 0x1fffe, 0x2fffe, 0x3fffe, 0x4fffe, 0x5fffe,
		0x6fffe, 0x7fffe, 0x8fffe, 0x9fffe, 0xafffe, 0xbfffe, 0xcfffe, 0xdfffe,
		0xefffe, 0xffffe, 0x10fffe} {
		m[base] = true
		m[base+1] = true
	}
	return m
}

// unescape replaces every character reference in s, using the module-level
// html5 table.
func unescape(s string) string {
	return Unescape(s)
}

// Unescape is the exported form of html.unescape, used by html.parser to
// convert character references in data.
func Unescape(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	table := html5Table()
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end, replacement, ok := matchCharref(s, i, table)
		if !ok {
			b.WriteByte('&')
			i++
			continue
		}
		b.WriteString(replacement)
		i = end
	}
	return b.String()
}

// matchCharref matches CPython's _charref regex at s[start:] (which begins with
// '&') and returns the end offset and the replacement text.  ok is false when
// no reference matches there, in which case the '&' is literal.
func matchCharref(s string, start int, table map[string]string) (int, string, bool) {
	i := start + 1
	if i >= len(s) {
		return 0, "", false
	}
	if s[i] == '#' {
		// Numeric: &#\d+;? or &#x[0-9a-fA-F]+;?
		j := i + 1
		hex := false
		if j < len(s) && (s[j] == 'x' || s[j] == 'X') {
			hex = true
			j++
		}
		digitsStart := j
		if hex {
			for j < len(s) && isHexDigit(s[j]) {
				j++
			}
		} else {
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
		}
		if j == digitsStart {
			return 0, "", false
		}
		if j < len(s) && s[j] == ';' {
			j++
		}
		numText := s[digitsStart : j]
		numText = strings.TrimSuffix(numText, ";")
		num := parseRefInt(numText, hex)
		if num < 0 {
			return 0, "", false
		}
		if r, ok := invalidCharrefs[num]; ok {
			return j, string(r), true
		}
		if num >= 0xD800 && num <= 0xDFFF || num > 0x10FFFF {
			return j, "\uFFFD", true
		}
		if invalidCodepoints[num] {
			return j, "", true
		}
		return j, string(rune(num)), true
	}
	// Named: [^\t\n\f <&#;]{1,32};?
	j := i
	for j < len(s) && j-i < 32 && !isCharrefStop(s[j]) {
		j++
	}
	if j == i {
		return 0, "", false
	}
	full := s[i:j]
	if j < len(s) && s[j] == ';' {
		full += ";"
	}
	if v, ok := table[full]; ok {
		end := i + len(full)
		return end, v, true
	}
	// Find the longest matching name, as the standard prescribes.  The whole
	// matched text is consumed and the unmatched tail is carried in the
	// replacement, exactly as CPython's _replace_charref does.
	end := i + len(full)
	for x := len(full) - 1; x > 1; x-- {
		if v, ok := table[full[:x]]; ok {
			return end, v + full[x:], true
		}
	}
	return 0, "", false
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func isCharrefStop(c byte) bool {
	switch c {
	case '\t', '\n', '\f', ' ', '<', '&', '#', ';':
		return true
	}
	return false
}

// parseRefInt parses a numeric character reference; -1 means "not a number".
func parseRefInt(s string, hex bool) int {
	return ParseRefInt(s, hex)
}

// ParseRefInt is the exported form of the numeric-reference parser.
func ParseRefInt(s string, hex bool) int {
	base := 10
	if hex {
		base = 16
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		d := -1
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case hex && c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case hex && c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return -1
		}
		n = n*base + d
		if n > 0x110000 {
			// Clamp; the caller reports anything past 0x10FFFF as U+FFFD.
			return 0x110001
		}
	}
	return n
}

// html5Table reads html.entities.html5, caching it for the process.
func html5Table() map[string]string {
	return HTML5Table()
}

// html5Cache caches the html5 table for the process.
var html5Cache map[string]string

// CharrefReplacement applies the HTML5 standard's remapping and invalid-codepoint
// rules to a numeric reference, returning the replacement text.
func CharrefReplacement(num int) string {
	if r, ok := invalidCharrefs[num]; ok {
		return string(r)
	}
	if num >= 0xD800 && num <= 0xDFFF || num > 0x10FFFF {
		return "\uFFFD"
	}
	if invalidCodepoints[num] {
		return ""
	}
	return string(rune(num))
}

// HTML5Table is the exported form of the html.entities.html5 lookup table.
func HTML5Table() map[string]string {
	if html5Cache != nil {
		return html5Cache
	}
	m := map[string]string{}
	mod := py.GetModuleImplOrNil("html.entities")
	if mod != nil {
		if v, ok := mod.Globals.Get("html5"); ok {
			if d, ok := v.(py.StringDict); ok {
				for _, entry := range d.Items() {
					key, err := d.DecodeKey(entry.Key)
					if err != nil {
						continue
					}
					ks, ok := key.(py.String)
					if !ok {
						continue
					}
					vs, err := py.StrAsString(entry.Value)
					if err != nil {
						continue
					}
					m[string(ks)] = vs
				}
			}
		}
	}
	html5Cache = m
	return m
}

// utf8Len keeps the unused-import shim honest for older toolchains; it is used
// by the parser's position bookkeeping.
var _ = utf8.RuneLen
