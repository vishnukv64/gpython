package pathlib

// The filesystem half of pathlib: the concrete Path classes.  The pure path
// arithmetic lives in pathlib.go; everything below calls into Go's os package.

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
	pyos "github.com/vishnukv64/gpython/stdlib/os"
)

// unsupported reports the honest error for an operation this interpreter has
// no plumbing for, naming what is missing rather than pretending to succeed.
func unsupported(what string) error {
	return py.ExceptionNewf(py.NotImplementedError, "%s is not supported by gpython", what)
}

// ---------------------------------------------------------------------------
// Constructors
//
// Every path class inherits one New, so a call always arrives with the class
// it was made from in metatype.  That is what lets Path pick a concrete
// flavour at construction time, as CPython's __new__ does.
// ---------------------------------------------------------------------------

// shortName is the last dotted component of a type name, which is what repr
// shows: "pathlib.Path" -> "Path".
func shortName(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// isKindOf walks the base chain.
func isKindOf(t, want *py.Type) bool {
	for ; t != nil; t = t.Base {
		if t == want {
			return true
		}
	}
	return false
}

func windowsSupported() bool { return runtime.GOOS == "windows" }

// parseCtorArgs checks that every argument is a str, bytes or os.PathLike.
func parseCtorArgs(args py.Tuple) error {
	for _, a := range args {
		switch a.(type) {
		case py.String, py.Bytes, *path:
			continue
		}
		if _, ok, err := py.TypeCall0(a, "__fspath__"); ok {
			if err != nil {
				return err
			}
			continue
		}
		return py.ExceptionNewf(py.TypeError,
			"argument should be a str or an os.PathLike object where __fspath__ returns a str, not %s",
			a.Type().Name)
	}
	return nil
}

func pathNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := parseCtorArgs(args); err != nil {
		return nil, err
	}

	// The abstract classes pick a concrete one, as CPython does: on this
	// host Path is PosixPath and PurePath is PurePosixPath.
	concrete := metatype
	if concrete == PathType {
		if windowsSupported() {
			concrete = WindowsPathType
		} else {
			concrete = PosixPathType
		}
	} else if concrete == PurePathType {
		if windowsSupported() {
			concrete = PureWindowsPathType
		} else {
			concrete = PurePosixPathType
		}
	}

	f := posixFlavour
	if isKindOf(concrete, PureWindowsPathType) {
		f = ntFlavour
	}
	// A concrete WindowsPath or PosixPath cannot exist on the wrong host, as
	// in CPython.  The pure classes may: PureWindowsPath is a path with
	// Windows parsing rules, and is usable anywhere.
	isConcreteWindows := isKindOf(concrete, WindowsPathType)
	isConcretePosix := isKindOf(concrete, PosixPathType)
	if isConcreteWindows && !windowsSupported() {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"cannot instantiate %s on your system", reprOf(shortName(concrete.Name)))
	}
	if isConcretePosix && runtime.GOOS == "windows" {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"cannot instantiate %s on your system", reprOf(shortName(concrete.Name)))
	}

	if len(args) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "missing argument")
	}
	return newPath(concrete, shortName(concrete.Name), f, args)
}

// ---------------------------------------------------------------------------
// os error mapping
// ---------------------------------------------------------------------------

// osErr maps a Go error from the os package onto the exception CPython would
// raise.  It delegates to py.OSErrorFrom, which picks the subclass by errno and
// sets errno/strerror/filename; the table that was here hardcoded darwin's
// errno numbers (66 for ENOTEMPTY, which is 39 on linux).
func osErr(err error, path string) error {
	if err == nil {
		return nil
	}
	return py.OSErrorFrom(err, path)
}

func init() {
	// The path constructors are attached here rather than in the type
	// initialisers, because pathNew names the type variables and would
	// otherwise make their initialisation circular.  Every class is set
	// explicitly: the interpreter inherits New at type-creation time, which
	// happens before init() runs, so the subclasses would otherwise have no
	// constructor at all.
	for _, t := range []*py.Type{PurePathType, PurePosixPathType, PureWindowsPathType,
		PathType, PosixPathType, WindowsPathType} {
		t.New = pathNew
	}

}

