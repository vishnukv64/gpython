// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"fmt"
	"os"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// StatResult is os.stat_result.
//
// Like sys.version_info it is a struct sequence: a 10-item tuple - mode, ino,
// dev, nlink, uid, gid, size, and the three times as WHOLE seconds - whose
// fields also have names, where the st_*time names are FLOATS and st_*time_ns
// are exact.  pathlib had a names-only version (no len, no indexing, a repr
// that stopped at st_size); this one serves both, so os.stat(p) and
// Path(p).stat() return the same thing, as they do in CPython.
type StatResult struct {
	Dict py.StringDict
	seq  [10]py.Object
}

// StatResultType is os.stat_result's type.
var StatResultType = py.NewTypeX("stat_result",
	"stat_result: Result from stat, fstat, or lstat.", nil, nil)

func (s *StatResult) Type() *py.Type { return StatResultType }

func (s *StatResult) GetDict() py.StringDict { return s.Dict }

var _ py.IGetDict = (*StatResult)(nil)

var statSeqNames = [10]string{"st_mode", "st_ino", "st_dev", "st_nlink", "st_uid",
	"st_gid", "st_size", "st_atime", "st_mtime", "st_ctime"}

// statFields is what every platform can supply; the per-OS files fill it from
// whatever the operating system returned.
type statFields struct {
	mode                      uint32
	ino, dev, nlink           uint64
	uid, gid                  uint32
	size                      int64
	atimeS, mtimeS, ctimeS    int64 // whole seconds
	atimeNs, mtimeNs, ctimeNs int64 // the nanosecond REMAINDER, 0..999999999
	// Present only where the system reports them, so only those platforms
	// expose st_rdev, st_blocks and st_blksize - as CPython does.
	rdev, blocks, blksize int64
	hasBlocks             bool
}

func newStatResult(f statFields) *StatResult {
	s := &StatResult{Dict: py.NewStringDict()}
	s.seq = [10]py.Object{
		py.Int(f.mode), py.Int(f.ino), py.Int(f.dev), py.Int(f.nlink),
		py.Int(f.uid), py.Int(f.gid), py.Int(f.size),
		py.Int(f.atimeS), py.Int(f.mtimeS), py.Int(f.ctimeS),
	}
	for i, name := range statSeqNames[:7] {
		s.Dict.Set(name, s.seq[i])
	}
	// CPython computes the float as sec + nsec*1e-9 (posixmodule.c fill_time);
	// dividing the total nanoseconds by 1e9 differs in the last bit.
	for _, t := range []struct {
		name  string
		s, ns int64
	}{{"st_atime", f.atimeS, f.atimeNs}, {"st_mtime", f.mtimeS, f.mtimeNs}, {"st_ctime", f.ctimeS, f.ctimeNs}} {
		s.Dict.Set(t.name, py.Float(float64(t.s)+float64(t.ns)*1e-9))
		s.Dict.Set(t.name+"_ns", py.Int(t.s*1_000_000_000+t.ns))
	}
	if f.hasBlocks {
		s.Dict.Set("st_rdev", py.Int(f.rdev))
		s.Dict.Set("st_blocks", py.Int(f.blocks))
		s.Dict.Set("st_blksize", py.Int(f.blksize))
	}
	return s
}

// NewStatResult builds a stat_result from what os.Stat or os.Lstat returned.
// It is exported for pathlib, whose Path.stat() is the same object.
func NewStatResult(fi os.FileInfo) *StatResult {
	if f, ok := sysStatFields(fi); ok {
		return newStatResult(f)
	}
	// Where the platform's raw stat is not available (Windows), the portable
	// FileInfo still gives size, mode and modification time.
	f := statFields{size: fi.Size(), mode: pyMode(fi.Mode())}
	mt := fi.ModTime()
	f.mtimeS, f.mtimeNs = mt.Unix(), int64(mt.Nanosecond())
	f.atimeS, f.atimeNs, f.ctimeS, f.ctimeNs = f.mtimeS, f.mtimeNs, f.mtimeS, f.mtimeNs
	return newStatResult(f)
}

// pyMode converts Go's FileMode to the st_mode bits Python's stat module reads:
// the permission bits plus the S_IF* file-type bits.
func pyMode(m os.FileMode) uint32 {
	mode := uint32(m.Perm())
	switch {
	case m.IsDir():
		mode |= 0o040000
	case m&os.ModeSymlink != 0:
		mode |= 0o120000
	case m&os.ModeNamedPipe != 0:
		mode |= 0o010000
	case m&os.ModeSocket != 0:
		mode |= 0o140000
	case m&os.ModeCharDevice != 0:
		mode |= 0o020000
	case m&os.ModeDevice != 0:
		mode |= 0o060000
	default:
		mode |= 0o100000
	}
	if m&os.ModeSetuid != 0 {
		mode |= 0o4000
	}
	if m&os.ModeSetgid != 0 {
		mode |= 0o2000
	}
	if m&os.ModeSticky != 0 {
		mode |= 0o1000
	}
	return mode
}

