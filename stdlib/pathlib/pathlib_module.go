package pathlib

// Registration: the module globals, the methods and the properties of each
// class.  The implementations live in pathlib.go (pure) and pathlib_io.go
// (filesystem).

import (
	"github.com/vishnukv64/gpython/py"
)

// ---------------------------------------------------------------------------
// kwarg parsing helpers
// ---------------------------------------------------------------------------

func optBool(kwargs py.StringDict, name string, def bool) bool {
	v, ok := kwargs.Get(name)
	if !ok || v == py.None {
		return def
	}
	return toBool(v)
}

func optInt(kwargs py.StringDict, name string, def int64) (int64, error) {
	v, ok := kwargs.Get(name)
	if !ok || v == py.None {
		return def, nil
	}
	n, ok := v.(py.Int)
	if !ok {
		return 0, py.ExceptionNewf(py.TypeError, "%s must be an integer, not %s", name, v.Type().Name)
	}
	return int64(n), nil
}

func optObj(kwargs py.StringDict, name string) py.Object {
	if v, ok := kwargs.Get(name); ok {
		return v
	}
	return nil
}

// checkNoKwargs rejects a keyword that no longer has a keyword parameter,
// matching Python's "takes no keyword arguments" behaviour.
func checkNoKwargs(kwargs py.StringDict, allowed ...string) error {
	for _, k := range kwargs.Keys() {
		found := false
		for _, a := range allowed {
			if k == a {
				found = true
				break
			}
		}
		if !found {
			return py.ExceptionNewf(py.TypeError, "unexpected keyword argument '%s'", k)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// strProp builds a read-only string property from a getter.
func strProp(name string, get func(*path) string) *py.Property {
	return &py.Property{Fget: func(self py.Object) (py.Object, error) {
		p, ok := self.(*path)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "descriptor %s requires a path object", name)
		}
		return py.String(get(p)), nil
	}}
}

// pathProp builds a read-only path property from a getter.
func pathProp(name string, get func(*path) *path) *py.Property {
	return &py.Property{Fget: func(self py.Object) (py.Object, error) {
		p, ok := self.(*path)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "descriptor %s requires a path object", name)
		}
		return get(p), nil
	}}
}

// listProp builds a read-only list property, for parents and suffixes.
func listProp(name string, get func(*path) []py.Object) *py.Property {
	return &py.Property{Fget: func(self py.Object) (py.Object, error) {
		p, ok := self.(*path)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "descriptor %s requires a path object", name)
		}
		return py.NewListFromItems(get(p)), nil
	}}
}

// tupleProp builds a read-only tuple property, for parts.
func tupleProp(name string, get func(*path) []py.Object) *py.Property {
	return &py.Property{Fget: func(self py.Object) (py.Object, error) {
		p, ok := self.(*path)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "descriptor %s requires a path object", name)
		}
		return py.Tuple(get(p)), nil
	}}
}

// pathBases records, for each class, the classes its instances belong to,
// nearest first.  CPython's hierarchy is Path under PurePath, and PosixPath
// under Path and PurePosixPath, so both branches appear.
var pathBases = map[*py.Type][]*py.Type{
	PurePathType:        {PurePathType},
	PurePosixPathType:   {PurePosixPathType, PurePathType},
	PureWindowsPathType: {PureWindowsPathType, PurePathType},
	PathType:            {PathType, PurePathType},
	PosixPathType:       {PosixPathType, PathType, PurePosixPathType, PurePathType},
	WindowsPathType:     {WindowsPathType, PathType, PureWindowsPathType, PurePathType},
}

// pathIsInstance answers isinstance for the path classes.
func pathIsInstance(obj py.Object, class *py.Type) bool {
	p, ok := obj.(*path)
	if !ok {
		return false
	}
	for _, c := range pathBases[p.t] {
		if c == class {
			return true
		}
	}
	return false
}

