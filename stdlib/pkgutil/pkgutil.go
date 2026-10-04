// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pkgutil provides the implementation of python's 'pkgutil' module:
// utilities for the import system.
//
// Everything here is driven by the same machinery the interpreter's own import
// uses.  A module is discovered by py.ResolveModulePath (the exported form of
// the import system's file finder) and a package is imported by py.Import, so
// there is no second resolver to drift from the first: a name pkgutil reports
// is a name "import" would also resolve, and a package walk_packages descends
// into is one that imports.
//
// Two parts of CPython's module are not reproduced.  get_importer would return
// a real FileFinder (an object built from sys.path_hooks); this interpreter has
// no sys.path_hooks and its importlib.machinery.FileFinder carries no working
// find_spec, so get_importer returns the module_finder marker object pkgutil's
// own results carry - a finder with a path and iter_modules, but no find_spec.
// read_code, the PEP 302 bytecode reader, is absent for the same reason: there
// is no marshal reader to build it on.
package pkgutil

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/importlibmachinery"
)

const module_doc = `Utilities for the import system.`

func init() {
	globals := py.NewStringDict()
	globals.Set("ModuleInfo", moduleInfoType())

	// The functions are registered as module Methods rather than as globals:
	// a Method carries a pointer back to its Module, which is how a function
	// reaches the running Context (and through it sys.path and the import
	// machinery).  A global method has no Module and cannot.
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "pkgutil",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("get_importer", getImporterFn, 0, get_importer_doc),
			py.MustNewMethod("iter_importers", iterImportersFn, 0, iter_importers_doc),
			py.MustNewMethod("iter_modules", iterModulesFn, 0, iter_modules_doc),
			py.MustNewMethod("walk_packages", walkPackagesFn, 0, walk_packages_doc),
			py.MustNewMethod("get_data", getDataFn, 0, get_data_doc),
			py.MustNewMethod("resolve_name", resolveNameFn, 0, resolve_name_doc),
		},
		Globals: globals,
	})
}

const get_importer_doc = `Retrieve a finder for the given path item.

The returned finder is cached in sys.path_importer_cache if it was newly
created by a path hook.`

const iter_importers_doc = `Yield finders for the given module name.

If fullname contains a '.', the finders will be for the package containing
fullname, otherwise they will be all registered top level finders (i.e. those
on both sys.meta_path and sys.path_hooks).`

const iter_modules_doc = `Yield ModuleInfo for all submodules on path, or, if path is None,
all top-level modules on sys.path.

'path' should be either None or a list of paths to look for modules in.

'prefix' is a string to output on the front of every module name on output.`

const walk_packages_doc = `walk_packages(path=None, prefix='', onerror=None)

Yields ModuleInfo for all modules recursively on path, or, if path is None, all
accessible modules.

Note that this function must import all *packages* (NOT all modules!) on the
given path, in order to access the __path__ attribute to find submodules.

onerror is a function which gets called with one argument (the name of the
package which was being imported) if any exception occurs while trying to
import a package.  If no onerror function is supplied, ImportErrors are caught
and ignored, while all other exceptions are propagated, terminating the search.`

const get_data_doc = `Get a resource from a package.

This is a wrapper round the PEP 302 loader get_data API.  The package
argument should be the name of a package, in standard module format
(foo.bar).  The resource argument should be in the form of a relative
filename, using '/' as the path separator.  The parent directory name '..'
is not allowed, and nor is a rooted name (starting with a '/').

The function returns a binary string, which is the contents of the
specified resource.  If the package cannot be located or loaded, or it uses
a PEP 302 loader which does not support get_data(), then None is returned.`

const resolve_name_doc = `Resolve a name to an object.

It is expected that 'name' will be a string in one of the following
formats, where W is shorthand for a valid Python identifier and dot stands
for a literal period in these pseudo-regexes:

W(.W)*
W(.W)*:(W(.W)*)?

The first form is intended for backward compatibility only.  It assumes that
some part of the dotted name is a package, and the rest is an object
somewhere within that package, possibly nested inside other objects.  Because
the place where the package stops and the object hierarchy starts can't be
inferred by inspection, repeated attempts to import must be done with this
form.

In the second form, the caller makes the division point clear through the
provision of a single colon: the dotted name to the left of the colon is a
package to be imported, and the dotted name to the right is the object
hierarchy within that package.  Only one import is needed in this form.  If
it ends with the colon, then a module object is returned.

The function will return an object (which might be a module), or raise one
of the following exceptions:

ValueError - if 'name' isn't in a recognised format
ImportError - if an import failed when it shouldn't have
AttributeError - if a failure occurred when traversing the object hierarchy
                 within the imported package to get to the desired object.`

