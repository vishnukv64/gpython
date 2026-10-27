package os

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/vishnukv64/gpython/py"
)

// os.path: posixpath and ntpath, two modules that behave the same on every
// host (pathcore.go).  os.path is whichever one matches GOOS, and it is the
// very module object "import posixpath" / "import ntpath" returns.

// flavor is the string logic that differs between posixpath and ntpath.
type flavor struct {
	name       string
	nt         bool
	join       func([]string) string
	split      func(string) (string, string)
	splitroot  func(string) (string, string, string)
	splitext   func(string) (string, string)
	normpath   func(string) string
	normcase   func(string) string
	isabs      func(string) bool
	abs        func(p, cwd string) string
	expanduser func(string) string
	expandvars func(string) string
	commonpath func([]string) (string, error)
	relpath    func(path, start, cwd string) (string, error)
	globals    []py.DictEntry
}

var posixFlavor = &flavor{
	name:       "posixpath",
	join:       posixJoin,
	split:      posixSplit,
	splitroot:  posixSplitroot,
	splitext:   func(p string) (string, string) { return splitExt(p, false, '/') },
	normpath:   posixNormpath,
	normcase:   func(s string) string { return s },
	isabs:      func(s string) bool { return strings.HasPrefix(s, "/") },
	abs:        posixAbs,
	expanduser: posixExpanduser,
	expandvars: posixExpandvars,
	commonpath: posixCommonpath,
	relpath: func(path, start, cwd string) (string, error) {
		return posixRelpath(path, start, cwd), nil
	},
	globals: []py.DictEntry{
		{Key: "sep", Value: py.String("/")}, {Key: "altsep", Value: py.None},
		{Key: "pathsep", Value: py.String(":")}, {Key: "defpath", Value: py.String(":/bin:/usr/bin")},
		{Key: "devnull", Value: py.String("/dev/null")},
		{Key: "supports_unicode_filenames", Value: py.NewBool(runtime.GOOS == "darwin")},
	},
}

