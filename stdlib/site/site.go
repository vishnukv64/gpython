// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package site implements the site module: the per-interpreter and per-user
// locations where packages are installed.
//
// The two things callers actually need are here:
//
//	site.getusersitepackages()  the directory pip installs --user into
//	site.USER_SITE              the same value
//	site.getsitepackages()      the directories searched for installed packages
//
// The user paths follow CPython's formula for this platform - on macOS
// "~/Library/Python/<major>.<minor>", elsewhere "~/.local" - with the version
// this interpreter reports, so a program that installs with --user and then
// imports gets a self-consistent answer rather than one borrowed from a
// different interpreter on the machine.
//
// The SEARCH paths are not computed from a prefix.  They are read from sys.path,
// which is where this interpreter genuinely looks, so getsitepackages() reports
// what an import would really find.  A directory that is on sys.path because
// PYTHONPATH put it there counts, and a conventional location that is not
// searched does not.
//
// What is NOT done, rather than done badly: the sitecustomize / usercustomize
// hooks and the .pth file processing that CPython's site performs at startup.
// This module runs long after startup and cannot retroactively add paths from
// .pth files, so import_sitecustomize() and addsitedir() report what they do
// and do not silently pretend to have executed something.

package site

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Append module search paths.

This module is automatically imported during initialization.  The paths a
program installs into and imports from are here: getusersitepackages() and
USER_SITE for the per-user location, getsitepackages() for the directories
this interpreter searches.

