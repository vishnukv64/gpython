// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package os implements the Python os module.
package os

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

var (
	osSep     = py.String("/")
	osName    = py.String("posix")
	osPathsep = py.String(":")
	osLinesep = py.String("\n")
	osDefpath = py.String(":/bin:/usr/bin")
	osDevnull = py.String("/dev/null")

	osAltsep py.Object = py.None
)

func initGlobals() {
	switch runtime.GOOS {
	case "android":
		osName = py.String("java")
	case "windows":
		osSep = py.String(`\`)
		osName = py.String("nt")
		osPathsep = py.String(";")
		osLinesep = py.String("\r\n")
		osDefpath = py.String(`C:\bin`)
		osDevnull = py.String("nul")
		osAltsep = py.String("/")
	}
}

func init() {
	initGlobals()

	methods := []*py.Method{
		py.MustNewMethod("_exit", _exit, 0, "Immediate program termination."),
		py.MustNewMethod("close", closefd, 0, closefd_doc),
		py.MustNewMethod("fdopen", fdopen, 0, fdopen_doc),
		py.MustNewMethod("getcwd", getCwd, 0, "Get the current working directory"),
		py.MustNewMethod("getcwdb", getCwdb, 0, "Get the current working directory in a byte slice"),
		py.MustNewMethod("chdir", chdir, 0, "Change the current working directory"),
		py.MustNewMethod("getenv", getenv, 0, "Return the value of the environment variable key if it exists, or default if it doesn’t. key, default and the result are str."),
		py.MustNewMethod("getpid", getpid, 0, "Return the current process id."),
		py.MustNewMethod("listdir", listDir, 0, listDir_doc),
		py.MustNewMethod("walk", walk, 0, walk_doc),
		py.MustNewMethod("makedirs", makedirs, 0, makedirs_doc),
		py.MustNewMethod("mkdir", mkdir, 0, mkdir_doc),
		py.MustNewMethod("putenv", putenv, 0, "Set the environment variable named key to the string value."),
		py.MustNewMethod("remove", remove, 0, remove_doc),
		py.MustNewMethod("rename", rename, 0, rename_doc),
		py.MustNewMethod("replace", replace, 0, replace_doc),
		// unlink is the same operation under CPython's other name for it, and
		// code uses both.  Registered as its own method rather than an alias so
		// its docstring says which is which.
		py.MustNewMethod("unlink", remove, 0, "Remove a file (same as remove())."),
		py.MustNewMethod("removedirs", removedirs, 0, removedirs_doc),
		py.MustNewMethod("rmdir", rmdir, 0, rmdir_doc),
		py.MustNewMethod("system", system, 0, "Run shell commands, prints stdout directly to default"),
		py.MustNewMethod("unsetenv", unsetenv, 0, "Unset (delete) the environment variable named key."),
	}
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "error", Value: py.OSError},
		py.DictEntry{Key: "environ", Value: Environ},
		// _Environ is the TYPE of os.environ, and code keys a dispatch table by
		// it - rich does, with "os._Environ: lambda _object: ..." - so the name
		// has to be reachable from the module rather than only from Go.
		py.DictEntry{Key: "_Environ", Value: EnvironType},
		py.DictEntry{Key: "sep", Value: osSep},
		py.DictEntry{Key: "name", Value: osName},
		py.DictEntry{Key: "curdir", Value: py.String(".")},
		py.DictEntry{Key: "pardir", Value: py.String("..")},
		py.DictEntry{Key: "extsep", Value: py.String(".")},
		py.DictEntry{Key: "altsep", Value: osAltsep},
		py.DictEntry{Key: "pathsep", Value: osPathsep},
		py.DictEntry{Key: "linesep", Value: osLinesep},
		py.DictEntry{Key: "defpath", Value: osDefpath},
		py.DictEntry{Key: "devnull", Value: osDevnull},
	)
	// os.path is the posixpath module object, so os.path.join and friends
	// reach the functions registered there, and "os.path is posixpath" holds.
	globals.Set("path", PathModule())
	// The separator constants, which code joins paths with.
	globals.Set("pathsep", osPathsep)
	globals.Set("linesep", osLinesep)
	globals.Set("altsep", osAltsep)
	// os.PathLike: the protocol a path-like object implements.  It needs to
	// be a class, since "os.PathLike[str]" appears in annotations and real
	// code tests for it, and it must accept a subscription.
	pathLikeType := py.NewType("os.PathLike", "Abstract base class for objects representing a file system path.")
	pathLikeType.Dict.Set("__class_getitem__", py.MustNewMethod("__class_getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Return the class, ignoring the subscription parameters."))
	globals.Set("PathLike", pathLikeType)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "os",
			Doc:  "Miscellaneous operating system interfaces",
		},
		Methods: methods,
		Globals: globals,
	})
}

// getEnvVariables returns the dictionary of environment variables.
func getEnvVariables() py.StringDict {
	vs := os.Environ()
	dict := py.NewStringDictSized(len(vs))
	for _, evar := range vs {
		key_value := strings.SplitN(evar, "=", 2) // returns a []string containing [key,value]
		dict.M__setitem__(py.String(key_value[0]), py.String(key_value[1]))
	}

	return dict
}

const closefd_doc = `Close a file descriptor`

func closefd(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pyfd py.Object
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "i", []string{"fd"}, &pyfd)
	if err != nil {
		return nil, err
	}

	var (
		fd   = uintptr(pyfd.(py.Int))
		name = strconv.Itoa(int(fd))
	)

	f := os.NewFile(fd, name)
	if f == nil {
		return nil, py.ExceptionNewf(py.OSError, "Bad file descriptor")
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return py.None, nil
}

const fdopen_doc = `# Supply os.fdopen()`

func fdopen(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pyfd        py.Object
		pymode      py.Object = py.String("r")
		pybuffering py.Object = py.Int(-1)
		pyencoding  py.Object = py.None
	)
	err := py.ParseTupleAndKeywords(
		args, kwargs,
		"i|s#is#", []string{"fd", "mode", "buffering", "encoding"},
		&pyfd, &pymode, &pybuffering, &pyencoding,
	)
	if err != nil {
		return nil, err
	}

	// FIXME(sbinet): handle buffering
	// FIXME(sbinet): handle encoding

	var (
		fd   = uintptr(pyfd.(py.Int))
		name = strconv.Itoa(int(fd))
		mode string
	)

	switch v := pymode.(type) {
	case py.String:
		mode = string(v)
	case py.Bytes:
		mode = string(v)
	}

	perm, _, _, err := py.FileModeFrom(mode)
	if err != nil {
		return nil, err
	}

	f := os.NewFile(fd, name)
	if f == nil {
		return nil, py.ExceptionNewf(py.OSError, "Bad file descriptor")
	}

	return &py.File{f, perm}, nil
}

// getCwd returns the current working directory.
func getCwd(self py.Object, args py.Tuple) (py.Object, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "Unable to get current working directory.")
	}
	return py.String(dir), nil
}

