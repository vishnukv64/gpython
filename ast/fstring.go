// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ast

import (
	"fmt"
	"strings"
)

// FStringPart is one piece of an f-string: either a literal or a
// replacement field.
type FStringPart struct {
	// Text is the literal text (used when Field is false)
	Text string
	// field parts
	Field bool
	Expr  string
	// Offset is the index, in runes of the text given to ParseFString, of the
	// first character of Expr.
	Offset     int
	Conversion byte // 0, 'r', 's' or 'a'
	FormatSpec string
	// Debug is the self-documenting form, f"{x=}": the rendered output is
	// prefixed with the source text of the expression and an "=".
	Debug     bool
	DebugText string
}

// fieldScan locates the parts of one replacement field.  All positions are
// indexes into the rune slice, or -1 when absent.
type fieldScan struct {
	end, eq, conv, spec int
}

func isPrefixLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func hasQuote(r []rune, i int, q string) bool {
	if i+len(q) > len(r) {
		return false
	}
	for k := 0; k < len(q); k++ {
		if r[i+k] != rune(q[k]) {
			return false
		}
	}
	return true
}

// skipString returns the index of the last rune of the string literal whose
// opening quote is r[j].  Nested f-strings and t-strings are scanned for
// their own replacement fields (PEP 701), so a quote inside one does not end
// the outer string.
func skipString(r []rune, j int) (int, error) {
	q := string(r[j])
	if hasQuote(r, j, q+q+q) {
		q = q + q + q
	}
	k := j
	for k > 0 && isPrefixLetter(r[k-1]) {
		k--
	}
	interp := strings.ContainsAny(string(r[k:j]), "fFtT")
	for p := j + len(q); p < len(r); p++ {
		switch c := r[p]; {
		case hasQuote(r, p, q):
			return p + len(q) - 1, nil
		case c == '\\' && p+1 < len(r) && !(interp && (r[p+1] == '{' || r[p+1] == '}')):
			p++
		case interp && c == '{':
			if p+1 < len(r) && r[p+1] == '{' {
				p++
				continue
			}
			fs, err := scanField(r, p+1)
			if err != nil {
				return 0, err
			}
			p = fs.end
		case interp && c == '}' && p+1 < len(r) && r[p+1] == '}':
			p++
		case c == '\n' && len(q) == 1:
			return 0, fmt.Errorf("unterminated string literal")
		}
	}
	return 0, fmt.Errorf("unterminated string literal")
}

// scanField scans a replacement field whose first rune after the "{" is
// r[i].  It returns where the field ends (the index of its closing "}") and
// where its "=", "!" and ":" markers are.
func scanField(r []rune, i int) (fieldScan, error) {
	fs := fieldScan{end: -1, eq: -1, conv: -1, spec: -1}
	depth := 0
	for j := i; j < len(r); j++ {
		switch c := r[j]; c {
		case '\'', '"':
			e, err := skipString(r, j)
			if err != nil {
				return fs, err
			}
			j = e
		case '#':
			for j+1 < len(r) && r[j+1] != '\n' {
				j++
			}
		case '(', '[', '{':
			depth++
		case ')', ']':
			depth--
		case '}':
			if depth > 0 {
				depth--
				continue
			}
			fs.end = j
			return fs, nil
		case '!':
			if j+1 < len(r) && r[j+1] == '=' {
				j++
			} else if depth == 0 && fs.conv < 0 && fs.spec < 0 {
				fs.conv = j
			}
		case ':':
			if depth != 0 || fs.spec >= 0 {
				continue
			}
			fs.spec = j
			for j++; j < len(r); j++ {
				if r[j] == '{' {
					sub, err := scanField(r, j+1)
					if err != nil {
						return fs, err
					}
					j = sub.end
				} else if r[j] == '}' {
					fs.end = j
					return fs, nil
				}
			}
		case '=':
			if j+1 < len(r) && r[j+1] == '=' {
				j++
				continue
			}
			if depth == 0 && fs.eq < 0 && fs.conv < 0 && fs.spec < 0 && j > i && !strings.ContainsRune("<>=!", r[j-1]) {
				fs.eq = j
			}
		}
	}
	return fs, fmt.Errorf("f-string: expecting '}'")
}

