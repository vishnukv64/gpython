// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

// OSErrorNew is OSError.__new__: OSError(errno, strerror[, filename[, winerror
// [, filename2]]]) sets the matching attributes, and OSError(2, 'x') itself
// (not a subclass) returns the PEP 3151 subclass for the errno.  When a
// filename is given, .args shrinks to (errno, strerror).
func OSErrorNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	if len(args) >= 2 && len(args) <= 5 {
		if en, ok := args[0].(Int); ok && metatype == OSError {
			metatype = errnoArgClass(int(en))
		}
	}
	e := exceptionNew(metatype, args)
	if len(args) >= 2 && len(args) <= 5 {
		e.Dict.Set("errno", args[0])
		e.Dict.Set("strerror", args[1])
		if len(args) > 2 && args[2] != None {
			e.Dict.Set("filename", args[2])
			if len(args) > 4 && args[4] != None {
				e.Dict.Set("filename2", args[4])
			}
			e.Args = Tuple{args[0], args[1]}
		}
	}
	return e, nil
}

// errnoArgClass maps an errno given to OSError() to its PEP 3151 subclass.
// The number is a POSIX/CRT errno, never a Win32 code, so it must not go
// through osErrorClass's fs.Err* checks: on Windows syscall.Errno(13) is
// ERROR_INVALID_DATA, not EACCES.  ENOENT, EEXIST, EACCES and EPERM have the
// same numbers on Linux, macOS and the Windows CRT.
func errnoArgClass(n int) *Type {
	switch n {
	case 2:
		return FileNotFoundError
	case 17:
		return FileExistsError
	case 1, 13:
		return PermissionError
	}
	return errnoClass(n)
}

// oserrorStr is OSError.__str__.  It only formats the (errno, strerror) form;
// any other shape (including errors the interpreter itself raises with one
// preformatted message) falls through to BaseException's rule.
func oserrorStr(self Object, args Tuple) (Object, error) {
	e, ok := self.(*Exception)
	if !ok {
		return nil, ExceptionNewf(TypeError, "descriptor '__str__' requires an 'OSError' object")
	}
	if a, ok := e.Args.(Tuple); ok && len(a) == 2 {
		r := func(o Object) (string, error) { return ReprAsString(o) }
		code, err := StrAsString(a[0])
		if err != nil {
			return nil, err
		}
		msg, err := StrAsString(a[1])
		if err != nil {
			return nil, err
		}
		s := "[Errno " + code + "] " + msg
		if fn, ok := e.Dict.Get("filename"); ok && fn != None {
			q, err := r(fn)
			if err != nil {
				return nil, err
			}
			s += ": " + q
			if fn2, ok := e.Dict.Get("filename2"); ok && fn2 != None {
				q, err := r(fn2)
				if err != nil {
					return nil, err
				}
				s += " -> " + q
			}
		}
		return String(s), nil
	}
	return e.M__str__()
}

func init() {
	for _, t := range []*Type{OSError, BlockingIOError, ChildProcessError, ConnectionError, BrokenPipeError,
		ConnectionAbortedError, ConnectionRefusedError, ConnectionResetError, FileExistsError, FileNotFoundError,
		InterruptedError, IsADirectoryError, NotADirectoryError, PermissionError, ProcessLookupError, TimeoutError} {
		t.New = OSErrorNew
	}
	OSError.Dict.Set("__str__", MustNewMethod("__str__", oserrorStr, 0, "Return str(self)."))
	OSError.Dict.Set("filename2", &Property{Fget: func(self Object) (Object, error) {
		if e, ok := self.(*Exception); ok {
			if v, ok := e.Dict.Get("filename2"); ok {
				return v, nil
			}
		}
		return None, nil
	}})
}