// onePath is the receiver of a method, which the interpreter supplies as self
// rather than as the first element of args.
func onePath(self py.Object, name string) (*path, error) {
	p, ok := self.(*path)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "descriptor '%s' requires a 'path' object", name)
	}
	return p, nil
}

func init() {
	registerPurePath()
	registerConcretePath()

	// The classes are siblings rather than a base chain (see the notes in
	// pathlib.go), so the methods registered on the two base classes are copied
	// into each class instead of being inherited.  A method already set on a
	// class wins, so a class can still override one.
	copyMissing := func(from *py.Type, to ...*py.Type) {
		for _, t := range to {
			toDict := &t.Dict
			from.Dict.Range(func(k string, v py.Object) bool {
				if !t.Dict.Has(k) {
					toDict.Set(k, v)
				}
				// Range stops when the callback returns true, so
				// false keeps it going.
				return false
			})
		}
	}
	allClasses := []*py.Type{PurePathType, PurePosixPathType, PureWindowsPathType,
		PathType, PosixPathType, WindowsPathType}
	copyMissing(PurePathType, allClasses...)
	copyMissing(PathType, PathType, PosixPathType, WindowsPathType)

	// isinstance support.  The path classes are siblings rather than a base
	// chain, because a class made with NewType would report the wrong class
	// from type() (see the notes in pathlib.go), so the hierarchy is declared
	// here through the same hook the abstract base classes use for their
	// structural checks.
	py.ABCHooks = append(py.ABCHooks, pathIsInstance)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "pathlib",
			Doc:  module_doc,
		},
		Globals: py.NewStringDictFrom(py.DictEntry{Key: "PurePath", Value: PurePathType}, py.DictEntry{Key: "PurePosixPath", Value: PurePosixPathType}, py.DictEntry{Key: "PureWindowsPath", Value: PureWindowsPathType}, py.DictEntry{Key: "Path", Value: PathType}, py.DictEntry{Key: "PosixPath", Value: PosixPathType}, py.DictEntry{Key: "WindowsPath", Value: WindowsPathType}),
	})
}