// ParseFString splits the raw text of an f-string or t-string literal into
// literal parts and replacement fields.
func ParseFString(text string) ([]FStringPart, error) {
	var parts []FStringPart
	var lit strings.Builder

	r := []rune(text)
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case c == '{' && i+1 < len(r) && r[i+1] == '{', c == '}' && i+1 < len(r) && r[i+1] == '}':
			lit.WriteRune(c)
			i += 2
		case c == '{':
			fs, err := scanField(r, i+1)
			if err != nil {
				return nil, err
			}
			field, err := buildField(r, i+1, fs)
			if err != nil {
				return nil, err
			}
			if lit.Len() > 0 {
				parts = append(parts, FStringPart{Text: lit.String()})
				lit.Reset()
			}
			parts = append(parts, field)
			i = fs.end + 1
		case c == '}':
			return nil, fmt.Errorf("f-string: single '}' is not allowed")
		default:
			lit.WriteRune(c)
			i++
		}
	}
	if lit.Len() > 0 {
		parts = append(parts, FStringPart{Text: lit.String()})
	}
	return parts, nil
}

// buildField turns the scan of the field starting at r[start] into a part.
func buildField(r []rune, start int, fs fieldScan) (FStringPart, error) {
	field := FStringPart{Field: true}
	exprEnd := fs.end
	for _, m := range []int{fs.spec, fs.conv, fs.eq} {
		if m >= 0 && m < exprEnd {
			exprEnd = m
		}
	}
	if fs.eq >= 0 {
		// The source text is echoed back EXACTLY as written, padding and
		// all: f"{ x = }" renders " x = 5".
		field.Debug = true
		t := fs.eq + 1
		for t < len(r) && (r[t] == ' ' || r[t] == '\t') {
			t++
		}
		field.DebugText = string(r[start:fs.eq+1]) + string(r[fs.eq+1:t])
	}
	field.Expr = string(r[start:exprEnd])
	field.Offset = start
	if strings.TrimSpace(stripComments(field.Expr)) == "" {
		return field, fmt.Errorf("f-string: valid expression required before '%s'", string(r[exprEnd]))
	}
	if fs.spec >= 0 && strings.HasPrefix(strings.TrimSpace(field.Expr), "lambda") {
		return field, fmt.Errorf("f-string: lambda expressions are not allowed without parentheses")
	}
	if fs.conv >= 0 {
		if fs.conv+1 >= len(r) || fs.conv+1 >= fs.end {
			return field, fmt.Errorf("f-string: missing conversion character")
		}
		field.Conversion = byte(r[fs.conv+1])
		if r[fs.conv+1] > 127 || !strings.ContainsRune("rsa", r[fs.conv+1]) {
			return field, fmt.Errorf("f-string: invalid conversion character '%s': expected 's', 'r', or 'a'", string(r[fs.conv+1]))
		}
		if n := fs.conv + 2; n < fs.end && r[n] != ':' {
			return field, fmt.Errorf("f-string: expecting ':' or '}'")
		}
	}
	if fs.spec >= 0 {
		field.FormatSpec = string(r[fs.spec+1 : fs.end])
	}
	return field, nil
}

// stripComments drops "# ..." comments (not those inside strings) from an
// expression, for the emptiness check only.
func stripComments(s string) string {
	r := []rune(s)
	var out strings.Builder
	for j := 0; j < len(r); j++ {
		switch r[j] {
		case '\'', '"':
			e, err := skipString(r, j)
			if err != nil {
				return s
			}
			out.WriteString(string(r[j : e+1]))
			j = e
		case '#':
			for j+1 < len(r) && r[j+1] != '\n' {
				j++
			}
		default:
			out.WriteRune(r[j])
		}
	}
	return out.String()
}

// FStringOpen reports whether text, the start of an f-string body, ends
// inside a replacement field.  The lexer uses it to decide whether a quote
// or a newline belongs to the field (PEP 701) or ends the string.
func FStringOpen(text string) bool {
	if !strings.Contains(text, "{") {
		return false
	}
	r := []rune(text)
	for i := 0; i < len(r); i++ {
		switch {
		case r[i] == '{' && i+1 < len(r) && r[i+1] == '{':
			i++
		case r[i] == '{':
			fs, err := scanField(r, i+1)
			if err != nil {
				return true
			}
			i = fs.end
		}
	}
	return false
}

// ParseExpr parses the field's expression.  It is parenthesised so that a
// multi-line expression, or one that ends in a comment, parses (PEP 701).
func (p FStringPart) ParseExpr() (Expr, error) {
	return ParseExpr("(" + p.Expr + "\n)")
}
