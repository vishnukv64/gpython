// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// tempfile.NamedTemporaryFile and the wrappers it returns.

package tempfile

import (
	"os"

	"github.com/vishnukv64/gpython/py"
)

const namedTemporaryFile_doc = `Create and return a temporary file.

Arguments:
'prefix', 'suffix', 'dir' -- as for mkstemp.
'mode' -- the mode argument to io.open (default "w+b").
'buffering' -- the buffer size argument to io.open (default -1).
'encoding' -- the encoding argument to io.open (default None)
'newline' -- the newline argument to io.open (default None)
'delete' -- whether the file is deleted on close (default True).
'errors' -- the errors argument to io.open (default None)
'delete_on_close' -- if false, the file is not deleted on close; it is
                     deleted when the returned object is garbage collected.
Returns the file object.`

// temporaryFile is the Go value behind the object NamedTemporaryFile returns.
//
// It WRAPS an open file rather than being one, as CPython's
// _TemporaryFileWrapper does, so that closing it can remove the file: the
// attribute access is forwarded, and close() is intercepted.
type temporaryFile struct {
	file py.Object
	name string
	// delete says whether close() removes the file.
	delete bool
	closed bool
}

var temporaryFileType = py.NewType("tempfile._TemporaryFileWrapper",
	"A temporary file that is removed when it is closed.")

func (t *temporaryFile) Type() *py.Type { return temporaryFileType }

// forward passes an attribute read on to the wrapped file, so the wrapper
// behaves like the file it holds for everything except close().
func (t *temporaryFile) M__getattr__(name string) (py.Object, error) {
	if t.file == nil {
		return nil, py.ExceptionNewf(py.AttributeError, "the temporary file is closed")
	}
	// The wrapper's OWN attributes are found before this is reached - the
	// interpreter tries the type's dict first - so anything arriving here
	// belongs to the file.
	return py.GetAttrString(t.file, name)
}

// namedTemporaryFile implements tempfile.NamedTemporaryFile.
//
// pip imports it in pip/_internal/utils/filesystem.py, and the parts that
// matter are honoured exactly: the file EXISTS while the object is alive, it
// reads and writes as an ordinary file, and close() removes it unless
// delete=False - which is the difference a caller relies on to keep the result.
func namedTemporaryFile(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		suffix   py.Object = py.None
		prefix   py.Object = py.None
		dirObj   py.Object = py.None
		mode     py.Object = py.String("w+b")
		encoding py.Object = py.None
		delObj   py.Object = py.True
	)
	// The parameters are read by name, so an unexpected one is reported rather
	// than silently ignored - a config that passes "buffering" and gets no
	// buffering is a bug the caller cannot see.
	known := []string{"suffix", "prefix", "dir", "mode", "buffering", "encoding",
		"newline", "delete", "errors", "delete_on_close"}
	for _, ent := range kwargs.Items() {
		found := false
		for _, k := range known {
			if k == ent.Key {
				found = true
				break
			}
		}
		if !found {
			return nil, py.ExceptionNewf(py.TypeError,
				"NamedTemporaryFile() got an unexpected keyword argument %q", ent.Key)
		}
	}
	if v, ok := kwargs.Get("suffix"); ok {
		suffix = v
	}
	if v, ok := kwargs.Get("prefix"); ok {
		prefix = v
	}
	if v, ok := kwargs.Get("dir"); ok {
		dirObj = v
	}
	if v, ok := kwargs.Get("mode"); ok {
		mode = v
	}
	if v, ok := kwargs.Get("encoding"); ok {
		encoding = v
	}
	if v, ok := kwargs.Get("delete"); ok {
		delObj = v
	}

	// The name is produced the same way mkstemp produces one, so the file is
	// created atomically with no window in which another process could take it.
	patPrefix := "tmp"
	if prefix != py.None {
		if s, err := py.StrAsString(prefix); err == nil {
			patPrefix = s
		}
	}
	patSuffix := ""
	if suffix != py.None {
		if s, err := py.StrAsString(suffix); err == nil {
			patSuffix = s
		}
	}
	dir := ""
	if dirObj != py.None {
		if s, err := py.StrAsString(dirObj); err == nil {
			dir = s
		}
	}
	f, err := os.CreateTemp(dir, patPrefix+"*"+patSuffix)
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
	}

	// Opened through the interpreter's own open, so the result carries every
	// file method a program expects rather than the subset this package
	// re-implements.
	modeStr := "w+b"
	if s, err := py.StrAsString(mode); err == nil {
		modeStr = s
	}
	fileArgs := py.NewStringDict()
	fileArgs.Set("mode", py.String(modeStr))
	if encoding != py.None {
		fileArgs.Set("encoding", encoding)
	}
	builtins := py.GetModuleImplOrNil("builtins")
	if builtins == nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "the builtins module is not available")
	}
	opener := builtins.Globals.GetOrNil("open")
	file, err := py.Call(opener, py.Tuple{py.String(name)}, fileArgs)
	if err != nil {
		return nil, err
	}

	wrapper := &temporaryFile{
		file:   file,
		name:   name,
		delete: delObj != py.False,
	}
	return wrapper, nil
}

