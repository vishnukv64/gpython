// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package compileall provides python's 'compileall' module.
//
// CPython's compileall exists to write .pyc files.  This interpreter has no
// bytecode cache, so there is nothing to write: every function here accepts
// CPython's signature and reports False, "no bytecode was produced".  pip
// byte-compiles each installed wheel through compile_file and only records a
// .pyc when it returns True, so a False return lets "pip install" finish
// without claiming files that do not exist.
//
// ponytail: no syntax checking either; add it if a caller relies on
// compile_dir to report broken sources.
package compileall

import (
	"github.com/vishnukv64/gpython/py"
)

const codeSrc = `"""Module/script to byte-compile all .py files to .pyc files.

gpython keeps no bytecode cache: the functions below accept CPython's
arguments and return False, meaning no bytecode was written.
"""

__all__ = ["compile_dir", "compile_file", "compile_path"]


def compile_dir(dir, maxlevels=None, ddir=None, force=False, rx=None,
                quiet=0, legacy=False, optimize=-1, workers=1,
                invalidation_mode=None, *, stripdir=None, prependdir=None,
                limit_sl_dest=None, hardlink_dupes=False):
    return False


def compile_file(fullname, ddir=None, force=False, rx=None, quiet=0,
                 legacy=False, optimize=-1, invalidation_mode=None, *,
                 stripdir=None, prependdir=None, limit_sl_dest=None,
                 hardlink_dupes=False):
    return False


def compile_path(skip_curdir=1, maxlevels=0, force=False, quiet=0,
                 legacy=False, optimize=-1, invalidation_mode=None):
    return False
`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:     "compileall",
			FileDesc: "compileall.py",
			Doc:      "Module/script to byte-compile all .py files to .pyc files.",
		},
		CodeSrc: codeSrc,
	})
}
