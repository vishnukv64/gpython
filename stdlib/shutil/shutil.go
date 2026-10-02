// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package shutil provides the implementation of python's 'shutil' module.
//
// This is the subset a program actually calls: the PATH search, the copy
// family, the tree operations and the terminal/disk queries.  The archive
// helpers (make_archive and the _make_tarball/_make_zipfile behind it) are
// NOT implemented: they need the zipfile and tarfile writers to be driven
// from paths in a way this module cannot do honestly, so make_archive is
// absent rather than present-and-wrong.  Code that calls it gets
// AttributeError, which is what a missing function should do.
//
// Two details of the environment this runs in:
//
//   - os has no os.access and no F_OK/X_OK here, so which() implements the
//     access check from the permission bits of os.Stat and exports F_OK,
//     X_OK, R_OK and W_OK itself.  This checks the mode bits only: it does
//     not consult the effective uid/gid or an ACL, so a file that is
//     executable only for another user answers differently from CPython.
//
//   - the interpreter has no os.terminal_size and no os.stat_result, so
//     get_terminal_size defines the object it returns, as pathlib does for
//     os.stat_result.  Its repr and its .columns/.lines names match CPython.

package shutil

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Utility functions for copying and archiving files and directory trees.

Implemented: which, get_terminal_size, copyfileobj, copyfile, copymode,
copystat, copy, copy2, copytree, move, rmtree, disk_usage, Error and
SameFileError.

Not implemented: make_archive (and register_archive_format,
unregister_archive_format and get_archive_formats), chown, ignore_patterns.

