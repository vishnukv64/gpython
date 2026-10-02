// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package http provides the implementation of Python's 'http' package.
//
// CPython's http is a package with three submodules - http.client,
// http.cookiejar and http.cookies.  This interpreter registers a module by
// its fully dotted name (py.RegisterModule), and "http.client" is exactly the
// name the import machinery looks up, so each submodule is registered here as
// its own module - the same way stdlib/abc registers "collections.abc" under a
// name it does not own.  This file registers the parent "http" package.
package http

import (
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `HTTP modules.

This package collects three submodules:

    http.client    -- HTTP protocol client
    http.cookiejar -- manage a jar of HTTP cookies
    http.cookies   -- parse and format HTTP cookies
`

func init() {
	globals := py.NewStringDict()
	globals.Set("__all__", py.NewListFromItems([]py.Object{
		py.String("client"), py.String("cookiejar"), py.String("cookies"),
	}))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "http",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
