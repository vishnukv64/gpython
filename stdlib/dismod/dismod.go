// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dismod provides gpython's own dis and _tokenize.  They live here,
// not in stdlib/pylib/lib, which holds CPython's Lib verbatim.
//
// ponytail: dis exposes the names importers touch but cannot disassemble
// (gpython bytecode is not CPython's), and _tokenize lets CPython's
// tokenize.py import while tokenizing raises; port the C tokenizer to make
// tokenize.generate_tokens work.
package dismod

import (
	_ "embed"

	"github.com/vishnukv64/gpython/py"
)

//go:embed dis.py
var disSrc string

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "dis", FileDesc: "dis.py", Doc: "Disassembler (names only in gpython)."},
		CodeSrc: disSrc,
		PreSetGlobals: func(g py.StringDict) {
			g.Set("__spec__", py.NewModuleSpec("dis", py.String("dis.py"), false))
		},
	})
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_tokenize", Doc: "C tokenizer (not available in gpython)."},
		Methods: []*py.Method{
			py.MustNewMethod("TokenizerIter", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				return nil, py.ExceptionNewf(py.NotImplementedError, "tokenize needs the _tokenize C module, which gpython lacks")
			}, 0, "Tokenizer iterator (unsupported in gpython)."),
		},
	})
}
