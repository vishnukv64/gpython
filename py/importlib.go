// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Module finding and loading
//
// This implements the file-system half of the import system: locating a
// module on sys.path or inside an already imported package, loading it and
// wiring up packages (__path__), submodules and relative imports.

package py

import (
	"os"
	"path/filepath"
	"strings"
)

// sysPaths returns the current search path, taken from the sys module's
// "path" list, falling back to the current directory.
func sysPaths(ctx Context) []string {
	sysMod, err := ctx.Store().GetModule("sys")
	if err != nil {
		return []string{"."}
	}
	pathList, ok := sysMod.Globals.GetOrNil("path").(*List)
	if !ok {
		return []string{"."}
	}
	paths := make([]string, 0, len(pathList.Items))
	for _, item := range pathList.Items {
		if dir, ok := item.(String); ok {
			paths = append(paths, string(dir))
		}
	}
	return paths
}

// packagePaths returns the __path__ of an imported package, which is where
// its submodules are searched for.
func packagePaths(parent *Module) []string {
	pathList, ok := parent.Globals.GetOrNil("__path__").(*List)
	if !ok {
		return nil
	}
	paths := make([]string, 0, len(pathList.Items))
	for _, item := range pathList.Items {
		if dir, ok := item.(String); ok {
			paths = append(paths, string(dir))
		}
	}
	return paths
}

// findModule locates the source of the fully qualified dotted name, returning
// the path of the module file and whether it is a package (__init__.py).
//
// The first component is searched on sys.path; every further component is
// searched in the __path__ of the package it belongs to.
func findModule(ctx Context, name string) (path string, isPkg bool, err error) {
	base := name
	var searchPaths []string
	if i := strings.LastIndex(name, "."); i < 0 {
		searchPaths = sysPaths(ctx)
	} else {
		base = name[i+1:]
		parent, err := ctx.GetModule(name[:i])
		if err != nil {
			return "", false, ExceptionNewf(ModuleNotFoundError, "No module named %q", name)
		}
		searchPaths = packagePaths(parent)
	}

	// Namespace package directories found so far, across all search paths.
	// PEP 420 collects EVERY directory that matches, so a package split over
	// several sys.path entries contributes all of them - which is the feature:
	// two distributions can each add to the same namespace.
	var nsDirs []string

	for _, dir := range searchPaths {
		// A package directory wins over a module of the same name
		candidates := []struct {
			file  string
			isPkg bool
		}{
			{filepath.Join(dir, base, "__init__.py"), true},
			{filepath.Join(dir, base+".py"), false},
			{filepath.Join(dir, base, "__init__.pyc"), true},
			{filepath.Join(dir, base+".pyc"), false},
		}
		for _, c := range candidates {
			stat, err := os.Stat(c.file)
			if err == nil && !stat.IsDir() {
				return c.file, c.isPkg, nil
			}
		}
		// NAMESPACE PACKAGE: a directory with no __init__.py is still a
		// package, whose __path__ is that directory.  CPython has had these
		// since PEP 420 and modern packaging uses them, so a tree split across
		// several directories imports without any __init__.py at all.
		//
		// It is reported with an EMPTY path, which initModuleFromPath reads as
		// "a package with no code to run".
		// collected below, across EVERY search path entry
		nsDir := filepath.Join(dir, base)
		if stat, err := os.Stat(nsDir); err == nil && stat.IsDir() {
			nsDirs = append(nsDirs, nsDir)
		}
	}

	if len(nsDirs) > 0 {
		// Every matching directory, joined so that initModuleFromPath sees the
		// whole path.  A single directory is passed as itself, which keeps the
		// common case unchanged.
		if len(nsDirs) == 1 {
			return nsDirs[0], true, nil
		}
		return strings.Join(nsDirs, string(os.PathListSeparator)), true, nil
	}

	return "", false, ExceptionNewf(ModuleNotFoundError, "No module named %q", name)
}

