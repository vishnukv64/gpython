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
	"os"
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
			globals = py.NewStringDictFrom(
				py.DictEntry{Key: "__package__", Value: py.String(pkg)},
				py.DictEntry{Key: "__name__", Value: py.String(pkg)},
			)
		} else {
			// "import_module('a.b')" with a package just means resolve
			// relative to it; an absolute name needs no anchor.
			globals = py.NewStringDictFrom(
				py.DictEntry{Key: "__package__", Value: py.String(pkg)},
				py.DictEntry{Key: "__name__", Value: py.String(pkg)},
			)
		}
	} else {
		globals = py.NewStringDictFrom(
			py.DictEntry{Key: "__package__", Value: py.None},
			py.DictEntry{Key: "__name__", Value: py.String("__main__")},
		)
	}

	var (
		mod py.Object
		err error
	)
	if level > 0 {
		mod, err = py.ImportModuleLevelObject(ctx, name, globals, py.StringDict{}, nil, level)
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
	nameObj, ok := mod.Globals.Get("__name__")
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
	if p, ok := mod.Globals.Get("__file__"); ok {
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
	ModuleSpecType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*ModuleSpec).Name), nil },
		Doc:  "the module's name",
	})
	ModuleSpecType.Dict.Set("origin", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*ModuleSpec).Origin), nil },
		Doc:  "the module's file location",
	})
	ModuleSpecType.Dict.Set("loader", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*ModuleSpec).Loader, nil },
		Doc:  "the loader for this module",
	})
	ModuleSpecType.Dict.Set("submodule_search_locations", &py.Property{
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
	})

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
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "__doc__", Value: py.String(importlib_doc)},
			py.DictEntry{Key: "__package__", Value: py.String("importlib")},
		),
	})
	// importlib.util is a module of its own so that "from importlib.util import
	// find_spec" resolves, exactly as it does in CPython.
	py.RegisterModule(utilModule)
	py.RegisterModuleAlias("importlib._bootstrap", "importlib")
	// importlib.metadata is a module of its own; version() reads a package's
	// installed metadata from disk.
	py.RegisterModule(metadataModule)
	// importlib.resources is a module of its own; it reads a package's data
	// files from the filesystem.
	py.RegisterModule(resourceModule)

	// The Traversable object files() returns.
	for _, m := range []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"joinpath", traversableJoinpath, "Return a child Traversable."},
		{"read_text", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return traversableReadText(self, args, kw)
		}, "Read the file as text."},
		{"read_bytes", traversableReadBytes, "Read the file as bytes."},
		{"is_file", traversableIsFile, "True if this is a file."},
		{"is_dir", traversableIsDir, "True if this is a directory."},
		{"exists", traversableExists, "True if the resource exists."},
	} {
		traversableType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}
	// The as_file() context manager.
	asFileContextType.Dict.Set("__enter__", py.MustNewMethod("__enter__", asFileEnter, 0, "Enter the context, yielding the path."))
	asFileContextType.Dict.Set("__exit__", py.MustNewMethod("__exit__", asFileExit, 0, "Exit the context."))
}

var metadataDoc = `Access to the metadata of an installed distribution.

version(distribution_name) returns the version recorded in the
.dist-info/METADATA of an installed distribution, searching sys.path.`

// metadataModule is importlib.metadata.
var metadataModule = &py.ModuleImpl{
	Info: py.ModuleInfo{
		Name: "importlib.metadata",
		Doc:  metadataDoc,
	},
	Methods: []*py.Method{
		py.MustNewMethod("version", metadataVersion, 0, "version(distribution_name) -> version string"),
	},
}

// metadataVersion looks for <name>-<ver>.dist-info/METADATA (or .egg-info) on
// sys.path and returns its Version field.  A distribution that is not found
// raises PackageNotFoundError, which is a real exception type so callers can
// catch it.
func metadataVersion(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var nameObj py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "s:version", []string{"distribution_name"}, &nameObj); err != nil {
		return nil, err
	}
	name := strings.ToLower(string(nameObj.(py.String)))
	for _, dir := range metadataSearchDirs(currentContext(self)) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			base := e.Name()
			if !strings.HasSuffix(base, ".dist-info") && !strings.HasSuffix(base, ".egg-info") {
				continue
			}
			stem := strings.TrimSuffix(strings.TrimSuffix(base, ".dist-info"), ".egg-info")
			parts := strings.SplitN(stem, "-", 2)
			if !strings.EqualFold(parts[0], name) {
				continue
			}
			if v := readMetadataVersion(filepath.Join(dir, base, "METADATA")); v != "" {
				return py.String(v), nil
			}
		}
	}
	return nil, py.ExceptionNewf(packageNotFoundType, "No package metadata was found for %s", string(nameObj.(py.String)))
}