// getCwdb returns the current working directory as a byte list.
func getCwdb(self py.Object, args py.Tuple) (py.Object, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "Unable to get current working directory.")
	}
	return py.Bytes(dir), nil
}

// chdir changes the current working directory to the provided path.
func chdir(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "Missing required argument 'path' (pos 1)")
	}
	dir, ok := args[0].(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "str expected, not "+args[0].Type().Name)
	}
	err := os.Chdir(string(dir))
	if err != nil {
		return nil, py.ExceptionNewf(py.NotADirectoryError, "Couldn't change cwd; "+err.Error())
	}
	return py.None, nil
}

// getenv returns the value of the environment variable key.
// If no such environment variable exists and a default value was provided, that value is returned.
func getenv(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "missing one required argument: 'name:str'")
	}
	k, ok := args[0].(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "str expected (pos 1), not "+args[0].Type().Name)
	}
	v, ok := os.LookupEnv(string(k))
	if ok {
		return py.String(v), nil
	}
	if len(args) == 2 {
		return args[1], nil
	}
	return py.None, nil
}

// getpid returns the current process id.
func getpid(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Int(os.Getpid()), nil
}

const listDir_doc = `
Return a list containing the names of the files in the directory.

path can be specified as either str, bytes.  If path is bytes, the filenames
  returned will also be bytes; in all other circumstances
  the filenames returned will be str.
If path is None, uses the path='.'.

The list is in arbitrary order.  It does not include the special
entries '.' and '..' even if they are present in the directory.
`