// ---------------------------------------------------------------------------
// Filesystem helpers
// ---------------------------------------------------------------------------

func (p *path) statInfo(follow bool) (os.FileInfo, error) {
	name := p.String()
	op := os.Stat
	if !follow {
		op = os.Lstat
	}
	fi, err := op(name)
	if err != nil {
		return nil, osErr(err, name)
	}
	return fi, nil
}

func toBool(o py.Object) bool {
	if o == nil || o == py.None {
		return false
	}
	t, err := py.ObjectIsTrue(o)
	if err != nil {
		return false
	}
	return t
}

// swallowMissing turns an OSError into False and every other error into nil,
// which is the contract of exists(), is_file() and the rest.
func swallowMissing(o py.Object, err error) (py.Object, error) {
	if err == nil {
		return o, nil
	}
	if e, ok := err.(*py.Exception); ok && e.Base.IsSubtype(py.OSError) {
		return py.False, nil
	}
	return nil, err
}

// ---------------------------------------------------------------------------
// Path methods: stat and the mode predicates
// ---------------------------------------------------------------------------

func (p *path) statMethod(kwargs py.StringDict) (py.Object, error) {
	follow := true
	if v, ok := kwargs.Get("follow_symlinks"); ok {
		follow = toBool(v)
	}
	fi, err := p.statInfo(follow)
	if err != nil {
		return nil, err
	}
	return pyos.NewStatResult(fi), nil
}

func (p *path) lstatMethod() (py.Object, error) {
	fi, err := p.statInfo(false)
	if err != nil {
		return nil, err
	}
	return pyos.NewStatResult(fi), nil
}

func (p *path) existsMethod(follow bool) (py.Object, error) {
	if _, err := p.statInfo(follow); err != nil {
		return swallowMissing(nil, err)
	}
	return py.True, nil
}

func (p *path) isFileMethod() (py.Object, error) {
	fi, err := p.statInfo(true)
	return swallowMissing(py.Bool(err == nil && fi.Mode().IsRegular()), err)
}

func (p *path) isDirMethod() (py.Object, error) {
	fi, err := p.statInfo(true)
	return swallowMissing(py.Bool(err == nil && fi.Mode().IsDir()), err)
}

func (p *path) isSymlinkMethod() (py.Object, error) {
	fi, err := p.statInfo(false)
	return swallowMissing(py.Bool(err == nil && fi.Mode()&os.ModeSymlink != 0), err)
}

func (p *path) isFIFOMethod() (py.Object, error) {
	fi, err := p.statInfo(true)
	return swallowMissing(py.Bool(err == nil && fi.Mode()&os.ModeNamedPipe != 0), err)
}

func (p *path) isSocketMethod() (py.Object, error) {
	fi, err := p.statInfo(true)
	return swallowMissing(py.Bool(err == nil && fi.Mode()&os.ModeSocket != 0), err)
}

func (p *path) isBlockDeviceMethod() (py.Object, error) {
	fi, err := p.statInfo(true)
	return swallowMissing(py.Bool(err == nil && fi.Mode()&os.ModeDevice != 0 && fi.Mode()&os.ModeCharDevice == 0), err)
}

func (p *path) isCharDeviceMethod() (py.Object, error) {
	fi, err := p.statInfo(true)
	return swallowMissing(py.Bool(err == nil && fi.Mode()&os.ModeCharDevice != 0), err)
}

func (p *path) samefileMethod(other py.Object) (py.Object, error) {
	o, err := p.asPath(other)
	if err != nil {
		return nil, err
	}
	a, b := p.String(), o.String()
	ai, err := os.Stat(a)
	if err != nil {
		return nil, osErr(err, a)
	}
	bi, err := os.Stat(b)
	if err != nil {
		return nil, osErr(err, b)
	}
	return py.Bool(os.SameFile(ai, bi)), nil
}

// ---------------------------------------------------------------------------
// Path methods: absolute forms
// ---------------------------------------------------------------------------