The access check behind which() reads the permission bits from os.Stat
rather than the effective access of the process, and it does not consult an
ACL.`

// Access modes, as os.access takes them.  They live here because this
// interpreter's os module does not export them; the values are the POSIX
// ones, which is what shutil's default mode argument means.
const (
	F_OK = 0
	X_OK = 1
	W_OK = 2
	R_OK = 4
)

// CopyBufSize is the default buffer copyfileobj and copyfile use.
const CopyBufSize = 64 * 1024

// Error is raised when a copy or a tree operation has to report more than
// one failure, and it is the base of SameFileError.  Callers catch it by
// name, so it has to exist as a real subclass of OSError.
var Error = py.OSError.NewType("shutil.Error", "Base class for all errors raised by shutil.", nil, nil)

// SameFileError is raised by copyfile when source and destination are the
// same file, which would truncate the source.
var SameFileError = Error.NewType("shutil.SameFileError", "Raised when the source and destination are the same file.", nil, nil)

// ---------------------------------------------------------------------------
// paths

// fspath converts a path argument to a Go string, honouring the os.PathLike
// protocol the way CPython's os.fspath does.  Bytes are accepted as they are,
// because on this platform a bytes path is a path.
func fspath(o py.Object) (string, error) {
	switch v := o.(type) {
	case py.String:
		return string(v), nil
	case py.Bytes:
		return string(v), nil
	}
	// A generic path-like object (pathlib.Path, or anything defining
	// __fspath__) answers with its own string.
	if res, ok, err := py.TypeCall0(o, "__fspath__"); ok {
		if err != nil {
			return "", err
		}
		switch v := res.(type) {
		case py.String:
			return string(v), nil
		case py.Bytes:
			return string(v), nil
		}
	}
	return "", py.ExceptionNewf(py.TypeError,
		"expected str, bytes or os.PathLike object, not %s", o.Type().Name)
}

// pathArg unpacks args[pos] as a path.
func pathArg(args py.Tuple, pos int, name string) (string, error) {
	if pos >= len(args) {
		return "", py.ExceptionNewf(py.TypeError, "%s() missing required argument", name)
	}
	return fspath(args[pos])
}

// oserr converts a Go file-system error to the Python exception CPython
// would raise for the same condition, so that "except FileNotFoundError"
// written against CPython catches it here.
func oserr(err error) *py.Exception {
	if err == nil {
		return nil
	}
	var pathErr *os.PathError
	name := ""
	if errors.As(err, &pathErr) {
		name = pathErr.Path
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return py.ExceptionNewf(py.FileNotFoundError, "%s: %s", name, sysMsg(pathErr, err))
	case errors.Is(err, fs.ErrPermission):
		return py.ExceptionNewf(py.PermissionError, "%s: %s", name, sysMsg(pathErr, err))
	case errors.Is(err, fs.ErrExist):
		return py.ExceptionNewf(py.FileExistsError, "%s: %s", name, sysMsg(pathErr, err))
	}
	return py.ExceptionNewf(py.OSError, "%s: %s", name, err.Error())
}

// ostat stats a path, following a symlink only when asked, so that copyfile
// and copystat can be told not to dereference one.
func ostat(path string, followSymlinks py.Object) (os.FileInfo, error) {
	follow, err := py.ObjectIsTrue(followSymlinks)
	if err != nil {
		return nil, err
	}
	var fi os.FileInfo
	if follow {
		fi, err = os.Stat(path)
	} else {
		fi, err = os.Lstat(path)
	}
	if err != nil {
		return nil, oserr(err)
	}
	return fi, nil
}

// copystatTimes applies a source's timestamps to dst.
//
// os.FileInfo exposes the modification time but not the access time without
// reaching into syscall.Stat_t, which is not portable across the platforms
// this interpreter builds for.  Both timestamps are therefore set to the
// source's mtime: CPython preserves atime separately, and a caller that
// reads atime back sees a difference.
func copystatTimes(fi os.FileInfo, dst string) error {
	if err := os.Chtimes(dst, fi.ModTime(), fi.ModTime()); err != nil {
		return oserr(err)
	}
	return nil
}

// sysMsg returns the underlying message of a PathError, falling back to the
// error itself when the unwrap is not possible.
func sysMsg(pathErr *os.PathError, err error) string {
	if pathErr != nil && pathErr.Err != nil {
		return pathErr.Err.Error()
	}
	return err.Error()
}

// sameFile reports whether two paths name the same file, which is what
// copyfile has to refuse before it truncates its source.
func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

// ---------------------------------------------------------------------------
// which

// which is shutil.which: a PATH search for an executable.
//
// cmd may contain a directory part, in which case it is looked up directly
// rather than on PATH - including relative to the current directory.  path
// overrides os.environ["PATH"].  A directory is never a result, as in
// CPython's _access_check.
func which(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var cmdObj py.Object
	mode := py.Object(py.Int(F_OK | X_OK))
	pathObj := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:which",
		[]string{"cmd", "mode", "path"}, &cmdObj, &mode, &pathObj); err != nil {
		return nil, err
	}
	cmd, err := fspath(cmdObj)
	if err != nil {
		return nil, err
	}
	modeInt, err := py.IndexInt(mode)
	if err != nil {
		return nil, err
	}

	var dirs []string
	if dirname := filepath.Dir(cmd); dirname != filepath.Dir("") {
		// The command carries a directory part, so the search is that one
		// directory and nothing else.
		dirs = []string{dirname}
	} else {
		searchPath := os.Getenv("PATH")
		if pathObj != nil && pathObj != py.None {
			searchPath, err = fspath(pathObj)
			if err != nil {
				return nil, err
			}
		} else if searchPath == "" {
			// PATH unset and no override: CPython falls back to os.defpath.
			searchPath = "/bin:/usr/bin"
		}
		if searchPath == "" {
			// An empty PATH matches nothing, unlike PATH=':'.  CPython made
			// this distinction deliberately.
			return py.None, nil
		}
		dirs = strings.Split(searchPath, string(os.PathListSeparator))
		// A relative entry in PATH, and the empty entry that a leading or
		// trailing separator produces, both mean the current directory.
		for i, d := range dirs {
			if d == "" {
				dirs[i] = "."
			}
		}
	}

	cmd = filepath.Base(cmd)
	seen := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		norm := filepath.Clean(dir)
		if seen[norm] {
			continue
		}
		seen[norm] = true
		candidate := filepath.Join(dir, cmd)
		if modeCheck(candidate, modeInt) {
			return py.String(candidate), nil
		}
	}
	return py.None, nil
}

// modeCheck reports whether path satisfies mode, standing in for
// os.access(name, mode) plus the "not a directory" test of CPython's
// _access_check.
//
// It reads the permission bits: the owner's execute bit is not distinguished
// from anyone else's, so a file that is executable only for another user is
// treated as executable here.  The F_OK bit alone is checked against
// existence.
func modeCheck(path string, mode int) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		// CPython's _access_check excludes directories even when every
		// requested bit is set.
		return false
	}
	if mode == F_OK {
		return true
	}
	perm := int(fi.Mode().Perm())
	if mode&R_OK != 0 && perm&0o444 == 0 {
		return false
	}
	if mode&W_OK != 0 && perm&0o222 == 0 {
		return false
	}
	if mode&X_OK != 0 && perm&0o111 == 0 {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// copying

// copyfileobj is the byte pump behind the copy functions.  fsrc and fdst are
// file-like objects; length is the buffer size, and the default 0 is
// replaced by CopyBufSize.
func copyfileobj(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fsrc, fdst py.Object
	length := py.Object(py.Int(0))
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|O:copyfileobj",
		[]string{"fsrc", "fdst", "length"}, &fsrc, &fdst, &length); err != nil {
		return nil, err
	}
	bufLen, err := py.IndexInt(length)
	if err != nil {
		return nil, err
	}
	if bufLen <= 0 {
		bufLen = CopyBufSize
	}
	readFn, err := py.GetAttrString(fsrc, "read")
	if err != nil {
		return nil, err
	}
	writeFn, err := py.GetAttrString(fdst, "write")
	if err != nil {
		return nil, err
	}
	for {
		chunk, err := py.Call(readFn, py.Tuple{py.Int(bufLen)}, py.StringDict{})
		if err != nil {
			return nil, err
		}
		var data py.Object
		switch v := chunk.(type) {
		case py.Bytes:
			if len(v) == 0 {
				return py.None, nil
			}
			data = v
		case py.String:
			if len(v) == 0 {
				return py.None, nil
			}
			data = v
		default:
			return nil, py.ExceptionNewf(py.TypeError,
				"read() returned %s, not bytes or str", chunk.Type().Name)
		}
		if _, err := py.Call(writeFn, py.Tuple{data}, py.StringDict{}); err != nil {
			return nil, err
		}
	}
}

// openBinary opens a file for the copy helpers, mapping the error.
func openBinary(path string, flags int, perm os.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, flags, perm)
	if err != nil {
		return nil, oserr(err)
	}
	return f, nil
}

// copyfile copies the contents of src to dst, creating or truncating dst.
// follow_symlinks controls whether a symlink src is dereferenced.
func copyfile(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var srcObj, dstObj py.Object
	followSymlinks := py.Object(py.True)
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|O:copyfile",
		[]string{"src", "dst", "follow_symlinks"}, &srcObj, &dstObj, &followSymlinks); err != nil {
		return nil, err
	}
	src, err := fspath(srcObj)
	if err != nil {
		return nil, err
	}
	dst, err := fspath(dstObj)
	if err != nil {
		return nil, err
	}
	follow, err := py.ObjectIsTrue(followSymlinks)
	if err != nil {
		return nil, err
	}
	if follow && sameFile(src, dst) {
		return nil, py.ExceptionNewf(SameFileError,
			"%s and %s are the same file", src, dst)
	}
	// Refuse to copy a named pipe or a device: reading one can block forever,
	// and CPython raises SpecialFileError for it.
	if fi, err := os.Stat(src); err == nil {
		if fi.Mode()&(os.ModeNamedPipe|os.ModeDevice|os.ModeSocket) != 0 {
			return nil, py.ExceptionNewf(Error, "%s is a named pipe, socket or device file", src)
		}
	} else {
		return nil, oserr(err)
	}

	in, err := openBinary(src, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	out, err := openBinary(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return nil, err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return nil, oserr(copyErr)
	}
	if closeErr != nil {
		return nil, oserr(closeErr)
	}
	return py.None, nil
}

// copymode copies the permission bits of src to dst.
func copymode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var srcObj, dstObj py.Object
	followSymlinks := py.Object(py.True)
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|O:copymode",
		[]string{"src", "dst", "follow_symlinks"}, &srcObj, &dstObj, &followSymlinks); err != nil {
		return nil, err
	}
	src, err := fspath(srcObj)
	if err != nil {
		return nil, err
	}
	dst, err := fspath(dstObj)
	if err != nil {
		return nil, err
	}
	fi, err := ostat(src, followSymlinks)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(dst, fi.Mode().Perm()); err != nil {
		return nil, oserr(err)
	}
	return py.None, nil
}

// copystat copies the permission bits and the timestamps of src to dst.
func copystat(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var srcObj, dstObj py.Object
	followSymlinks := py.Object(py.True)
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|O:copystat",
		[]string{"src", "dst", "follow_symlinks"}, &srcObj, &dstObj, &followSymlinks); err != nil {
		return nil, err
	}
	src, err := fspath(srcObj)
	if err != nil {
		return nil, err
	}
	dst, err := fspath(dstObj)
	if err != nil {
		return nil, err
	}
	fi, err := ostat(src, followSymlinks)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(dst, fi.Mode().Perm()); err != nil {
		return nil, oserr(err)
	}
	if st, err := ostat(src, followSymlinks); err == nil {
		if err := copystatTimes(st, dst); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	return py.None, nil
}

// copy is copyfile plus copymode: it copies the contents and the permission
// bits, but not the timestamps.
func copy(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	src, dst, err := copyArgs(args, kwargs, "copy")
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(src); err == nil && fi.IsDir() {
		return nil, py.ExceptionNewf(py.IsADirectoryError,
			"`%s` is a directory (or a symlink to a directory)", src)
	}
	dst, err = adjustDst(src, dst)
	if err != nil {
		return nil, err
	}
	if _, err := copyfile(self, py.Tuple{py.String(src), py.String(dst)},
		py.NewStringDictFrom(py.DictEntry{Key: "follow_symlinks", Value: py.True})); err != nil {
		return nil, err
	}
	if _, err := copymode(self, py.Tuple{py.String(src), py.String(dst)}, py.NewStringDict()); err != nil {
		return nil, err
	}
	return py.String(dst), nil
}

// copy2 is copyfile plus copystat: the contents, the permission bits and the
// timestamps.
func copy2(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	src, dst, err := copyArgs(args, kwargs, "copy2")
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(src); err == nil && fi.IsDir() {
		return nil, py.ExceptionNewf(py.IsADirectoryError,
			"`%s` is a directory (or a symlink to a directory)", src)
	}
	dst, err = adjustDst(src, dst)
	if err != nil {
		return nil, err
	}
	if _, err := copyfile(self, py.Tuple{py.String(src), py.String(dst)},
		py.NewStringDictFrom(py.DictEntry{Key: "follow_symlinks", Value: py.True})); err != nil {
		return nil, err
	}
	if _, err := copystat(self, py.Tuple{py.String(src), py.String(dst)}, py.NewStringDict()); err != nil {
		return nil, err
	}
	return py.String(dst), nil
}

// copyArgs unpacked the (src, dst) pair shared by copy and copy2.
func copyArgs(args py.Tuple, kwargs py.StringDict, name string) (string, string, error) {
	var srcObj, dstObj py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO:"+name,
		[]string{"src", "dst"}, &srcObj, &dstObj); err != nil {
		return "", "", err
	}
	src, err := fspath(srcObj)
	if err != nil {
		return "", "", err
	}
	dst, err := fspath(dstObj)
	if err != nil {
		return "", "", err
	}
	return src, dst, nil
}

// adjustDst turns "copy a file into this directory" into the full
// destination name.
func adjustDst(src, dst string) (string, error) {
	fi, err := os.Stat(dst)
	if err != nil || !fi.IsDir() {
		return dst, nil
	}
	return filepath.Join(dst, filepath.Base(src)), nil
}

// ---------------------------------------------------------------------------
// copytree

// copytree copies the directory tree at src to dst.
func copytree(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var srcObj, dstObj py.Object
	symlinks := py.Object(py.False)
	ignore := py.Object(py.None)
	copyFunction := py.Object(py.None)
	dirsExistOK := py.Object(py.False)
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|OOOO:copytree",
		[]string{"src", "dst", "symlinks", "ignore", "copy_function", "dirs_exist_ok"},
		&srcObj, &dstObj, &symlinks, &ignore, &copyFunction, &dirsExistOK); err != nil {
		return nil, err
	}
	// ignore_dangling_symlinks is deliberately not accepted: this
	// implementation would have to honour it, and accepting a keyword it
	// ignores would be worse than raising for it.
	_ = ignore
	src, err := fspath(srcObj)
	if err != nil {
		return nil, err
	}
	dst, err := fspath(dstObj)
	if err != nil {
		return nil, err
	}
	symlinksOn, err := py.ObjectIsTrue(symlinks)
	if err != nil {
		return nil, err
	}
	dirsOK, err := py.ObjectIsTrue(dirsExistOK)
	if err != nil {
		return nil, err
	}
	if !dirsOK {
		if _, err := os.Lstat(dst); err == nil {
			return nil, py.ExceptionNewf(py.FileExistsError, "destination path '%s' already exists", dst)
		}
	}
	if _, err := os.Stat(src); err != nil {
		return nil, oserr(err)
	}
	if err := os.MkdirAll(dst, 0o777); err != nil {
		return nil, oserr(err)
	}
	c := &treeCopier{
		self:     self,
		symlinks: symlinksOn,
		copyFn:   copyFunction,
	}
	if ignore != py.None {
		c.ignore = ignore
	}
	if err := c.copyTree(src, dst); err != nil {
		return nil, err
	}
	if len(c.errors) != 0 {
		// CPython collects every failure and raises a single Error whose
		// argument is the list of (src, dst, why) triples.
		items := make([]py.Object, len(c.errors))
		for i, e := range c.errors {
			items[i] = py.Tuple{py.String(e.src), py.String(e.dst), py.String(e.why)}
		}
		repr, err := py.ReprAsString(py.NewListFromItems(items))
		if err != nil {
			return nil, err
		}
		return nil, py.ExceptionNewf(Error, "%s", repr)
	}
	return py.String(dst), nil
}

// treeError is one (src, dst, why) triple as copytree reports it.
type treeError struct {
	src, dst, why string
}

// treeCopier carries the options and the collected failures of one copytree.
type treeCopier struct {
	self     py.Object
	symlinks bool
	copyFn   py.Object
	ignore   py.Object
	errors   []treeError
}

func (c *treeCopier) add(src, dst string, why error) {
	c.errors = append(c.errors, treeError{src: src, dst: dst, why: why.Error()})
}

// names returns the entries of a directory, honouring an ignore callable.
func (c *treeCopier) names(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if c.ignore == nil {
		return names, nil
	}
	// ignore(src, names) returns the subset to skip.
	items := make([]py.Object, len(names))
	for i, n := range names {
		items[i] = py.String(n)
	}
	res, err := py.Call(c.ignore, py.Tuple{py.String(dir), py.NewListFromItems(items)}, py.StringDict{})
	if err != nil {
		return nil, err
	}
	skip, err := py.SequenceList(res)
	if err != nil {
		return nil, err
	}
	keep := make([]string, 0, len(names))
	for _, n := range names {
		drop := false
		for _, s := range skip.Items {
			str, ok := s.(py.String)
			if ok && string(str) == n {
				drop = true
				break
			}
		}
		if !drop {
			keep = append(keep, n)
		}
	}
	return keep, nil
}

// copyTree copies the contents of src into the already created dst.
func (c *treeCopier) copyTree(src, dst string) error {
	names, err := c.names(src)
	if err != nil {
		return oserr(err)
	}
	for _, name := range names {
		s := filepath.Join(src, name)
		d := filepath.Join(dst, name)
		fi, err := os.Lstat(s)
		if err != nil {
			c.add(s, d, err)
			continue
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0 && c.symlinks:
			target, err := os.Readlink(s)
			if err != nil {
				c.add(s, d, err)
				continue
			}
			if err := os.Symlink(target, d); err != nil && !errors.Is(err, fs.ErrExist) {
				c.add(s, d, err)
			}
		case fi.IsDir():
			if err := os.MkdirAll(d, fi.Mode().Perm()); err != nil {
				c.add(s, d, err)
				continue
			}
			if err := c.copyTree(s, d); err != nil {
				return err
			}
			c.copyStat(s, d)
		default:
			if err := c.copyOne(s, d); err != nil {
				c.add(s, d, err)
			}
		}
	}
	return nil
}

// copyOne copies a regular file with the requested copy function, falling
// back to copy2.
func (c *treeCopier) copyOne(src, dst string) error {
	if c.copyFn != nil && c.copyFn != py.None {
		_, err := py.Call(c.copyFn, py.Tuple{py.String(src), py.String(dst)}, py.StringDict{})
		return err
	}
	_, err := copy2(c.self, py.Tuple{py.String(src), py.String(dst)}, py.StringDict{})
	return err
}

// copyStat applies the source's mode and times to a copied directory.
func (c *treeCopier) copyStat(src, dst string) {
	fi, err := os.Stat(src)
	if err != nil {
		return
	}
	_ = os.Chmod(dst, fi.Mode().Perm())
	_ = copystatTimes(fi, dst)
}

// ---------------------------------------------------------------------------
// move

// move moves a file or a tree.
func move(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var srcObj, dstObj py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO:move",
		[]string{"src", "dst"}, &srcObj, &dstObj); err != nil {
		return nil, err
	}
	src, err := fspath(srcObj)
	if err != nil {
		return nil, err
	}
	dst, err := fspath(dstObj)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
		dst = filepath.Join(dst, filepath.Base(src))
	}
	if err := os.Rename(src, dst); err == nil {
		return py.String(dst), nil
	}
	// The rename failed.  CPython falls back to copy-and-delete only for a
	// cross-device rename and re-raises anything else.  Here every failure
	// falls back, so a rename refused for another reason (a permission, say)
	// is attempted as a copy instead; that copy then fails with its own
	// error, which is what reaches the caller.
	fi, err := os.Lstat(src)
	if err != nil {
		return nil, oserr(err)
	}
	if fi.IsDir() {
		if _, err := copytree(self, py.Tuple{py.String(src), py.String(dst)}, py.StringDict{}); err != nil {
			return nil, err
		}
		if err := rmtreePath(src, false, nil); err != nil {
			return nil, err
		}
		return py.String(dst), nil
	}
	if _, err := copy2(self, py.Tuple{py.String(src), py.String(dst)}, py.StringDict{}); err != nil {
		return nil, err
	}
	if err := os.Remove(src); err != nil {
		return nil, oserr(err)
	}
	return py.String(dst), nil
}

// ---------------------------------------------------------------------------
// rmtree

// rmtree deletes a directory tree.
//
// A symlink to a directory is refused rather than followed, as CPython does:
// following it would delete the tree it points at, which is never what the
// caller asked for.
func rmtree(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var pathObj py.Object
	ignoreErrors := py.Object(py.False)
	onerror := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:rmtree",
		[]string{"path", "ignore_errors", "onerror"}, &pathObj, &ignoreErrors, &onerror); err != nil {
		return nil, err
	}
	path, err := fspath(pathObj)
	if err != nil {
		return nil, err
	}
	ignore, err := py.ObjectIsTrue(ignoreErrors)
	if err != nil {
		return nil, err
	}
	fi, err := os.Lstat(path)
	if err != nil {
		if ignore {
			return py.None, nil
		}
		return nil, oserr(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, py.ExceptionNewf(py.OSError, "Cannot call rmtree on a symbolic link")
	}
	if err := rmtreePath(path, ignore, onerror); err != nil {
		return nil, err
	}
	return py.None, nil
}

// rmtreePath removes path recursively.  onerror, when it is a callable, is
// given (function, path, exc_info) for every failure, as CPython does; when
// it is nil and ignoreErrors is false the first failure is returned.
func rmtreePath(path string, ignoreErrors bool, onerror py.Object) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return handleRmError(err, path, "os.lstat", ignoreErrors, onerror)
	}
	if fi.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return handleRmError(err, path, "os.listdir", ignoreErrors, onerror)
		}
		for _, e := range entries {
			if err := rmtreePath(filepath.Join(path, e.Name()), ignoreErrors, onerror); err != nil {
				return err
			}
		}
		if err := os.Remove(path); err != nil {
			return handleRmError(err, path, "os.rmdir", ignoreErrors, onerror)
		}
		return nil
	}
	if err := os.Remove(path); err != nil {
		return handleRmError(err, path, "os.unlink", ignoreErrors, onerror)
	}
	return nil
}

// handleRmError routes one rmtree failure to onerror, swallows it under
// ignore_errors, or returns it.
func handleRmError(err error, path, function string, ignoreErrors bool, onerror py.Object) error {
	exc := oserr(err)
	if onerror != nil && onerror != py.None {
		// the exc_info triple, with no traceback available for a Go error.
		info := py.Tuple{exc.Type(), exc, py.None}
		if _, callErr := py.Call(onerror, py.Tuple{py.String(function), py.String(path), info}, py.StringDict{}); callErr != nil {
			return callErr
		}
		return nil
	}
	if ignoreErrors {
		return nil
	}
	return exc
}

// ---------------------------------------------------------------------------
// terminal_size

// terminalSize is os.terminal_size: a (columns, lines) pair.  This
// interpreter has no such type in os, so the object get_terminal_size
// returns is defined here, as pathlib defines os.stat_result.
type terminalSize struct {
	columns, lines int
	Dict           py.StringDict
}

var terminalSizeType = py.NewTypeX("os.terminal_size",
	"the result of os.get_terminal_size(), a (columns, lines) pair",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		items, err := py.SequenceList(args)
		if err != nil {
			return nil, err
		}
		if len(items.Items) != 2 {
			return nil, py.ExceptionNewf(py.TypeError, "terminal_size() takes exactly 2 arguments")
		}
		columns, err := py.IndexInt(items.Items[0])
		if err != nil {
			return nil, err
		}
		lines, err := py.IndexInt(items.Items[1])
		if err != nil {
			return nil, err
		}
		return &terminalSize{columns: columns, lines: lines, Dict: py.NewStringDict()}, nil
	}, nil)

func (t *terminalSize) Type() *py.Type { return terminalSizeType }

// get_terminal_size answers with the COLUMNS and LINES environment variables
// where they are set, and with fallback otherwise.
//
// CPython queries the terminal attached to sys.__stdout__ before falling
// back.  It cannot do that here: this interpreter's sys.stdout has no
// fileno(), so there is no descriptor to ask.  A program running under a
// terminal therefore gets the environment or the fallback, not the real
// window size.
func getTerminalSize(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fallback py.Object = py.Tuple{py.Int(80), py.Int(24)}
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:get_terminal_size",
		[]string{"fallback"}, &fallback); err != nil {
		return nil, err
	}
	fbCols, fbLines := 80, 24
	if fallback != py.None {
		items, err := py.SequenceList(fallback)
		if err != nil {
			return nil, err
		}
		if len(items.Items) == 2 {
			if n, err := py.IndexInt(items.Items[0]); err == nil {
				fbCols = n
			}
			if n, err := py.IndexInt(items.Items[1]); err == nil {
				fbLines = n
			}
		}
	}
	columns := envInt("COLUMNS")
	lines := envInt("LINES")
	if columns <= 0 {
		columns = fbCols
	}
	if lines <= 0 {
		lines = fbLines
	}
	return &terminalSize{columns: columns, lines: lines, Dict: py.NewStringDict()}, nil
}

// envInt reads a positive integer from the environment, or 0.
func envInt(name string) int {
	v := os.Getenv(name)
	if v == "" {
		return 0
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			// CPython's int() would reject "80px" and the except clause
			// falls back, so a non-numeric value means "not set here".
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ---------------------------------------------------------------------------
// usage

// usage is what disk_usage returns: total, used and free, in bytes.
type usage struct {
	total, used, free int64
	Dict              py.StringDict
}

var usageType = py.NewTypeX("shutil.usage",
	"Result of shutil.disk_usage() with fields total, used and free.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		items, err := py.SequenceList(args)
		if err != nil {
			return nil, err
		}
		if len(items.Items) != 3 {
			return nil, py.ExceptionNewf(py.TypeError, "usage() takes exactly 3 arguments")
		}
		total, err := py.IndexInt(items.Items[0])
		if err != nil {
			return nil, err
		}
		used, err := py.IndexInt(items.Items[1])
		if err != nil {
			return nil, err
		}
		free, err := py.IndexInt(items.Items[2])
		if err != nil {
			return nil, err
		}
		return &usage{total: int64(total), used: int64(used), free: int64(free), Dict: py.NewStringDict()}, nil
	}, nil)

func (u *usage) Type() *py.Type { return usageType }

// diskUsage is the exported entry point of disk_usage; the query itself is
// in a file per platform.
func diskUsage(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var pathObj py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:disk_usage",
		[]string{"path"}, &pathObj); err != nil {
		return nil, err
	}
	path, err := fspath(pathObj)
	if err != nil {
		return nil, err
	}
	total, used, free, err := diskUsageQuery(path)
	if err != nil {
		return nil, err
	}
	return &usage{total: total, used: used, free: free, Dict: py.NewStringDict()}, nil
}

// ---------------------------------------------------------------------------
// named-tuple reprs

// reprTerminalSize formats a terminal_size the way its named-tuple repr
// does, so that printing one looks like CPython's.
func reprTerminalSize(t *terminalSize) string {
	return "os.terminal_size(" + tupleBody(py.Tuple{py.Int(t.columns), py.Int(t.lines)}) + ")"
}

// reprUsage formats a usage the way its named-tuple repr does.
func reprUsage(u *usage) string {
	return "usage(" + tupleBody(py.Tuple{py.Int(u.total), py.Int(u.used), py.Int(u.free)}) + ")"
}

// tupleBody renders a tuple and strips the enclosing parentheses, giving the
// field list a named tuple's repr needs.
func tupleBody(t py.Tuple) string {
	s, err := py.ReprAsString(t)
	if err != nil || len(s) < 2 {
		return strings.Trim(strings.Repeat(", ", len(t)-1), ", ")
	}
	return s[1 : len(s)-1]
}

// ---------------------------------------------------------------------------
// registration

func init() {
	// The named results expose their fields by name and by index, as CPython
	// named tuples do.
	for i, name := range []string{"columns", "lines"} {
		idx := i
		terminalSizeType.Dict.Set(name, &py.Property{Fget: func(self py.Object) (py.Object, error) {
			t := self.(*terminalSize)
			if idx == 0 {
				return py.Int(t.columns), nil
			}
			return py.Int(t.lines), nil
		}})
	}
	terminalSizeType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*terminalSize)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		i, err := py.IndexInt(key)
		if err != nil {
			return nil, err
		}
		if i < 0 {
			i += 2
		}
		switch i {
		case 0:
			return py.Int(t.columns), nil
		case 1:
			return py.Int(t.lines), nil
		}
		return nil, py.ExceptionNewf(py.IndexError, "tuple index out of range")
	}, 0, "Return the field at index i."))
	terminalSizeType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(2), nil
	}, 0, "Number of fields."))
	terminalSizeType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*terminalSize)
		return py.NewIterator(py.Tuple{py.Int(t.columns), py.Int(t.lines)}), nil
	}, 0, "Iterate over the fields."))
	terminalSizeType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*terminalSize)
		return py.String(reprTerminalSize(t)), nil
	}, 0, "Return repr(self)."))
	terminalSizeType.Dict.Set("__str__", py.MustNewMethod("__str__", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*terminalSize)
		return py.String(reprTerminalSize(t)), nil
	}, 0, "Return str(self)."))

	for i, name := range []string{"total", "used", "free"} {
		idx := i
		usageType.Dict.Set(name, &py.Property{Fget: func(self py.Object) (py.Object, error) {
			u := self.(*usage)
			switch idx {
			case 0:
				return py.Int(u.total), nil
			case 1:
				return py.Int(u.used), nil
			}
			return py.Int(u.free), nil
		}})
	}
	usageType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		u := self.(*usage)
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		i, err := py.IndexInt(key)
		if err != nil {
			return nil, err
		}
		if i < 0 {
			i += 3
		}
		switch i {
		case 0:
			return py.Int(u.total), nil
		case 1:
			return py.Int(u.used), nil
		case 2:
			return py.Int(u.free), nil
		}
		return nil, py.ExceptionNewf(py.IndexError, "tuple index out of range")
	}, 0, "Return the field at index i."))
	usageType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(3), nil
	}, 0, "Number of fields."))
	usageType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		u := self.(*usage)
		return py.NewIterator(py.Tuple{py.Int(u.total), py.Int(u.used), py.Int(u.free)}), nil
	}, 0, "Iterate over the fields."))
	usageType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		u := self.(*usage)
		return py.String(reprUsage(u)), nil
	}, 0, "Return repr(self)."))
	usageType.Dict.Set("__str__", py.MustNewMethod("__str__", func(self py.Object, args py.Tuple) (py.Object, error) {
		u := self.(*usage)
		return py.String(reprUsage(u)), nil
	}, 0, "Return str(self)."))

	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "Error", Value: Error},
		py.DictEntry{Key: "SameFileError", Value: SameFileError},
		py.DictEntry{Key: "which", Value: py.MustNewMethod("which", which, 0, "Given a command, mode, and a PATH string, return the path which conforms to the given mode on the PATH, or None if there is no such file.")},
		py.DictEntry{Key: "get_terminal_size", Value: py.MustNewMethod("get_terminal_size", getTerminalSize, 0, "Get the size of the terminal window.")},
		py.DictEntry{Key: "copyfileobj", Value: py.MustNewMethod("copyfileobj", copyfileobj, 0, "copy data from file-like object fsrc to file-like object fdst")},
		py.DictEntry{Key: "copyfile", Value: py.MustNewMethod("copyfile", copyfile, 0, "Copy data from src to dst in the most efficient way possible.")},
		py.DictEntry{Key: "copymode", Value: py.MustNewMethod("copymode", copymode, 0, "Copy the permission bits of src to dst.")},
		py.DictEntry{Key: "copystat", Value: py.MustNewMethod("copystat", copystat, 0, "Copy the permission bits, last access time, last modification time.")},
		py.DictEntry{Key: "copy", Value: py.MustNewMethod("copy", copy, 0, "Copy data and mode bits ('cp src dst').  The destination may be a directory.")},
		py.DictEntry{Key: "copy2", Value: py.MustNewMethod("copy2", copy2, 0, "Copy data and metadata.  The destination may be a directory.")},
		py.DictEntry{Key: "copytree", Value: py.MustNewMethod("copytree", copytree, 0, "Recursively copy an entire directory tree rooted at src.")},
		py.DictEntry{Key: "move", Value: py.MustNewMethod("move", move, 0, "Recursively move a file or directory (src) to another location (dst).")},
		py.DictEntry{Key: "rmtree", Value: py.MustNewMethod("rmtree", rmtree, 0, "Recursively delete a directory tree.")},
		py.DictEntry{Key: "disk_usage", Value: py.MustNewMethod("disk_usage", diskUsage, 0, "Return disk usage statistics about the given path as a named tuple with the attributes total, used and free.")},
		py.DictEntry{Key: "COPY_BUFSIZE", Value: py.Int(CopyBufSize)},
		py.DictEntry{Key: "F_OK", Value: py.Int(F_OK)},
		py.DictEntry{Key: "X_OK", Value: py.Int(X_OK)},
		py.DictEntry{Key: "W_OK", Value: py.Int(W_OK)},
		py.DictEntry{Key: "R_OK", Value: py.Int(R_OK)},
		py.DictEntry{Key: "usage", Value: usageType},
	)
	globals.Set("__all__", py.NewListFromItems([]py.Object{
		py.String("Error"), py.String("SameFileError"),
		py.String("copyfileobj"), py.String("copyfile"), py.String("copymode"),
		py.String("copystat"), py.String("copy"), py.String("copy2"),
		py.String("copytree"), py.String("move"), py.String("rmtree"),
		py.String("disk_usage"), py.String("which"), py.String("get_terminal_size"),
	}))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "shutil",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
