// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mpcore

import "github.com/vishnukv64/gpython/py"

func register(globals py.StringDict) {
	globals.Set("flags", py.NewStringDictFrom(
		py.DictEntry{Key: "HAVE_SEM_OPEN", Value: py.Int(1)},
		py.DictEntry{Key: "HAVE_SEM_TIMEDWAIT", Value: py.Int(1)},
	))
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "_multiprocessing", Doc: "Support for POSIX-style semaphores and subprocess spawning."},
		Globals: globals,
	})
}