// fromParsed builds a path of the receiver's class from a plain string.
func (p *path) fromParsed(s string) (py.Object, error) {
	return newPath(p.t, p.name, p.f, []py.Object{py.String(s)})
}

func (p *path) resolveMethod(strict bool) (py.Object, error) {
	name := p.String()
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil {
		if strict {
			return nil, osErr(err, name)
		}
		// A path that cannot be resolved yet (it does not exist) is made
		// absolute and normalised, which is what CPython does when strict
		// is false.
		abs, aerr := filepath.Abs(name)
		if aerr != nil {
			return nil, osErr(aerr, name)
		}
		resolved = abs
	}
	return p.fromParsed(resolved)
}

func (p *path) absoluteMethod() (py.Object, error) {
	abs, err := filepath.Abs(p.String())
	if err != nil {
		return nil, osErr(err, p.String())
	}
	return p.fromParsed(abs)
}

func cwdPath(f *flavour, t *py.Type, name string) (py.Object, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, osErr(err, ".")
	}
	return newPath(t, name, f, []py.Object{py.String(dir)})
}

// homeCurrent is Path.home, which always builds a PosixPath (or a WindowsPath)
// rather than using the receiver's class, as CPython does.
func homeCurrent() (py.Object, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "Could not determine home directory.")
	}
	if windowsSupported() {
		return newPath(WindowsPathType, "WindowsPath", ntFlavour, []py.Object{py.String(home)})
	}
	return newPath(PosixPathType, "PosixPath", posixFlavour, []py.Object{py.String(home)})
}

func (p *path) newFromString(s string) (py.Object, error) {
	return newPath(p.t, p.name, p.f, []py.Object{py.String(s)})
}

// expanduserMethod is Path.expanduser.
func (p *path) expanduserMethod() (py.Object, error) {
	if p.drv != "" || p.root != "" || len(p.tail) == 0 || !strings.HasPrefix(p.tail[0], "~") {
		return p, nil
	}
	first := p.tail[0]
	var home string
	if first == "~" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, py.ExceptionNewf(py.RuntimeError, "Could not determine home directory.")
		}
		home = h
	} else {
		// "~user": as os.path.expanduser does, an unknown user leaves the
		// component untouched.
		u, err := user.Lookup(strings.TrimPrefix(first, "~"))
		if err != nil {
			return p, nil
		}
		home = u.HomeDir
	}
	drv, root, tail := p.f.parsePath(home)
	tail = append(tail, p.tail[1:]...)
	return p.fromParts(drv, root, tail), nil
}

// ---------------------------------------------------------------------------
// Path methods: iteration and creation
// ---------------------------------------------------------------------------

