// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package importlib provides the implementation of python's 'importlib'
// module.
//
// Only the parts the interpreter can honour are here: import_module and
// reload drive the interpreter's own import machinery, which is what makes
// them exact, and invalidate_caches is a no-op because there are no finder
// caches to clear.
//
// importlib.util is present with find_spec and spec_from_file_location,
// which resolve against the interpreter's own module search path.  The
// machinery those names sit on in CPython - MetaPathFinder, PathFinder,
// SourceFileLoader, spec.loader.exec_module - is not implemented, so code
// that drives the import system itself rather than calling into it has
// nothing to drive.
package importlib

import (
	"path/filepath"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const importlib_doc = `A pure Python implementation of import.

__import__(name, globals, locals, fromlist, level) -> module

Import a module. Because this function is meant for use by the Python
interpreter and not for general use, it is better to use
importlib.import_module() to programmatically import a module.`

// currentContext is the context an import should be run in.  The module is
// loaded per context, so the store it was created with is the one owning it.
func currentContext(self py.Object) py.Context {
	if m, ok := self.(*py.Module); ok {
		return m.Context
	}
	return nil
}

// dottedPath splits a dotted module name, rejecting the malformed forms
// CPython rejects before it ever reaches the importer.
func dottedPath(name, what string) ([]string, error) {
	if name == "" {
		return nil, py.ExceptionNewf(py.ValueError, "Empty module name")
	}
	parts := strings.Split(name, ".")
	for _, p := range parts {
		if p == "" {
			return nil, py.ExceptionNewf(py.ValueError, "%s has an empty part in it: %q", what, name)
		}
	}
	return parts, nil
}

const import_module_doc = `import_module(name, package=None)

Import a module. The 'name' argument specifies what module to
import in absolute or relative terms (e.g. either pkg.mod or
..mod).  If the name is specified in relative terms, then the
'package' argument must be set to the name of the package which
is to act as the anchor for resolving the package name (e.g.
import_module('..mod', 'pkg.subpkg') will import pkg.mod).

The specified module will be inserted into 'sys.modules' and returned.`

func import_module(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		nameObj py.Object
		pkgObj  py.Object = py.None
	)
	kwlist := []string{"name", "package"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "s|z:import_module", kwlist, &nameObj, &pkgObj); err != nil {
		return nil, err
	}
	name := string(nameObj.(py.String))
	level := 0
	if strings.HasPrefix(name, ".") {
		level = 0
		for level < len(name) && name[level] == '.' {
			level++
		}
		if level > 1 && pkgObj == py.None {
			return nil, py.ExceptionNewf(py.TypeError, "the 'package' argument is required to perform a relative import for %q", name)
		}
	}
	if _, err := dottedPath(name, "package"); err != nil && level == 0 {
		return nil, err
	}

	ctx := currentContext(self)
	var globals py.StringDict
	if pkgObj != nil && pkgObj != py.None {
		pkg := string(pkgObj.(py.String))
		if level > 0 {
			// The relative import is resolved against the package, which is
			// recorded in __package__ - the same hook the import statement
			// uses.
			globals = py.StringDict{"__package__": py.String(pkg), "__name__": py.String(pkg)}
		} else {
			// "import_module('a.b')" with a package just means resolve
			// relative to it; an absolute name needs no anchor.
			globals = py.StringDict{"__package__": py.String(pkg), "__name__": py.String(pkg)}
		}
	} else {
		globals = py.StringDict{"__package__": py.None, "__name__": py.String("__main__")}
	}

	var (
		mod py.Object
		err error
	)
	if level > 0 {
		mod, err = py.ImportModuleLevelObject(ctx, name, globals, nil, nil, level)
	} else {
		if err := py.Import(ctx, name); err != nil {
			return nil, err
		}
		m, err := ctx.Store().GetModule(name)
		if err != nil {
			return nil, err
		}
		mod = m
	}
	if err != nil {
		return nil, err
	}
	return mod, nil
}

const reload_doc = `reload(module)

Reload a previously imported module.  The argument must be a module object,
so it must have been successfully imported before.  This is useful if you
have edited the module source file using an external editor and want to try
out the new version without leaving the Python interpreter.`

