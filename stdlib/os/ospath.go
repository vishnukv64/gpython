// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// os.path: common pathname manipulations.
//
// This is the posixpath module, which os.path is bound to on a posix host
// (and ntpath elsewhere, but this interpreter targets the host it runs on).
// It is registered as both "posixpath" and "ntpath" so that either import
// resolves, and os binds the module as its "path" attribute.
package os

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

const pathModule_doc = `Common pathname manipulations, os.path.

This module implements some useful functions on pathnames.`

// pathModule is the module object that os.path is bound to.  It is built
// once and shared, so that "os.path is posixpath" holds as it does in
// CPython.
//
// It is built lazily because Go runs the init functions of a package in file
// order, and os.go comes before this file: binding os.path there would read
// a module that has not been built yet.  Building on first use removes the
// dependency on that order entirely.
var (
	pathModuleOnce sync.Once
	pathModule     *py.Module
)

// pathImpl and its registration happen at package variable initialisation
// time, which Go runs before any init function - including os.go's, which
// binds os.path.  Registering inside init() was too late: os.go's init ran
// first and bound a nil module.
var pathImpl = &py.ModuleImpl{
	Info: py.ModuleInfo{
		Name: "posixpath",
		Doc:  pathModule_doc,
	},
	Methods: []*py.Method{
		py.MustNewMethod("join", pathJoin, 0, "join(a, *p) -> Join two or more pathname components, inserting '/' as needed."),
		py.MustNewMethod("split", pathSplit, 0, "split(p) -> Split a pathname into (head, tail)."),
		py.MustNewMethod("splitext", pathSplitext, 0, "splitext(p) -> Split the extension from a pathname."),
		py.MustNewMethod("basename", pathBasename, 0, "basename(p) -> Returns the final component of a pathname."),
		py.MustNewMethod("dirname", pathDirname, 0, "dirname(p) -> Returns the directory component of a pathname."),
		py.MustNewMethod("abspath", pathAbspath, 0, "abspath(path) -> Return the absolute version of a path."),
		py.MustNewMethod("normpath", pathNormpath, 0, "normpath(path) -> Normalize path, eliminating double slashes, etc."),
		py.MustNewMethod("normcase", pathNormcase, 0, "normcase(s) -> Normalize case of pathname."),
		py.MustNewMethod("isabs", pathIsabs, 0, "isabs(s) -> Test whether a path is absolute."),
		py.MustNewMethod("exists", pathExists, 0, "exists(path) -> Test whether a path exists."),
		py.MustNewMethod("lexists", pathLexists, 0, "lexists(path) -> Test whether a path exists, without following a final symlink."),
		py.MustNewMethod("commonpath", pathCommonpath, 0, "commonpath(paths) -> Return the longest common sub-path of the given paths."),
		py.MustNewMethod("isdir", pathIsdir, 0, "isdir(s) -> Return true if the pathname refers to an existing directory."),
		py.MustNewMethod("isfile", pathIsfile, 0, "isfile(path) -> Test whether a path is a regular file."),
		py.MustNewMethod("islink", pathIslink, 0, "islink(path) -> Test whether a path is a symbolic link."),
		py.MustNewMethod("getsize", pathGetsize, 0, "getsize(filename) -> Return the size of a file."),
		py.MustNewMethod("getmtime", pathGetmtime, 0, "getmtime(filename) -> Return the last modification time of a file."),
		py.MustNewMethod("expanduser", pathExpanduser, 0, "expanduser(path) -> Expand ~ and ~user constructs."),
		py.MustNewMethod("expandvars", pathExpandvars, 0, "expandvars(path) -> Expand shell variables."),
		py.MustNewMethod("realpath", pathRealpath, 0, "realpath(filename) -> Return the canonical path."),
		py.MustNewMethod("relpath", pathRelpath, 0, "relpath(path, start=os.curdir) -> Return a relative filepath."),
		py.MustNewMethod("samefile", pathSamefile, 0, "samefile(f1, f2) -> Test whether two pathnames reference the same file."),
	},
}

// A program may import either name, and os.path is the module object both
// refer to, so a single registration is shared rather than one per name.
//
// This runs as a package variable initialiser rather than in init(): Go runs
// every package variable initialiser before any init function, and os.go's
// init binds os.path.  Registering in init() would be too late, because init
// functions run in file order and os.go comes first.
// pathGlobals holds the constants, and is attached before registration so
// that the module os.path resolves to has them: a Globals map built later in
// init() was never seen.
var pathGlobals = py.NewStringDictFrom(
	py.DictEntry{Key: "sep", Value: osSep},
	py.DictEntry{Key: "altsep", Value: osAltsep},
	py.DictEntry{Key: "pathsep", Value: osPathsep},
	py.DictEntry{Key: "curdir", Value: py.String(".")},
	py.DictEntry{Key: "pardir", Value: py.String("..")},
	py.DictEntry{Key: "extsep", Value: py.String(".")},
	py.DictEntry{Key: "defpath", Value: osDefpath},
	py.DictEntry{Key: "devnull", Value: osDevnull},
)