func (p *path) iterdirMethod() (py.Object, error) {
	dir := p.String()
	entries, err := dirEntries(dir)
	if err != nil {
		return nil, err
	}
	items := make([]py.Object, 0, len(entries))
	for _, e := range entries {
		child := e.Name()
		if dir != "." {
			child = p.f.join(dir, e.Name())
		}
		c, err := newPath(p.t, p.name, p.f, []py.Object{py.String(child)})
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return py.NewIterator(py.Tuple(items)), nil
}

// walkMethod is Path.walk, a recursive os.walk equivalent.
func (p *path) walkMethod(topDown, followSymlinks bool) py.Object {
	var topLevel []py.Object
	var walk func(dir *path, out *[]py.Object)
	walk = func(dir *path, out *[]py.Object) {
		entries, err := dirEntries(dir.String())
		if err != nil {
			*out = append(*out, py.Tuple{py.String(dir.String()), py.NewList(), py.NewList()})
			return
		}
		var dirs, files []py.Object
		for _, e := range entries {
			child := p.f.join(dir.String(), e.Name())
			if e.IsDir() {
				dirs = append(dirs, py.String(e.Name()))
			} else {
				files = append(files, py.String(e.Name()))
			}
			_ = child
		}
		*out = append(*out, py.Tuple{py.String(dir.String()), py.NewListFromItems(dirs), py.NewListFromItems(files)})
		for _, d := range dirs {
			name, _ := py.StrAsString(d)
			sub := dir.fromParts(dir.drv, dir.root, append(append([]string(nil), dir.tail...), name))
			walk(sub, out)
		}
	}
	var out []py.Object
	walk(p, &out)
	topLevel = out
	return py.NewIterator(py.Tuple(topLevel))
}

func (p *path) mkdirMethod(mode uint32, parents, existOk bool) (py.Object, error) {
	name := p.String()
	if parents {
		err := os.MkdirAll(name, os.FileMode(mode))
		if err != nil {
			return nil, osErr(err, name)
		}
		return py.None, nil
	}
	err := os.Mkdir(name, os.FileMode(mode))
	if err != nil {
		if existOk && errors.Is(err, fs.ErrExist) {
			return py.None, nil
		}
		return nil, osErr(err, name)
	}
	return py.None, nil
}

func (p *path) rmdirMethod() (py.Object, error) {
	if err := os.Remove(p.String()); err != nil {
		return nil, osErr(err, p.String())
	}
	return py.None, nil
}

func (p *path) unlinkMethod(missingOk bool) (py.Object, error) {
	err := os.Remove(p.String())
	if err != nil {
		if missingOk && errors.Is(err, fs.ErrNotExist) {
			return py.None, nil
		}
		return nil, osErr(err, p.String())
	}
	return py.None, nil
}

// touchMethod is Path.touch: create if absent, and set the mtime to now in
// either case.
func (p *path) touchMethod(mode uint32, existOk bool) (py.Object, error) {
	name := p.String()
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY, os.FileMode(mode))
	if err != nil {
		if existOk && errors.Is(err, fs.ErrExist) {
			return py.None, nil
		}
		return nil, osErr(err, name)
	}
	f.Close()
	now := time.Now()
	if err := os.Chtimes(name, now, now); err != nil {
		return nil, osErr(err, name)
	}
	return py.None, nil
}

func (p *path) renameMethod(target py.Object, missingOk bool) (py.Object, error) {
	o, err := p.asPath(target)
	if err != nil {
		return nil, err
	}
	from, to := p.String(), o.String()
	if missingOk && !o.exists() {
		// Only POSIX rename allows a missing destination; on the platforms
		// where it does not, missing_ok has nothing to suppress, so the
		// rename below is left to report the real error.
		_ = from
		_ = to
	}
	if err := os.Rename(from, to); err != nil {
		return nil, osErr(err, to)
	}
	return p.fromParsed(to)
}

func (p *path) chmodMethod(mode uint32, follow bool) (py.Object, error) {
	// lchmod has no portable binding in Go, so a non-following chmod on a
	// symlink is reported honestly rather than silently following the link.
	if !follow {
		fi, err := os.Lstat(p.String())
		if err == nil && fi.Mode()&os.ModeSymlink == 0 {
			follow = true
		}
	}
	if !follow {
		return nil, unsupported("Path.chmod(follow_symlinks=False)")
	}
	if err := os.Chmod(p.String(), os.FileMode(mode)); err != nil {
		return nil, osErr(err, p.String())
	}
	return py.None, nil
}

func (p *path) symlinkToMethod(target py.Object, targetIsDir bool) (py.Object, error) {
	t, err := p.asPath(target)
	if err != nil {
		return nil, err
	}
	if err := os.Symlink(t.String(), p.String()); err != nil {
		return nil, osErr(err, p.String())
	}
	return py.None, nil
}

func (p *path) hardlinkToMethod(target py.Object) (py.Object, error) {
	t, err := p.asPath(target)
	if err != nil {
		return nil, err
	}
	if err := os.Link(t.String(), p.String()); err != nil {
		return nil, osErr(err, p.String())
	}
	return py.None, nil
}

func (p *path) readlinkMethod() (py.Object, error) {
	dest, err := os.Readlink(p.String())
	if err != nil {
		return nil, osErr(err, p.String())
	}
	return p.newFromString(dest)
}