func listDir(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		path py.Object = py.None
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "|z*:listdir", []string{"path"}, &path)
	if err != nil {
		return nil, err
	}

	if path == py.None {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, py.ExceptionNewf(py.OSError, "cannot get cwd, error %s", err.Error())
		}
		path = py.String(cwd)
	}

	dirName := ""
	returnsBytes := false
	switch v := path.(type) {
	case py.String:
		dirName = string(v)
	case py.Bytes:
		dirName = string(v)
		returnsBytes = true
	default:
		return nil, py.ExceptionNewf(py.TypeError, "str or bytes expected, not %T", path)
	}

	dirEntries, err := os.ReadDir(dirName)
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "cannot read directory %s, error %s", dirName, err.Error())
	}
	result := py.NewListSized(len(dirEntries))
	for i, dirEntry := range dirEntries {
		if returnsBytes {
			result.Items[i] = py.Bytes(dirEntry.Name())
		} else {
			result.Items[i] = py.String(dirEntry.Name())
		}
	}
	return result, nil
}

const walk_doc = `Directory tree generator.

For each directory in the directory tree rooted at top (including top
itself, but excluding '.' and '..'), yields a 3-tuple

    dirpath, dirnames, filenames

dirpath is a string, the path to the directory.  dirnames is a list of
the names of the subdirectories in dirpath (including symlinks to
directories, and excluding '.' and '..').  filenames is a list of the
names of the non-directory files in dirpath.
Note that the names in the lists are just names, with no path
components.  To get a full path (which begins with top) to a file or
directory in dirpath, do os.path.join(dirpath, name).

If optional arg 'topdown' is true or not specified, the triple for a
directory is generated before the triples for any of its subdirectories
(directories are generated top down).  If topdown is false, the triple
for a directory is generated after the triples for all of its
subdirectories (directories are generated bottom up).

When topdown is true, the caller can modify the dirnames list in-place
(e.g., via del or slice assignment), and walk will only recurse into the
subdirectories whose names remain in dirnames; this can be used to prune
the search, or to impose a specific order of visiting.  Modifying
dirnames when topdown is false has no effect on the behavior of
os.walk(), since the directories in dirnames have already been generated
by the time dirnames itself is generated.  No matter the value of
topdown, the list of subdirectories is retrieved before the tuples for
the directory and its subdirectories are generated.

By default errors from the os.scandir() call are ignored.  If
optional arg 'onerror' is specified, it should be a function; it
will be called with one argument, an OSError instance.  It can
report the error to continue with the walk, or raise the exception
to abort the walk.  Note that the filename is available as the
filename attribute of the exception object.

By default, os.walk does not follow symbolic links to subdirectories on
systems that support them.  In order to get this functionality, set the
optional argument 'followlinks' to true.

Caution:  if you pass a relative pathname for top, don't change the
current working directory between resumptions of walk.  walk never
changes the current directory, and assumes that the client doesn't
either.`

// walkIterator is os.walk.  CPython implements walk as a generator over an
// explicit stack, and the same stack lives here so that pruning behaves: the
// children of a directory are pushed only after its triple has been yielded
// and the caller has had the chance to edit dirnames in place.
type walkIterator struct {
	// stack holds work items.  A py.Tuple is a finished bottom-up triple
	// waiting to be yielded; anything else is a path still to visit.
	stack   []py.Object
	topdown bool
	follow  bool
	onerror py.Object
	// pending and yielded are the directory whose children are pushed next
	// and the list the caller may have pruned while handling the yield.
	pending    py.Object
	yielded    *py.List
	hasPending bool
}

var walkIteratorType = py.NewType("os.walk", "Directory tree generator.")

func (w *walkIterator) Type() *py.Type { return walkIteratorType }

func (w *walkIterator) M__iter__() (py.Object, error) { return w, nil }

var _ py.I__iter__ = (*walkIterator)(nil)
var _ py.I__next__ = (*walkIterator)(nil)