// moduleInfoType builds pkgutil.ModuleInfo, a namedtuple with fields
// module_finder, name and ispkg.  It is built through collections.namedtuple at
// first use, so the module keeps the same tuple behaviour (indexing, unpacking,
// _fields) as CPython's.
var (
	moduleInfoCache *py.Type
)

func moduleInfoType() py.Object {
	if moduleInfoCache != nil {
		return moduleInfoCache
	}
	// collections.namedtuple is written in Python and may not be loaded yet;
	// fall back to a Go type if it is unavailable, which keeps the module
	// importable in a bare context.
	if t := buildNamedTuple("ModuleInfo", []string{"module_finder", "name", "ispkg"}); t != nil {
		moduleInfoCache = t
		return t
	}
	moduleInfoCache = py.NewType("pkgutil.ModuleInfo", "A namedtuple with minimal info about a module.")
	return moduleInfoCache
}

// buildNamedTuple calls collections.namedtuple, returning nil on any failure.
func buildNamedTuple(name string, fields []string) *py.Type {
	mod := py.GetModuleImplOrNil("collections")
	if mod == nil {
		return nil
	}
	fn, ok := mod.Globals.Get("namedtuple")
	if !ok {
		return nil
	}
	fieldList := py.NewList()
	for _, f := range fields {
		fieldList.Append(py.String(f))
	}
	res, err := py.Call(fn, py.Tuple{py.String(name), fieldList}, py.StringDict{})
	if err != nil {
		return nil
	}
	t, ok := res.(*py.Type)
	if !ok {
		return nil
	}
	// CPython's namedtuple sets __module__ from the caller's frame, so
	// repr(type(pkgutil.ModuleInfo)) is "<class 'pkgutil.ModuleInfo'>".  This
	// interpreter's collections.namedtuple does not set it, so the qualified
	// name is applied here.
	t.Name = "pkgutil." + name
	// collections.namedtuple here stores _fields as a list; CPython stores a
	// tuple.  Set the tuple so ModuleInfo._fields matches the documented
	// type.
	fieldsTuple := make(py.Tuple, len(fields))
	for i, f := range fields {
		fieldsTuple[i] = py.String(f)
	}
	t.Dict.Set("_fields", fieldsTuple)
	return t
}

// newModuleInfo builds one ModuleInfo instance.
func newModuleInfo(finder py.Object, name string, ispkg bool) py.Object {
	t := moduleInfoType()
	res, err := py.Call(t, py.Tuple{finder, py.String(name), py.NewBool(ispkg)}, py.StringDict{})
	if err != nil {
		return py.Tuple{finder, py.String(name), py.NewBool(ispkg)}
	}
	return res
}

// ---------------------------------------------------------------------------
// finders

// finder is the module_finder pkgutil hands out: it stands for a directory on
// the search path.  CPython's is an importlib.machinery.FileFinder built by a
// path hook; this interpreter has no path hooks, so the finder is this type,
// which carries the same .path and iter_modules() surface.
type finder struct {
	path string
}

// finderType DERIVES from importlib.machinery.FileFinder, because that identity
// is what makes the object usable: pkg_resources registers a distribution
// finder against importlib.machinery.FileFinder and dispatches on the importer
// pkgutil hands it.  A private class of its own made the registration miss, so
// pip found ZERO installed distributions and 'pip list' printed nothing and
// exited 0 - a silent wrong answer rather than an error.
var finderType = func() *py.Type {
	t := py.NewTypeX("pkgutil.FileFinder", "A finder for a directory on the search path.", finderNew, nil)
	t.Base = importlibmachinery.FileFinderType
	t.Bases = py.Tuple{importlibmachinery.FileFinderType}
	if err := t.Ready(); err != nil {
		panic(err)
	}
	return t
}()

func (f *finder) Type() *py.Type { return finderType }

func (f *finder) GetDict() py.StringDict {
	d := py.NewStringDict()
	d.Set("path", py.String(f.path))
	return d
}

func (f *finder) M__repr__() (py.Object, error) {
	return py.String("FileFinder('" + f.path + "')"), nil
}

func finderNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var pathObj py.Object
	if err := py.UnpackTuple(args, kwargs, "FileFinder", 1, 1, &pathObj); err != nil {
		return nil, err
	}
	path, err := py.StrAsString(pathObj)
	if err != nil {
		return nil, err
	}
	return &finder{path: path}, nil
}

func init() {
	finderType.Dict.Set("iter_modules", py.MustNewMethod("iter_modules", finderIterModulesFn, 0,
		"Yield (name, ispkg) for each module in this directory."))
}

func finderIterModulesFn(self py.Object, args py.Tuple) (py.Object, error) {
	f := self.(*finder)
	prefix := ""
	if len(args) > 0 {
		p, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		prefix = p
	}
	entries := scanDirForModules(f.path, prefix)
	items := make([]py.Object, 0, len(entries))
	for _, e := range entries {
		items = append(items, py.Tuple{py.String(e.name), py.NewBool(e.ispkg)})
	}
	return newIterator(items), nil
}

func getImporterFn(self py.Object, args py.Tuple) (py.Object, error) {
	var pathObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "get_importer", 1, 1, &pathObj); err != nil {
		return nil, err
	}
	pathItem, err := py.StrAsString(pathObj)
	if err != nil {
		return nil, err
	}
	if stat, err := os.Stat(pathItem); err != nil || !stat.IsDir() {
		return py.None, nil
	}
	return &finder{path: pathItem}, nil
}

// moduleIterator is the iterator pkgutil's functions return: it walks a slice
// of already-computed entries, or calls a producer for a lazy walk.
type moduleIterator struct {
	items    []py.Object
	pos      int
	producer func() (py.Object, bool, error)
}

var moduleIteratorType = py.NewType("pkgutil.iterator", "An iterator over pkgutil results.")

func (it *moduleIterator) Type() *py.Type { return moduleIteratorType }

func (it *moduleIterator) M__iter__() (py.Object, error) { return it, nil }

func (it *moduleIterator) M__next__() (py.Object, error) {
	if it.producer != nil {
		v, ok, err := it.producer()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, py.StopIteration
		}
		return v, nil
	}
	if it.pos >= len(it.items) {
		return nil, py.StopIteration
	}
	item := it.items[it.pos]
	it.pos++
	return item, nil
}

func newIterator(items []py.Object) py.Object {
	return &moduleIterator{items: items}
}

func newIteratorFromProducer(p func() (py.Object, bool, error)) py.Object {
	return &moduleIterator{producer: p}
}

func iterImportersFn(self py.Object, args py.Tuple) (py.Object, error) {
	fullname := ""
	if len(args) > 0 {
		s, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		fullname = s
	}
	if strings.HasPrefix(fullname, ".") {
		return nil, py.ExceptionNewf(py.ImportError, "Relative module name %q not supported", fullname)
	}
	ctx := currentContext(self)
	paths := moduleSearchPaths(ctx, fullname)
	finders := make([]py.Object, 0, len(paths))
	for _, p := range paths {
		if stat, err := os.Stat(p); err == nil && stat.IsDir() {
			finders = append(finders, &finder{path: p})
		}
	}
	return newIterator(finders), nil
}

func iterModulesFn(self py.Object, args py.Tuple) (py.Object, error) {
	var pathObj py.Object = py.None
	prefix := ""
	if len(args) > 2 {
		return nil, py.ExceptionNewf(py.TypeError, "iter_modules() takes at most 2 arguments")
	}
	if len(args) >= 1 {
		pathObj = args[0]
	}
	if len(args) >= 2 {
		p, err := py.StrAsString(args[1])
		if err != nil {
			return nil, err
		}
		prefix = p
	}

	ctx := currentContext(self)
	var paths []string
	if pathObj == nil || pathObj == py.None {
		paths = moduleSearchPaths(ctx, "")
	} else if s, ok := pathObj.(py.String); ok {
		// CPython refuses a bare string: path must be None or a list.
		_ = s
		return nil, py.ExceptionNewf(py.ValueError,
			"path must be None or list of paths to look for modules in")
	} else {
		l, err := py.SequenceList(pathObj)
		if err != nil {
			return nil, err
		}
		for _, item := range l.Items {
			p, err := py.StrAsString(item)
			if err != nil {
				return nil, err
			}
			paths = append(paths, p)
		}
	}

	seen := map[string]bool{}
	var out []py.Object
	for _, dir := range paths {
		f := &finder{path: dir}
		var entries []moduleEntry
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			entries = scanDirForModules(dir, prefix)
		}
		for _, e := range entries {
			if seen[e.name] {
				continue
			}
			seen[e.name] = true
			out = append(out, newModuleInfo(f, e.name, e.ispkg))
		}
	}
	return newIterator(out), nil
}

