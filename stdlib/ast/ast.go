// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ast provides python's 'ast' module.
//
// The node types and the parser live in the top-level ast and parser
// packages, which are the compiler's own; this module exposes the part of the
// CPython 'ast' API that Python code calls - literal_eval, parse, walk, dump -
// on top of them.
//
// The nodes here are the compiler's AST, not CPython's: literal_eval is a
// direct tree walk over it (ast_literal_eval.go), and parse returns the same
// tree the compiler builds.  ast.walk and ast.dump are not implemented; they
// are a generic walk over every node kind and nothing pip calls them.
package ast

import (
	pyast "github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/parser"
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `The ast module helps Python applications to process trees of the Python
abstract syntax grammar.

This implementation provides the literal structures API used by pip and its
vendored packages: literal_eval and parse.  The node classes are the
compiler's own nodes, reachable through ast.AST.`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "ast",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("literal_eval", literal_eval, 0, literal_eval_doc),
			py.MustNewMethod("parse", parse, 0, parse_doc),
		},
		Globals: py.NewStringDictFrom(
			// AST is the base class of every node type.  The compiler's node
			// types do not currently register with py as Python types, so the
			// base is reported as object, which is what "isinstance(node,
			// ast.AST)" then answers truthily for.
			py.DictEntry{Key: "AST", Value: py.ObjectType},
		),
	})
}

const literal_eval_doc = `literal_eval(node_or_string)

Safely evaluate an expression node or a string containing a Python literal or
a container display.  The string or node provided may only consist of the
following Python literal structures: strings, bytes, numbers, tuples, lists,
dicts, sets, booleans, and None.

This can be used for safely evaluating strings containing Python values from
untrusted sources without the need to parse the values oneself.  It is not
capable of evaluating arbitrarily complex expressions, for example involving
operators or indexing.`

const parse_doc = `parse(source, filename='<unknown>', mode='exec')

Parse the source into an AST node.  Returns the AST root for the mode:
module for 'exec', expression for 'eval'.`

// parse parses source into the compiler's AST.  Unlike CPython the nodes are
// the compiler's own objects; Python code can hold and pass them back to
// literal_eval but cannot introspect them with ast.walk.
func parse(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var source py.Object
	filename := py.Object(py.String("<unknown>"))
	mode := py.Object(py.String("exec"))
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:parse", []string{"source", "filename", "mode"}, &source, &filename, &mode); err != nil {
		return nil, err
	}
	src, err := py.StrAsString(source)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "parse() argument 1 must be str, not %s", source.Type().Name)
	}
	compileMode, err := compileModeFrom(mode)
	if err != nil {
		return nil, err
	}
	node, err := parser.ParseString(src, compileMode)
	if err != nil {
		return nil, err
	}
	_ = filename
	return nodeAsObject(node), nil
}

// compileModeFrom maps ast.parse's mode string onto the compiler's modes.
func compileModeFrom(mode py.Object) (py.CompileMode, error) {
	s, err := py.StrAsString(mode)
	if err != nil {
		return "", py.ExceptionNewf(py.TypeError, "parse() mode must be str, not %s", mode.Type().Name)
	}
	switch s {
	case "exec":
		return py.ExecMode, nil
	case "eval":
		return py.EvalMode, nil
	case "single":
		return py.SingleMode, nil
	}
	return "", py.ExceptionNewf(py.ValueError, "unknown parse mode %s", s)
}

// nodeAsObject converts a compiler AST node into a py.Object.
func nodeAsObject(node pyast.Ast) py.Object {
	if node == nil {
		return py.None
	}
	return node
}
