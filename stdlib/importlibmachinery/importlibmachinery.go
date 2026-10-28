// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package importlibmachinery provides python's 'importlib.machinery' module.
//
// The module carries the names the import system is built from: the three
// suffix tuples, all_suffixes, the two finders, and ModuleSpec, which is the
// same object the importlib package already defines and so is shared with it.
//
// PathFinder and FileFinder are provided as types because code imports them and
// registers handlers against them (pip's vendored pkg_resources does
// "register_finder(importlib.machinery.FileFinder, ...)").  There is no Python
// importer to plug into here - the interpreter loads modules itself - so neither
// has a working find_spec; that is the honest state, and it is what the doc
// string says.
package importlibmachinery

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/importlib"
)

const module_doc = `This module contains the various objects that help in the construction of an
import.

Only the names the import system publishes are provided.  The interpreter loads
its modules itself, so PathFinder and FileFinder carry no working find_spec.`

func init() {
	spec := importlib.ModuleSpecType

	// The suffixes come from the interpreter's own settings, as they do in
	// CPython, where EXTENSION_SUFFIXES is derived from sysconfig's EXT_SUFFIX.
	extensionSuffixes := py.Tuple{py.String("." + soabi() + ".so")}
	sourceSuffixes := py.Tuple{py.String(".py")}
	bytecodeSuffixes := py.Tuple{py.String(".pyc")}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "importlib.machinery",
			Doc:  module_doc,
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "EXTENSION_SUFFIXES", Value: extensionSuffixes},
			py.DictEntry{Key: "SOURCE_SUFFIXES", Value: sourceSuffixes},
			py.DictEntry{Key: "BYTECODE_SUFFIXES", Value: bytecodeSuffixes},
			py.DictEntry{Key: "ModuleSpec", Value: spec},
			py.DictEntry{Key: "PathFinder", Value: PathFinderType},
			py.DictEntry{Key: "FileFinder", Value: FileFinderType},
			py.DictEntry{Key: "SourceFileLoader", Value: SourceLoaderType},
			py.DictEntry{Key: "BuiltinImporter", Value: importlib.BuiltinImporterType},
		),
		Methods: []*py.Method{
			py.MustNewMethod("all_suffixes", allSuffixesFn, 0, "Return a list of all recognized module suffixes for this process."),
		},
	})
}

// PathFinderType is importlib.machinery.PathFinder, the finder that searches the
// path entries.
var PathFinderType = py.NewType("importlib.machinery.PathFinder", "PathFinder searches the sys.path entries for a module.")

// FileFinderType is importlib.machinery.FileFinder, the finder for a directory.
var FileFinderType = py.NewType("importlib.machinery.FileFinder", "FileFinder locates a module inside one directory.")

type SourceLoader = importlib.SourceLoader

var SourceLoaderType = importlib.SourceLoaderType

func init() {
	PathFinderType.Dict.Set("find_spec", py.MustNewMethod("find_spec", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var name, path, target py.Object = nil, py.None, py.None
		if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:find_spec", []string{"fullname", "path", "target"}, &name, &path, &target); err != nil {
			return nil, err
		}
		full, err := py.StrAsString(name)
		if err != nil {
			return nil, err
		}
		if path == py.None {
			return py.None, nil
		}
		last := full[strings.LastIndex(full, ".")+1:]
		it, err := py.Iter(path)
		if err != nil {
			return nil, err
		}
		for {
			d, err := py.Next(it)
			if err != nil {
				if py.IsException(py.StopIteration, err) {
					return py.None, nil
				}
				return nil, err
			}
			dir, ok := d.(py.String)
			if !ok {
				continue
			}
			pkg := filepath.Join(string(dir), last)
			if _, err := os.Stat(filepath.Join(pkg, "__init__.py")); err == nil {
				o := filepath.Join(pkg, "__init__.py")
				return &importlib.ModuleSpec{Name: full, Loader: &SourceLoader{Name: full, Path: o}, Origin: o, IsPkg: true, Location: pkg}, nil
			}
			if f := pkg + ".py"; fileExists(f) {
				return &importlib.ModuleSpec{Name: full, Loader: &SourceLoader{Name: full, Path: f}, Origin: f}, nil
			}
		}
	}, py.METH_CLASS, "Find a module in the given path list."))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// allSuffixesFn returns the recognised module suffixes, source first, as
// CPython's all_suffixes does.
func allSuffixesFn(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 0 {
		return nil, py.ExceptionNewf(py.TypeError, "all_suffixes() takes no arguments")
	}
	return py.NewListFromItems([]py.Object{
		py.String(".py"),
		py.String(".pyc"),
		py.String("." + soabi() + ".so"),
	}), nil
}

// soabi is the interpreter's SOABI.  It is computed the same way sysconfig's
// is, so that the suffix a built extension would carry matches the one
// EXTENSION_SUFFIXES publishes.
func soabi() string {
	abi := "cpython-34"
	switch runtime.GOOS {
	case "darwin":
		return abi + "-darwin"
	case "windows":
		return "cp34-win_amd64"
	}
	return abi + "-" + runtime.GOOS + "-" + runtime.GOARCH
}