// registerPurePath installs everything PurePath provides; the concrete classes
// inherit it through their base.
func registerPurePath() {
	t := PurePathType
	d := &t.Dict

	d.Set("name", strProp("name", func(p *path) string { return p.propName() }))
	d.Set("stem", strProp("stem", func(p *path) string { return stemOf(p.propName()) }))
	d.Set("suffix", strProp("suffix", func(p *path) string { return sufOne(p.propName()) }))
	d.Set("suffixes", listProp("suffixes", func(p *path) []py.Object {
		out := []py.Object{}
		for _, s := range sufList(p.propName()) {
			out = append(out, py.String(s))
		}
		return out
	}))
	d.Set("parts", tupleProp("parts", func(p *path) []py.Object {
		out := make([]py.Object, 0, len(p.tail)+1)
		for _, s := range p.propParts() {
			out = append(out, py.String(s))
		}
		return out
	}))
	d.Set("parent", pathProp("parent", func(p *path) *path { return p.propParent() }))
	d.Set("parents", listProp("parents", func(p *path) []py.Object {
		out := []py.Object{}
		for _, par := range p.propParents() {
			out = append(out, par)
		}
		return out
	}))
	d.Set("drive", strProp("drive", func(p *path) string { return p.drv }))
	d.Set("root", strProp("root", func(p *path) string { return p.root }))
	d.Set("anchor", strProp("anchor", func(p *path) string { return p.anchor() }))

	d.Set("as_posix", py.MustNewMethod("as_posix", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "as_posix")
		if err != nil {
			return nil, err
		}
		return py.String(p.asPosix()), nil
	}, 0, "Return the string representation of the path with forward (/) slashes."))

	d.Set("is_absolute", py.MustNewMethod("is_absolute", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "is_absolute")
		if err != nil {
			return nil, err
		}
		return py.Bool(p.isAbsolute()), nil
	}, 0, "True if the path is absolute (has both a root and, if applicable, a drive)."))

	d.Set("as_uri", py.MustNewMethod("as_uri", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "as_uri")
		if err != nil {
			return nil, err
		}
		return p.asURI()
	}, 0, "Return the path as a file URI."))

	d.Set("__str__", py.MustNewMethod("__str__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*path).M__str__()
	}, 0, ""))
	d.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*path).M__repr__()
	}, 0, ""))
	d.Set("__fspath__", py.MustNewMethod("__fspath__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*path).M__fspath__()
	}, 0, "Return the path as a str, for os.fspath()."))
	d.Set("__bytes__", py.MustNewMethod("__bytes__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*path).M__bytes__()
	}, 0, ""))
	d.Set("__hash__", py.MustNewMethod("__hash__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*path).M__hash__()
	}, 0, "Hash the normalised string form."))

	// The rich comparisons and both division operators go through the same
	// dispatch the interpreter uses for Go-defined types, but the methods are
	// also reachable by name from Python, so they are registered here too.
	binOp := func(name string, fn func(p *path, other py.Object) (py.Object, error)) {
		d.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
			p, err := onePath(self, name)
			if err != nil {
				return nil, err
			}
			return fn(p, args[0])
		}, 0, ""))
	}
	_ = binOp

	// The interpreter dispatches these through the Go interfaces the path
	// type implements, so only the Python-visible spellings are registered:
	// a call such as "p.__eq__(q)" arrives as one positional argument.
	d.Set("__eq__", py.MustNewMethod("__eq__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*path).M__eq__(args[0])
	}, 0, ""))
	d.Set("__ne__", py.MustNewMethod("__ne__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*path).M__ne__(args[0])
	}, 0, ""))

	d.Set("with_name", py.MustNewMethod("with_name", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "with_name")
		if err != nil {
			return nil, err
		}
		var name py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "with_name", 1, 1, &name); err != nil {
			return nil, err
		}
		s, ok := name.(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "with_name() argument must be str, not %s", name.Type().Name)
		}
		return p.withName(string(s))
	}, 0, "Return a new path with the file name changed."))

	d.Set("with_suffix", py.MustNewMethod("with_suffix", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "with_suffix")
		if err != nil {
			return nil, err
		}
		var sfx py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "with_suffix", 1, 1, &sfx); err != nil {
			return nil, err
		}
		s, ok := sfx.(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "with_suffix() argument must be str, not %s", sfx.Type().Name)
		}
		return p.withSuffix(string(s))
	}, 0, "Return a new path with the file suffix changed."))

	d.Set("with_stem", py.MustNewMethod("with_stem", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "with_stem")
		if err != nil {
			return nil, err
		}
		var stem py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "with_stem", 1, 1, &stem); err != nil {
			return nil, err
		}
		s, ok := stem.(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "with_stem() argument must be str, not %s", stem.Type().Name)
		}
		return p.withStem(string(s))
	}, 0, "Return a new path with the stem changed."))

	d.Set("joinpath", py.MustNewMethod("joinpath", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "joinpath")
		if err != nil {
			return nil, err
		}
		return p.joinpath(args)
	}, 0, "Combine this path with one or several arguments, and return a new path."))

	d.Set("relative_to", py.MustNewMethod("relative_to", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "relative_to")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "walk_up"); err != nil {
			return nil, err
		}
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "relative_to", 1, 1, &other); err != nil {
			return nil, err
		}
		return p.relativeTo(other, optBool(kwargs, "walk_up", false))
	}, 0, "Return the relative path to another path."))

	d.Set("is_relative_to", py.MustNewMethod("is_relative_to", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "is_relative_to")
		if err != nil {
			return nil, err
		}
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "is_relative_to", 1, 1, &other); err != nil {
			return nil, err
		}
		return p.isRelativeTo(other)
	}, 0, "Return True if the path is relative to another path or False."))

	d.Set("match", py.MustNewMethod("match", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "match")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "case_sensitive"); err != nil {
			return nil, err
		}
		var pat py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "match", 1, 1, &pat); err != nil {
			return nil, err
		}
		return p.match(pat, optObj(kwargs, "case_sensitive"))
	}, 0, "Return True if this path matches the given pattern."))

	d.Set("full_match", py.MustNewMethod("full_match", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "full_match")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "case_sensitive"); err != nil {
			return nil, err
		}
		var pat py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "full_match", 1, 1, &pat); err != nil {
			return nil, err
		}
		return p.fullMatch(pat, optObj(kwargs, "case_sensitive"))
	}, 0, "Return True if this path matches the given glob-style pattern."))
}

