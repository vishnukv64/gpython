// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compile

import (
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/py"
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
