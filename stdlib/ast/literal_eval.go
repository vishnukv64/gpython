// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ast

import (
	pyast "github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/parser"
	"github.com/vishnukv64/gpython/py"
)

// literal_eval safely evaluates an expression node or a string containing a
// Python literal or a container display, in the same terms CPython's
// ast.literal_eval does: a walk over the parsed tree accepting only the
// literal structures, so no name is ever looked up and no code is run.
//
// The tree is the compiler's own AST (the "ast" package, not this Python-level
// module), and the accepted node kinds mirror CPython's _convert exactly.
func literal_eval(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var nodeOrString py.Object
	if err := py.UnpackTuple(args, kwargs, "literal_eval", 1, 1, &nodeOrString); err != nil {
		return nil, err
	}

	node := nodeOrString
	if s, ok := nodeOrString.(py.String); ok {
		// CPython strips leading spaces before parsing so that an indented
		// fragment is accepted; anything that is not an expression is a
		// SyntaxError, which ParseString raises.
		parsed, err := parseExpression(string(s))
		if err != nil {
			return nil, err
		}
		node = parsed
	}

	astNode, ok := astObjectToNode(node)
	if !ok {
		return nil, py.ExceptionNewf(py.ValueError, "malformed node or string: %s", reprOf(node))
	}
	if expr, ok := astNode.(*pyast.Expression); ok {
		astNode = expr.Body
	}
	exprNode, ok := astNode.(pyast.Expr)
	if !ok {
		return nil, py.ExceptionNewf(py.ValueError, "malformed node or string: %s", reprOf(node))
	}
	return convert(exprNode)
}

// parseExpression parses text as an eval-mode expression.
func parseExpression(text string) (py.Object, error) {
	in, err := parser.ParseString(stripLeadingSpace(text), py.EvalMode)
	if err != nil {
		return nil, err
	}
	return nodeAsObject(in), nil
}

// stripLeadingSpace removes leading spaces and tabs, which CPython does with
// node_or_string.lstrip(" \t") before parsing.
func stripLeadingSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:]
}

// astObjectToNode recovers a compiler AST node from the py.Object that ast.parse
// or a caller handed us.
func astObjectToNode(o py.Object) (pyast.Ast, bool) {
	if node, ok := o.(pyast.Ast); ok {
		return node, true
	}
	return nil, false
}

// convert is CPython's _convert: it accepts only literal node kinds and raises
// ValueError for anything else.
func convert(node pyast.Expr) (py.Object, error) {
	switch n := node.(type) {
	case *pyast.Num:
		// The compiler's Num covers every numeric literal.
		return n.N, nil
	case *pyast.Str:
		return py.String(n.S), nil
	case *pyast.Bytes:
		return py.Bytes(n.S), nil
	case *pyast.NameConstant:
		return py.Object(n.Value), nil
	case *pyast.Ellipsis:
		return py.Ellipsis, nil
	case *pyast.Tuple:
		items := make([]py.Object, len(n.Elts))
		for i, elt := range n.Elts {
			v, err := convert(elt)
			if err != nil {
				return nil, err
			}
			items[i] = v
		}
		return py.Tuple(items), nil
	case *pyast.List:
		items := make([]py.Object, len(n.Elts))
		for i, elt := range n.Elts {
			v, err := convert(elt)
			if err != nil {
				return nil, err
			}
			items[i] = v
		}
		return py.NewListFromItems(items), nil
	case *pyast.Set:
		items := make([]py.Object, len(n.Elts))
		for i, elt := range n.Elts {
			v, err := convert(elt)
			if err != nil {
				return nil, err
			}
			items[i] = v
		}
		return py.NewSetFromItemsErr(items)
	case *pyast.Dict:
		if len(n.Keys) != len(n.Values) {
			return nil, malformed(node)
		}
		return convertDict(n)
	case *pyast.Call:
		// CPython accepts exactly "set()" as the empty set.
		if name, ok := n.Func.(*pyast.Name); ok && string(name.Id) == "set" &&
			len(n.Args) == 0 && len(n.Keywords) == 0 && n.Starargs == nil && n.Kwargs == nil {
			return py.NewSet(), nil
		}
	case *pyast.UnaryOp:
		return convertSignedNum(n)
	case *pyast.BinOp:
		// CPython's special case: int/float +/- complex, for literals such as
		// "1+2j".  Any other operator is not a literal structure.
		if n.Op == pyast.Add || n.Op == pyast.Sub {
			left, err := convertSignedNum(n.Left)
			if err != nil {
				return nil, err
			}
			right, err := convertNum(n.Right)
			if err != nil {
				return nil, err
			}
			if n.Op == pyast.Add {
				return py.Add(left, right)
			}
			return py.Sub(left, right)
		}
	}
	return nil, malformed(node)
}

// convertDict builds the dict for a literal display.  String keys - which is
// what pip and its vendored packages pass to literal_eval - go through the
// general dict machinery too, so that a key of any hashable literal type works.
func convertDict(n *pyast.Dict) (py.Object, error) {
	d := py.NewStringDict()
	for i := range n.Keys {
		k, err := convert(n.Keys[i])
		if err != nil {
			return nil, err
		}
		v, err := convert(n.Values[i])
		if err != nil {
			return nil, err
		}
		ks, err := py.StrAsString(k)
		if err != nil {
			return nil, py.ExceptionNewf(py.ValueError, "malformed node or string: %s", reprOf(k))
		}
		(&d).Set(ks, v)
	}
	return d, nil
}

// convertNum is CPython's _convert_num: a number literal, nothing else.
func convertNum(node pyast.Expr) (py.Object, error) {
	n, ok := node.(*pyast.Num)
	if !ok {
		return nil, malformed(node)
	}
	return n.N, nil
}

// convertSignedNum is CPython's _convert_signed_num: a number, optionally
// behind a unary plus or minus.
func convertSignedNum(node pyast.Expr) (py.Object, error) {
	if u, ok := node.(*pyast.UnaryOp); ok {
		if u.Op == pyast.UAdd || u.Op == pyast.USub {
			operand, err := convertNum(u.Operand)
			if err != nil {
				return nil, err
			}
			if u.Op == pyast.UAdd {
				return py.Pos(operand)
			}
			return py.Neg(operand)
		}
	}
	return convertNum(node)
}

func malformed(node pyast.Ast) error {
	return py.ExceptionNewf(py.ValueError, "malformed node or string: %s", pyast.Dump(node))
}

// reprOf renders a value the way CPython's f-string "{node!r}" would in the
// malformed-node message.
func reprOf(o py.Object) string {
	if s, err := py.ReprAsString(o); err == nil {
		return s
	}
	return o.Type().Name
}
