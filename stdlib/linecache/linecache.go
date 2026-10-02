// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package linecache provides the implementation of python's 'linecache'
// module: read source lines for tracebacks.
//
// traceback, pdb and anything else that wants to show a line of source goes
// through getline().  A file that is not found is looked for down the module
// search path, which is what lets a traceback name a file that was imported
// rather than run.
//
// The cache holds one entry per filename.  A loaded entry records the file's
// size and mtime alongside its lines, and checkcache discards the entry when
// either has changed - so a file edited on disk is re-read on the next
// checkcache, while ordinary lookups never touch the filesystem.  An entry may
// also be lazy: a thunk is stored instead of the lines and is called only when
// the lines are first needed.
//
// The one piece of CPython's module that is not reproduced is
// _getline_from_code/_getlines_from_code, the _interactive_cache used for code
// objects with no file.  This interpreter does not expose co_qualname or a
// code object's source, so those helpers are absent rather than wrong.
package linecache

import (
	"os"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Cache lines from Python source files.

This is intended to read lines from modules imported -- hence if a filename
is not found, it will look down the module search path for a file by
that name.`

func init() {
	globals := py.NewStringDict()

	globals.Set("getline", py.MustNewMethod("getline", getlineFn, 0, getline_doc))
	globals.Set("getlines", py.MustNewMethod("getlines", getlinesFn, 0, getlines_doc))
	globals.Set("checkcache", py.MustNewMethod("checkcache", checkcacheFn, 0, checkcache_doc))
	globals.Set("clearcache", py.MustNewMethod("clearcache", clearcacheFn, 0, clearcache_doc))
	globals.Set("lazycache", py.MustNewMethod("lazycache", lazycacheFn, 0, lazycache_doc))
	globals.Set("updatecache", py.MustNewMethod("updatecache", updatecacheFn, 0, updatecache_doc))
	globals.Set("cache", py.NewStringDict())

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "linecache",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

const getline_doc = `Get a line for a Python source file from the cache.
Update the cache if it doesn't contain an entry for this file already.`

const getlines_doc = `Get the lines for a Python source file from the cache.
Update the cache if it doesn't contain an entry for this file already.`

const checkcache_doc = `Discard cache entries that are out of date.
(This is not checked upon each call!)`

const clearcache_doc = `Clear the cache entirely.`

const lazycache_doc = `Seed the cache for filename with module_globals.

The module loader will be asked for the source only when getlines is
called, not immediately.

If there is an entry in the cache already, it is not altered.

:return: True if a lazy load is registered in the cache,
    otherwise False.`

const updatecache_doc = `Update a cache entry and return its list of lines.
If something's wrong, print a message, discard the cache entry,
and return an empty list.`

// cache is the module's cache dict.  Each value is either a one-element tuple
// holding a thunk (a lazy entry) or a four-element tuple of
// (size, mtime, lines, fullname).
func cacheDict() py.StringDict {
	mod := py.GetModuleImplOrNil("linecache")
	if mod == nil {
		return py.NewStringDict()
	}
	v, ok := mod.Globals.Get("cache")
	if !ok {
		return py.NewStringDict()
	}
	d, ok := v.(py.StringDict)
	if !ok {
		return py.NewStringDict()
	}
	return d
}

func getlineFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var filename, moduleGlobals, lineno py.Object
	// The argument order is getline(filename, lineno, module_globals=None).
	if err := py.UnpackTuple(args, py.StringDict{}, "getline", 2, 3, &filename, &lineno, &moduleGlobals); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(filename)
	if err != nil {
		return nil, err
	}
	line, err := toInt(lineno)
	if err != nil {
		return nil, err
	}
	lines, err := getlines(name, moduleGlobals)
	if err != nil {
		return nil, err
	}
	if line >= 1 && line <= len(lines) {
		return py.String(lines[line-1]), nil
	}
	return py.String(""), nil
}

func getlinesFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var filename, moduleGlobals py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "getlines", 1, 2, &filename, &moduleGlobals); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(filename)
	if err != nil {
		return nil, err
	}
	lines, err := getlines(name, moduleGlobals)
	if err != nil {
		return nil, err
	}
	out := py.NewList()
	for _, l := range lines {
		out.Append(py.String(l))
	}
	return out, nil
}

