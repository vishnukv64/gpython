// Package pathlib implements the pathlib module: object oriented filesystem
// paths.
//
// The pure path classes are implemented natively, following CPython's own
// algorithm (posixpath.splitroot / ntpath.splitroot, then _parse_path) for
// turning a path string into a drive, a root and a list of components.  The
// concrete Path classes reuse the same representation and add the calls that
// touch the filesystem.
//
// One deliberate difference from CPython, forced by this interpreter: a path
// cannot be used as a dict key or a set member.  py/dict.go's appendKey is a
// closed type switch over the nine builtin value types, so no type declared
// outside package py can be a dict key - that is equally true of the
// interpreter's own uuid.UUID and of instances of Python classes.  __hash__
// and __eq__ are implemented here, so hash(p) works and p == q is correct for
// any two paths, but {p: v} and {p} raise the interpreter's usual
// "unhashable type" TypeError.  Lifting that needs one case arm in
// py/dict.go, which is outside this package.
//
// Text I/O (read_text, write_text and open in text mode) accepts the encodings
// this interpreter can decode without a codec registry: utf-8, utf-8-sig,
// ascii and latin-1.  Any other name raises LookupError rather than being
// silently treated as utf-8.
package pathlib

import (
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Object-oriented filesystem paths.

This module offers classes representing filesystem paths with semantics
appropriate for different operating systems.  Path classes are divided
between pure paths, which provide purely computational operations without
I/O, and concrete paths, which inherit from pure paths but also provide
I/O operations.`

// ---------------------------------------------------------------------------
// Path flavours
//
// CPython parameterises PurePath by a "parser" module (posixpath or ntpath)
// supplying sep, altsep and splitroot.  The same split is kept here so both
// flavours share one implementation of everything else.
// ---------------------------------------------------------------------------

type flavour struct {
	sep           string
	altsep        string
	caseSensitive bool // false for the Windows flavour: equality is case blind
	posix         bool
}

var (
	posixFlavour = &flavour{sep: "/", altsep: "", caseSensitive: true, posix: true}
	ntFlavour    = &flavour{sep: `\`, altsep: "/", caseSensitive: false, posix: false}
)

// splitDriveRoot is posixpath.splitroot / ntpath.splitroot: split a path into a
// drive, a root, and everything after them.
func (f *flavour) splitDriveRoot(p string) (drv, root, tail string) {
	if f.posix {
		if p == "" || p[0] != '/' {
			// Relative path, e.g. "foo".
			return "", "", p
		}
		if len(p) < 2 || p[1] != '/' || (len(p) > 2 && p[2] == '/') {
			// Absolute path, e.g. "/foo", "///foo", "////foo".
			return "", "/", p[1:]
		}
		// Precisely two leading slashes, e.g. "//foo".  Implementation
		// defined per POSIX.
		return "", p[:2], p[2:]
	}

	// The Windows flavour treats both separators as separators.
	norm := strings.ReplaceAll(p, "/", `\`)
	const uncPrefix = `\\?\UNC\`
	switch {
	case strings.HasPrefix(norm, `\`):
		if len(norm) > 1 && norm[1] == '\\' {
			// UNC drive, "\\server\share", or a device drive,
			// "\\.\device" / "\\?\device".
			start := 2
			if len(norm) >= 8 && strings.EqualFold(norm[:8], uncPrefix) {
				start = 8
			}
			i := strings.IndexByte(norm[start:], '\\')
			if i < 0 {
				return p, "", ""
			}
			i += start
			j := strings.IndexByte(norm[i+1:], '\\')
			if j < 0 {
				return p, "", ""
			}
			j += i + 1
			return p[:j], p[j : j+1], p[j+1:]
		}
		// Relative path with root, e.g. "\Windows".
		return "", p[:1], p[1:]
	case len(norm) > 1 && norm[1] == ':':
		if len(norm) > 2 && norm[2] == '\\' {
			// Absolute drive-letter path, e.g. "X:\Windows".
			return p[:2], p[2:3], p[3:]
		}
		// Relative path with drive, e.g. "X:Windows".
		return p[:2], "", p[2:]
	}
	return "", "", p
}

// parsePath is PurePath._parse_path: normalise the separators, split off the
// drive and the root, and return the remaining components with "." and empty
// components dropped.
func (f *flavour) parsePath(p string) (drv, root string, tail []string) {
	if p == "" {
		return "", "", nil
	}
	if f.altsep != "" {
		p = strings.ReplaceAll(p, f.altsep, f.sep)
	}
	drv, root, rel := f.splitDriveRoot(p)
	if root == "" && strings.HasPrefix(drv, f.sep) && !strings.HasSuffix(drv, f.sep) {
		drvParts := strings.Split(drv, f.sep)
		// "//server/share" and "//?/unc/server/share" are anchored even
		// though splitroot returned the whole thing as the drive.
		if len(drvParts) == 4 && drvParts[2] != "?" && drvParts[2] != "." {
			root = f.sep
		} else if len(drvParts) == 6 {
			root = f.sep
		}
	}
	for _, x := range strings.Split(rel, f.sep) {
		if x != "" && x != "." {
			tail = append(tail, x)
		}
	}
	return drv, root, tail
}

// formatParsedParts is PurePath._format_parsed_parts, the inverse of parsePath.
func (f *flavour) formatParsedParts(drv, root string, tail []string) string {
	if drv != "" || root != "" {
		return drv + root + strings.Join(tail, f.sep)
	}
	if len(tail) > 0 {
		// A leading component that parses as a drive needs a "." in front
		// so that the result does not re-parse as a drive.
		if d, _, _ := f.splitDriveRoot(tail[0]); d != "" {
			return "." + f.sep + strings.Join(tail, f.sep)
		}
	}
	return strings.Join(tail, f.sep)
}

// str is PurePath.__str__: the formatted string, or "." when empty.
func (f *flavour) str(s string) string {
	if s == "" {
		return "."
	}
	return s
}

// join is posixpath.join for the posix flavour, ntpath.join for the Windows
// one.  Both are written out here rather than delegated to stdlib/posixpath,
// whose join predates splitroot and drops earlier components differently.
func (f *flavour) join(a string, rest ...string) string {
	if f.posix {
		path := a
		for _, b := range rest {
			switch {
			case strings.HasPrefix(b, "/") || path == "":
				path = b
			case strings.HasSuffix(path, "/"):
				path += b
			default:
				path += "/" + b
			}
		}
		return path
	}

	resultDrv, resultRoot, resultPath := f.splitDriveRoot(a)
	const seps = `\/`
	const colonSeps = `:\/`
	for _, p := range rest {
		pDrv, pRoot, pPath := f.splitDriveRoot(p)
		if pRoot != "" {
			// Second path is absolute.
			if pDrv != "" || resultDrv == "" {
				resultDrv = pDrv
			}
			resultRoot = pRoot
			resultPath = pPath
			continue
		} else if pDrv != "" && pDrv != resultDrv {
			if !strings.EqualFold(pDrv, resultDrv) {
				// Different drives: the first path is dropped entirely.
				resultDrv, resultRoot, resultPath = pDrv, pRoot, pPath
				continue
			}
			resultDrv = pDrv
		}
		if resultPath != "" && !strings.ContainsRune(seps, rune(resultPath[len(resultPath)-1])) {
			resultPath += `\`
		}
		resultPath += pPath
	}
	if resultPath != "" && resultRoot == "" && resultDrv != "" &&
		!strings.ContainsRune(colonSeps, rune(resultDrv[len(resultDrv)-1])) {
		return resultDrv + `\` + resultPath
	}
	return resultDrv + resultRoot + resultPath
}

// ---------------------------------------------------------------------------
// The path object
// ---------------------------------------------------------------------------

// path is a PurePath.  A concrete Path is the same Go type with a concrete
// python type recorded in t, which is what makes repr, type() and isinstance
// agree with CPython while keeping a single implementation.
type path struct {
	f    *flavour
	t    *py.Type // the python type this instance reports
	name string   // short type name, for repr
	drv  string
	root string
	tail []string
}

var (
	// Every class inherits the one New, pathNew, which is what lets Path
	// choose a concrete flavour at construction time.  The constructors are
	// attached in init() below, because a New that mentions the type
	// variables would make their initialisation circular.
	PurePathType        = py.NewTypeX("pathlib.PurePath", "Base class for manipulating paths without I/O.", nil, nil)
	PurePosixPathType   = PurePathType.NewType("pathlib.PurePosixPath", "PurePath subclass for non-Windows systems.", nil, nil)
	PureWindowsPathType = PurePathType.NewType("pathlib.PureWindowsPath", "PurePath subclass for Windows systems.", nil, nil)
	PathType            = PurePathType.NewType("pathlib.Path", "Base class for manipulating paths with I/O.", nil, nil)
	PosixPathType       = PathType.NewType("pathlib.PosixPath", "Path subclass for non-Windows systems.", nil, nil)
	WindowsPathType     = PathType.NewType("pathlib.WindowsPath", "Path subclass for Windows systems.", nil, nil)
)

func (p *path) Type() *py.Type { return p.t }

func (p *path) String() string {
	return p.f.str(p.f.formatParsedParts(p.drv, p.root, p.tail))
}

// newPath is PurePath.__init__ + _parse_path: every argument contributes its
// path string, the pieces are joined with the flavour's join, and the result
// is parsed.
func newPath(t *py.Type, name string, f *flavour, args []py.Object) (*path, error) {
	if len(args) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "missing argument")
	}
	raws := make([]string, 0, len(args))
	for _, a := range args {
		s, err := fspathString(a)
		if err != nil {
			return nil, err
		}
		raws = append(raws, s)
	}
	drv, root, tail := f.parsePath(f.join(raws[0], raws[1:]...))
	return &path{f: f, t: t, name: name, drv: drv, root: root, tail: tail}, nil
}

// fromParts is PurePath._from_parsed_parts.  Every derived path goes through
// it so a PosixPath stays a PosixPath.
func (p *path) fromParts(drv, root string, tail []string) *path {
	return &path{f: p.f, t: p.t, name: p.name, drv: drv, root: root, tail: tail}
}

// fspathString is os.fspath plus the check that the result is a str.
func fspathString(o py.Object) (string, error) {
	switch v := o.(type) {
	case py.String:
		return string(v), nil
	case py.Bytes:
		return string(v), nil
	case *path:
		return v.String(), nil
	}
	// Anything else must supply __fspath__.
	if res, ok, err := py.TypeCall0(o, "__fspath__"); ok {
		if err != nil {
			return "", err
		}
		return fspathString(res)
	}
	return "", py.ExceptionNewf(py.TypeError,
		"expected str, bytes or os.PathLike object, not %s", o.Type().Name)
}

// asPath coerces a python object to a path of the receiver's flavour.
func (like *path) asPath(o py.Object) (*path, error) {
	if p, ok := o.(*path); ok {
		return p, nil
	}
	s, err := fspathString(o)
	if err != nil {
		return nil, err
	}
	drv, root, tail := like.f.parsePath(s)
	return &path{f: like.f, t: like.t, name: like.name, drv: drv, root: root, tail: tail}, nil
}

// ---------------------------------------------------------------------------
// Type protocol
// ---------------------------------------------------------------------------

func (p *path) M__str__() (py.Object, error)    { return py.String(p.String()), nil }
func (p *path) M__fspath__() (py.Object, error) { return py.String(p.String()), nil }
func (p *path) M__bytes__() (py.Object, error)  { return py.Bytes(p.String()), nil }

func (p *path) M__repr__() (py.Object, error) {
	return py.String(fmt.Sprintf("%s('%s')", p.name, p.String())), nil
}

// strHash is the same FNV-1a the interpreter uses for hash(str), so that
// hash(Path("a")) and hash("a") agree, as they do in CPython (whose path hash
// is the hash of the normalised string).
func strHash(s string) py.Int {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return py.Int(int64(h & (1<<63 - 1)))
}

func (p *path) M__hash__() (py.Object, error) { return strHash(p.normcase()), nil }

// normcase is _str_normcase: the string form, lowercased for the Windows
// flavour.
func (p *path) normcase() string {
	s := p.String()
	if !p.f.caseSensitive {
		s = strings.ToLower(s)
	}
	return s
}

// partsNormcase is _parts_normcase, which the ordering comparisons use.
func (p *path) partsNormcase() []string {
	return strings.Split(p.normcase(), p.f.sep)
}

func (p *path) sameFlavour(o *path) bool { return p.f == o.f }

// pathEq is PurePath.__eq__.
func (p *path) pathEq(o *path) bool {
	return p.sameFlavour(o) && p.normcase() == o.normcase()
}

func (p *path) M__eq__(other py.Object) (py.Object, error) {
	o, ok := other.(*path)
	if !ok {
		return py.NotImplemented, nil
	}
	return py.Bool(p.pathEq(o)), nil
}

func (p *path) M__ne__(other py.Object) (py.Object, error) {
	o, ok := other.(*path)
	if !ok {
		return py.NotImplemented, nil
	}
	return py.Bool(!p.pathEq(o)), nil
}

// compare implements the four ordering methods against _parts_normcase.
func (p *path) compare(other py.Object, want int) (py.Object, error) {
	o, ok := other.(*path)
	if !ok || !p.sameFlavour(o) {
		return py.NotImplemented, nil
	}
	a, b := p.partsNormcase(), o.partsNormcase()
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	c := 0
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				c = -1
			} else {
				c = 1
			}
			break
		}
	}
	if c == 0 {
		switch {
		case len(a) < len(b):
			c = -1
		case len(a) > len(b):
			c = 1
		}
	}
	switch want {
	case -1:
		return py.Bool(c < 0), nil
	case 1:
		return py.Bool(c > 0), nil
	}
	return py.Bool(c == 0), nil
}

func (p *path) M__lt__(o py.Object) (py.Object, error) { return p.compare(o, -1) }
func (p *path) M__le__(o py.Object) (py.Object, error) { return p.compare(o, 0) }
func (p *path) M__gt__(o py.Object) (py.Object, error) { return p.compare(o, 1) }
func (p *path) M__ge__(o py.Object) (py.Object, error) { return p.compare(o, 0) }

// M__truediv__ is the "/" operator.
func (p *path) M__truediv__(other py.Object) (py.Object, error) {
	switch other.(type) {
	case py.String, *path:
		return newPath(p.t, p.name, p.f, []py.Object{p, other})
	}
	// A generic os.PathLike is accepted too, as in CPython.
	if _, ok, _ := py.TypeCall0(other, "__fspath__"); ok {
		return newPath(p.t, p.name, p.f, []py.Object{p, other})
	}
	return py.NotImplemented, nil
}

// M__rtruediv__ handles "str" / Path.
func (p *path) M__rtruediv__(other py.Object) (py.Object, error) {
	if _, ok := other.(py.String); !ok {
		return py.NotImplemented, nil
	}
	return newPath(p.t, p.name, p.f, []py.Object{other, p})
}

// ---------------------------------------------------------------------------
// Pure path properties
// ---------------------------------------------------------------------------

func (p *path) propName() string {
	if len(p.tail) == 0 {
		return ""
	}
	return p.tail[len(p.tail)-1]
}

// sufList is PurePath.suffixes.
func sufList(name string) []string {
	fields := strings.Split(strings.TrimLeft(name, "."), ".")
	out := make([]string, 0, len(fields))
	for _, f := range fields[1:] {
		out = append(out, "."+f)
	}
	return out
}

// sufOne is PurePath.suffix.
func sufOne(name string) string {
	base := strings.TrimLeft(name, ".")
	if i := strings.LastIndexByte(base, '.'); i != -1 {
		return base[i:]
	}
	return ""
}

// stemOf is PurePath.stem: the final component minus its last suffix, except
// that a name which is nothing but dots is its own stem.
func stemOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i != -1 {
		stem := name[:i]
		if strings.Trim(stem, ".") != "" {
			return stem
		}
	}
	return name
}

func (p *path) propParts() []string {
	items := make([]string, 0, len(p.tail)+1)
	if p.drv != "" || p.root != "" {
		items = append(items, p.drv+p.root)
	}
	return append(items, p.tail...)
}

func (p *path) propParent() *path {
	if len(p.tail) == 0 {
		return p
	}
	return p.fromParts(p.drv, p.root, p.tail[:len(p.tail)-1])
}

// propParents is _PathParents: the logical ancestors, nearest first.  A path
// with no components has no parents, so "/a" has exactly the parent "/".
func (p *path) propParents() []*path {
	n := len(p.tail)
	items := make([]*path, 0, n)
	for i := n - 1; i >= 0; i-- {
		items = append(items, p.fromParts(p.drv, p.root, p.tail[:i]))
	}
	return items
}

// isParentOf reports whether q is one of p's logical ancestors.
func (p *path) isParentOf(q *path) bool {
	for _, a := range p.propParents() {
		if a.pathEq(q) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Pure path methods
// ---------------------------------------------------------------------------

func reprOf(s string) string { return fmt.Sprintf("'%s'", s) }

func (p *path) withName(name string) (*path, error) {
	if name == "" || strings.Contains(name, p.f.sep) ||
		(p.f.altsep != "" && strings.Contains(name, p.f.altsep)) || name == "." {
		return nil, py.ExceptionNewf(py.ValueError, "Invalid name %s", reprOf(name))
	}
	if len(p.tail) == 0 {
		return nil, py.ExceptionNewf(py.ValueError, "%s has an empty name", reprOf(p.String()))
	}
	tail := append([]string(nil), p.tail...)
	tail[len(tail)-1] = name
	return p.fromParts(p.drv, p.root, tail), nil
}

// withSuffix is PurePath.with_suffix.
func (p *path) withSuffix(suffix string) (*path, error) {
	stem := stemOf(p.propName())
	switch {
	case stem == "":
		// If the stem is empty the suffix cannot be made non-empty.
		return nil, py.ExceptionNewf(py.ValueError, "%s has an empty name", reprOf(p.String()))
	case suffix != "" && !strings.HasPrefix(suffix, "."):
		return nil, py.ExceptionNewf(py.ValueError, "Invalid suffix %s", reprOf(suffix))
	}
	return p.withName(stem + suffix)
}

// withStem is PurePath.with_stem.
func (p *path) withStem(stem string) (*path, error) {
	suffix := sufOne(p.propName())
	switch {
	case suffix == "":
		return p.withName(stem)
	case stem == "":
		return nil, py.ExceptionNewf(py.ValueError, "%s has a non-empty suffix", reprOf(p.String()))
	}
	return p.withName(stem + suffix)
}

func (p *path) joinpath(args []py.Object) (py.Object, error) {
	return newPath(p.t, p.name, p.f, append([]py.Object{p}, args...))
}

// relativeTo is PurePath.relative_to: walk the other path and its ancestors
// looking for one that equals the receiver or one of its parents.  The number
// of steps taken becomes the count of leading ".." components.
func (p *path) relativeTo(other py.Object, walkUp bool) (py.Object, error) {
	o, err := p.asPath(other)
	if err != nil {
		return nil, err
	}
	candidates := append([]*path{o}, o.propParents()...)
	found := false
	step := 0
	var match *path
	for i, cand := range candidates {
		if cand.pathEq(p) || p.isParentOf(cand) {
			found, step, match = true, i, cand
			break
		}
		if !walkUp {
			return nil, py.ExceptionNewf(py.ValueError,
				"%s is not in the subpath of %s", reprOf(p.String()), reprOf(o.String()))
		}
		if cand.propName() == ".." {
			return nil, py.ExceptionNewf(py.ValueError,
				"'..' segment in %s cannot be walked", reprOf(o.String()))
		}
	}
	if !found {
		return nil, py.ExceptionNewf(py.ValueError,
			"%s and %s have different anchors", reprOf(p.String()), reprOf(o.String()))
	}
	tail := make([]string, 0, step+len(p.tail))
	for i := 0; i < step; i++ {
		tail = append(tail, "..")
	}
	// match is a prefix of p, so its component count indexes the remainder.
	tail = append(tail, p.tail[len(match.tail):]...)
	return p.fromParts("", "", tail), nil
}

func (p *path) isRelativeTo(other py.Object) (py.Object, error) {
	o, err := p.asPath(other)
	if err != nil {
		return nil, err
	}
	return py.Bool(o.pathEq(p) || p.isParentOf(o)), nil
}

// asPosix is PurePath.as_posix.
func (p *path) asPosix() string {
	if p.f.sep == "/" {
		return p.String()
	}
	return strings.ReplaceAll(p.String(), `\`, "/")
}

// asURI is PurePath.as_uri.
func (p *path) asURI() (py.Object, error) {
	if !p.isAbsolute() {
		return nil, py.ExceptionNewf(py.ValueError, "relative path can't be expressed as a file URI")
	}
	drive := p.drv
	var prefix, s string
	switch {
	case len(drive) == 2 && drive[1] == ':':
		// A path on a local drive.
		prefix = "file:///" + drive
		s = p.asPosix()[2:]
	case drive != "":
		// A path on a network drive.
		prefix = "file:"
		s = p.asPosix()
	default:
		prefix = "file://"
		s = p.String()
	}
	return py.String(prefix + quoteFromBytes([]byte(s))), nil
}

// quoteFromBytes is urllib.parse.quote_from_bytes for the default safe set:
// letters, digits, "_.-~" and "/" pass through, everything else is percent
// escaped byte by byte.
func quoteFromBytes(b []byte) string {
	const safe = "_.-~/"
	var sb strings.Builder
	for _, c := range b {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			strings.IndexByte(safe, c) >= 0 {
			sb.WriteByte(c)
			continue
		}
		fmt.Fprintf(&sb, "%%%02X", c)
	}
	return sb.String()
}

// isAbsolute: the path has a root.  For the Windows flavour a drive is not
// required, because ntpath.isabs("\\a") is true.
func (p *path) isAbsolute() bool { return p.root != "" }

func (p *path) anchor() string { return p.drv + p.root }

// ---------------------------------------------------------------------------
// Glob translation
//
// This is glob.translate with include_hidden=True, which is what pathlib uses
// for match, full_match and glob.
// ---------------------------------------------------------------------------

// fnTranslate is fnmatch._translate for the single-character wildcards.
func fnTranslate(pat, star, questionMark string) string {
	var sb strings.Builder
	i, n := 0, len(pat)
	for i < n {
		c := pat[i]
		i++
		switch c {
		case '*':
			sb.WriteString(star)
			for i < n && pat[i] == '*' {
				i++
			}
		case '?':
			sb.WriteString(questionMark)
		case '[':
			j := i
			if j < n && pat[j] == '!' {
				j++
			}
			if j < n && pat[j] == ']' {
				j++
			}
			for j < n && pat[j] != ']' {
				j++
			}
			if j >= n {
				sb.WriteString(`\[`)
				break
			}
			stuff := pat[i:j]
			i = j + 1
			switch {
			case stuff == "":
				// An empty range never matches.
				sb.WriteString("(?!)")
				continue
			case stuff == "!":
				sb.WriteString(".")
				continue
			}
			stuff = strings.ReplaceAll(stuff, `\`, `\\`)
			if stuff[0] == '!' {
				stuff = "^" + stuff[1:]
			} else if stuff[0] == '^' || stuff[0] == '[' {
				stuff = `\` + stuff
			}
			sb.WriteString("[" + stuff + "]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return sb.String()
}

// globTranslate is glob.translate(pat, recursive, include_hidden=True).
func globTranslate(pat string, recursive bool, sep string) string {
	notSep := "[^" + regexp.QuoteMeta(sep) + "]"
	sepRe := regexp.QuoteMeta(sep)
	oneLastSegment := notSep + "+"
	oneSegment := oneLastSegment + sepRe
	anySegments := "(?:.+" + sepRe + ")?"
	anyLastSegments := ".*"

	parts := strings.Split(pat, sep)
	last := len(parts) - 1
	var sb strings.Builder
	for idx, part := range parts {
		switch {
		case part == "*":
			if idx < last {
				sb.WriteString(oneSegment)
			} else {
				sb.WriteString(oneLastSegment)
			}
		case recursive && part == "**":
			if idx < last {
				if parts[idx+1] != "**" {
					sb.WriteString(anySegments)
				}
			} else {
				sb.WriteString(anyLastSegments)
			}
		default:
			if part != "" {
				sb.WriteString(fnTranslate(part, notSep+"*", notSep))
			}
			if idx < last {
				sb.WriteString(sepRe)
			}
		}
	}
	return "(?s:" + sb.String() + `)\z`
}

var globCache = map[string]*regexp.Regexp{}

func globRegex(pat string, recursive, caseSensitive bool, sep string) (*regexp.Regexp, error) {
	key := fmt.Sprintf("%v\x00%v\x00%v\x00%s", pat, recursive, caseSensitive, sep)
	if re, ok := globCache[key]; ok {
		return re, nil
	}
	src := globTranslate(pat, recursive, sep)
	if !caseSensitive {
		src = "(?i)" + src
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, py.ExceptionNewf(py.ValueError, "invalid glob pattern %s", reprOf(pat))
	}
	globCache[key] = re
	return re, nil
}

// matchSegment is _StringGlobber matching one path component against one
// pattern component.
func matchSegment(pat, s string, caseSensitive bool, sep string) (bool, error) {
	re, err := globRegex(pat, false, caseSensitive, sep)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

func matchWhole(pat, s string, caseSensitive bool, sep string) (bool, error) {
	re, err := globRegex(pat, true, caseSensitive, sep)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

// match is PurePath.match: a relative pattern matches from the right, an
// absolute pattern must match the whole path.  "**" is not supported here.
func (like *path) match(other py.Object, caseSensitive py.Object) (py.Object, error) {
	pattern, err := like.asPath(other)
	if err != nil {
		return nil, err
	}
	cs := like.caseSensitivity(caseSensitive)
	pathParts := like.propParts()
	patParts := pattern.propParts()
	if len(patParts) == 0 {
		return nil, py.ExceptionNewf(py.ValueError, "empty pattern")
	}
	if len(pathParts) < len(patParts) {
		return py.False, nil
	}
	if len(pathParts) > len(patParts) && pattern.anchor() != "" {
		return py.False, nil
	}
	for i := 0; i < len(patParts); i++ {
		pp := pathParts[len(pathParts)-1-i]
		pt := patParts[len(patParts)-1-i]
		ok, err := matchSegment(pt, pp, cs, like.f.sep)
		if err != nil {
			return nil, err
		}
		if !ok {
			return py.False, nil
		}
	}
	return py.True, nil
}

func (like *path) caseSensitivity(o py.Object) bool {
	if b, ok := o.(py.Bool); ok {
		return bool(b)
	}
	return like.f.caseSensitive
}

// fullMatch is PurePath.full_match.
func (like *path) fullMatch(other py.Object, caseSensitive py.Object) (py.Object, error) {
	pattern, err := like.asPath(other)
	if err != nil {
		return nil, err
	}
	cs := like.caseSensitivity(caseSensitive)
	// The string form of an empty path is a single dot.  Empty paths should
	// not match wildcards, so "" is used instead.
	s := ""
	if len(like.propParts()) > 0 {
		s = like.String()
	}
	pat := ""
	if len(pattern.propParts()) > 0 {
		pat = pattern.String()
	}
	ok, err := matchWhole(pat, s, cs, like.f.sep)
	if err != nil {
		return nil, err
	}
	return py.Bool(ok), nil
}

// ---------------------------------------------------------------------------
// Directory walking, shared by glob and rglob
// ---------------------------------------------------------------------------

// dirEntries reads a directory sorted by name, so results are deterministic.
func dirEntries(dir string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, osErr(err, dir)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// hasMagic reports whether a glob component contains wildcards.
func hasMagic(s string) bool { return strings.ContainsAny(s, "*?[") }

// parsePattern is PurePath._parse_pattern: the pattern must be relative, and a
// trailing separator is preserved as a final empty component.
func (p *path) parsePattern(pattern string) ([]string, error) {
	f := p.f
	drv, root, rel := f.splitDriveRoot(pattern)
	if drv != "" || root != "" {
		return nil, py.ExceptionNewf(py.NotImplementedError, "Non-relative patterns are unsupported")
	}
	if f.altsep != "" {
		rel = strings.ReplaceAll(rel, f.altsep, f.sep)
	}
	var parts []string
	for _, x := range strings.Split(rel, f.sep) {
		if x != "" && x != "." {
			parts = append(parts, x)
		}
	}
	if len(parts) == 0 {
		return nil, py.ExceptionNewf(py.ValueError, "Unacceptable pattern: %s", reprOf(pattern))
	}
	if strings.HasSuffix(rel, f.sep) {
		// GH-65238: a trailing slash means directories only.
		parts = append(parts, "")
	}
	return parts, nil
}

// globParts walks base collecting every descendant that matches parts.  A
// trailing empty component selects directories.
func (p *path) globParts(base string, parts []string, out *[]string, cs bool) error {
	if len(parts) == 0 {
		*out = append(*out, base)
		return nil
	}
	part, rest := parts[0], parts[1:]
	joinName := func(dir, name string) string {
		if dir == "" || dir == "." {
			return name
		}
		return dir + p.f.sep + name
	}
	takeDir := func(target string, e fs.DirEntry) error {
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			// A symlink to a directory still counts as a directory for a
			// trailing-slash pattern.
			if st, err := os.Stat(target); err == nil {
				isDir = st.IsDir()
			}
		}
		if isDir {
			*out = append(*out, target)
		}
		return nil
	}

	switch {
	case part == "":
		// Only reachable as the final component, from parsePattern.
		target := base
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			*out = append(*out, target)
		}
		return nil

	case part == "**":
		// Zero directories consumed.
		if err := p.globParts(base, rest, out, cs); err != nil {
			return err
		}
		entries, err := dirEntries(base)
		if err != nil {
			if isNotExist(err) {
				return nil
			}
			return err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			target := joinName(base, e.Name())
			// A symlinked directory is not descended into, matching
			// pathlib's default recurse_symlinks=False.
			if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if err := p.globParts(target, parts, out, cs); err != nil {
				return err
			}
		}
		return nil

	case !hasMagic(part):
		target := joinName(base, part)
		if _, err := os.Lstat(target); err != nil {
			return nil
		}
		if len(rest) == 0 {
			*out = append(*out, target)
			return nil
		}
		return p.globParts(target, rest, out, cs)
	}

	entries, err := dirEntries(base)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		return err
	}
	lastIsEmpty := len(rest) == 1 && rest[0] == ""
	for _, e := range entries {
		ok, err := matchSegment(part, e.Name(), cs, p.f.sep)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		target := joinName(base, e.Name())
		switch {
		case lastIsEmpty:
			if err := takeDir(target, e); err != nil {
				return err
			}
		case len(rest) == 0:
			*out = append(*out, target)
		case e.IsDir():
			if err := p.globParts(target, rest, out, cs); err != nil {
				return err
			}
		}
	}
	return nil
}

func isNotExist(err error) bool {
	if e, ok := err.(*py.Exception); ok {
		return e.Base.IsSubtype(py.FileNotFoundError) || e.Base.IsSubtype(py.NotADirectoryError)
	}
	return false
}

// glob is Path.glob.
func (p *path) glob(pattern string, caseSensitive py.Object) (py.Object, error) {
	parts, err := p.parsePattern(pattern)
	if err != nil {
		return nil, err
	}
	cs := p.caseSensitivity(caseSensitive)
	var found []string
	if err := p.globParts(p.String(), parts, &found, cs); err != nil {
		return nil, err
	}
	items := make([]py.Object, 0, len(found))
	for _, s := range found {
		child, err := newPath(p.t, p.name, p.f, []py.Object{py.String(s)})
		if err != nil {
			return nil, err
		}
		items = append(items, child)
	}
	return py.NewIterator(py.Tuple(items)), nil
}

// rglob is Path.rglob: prepend "**".
func (p *path) rglob(pattern string, caseSensitive py.Object) (py.Object, error) {
	return p.glob(p.f.join("**", pattern), caseSensitive)
}