func reload(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "reload", 1, 1); err != nil {
		return nil, err
	}
	mod, ok := args[0].(*py.Module)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "reload() argument must be a module")
	}
	nameObj, ok := mod.Globals["__name__"]
	if !ok {
		return nil, py.ExceptionNewf(py.ImportError, "module has no __name__ attribute")
	}
	name, err := py.StrAsString(nameObj)
	if err != nil {
		return nil, err
	}
	if _, err := dottedPath(name, "module name"); err != nil {
		return nil, err
	}
	ctx := currentContext(self)
	if ctx == nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "reload() needs a running interpreter context")
	}
	// Re-executing the module's own file is exactly what reload means here;
	// the module object is reused, so existing references keep working.
	store := ctx.Store()
	path := ""
	if p, ok := mod.Globals["__file__"]; ok {
		if s, err := py.StrAsString(p); err == nil {
			path = s
		}
	}
	if path == "" {
		// A module with no source file - a built-in such as sys, or the
		// interpreter's own Go modules - has nothing to re-execute, so the
		// module is returned unchanged, which is what CPython does for an
		// extension module.
		return mod, nil
	}
	if _, err := py.RunFile(ctx, path, py.CompileOpts{}, mod); err != nil {
		return nil, err
	}
	_ = store
	return mod, nil
}

const invalidate_caches_doc = `invalidate_caches()

Invalidate the caches of all finders on sys.meta_path.`

func invalidate_caches(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "invalidate_caches", 0, 0); err != nil {
		return nil, err
	}
	// The interpreter has no finder caches of its own - module lookup goes
	// straight to the file system - so there is nothing to drop.  The name
	// exists because callers invoke it after writing a module to disk.
	return py.None, nil
}

// findSpec is the implementation of util.find_spec.
func findSpec(ctx py.Context, name string) (py.Object, error) {
	if _, err := dottedPath(name, "package"); err != nil {
		return nil, err
	}
	if ctx != nil {
		if _, err := ctx.Store().GetModule(name); err == nil {
			return &ModuleSpec{Name: name, Loader: py.None, Origin: "built-in"}, nil
		}
		if path, isPkg, err := py.ResolveModulePath(ctx, name); err == nil {
			return &ModuleSpec{
				Name:     name,
				Loader:   py.None,
				Origin:   path,
				IsPkg:    isPkg,
				Location: filepath.Dir(path),
			}, nil
		}
	}
	return py.None, nil
}

// ModuleSpec is the object find_spec and spec_from_file_location return.  It
// carries the fields a caller reads to decide what to do next; it is not a
// live hook into the import system.
type ModuleSpec struct {
	Name            string
	Loader          py.Object
	Origin          string
	IsPkg           bool
	Location        string
	SubmoduleSearch bool
}

var ModuleSpecType = py.NewType("importlib.machinery.ModuleSpec", "The specification for a module, used for loading.")

func (s *ModuleSpec) Type() *py.Type { return ModuleSpecType }

func (s *ModuleSpec) M__repr__() (py.Object, error) {
	return py.String("<ModuleSpec name=" + s.Name + ">"), nil
}

var utilDoc = `Utility module for importlib.  Only find_spec and
spec_from_file_location are implemented; the loader classes are not,
because the interpreter loads modules itself.`

// utilModule is importlib.util, registered as a module of its own because
// "from importlib.util import find_spec" is how the name is reached.
var utilModule = &py.ModuleImpl{
	Info: py.ModuleInfo{
		Name: "importlib.util",
		Doc:  utilDoc,
	},
	Methods: []*py.Method{
		py.MustNewMethod("find_spec", utilFindSpec, 0, find_spec_doc),
		py.MustNewMethod("spec_from_file_location", utilSpecFromFileLocation, 0, spec_from_file_location_doc),
		py.MustNewMethod("module_from_spec", utilModuleFromSpec, 0, module_from_spec_doc),
	},
}

func utilFindSpec(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		nameObj py.Object
		pkgObj  py.Object
		pathObj py.Object
	)
	kwlist := []string{"name", "package", "path"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "s|OO:find_spec", kwlist,
		&nameObj, &pkgObj, &pathObj); err != nil {
		return nil, err
	}
	return findSpec(currentContext(self), string(nameObj.(py.String)))
}