func (w *walkIterator) M__next__() (py.Object, error) {
	for len(w.stack) > 0 || w.hasPending {
		if w.hasPending {
			dir, names := w.pending, w.yielded
			w.pending, w.yielded, w.hasPending = nil, nil, false
			w.pushChildren(dir, names)
			continue
		}
		top := w.stack[len(w.stack)-1]
		// A bottom-up triple is already a value to yield.
		if tup, ok := top.(py.Tuple); ok {
			w.stack = w.stack[:len(w.stack)-1]
			return tup, nil
		}

		dirPath, isBytes := pathString(top)
		entries, err := readDirNames(dirPath)
		if err != nil {
			if w.onerror != nil && w.onerror != py.None {
				if _, callErr := py.Call(w.onerror, py.Tuple{osErrorFor(err, dirPath)}, py.StringDict{}); callErr != nil {
					return nil, callErr
				}
			}
			w.stack = w.stack[:len(w.stack)-1]
			continue
		}
		w.stack = w.stack[:len(w.stack)-1]

		var dirs, walkDirs, nondirs []string
		for _, name := range entries {
			full := joinOne(dirPath, name)
			// A symlink to a directory is a directory here, which is
			// entry.is_dir() following the link.
			if entryIsDir(full) {
				dirs = append(dirs, name)
				if !w.topdown && (w.follow || !isSymlink(full)) {
					walkDirs = append(walkDirs, full)
				}
			} else {
				nondirs = append(nondirs, name)
			}
		}

		mk := func(s string) py.Object {
			if isBytes {
				return py.Bytes(s)
			}
			return py.String(s)
		}
		dirList := py.NewListSized(len(dirs))
		for i, d := range dirs {
			dirList.Items[i] = mk(d)
		}
		fileList := py.NewListSized(len(nondirs))
		for i, d := range nondirs {
			fileList.Items[i] = mk(d)
		}
		triple := py.Tuple{mk(dirPath), dirList, fileList}

		if w.topdown {
			w.pending, w.yielded, w.hasPending = top, dirList, true
			return triple, nil
		}

		w.stack = append(w.stack, triple)
		for i := len(walkDirs) - 1; i >= 0; i-- {
			w.stack = append(w.stack, mk(walkDirs[i]))
		}
	}
	return nil, py.StopIteration
}

// pushChildren queues the surviving children of a just-yielded directory.  A
// name the caller removed from the list during the yield is skipped, which is
// what makes "del dirs[i]" prune the walk.
func (w *walkIterator) pushChildren(dir py.Object, dirnames *py.List) {
	dirPath, isBytes := pathString(dir)
	for i := len(dirnames.Items) - 1; i >= 0; i-- {
		name, err := py.StrAsString(dirnames.Items[i])
		if err != nil {
			continue
		}
		newPath := joinOne(dirPath, name)
		if !w.follow && isSymlink(newPath) {
			continue
		}
		if isBytes {
			w.stack = append(w.stack, py.Bytes(newPath))
		} else {
			w.stack = append(w.stack, py.String(newPath))
		}
	}
}

// pathString extracts the string of a path argument.
func pathString(v py.Object) (string, bool) {
	switch p := v.(type) {
	case py.String:
		return string(p), false
	case py.Bytes:
		return string(p), true
	}
	return "", false
}

// joinOne is os.path.join for a single name on the running platform's
// separator.
func joinOne(dir, name string) string {
	if dir == "" {
		return name
	}
	if strings.HasSuffix(dir, string(osSep)) {
		return dir + name
	}
	return dir + string(osSep) + name
}

// readDirNames lists a directory in the order the filesystem returns it.
// os.ReadDir sorts, and sorting the names would not match CPython's scandir
// order, so Readdirnames is used directly.
func readDirNames(dir string) ([]string, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}

// entryIsDir follows symlinks, so a link to a directory lands in dirnames the
// way entry.is_dir() does.
func entryIsDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func isSymlink(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// osErrorFor wraps a filesystem error as the OSError onerror receives,
// carrying the offending path in filename.
func osErrorFor(err error, path string) py.Object {
	e := py.ExceptionNewf(py.OSError, "%s: '%s'", err.Error(), path)
	e.Dict.Set("filename", py.String(path))
	return e
}

func walk(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		top       py.Object
		topdown   py.Object = py.True
		onerror   py.Object = py.None
		followObj py.Object = py.False
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "O|OOO:walk",
		[]string{"top", "topdown", "onerror", "followlinks"},
		&top, &topdown, &onerror, &followObj)
	if err != nil {
		return nil, err
	}
	if _, ok := top.(py.String); !ok {
		if _, ok := top.(py.Bytes); !ok {
			return nil, py.ExceptionNewf(py.TypeError, "expected str, bytes or os.PathLike object, not %s", top.Type().Name)
		}
	}
	td, err := py.ObjectIsTrue(topdown)
	if err != nil {
		return nil, err
	}
	fl, err := py.ObjectIsTrue(followObj)
	if err != nil {
		return nil, err
	}
	return &walkIterator{
		stack:   []py.Object{top},
		topdown: td,
		follow:  fl,
		onerror: onerror,
	}, nil
}