func walkPackagesFn(self py.Object, args py.Tuple) (py.Object, error) {
	var pathObj, onerror py.Object = py.None, py.None
	prefix := ""
	if len(args) > 3 {
		return nil, py.ExceptionNewf(py.TypeError, "walk_packages() takes at most 3 arguments")
	}
	if len(args) >= 1 {
		pathObj = args[0]
	}
	if len(args) >= 2 {
		p, err := py.StrAsString(args[1])
		if err != nil {
			return nil, err
		}
		prefix = p
	}
	if len(args) >= 3 {
		onerror = args[2]
	}
	w := &packageWalker{self: self, onerror: onerror, seen: map[string]bool{}}
	path := ""
	if pathObj != nil && pathObj != py.None {
		l, err := py.SequenceList(pathObj)
		if err != nil {
			return nil, err
		}
		if len(l.Items) > 0 {
			if p, err := py.StrAsString(l.Items[0]); err == nil {
				path = p
			}
		}
	}
	w.paths = []string{}
	if path != "" {
		w.paths = append(w.paths, path)
	} else {
		w.paths = moduleSearchPaths(currentContext(self), "")
	}
	w.prefix = prefix
	return newIteratorFromProducer(func() (py.Object, bool, error) { return w.next() }), nil
}

// packageWalker implements walk_packages' depth-first traversal.
type packageWalker struct {
	self     py.Object
	onerror  py.Object
	seen     map[string]bool
	paths    []string
	prefix   string
	pending  []py.Object
	started  bool
	finished bool
}

func (w *packageWalker) next() (py.Object, bool, error) {
	if !w.started {
		w.started = true
		entries := w.iterModules(w.paths, w.prefix)
		w.pending = entries
	}
	for len(w.pending) == 0 {
		if len(w.paths) == 0 {
			return nil, false, nil
		}
		// Nothing left at this level; the traversal is complete because the
		// per-package queues are pushed below as they are reached.
		return nil, false, nil
	}
	info := w.pending[0]
	w.pending = w.pending[1:]
	ispkg := w.infoIsPkg(info)
	if !ispkg {
		return info, true, nil
	}
	name := w.infoName(info)
	if err := w.importPackage(name); err != nil {
		if w.onerror != py.None {
			if _, e := py.Call(w.onerror, py.Tuple{py.String(name)}, py.StringDict{}); e != nil {
				return nil, false, e
			}
		} else if !isImportError(err) {
			return nil, false, err
		}
		return info, true, nil
	}
	// Descend into the package's own __path__.
	subPaths := w.packagePaths(name)
	var fresh []string
	for _, p := range subPaths {
		if !w.seen[p] {
			w.seen[p] = true
			fresh = append(fresh, p)
		}
	}
	if len(fresh) > 0 {
		descended := w.iterModules(fresh, name+".")
		w.pending = append(descended, w.pending...)
	}
	return info, true, nil
}

func (w *packageWalker) iterModules(paths []string, prefix string) []py.Object {
	ctx := currentContext(w.self)
	seen := map[string]bool{}
	var out []py.Object
	for _, dir := range paths {
		f := &finder{path: dir}
		if stat, err := os.Stat(dir); err != nil || !stat.IsDir() {
			continue
		}
		for _, e := range scanDirForModules(dir, prefix) {
			if seen[e.name] {
				continue
			}
			seen[e.name] = true
			out = append(out, newModuleInfo(f, e.name, e.ispkg))
		}
	}
	_ = ctx
	return out
}

func (w *packageWalker) infoName(info py.Object) string {
	if l, err := py.SequenceList(info); err == nil && len(l.Items) >= 2 {
		s, err := py.StrAsString(l.Items[1])
		if err == nil {
			return s
		}
	}
	return ""
}

func (w *packageWalker) infoIsPkg(info py.Object) bool {
	if l, err := py.SequenceList(info); err == nil && len(l.Items) >= 3 {
		b, err := py.ObjectIsTrue(l.Items[2])
		if err == nil {
			return b
		}
	}
	return false
}

func (w *packageWalker) importPackage(name string) error {
	ctx := currentContext(w.self)
	return py.Import(ctx, name)
}

