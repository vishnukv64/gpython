// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package concurrent provides python's 'concurrent' package.
//
// It exists so that "import concurrent" and "import concurrent.futures"
// both resolve.  All of the behaviour is in the futures submodule.
package concurrent

import (
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `concurrent package: the futures submodule provides thread-pool executors.`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "concurrent",
			Doc:  module_doc,
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "__doc__", Value: py.String(module_doc)},
			py.DictEntry{Key: "__package__", Value: py.String("concurrent")},
		),
	})

	// concurrent.futures is a module of its own; "from concurrent.futures
	// import ThreadPoolExecutor" resolves through this package.
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "concurrent.futures.thread",
			Doc:  "Implementation of ThreadPoolExecutor",
		},
		Globals: py.NewStringDict(),
	})
}
