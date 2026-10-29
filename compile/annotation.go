// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compile

import (
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/parser"
	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/vm"
)

// annotationSource rebuilds the source text of an annotation.
//
// "from __future__ import annotations" (PEP 563) keeps annotations as their
// source text instead of evaluating them, so that a name which exists only
// for a type checker - or a subscription such as Iterable[int] on a typing
// object that cannot be subscripted here - is never a run-time error.
//
// The parser does not record reliable column offsets, so the text is
// reconstructed from the syntax tree rather than sliced out of the source.
// It is equivalent source rather than a byte-for-byte copy, which is what
// matters: the string is only ever evaluated by code that asks for it.
func annotationSource(node ast.Expr) (string, error) {
	var b strings.Builder
	if err := writeAnnotation(&b, node); err != nil {
		return "", err
	}
	return b.String(), nil
}

func writeAnnotation(b *strings.Builder, node ast.Expr) error {
	switch n := node.(type) {
	case *ast.Name:
		b.WriteString(string(n.Id))
	case *ast.Attribute:
		if err := writeAnnotation(b, n.Value); err != nil {
			return err
		}
		b.WriteByte('.')
		b.WriteString(string(n.Attr))
	case *ast.Subscript:
		if err := writeAnnotation(b, n.Value); err != nil {
			return err
		}
		b.WriteByte('[')
		if err := writeSlicer(b, n.Slice); err != nil {
			return err
		}
		b.WriteByte(']')
	case *ast.Tuple:
		b.WriteByte('(')
		for i, elt := range n.Elts {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := writeAnnotation(b, elt); err != nil {
				return err
			}
		}
		if len(n.Elts) == 1 {
			b.WriteByte(',')
		}
		b.WriteByte(')')
	case *ast.List:
		b.WriteByte('[')
		for i, elt := range n.Elts {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := writeAnnotation(b, elt); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case *ast.BinOp:
		// The only operator that appears in an annotation is the "|" of
		// PEP 604 unions, but this handles the rest for completeness.
		if err := writeAnnotation(b, n.Left); err != nil {
			return err
		}
		b.WriteByte(' ')
		b.WriteString(operatorText(n.Op))
		b.WriteByte(' ')
		return writeAnnotation(b, n.Right)
	case *ast.Call:
		if err := writeAnnotation(b, n.Func); err != nil {
			return err
		}
		b.WriteByte('(')
		for i, arg := range n.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			if err := writeAnnotation(b, arg); err != nil {
				return err
			}
		}
		for i, kw := range n.Keywords {
			if i > 0 || len(n.Args) > 0 {
				b.WriteString(", ")
			}
			if kw.Arg != "" {
				b.WriteString(string(kw.Arg))
				b.WriteByte('=')
			} else {
				b.WriteString("**")
			}
			if err := writeAnnotation(b, kw.Value); err != nil {
				return err
			}
		}
		b.WriteByte(')')
	case *ast.Str:
		// A forward reference written as a string stays a string literal in
		// the annotation's text, so it is quoted again.
		quoted, err := py.ReprAsString(n.S)
		if err != nil {
			return err
		}
		b.WriteString(quoted)
	case *ast.Bytes:
		quoted, err := py.ReprAsString(n.S)
		if err != nil {
			return err
		}
		b.WriteString(quoted)
	case *ast.Num:
		text, err := py.ReprAsString(n.N)
		if err != nil {
			return err
		}
		b.WriteString(text)
	case *ast.NameConstant:
		text, err := py.ReprAsString(n.Value)
		if err != nil {
			return err
		}
		b.WriteString(text)
	case *ast.Ellipsis:
		b.WriteString("...")
	default:
		// Rather than write something that would not be valid Python, say so
		// in a form that is still a single string, so callers that only pass
		// the annotation around keep working.
		b.WriteString("typing.Any")
	}
	return nil
}

// writeSlicer writes a subscript's contents, which is either an expression
// or a slice (a[1:2]).
func writeSlicer(b *strings.Builder, node ast.Slicer) error {
	switch n := node.(type) {
	case *ast.Index:
		// The common case: a single subscript such as Iterable[int].
		return writeAnnotation(b, n.Value)
	case ast.Expr:
		return writeAnnotation(b, n)
	case *ast.Slice:
		if n.Lower != nil {
			if err := writeAnnotation(b, n.Lower); err != nil {
				return err
			}
		}
		b.WriteByte(':')
		if n.Upper != nil {
			if err := writeAnnotation(b, n.Upper); err != nil {
				return err
			}
		}
		if n.Step != nil {
			b.WriteByte(':')
			if err := writeAnnotation(b, n.Step); err != nil {
				return err
			}
		}
	}
	return nil
}