const makedirs_doc = `makedirs(name [, mode=0o777][, exist_ok=False])
Super-mkdir; create a leaf directory and all intermediate ones.  Works like
mkdir, except that any intermediate path segment (not just the rightmost)
will be created if it does not exist. If the target directory already
exists, raise an OSError if exist_ok is False. Otherwise no exception is
raised.  This is recursive.`

func makedirs(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pypath py.Object
		pymode py.Object = py.Int(0o777)
		pyok   py.Object = py.False
	)
	err := py.ParseTupleAndKeywords(
		args, kwargs,
		"s#|ip:makedirs", []string{"path", "mode", "exist_ok"},
		&pypath, &pymode, &pyok,
	)
	if err != nil {
		return nil, err
	}

	var (
		path = ""
		mode = os.FileMode(pymode.(py.Int))
	)
	switch v := pypath.(type) {
	case py.String:
		path = string(v)
	case py.Bytes:
		path = string(v)
	}

	if pyok.(py.Bool) == py.False {
		// check if leaf exists.
		_, err := os.Stat(path)
		// FIXME(sbinet): handle other errors.
		if err == nil {
			return nil, py.ExceptionNewf(py.FileExistsError, "File exists: '%s'", path)
		}
	}

	err = os.MkdirAll(path, mode)
	if err != nil {
		return nil, err
	}

	return py.None, nil
}

const mkdir_doc = `Create a directory.

If dir_fd is not None, it should be a file descriptor open to a directory,
  and path should be relative; path will then be relative to that directory.
dir_fd may not be implemented on your platform.
  If it is unavailable, using it will raise a NotImplementedError.

The mode argument is ignored on Windows.`

func mkdir(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pypath  py.Object
		pymode  py.Object = py.Int(511)
		pydirfd py.Object = py.None
	)
	err := py.ParseTupleAndKeywords(
		args, kwargs,
		"s#|ii:mkdir", []string{"path", "mode", "dir_fd"},
		&pypath, &pymode, &pydirfd,
	)
	if err != nil {
		return nil, err
	}

	var (
		path = ""
		mode = os.FileMode(pymode.(py.Int))
	)
	switch v := pypath.(type) {
	case py.String:
		path = string(v)
	case py.Bytes:
		path = string(v)
	}

	if pydirfd != py.None {
		// FIXME(sbinet)
		return nil, py.ExceptionNewf(py.NotImplementedError, "mkdir(dir_fd=XXX) not implemented")
	}

	err = os.Mkdir(path, mode)
	if err != nil {
		return nil, err
	}

	return py.None, nil
}

// putenv sets the value of an environment variable named by the key.
func putenv(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "missing required arguments: 'key:str' and 'value:str'")
	}
	k, ok := args[0].(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "str expected (pos 1), not "+args[0].Type().Name)
	}
	v, ok := args[1].(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "str expected (pos 2), not "+args[1].Type().Name)
	}
	err := os.Setenv(string(k), string(v))
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "Unable to set enviroment variable")
	}
	return py.None, nil
}

// Unset (delete) the environment variable named key.
func unsetenv(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "missing one required argument: 'key:str'")
	}
	k, ok := args[0].(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "str expected (pos 1), not "+args[0].Type().Name)
	}
	err := os.Unsetenv(string(k))
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "Unable to unset enviroment variable")
	}
	return py.None, nil
}

// os._exit() immediate program termination; unlike sys.exit(), which raises a SystemExit, this function will termninate the program immediately.
func _exit(self py.Object, args py.Tuple) (py.Object, error) { // can never return
	if len(args) == 0 {
		os.Exit(0)
	}
	arg, ok := args[0].(py.Int)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "expected int (pos 1), not "+args[0].Type().Name)
	}
	os.Exit(int(arg))
	return nil, nil
}