func (p *path) ownerMethod() (py.Object, error) {
	fi, err := p.statInfo(true)
	if err != nil {
		return nil, err
	}
	uid, _, ok := ownerIDs(fi)
	if !ok {
		return nil, unsupported("Path.owner")
	}
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return nil, py.ExceptionNewf(py.KeyError, "uid not found: %d", uid)
	}
	return py.String(u.Username), nil
}

func (p *path) groupMethod() (py.Object, error) {
	fi, err := p.statInfo(true)
	if err != nil {
		return nil, err
	}
	_, gid, ok := ownerIDs(fi)
	if !ok {
		return nil, unsupported("Path.group")
	}
	g, err := user.LookupGroupId(strconv.FormatUint(uint64(gid), 10))
	if err != nil {
		return nil, py.ExceptionNewf(py.KeyError, "gid not found: %d", gid)
	}
	return py.String(g.Name), nil
}

// exists is the Go-side predicate renameMethod uses.
func (p *path) exists() bool {
	_, err := os.Lstat(p.String())
	return err == nil
}

// ---------------------------------------------------------------------------
// Path methods: text and bytes I/O
//
// The interpreter's file object carries no encoding, so text I/O is done here
// over byte slices, which also lets read_text honour the newline argument.
// Encoding names outside the small set Go can decode without a codec registry
// raise LookupError rather than being silently treated as utf-8.
// ---------------------------------------------------------------------------

type codec struct {
	name   string
	encode func(string) ([]byte, error)
	decode func([]byte) (string, error)
}

func lookupCodec(name string) (*codec, error) {
	norm := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
	switch norm {
	case "utf-8", "utf8", "u8", "utf", "cp65001":
		return &codec{
			name:   "utf-8",
			encode: func(s string) ([]byte, error) { return []byte(s), nil },
			decode: func(b []byte) (string, error) { return string(b), nil },
		}, nil
	case "ascii", "us-ascii", "646":
		return &codec{
			name: "ascii",
			encode: func(s string) ([]byte, error) {
				for _, r := range s {
					if r > 0x7f {
						return nil, py.ExceptionNewf(py.UnicodeEncodeError,
							"'ascii' codec can't encode character %s", reprOf(string(r)))
					}
				}
				return []byte(s), nil
			},
			decode: func(b []byte) (string, error) {
				for _, c := range b {
					if c > 0x7f {
						return "", py.ExceptionNewf(py.UnicodeDecodeError,
							"'ascii' codec can't decode byte 0x%02x", c)
					}
				}
				return string(b), nil
			},
		}, nil
	case "latin-1", "latin1", "iso-8859-1", "iso8859-1", "8859", "cp819", "l1":
		// latin-1 is the identity mapping between bytes and code points, so
		// decoding never fails; encoding fails only above U+00FF.
		return &codec{
			name: "latin-1",
			encode: func(s string) ([]byte, error) {
				out := make([]byte, 0, len(s))
				for _, r := range s {
					if r > 0xff {
						return nil, py.ExceptionNewf(py.UnicodeEncodeError,
							"'latin-1' codec can't encode character %s", reprOf(string(r)))
					}
					out = append(out, byte(r))
				}
				return out, nil
			},
			decode: func(b []byte) (string, error) {
				rs := make([]rune, 0, len(b))
				for _, c := range b {
					rs = append(rs, rune(c))
				}
				return string(rs), nil
			},
		}, nil
	case "utf-8-sig":
		inner, _ := lookupCodec("utf-8")
		return &codec{
			name:   "utf-8-sig",
			encode: func(s string) ([]byte, error) { return append([]byte{0xef, 0xbb, 0xbf}, []byte(s)...), nil },
			decode: func(b []byte) (string, error) { return inner.decode(stripBOM(b)) },
		}, nil
	}
	return nil, py.ExceptionNewf(py.LookupError, "unknown encoding: %s", name)
}

func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xef && b[1] == 0xbb && b[2] == 0xbf {
		return b[3:]
	}
	return b
}