// registerConcretePath installs the filesystem methods.  They are registered on
// PathType, so PurePath does not have them, as in CPython.
func registerConcretePath() {
	t := PathType
	d := &t.Dict

	d.Set("stat", py.MustNewMethod("stat", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "stat")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "follow_symlinks"); err != nil {
			return nil, err
		}
		return p.statMethod(kwargs)
	}, 0, "Return the result of the stat() system call on this path."))

	d.Set("lstat", py.MustNewMethod("lstat", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "lstat")
		if err != nil {
			return nil, err
		}
		return p.lstatMethod()
	}, 0, "Like stat(), except if the path points to a symlink, the symlink's status is returned."))

	// exists and the is_* predicates.
	d.Set("exists", py.MustNewMethod("exists", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "exists")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "follow_symlinks"); err != nil {
			return nil, err
		}
		return p.existsMethod(optBool(kwargs, "follow_symlinks", true))
	}, 0, "Whether the path points to an existing file or directory."))

	pred := func(name string, fn func(p *path) (py.Object, error)) {
		d.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
			p, err := onePath(self, name)
			if err != nil {
				return nil, err
			}
			return fn(p)
		}, 0, ""))
	}
	pred("is_file", func(p *path) (py.Object, error) { return p.isFileMethod() })
	pred("is_dir", func(p *path) (py.Object, error) { return p.isDirMethod() })
	pred("is_symlink", func(p *path) (py.Object, error) { return p.isSymlinkMethod() })
	pred("is_fifo", func(p *path) (py.Object, error) { return p.isFIFOMethod() })
	pred("is_socket", func(p *path) (py.Object, error) { return p.isSocketMethod() })
	pred("is_block_device", func(p *path) (py.Object, error) { return p.isBlockDeviceMethod() })
	pred("is_char_device", func(p *path) (py.Object, error) { return p.isCharDeviceMethod() })
	// is_junction is a Windows concept; CPython answers False elsewhere.
	pred("is_junction", func(p *path) (py.Object, error) { return py.False, nil })

	d.Set("samefile", py.MustNewMethod("samefile", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "samefile")
		if err != nil {
			return nil, err
		}
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "samefile", 1, 1, &other); err != nil {
			return nil, err
		}
		return p.samefileMethod(other)
	}, 0, "Return whether other_path is the same file as this path."))

	d.Set("resolve", py.MustNewMethod("resolve", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "resolve")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "strict"); err != nil {
			return nil, err
		}
		return p.resolveMethod(optBool(kwargs, "strict", false))
	}, 0, "Make the path absolute, resolving any symlinks."))

	d.Set("absolute", py.MustNewMethod("absolute", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "absolute")
		if err != nil {
			return nil, err
		}
		return p.absoluteMethod()
	}, 0, "Return an absolute version of this path."))

	d.Set("cwd", py.MustNewMethod("cwd", func(self py.Object, args py.Tuple) (py.Object, error) {
		f, ty, name := posixFlavour, PosixPathType, "PosixPath"
		if windowsSupported() {
			f, ty, name = ntFlavour, WindowsPathType, "WindowsPath"
		}
		if p, ok := self.(*path); ok {
			f, ty, name = p.f, p.t, p.name
		}
		return cwdPath(f, ty, name)
	}, 0, "Return a new path pointing to the current working directory."))

	d.Set("home", py.MustNewMethod("home", func(self py.Object, args py.Tuple) (py.Object, error) {
		return homeCurrent()
	}, 0, "Return a new path pointing to the user's home directory."))

	d.Set("expanduser", py.MustNewMethod("expanduser", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "expanduser")
		if err != nil {
			return nil, err
		}
		return p.expanduserMethod()
	}, 0, "Return a new path with expanded ~ and ~user constructs."))

	d.Set("iterdir", py.MustNewMethod("iterdir", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "iterdir")
		if err != nil {
			return nil, err
		}
		return p.iterdirMethod()
	}, 0, "Yield path objects of the directory contents."))

	d.Set("glob", py.MustNewMethod("glob", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "glob")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "case_sensitive", "recurse_symlinks"); err != nil {
			return nil, err
		}
		var pat py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "glob", 1, 1, &pat); err != nil {
			return nil, err
		}
		s, ok := pat.(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "glob() argument must be str, not %s", pat.Type().Name)
		}
		return p.glob(string(s), optObj(kwargs, "case_sensitive"))
	}, 0, "Iterate over this subtree and yield all existing files matching the given pattern."))

	d.Set("rglob", py.MustNewMethod("rglob", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "rglob")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "case_sensitive", "recurse_symlinks"); err != nil {
			return nil, err
		}
		var pat py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "rglob", 1, 1, &pat); err != nil {
			return nil, err
		}
		s, ok := pat.(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "rglob() argument must be str, not %s", pat.Type().Name)
		}
		return p.rglob(string(s), optObj(kwargs, "case_sensitive"))
	}, 0, "Recursively yield all existing files matching the given pattern."))

	d.Set("walk", py.MustNewMethod("walk", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "walk")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "top_down", "on_error", "follow_symlinks"); err != nil {
			return nil, err
		}
		return p.walkMethod(optBool(kwargs, "top_down", true), optBool(kwargs, "follow_symlinks", false)), nil
	}, 0, "Walk the directory tree from this directory, similar to os.walk()."))

	d.Set("mkdir", py.MustNewMethod("mkdir", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "mkdir")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "mode", "parents", "exist_ok"); err != nil {
			return nil, err
		}
		mode, err := optInt(kwargs, "mode", 0o777)
		if err != nil {
			return nil, err
		}
		return p.mkdirMethod(uint32(mode), optBool(kwargs, "parents", false), optBool(kwargs, "exist_ok", false))
	}, 0, "Create a new directory at this given path."))

	d.Set("rmdir", py.MustNewMethod("rmdir", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "rmdir")
		if err != nil {
			return nil, err
		}
		return p.rmdirMethod()
	}, 0, "Remove this directory.  The directory must be empty."))

	d.Set("unlink", py.MustNewMethod("unlink", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "unlink")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "missing_ok"); err != nil {
			return nil, err
		}
		return p.unlinkMethod(optBool(kwargs, "missing_ok", false))
	}, 0, "Remove this file or link."))

	d.Set("touch", py.MustNewMethod("touch", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "touch")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "mode", "exist_ok"); err != nil {
			return nil, err
		}
		mode, err := optInt(kwargs, "mode", 0o666)
		if err != nil {
			return nil, err
		}
		return p.touchMethod(uint32(mode), optBool(kwargs, "exist_ok", true))
	}, 0, "Create a file at this given path."))

	// rename and replace differ only in their default for missing_ok.
	d.Set("rename", py.MustNewMethod("rename", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "rename")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "missing_ok"); err != nil {
			return nil, err
		}
		var target py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "rename", 1, 1, &target); err != nil {
			return nil, err
		}
		return p.renameMethod(target, optBool(kwargs, "missing_ok", false))
	}, 0, "Rename this file or directory to the given target."))

	d.Set("replace", py.MustNewMethod("replace", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "replace")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "missing_ok"); err != nil {
			return nil, err
		}
		var target py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "replace", 1, 1, &target); err != nil {
			return nil, err
		}
		// replace() overwrites the destination if it exists, which is what
		// os.Rename does on the platforms this interpreter runs on.
		return p.renameMethod(target, false)
	}, 0, "Rename this file or directory to the given target, overwriting it."))

	d.Set("chmod", py.MustNewMethod("chmod", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "chmod")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "follow_symlinks"); err != nil {
			return nil, err
		}
		var mode py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "chmod", 1, 1, &mode); err != nil {
			return nil, err
		}
		n, ok := mode.(py.Int)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "chmod() argument must be int, not %s", mode.Type().Name)
		}
		return p.chmodMethod(uint32(n), optBool(kwargs, "follow_symlinks", true))
	}, 0, "Change the permissions of the path, like os.chmod()."))

	d.Set("symlink_to", py.MustNewMethod("symlink_to", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "symlink_to")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "target_is_directory"); err != nil {
			return nil, err
		}
		var target py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "symlink_to", 1, 1, &target); err != nil {
			return nil, err
		}
		return p.symlinkToMethod(target, optBool(kwargs, "target_is_directory", false))
	}, 0, "Make this path a symbolic link to target."))

	d.Set("hardlink_to", py.MustNewMethod("hardlink_to", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "hardlink_to")
		if err != nil {
			return nil, err
		}
		var target py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "hardlink_to", 1, 1, &target); err != nil {
			return nil, err
		}
		return p.hardlinkToMethod(target)
	}, 0, "Make this path a hard link to the same file as target."))

	d.Set("readlink", py.MustNewMethod("readlink", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "readlink")
		if err != nil {
			return nil, err
		}
		return p.readlinkMethod()
	}, 0, "Return the path to which the symbolic link points."))

	d.Set("owner", py.MustNewMethod("owner", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "owner")
		if err != nil {
			return nil, err
		}
		return p.ownerMethod()
	}, 0, "Return the login name of the file owner."))

	d.Set("group", py.MustNewMethod("group", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "group")
		if err != nil {
			return nil, err
		}
		return p.groupMethod()
	}, 0, "Return the group name of the file group."))

	d.Set("read_text", py.MustNewMethod("read_text", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "read_text")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "encoding", "errors", "newline"); err != nil {
			return nil, err
		}
		return p.readTextMethod(optObj(kwargs, "encoding"), optObj(kwargs, "newline"))
	}, 0, "Open the file in text mode, read it, and close the file."))

	d.Set("read_bytes", py.MustNewMethod("read_bytes", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "read_bytes")
		if err != nil {
			return nil, err
		}
		return p.readBytesMethod()
	}, 0, "Open the file in bytes mode, read it, and close the file."))

	d.Set("write_text", py.MustNewMethod("write_text", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "write_text")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "encoding", "errors", "newline"); err != nil {
			return nil, err
		}
		var data py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "write_text", 1, 1, &data); err != nil {
			return nil, err
		}
		return p.writeTextMethod(data, optObj(kwargs, "encoding"), optObj(kwargs, "newline"))
	}, 0, "Open the file in text mode, write to it, and close the file."))

	d.Set("write_bytes", py.MustNewMethod("write_bytes", func(self py.Object, args py.Tuple) (py.Object, error) {
		p, err := onePath(self, "write_bytes")
		if err != nil {
			return nil, err
		}
		var data py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "write_bytes", 1, 1, &data); err != nil {
			return nil, err
		}
		return p.writeBytesMethod(data)
	}, 0, "Open the file in bytes mode, write to it, and close the file."))

	d.Set("open", py.MustNewMethod("open", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p, err := onePath(self, "open")
		if err != nil {
			return nil, err
		}
		if err := checkNoKwargs(kwargs, "mode", "buffering", "encoding", "errors", "newline"); err != nil {
			return nil, err
		}
		mode := py.Object(py.String("r"))
		if len(args) > 0 {
			mode = args[0]
		}
		return p.openMethod(mode, optObj(kwargs, "buffering"), optObj(kwargs, "encoding"),
			optObj(kwargs, "errors"), optObj(kwargs, "newline"))
	}, 0, "Open the file pointed to by this path and return a file object."))
}
