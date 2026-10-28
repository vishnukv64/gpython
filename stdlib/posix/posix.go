// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package posix provides the raw operating-system module CPython's os is built
// on: "posix" on Unix and "nt" on Windows (the other name does not exist, as in
// CPython).  gpython's os is written in Go, so this module re-exports the
// subset of os that is posix-level rather than duplicating it.
package posix

import (
	"runtime"

	"github.com/vishnukv64/gpython/py"
)

// notPosix are the os names that CPython defines in os.py (or that gpython's
// os adds on top), not in the raw module.
const src = `
import os as _os
_skip = {'path', 'walk', 'makedirs', 'removedirs', 'getenv', 'fsencode',
         'fsdecode', 'PathLike', '_Environ', 'environ', 'sep', 'altsep',
         'pathsep', 'linesep', 'defpath', 'devnull', 'curdir', 'pardir',
         'extsep', 'name', 'fdopen', 'process_cpu_count'}
for _k in dir(_os):
    if not _k.startswith('__') and _k not in _skip:
        globals()[_k] = getattr(_os, _k)
if _os.name == 'nt':
    environ = {k.upper(): v for k, v in _os.environ.items()}
else:
    environ = {_os.fsencode(k): _os.fsencode(v) for k, v in _os.environ.items()}
del _os, _skip, _k
`

func init() {
	name := "posix"
	if runtime.GOOS == "windows" {
		name = "nt"
	}
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: name, Doc: "This module provides access to operating system functionality that is\nstandardized by the C Standard and the POSIX standard."},
		CodeSrc: src,
	})
}