// initModuleFromPath compiles and runs the module at path into a new Module
// belonging to ctx.
//
// __package__ and (for packages) __path__ are installed before the module body
// runs so that the body's own relative imports and submodule imports resolve.
func initModuleFromPath(ctx Context, name, path string, isPkg bool) (*Module, error) {
	// A NAMESPACE PACKAGE is a directory, not a file: there is no code to
	// compile, and the module is a package whose __path__ is that directory.
	// Its body cannot fail, and its submodules resolve through __path__ as any
	// package's do.
	if isPkg {
		// A namespace path may be SEVERAL directories separated by the path
		// list separator, collected from every matching sys.path entry.
		if strings.Contains(path, string(os.PathListSeparator)) {
			var dirs []string
			for _, d := range strings.Split(path, string(os.PathListSeparator)) {
				if abs, err := filepath.Abs(d); err == nil {
					dirs = append(dirs, abs)
				} else {
					dirs = append(dirs, d)
				}
			}
			mod, err := ctx.ModuleInit(&ModuleImpl{
				Info: ModuleInfo{Name: name, FileDesc: "<namespace>"},
			})
			if err != nil {
				return nil, err
			}
			mod.Globals.Set("__path__", NewListFromStrings(dirs))
			mod.Globals.Set("__package__", String(name))
			mod.Globals.Set("__file__", None)
			return mod, nil
		}
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			mod, err := ctx.ModuleInit(&ModuleImpl{
				Info: ModuleInfo{Name: name, FileDesc: path},
			})
			if err != nil {
				return nil, err
			}
			// The path is made ABSOLUTE, which is what CPython's
			// _NamespacePath holds: a relative entry would resolve against
			// whatever the current directory is when a submodule is imported
			// later, which is not where the package was found.
			absPath := path
			if abs, err := filepath.Abs(path); err == nil {
				absPath = abs
			}
			mod.Globals.Set("__path__", NewListFromStrings([]string{absPath}))
			mod.Globals.Set("__package__", String(name))
			mod.Globals.Set("__file__", None)
			return mod, nil
		}
	}

	out, err := ctx.ResolveAndCompile(path, CompileOpts{})
	if err != nil {
		return nil, err
	}

	// Create the module without running any code yet...
	mod, err := ctx.ModuleInit(&ModuleImpl{
		Info: ModuleInfo{
			Name:     name,
			FileDesc: out.FileDesc,
		},
	})
	if err != nil {
		return nil, err
	}

	// ...then give it the package context it needs to import its own members
	//
	// __package__ is the package a module belongs to, which is the module's
	// OWN name when the module IS a package and its parent otherwise.  Using
	// the parent for both made every relative import inside a sub-package
	// resolve one level too high: "from .leaf import basic" in
	// p2/mid/__init__.py looked for p2.leaf instead of p2.mid.leaf.
	pkg := name
	if !isPkg {
		if i := strings.LastIndex(name, "."); i >= 0 {
			pkg = name[:i]
		} else {
			pkg = ""
		}
	}
	mod.Globals.Set("__package__", String(pkg))
	// __file__ is the path the module was loaded from.  CPython sets it on a
	// file-loaded module, and code reads it - importlib.resources uses it to
	// find a package's data files.
	mod.Globals.Set("__file__", String(path))
	// __spec__ describes how the module was found.  CPython sets it on every
	// imported module and code reads it: pip's __main__.py tests
	// "__spec__.parent == ''" on its first statement, and without the
	// attribute "python -m pip" died with "name '__spec__' is not defined".
	mod.Globals.Set("__spec__", NewModuleSpec(name, String(path), isPkg))
	if isPkg {
		mod.Globals.Set("__path__", NewListFromItems([]Object{String(filepath.Dir(path))}))
	}

	if _, err := ctx.RunCode(out.Code, mod.Globals, mod.Globals, nil); err != nil {
		return nil, err
	}

	return mod, nil
}

// ensureFromlist implements "from <module> import a, b": it imports and binds
// any of the named entries that are submodules of mod. Names that are plain
// attributes are left alone; the IMPORT_FROM opcode picks those up.
func ensureFromlist(ctx Context, mod *Module, modName string, fromlist Tuple) error {
	if mod == nil || len(fromlist) == 0 {
		return nil
	}
	// A module does not need a __path__ to have submodules: "from
	// collections import abc" reaches a submodule that is registered
	// natively rather than found as a file.
	_, isPkg := mod.Globals.Get("__path__")
	for _, item := range fromlist {
		sub, ok := item.(String)
		if !ok || string(sub) == "*" {
			continue
		}
		full := modName + "." + string(sub)
		if _, err := ctx.GetModule(full); err == nil {
			continue
		}
		if impl := GetModuleImpl(full); impl != nil {
			subMod, err := ctx.ModuleInit(impl)
			if err != nil {
				return err
			}
			mod.Globals.Set(string(sub), subMod)
			continue
		}
		if !isPkg {
			// Not a package, so the name can only be an attribute
			continue
		}
		path, isPkg2, err := findModule(ctx, full)
		if err != nil {
			// Not a submodule: it should be a plain attribute of the module
			continue
		}
		subMod, err := initModuleFromPath(ctx, full, path, isPkg2)
		if err != nil {
			return err
		}
		mod.Globals.Set(string(sub), subMod)
	}
	return nil
}

// ResolveModulePath returns the file path of the named module and whether it
// is a package (a directory with __init__.py).
//
// It is the exported form of the import system's file finder, used to
// implement running a module as a script.
func ResolveModulePath(ctx Context, name string) (string, bool, error) {
	return findModule(ctx, name)
}

// resolveRelativeImport turns an explicit relative import (level > 0) into an
// absolute dotted name using the importing module's package context (PEP 328).
func resolveRelativeImport(name string, level int, globals StringDict) (string, error) {
	pkg := ""
	if p, ok := globals.Get("__package__"); ok {
		if s, ok := p.(String); ok {
			pkg = string(s)
		}
	}
	if pkg == "" {
		// Fall back to deriving the package from __name__
		if n, ok := globals.Get("__name__"); ok {
			if s, ok := n.(String); ok {
				pkg = string(s)
			}
		}
		if _, isPkg := globals.Get("__path__"); !isPkg {
			if i := strings.LastIndex(pkg, "."); i >= 0 {
				pkg = pkg[:i]
			} else {
				pkg = ""
			}
		}
	}

	// level 1 is the current package, each extra level moves up one package
	for i := 1; i < level; i++ {
		if i := strings.LastIndex(pkg, "."); i >= 0 {
			pkg = pkg[:i]
		} else {
			pkg = ""
		}
	}

	if pkg == "" {
		if level > 1 {
			return "", ExceptionNewf(ImportError, "attempted relative import beyond top-level package")
		}
		return name, nil
	}
	if name == "" {
		return pkg, nil
	}
	return pkg + "." + name, nil
}
