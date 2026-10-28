// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ast

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// FStringPart is one piece of an f-string: either a literal or a
// replacement field.
type FStringPart struct {
	// Text is the literal text (used when Field is false)
	Text string
	// field parts
	Field      bool
	Expr       string
	Conversion byte // 0, 'r', 's' or 'a'
	FormatSpec string
	// debug is the self-documenting form, f"{x=}": the rendered output is
	// prefixed with the source text of the expression and an "=".
	Debug     bool
	DebugText string
}

// ParseFString splits the raw text of an f-string literal into literal
// parts and replacement fields.
//
// Limitations: a replacement field may not itself contain an f-string, and
// a `!` or `:` inside the field is taken as the conversion/format separator
// unless it appears inside a string, a bracket or a nested call.  Nested
// f-strings are reported as a SyntaxError rather than silently mis-parsed.
func ParseFString(text string) ([]FStringPart, error) {
	var parts []FStringPart
	var lit strings.Builder

	runes := []rune(text)
	i := 0
	for i < len(runes) {
		c := runes[i]
		switch c {
		case '{':
			if i+1 < len(runes) && runes[i+1] == '{' {
				lit.WriteRune('{')
				i += 2
				continue
			}
			// replacement field: find its matching close brace
			depth := 1
			j := i + 1
			var inStr rune // quote char when inside a string literal
			var brackets int
			for ; j < len(runes); j++ {
				r := runes[j]
				if inStr != 0 {
					if r == '\\' {
						j++
						continue
					}
					if r == inStr {
						inStr = 0
					}
					continue
				}
				switch r {
				case '\'', '"':
					inStr = r
				case '(', '[', '{':
					brackets++
				case ')', ']':
					brackets--
				case '}':
					if brackets == 0 {
						depth--
					} else {
						brackets--
					}
				}
				if depth == 0 {
					break
				}
			}
			if j >= len(runes) {
				return nil, py.ExceptionNewf(py.SyntaxError, "f-string: expecting '}'")
			}
			body := string(runes[i+1 : j])

			if lit.Len() > 0 {
				parts = append(parts, FStringPart{Text: lit.String()})
				lit.Reset()
			}
			field, err := parseField(body)
			if err != nil {
				return nil, err
			}
			parts = append(parts, field)
			i = j + 1
		case '}':
			if i+1 < len(runes) && runes[i+1] == '}' {
				lit.WriteRune('}')
				i += 2
				continue
			}
			return nil, py.ExceptionNewf(py.SyntaxError, "f-string: single '}' is not allowed")
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

// parseField splits a replacement field ("expr[!conv][:spec]") into its parts.
func parseField(body string) (FStringPart, error) {
	field := FStringPart{Field: true}

	// f"{x=}" renders as "x=<repr of x>".  The trailing "=" has to be found
	// before the conversion and format scan, because it is the last
	// character of the expression and everything after it is ordinary
	// conversions and format specs.
	// The marker is the first "=" at depth 0 outside a string that is not
	// part of a comparison: it is followed by the conversion, the format
	// spec or the end of the field, and preceded by the end of the
	// expression.  Everything after it is parsed as usual, so "f"{x=:>5}""
	// keeps its format spec.
	{
		runes := []rune(body)
		depth := 0
		var inStr rune
		for i := 0; i < len(runes); i++ {
			r := runes[i]
			if inStr != 0 {
				if r == '\\' {
					i++
					continue
				}
				if r == inStr {
					inStr = 0
				}
				continue
			}
			switch r {
			case '\'', '"':
				inStr = r
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
			case '=':
				if depth != 0 || i == 0 {
					continue
				}
				prev, next := runes[i-1], ' '
				if i+1 < len(runes) {
					next = runes[i+1]
				}
				// "==", "!=", "<=", ">=" are comparisons, not the marker.
				if prev == '=' || prev == '!' || prev == '<' || prev == '>' || next == '=' {
					continue
				}
				field.Debug = true
				// The source text is kept EXACTLY as written, padding and
				// all, because CPython echoes it back: f"{ x = }" renders
				// " x = 5".  The whitespace that followed the "=" is part of
				// it too, but only up to the conversion or the format spec.
				// Only the expression compiled below is trimmed.
				field.DebugText = string(runes[:i]) + "="
				rest := runes[i+1:]
				for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
					field.DebugText += string(rest[0])
					rest = rest[1:]
				}
				body = string(runes[:i]) + string(rest)
				i = len(runes)
			}
		}
	}

	// Find the conversion and format separators at depth 0, outside strings.
	runes := []rune(body)
	depth := 0
	var inStr rune
	conv := -1
	spec := -1
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if inStr != 0 {
			if r == '\\' {
				i++
				continue
			}
			if r == inStr {
				inStr = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			inStr = r
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '!':
			if depth == 0 && conv < 0 && spec < 0 {
				conv = i
			}
		case ':':
			if depth == 0 && spec < 0 {
				spec = i
			}
		}
	}

	end := len(runes)
	if conv >= 0 {
		end = conv
		if conv+1 >= len(runes) {
			return field, py.ExceptionNewf(py.SyntaxError, "f-string: missing conversion character")
		}
		field.Conversion = byte(runes[conv+1])
		if field.Conversion != 'r' && field.Conversion != 's' && field.Conversion != 'a' {
			return field, py.ExceptionNewf(py.SyntaxError, "f-string: invalid conversion character %q", string(field.Conversion))
		}
	}
	if spec >= 0 {
		if conv >= 0 && spec < conv {
			return field, py.ExceptionNewf(py.SyntaxError, "f-string: format specifier before conversion")
		}
		start := spec
		if conv < 0 {
			end = spec
		}
		field.FormatSpec = string(runes[start+1:])
	}

	field.Expr = strings.TrimSpace(string(runes[:end]))
	if field.Expr == "" {
		return field, py.ExceptionNewf(py.SyntaxError, "f-string: empty expression not allowed")
	}
	return field, nil
}