const find_spec_doc = `find_spec(name, package=None) -> ModuleSpec or None

Find the spec for a module, optionally relative to the specified package name.`

func utilSpecFromFileLocation(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		nameObj py.Object = py.None
		locObj  py.Object
		fileObj py.Object
		subObj  py.Object
		loadObj py.Object
	)
	kwlist := []string{"name", "location", "file", "submodule_search_locations", "loader"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O$OOO:spec_from_file_location", kwlist,
		&nameObj, &locObj, &fileObj, &subObj, &loadObj); err != nil {
		return nil, err
	}
	location, err := py.StrAsString(locObj)
	if err != nil {
		return nil, err
	}
	name := ""
	if nameObj != nil && nameObj != py.None {
		name, err = py.StrAsString(nameObj)
		if err != nil {
			return nil, err
		}
	} else {
		base := filepath.Base(location)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return &ModuleSpec{Name: name, Loader: py.None, Origin: location, Location: filepath.Dir(location)}, nil
}

const spec_from_file_location_doc = `spec_from_file_location(name, location=None, *, file=None,
                        submodule_search_locations=None, loader=None)

Return a module spec based on a file location.`

// utilModuleFromSpec makes the module object a spec describes.  Because the
// loader classes are not present, only a spec carrying a module object - the
// shape importlib hands back for an already imported module - can be
// completed; anything else is an honest error rather than a half-built
// module.
func utilModuleFromSpec(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "module_from_spec", 1, 1); err != nil {
		return nil, err
	}
	spec, ok := args[0].(*ModuleSpec)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "module_from_spec() argument must be a ModuleSpec")
	}
	ctx := currentContext(self)
	if ctx != nil {
		if m, err := ctx.Store().GetModule(spec.Name); err == nil {
			return m, nil
		}
	}
	return nil, py.ExceptionNewf(py.ImportError, "cannot create module %q: its loader is not available", spec.Name)
}

const module_from_spec_doc = `module_from_spec(spec)

Create a module based on the provided spec.`

func init() {
	ModuleSpecType.Dict["name"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*ModuleSpec).Name), nil },
		Doc:  "the module's name",
	}
	ModuleSpecType.Dict["origin"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*ModuleSpec).Origin), nil },
		Doc:  "the module's file location",
	}
	ModuleSpecType.Dict["loader"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*ModuleSpec).Loader, nil },
		Doc:  "the loader for this module",
	}
	ModuleSpecType.Dict["submodule_search_locations"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			s := self.(*ModuleSpec)
			if !s.IsPkg {
				return py.None, nil
			}
			l := py.NewList()
			l.Append(py.String(s.Location))
			return l, nil
		},
		Doc: "the package search locations, or None",
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "importlib",
			Doc:  importlib_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("import_module", import_module, 0, import_module_doc),
			py.MustNewMethod("reload", reload, 0, reload_doc),
			py.MustNewMethod("invalidate_caches", invalidate_caches, 0, invalidate_caches_doc),
		},
		Globals: py.StringDict{
			"__doc__":     py.String(importlib_doc),
			"__package__": py.String("importlib"),
		},
	})
	// importlib.util is a module of its own so that "from importlib.util import
	// find_spec" resolves, exactly as it does in CPython.
	py.RegisterModule(utilModule)
	py.RegisterModuleAlias("importlib._bootstrap", "importlib")
}

// checkArgs validates the positional and keyword argument counts of a
// method whose body reads its arguments directly, rather than unpacking
// them into named variables.  min may be negative, meaning "no lower
// bound".
func checkArgs(args py.Tuple, kwargs py.StringDict, name string, min, max int) error {
	if len(kwargs) != 0 {
		return py.ExceptionNewf(py.TypeError, "%s() takes no keyword arguments", name)
	}
	n := len(args)
	if min >= 0 && n < min {
		return py.ExceptionNewf(py.TypeError, "%s() takes at least %d argument%s (%d given)", name, min, plural(min), n)
	}
	if max >= 0 && n > max {
		if min == max {
			return py.ExceptionNewf(py.TypeError, "%s() takes exactly %d argument%s (%d given)", name, max, plural(max), n)
		}
		return py.ExceptionNewf(py.TypeError, "%s() takes at most %d argument%s (%d given)", name, max, plural(max), n)
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