func init() {
	// The wrapper forwards everything it does not implement to the file it
	// holds, so a program can read, write, seek and iterate it as usual.  Only
	// the attribute that must know the NAME, and close(), are its own.
	temporaryFileType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if t, ok := self.(*temporaryFile); ok {
				// The FILE's name, not the wrapper's: they are the same, and a
				// wrapper that reported its own would be right by accident
				// rather than by construction.
				return py.String(t.name), nil
			}
			return py.None, nil
		},
		Doc: "The name of the temporary file.",
	})
	temporaryFileType.Dict.Set("file", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if t, ok := self.(*temporaryFile); ok {
				return t.file, nil
			}
			return py.None, nil
		},
		Doc: "The underlying file object.",
	})
	temporaryFileType.Dict.Set("delete", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if t, ok := self.(*temporaryFile); ok {
				return py.NewBool(t.delete), nil
			}
			return py.False, nil
		},
		Doc: "Whether close() removes the file.",
	})
	temporaryFileType.Dict.Set("closed", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if t, ok := self.(*temporaryFile); ok {
				return py.NewBool(t.closed), nil
			}
			return py.True, nil
		},
		Doc: "Whether this file has been closed.",
	})

	// close() removes the file when delete is set.  This is the whole point of
	// the wrapper, and the reason it is not simply an open file.
	temporaryFileType.Dict.Set("close", py.MustNewMethod("close", func(self py.Object, args py.Tuple) (py.Object, error) {
		t, ok := self.(*temporaryFile)
		if !ok || t.closed {
			return py.None, nil
		}
		if t.file != nil {
			closeFn, err := py.GetAttrString(t.file, "close")
			if err == nil {
				if _, cerr := py.Call(closeFn, py.Tuple{}, py.NewStringDict()); cerr != nil {
					return nil, cerr
				}
			}
		}
		t.closed = true
		if t.delete {
			if err := os.Remove(t.name); err != nil && !os.IsNotExist(err) {
				return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
			}
		}
		return py.None, nil
	}, 0, "Close the file, removing it when delete is set."))

	// The context-manager protocol, which a temporary file is used with.
	temporaryFileType.Dict.Set("__enter__", py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Return self, so the file can be used with 'with'."))
	temporaryFileType.Dict.Set("__exit__", py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		if t, ok := self.(*temporaryFile); ok && !t.closed {
			closeFn, err := py.GetAttrString(t.file, "close")
			if err == nil {
				if _, cerr := py.Call(closeFn, py.Tuple{}, py.NewStringDict()); cerr != nil {
					return nil, cerr
				}
			}
			t.closed = true
			if t.delete {
				if err := os.Remove(t.name); err != nil && !os.IsNotExist(err) {
					return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
				}
			}
		}
		return py.False, nil
	}, 0, "Close the file, as leave from a 'with' block."))
	temporaryFileType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		if t, ok := self.(*temporaryFile); ok {
			return py.String("<tempfile._TemporaryFileWrapper file=" + t.name + ">"), nil
		}
		return py.String("<tempfile._TemporaryFileWrapper>"), nil
	}, 0, "Return repr(self)."))
}
