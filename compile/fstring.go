// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compile

import (
	"bytes"
	"strings"

	"github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/parser"
	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/vm"
)

// fstringPart is one piece of an f-string: either a literal or a
// replacement field.
type fstringPart struct {
	// literal text (used when field is false)
	text string
	// field parts
	field      bool
	expr       string
	conversion byte // 0, 'r' or 'a'
	formatSpec string
}

// parseFString splits the raw text of an f-string literal into literal
// parts and replacement fields.
//
// Limitations: a replacement field may not itself contain an f-string, and
// a `!` or `:` inside the field is taken as the conversion/format separator
// unless it appears inside a string, a bracket or a nested call.  Nested
// f-strings are reported as a SyntaxError rather than silently mis-parsed.
func parseFString(text string) ([]fstringPart, error) {
	var parts []fstringPart
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
				parts = append(parts, fstringPart{text: lit.String()})
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
		parts = append(parts, fstringPart{text: lit.String()})
	}
	return parts, nil
}

// parseField splits a replacement field ("expr[!conv][:spec]") into its parts.
func parseField(body string) (fstringPart, error) {
	field := fstringPart{field: true}

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
		field.conversion = byte(runes[conv+1])
		if field.conversion != 'r' && field.conversion != 's' && field.conversion != 'a' {
			return field, py.ExceptionNewf(py.SyntaxError, "f-string: invalid conversion character %q", string(field.conversion))
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
		field.formatSpec = string(runes[start+1:])
	}

	field.expr = strings.TrimSpace(string(runes[:end]))
	if field.expr == "" {
		return field, py.ExceptionNewf(py.SyntaxError, "f-string: empty expression not allowed")
	}
	return field, nil
}

// compileFString lowers an f-string node into ordinary bytecode.
//
// Each literal becomes a constant; each field evaluates its expression, is
// converted (str by default, repr for !r, ascii for !a) and, if a format
// specifier is present, is passed through the format() builtin.  The parts
// are then concatenated.
func (c *compiler) compileFString(node *ast.FString) {
	parts, err := parseFString(node.Text)
	if err != nil {
		c.panicSyntaxErrorf(node, "f-string: %v", err)
		return
	}

	// An f-string with a single literal part is just that constant.
	if len(parts) == 1 && !parts[0].field {
		c.LoadConst(py.String(unescapePart(parts[0].text, node.Raw)))
		return
	}
	if len(parts) == 0 {
		c.LoadConst(py.String(""))
		return
	}

	for i, part := range parts {
		if !part.field {
			c.LoadConst(py.String(unescapePart(part.text, node.Raw)))
		} else {
			expr, err := ast.ParseExpr(part.expr)
			if err != nil {
				c.panicSyntaxErrorf(node, "f-string: %v", err)
				return
			}
			c.Expr(expr)

			// Order matches CPython: the conversion is applied first, then
			// format() gets the original value (or the converted one), and
			// str() is only used when there is no format specifier at all.
			switch part.conversion {
			case 'r':
				c.callBuiltin1("repr")
			case 'a':
				c.callBuiltin1("ascii")
			case 's':
				c.callBuiltin1("str")
			}

			if part.formatSpec != "" {
				c.callFormat(part.formatSpec)
			} else if part.conversion == 0 {
				c.callBuiltin1("str")
			}
		}

		// Concatenate with what has been built so far.  This has to come
		// after the part is pushed so that both operands are on the stack.
		if i > 0 {
			c.Op(vm.BINARY_ADD)
		}
	}
}

// callBuiltin1 rewrites the value on the stack through the named one-argument
// builtin, loading that builtin by name from the enclosing scope.
func (c *compiler) callBuiltin1(name string) {
	c.NameOp(name, ast.Load)
	// stack is now [value, fn]; swap so the call sees [fn, value]
	c.Op(vm.ROT_TWO)
	c.OpArg(vm.CALL_FUNCTION, 1)
}

// callFormat passes the value on the stack through format(value, spec).
func (c *compiler) callFormat(spec string) {
	c.NameOp("format", ast.Load)
	// stack is [value, fn]; swap to [fn, value] so the value is the first
	// argument of the call, then add the spec on top.
	c.Op(vm.ROT_TWO)
	c.LoadConst(py.String(spec))
	c.OpArg(vm.CALL_FUNCTION, 2)
}

// unescapePart processes the escape sequences of a literal part, unless the
// literal was raw.
func unescapePart(text string, raw bool) string {
	if raw {
		return text
	}
	out, err := parser.DecodeEscape(bytes.NewBufferString(text), false)
	if err != nil {
		return text
	}
	return out.String()
}