// packageNotFoundType is importlib.metadata.PackageNotFoundError.
var packageNotFoundType = py.ModuleNotFoundError.NewType("importlib.metadata.PackageNotFoundError",
	"No package metadata was found for the given name.", nil, nil)

// metadataSearchDirs returns the directories version() searches: the entries
// of sys.path, which is where site-packages lives.
func metadataSearchDirs(ctx py.Context) []string {
	if ctx == nil {
		return nil
	}
	seen := map[string]bool{}
	var dirs []string
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	mod, err := ctx.GetModule("sys")
	if err == nil {
		if p, err := py.GetAttrString(mod, "path"); err == nil {
			if l, ok := p.(*py.List); ok {
				for _, it := range l.Items {
					if s, ok := it.(py.String); ok {
						add(string(s))
					}
				}
			}
		}
	}
	return dirs
}

// readMetadataVersion reads the Version: field of a METADATA file.
func readMetadataVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "Version:"); ok {
			return strings.TrimSpace(v)
		}
		if line == "" {
			break // end of the header block
		}
	}
	return ""
}

// checkArgs validates the positional and keyword argument counts of a
// method whose body reads its arguments directly, rather than unpacking
// them into named variables.  min may be negative, meaning "no lower
// bound".
func checkArgs(args py.Tuple, kwargs py.StringDict, name string, min, max int) error {
	if kwargs.Len() != 0 {
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

// ---------------------------------------------------------------------------
// importlib.resources
// ---------------------------------------------------------------------------

// resourcesDoc describes the importlib.resources module.
var resourcesDoc = `Read, open, and access resources that are part of a package.

files(package) returns a Traversable for the package's directory, as_file()
returns a context manager yielding its filesystem path, and read_text/read_bytes
read a file.  This implementation works on the filesystem, which is where the
interpreter loads packages from; no zip importer is supported.`

// resourceModule is importlib.resources.
var resourceModule = &py.ModuleImpl{
	Info: py.ModuleInfo{
		Name: "importlib.resources",
		Doc:  resourcesDoc,
	},
	Methods: []*py.Method{
		py.MustNewMethod("files", resourceFiles, 0, "files(package) -> a Traversable for the package directory"),
		py.MustNewMethod("as_file", resourceAsFile, 0, "as_file(traversable) -> a context manager yielding its path"),
		py.MustNewMethod("read_text", resourceReadText, 0, "read_text(package, resource) -> the file's text"),
		py.MustNewMethod("read_binary", resourceReadBinary, 0, "read_binary(package, resource) -> the file's bytes"),
		py.MustNewMethod("is_resource", resourceIsResource, 0, "is_resource(package, name) -> bool"),
		py.MustNewMethod("open_text", resourceOpenText, 0, "open_text(package, resource) -> a text file object"),
		py.MustNewMethod("open_binary", resourceOpenBinary, 0, "open_binary(package, resource) -> a binary file object"),
		py.MustNewMethod("path", resourcePath, 0, "path(package, resource) -> a context manager yielding the path"),
	},
}

// packageDir returns the filesystem directory of a package, given by name or
// by module object.
func packageDir(ctx py.Context, pkg py.Object) (string, error) {
	if s, ok := pkg.(py.String); ok {
		if ctx == nil {
			return "", py.ExceptionNewf(py.RuntimeError, "no import context")
		}
		mod, err := ctx.GetModule(string(s))
		if err != nil {
			return "", err
		}
		pkg = mod
	}
	if f, err := py.GetAttrString(pkg, "__file__"); err == nil {
		if s, ok := f.(py.String); ok {
			return filepath.Dir(string(s)), nil
		}
	}
	if p, err := py.GetAttrString(pkg, "__path__"); err == nil {
		if l, ok := p.(*py.List); ok && len(l.Items) > 0 {
			if s, ok := l.Items[0].(py.String); ok {
				return string(s), nil
			}
		}
	}
	return "", py.ExceptionNewf(py.TypeError, "package argument must have __file__ or __path__")
}

// traversable is the object files() returns: a path with the read_text,
// read_bytes, joinpath and iteration operations.
type traversable struct {
	path string
	Dict py.StringDict
}

var traversableType = py.NewTypeX("importlib.abc.Traversable",
	"A path-like object describing a resource.", nil, nil)

func (t *traversable) Type() *py.Type         { return traversableType }
func (t *traversable) GetDict() py.StringDict { return t.Dict }

func (t *traversable) M__str__() (py.Object, error) { return py.String(t.path), nil }
func (t *traversable) M__fspath__() (py.Object, error) {
	return py.String(t.path), nil
}

func resourceFiles(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "files() takes exactly one argument")
	}
	dir, err := packageDir(currentContext(self), args[0])
	if err != nil {
		return nil, err
	}
	return &traversable{path: dir, Dict: py.NewStringDict()}, nil
}

