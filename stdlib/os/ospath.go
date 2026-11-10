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

// hostPath is chosen in a variable initialiser, not init(): os.go's init binds
// os.path and runs before this file's init, so choosing here in init() bound
// posixpath on Windows.
var (
	pathImpls = [...]*py.ModuleImpl{newPathImpl(posixFlavor), newPathImpl(ntFlavor)}
	hostPath  = pathImpls[hostFlavorIndex()]
)

func hostFlavorIndex() int {
	if runtime.GOOS == "windows" {
		return 1
	}
	return 0
}

// allowMissing is os.path.ALLOW_MISSING, the sentinel realpath(strict=) takes.
var allowMissing = py.NewList()

func init() {
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

// fsRaw is os.fspath: the path as a Go string, and whether it was bytes.
func fsRaw(o py.Object) (string, bool, error) {
	return fsPathArg("", "path", o, "")
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

const (
	sIFMT  = 0o170000
	sIFDIR = 0o040000
	sIFREG = 0o100000
	sIFLNK = 0o120000
)

func isOSOrValue(err error) bool {
	return py.IsException(py.OSError, err) || py.IsException(py.ValueError, err)
}

// statMode is os.stat/os.lstat(o).st_mode, with their errors.
func statMode(name string, o py.Object) (int64, error) {
	r, err := statCall(name, name == "stat", py.Tuple{o}, py.StringDict{})
	if err != nil {
		return 0, err
	}
	m, err := py.GetAttrString(r, "st_mode")
	if err != nil {
		return 0, err
	}
	return int64(m.(py.Int)), nil
}

// statTest is genericpath.exists and friends: stat the path and ask pred of
// its mode; an OSError or ValueError is False and a TypeError is not.
func statTest(name, call string, pred func(mode int64) bool) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		if err := oneArg(name, args); err != nil {
			return nil, err
		}
		m, err := statMode(call, args[0])
		if err != nil {
			if isOSOrValue(err) {
				return py.False, nil
			}
			return nil, err
		}
		return py.NewBool(pred(m)), nil
	}, 0, name+"(path)")
}

// statField is getsize and the getXtime functions: os.stat(path).field.
func statField(name, field string) *py.Method {
	return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
		if err := oneArg(name, args); err != nil {
			return nil, err
		}
		r, err := statCall("stat", true, py.Tuple{args[0]}, py.StringDict{})
		if err != nil {
			return nil, err
		}
		return py.GetAttrString(r, field)
	}, 0, name+"(filename)")
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
	// The embedded library is rooted at its own name, not the working directory.
	if py.IsEmbeddedPath(s) {
		return f.normpath(s), nil
	}
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
		py.MustNewMethod("normpath", func(self py.Object, args py.Tuple) (py.Object, error) {
			if err := oneArg("normpath", args); err != nil {
				return nil, err
			}
			if _, _, err := fsPathArg("_path_normpath", "path", args[0], pathKinds); err != nil {
				return nil, err
			}
			s, isB, err := fsArg(args[0])
			if err != nil {
				return nil, err
			}
			return out(f.normpath(s), isB), nil
		}, 0, "normpath(path)"),
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
		statTest("exists", "stat", func(int64) bool { return true }),
		statTest("lexists", "lstat", func(int64) bool { return true }),
		statTest("isdir", "stat", func(m int64) bool { return m&sIFMT == sIFDIR }),
		statTest("isfile", "stat", func(m int64) bool { return m&sIFMT == sIFREG }),
		statTest("islink", "lstat", func(m int64) bool { return m&sIFMT == sIFLNK }),
		test("isjunction", func(string) bool { return false }), // ponytail: junctions are Windows-only; always False
		py.MustNewMethod("ismount", func(self py.Object, args py.Tuple) (py.Object, error) {
			if err := oneArg("ismount", args); err != nil {
				return nil, err
			}
			if _, err := statMode("lstat", args[0]); err != nil && !isOSOrValue(err) {
				return nil, err
			}
			s, _, err := fsRaw(args[0])
			if err != nil {
				return nil, err
			}
			return py.NewBool(f.ismount(s)), nil
		}, 0, "ismount(path)"),
		statField("getsize", "st_size"),
		statField("getmtime", "st_mtime"),
		statField("getatime", "st_atime"),
		statField("getctime", "st_ctime"),
		py.MustNewMethod("samefile", func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "samefile expected 2 arguments, got %d", len(args))
			}
			var ids [2][2]py.Object
			for i, a := range args {
				r, err := statCall("stat", true, py.Tuple{a}, py.StringDict{})
				if err != nil {
					return nil, err
				}
				for j, n := range []string{"st_ino", "st_dev"} {
					if ids[i][j], err = py.GetAttrString(r, n); err != nil {
						return nil, err
					}
				}
			}
			return py.NewBool(ids[0] == ids[1]), nil
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
		if _, err := py.LstatPath(s); err != nil {
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
	s1, err := py.LstatPath(p)
	if err != nil || s1.Mode()&os.ModeSymlink != 0 {
		return false
	}
	s2, err := py.LstatPath(posixJoin([]string{p, ".."}))
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
	var ferr error
	err := py.Iterate(args[0], func(o py.Object) bool {
		s, b, err := fsRaw(o)
		if err != nil {
			ferr = err
			return true
		}
		items, isB = append(items, s), b
		return false
	})
	if err == nil {
		err = ferr
	}
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