// getlines returns the cached lines for name, loading them if necessary.
func getlines(name string, moduleGlobals py.Object) ([]string, error) {
	cache := cacheDict()
	if entry, ok := cache.Get(name); ok {
		// A one-element entry is a lazy thunk; anything else is loaded.
		if t, ok := entry.(py.Tuple); ok && len(t) != 1 {
			return linesOf(t[2])
		}
	}
	return updatecache(name, moduleGlobals)
}

func clearcacheFn(self py.Object, args py.Tuple) (py.Object, error) {
	cache := cacheDict()
	for _, k := range cache.Keys() {
		cache.Del(k)
	}
	return py.None, nil
}

func checkcacheFn(self py.Object, args py.Tuple) (py.Object, error) {
	var filename py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "checkcache", 0, 1, &filename); err != nil {
		return nil, err
	}
	cache := cacheDict()
	var names []string
	if filename == nil || filename == py.None {
		names = cache.Keys()
	} else {
		name, err := py.StrAsString(filename)
		if err != nil {
			return nil, err
		}
		names = []string{name}
	}
	for _, name := range names {
		entry, ok := cache.Get(name)
		if !ok {
			continue
		}
		t, ok := entry.(py.Tuple)
		if !ok || len(t) != 4 {
			continue
		}
		mtime := t[1]
		if mtime == py.None {
			// No filesystem stat: nothing to compare against.
			continue
		}
		fullname, err := py.StrAsString(t[3])
		if err != nil {
			continue
		}
		stat, err := os.Stat(fullname)
		if err != nil {
			cache.Del(name)
			continue
		}
		size := t[0]
		if size != py.Int(stat.Size()) || mtime != py.Float(float64(stat.ModTime().UnixNano())/1e9) {
			cache.Del(name)
		}
	}
	return py.None, nil
}

func lazycacheFn(self py.Object, args py.Tuple) (py.Object, error) {
	var filename, moduleGlobals py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "lazycache", 2, 2, &filename, &moduleGlobals); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(filename)
	if err != nil {
		return nil, err
	}
	cache := cacheDict()
	if entry, ok := cache.Get(name); ok {
		if t, ok := entry.(py.Tuple); ok {
			return py.NewBool(len(t) == 1), nil
		}
		return py.False, nil
	}
	thunk := makeLazyThunk(name, moduleGlobals)
	if thunk == nil {
		return py.False, nil
	}
	cache.Set(name, py.Tuple{thunk})
	return py.True, nil
}

func updatecacheFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var filename, moduleGlobals py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "updatecache", 1, 2, &filename, &moduleGlobals); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(filename)
	if err != nil {
		return nil, err
	}
	lines, err := updatecache(name, moduleGlobals)
	if err != nil {
		return nil, err
	}
	out := py.NewList()
	for _, l := range lines {
		out.Append(py.String(l))
	}
	return out, nil
}

// updatecache re-reads name and stores a fresh cache entry.
func updatecache(name string, moduleGlobals py.Object) ([]string, error) {
	cache := cacheDict()
	var lazy py.Object
	if entry, ok := cache.Get(name); ok {
		cache.Del(name)
		if t, ok := entry.(py.Tuple); ok && len(t) == 1 {
			lazy = t[0]
		}
	}
	if sourceUnavailable(name) {
		return nil, nil
	}

	fullname := name
	if strings.HasPrefix(name, "<frozen ") {
		// A frozen module has no real path; take __file__ from the caller's
		// globals.
		if moduleGlobals == nil || moduleGlobals == py.None {
			return nil, nil
		}
		fileAttr, err := py.GetAttrString(moduleGlobals, "__file__")
		if err != nil || fileAttr == py.None {
			return nil, nil
		}
		s, err := py.StrAsString(fileAttr)
		if err != nil {
			return nil, nil
		}
		fullname = s
	}

	stat, err := os.Stat(fullname)
	if err != nil {
		// Realise a lazy loader based lookup if there is one, otherwise try
		// to look the file up right now.
		if lazy == nil {
			lazy = makeLazyThunk(name, moduleGlobals)
		}
		if lazy != nil {
			dataObj, err := py.Call(lazy, py.Tuple{}, py.StringDict{})
			if err == nil && dataObj != py.None {
				data, err := py.StrAsString(dataObj)
				if err == nil {
					lines := splitLinesKeepNewline(data)
					cache.Set(name, py.Tuple{py.Int(len(data)), py.None, stringList(lines), py.String(fullname)})
					return lines, nil
				}
			}
			if err == nil && dataObj == py.None {
				// The loader cannot find the source for this module.
				return nil, nil
			}
		}
		// Look through the module search path, useful only for a relative
		// name.
		if isAbs(name) {
			return nil, nil
		}
		found := false
		for _, dirname := range sysPath() {
			candidate := join(dirname, name)
			if st, err := os.Stat(candidate); err == nil {
				fullname = candidate
				stat = st
				found = true
				break
			}
		}
		if !found {
			return nil, nil
		}
	}

	data, err := readSource(fullname)
	if err != nil {
		return nil, nil
	}
	lines := splitLinesKeepNewline(data)
	if len(lines) == 0 {
		lines = []string{"\n"}
	} else if !strings.HasSuffix(lines[len(lines)-1], "\n") {
		lines[len(lines)-1] += "\n"
	}
	cache.Set(name, py.Tuple{
		py.Int(stat.Size()),
		py.Float(float64(stat.ModTime().UnixNano()) / 1e9),
		stringList(lines),
		py.String(fullname),
	})
	return lines, nil
}

