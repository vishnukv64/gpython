// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package xattr provides a Go-native 'xattr' module: the module-level
// functions of PyPI's xattr (getxattr, setxattr, listxattr, removexattr).
// The PyPI package is a cffi extension gpython cannot build; poetry imports
// it on macOS to exclude a new virtualenv from Time Machine.  The xattr class
// is not provided.  On platforms without extended attributes the functions
// raise OSError(ENOTSUP).
package xattr

import (
	"github.com/vishnukv64/gpython/py"
)

type fn = func(py.Object, py.Tuple, py.StringDict) (py.Object, error)

func init() {
	g := py.NewStringDict()
	for n, f := range map[string]fn{
		"getxattr":    getxattr,
		"setxattr":    setxattr,
		"listxattr":   listxattr,
		"removexattr": removexattr,
	} {
		g.Set(n, py.MustNewMethod(n, f, 0, ""))
	}
	g.Set("__version__", py.String("1.3.0"))
	py.RegisterModule(&py.ModuleImpl{Info: py.ModuleInfo{Name: "xattr", Doc: "Extended attributes (Go-native subset)"}, Globals: g})
}

// str accepts str or bytes, as xattr does.
func str(o py.Object) (string, error) {
	switch v := o.(type) {
	case py.String:
		return string(v), nil
	case py.Bytes:
		return string(v), nil
	}
	return "", py.ExceptionNewf(py.TypeError, "expected str or bytes, not %s", o.Type().Name)
}

func parse(args py.Tuple, kwargs py.StringDict, format string, names []string, out ...*py.Object) ([]string, error) {
	if err := py.ParseTupleAndKeywords(args, kwargs, format, names, out...); err != nil {
		return nil, err
	}
	var s []string
	for _, o := range out {
		if *o == nil {
			continue
		}
		if v, ok := (*o).(py.Int); ok {
			s = append(s, string(rune(v)))
			continue
		}
		v, err := str(*o)
		if err != nil {
			return nil, err
		}
		s = append(s, v)
	}
	return s, nil
}

func getxattr(_ py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
	var p, n, nf py.Object
	s, err := parse(args, kw, "OO|O:getxattr", []string{"item", "attribute", "options"}, &p, &n, &nf)
	if err != nil {
		return nil, err
	}
	b, err := osGet(s[0], s[1])
	if err != nil {
		return nil, err
	}
	return py.Bytes(b), nil
}

func setxattr(_ py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
	var p, n, v, o py.Object
	if err := py.ParseTupleAndKeywords(args, kw, "OOO|O:setxattr", []string{"item", "attribute", "value", "options"}, &p, &n, &v, &o); err != nil {
		return nil, err
	}
	var ss [3]string
	for i, o := range []py.Object{p, n, v} {
		var err error
		if ss[i], err = str(o); err != nil {
			return nil, err
		}
	}
	if err := osSet(ss[0], ss[1], []byte(ss[2])); err != nil {
		return nil, err
	}
	return py.None, nil
}

func listxattr(_ py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
	var p, o py.Object
	s, err := parse(args, kw, "O|O:listxattr", []string{"item", "options"}, &p, &o)
	if err != nil {
		return nil, err
	}
	names, err := osList(s[0])
	if err != nil {
		return nil, err
	}
	items := make([]py.Object, len(names))
	for i, n := range names {
		items[i] = py.String(n)
	}
	return py.NewListFromItems(items), nil
}

func removexattr(_ py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
	var p, n, o py.Object
	s, err := parse(args, kw, "OO|O:removexattr", []string{"item", "attribute", "options"}, &p, &n, &o)
	if err != nil {
		return nil, err
	}
	if err := osRemove(s[0], s[1]); err != nil {
		return nil, err
	}
	return py.None, nil
}
