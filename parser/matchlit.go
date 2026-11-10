// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"github.com/vishnukv64/gpython/ast"
	"github.com/vishnukv64/gpython/py"
)

// matchLiteral is the value of a str or bytes literal in a match pattern
// ("case 'x':", "case b'x':", and a mapping-pattern key).  The grammar used
// to assert a str, so a bytes literal panicked the parser.
func matchLiteral(pos ast.Pos, obj interface{}) ast.Expr {
	base := ast.ExprBase{Pos: pos}
	if b, ok := obj.(py.Bytes); ok {
		return &ast.Bytes{ExprBase: base, S: b}
	}
	return &ast.Str{ExprBase: base, S: obj.(py.String)}
}