var _ = func() bool {
	pathImpl.Globals = pathGlobals
	py.RegisterModule(pathImpl)
	py.RegisterModuleAlias("ntpath", "posixpath")
	return true
}()

// PathModule returns the module object that os.path refers to, building it
// on first use so that the result does not depend on init order.
//
// Note: "import posixpath" produces a second module object built from the
// same implementation, so "os.path is posixpath" is False here where CPython
// makes it True.  They share the implementation, so behaviour is identical;
// only object identity differs, and only code comparing the modules by
// identity would notice.
func PathModule() *py.Module {
	pathModuleOnce.Do(func() {
		impl := py.GetModuleImpl("posixpath")
		if impl == nil {
			return
		}
		mod, err := py.NewModuleStore().NewModule(nil, impl)
		if err != nil {
			panic(err)
		}
		pathModule = mod
	})
	return pathModule
}

// argsToStrings unpacks the argument tuple into Go strings, accepting the
// path-like objects that Python allows (str and bytes).
func argsToStrings(name string, args py.Tuple, min, max int) ([]string, error) {
	if len(args) < min || (max >= 0 && len(args) > max) {
		return nil, py.ExceptionNewf(py.TypeError, "%s: expected %d to %d arguments, got %d", name, min, max, len(args))
	}
	out := make([]string, len(args))
	for i, arg := range args {
		s, err := py.StrAsString(arg)
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "%s: argument must be a path string", name)
		}
		out[i] = s
	}
	return out, nil
}

func pathJoin(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("join", args, 0, -1)
	if err != nil {
		return nil, err
	}
	return py.String(posixJoin(parts)), nil
}

// posixJoin joins path components by POSIX's rule, which filepath.Join does
// not follow.
//
// The rule that matters: an ABSOLUTE component DISCARDS everything before it,
// so join("/base", "/opt/x") is "/opt/x".  filepath.Join keeps the first part
// and appends, giving "/base/opt/x" - which is an entirely different path, and
// a path that does not exist.  pkg_resources joins a directory onto an entry it
// has already made absolute, so every distribution it looked for was reported
// missing: 'pip list' found ZERO installed packages and printed nothing.
//
// An empty component is ignored, and if the result is empty the answer is ".",
// both of which are CPython's rules as well.
func posixJoin(parts []string) string {
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "/") {
			// Absolute: it replaces everything gathered so far.
			b.Reset()
		} else if b.Len() > 0 {
			if !strings.HasSuffix(b.String(), "/") {
				b.WriteByte('/')
			}
		}
		b.WriteString(p)
	}
	if b.Len() == 0 {
		// Every component was empty, and an empty RESULT stays empty - CPython
		// gives "" for join("") and "." only for join() with no arguments at
		// all, which this function cannot be reached with.
		return ""
	}
	return b.String()
}

func pathSplit(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("split", args, 1, 1)
	if err != nil {
		return nil, err
	}
	dir, file := filepath.Split(parts[0])
	// filepath.Split leaves the separator on the directory; Python strips it
	// except when the path is all separators.
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" && strings.HasPrefix(parts[0], "/") {
		dir = "/"
	}
	return py.Tuple{py.String(dir), py.String(file)}, nil
}

func pathSplitext(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("splitext", args, 1, 1)
	if err != nil {
		return nil, err
	}
	ext := filepath.Ext(parts[0])
	return py.Tuple{py.String(strings.TrimSuffix(parts[0], ext)), py.String(ext)}, nil
}

func pathBasename(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("basename", args, 1, 1)
	if err != nil {
		return nil, err
	}
	return py.String(filepath.Base(parts[0])), nil
}

func pathDirname(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("dirname", args, 1, 1)
	if err != nil {
		return nil, err
	}
	return py.String(filepath.Dir(parts[0])), nil
}