func traversableJoinpath(self py.Object, args py.Tuple) (py.Object, error) {
	t := self.(*traversable)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "joinpath() takes exactly one argument")
	}
	child, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	return &traversable{path: filepath.Join(t.path, child), Dict: py.NewStringDict()}, nil
}

func traversableReadText(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	t := self.(*traversable)
	encoding := py.Object(py.String("utf-8"))
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:read_text",
		[]string{"encoding"}, &encoding); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%s", err)
	}
	return py.String(string(data)), nil
}

func traversableReadBytes(self py.Object, args py.Tuple) (py.Object, error) {
	t := self.(*traversable)
	data, err := os.ReadFile(t.path)
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%s", err)
	}
	return py.Bytes(data), nil
}

func traversableExists(self py.Object, args py.Tuple) (py.Object, error) {
	t := self.(*traversable)
	_, err := os.Stat(t.path)
	return py.Bool(err == nil), nil
}

func traversableIsFile(self py.Object, args py.Tuple) (py.Object, error) {
	t := self.(*traversable)
	fi, err := os.Stat(t.path)
	return py.Bool(err == nil && !fi.IsDir()), nil
}

func traversableIsDir(self py.Object, args py.Tuple) (py.Object, error) {
	t := self.(*traversable)
	fi, err := os.Stat(t.path)
	return py.Bool(err == nil && fi.IsDir()), nil
}

// asFileContext is the context manager as_file() returns: entering yields the
// path, and exiting does nothing because the resource is not copied out of a
// zip.
type asFileContext struct {
	path string
	Dict py.StringDict
}

var asFileContextType = py.NewTypeX("importlib.resources._AsFileContext",
	"A context manager yielding a resource's filesystem path.", nil, nil)

func (c *asFileContext) Type() *py.Type         { return asFileContextType }
func (c *asFileContext) GetDict() py.StringDict { return c.Dict }

func resourceAsFile(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "as_file() takes exactly one argument")
	}
	t, ok := args[0].(*traversable)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "as_file() takes a Traversable")
	}
	return &asFileContext{path: t.path, Dict: py.NewStringDict()}, nil
}

func asFileEnter(self py.Object, args py.Tuple) (py.Object, error) {
	return py.String(self.(*asFileContext).path), nil
}

func asFileExit(self py.Object, args py.Tuple) (py.Object, error) {
	return py.None, nil
}

func resourceReadText(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	dir, name, err := packageAndResource(currentContext(self), args, kwargs)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, py.ExceptionNewf(py.FileNotFoundError, "%s", err)
	}
	return py.String(string(data)), nil
}

func resourceReadBinary(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	dir, name, err := packageAndResource(currentContext(self), args, kwargs)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, py.ExceptionNewf(py.FileNotFoundError, "%s", err)
	}
	return py.Bytes(data), nil
}

func packageAndResource(pkgCtx py.Context, args py.Tuple, kwargs py.StringDict) (string, string, error) {
	var pkg, res py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO:resource",
		[]string{"package", "resource"}, &pkg, &res); err != nil {
		return "", "", err
	}
	dir, err := packageDir(pkgCtx, pkg)
	if err != nil {
		return "", "", err
	}
	name, err := py.StrAsString(res)
	if err != nil {
		return "", "", err
	}
	return dir, name, nil
}

func resourceIsResource(self py.Object, args py.Tuple) (py.Object, error) {
	dir, name, err := packageAndResource(currentContext(self), args, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(filepath.Join(dir, name))
	return py.Bool(err == nil && !fi.IsDir()), nil
}

// openBinaryFile and openTextFile return python file objects over the resource
// path via builtins.open.
func openResourceFile(ctx py.Context, dir, name, mode string) (py.Object, error) {
	path := filepath.Join(dir, name)
	var mod py.Object
	if ctx == nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "no import context")
	}
	m, err := ctx.GetModule("builtins")
	if err != nil {
		return nil, err
	}
	mod = m
	openFn, err := py.GetAttrString(mod, "open")
	if err != nil {
		return nil, err
	}
	return py.Call(openFn, py.Tuple{py.String(path), py.String(mode)}, py.NewStringDict())
}

func resourceOpenText(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	dir, name, err := packageAndResource(currentContext(self), args, kwargs)
	if err != nil {
		return nil, err
	}
	return openResourceFile(currentContext(self), dir, name, "r")
}

func resourceOpenBinary(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	dir, name, err := packageAndResource(currentContext(self), args, kwargs)
	if err != nil {
		return nil, err
	}
	return openResourceFile(currentContext(self), dir, name, "rb")
}

func resourcePath(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	dir, name, err := packageAndResource(currentContext(self), args, kwargs)
	if err != nil {
		return nil, err
	}
	return &asFileContext{path: filepath.Join(dir, name), Dict: py.NewStringDict()}, nil
}