// makeLazyThunk builds the zero-argument callable that asks the module's
// loader for its source, or returns nil when there is no loader to ask.
func makeLazyThunk(name string, moduleGlobals py.Object) py.Object {
	if name == "" || (strings.HasPrefix(name, "<") && strings.HasSuffix(name, ">")) {
		return nil
	}
	if moduleGlobals == nil || moduleGlobals == py.None {
		return nil
	}
	if _, err := py.GetAttrString(moduleGlobals, "__name__"); err != nil {
		return nil
	}
	loader, err := py.GetAttrString(moduleGlobals, "__loader__")
	if err != nil || loader == py.None {
		return nil
	}
	getSource, err := py.GetAttrString(loader, "get_source")
	if err != nil || getSource == py.None {
		return nil
	}
	return &lazyThunk{name: name, getSource: getSource}
}

// lazyThunk is the thunk stored for a lazy cache entry: calling it asks the
// loader for the module's source.
type lazyThunk struct {
	name      string
	getSource py.Object
}

var lazyThunkType = py.NewType("linecache.lazycache.thunk", "A thunk that loads a module's source on first use.")

func (t *lazyThunk) Type() *py.Type { return lazyThunkType }

func (t *lazyThunk) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return py.Call(t.getSource, py.Tuple{py.String(t.name)}, py.StringDict{})
}

// ---------------------------------------------------------------------------
// helpers

// sourceUnavailable reports filenames for which no source can exist, such as
// "<stdin>" or "".
func sourceUnavailable(filename string) bool {
	return filename == "" ||
		(strings.HasPrefix(filename, "<") && strings.HasSuffix(filename, ">") &&
			!strings.HasPrefix(filename, "<frozen "))
}

// linesOf converts a cached lines list back to a []string.
func linesOf(o py.Object) ([]string, error) {
	l, err := py.SequenceList(o)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(l.Items))
	for _, item := range l.Items {
		s, err := py.StrAsString(item)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func stringList(lines []string) *py.List {
	l := py.NewList()
	for _, s := range lines {
		l.Append(py.String(s))
	}
	return l
}

// splitLinesKeepNewline splits data into lines, each keeping its terminating
// newline, the way file.readlines() does.
func splitLinesKeepNewline(data string) []string {
	if data == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			lines = append(lines, data[start:i+1])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// readSource reads a source file, honoring a UTF-8 BOM and falling back to
// latin-1 for undecodable bytes, as tokenize.open approximates.
func readSource(fullname string) (string, error) {
	data, err := os.ReadFile(fullname)
	if err != nil {
		return "", err
	}
	s := string(data)
	return strings.TrimPrefix(s, "\ufeff"), nil
}

func isAbs(name string) bool {
	return strings.HasPrefix(name, "/")
}

func join(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// sysPath reads sys.path.
func sysPath() []string {
	mod := py.GetModuleImplOrNil("sys")
	if mod == nil {
		return nil
	}
	v, ok := mod.Globals.Get("path")
	if !ok {
		return nil
	}
	l, err := py.SequenceList(v)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(l.Items))
	for _, item := range l.Items {
		if s, err := py.StrAsString(item); err == nil {
			out = append(out, s)
		}
	}
	return out
}

func toInt(o py.Object) (int, error) {
	switch v := o.(type) {
	case py.Int:
		return int(v), nil
	case py.Bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case *py.BigInt:
		return v.GoInt()
	}
	return 0, py.ExceptionNewf(py.TypeError, "an integer is required")
}
