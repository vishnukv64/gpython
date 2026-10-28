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

// compileFString lowers an f-string node into ordinary bytecode.
//
// Each literal becomes a constant; each field evaluates its expression, is
// converted (str by default, repr for !r, ascii for !a) and, if a format
// specifier is present, is passed through the format() builtin.  The parts
// are then concatenated.
func (c *compiler) compileFString(node *ast.FString) {
	parts, err := ast.ParseFString(node.Text)
	if err != nil {
		c.panicSyntaxErrorf(node, "f-string: %v", err)
		return
	}
	c.fstringParts(node, parts)
}

// fstringParts pushes the concatenation of parts.
func (c *compiler) fstringParts(node *ast.FString, parts []ast.FStringPart) {
	// An f-string with a single literal part is just that constant.
	if len(parts) == 1 && !parts[0].Field {
		c.LoadConst(py.String(unescapePart(parts[0].Text, node.Raw)))
		return
	}
	if len(parts) == 0 {
		c.LoadConst(py.String(""))
		return
	}

	for i, part := range parts {
		if !part.Field {
			c.LoadConst(py.String(unescapePart(part.Text, node.Raw)))
		} else {
			// The self-documenting form prints its own source text first, so
			// the text is pushed and then joined to the value once the value
			// has been produced.
			if part.Debug {
				c.LoadConst(py.String(part.DebugText))
			}
			expr, err := ast.ParseExpr(part.Expr)
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
			case part.Conversion == 'r' || (part.Debug && part.Conversion == 0):
				// f"{x=}" is f"{x=!r}" unless a conversion says otherwise.
				c.callBuiltin1("repr")
			case part.Conversion == 0:
				// nothing
			case part.Conversion == 'a':
				c.callBuiltin1("ascii")
			case part.Conversion == 's':
				c.callBuiltin1("str")
			}

			if strings.Contains(part.FormatSpec, "{") {
				// A nested spec, f"{x:{w}d}", is itself an f-string.
				specParts, err := ast.ParseFString(part.FormatSpec)
				if err != nil {
					c.panicSyntaxErrorf(node, "f-string: %v", err)
					return
				}
				c.NameOp("format", ast.Load)
				c.Op(vm.ROT_TWO)
				c.fstringParts(node, specParts)
				c.OpArg(vm.CALL_FUNCTION, 2)
			} else if part.FormatSpec != "" {
				c.callFormat(part.FormatSpec)
			} else if part.Conversion == 0 && !part.Debug {
				// A debug field defaults to repr, not str.  format(x, "") is
				// what CPython's FORMAT_VALUE does, so a __format__ the
				// class defines is honoured.
				c.callFormat("")
			}

			// Join the source text pushed above to the rendered value.
			if part.Debug {
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