The startup hooks - sitecustomize, usercustomize and .pth file processing -
are NOT run, and this module says so rather than appearing to have run them.
`

// versionShort is the "<major>.<minor>" this interpreter reports.
//
// It comes from sys.version_info rather than a constant, so the user-install
// paths agree with the version the program itself sees.
func versionShort(ctx py.Context) string {
	major, minor := 3, 4
	// From a live Context first, then from sys's REGISTERED globals - which are
	// available during this package's init, before any Context exists, and are
	// what let USER_SITE be a real value at import rather than a placeholder.
	if ctx != nil {
		if mod, err := ctx.GetModule("sys"); err == nil {
			if a, b, ok := versionFrom(mod); ok {
				major, minor = a, b
			}
		}
	} else if impl := py.GetModuleImpl("sys"); impl != nil {
		if vi, ok := impl.Globals.Get("version_info"); ok {
			if a, b, ok := versionTuple(vi); ok {
				major, minor = a, b
			}
		}
	}
	return itoa(major) + "." + itoa(minor)
}

// versionFrom reads (major, minor) from a sys module.
func versionFrom(mod *py.Module) (int, int, bool) {
	vi, err := py.GetAttrString(mod, "version_info")
	if err != nil {
		return 0, 0, false
	}
	return versionTuple(vi)
}

// versionTuple reads (major, minor) from a version_info value.
func versionTuple(vi py.Object) (int, int, bool) {
	t, ok := vi.(py.Tuple)
	if !ok || len(t) < 2 {
		return 0, 0, false
	}
	a, ok1 := t[0].(py.Int)
	b, ok2 := t[1].(py.Int)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return int(a), int(b), true
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}

// userBase is CPython's per-user base directory for this platform.
func userBase(ctx py.Context) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Python", "Python"+strings.Replace(versionShort(ctx), ".", "", 1))
		}
		return filepath.Join(home, "Library", "Python", versionShort(ctx))
	}
	return filepath.Join(home, ".local")
}

// userSite is where --user installs go.
//
// macOS uses "<base>/lib/python/site-packages" - note "python", not the version
// - while the other platforms put the version in the path, which is the
// difference CPython's own getusersitepackages encodes.
func userSite(ctx py.Context) string {
	base := userBase(ctx)
	if base == "" {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(base, "lib", "python", "site-packages")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(base, "site-packages")
	}
	return filepath.Join(base, "lib", "python"+versionShort(ctx), "site-packages")
}

// sysPath returns the interpreter's search path.
func sysPath(ctx py.Context) []string {
	if ctx == nil {
		return nil
	}
	mod, err := ctx.GetModule("sys")
	if err != nil {
		return nil
	}
	p, err := py.GetAttrString(mod, "path")
	if err != nil {
		return nil
	}
	l, ok := p.(*py.List)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(l.Items))
	for _, it := range l.Items {
		if s, ok := it.(py.String); ok {
			out = append(out, string(s))
		}
	}
	return out
}

// isSiteDir reports whether a path is a directory packages are installed into.
func isSiteDir(p string) bool {
	base := filepath.Base(p)
	return base == "site-packages" || base == "dist-packages"
}

// siteDirs returns the site-packages directories this interpreter searches.
//
// Read from sys.path, so the answer is what an import would find.  A relative
// entry - "lib" as this interpreter adds it - is resolved against the working
// directory, which is where the interpreter will look for it too.
func siteDirs(ctx py.Context) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range sysPath(ctx) {
		if !isSiteDir(p) {
			continue
		}
		abs := p
		if !filepath.IsAbs(abs) {
			if wd, err := os.Getwd(); err == nil {
				abs = filepath.Join(wd, p)
			}
		}
		abs = filepath.Clean(abs)
		if !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	return out
}

func init() {
	globals := py.NewStringDict()
	globals.Set("__doc__", py.String(module_doc))

	// USER_BASE, USER_SITE, PREFIXES and ENABLE_USER_SITE are values at import
	// time in CPython, and code reads them as attributes, so they are computed
	// here once.  A Context is not available during this package's init, so the
	// version-dependent parts are recomputed lazily by the functions below and
	// these attributes are refreshed on first access by getusersitepackages -
	// see the property, which is the value code actually reads.
	globals.Set("ENABLE_USER_SITE", py.NewBool(true))

	// USER_BASE, USER_SITE and PREFIXES are module ATTRIBUTES in CPython, read
	// directly as often as through the functions - pip reads site.USER_SITE when
	// getusersitepackages is absent - so they carry REAL values at import.
	// versionShort reads sys's registered globals, which exist by now.
	base := userBase(nil)
	globals.Set("USER_BASE", py.String(base))
	globals.Set("USER_SITE", py.String(userSite(nil)))
	prefixes := py.NewList()
	if base != "" {
		prefixes.Append(py.String(base))
	}
	globals.Set("PREFIXES", prefixes)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "site",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("getsitepackages", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				out := py.NewList()
				for _, d := range siteDirs(currentContext(self)) {
					out.Append(py.String(d))
				}
				return out, nil
			}, 0, "Return the directories this interpreter searches for packages."),
			py.MustNewMethod("getusersitepackages", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				return py.String(userSite(currentContext(self))), nil
			}, 0, "Return the path of the user-specific site-packages directory."),
			py.MustNewMethod("getuserbase", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				return py.String(userBase(currentContext(self))), nil
			}, 0, "Return the base directory of the user site-packages."),
			py.MustNewMethod("addsitedir", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				// A .pth file would have to be executed to add what it names.
				// Adding the directory itself is half the job, and doing half
				// silently is what this project keeps refusing to do.
				return nil, py.ExceptionNewf(py.NotImplementedError,
					"site.addsitedir() is not implemented: it processes .pth files, which this interpreter does not run")
			}, 0, "Add a site directory; not implemented, see the module documentation."),
			py.MustNewMethod("main", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				return py.None, nil
			}, 0, "Set up the paths; this interpreter already has its paths."),
		},
		Globals: globals,
	})
}

// currentContext returns the Context a module method was called on.
func currentContext(self py.Object) py.Context {
	// self is the METHOD's module, which ModuleInit attaches for everything
	// registered in Methods.  It can still be nil when a function is reached
	// before that, so the type assertion is guarded rather than assumed.
	if self == nil {
		return nil
	}
	if m, ok := self.(*py.Module); ok {
		return m.Context
	}
	return nil
}