func pathAbspath(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("abspath", args, 1, 1)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(parts[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%s", err)
	}
	return py.String(abs), nil
}

func pathNormpath(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("normpath", args, 1, 1)
	if err != nil {
		return nil, err
	}
	return py.String(filepath.Clean(parts[0])), nil
}

func pathNormcase(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("normcase", args, 1, 1)
	if err != nil {
		return nil, err
	}
	// On a case-sensitive filesystem this is the identity, as in CPython on
	// posix.
	return py.String(parts[0]), nil
}

func pathIsabs(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("isabs", args, 1, 1)
	if err != nil {
		return nil, err
	}
	return py.NewBool(filepath.IsAbs(parts[0])), nil
}

func pathExists(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("exists", args, 1, 1)
	if err != nil {
		return nil, err
	}
	return py.NewBool(fileExists(parts[0])), nil
}

// pathLexists reports whether a path exists WITHOUT following a final symlink,
// which is the difference from exists(): a dangling link is lexists-true and
// exists-false.  pip calls it to decide whether it may write somewhere.
func pathLexists(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("lexists", args, 1, 1)
	if err != nil {
		return nil, err
	}
	_, lerr := os.Lstat(parts[0])
	return py.NewBool(lerr == nil), nil
}

// pathCommonpath is the longest common sub-path, compared COMPONENT-wise (unlike
// commonprefix, which is character-wise): "/a/bc" and "/a/bd" share "/a".
// Empty and "." components are dropped first, as CPython does, so
// "a//b/./c" and "a/b/c" agree.  pip's network/auth imports it at module
// level, so its absence stopped every "pip install".
func pathCommonpath(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "commonpath() takes exactly one argument (%d given)", len(args))
	}
	var paths []string
	var ierr error
	err := py.Iterate(args[0], func(o py.Object) bool {
		s, ok := o.(py.String)
		if !ok {
			ierr = py.ExceptionNewf(py.TypeError, "commonpath() argument must be str, not '%s'", o.Type().Name)
			return true
		}
		paths = append(paths, string(s))
		return false
	})
	if err != nil {
		return nil, err
	}
	if ierr != nil {
		return nil, ierr
	}
	if len(paths) == 0 {
		return nil, py.ExceptionNewf(py.ValueError, "commonpath() arg is an empty sequence")
	}
	abs := strings.HasPrefix(paths[0], "/")
	var common []string
	for i, p := range paths {
		if strings.HasPrefix(p, "/") != abs {
			return nil, py.ExceptionNewf(py.ValueError, "Can't mix absolute and relative paths")
		}
		var parts []string
		for _, c := range strings.Split(p, "/") {
			if c != "" && c != "." {
				parts = append(parts, c)
			}
		}
		if i == 0 {
			common = parts
			continue
		}
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
	}
	out := strings.Join(common, "/")
	if abs {
		out = "/" + out
	}
	return py.String(out), nil
}

func pathIsdir(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("isdir", args, 1, 1)
	if err != nil {
		return nil, err
	}
	return py.NewBool(fileIsDir(parts[0])), nil
}

func pathIsfile(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("isfile", args, 1, 1)
	if err != nil {
		return nil, err
	}
	return py.NewBool(fileIsFile(parts[0])), nil
}

func pathIslink(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("islink", args, 1, 1)
	if err != nil {
		return nil, err
	}
	return py.NewBool(fileIsLink(parts[0])), nil
}

func pathGetsize(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("getsize", args, 1, 1)
	if err != nil {
		return nil, err
	}
	size, err := fileSize(parts[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%s", err)
	}
	return py.Int(size), nil
}

func pathGetmtime(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("getmtime", args, 1, 1)
	if err != nil {
		return nil, err
	}
	secs, err := fileModTime(parts[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%s", err)
	}
	return py.Float(secs), nil
}

func pathExpanduser(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("expanduser", args, 1, 1)
	if err != nil {
		return nil, err
	}
	expanded, err := expandUser(parts[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%s", err)
	}
	return py.String(expanded), nil
}

func pathExpandvars(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("expandvars", args, 1, 1)
	if err != nil {
		return nil, err
	}
	return py.String(expandVars(parts[0])), nil
}

func pathRealpath(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("realpath", args, 1, 1)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(parts[0])
	if err != nil {
		// A path that does not exist has no resolved form; CPython returns
		// the absolute version rather than raising.
		abs, absErr := filepath.Abs(parts[0])
		if absErr != nil {
			return py.String(parts[0]), nil
		}
		return py.String(abs), nil
	}
	return py.String(real), nil
}

func pathRelpath(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("relpath", args, 1, 2)
	if err != nil {
		return nil, err
	}
	start := "."
	if len(parts) == 2 {
		start = parts[1]
	}
	rel, err := filepath.Rel(start, parts[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.ValueError, "%s", err)
	}
	return py.String(rel), nil
}

func pathSamefile(self py.Object, args py.Tuple) (py.Object, error) {
	parts, err := argsToStrings("samefile", args, 2, 2)
	if err != nil {
		return nil, err
	}
	return py.NewBool(sameFile(parts[0], parts[1])), nil
}

// The small filesystem questions below are answered here rather than through
// helpers, because each is a single os call and naming them all would add
// more surface than it removes.

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func fileIsDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileIsFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

func fileIsLink(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func fileSize(p string) (int64, error) {
	info, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func fileModTime(p string) (float64, error) {
	info, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return float64(info.ModTime().UnixNano()) / 1e9, nil
}

func expandUser(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p, err
	}
	if p == "~" {
		return home, nil
	}
	// "~user" is expanded through the user database, which this interpreter
	// does not read; leaving it alone is what CPython does for an unknown
	// user, and is honest about not knowing.
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// expandVars expands $var and ${var}, which is the posix form.
func expandVars(p string) string {
	return os.Expand(p, func(name string) string {
		if value, ok := os.LookupEnv(name); ok {
			return value
		}
		return "${" + name + "}"
	})
}

func sameFile(a, b string) bool {
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	if aerr != nil || berr != nil {
		return false
	}
	return os.SameFile(ai, bi)
}
