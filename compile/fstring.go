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
	"github.com/vishnukv64/gpython/symtable"
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
	// debug is the self-documenting form, f"{x=}": the rendered output is
	// prefixed with the source text of the expression and an "=".
	debug     bool
	debugText string
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
				field.debug = true
				// The source text is kept EXACTLY as written, padding and
				// all, because CPython echoes it back: f"{ x = }" renders
				// " x = 5".  The whitespace that followed the "=" is part of
				// it too, but only up to the conversion or the format spec.
				// Only the expression compiled below is trimmed.
				field.debugText = string(runes[:i]) + "="
				rest := runes[i+1:]
				for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
					field.debugText += string(rest[0])
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
			// The self-documenting form prints its own source text first, so
			// the text is pushed and then joined to the value once the value
			// has been produced.
			if part.debug {
				c.LoadConst(py.String(part.debugText))
			}
			expr, err := ast.ParseExpr(part.expr)
			if err != nil {
				c.panicSyntaxErrorf(node, "f-string: %v", err)
				return
			}
			// The expression was parsed on its own, so a comprehension
			// inside it has a symbol table built against a throwaway
			// parent.  Adopting its child scopes into the enclosing table
			// is what lets the compiler find them: f"{' '.join(x for x in
			// y)}" otherwise fails with "No symtable found for scope
			// type 4".
			if containsComprehension(expr) {
				if table, tableErr := symtable.NewSymTable(&ast.Expression{Body: expr}, c.Filename); tableErr == nil {
					c.SymTable.AdoptChildren(table)
				}
			}
			c.Expr(expr)

			// Order matches CPython: the conversion is applied first, then
			// format() gets the original value (or the converted one), and
			// str() is only used when there is no format specifier at all.
			switch {
			case part.conversion == 'r' || (part.debug && part.conversion == 0):
				// f"{x=}" is f"{x=!r}" unless a conversion says otherwise.
				c.callBuiltin1("repr")
			case part.conversion == 0:
				// nothing
			case part.conversion == 'a':
				c.callBuiltin1("ascii")
			case part.conversion == 's':
				c.callBuiltin1("str")
			}

			if part.formatSpec != "" {
				c.callFormat(part.formatSpec)
			} else if part.conversion == 0 && !part.debug {
				// A debug field defaults to repr, not str.
				c.callBuiltin1("str")
			}

			// Join the source text pushed above to the rendered value.
			if part.debug {
				c.Op(vm.BINARY_ADD)
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

// containsComprehension reports whether the expression holds a
// comprehension anywhere, which is the only node that needs a child scope of
// its own.
func containsComprehension(expr ast.Expr) bool {
	found := false
	ast.Walk(expr, func(node ast.Ast) bool {
		switch node.(type) {
		case *ast.ListComp, *ast.SetComp, *ast.DictComp, *ast.GeneratorExp:
			found = true
			return false
		}
		return !found
	})
	return found
}
