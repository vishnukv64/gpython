// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package importlib

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// SourceLoader loads one .py file.  find_spec hands it out, which is what lets
// runpy (and a finder on sys.meta_path, like pip's __pip-runner__.py) ask a
// spec for its code and source.
type SourceLoader struct{ Name, Path string }

// SourceLoaderType is importlib.machinery.SourceFileLoader.
var SourceLoaderType = py.NewType("_frozen_importlib_external.SourceFileLoader", "Loader for a .py source file.")

func (l *SourceLoader) Type() *py.Type { return SourceLoaderType }

func (l *SourceLoader) code() (*py.Code, error) {
	src, err := py.ReadPath(l.Path)
	if err != nil {
		return nil, py.ExceptionNewf(py.ImportError, "%v", err)
	}
	return py.Compile(string(src), l.Path, py.ExecMode, 0, true)
}

// BuiltinImporterType is importlib.machinery.BuiltinImporter.  Like CPython's,
// the class itself is the loader, so its methods are class methods.
var BuiltinImporterType = py.NewType("_frozen_importlib.BuiltinImporter",
	"Meta path import for built-in modules.")

func init() {
	str := func(s string) py.Object { return py.String(s) }
	SourceLoaderType.Dict.Set("name", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return str(self.(*SourceLoader).Name), nil
	}})
	SourceLoaderType.Dict.Set("path", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return str(self.(*SourceLoader).Path), nil
	}})
	SourceLoaderType.Dict.Set("get_filename", py.MustNewMethod("get_filename", func(self py.Object, args py.Tuple) (py.Object, error) {
		return str(self.(*SourceLoader).Path), nil
	}, 0, "Return the path of the source file."))
	SourceLoaderType.Dict.Set("is_package", py.MustNewMethod("is_package", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := strings.ReplaceAll(self.(*SourceLoader).Path, "\\", "/")
		return py.NewBool(p[strings.LastIndex(p, "/")+1:] == "__init__.py"), nil
	}, 0, "True if the file is a package's __init__.py."))
	SourceLoaderType.Dict.Set("get_source", py.MustNewMethod("get_source", func(self py.Object, args py.Tuple) (py.Object, error) {
		b, err := py.ReadPath(self.(*SourceLoader).Path)
		if err != nil {
			return nil, py.ExceptionNewf(py.ImportError, "source not available through get_data()")
		}
		return str(string(b)), nil
	}, 0, "Return the source text."))
	SourceLoaderType.Dict.Set("get_code", py.MustNewMethod("get_code", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*SourceLoader).code()
	}, 0, "Return the compiled code of the file."))
	SourceLoaderType.Dict.Set("exec_module", py.MustNewMethod("exec_module", func(self py.Object, args py.Tuple) (py.Object, error) {
		mod, ok := args[0].(*py.Module)
		if len(args) != 1 || !ok {
			return nil, py.ExceptionNewf(py.TypeError, "exec_module() needs a module")
		}
		code, err := self.(*SourceLoader).code()
		if err != nil {
			return nil, err
		}
		if _, err := mod.Context.RunCode(code, mod.Globals, mod.Globals, nil); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Execute the file in the module."))

	cm := func(name, doc string, fn func(args py.Tuple) py.Object) {
		BuiltinImporterType.Dict.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
			return fn(args), nil
		}, py.METH_CLASS, doc))
	}
	cm("is_package", "Return False as built-in modules are never packages.", func(py.Tuple) py.Object { return py.False })
	cm("get_code", "Return None: built-in modules have no code object.", func(py.Tuple) py.Object { return py.None })
	cm("get_source", "Return None: built-in modules have no source.", func(py.Tuple) py.Object { return py.None })
	cm("create_module", "Return None: the interpreter creates built-in modules itself.", func(py.Tuple) py.Object { return py.None })
	cm("exec_module", "Do nothing: built-in modules are executed when created.", func(py.Tuple) py.Object { return py.None })
}
