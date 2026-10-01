// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package future provides the implementation of python's '__future__' module.
//
// The module is a plain module of feature constants, which is all that
// "from __future__ import <name>" needs: the import machinery binds each
// name from the module's globals.  Since every feature this interpreter
// reports already behaves the modern way, the flags are informational.
package future

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Record of the features known to this interpreter.

Future statements are directives to the compiler enabling features that
were not yet part of the language.  The features recorded here are all
unconditionally enabled, so importing them has no effect beyond making
the name available.`

// release mirrors CPython's namedtuple used for optional/mandatory releases.
type release struct {
	py.Object
	values []py.Object
}

var releaseType = py.NewTypeX("_Feature.release", "sys.version_info for a future feature", nil, nil)

func newRelease(major, minor, micro int64, level string, serial int64) *release {
	return &release{values: []py.Object{
		py.Int(major), py.Int(minor), py.Int(micro), py.String(level), py.Int(serial),
	}}
}

func (r *release) Type() *py.Type { return releaseType }

func (r *release) M__len__() (int, error) { return len(r.values), nil }

func (r *release) M__getitem__(key py.Object) (py.Object, error) {
	if index, ok := key.(py.Int); ok {
		i, err := index.GoInt()
		if err != nil {
			return nil, err
		}
		if i < 0 {
			i += len(r.values)
		}
		if i < 0 || i >= len(r.values) {
			return nil, py.ExceptionNewf(py.IndexError, "index out of range")
		}
		return r.values[i], nil
	}
	fields := []string{"major", "minor", "micro", "releaselevel", "serial"}
	if name, ok := key.(py.String); ok {
		for i, field := range fields {
			if field == string(name) {
				return r.values[i], nil
			}
		}
	}
	return nil, py.ExceptionNewf(py.TypeError, "release indices must be integers or field names")
}

func (r *release) M__repr__() (py.Object, error) {
	parts := make([]string, len(r.values))
	for i, v := range r.values {
		s, err := py.ReprAsString(v)
		if err != nil {
			return nil, err
		}
		parts[i] = s
	}
	return py.String("_Feature.release(" + strings.Join(parts, ", ") + ")"), nil
}

// feature is the type of the module level constants.
type feature struct {
	optional  *release
	mandatory *release
}

var featureType = py.NewTypeX("_Feature", "A future feature flag", featureNew, nil)

func featureNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		name      py.Object
		optional  py.Object
		mandatory py.Object
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "s|OO:_Feature", []string{"name", "optional", "mandatory"}, &name, &optional, &mandatory)
	if err != nil {
		return nil, err
	}
	f := &feature{}
	if o, ok := optional.(*release); ok {
		f.optional = o
	}
	if m, ok := mandatory.(*release); ok {
		f.mandatory = m
	}
	return f, nil
}

func (f *feature) Type() *py.Type { return featureType }

func init() {
	featureType.Dict["optional"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			f := self.(*feature)
			if f.optional == nil {
				return py.None, nil
			}
			return f.optional, nil
		},
	}
	featureType.Dict["mandatory"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			f := self.(*feature)
			if f.mandatory == nil {
				return py.None, nil
			}
			return f.mandatory, nil
		},
	}

	// The release at which each feature became available, following CPython.
	// Features introduced before this interpreter's version get 2.1/2.2 etc.
	releases := map[string]*release{
		"nested_scopes":    newRelease(2, 1, 0, "beta", 1),
		"generators":       newRelease(2, 2, 0, "alpha", 1),
		"division":         newRelease(2, 2, 0, "alpha", 2),
		"absolute_import":  newRelease(2, 5, 0, "alpha", 1),
		"with_statement":   newRelease(2, 5, 0, "alpha", 1),
		"print_function":   newRelease(2, 6, 0, "alpha", 2),
		"unicode_literals": newRelease(2, 6, 0, "alpha", 2),
		"barry_as_FLUFL":   newRelease(3, 1, 0, "alpha", 2),
		"generator_stop":   newRelease(3, 5, 0, "beta", 1),
		"annotations":      newRelease(3, 7, 0, "beta", 1),
	}

	globals := py.StringDict{}
	names := make([]py.Object, 0, len(releases))
	for _, name := range featureNames {
		r := releases[name]
		globals[name] = &feature{optional: r, mandatory: r}
		names = append(names, py.String(name))
	}
	globals["all_feature_names"] = py.NewListFromItems(names)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "__future__",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// featureNames is the ordered list of feature names, as in CPython.
var featureNames = []string{
	"nested_scopes",
	"generators",
	"division",
	"absolute_import",
	"with_statement",
	"print_function",
	"unicode_literals",
	"barry_as_FLUFL",
	"generator_stop",
	"annotations",
}
