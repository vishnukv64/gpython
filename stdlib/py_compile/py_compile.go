// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package py_compile provides python's 'py_compile' module.
//
// CPython's py_compile writes a .pyc.  This interpreter has no bytecode cache,
// so compile() checks that the source compiles, reports a failure the way
// CPython does, and returns None - CPython's own "nothing was written" result -
// instead of naming a cache file that does not exist.  Callers that only need
// the module to import (pip's vendored distlib) or the syntax check work.
package py_compile

import (
	"github.com/vishnukv64/gpython/py"
)

const codeSrc = `"""Routine to "compile" a .py file to a .pyc file (a syntax check here)."""

import builtins
import enum
import sys

__all__ = ["compile", "main", "PyCompileError", "PycInvalidationMode"]


class PyCompileError(Exception):
    def __init__(self, exc_type, exc_value, file, msg=''):
        exc_type_name = exc_type.__name__
        if exc_type is SyntaxError:
            tbtext = ''.join(__import__('traceback').format_exception_only(
                exc_type, exc_value))
            errmsg = tbtext.replace('File "<string>"', 'File "%s"' % file)
        else:
            errmsg = "Sorry: %s: %s" % (exc_type_name, exc_value)
        Exception.__init__(self, msg or errmsg, exc_type_name, exc_value, file)
        self.exc_type_name = exc_type_name
        self.exc_value = exc_value
        self.file = file
        self.msg = msg or errmsg

    def __str__(self):
        return self.msg


class PycInvalidationMode(enum.Enum):
    TIMESTAMP = 1
    CHECKED_HASH = 2
    UNCHECKED_HASH = 3


def compile(file, cfile=None, dfile=None, doraise=False, optimize=-1,
            invalidation_mode=None, quiet=0):
    with open(file, 'rb') as f:
        source = f.read()
    try:
        builtins.compile(source, dfile or file, 'exec', dont_inherit=True)
    except Exception as err:
        py_exc = PyCompileError(err.__class__, err, dfile or file)
        if quiet < 2:
            if doraise:
                raise py_exc
            sys.stderr.write(py_exc.msg + '\n')
        return None
    return None


def main():
    rv = 0
    for filename in sys.argv[1:]:
        try:
            compile(filename, doraise=True)
        except PyCompileError as error:
            rv = 1
            sys.stderr.write("%s\n" % error.msg)
        except OSError as error:
            rv = 1
            sys.stderr.write("%s\n" % error)
    return rv
`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:     "py_compile",
			FileDesc: "py_compile.py",
			Doc:      "Routine to compile a .py file to a .pyc file.",
		},
		CodeSrc: codeSrc,
	})
}
