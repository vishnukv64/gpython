// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package copy provides the implementation of python's 'copy' module.
//
// The algorithms are CPython's own (copy.py, embedded as copy.py.txt and
// compiled on first use), so copy() and deepcopy() follow the memo, the
// __copy__/__deepcopy__ hooks and the __reduce_ex__ protocol exactly.
package copy

import (
	_ "embed"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

//go:embed copy.py.txt
var copySrc string

const module_doc = `Generic (shallow and deep) copying operations.

copy(x)        -- Return a shallow copy of x.
deepcopy(x)    -- Return a deep copy of x.
`

// errorType is copy.Error.
var errorType = py.ExceptionType.NewType("copy.Error", "Raised for copy errors.", nil, nil)

var (
	once sync.Once
	mod  *py.Module
	err  error
)

// call runs the named function of the embedded copy.py.
func call(name string, args py.Tuple, kw py.StringDict) (py.Object, error) {
	once.Do(func() {
		f := py.CurrentFrame()
		if f == nil {
			err = py.ExceptionNewf(py.RuntimeError, "no running frame")
			return
		}
		if mod, err = py.RunSrc(f.Context, "", "<copy>", "_gpython_copy"); err != nil {
			return
		}
		mod.Globals.Set("Error", errorType)
		mod.Globals.Set("error", errorType)
		_, err = py.RunSrc(f.Context, copySrc, "<copy>", mod)
	})
	if err != nil {
		return nil, err
	}
	return py.Call(mod.Globals.GetOrNil(name), args, kw)
}

func init() {
	globals := py.NewStringDict()
	globals.Set("Error", errorType)
	globals.Set("error", errorType)
	for _, n := range []struct{ name, doc string }{
		{"copy", "Return a shallow copy of x."},
		{"deepcopy", "Return a deep copy of x."},
		{"replace", "Return a copy of obj with the named fields replaced."},
	} {
		name := n.name
		globals.Set(name, py.MustNewMethod(name, func(_ py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return call(name, args, kw)
		}, 0, n.doc))
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "copy",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
