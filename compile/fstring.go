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
	if node.Template {
		c.compileTString(node)
		return
	}
	parts, err := ast.ParseFString(node.Text)
	if err != nil {
		c.panicSyntaxErrorf(node, "%s", fstringMsg(err))
		return
	}
	c.fstringParts(node, parts, []rune(node.Text), true)
}

// fstringParts pushes the concatenation of parts.
func (c *compiler) fstringParts(node *ast.FString, parts []ast.FStringPart, nodeText []rune, top bool) {
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
			// Parenthesised so that a multi-line expression, or one ending in a
			// comment, parses (PEP 701).
			expr, err := part.ParseExpr()
			if err != nil {
				c.panicSyntaxErrorf(node, "%s", fstringMsg(err))
				return
			}
			placeFieldExpr(node, nodeText, part.Offset, expr, top)
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
					c.panicSyntaxErrorf(node, "%s", fstringMsg(err))
					return
				}
				c.NameOp("format", ast.Load)
				c.Op(vm.ROT_TWO)
				c.fstringParts(node, specParts, []rune(part.FormatSpec), false)
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

// fstringMsg is the SyntaxError message for an error from parsing an
// f-string or one of its fields.
func fstringMsg(err error) string {
	if e, ok := err.(*py.Exception); ok {
		if args, ok := e.Args.(py.Tuple); ok && len(args) > 0 {
			if msg, ok := args[0].(py.String); ok {
				if msg == "unexpected EOF while parsing" {
					return "f-string: expecting '=', or '!', or ':', or '}'"
				}
				return string(msg)
			}
		}
	}
	return err.Error()
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
		case *ast.ListComp, *ast.SetComp, *ast.DictComp, *ast.GeneratorExp, *ast.Lambda:
			found = true
			return false
		}
		return !found
	})
	return found
}

// compileTString lowers a t-string (PEP 750) to
// string._gpy_template((literals...), (_gpy_interp(value, expr, conv, spec)...)).
// A debug field "{x=}" folds its source text into the literal before it.
func (c *compiler) compileTString(node *ast.FString) {
	parts, err := ast.ParseFString(node.Text)
	if err != nil {
		c.panicSyntaxErrorf(node, "%s", strings.Replace(fstringMsg(err), "f-string", "t-string", 1))
		return
	}
	pos := ast.ExprBase{Pos: node.Pos}
	str := func(s string) ast.Expr { return &ast.Str{ExprBase: pos, S: py.String(s)} }
	call := func(fn string, args ...ast.Expr) ast.Expr {
		imp := &ast.Call{ExprBase: pos, Func: &ast.Name{ExprBase: pos, Id: "__import__", Ctx: ast.Load}, Args: []ast.Expr{str("string")}}
		return &ast.Call{ExprBase: pos, Func: &ast.Attribute{ExprBase: pos, Value: imp, Attr: ast.Identifier(fn), Ctx: ast.Load}, Args: args}
	}
	var lits, interps []ast.Expr
	cur := ""
	for _, part := range parts {
		if !part.Field {
			cur += unescapePart(part.Text, node.Raw)
			continue
		}
		expr, err := part.ParseExpr()
		if err != nil {
			c.panicSyntaxErrorf(node, "%s", strings.Replace(fstringMsg(err), "f-string", "t-string", 1))
			return
		}
		placeFieldExpr(node, []rune(node.Text), part.Offset, expr, true)
		if containsComprehension(expr) {
			if table, tableErr := symtable.NewSymTable(&ast.Expression{Body: expr}, c.Filename); tableErr == nil {
				c.SymTable.AdoptChildren(table)
			}
		}
		if part.Debug {
			cur += part.DebugText
		}
		lits = append(lits, str(cur))
		cur = ""
		var conv ast.Expr = &ast.NameConstant{ExprBase: pos, Value: py.None}
		switch {
		case part.Conversion != 0:
			conv = str(string(part.Conversion))
		case part.Debug && part.FormatSpec == "":
			conv = str("r")
		}
		var spec ast.Expr = str(part.FormatSpec)
		if strings.Contains(part.FormatSpec, "{") {
			spec = &ast.FString{ExprBase: pos, Text: part.FormatSpec, Raw: node.Raw}
		}
		interps = append(interps, call("_gpy_interp", expr, str(strings.TrimRight(part.Expr, " \t\r\n\f\v")), conv, spec))
	}
	lits = append(lits, str(cur))
	c.Expr(call("_gpy_template", &ast.Tuple{ExprBase: pos, Elts: lits, Ctx: ast.Load}, &ast.Tuple{ExprBase: pos, Elts: interps, Ctx: ast.Load}))
}

// placeFieldExpr moves the positions of a field's expression, which was
// parsed on its own as "(" + expr + "\n)", to where it sits in the source.
// top is false for a field nested in a format spec, and a literal that was
// concatenated from several has no known layout: those get the literal's
// own line and no column range.
func placeFieldExpr(node *ast.FString, text []rune, offset int, expr ast.Expr, top bool) {
	if !node.Located || !top {
		ast.Walk(expr, func(n ast.Ast) bool {
			n.SetPosition(ast.Pos{Lineno: node.Lineno, ColOffset: node.ColOffset})
			return true
		})
		return
	}
	prefix := string(text[:offset])
	baseLine := node.Lineno + strings.Count(prefix, "\n")
	baseCol := node.ColOffset + node.BodyCol + len(prefix)
	if i := strings.LastIndex(prefix, "\n"); i >= 0 {
		baseCol = len(prefix) - i - 1
	}
	move := func(line, col int) (int, int) {
		if line == 1 {
			return baseLine, baseCol + col - 1
		}
		return baseLine + line - 1, col
	}
	ast.Walk(expr, func(n ast.Ast) bool {
		p := ast.Pos{Lineno: n.GetLineno(), ColOffset: n.GetColOffset(), EndLineno: n.GetEndLineno(), EndColOffset: n.GetEndColOffset()}
		if p.Lineno > 0 {
			p.Lineno, p.ColOffset = move(p.Lineno, p.ColOffset)
		}
		if p.EndLineno > 0 {
			p.EndLineno, p.EndColOffset = move(p.EndLineno, p.EndColOffset)
		}
		n.SetPosition(p)
		return true
	})
}
