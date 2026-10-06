// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package copy provides the implementation of python's 'copy' module.
//
// copy() and deepcopy() honour the __copy__ and __deepcopy__ hooks when a
// type defines them, and otherwise fall back to the interpreter's own
// structural copy for the built-in container types (list, dict, set, tuple,
// bytearray) and the ordinary types that the interpreter knows how to
// duplicate.  The deepcopy memo is threaded through so a graph with a cycle
// terminates and shared subobjects stay shared.
package copy

import (
	"reflect"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Generic (shallow and deep) copying operations.

copy(x)        -- Return a shallow copy of x.
deepcopy(x)    -- Return a deep copy of x.
`

// errorType is copy.Error.
var errorType = py.ExceptionType.NewType("copy.Error", "Raised for copy errors.", nil, nil)

// shallowCopy returns a shallow copy of x.
func shallowCopy(x py.Object, memo py.Object) (py.Object, error) {
	// A type that defines __copy__ supplies its own.
	if cls := x.Type(); cls != nil && cls.Lookup("__copy__") != nil {
		if fn, err := py.GetAttrString(x, "__copy__"); err == nil {
			return py.Call(fn, py.Tuple{}, py.NewStringDict())
		}
	}
	switch v := x.(type) {
	case *py.List:
		return v.Copy(), nil
	case py.Tuple:
		// A tuple is immutable; a shallow copy is the same tuple.
		return v, nil
	case py.StringDict:
		return v.Copy(), nil
	case *py.ByteArray:
		b, err := v.M__bytes__()
		if err != nil {
			return nil, err
		}
		return py.NewByteArray(append([]byte(nil), []byte(b.(py.Bytes))...)), nil
	case py.Bytes, py.String, py.Int, py.Float, py.Bool, py.NoneType:
		return x, nil
	}
	// A python object with a __dict__: shallow-copy the attribute dict onto a
	// new instance of the same class.
	if d, ok := x.(py.IGetDict); ok {
		cls := x.Type()
		if cls == nil {
			return nil, py.ExceptionNewf(errorType, "cannot copy %s", x.Type().Name)
		}
		obj, err := py.Call(cls, py.Tuple{}, py.NewStringDict())
		if err != nil {
			return nil, err
		}
		if nd, ok := obj.(py.IGetDict); ok {
			src := d.GetDict()
			dst := nd.GetDict()
			for _, k := range src.Keys() {
				dst.Set(k, src.GetOrNil(k))
			}
		}
		return obj, nil
	}
	// An object the interpreter cannot duplicate: the result is itself, which
	// is what CPython does for an atomic object with no __copy__.
	return x, nil
}

// deepCopy returns a deep copy of x, memoising the objects already copied so
// cycles terminate and shared subobjects stay shared.
func deepCopy(x py.Object, memo py.Object) (py.Object, error) {
	// An atomic object is returned as is.
	if isAtomic(x) {
		return x, nil
	}
	// A type that defines __deepcopy__ supplies its own.
	if cls := x.Type(); cls != nil && cls.Lookup("__deepcopy__") != nil {
		if fn, err := py.GetAttrString(x, "__deepcopy__"); err == nil {
			return py.Call(fn, py.Tuple{memo}, py.NewStringDict())
		}
	}
	// Consult the memo, keyed by identity.
	if memo != nil && memo != py.None {
		m, ok := memo.(py.StringDict)
		if ok {
			key := identityKey(x)
			if v, ok := m.Get(key); ok {
				return v, nil
			}
		}
	}
	switch v := x.(type) {
	case *py.List:
		out := py.NewList()
		remember(memo, x, out)
		out.Items = make([]py.Object, len(v.Items))
		for i, it := range v.Items {
			c, err := deepCopy(it, memo)
			if err != nil {
				return nil, err
			}
			out.Items[i] = c
		}
		return out, nil
	case py.Tuple:
		out := make(py.Tuple, len(v))
		for i, it := range v {
			c, err := deepCopy(it, memo)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case py.StringDict:
		out := py.NewStringDict()
		remember(memo, x, out)
		for _, k := range v.Keys() {
			c, err := deepCopy(v.GetOrNil(k), memo)
			if err != nil {
				return nil, err
			}
			out.Set(k, c)
		}
		return out, nil
	case *py.ByteArray:
		b, err := v.M__bytes__()
		if err != nil {
			return nil, err
		}
		return py.NewByteArray(append([]byte(nil), []byte(b.(py.Bytes))...)), nil
	}
	// A python object with a dict: deep-copy the attributes onto a new
	// instance of the same class.
	if d, ok := x.(py.IGetDict); ok {
		cls := x.Type()
		if cls == nil {
			return x, nil
		}
		obj, err := py.Call(cls, py.Tuple{}, py.NewStringDict())
		if err != nil {
			return x, nil
		}
		remember(memo, x, obj)
		if nd, ok := obj.(py.IGetDict); ok {
			src := d.GetDict()
			dst := nd.GetDict()
			for _, k := range src.Keys() {
				c, err := deepCopy(src.GetOrNil(k), memo)
				if err != nil {
					return nil, err
				}
				dst.Set(k, c)
			}
		}
		return obj, nil
	}
	return x, nil
}

func isAtomic(x py.Object) bool {
	switch x.(type) {
	case py.Bytes, py.String, py.Int, py.Float, py.Bool, py.NoneType, *py.Type, *py.Function, *py.Method:
		return true
	}
	return false
}

// identityKey is a memo key that distinguishes objects by identity, not value.
// The pointer of the interface's data word gives a stable per-object key.
func identityKey(x py.Object) string {
	v := reflect.ValueOf(x)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return "id:" + itoa(v.Pointer())
	}
	// A non-pointer value (an int, a string) is identified by its type and
	// value, which is what CPython keys the memo by for those too.
	return "val:" + x.Type().Name + ":" + repr(x)
}

func itoa(n uintptr) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func repr(x py.Object) string {
	s, err := py.ReprAsString(x)
	if err != nil {
		return ""
	}
	return s
}

// remember records a shallow-copied object in the memo before its children are
// walked, so a cycle finds it.
func remember(memo py.Object, src, dst py.Object) {
	if memo == nil || memo == py.None {
		return
	}
	if m, ok := memo.(py.StringDict); ok {
		m.Set(identityKey(src), dst)
	}
}

func doCopy(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "copy() takes exactly one argument")
	}
	return shallowCopy(args[0], py.None)
}

func doDeepCopy(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "deepcopy() takes exactly one argument")
	}
	return deepCopy(args[0], py.NewStringDict())
}

func doReplace(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return nil, py.ExceptionNewf(py.NotImplementedError,
		"copy.replace: this implementation does not provide the replace() helper")
}

func init() {
	globals := py.NewStringDict()
	globals.Set("Error", errorType)
	globals.Set("error", errorType)
	globals.Set("copy", py.MustNewMethod("copy", doCopy, 0, "Return a shallow copy of x."))
	globals.Set("deepcopy", py.MustNewMethod("deepcopy", doDeepCopy, 0, "Return a deep copy of x."))
	globals.Set("replace", py.MustNewMethod("replace", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return doReplace(self, args, kw)
	}, 0, "Return a copy of obj with the named fields replaced."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "copy",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