func operatorText(op ast.OperatorNumber) string {
	switch op {
	case ast.Add:
		return "+"
	case ast.Sub:
		return "-"
	case ast.Mult:
		return "*"
	case ast.BitOr:
		return "|"
	case ast.Div:
		return "/"
	case ast.FloorDiv:
		return "//"
	case ast.Modulo:
		return "%"
	case ast.Pow:
		return "**"
	case ast.LShift:
		return "<<"
	case ast.RShift:
		return ">>"
	case ast.BitAnd:
		return "&"
	case ast.BitXor:
		return "^"
	}
	return "?" + strconv.Itoa(int(op))
}

// loadAnnotation emits the value for one annotation.
//
// Normally that is the annotation evaluated as an expression, which is what
// Python does without the future import.  With "from __future__ import
// annotations" in effect the annotation's source text is loaded as a string
// instead, so that nothing in it is resolved at run time.
func (c *compiler) loadAnnotation(node ast.Expr) error {
	if !c.stringAnnotations {
		c.Expr(node)
		return nil
	}
	text, err := annotationSource(node)
	if err != nil {
		return err
	}
	c.LoadConst(py.String(text))
	return nil
}

// inAnnotationScope reports whether an AnnAssign in this scope records its
// annotation in __annotations__.  PEP 526 gives module and class bodies an
// __annotations__ dict; a function body's annotations on locals are not
// collected (only parameter and return annotations are, into the function's
// own __annotations__).
func (c *compiler) inAnnotationScope() bool {
	return c.scopeType == compilerScopeModule || c.scopeType == compilerScopeClass
}

// annotationsInitialised records, per code object being compiled, that
// __annotations__ has already been created, so a body with several annotated
// names does not rebuild (and thereby reset) it.
//
// The compiler emits "if '__annotations__' not in locals(): __annotations__ =
// {}" the first time a given scope needs it, then indexes into it.  This is
// done with existing opcodes rather than CPython's SETUP_ANNOTATIONS, so no
// new opcode is introduced.
func (c *compiler) ensureAnnotations() {
	if c.annotationsDone {
		return
	}
	c.annotationsDone = true
	// __annotations__ = {}
	c.OpArg(vm.BUILD_MAP, 0)
	c.NameOp("__annotations__", ast.Store)
}

func init() {
	py.CaretShapeOf = caretShapeOf
	py.FullLineCall = fullLineCall
}

// caretShapeOf parses "(\n"+segment+"\n)" for py's caret anchors.
func caretShapeOf(segment string) (py.CaretShape, bool) {
	tree, err := parser.Parse(strings.NewReader("(\n"+segment+"\n)\n"), "<segment>", py.ExecMode)
	if err != nil {
		return py.CaretShape{}, false
	}
	mod, ok := tree.(*ast.Module)
	if !ok || len(mod.Body) != 1 {
		return py.CaretShape{}, false
	}
	stmt, ok := mod.Body[0].(*ast.ExprStmt)
	if !ok {
		return py.CaretShape{}, false
	}
	end := func(e ast.Ast) (int, int) { return e.GetEndLineno(), e.GetEndColOffset() }
	var sh py.CaretShape
	switch e := stmt.Value.(type) {
	case *ast.BinOp:
		sh.Kind = 1
		sh.LeftEndLine, sh.LeftEndCol = end(e.Left)
		sh.RightStartLine, sh.RightStartCol = e.Right.GetLineno(), e.Right.GetColOffset()
	case *ast.Subscript:
		sh.Kind = 2
		sh.LeftEndLine, sh.LeftEndCol = end(e.Value)
	case *ast.Call:
		sh.Kind = 3
		sh.LeftEndLine, sh.LeftEndCol = end(e.Func)
	default:
		return py.CaretShape{}, false
	}
	sh.EndLine, sh.EndCol = end(stmt.Value)
	return sh, sh.LeftEndLine != 0 && sh.EndLine != 0
}

// fullLineCall reports the position of a "return f(...)" / "x = f(...)" call.
func fullLineCall(src string) (int, int, int, int, bool) {
	tree, err := parser.Parse(strings.NewReader(src+"\n"), "<segment>", py.ExecMode)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	mod, ok := tree.(*ast.Module)
	if !ok || len(mod.Body) == 0 {
		return 0, 0, 0, 0, false
	}
	var value ast.Expr
	switch s := mod.Body[0].(type) {
	case *ast.Return:
		value = s.Value
	case *ast.Assign:
		if _, ok := s.Value.(*ast.Call); ok && len(s.Targets) == 1 {
			if _, ok := s.Targets[0].(*ast.Name); ok {
				value = s.Value
			}
		}
	}
	call, ok := value.(*ast.Call)
	if !ok {
		return 0, 0, 0, 0, false
	}
	if _, ok := call.Func.(*ast.Name); !ok {
		return 0, 0, 0, 0, false
	}
	return call.GetLineno(), call.GetEndLineno(), call.GetColOffset(), call.GetEndColOffset(), true
}