var ntFlavor = &flavor{
	name:       "ntpath",
	nt:         true,
	join:       ntJoin,
	split:      ntSplit,
	splitroot:  ntSplitroot,
	splitext:   func(p string) (string, string) { return splitExt(p, true, '\\') },
	normpath:   ntNormpath,
	normcase:   ntNormcase,
	isabs:      ntIsabs,
	abs:        ntAbs,
	expanduser: ntExpanduser,
	expandvars: ntExpandvars,
	commonpath: ntCommonpath,
	relpath:    ntRelpath,
	globals: []py.DictEntry{
		{Key: "sep", Value: py.String(`\`)}, {Key: "altsep", Value: py.String("/")},
		{Key: "pathsep", Value: py.String(";")}, {Key: "defpath", Value: py.String(`.;C:\bin`)},
		{Key: "devnull", Value: py.String("nul")},
		{Key: "supports_unicode_filenames", Value: py.True},
	},
}

var (
	pathImpls = [...]*py.ModuleImpl{newPathImpl(posixFlavor), newPathImpl(ntFlavor)}
	hostPath  = pathImpls[0]
)

// allowMissing is os.path.ALLOW_MISSING, the sentinel realpath(strict=) takes.
var allowMissing = py.NewList()

func init() {
	if runtime.GOOS == "windows" {
		hostPath = pathImpls[1]
	}
	for _, impl := range pathImpls {
		py.RegisterModule(impl)
	}
}

// PathModule is the module object os.path refers to.
func PathModule() *py.Module { return hostPath.Shared() }

func newPathImpl(f *flavor) *py.ModuleImpl {
	impl := &py.ModuleImpl{
		Info: py.ModuleInfo{Name: f.name, Doc: "Common pathname manipulations, " + f.name + "."},
		Globals: py.NewStringDictFrom(append([]py.DictEntry{
			{Key: "curdir", Value: py.String(".")}, {Key: "pardir", Value: py.String("..")},
			{Key: "extsep", Value: py.String(".")}, {Key: "ALLOW_MISSING", Value: allowMissing},
		}, f.globals...)...),
		Methods: f.methods(),
	}
	// The module is stateless, so one object serves every interpreter: that
	// is what makes "os.path is posixpath" true.
	var once sync.Once
	var mod *py.Module
	impl.Shared = func() *py.Module {
		once.Do(func() {
			plain := *impl
			plain.Shared = nil
			var err error
			if mod, err = py.NewModuleStore().NewModule(nil, &plain); err != nil {
				panic(err)
			}
		})
		return mod
	}
	return impl
}

// ---------------------------------------------------------------- arguments

func badPath(o py.Object) error {
	return py.ExceptionNewf(py.TypeError, "expected str, bytes or os.PathLike object, not %s", o.Type().Name)
}

// fsRaw is os.fspath: the path as a Go string, and whether it was bytes.
func fsRaw(o py.Object) (string, bool, error) {
	switch v := o.(type) {
	case py.String:
		return string(v), false, nil
	case py.Bytes:
		return string(v), true, nil
	}
	r, err := fspath(nil, py.Tuple{o})
	if err != nil {
		return "", false, badPath(o)
	}
	switch v := r.(type) {
	case py.String:
		return string(v), false, nil
	case py.Bytes:
		return string(v), true, nil
	}
	return "", false, badPath(o)
}

// fsArg is fsRaw for the pure string logic.  Bytes are widened to one rune
// per byte so that "character" offsets mean what they mean in CPython.
func fsArg(o py.Object) (string, bool, error) {
	s, isB, err := fsRaw(o)
	if isB && !isASCII(s) {
		rs := make([]rune, len(s))
		for i := 0; i < len(s); i++ {
			rs[i] = rune(s[i])
		}
		s = string(rs)
	}
	return s, isB, err
}

func out(s string, isB bool) py.Object {
	if !isB {
		return py.String(s)
	}
	if isASCII(s) {
		return py.Bytes(s)
	}
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 256 {
			b = append(b, byte(r))
		} else {
			b = utf8.AppendRune(b, r)
		}
	}
	return py.Bytes(b)
}

func outRaw(s string, isB bool) py.Object {
	if isB {
		return py.Bytes(s)
	}
	return py.String(s)
}

// argsToStrings is for the functions that go to the OS: raw paths, no types.
func argsToStrings(name string, args py.Tuple, min, max int) ([]string, error) {
	if len(args) < min || (max >= 0 && len(args) > max) {
		return nil, py.ExceptionNewf(py.TypeError, "%s: expected %d to %d arguments, got %d", name, min, max, len(args))
	}
	out := make([]string, len(args))
	for i, a := range args {
		s, _, err := fsRaw(a)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

func oneArg(name string, args py.Tuple) error {
	if len(args) != 1 {
		return py.ExceptionNewf(py.TypeError, "%s() takes exactly one argument (%d given)", name, len(args))
	}
	return nil
}

// pure registers a path -> path function over the widened (character) view.
func pure(name string, f func(string) string) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		if err := oneArg(name, args); err != nil {
			return nil, err
		}
		s, isB, err := fsArg(args[0])
		if err != nil {
			return nil, err
		}
		return out(f(s), isB), nil
	}, 0, name+"(path)")
}

// raw is pure for functions that read the environment or case-fold, where the
// real bytes matter more than character offsets.
func raw(name string, f func(string) string) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		if err := oneArg(name, args); err != nil {
			return nil, err
		}
		s, isB, err := fsRaw(args[0])
		if err != nil {
			return nil, err
		}
		return outRaw(f(s), isB), nil
	}, 0, name+"(path)")
}

// pair registers a path -> (str, str) function.
func pair(name string, f func(string) (string, string)) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		if err := oneArg(name, args); err != nil {
			return nil, err
		}
		s, isB, err := fsArg(args[0])
		if err != nil {
			return nil, err
		}
		a, b := f(s)
		return py.Tuple{out(a, isB), out(b, isB)}, nil
	}, 0, name+"(path)")
}

// test registers a path -> bool function that asks the OS.
func test(name string, f func(string) bool) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		if err := oneArg(name, args); err != nil {
			return nil, err
		}
		s, _, err := fsRaw(args[0])
		if err != nil {
			return nil, err
		}
		return py.NewBool(f(s)), nil
	}, 0, name+"(path)")
}

func mixErr() error {
	return py.ExceptionNewf(py.TypeError, "Can't mix strings and bytes in path components")
}

// widenAll reads several paths, all of which must be str or all bytes.
func widenAll(paths []py.Object) ([]string, bool, error) {
	ss := make([]string, len(paths))
	var isB bool
	for i, p := range paths {
		s, b, err := fsArg(p)
		if err != nil {
			return nil, false, err
		}
		if i > 0 && b != isB {
			return nil, false, mixErr()
		}
		ss[i], isB = s, b
	}
	return ss, isB, nil
}

// cwdFor is os.getcwd() in the same view as the path it is joined to.
func cwdFor(isB bool) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", py.ExceptionNewf(py.OSError, "%s", err)
	}
	if isB && !isASCII(cwd) {
		rs := make([]rune, len(cwd))
		for i := 0; i < len(cwd); i++ {
			rs[i] = rune(cwd[i])
		}
		cwd = string(rs)
	}
	return cwd, nil
}

func (f *flavor) host() bool { return f.nt == (runtime.GOOS == "windows") }

func (f *flavor) abspath(s string, isB bool) (string, error) {
	if f.nt && f.host() && !isB { // GetFullPathName, as CPython does on Windows
		if a, err := filepath.Abs(f.normpath(s)); err == nil {
			return a, nil
		}
	}
	cwd, err := cwdFor(isB)
	if err != nil {
		return "", err
	}
	return f.abs(s, cwd), nil
}

func valueErr(err error) error { return py.ExceptionNewf(py.ValueError, "%s", err) }

func (f *flavor) methods() []*py.Method {
	ms := []*py.Method{
		py.MustNewMethod("join", func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) == 0 {
				return nil, py.ExceptionNewf(py.TypeError, "join() missing 1 required positional argument: 'a'")
			}
			ss, isB, err := widenAll(args)
			if err != nil {
				return nil, err
			}
			return out(f.join(ss), isB), nil
		}, 0, "join(a, *p) -> Join two or more pathname components."),
		pair("split", f.split),
		pair("splitdrive", func(p string) (string, string) {
			d, r, t := f.splitroot(p)
			return d, r + t
		}),
		py.MustNewMethod("splitroot", func(self py.Object, args py.Tuple) (py.Object, error) {
			if err := oneArg("splitroot", args); err != nil {
				return nil, err
			}
			s, isB, err := fsArg(args[0])
			if err != nil {
				return nil, err
			}
			d, r, t := f.splitroot(s)
			return py.Tuple{out(d, isB), out(r, isB), out(t, isB)}, nil
		}, 0, "splitroot(p) -> (drive, root, tail)"),
		pair("splitext", f.splitext),
		pure("basename", func(p string) string { _, t := f.split(p); return t }),
		pure("dirname", func(p string) string { h, _ := f.split(p); return h }),
		pure("normpath", f.normpath),
		raw("normcase", f.normcase),
		py.MustNewMethod("isabs", func(self py.Object, args py.Tuple) (py.Object, error) {
			if err := oneArg("isabs", args); err != nil {
				return nil, err
			}
			s, _, err := fsArg(args[0])
			if err != nil {
				return nil, err
			}
			return py.NewBool(f.isabs(s)), nil
		}, 0, "isabs(s) -> Test whether a path is absolute."),
		py.MustNewMethod("abspath", func(self py.Object, args py.Tuple) (py.Object, error) {
			if err := oneArg("abspath", args); err != nil {
				return nil, err
			}
			s, isB, err := fsArg(args[0])
			if err != nil {
				return nil, err
			}
			a, err := f.abspath(s, isB)
			if err != nil {
				return nil, err
			}
			return out(a, isB), nil
		}, 0, "abspath(path) -> Return the absolute version of a path."),
		raw("expanduser", f.expanduser),
		raw("expandvars", f.expandvars),
		py.MustNewMethod("commonpath", func(self py.Object, args py.Tuple) (py.Object, error) {
			if err := oneArg("commonpath", args); err != nil {
				return nil, err
			}
			var items []py.Object
			if err := py.Iterate(args[0], func(o py.Object) bool { items = append(items, o); return false }); err != nil {
				return nil, err
			}
			if len(items) == 0 {
				if !f.nt { // posixpath words it differently
					return nil, py.ExceptionNewf(py.ValueError, "commonpath() arg is an empty sequence")
				}
				return nil, py.ExceptionNewf(py.ValueError, "commonpath() arg is an empty iterable")
			}
			ss, isB, err := widenAll(items)
			if err != nil {
				return nil, err
			}
			r, err := f.commonpath(ss)
			if err != nil {
				return nil, valueErr(err)
			}
			return out(r, isB), nil
		}, 0, "commonpath(paths) -> Return the longest common sub-path of the given paths."),
		py.MustNewMethod("relpath", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			if len(args) < 1 || len(args) > 2 {
				return nil, py.ExceptionNewf(py.TypeError, "relpath() takes from 1 to 2 arguments (%d given)", len(args))
			}
			objs := []py.Object{args[0]}
			if len(args) == 2 {
				objs = append(objs, args[1])
			} else if v, ok := kwargs.Get("start"); ok && v != py.None {
				objs = append(objs, v)
			}
			ss, isB, err := widenAll(objs)
			if err != nil {
				return nil, err
			}
			if ss[0] == "" {
				return nil, py.ExceptionNewf(py.ValueError, "no path specified")
			}
			start := "."
			if len(ss) == 2 {
				start = ss[1]
			}
			cwd, err := cwdFor(isB)
			if err != nil {
				return nil, err
			}
			r, err := f.relpath(ss[0], start, cwd)
			if err != nil {
				return nil, valueErr(err)
			}
			return out(r, isB), nil
		}, 0, "relpath(path, start=os.curdir) -> Return a relative version of a path."),
		py.MustNewMethod("realpath", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			if len(args) != 1 {
				return nil, py.ExceptionNewf(py.TypeError, "realpath() takes exactly one positional argument (%d given)", len(args))
			}
			s, isB, err := fsRaw(args[0])
			if err != nil {
				return nil, err
			}
			strict, _ := kwargs.Get("strict")
			return f.realpath(s, isB, strict)
		}, 0, "realpath(filename, *, strict=False) -> Return the canonical path."),
		py.MustNewMethod("commonprefix", pathCommonprefix, 0, "commonprefix(m) -> Return the longest common leading component of a list of pathnames."),
		test("exists", fileExists),
		test("lexists", func(p string) bool { _, err := os.Lstat(p); return err == nil }),
		test("isdir", fileIsDir),
		test("isfile", fileIsFile),
		test("islink", fileIsLink),
		test("isjunction", func(string) bool { return false }), // ponytail: junctions are Windows-only; always False
		test("ismount", f.ismount),
		py.MustNewMethod("getsize", func(self py.Object, args py.Tuple) (py.Object, error) {
			ps, err := argsToStrings("getsize", args, 1, 1)
			if err != nil {
				return nil, err
			}
			size, err := fileSize(ps[0])
			if err != nil {
				return nil, py.OSErrorFrom(err, ps[0])
			}
			return py.Int(size), nil
		}, 0, "getsize(filename) -> Return the size of a file."),
		py.MustNewMethod("getmtime", func(self py.Object, args py.Tuple) (py.Object, error) {
			ps, err := argsToStrings("getmtime", args, 1, 1)
			if err != nil {
				return nil, err
			}
			secs, err := fileModTime(ps[0])
			if err != nil {
				return nil, py.OSErrorFrom(err, ps[0])
			}
			return py.Float(secs), nil
		}, 0, "getmtime(filename) -> Return the last modification time of a file."),
		py.MustNewMethod("samefile", func(self py.Object, args py.Tuple) (py.Object, error) {
			ps, err := argsToStrings("samefile", args, 2, 2)
			if err != nil {
				return nil, err
			}
			return py.NewBool(sameFile(ps[0], ps[1])), nil
		}, 0, "samefile(f1, f2) -> Test whether two pathnames reference the same file."),
	}
	if f.nt {
		ms = append(ms, test("isreserved", ntIsreserved))
	}
	return ms
}

// realpath resolves symlinks.  Only the host flavour can: the other has no
// filesystem to ask, so (like ntpath off Windows) it is abspath.
func (f *flavor) realpath(s string, isB bool, strict py.Object) (py.Object, error) {
	if !f.host() {
		w, _, _ := fsArg(outRaw(s, isB))
		a, err := f.abspath(w, isB)
		if err != nil {
			return nil, err
		}
		return out(a, isB), nil
	}
	if strict == py.Object(py.True) {
		if _, err := os.Lstat(s); err != nil {
			return nil, py.OSErrorFrom(err, s)
		}
	}
	abs, err := filepath.Abs(s)
	if err != nil {
		return outRaw(s, isB), nil
	}
	head, tail := abs, ""
	for {
		if real, err := filepath.EvalSymlinks(head); err == nil {
			return outRaw(filepath.Join(real, tail), isB), nil
		}
		parent := filepath.Dir(head)
		if parent == head {
			return outRaw(abs, isB), nil
		}
		tail = filepath.Join(filepath.Base(head), tail)
		head = parent
	}
}

func (f *flavor) ismount(p string) bool {
	if f.nt {
		cwd, err := os.Getwd()
		if err != nil {
			return false
		}
		d, r, rest := ntSplitroot(ntAbs(p, cwd))
		if d != "" && isNTSep(d[0]) {
			return rest == ""
		}
		return r != "" && rest == ""
	}
	s1, err := os.Lstat(p)
	if err != nil || s1.Mode()&os.ModeSymlink != 0 {
		return false
	}
	s2, err := os.Lstat(posixJoin([]string{p, ".."}))
	if err != nil {
		return false
	}
	f1, ok1 := sysStatFields(s1)
	f2, ok2 := sysStatFields(s2)
	if ok1 && ok2 && f1.dev != f2.dev {
		return true
	}
	return os.SameFile(s1, s2)
}

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

func sameFile(a, b string) bool {
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	if aerr != nil || berr != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// pathCommonprefix is the character-wise prefix of a sequence of strings, as
// os.path.commonprefix does (it does not respect path boundaries).

// pathCommonprefix is genericpath.commonprefix: the character-wise prefix of
// a sequence of paths (it does not respect path boundaries).
func pathCommonprefix(self py.Object, args py.Tuple) (py.Object, error) {
	if err := oneArg("commonprefix", args); err != nil {
		return nil, err
	}
	var items []string
	var isB bool
	err := py.Iterate(args[0], func(o py.Object) bool {
		s, b, err := fsRaw(o)
		if err != nil {
			return true
		}
		items, isB = append(items, s), b
		return false
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return py.String(""), nil
	}
	s1, s2 := items[0], items[0]
	for _, s := range items[1:] {
		s1, s2 = min(s1, s), max(s2, s)
	}
	n := 0
	for n < len(s1) && n < len(s2) && s1[n] == s2[n] {
		n++
	}
	for !isB && n > 0 && n < len(s1) && !utf8.RuneStart(s1[n]) {
		n--
	}
	return outRaw(s1[:n], isB), nil
}