const remove_doc = `Remove a file (same as unlink()).

If dir_fd is not None, it should be a file descriptor open to a directory,
  and path should be relative; path will then be relative to that directory.
dir_fd may not be implemented on your platform.
  If it is unavailable, using it will raise a NotImplementedError.`

const rename_doc = `rename(src, dst, *, src_dir_fd=None, dst_dir_fd=None)

Rename a file or directory.  On POSIX, if dst names an existing file it is
replaced silently; os.replace does the same everywhere, which is why it exists.`

func rename(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var src, dst py.Object
	if err := py.UnpackTuple(args, kwargs, "rename", 2, 2, &src, &dst); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(src)
	if err != nil {
		return nil, err
	}
	d, err := py.StrAsString(dst)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(s, d); err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
	}
	return py.None, nil
}

const replace_doc = `replace(src, dst, *, src_dir_fd=None, dst_dir_fd=None)

Rename a file or directory, SILENTLY replacing dst if it exists.  It differs
from rename only on Windows, where rename refuses an existing destination;
this is the one to use when the replacement is intended.`

// replace is os.rename here: Go's os.Rename replaces an existing destination on
// every platform this interpreter runs on, which is exactly what os.replace
// promises.  Registered separately because names are the API.
func replace(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return rename(self, args, kwargs)
}

func remove(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pypath py.Object
		pydir  py.Object = py.None
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "s#|i:remove", []string{"path", "dir_fd"}, &pypath, &pydir)
	if err != nil {
		return nil, err
	}

	if pydir != py.None {
		// FIXME(sbinet) ?
		return nil, py.ExceptionNewf(py.NotImplementedError, "remove(dir_fd=XXX) not implemented")
	}

	var name string
	switch v := pypath.(type) {
	case py.String:
		name = string(v)
	case py.Bytes:
		name = string(v)
	}

	err = os.Remove(name)
	if err != nil {
		return nil, err
	}

	return py.None, nil
}

const removedirs_doc = `removedirs(name)

Super-rmdir; remove a leaf directory and all empty intermediate
ones.  Works like rmdir except that, if the leaf directory is
successfully removed, directories corresponding to rightmost path
segments will be pruned away until either the whole path is
consumed or an error occurs.  Errors during this latter phase are
ignored -- they generally mean that a directory was not empty.`

func removedirs(self py.Object, args py.Tuple) (py.Object, error) {
	var pypath py.Object
	err := py.ParseTuple(args, "s#:rmdir", &pypath)
	if err != nil {
		return nil, err
	}

	var name string
	switch v := pypath.(type) {
	case py.String:
		name = string(v)
	case py.Bytes:
		name = string(v)
	}

	err = os.RemoveAll(name)
	if err != nil {
		return nil, err
	}

	return py.None, nil
}

const rmdir_doc = `Remove a directory.

If dir_fd is not None, it should be a file descriptor open to a directory,
  and path should be relative; path will then be relative to that directory.
dir_fd may not be implemented on your platform.
  If it is unavailable, using it will raise a NotImplementedError.`

func rmdir(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pypath py.Object
		pydir  py.Object = py.None
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "s#|i:rmdir", []string{"path", "dir_fd"}, &pypath, &pydir)
	if err != nil {
		return nil, err
	}

	if pydir != py.None {
		// FIXME(sbinet) ?
		return nil, py.ExceptionNewf(py.NotImplementedError, "rmdir(dir_fd=XXX) not implemented")
	}

	var name string
	switch v := pypath.(type) {
	case py.String:
		name = string(v)
	case py.Bytes:
		name = string(v)
	}

	err = os.Remove(name)
	if err != nil {
		return nil, err
	}

	return py.None, nil
}

// os.system(command string) this function runs a shell command and directs the output to standard output.
func system(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "missing one required argument: 'command:str'")
	}
	arg, ok := args[0].(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "str expected (pos 1), not "+args[0].Type().Name)
	}

	var command *exec.Cmd
	if runtime.GOOS != "windows" {
		command = exec.Command("/bin/sh", "-c", string(arg))
	} else {
		command = exec.Command("cmd.exe", string(arg))
	}
	outb, err := command.CombinedOutput() // - commbinedoutput to get both stderr and stdout -
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, err.Error())
	}
	ok = py.Println(self, string(outb))
	if !ok {
		return py.Int(1), nil
	}

	return py.Int(0), nil
}