// translateNewlines applies CPython's universal newline rules.  newline is
// nil or None for the universal mode, "" for no translation, else "\n", "\r"
// or "\r\n".
func translateNewlines(s string, newline py.Object) string {
	if newline == py.None || newline == nil {
		s = strings.ReplaceAll(s, "\r\n", "\n")
		return strings.ReplaceAll(s, "\r", "\n")
	}
	nl, ok := newline.(py.String)
	if !ok || string(nl) == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if string(nl) == "\n" {
		return s
	}
	return strings.ReplaceAll(s, "\n", string(nl))
}

func encodingName(o py.Object) (string, error) {
	switch v := o.(type) {
	case nil:
		return "utf-8", nil
	case py.NoneType:
		return "utf-8", nil
	case py.String:
		return string(v), nil
	}
	return "", py.ExceptionNewf(py.TypeError, "encoding must be a str, not %s", o.Type().Name)
}

func (p *path) readTextMethod(encoding, newline py.Object) (py.Object, error) {
	name, err := encodingName(encoding)
	if err != nil {
		return nil, err
	}
	c, err := lookupCodec(name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p.String())
	if err != nil {
		return nil, osErr(err, p.String())
	}
	s, err := c.decode(b)
	if err != nil {
		return nil, err
	}
	return py.String(translateNewlines(s, newline)), nil
}

func (p *path) readBytesMethod() (py.Object, error) {
	b, err := os.ReadFile(p.String())
	if err != nil {
		return nil, osErr(err, p.String())
	}
	return py.Bytes(b), nil
}

func (p *path) writeTextMethod(data, encoding, newline py.Object) (py.Object, error) {
	name, err := encodingName(encoding)
	if err != nil {
		return nil, err
	}
	c, err := lookupCodec(name)
	if err != nil {
		return nil, err
	}
	var s string
	switch v := data.(type) {
	case py.String:
		s = string(v)
	case py.Bytes:
		s = string(v)
	default:
		return nil, py.ExceptionNewf(py.TypeError, "write() argument must be str, not %s", data.Type().Name)
	}
	// newline only affects reading; CPython does not translate on write.
	_ = newline
	b, err := c.encode(s)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(p.String(), b, 0o666); err != nil {
		return nil, osErr(err, p.String())
	}
	return py.Int(len([]rune(s))), nil
}

func (p *path) writeBytesMethod(data py.Object) (py.Object, error) {
	var b []byte
	switch v := data.(type) {
	case py.Bytes:
		b = []byte(v)
	case py.String:
		return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not 'str'")
	default:
		return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not %s", data.Type().Name)
	}
	if err := os.WriteFile(p.String(), b, 0o666); err != nil {
		return nil, osErr(err, p.String())
	}
	return py.Int(len(b)), nil
}

// openMethod is Path.open.  The interpreter's file object reads raw bytes and
// decodes as UTF-8, so text mode is only offered for the encodings that agree
// with that; anything else is reported rather than silently mis-decoded.
func (p *path) openMethod(mode, buffering, encoding, errorsArg, newline py.Object) (py.Object, error) {
	modeStr := "r"
	if m, ok := mode.(py.String); ok {
		modeStr = string(m)
	} else if mode != py.None && mode != nil {
		return nil, py.ExceptionNewf(py.TypeError, "open() argument 'mode' must be str, not %s", mode.Type().Name)
	}
	name, err := encodingName(encoding)
	if err != nil {
		return nil, err
	}
	if !strings.ContainsRune(modeStr, 'b') {
		c, err := lookupCodec(name)
		if err != nil {
			return nil, err
		}
		if c.name != "utf-8" && c.name != "utf-8-sig" {
			return nil, unsupported("text mode with encoding " + reprOf(c.name))
		}
	}
	if errorsArg != py.None && errorsArg != nil {
		if e, ok := errorsArg.(py.String); !ok || string(e) == "" {
			return nil, py.ExceptionNewf(py.ValueError, "invalid errors value")
		}
	}
	_ = newline
	buf := -1
	if b, ok := buffering.(py.Int); ok {
		buf = int(b)
	}
	return py.OpenFile(p.String(), modeStr, buf)
}

// statRepr renders a stat result similarly to CPython's repr.  The exact
// spelling is not contract, but a useful one is.
