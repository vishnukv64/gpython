// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sre is the constants and case helpers of CPython's _sre, which is
// all that re._constants, re._parser and re._compiler (and so sre_parse,
// sre_constants and sre_compile) need to import and to parse or assemble a
// pattern.  There is no compile(): gpython's re is a Go engine, not the SRE
// bytecode machine, so there is nothing for compiled code to run on.
package sre

import (
	"unicode"

	"github.com/vishnukv64/gpython/py"
)

func charArg(args py.Tuple, name string) (rune, error) {
	if len(args) != 1 {
		return 0, py.ExceptionNewf(py.TypeError, "%s() takes exactly one argument (%d given)", name, len(args))
	}
	n, err := py.IndexInt(args[0])
	if err != nil {
		return 0, err
	}
	return rune(n), nil
}

func asciiLower(c rune) rune {
	if 'A' <= c && c <= 'Z' {
		return c + 32
	}
	return c
}

func init() {
	method := func(name string, f func(rune) py.Object) *py.Method {
		return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
			c, err := charArg(args, name)
			if err != nil {
				return nil, err
			}
			return f(c), nil
		}, 0, "")
	}
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_sre", Doc: "Constants and case helpers of the SRE regular expression engine."},
		Methods: []*py.Method{
			method("ascii_iscased", func(c rune) py.Object { return py.NewBool('A' <= c && c <= 'Z' || 'a' <= c && c <= 'z') }),
			method("ascii_tolower", func(c rune) py.Object { return py.Int(asciiLower(c)) }),
			method("unicode_iscased", func(c rune) py.Object {
				return py.NewBool(unicode.ToLower(c) != c || unicode.ToUpper(c) != c)
			}),
			method("unicode_tolower", func(c rune) py.Object { return py.Int(unicode.ToLower(c)) }),
			py.MustNewMethod("getcodesize", func(self py.Object, args py.Tuple) (py.Object, error) { return py.Int(4), nil }, 0, ""),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "MAGIC", Value: py.Int(20230612)},
			py.DictEntry{Key: "CODESIZE", Value: py.Int(4)},
			py.DictEntry{Key: "MAXREPEAT", Value: py.Int(4294967295)},
			py.DictEntry{Key: "MAXGROUPS", Value: py.Int(1073741823)},
		),
	})
}