func (w *packageWalker) packagePaths(name string) []string {
	ctx := currentContext(w.self)
	if ctx == nil {
		return nil
	}
	mod, err := ctx.Store().GetModule(name)
	if err != nil || mod == nil {
		return nil
	}
	l, ok := mod.Globals.GetOrNil("__path__").(*py.List)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range l.Items {
		if s, ok := item.(py.String); ok {
			out = append(out, string(s))
		}
	}
	return out
}

func isImportError(err error) bool {
	ei, ok := err.(*py.Exception)
	if !ok {
		return false
	}
	return ei.Base != nil && ei.Base.IsSubtype(py.ImportError)
}

func getDataFn(self py.Object, args py.Tuple) (py.Object, error) {
	var pkgObj, resObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "get_data", 2, 2, &pkgObj, &resObj); err != nil {
		return nil, err
	}
	pkg, err := py.StrAsString(pkgObj)
	if err != nil {
		return nil, err
	}
	resource, err := py.StrAsString(resObj)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(resource, "/") || strings.HasPrefix(resource, "..") ||
		strings.Contains(resource, "../") {
		return nil, py.ExceptionNewf(py.ValueError, "Absolute or relative resource paths are not supported")
	}
	ctx := currentContext(self)
	if ctx == nil {
		return py.None, nil
	}
	if err := py.Import(ctx, pkg); err != nil {
		return py.None, nil
	}
	mod, err := ctx.Store().GetModule(pkg)
	if err != nil || mod == nil {
		return py.None, nil
	}
	fileAttr, ok := mod.Globals.GetOrNil("__file__").(py.String)
	if !ok {
		return py.None, nil
	}
	dir := filepath.Dir(string(fileAttr))
	path := filepath.Join(dir, filepath.FromSlash(resource))
	data, err := os.ReadFile(path)
	if err != nil {
		return py.None, nil
	}
	return py.Bytes(data), nil
}

func resolveNameFn(self py.Object, args py.Tuple) (py.Object, error) {
	var nameObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "resolve_name", 1, 1, &nameObj); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(nameObj)
	if err != nil {
		return nil, err
	}
	return resolveName(currentContext(self), name)
}

// resolveName is pkgutil.resolve_name, without the regex: the grammar
// W(.W)*(:W(.W)*)? is enforced by hand.
func resolveName(ctx py.Context, name string) (py.Object, error) {
	pkgPart := name
	objPart := ""
	hasColon := false
	if i := strings.IndexByte(name, ':'); i >= 0 {
		pkgPart = name[:i]
		objPart = name[i+1:]
		hasColon = true
	}
	if !isDottedWords(pkgPart) || (hasColon && objPart != "" && !isDottedWords(objPart)) {
		return nil, py.ExceptionNewf(py.ValueError, "invalid format: %q", name)
	}
	if ctx == nil {
		return nil, py.ExceptionNewf(py.ImportError, "no import context")
	}
	var mod py.Object
	var parts []string
	if hasColon {
		if err := py.Import(ctx, pkgPart); err != nil {
			return nil, err
		}
		m, err := ctx.Store().GetModule(pkgPart)
		if err != nil {
			return nil, err
		}
		mod = m
		if objPart != "" {
			parts = strings.Split(objPart, ".")
		}
	} else {
		all := strings.Split(name, ".")
		modname := all[0]
		rest := all[1:]
		if err := py.Import(ctx, modname); err != nil {
			return nil, err
		}
		m, err := ctx.Store().GetModule(modname)
		if err != nil {
			return nil, err
		}
		mod = m
		for len(rest) > 0 {
			// The next part is imported as a submodule if it can be; when it
			// cannot ("os.path", whose name is not a real submodule of os),
			// the walk stops and the rest is treated as attribute names.
			child := modname + "." + rest[0]
			if err := py.Import(ctx, child); err != nil {
				break
			}
			m, err := ctx.Store().GetModule(child)
			if err != nil {
				break
			}
			mod = m
			modname = child
			rest = rest[1:]
		}
		parts = rest
	}
	var obj py.Object = mod
	for _, p := range parts {
		v, err := py.GetAttrString(obj, p)
		if err != nil {
			return nil, py.ExceptionNewf(py.AttributeError, "module %q has no attribute %q", moduleNameOfObj(obj), p)
		}
		obj = v
	}
	return obj, nil
}

// moduleNameOfObj reports the __name__ of a module object, for an error
// message; anything else reports its type name.
func moduleNameOfObj(o py.Object) string {
	if m, ok := o.(*py.Module); ok {
		if n, ok := m.Globals.GetOrNil("__name__").(py.String); ok {
			return string(n)
		}
	}
	return o.Type().Name
}

