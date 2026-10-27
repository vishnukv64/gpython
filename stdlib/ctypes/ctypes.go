// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ctypes provides a small ctypes: fixed-size C value types, arrays and
// Structures laid over a writable buffer.  It exists for
// multiprocessing.sharedctypes (Value, Array), whose objects live in shared
// memory and are built with type.from_buffer(memoryview).
//
// There are no pointers into the process, no shared libraries and no
// callbacks: CDLL, POINTER, cast and friends are absent rather than faked.
package ctypes

import (
	_ "embed"

	"github.com/vishnukv64/gpython/py"
)

//go:embed ctypes.py
var codeSrc string

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:     "ctypes",
			FileDesc: "ctypes.py",
			Doc:      "Fixed-size C types over a buffer (no foreign calls).",
		},
		CodeSrc: codeSrc,
		PreSetGlobals: func(g py.StringDict) {
			g.Set("__spec__", py.NewModuleSpec("ctypes", py.String("ctypes.py"), false))
		},
	})
}