func (s *StatResult) M__len__() (py.Object, error) { return py.Int(len(s.seq)), nil }

func (s *StatResult) M__iter__() (py.Object, error) {
	return py.NewListFromItems(s.seq[:]).M__iter__()
}

func (s *StatResult) M__getitem__(key py.Object) (py.Object, error) {
	return py.Tuple(s.seq[:]).M__getitem__(key)
}

// M__eq__ compares against a PLAIN tuple too: a struct sequence is a tuple, so
// "os.stat(p) == tuple(os.stat(p))" is True in CPython.
func (s *StatResult) M__eq__(other py.Object) (py.Object, error) {
	if o, ok := other.(*StatResult); ok {
		other = py.Tuple(o.seq[:])
	}
	if _, ok := other.(py.Tuple); !ok {
		return py.NotImplemented, nil
	}
	return py.Eq(py.Tuple(s.seq[:]), other)
}

func (s *StatResult) M__ne__(other py.Object) (py.Object, error) {
	eq, err := s.M__eq__(other)
	if err != nil || eq == py.NotImplemented {
		return eq, err
	}
	return py.Not(eq)
}

func (s *StatResult) M__hash__() (py.Object, error) {
	if h, ok := py.HashValue(py.Tuple(s.seq[:])); ok {
		return py.Int(h), nil
	}
	return py.NotImplemented, nil
}

// M__repr__ shows the ten sequence fields, the times as the whole seconds the
// sequence holds - CPython's structseq repr reads the sequence, not the floats.
func (s *StatResult) M__repr__() (py.Object, error) {
	var b strings.Builder
	b.WriteString("os.stat_result(")
	for i, name := range statSeqNames {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s=%d", name, int64(s.seq[i].(py.Int)))
	}
	b.WriteString(")")
	return py.String(b.String()), nil
}

// fsPath is the path an os function was given: a str, or anything with
// __fspath__ (pathlib.Path), as os.fspath accepts.
func fsPath(o py.Object) (string, error) {
	if s, ok := o.(py.String); ok {
		return string(s), nil
	}
	if fn, err := py.GetAttrString(o, "__fspath__"); err == nil {
		r, err := py.Call(fn, nil, py.NewStringDict())
		if err != nil {
			return "", err
		}
		if s, ok := r.(py.String); ok {
			return string(s), nil
		}
	}
	return "", py.ExceptionNewf(py.TypeError,
		"stat: path should be string, bytes, os.PathLike or integer, not %s", o.Type().Name)
}

func statCall(name string, follow bool, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var path py.Object
	var dirFd py.Object = py.None
	var followArg py.Object = py.True
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|$OO:"+name,
		[]string{"path", "dir_fd", "follow_symlinks"}, &path, &dirFd, &followArg); err != nil {
		return nil, err
	}
	if dirFd != py.None {
		return nil, py.ExceptionNewf(py.NotImplementedError, "%s(dir_fd=...) is not supported", name)
	}
	if f, ok := followArg.(py.Bool); ok && !bool(f) {
		follow = false
	}
	// An integer is a file descriptor: os.stat(fd) is os.fstat(fd).
	if fd, ok := path.(py.Int); ok {
		return fstatResult(int(fd))
	}
	p, err := fsPath(path)
	if err != nil {
		return nil, err
	}
	var fi os.FileInfo
	if follow {
		fi, err = os.Stat(p)
	} else {
		fi, err = os.Lstat(p)
	}
	if err != nil {
		return nil, py.OSErrorFrom(err, p)
	}
	return NewStatResult(fi), nil
}

func osStat(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return statCall("stat", true, args, kwargs)
}

func osLstat(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return statCall("lstat", false, args, kwargs)
}

func osFstat(self py.Object, args py.Tuple) (py.Object, error) {
	var fd py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "fstat", 1, 1, &fd); err != nil {
		return nil, err
	}
	n, err := py.IndexInt(fd)
	if err != nil {
		return nil, err
	}
	return fstatResult(n)
}

// The module goes in __module__, not in the name: CPython's type(x).__name__ is the bare name and type(x).__module__ is 'os'.
func init() {
	StatResultType.Dict.Set("__module__", py.String("os"))
}
