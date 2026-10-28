// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssl

import (
	_ "embed"

	"github.com/vishnukv64/gpython/py"
)

// ssl.py is gpython's own Python half of ssl, written over _ssl.  It lives
// here rather than in stdlib/pylib/lib, which holds CPython's Lib verbatim.
//
//go:embed ssl.py
var sslPySrc string

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:     "ssl",
			FileDesc: "ssl.py",
			Doc:      "TLS/SSL wrapper for socket objects.",
		},
		CodeSrc: sslPySrc,
		PreSetGlobals: func(g py.StringDict) {
			g.Set("__spec__", py.NewModuleSpec("ssl", py.String("ssl.py"), false))
		},
	})
}
