// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file holds the go generate command to run yacc on the grammar in grammar.y.
// To build y.go:
//      % go generate
//      % go build

//go:generate goyacc -v y.output grammar.y
package parser

import (
	"fmt"
	"strings"

	"github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/py"
)

// init wires up the expression parser the compiler needs to parse the
// expression parts of an f-string.
func init() {
	ast.ParseExpr = func(src string) (ast.Expr, error) {
		tree, err := Parse(strings.NewReader(src), "<fstring>", py.EvalMode)
		if err != nil {
			return nil, err
		}
		expr, ok := tree.(*ast.Expression)
		if !ok {
			return nil, fmt.Errorf("fstring: expression part did not parse as an expression")
		}
		return expr.Body, nil
	}
}