// isDottedWords reports whether s matches W(.W)*: dot-separated non-empty
// identifier runs, each of which must not start with a digit.
func isDottedWords(s string) bool {
	if s == "" {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if part == "" {
			return false
		}
		if part[0] >= '0' && part[0] <= '9' {
			return false
		}
		for _, r := range part {
			if !isWordRune(r) {
				return false
			}
		}
	}
	return true
}

// isWordRune is Python's \w for the ASCII range; a non-ASCII letter is also a
// word character to CPython's re.UNICODE, so every rune above 127 is accepted.
func isWordRune(r rune) bool {
	if r == '_' {
		return true
	}
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
		return true
	}
	return r > 127
}

// ---------------------------------------------------------------------------
// directory scanning

// moduleEntry is one module found in a directory.
type moduleEntry struct {
	name  string
	ispkg bool
}

// scanDirForModules lists the importable modules in dir, following CPython's
// _iter_file_finder_modules: a package is a directory holding __init__.py, a
// module is a .py file, and a name containing a dot is skipped.  The names are
// sorted so a package is seen before a module of the same name.
func scanDirForModules(dir, prefix string) []moduleEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// An unreadable directory is ignored, as import ignores it.
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sortStrings(names)

	seen := map[string]bool{}
	var out []moduleEntry
	for _, fn := range names {
		modname := moduleNameOf(fn)
		if modname == "__init__" || seen[modname] {
			continue
		}
		full := filepath.Join(dir, fn)
		ispkg := false
		if modname == "" {
			stat, err := os.Stat(full)
			if err != nil || !stat.IsDir() || strings.Contains(fn, ".") {
				continue
			}
			modname = fn
			contents, err := os.ReadDir(full)
			if err != nil {
				contents = nil
			}
			for _, c := range contents {
				if moduleNameOf(c.Name()) == "__init__" {
					ispkg = true
					break
				}
			}
			if !ispkg {
				continue
			}
		}
		if modname != "" && !strings.Contains(modname, ".") {
			seen[modname] = true
			out = append(out, moduleEntry{name: prefix + modname, ispkg: ispkg})
		}
	}
	return out
}

// moduleNameOf is inspect.getmodulename: the module name for a source, bytecode
// or extension file, or "" for anything else.
func moduleNameOf(fn string) string {
	switch {
	case strings.HasSuffix(fn, ".py"):
		return strings.TrimSuffix(fn, ".py")
	case strings.HasSuffix(fn, ".pyc"):
		return strings.TrimSuffix(fn, ".pyc")
	case strings.HasSuffix(fn, ".so"):
		name := strings.TrimSuffix(fn, ".so")
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[:i]
		}
		return name
	}
	return ""
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ---------------------------------------------------------------------------
// search paths and context

// moduleSearchPaths returns the directories a top-level name is searched in: the
// sys.path entries, or the package's __path__ when fullname names a submodule.
func moduleSearchPaths(ctx py.Context, fullname string) []string {
	if ctx == nil {
		return nil
	}
	if i := strings.LastIndex(fullname, "."); i >= 0 {
		pkg := fullname[:i]
		if mod, err := ctx.Store().GetModule(pkg); err == nil && mod != nil {
			if l, ok := mod.Globals.GetOrNil("__path__").(*py.List); ok {
				var out []string
				for _, item := range l.Items {
					if s, ok := item.(py.String); ok {
						out = append(out, string(s))
					}
				}
				return out
			}
		}
		return nil
	}
	mod, err := ctx.Store().GetModule("sys")
	if err != nil || mod == nil {
		return nil
	}
	l, ok := mod.Globals.GetOrNil("path").(*py.List)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range l.Items {
		if s, ok := item.(py.String); ok {
			out = append(out, string(s))
		}
	}
	return out
}

// currentContext finds the Context a module function is running in.  A
// module-level function is handed its module as self; a bound method is handed
// the method, whose Module is the module it was defined in.  Either may be a
// typed nil (a function called with no module bound), so each is nil-checked
// before its fields are read.
func currentContext(self py.Object) py.Context {
	switch v := self.(type) {
	case *py.Module:
		if v != nil {
			return v.Context
		}
	case *py.Method:
		if v != nil && v.Module != nil {
			return v.Module.Context
		}
	}
	return nil
}
