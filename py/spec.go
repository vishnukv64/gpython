// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"path/filepath"
	"strings"
	"sync/atomic"
)

// ModuleSpec is the module.__spec__ object: a description of how a module was
// found.
//
// CPython sets __spec__ on every imported module, and code reads it.  pip's
// __main__.py does exactly that - "if not __spec__ or __spec__.parent == ”" -
// to decide whether it is running out of a wheel, and without the attribute the
// very first line of "python -m pip" raised NameError.
//
// Only the fields that are actually read are modelled: name, parent, origin
// and submodule_search_locations.  A field that is not modelled is absent
// rather than wrong.
type ModuleSpec struct {
	name    string
	parent  string
	origin  Object
	submods Object
	// initializing is _initializing: the module's body is still running.
	initializing atomic.Bool
}

// ModuleSpecType is the type of a module spec.
var ModuleSpecType = NewType("ModuleSpec", "The specification of a module, as found by the import system.")

func (s *ModuleSpec) Type() *Type { return ModuleSpecType }

// NewModuleSpec returns a spec for a module of the given name.  origin is the
// path it was loaded from, or nil when there is none.
func NewModuleSpec(name string, origin Object, isPkg bool) *ModuleSpec {
	parent := ""
	if i := strings.LastIndex(name, "."); i >= 0 {
		parent = name[:i]
	}
	s := &ModuleSpec{name: name, parent: parent, origin: origin}
	if isPkg && origin != nil {
		s.submods = NewListFromItems([]Object{String(filepath.Dir(string(origin.(String))))})
	}
	return s
}

func init() {
	ModuleSpecType.Dict.Set("_initializing", &Property{
		Fget: func(self Object) (Object, error) { return NewBool(self.(*ModuleSpec).initializing.Load()), nil },
		Fset: func(self, value Object) error {
			b, err := MakeBool(value)
			self.(*ModuleSpec).initializing.Store(err == nil && b == True)
			return nil
		},
	})
	ModuleSpecType.Dict.Set("has_location", &Property{
		Fget: func(self Object) (Object, error) { return NewBool(self.(*ModuleSpec).origin != nil), nil },
	})
	ModuleSpecType.Dict.Set("name", &Property{
		Fget: func(self Object) (Object, error) { return String(self.(*ModuleSpec).name), nil },
	})
	ModuleSpecType.Dict.Set("parent", &Property{
		Fget: func(self Object) (Object, error) { return String(self.(*ModuleSpec).parent), nil },
	})
	ModuleSpecType.Dict.Set("origin", &Property{
		Fget: func(self Object) (Object, error) {
			if self.(*ModuleSpec).origin == nil {
				return None, nil
			}
			return self.(*ModuleSpec).origin, nil
		},
	})
	ModuleSpecType.Dict.Set("submodule_search_locations", &Property{
		Fget: func(self Object) (Object, error) {
			if self.(*ModuleSpec).submods == nil {
				return None, nil
			}
			return self.(*ModuleSpec).submods, nil
		},
		// Writable, as in CPython: six marks its own module a package with
		// "__spec__.submodule_search_locations = []".
		Fset: func(self, value Object) error {
			self.(*ModuleSpec).submods = value
			return nil
		},
	})
	ModuleSpecType.Dict.Set("__repr__", MustNewMethod("__repr__", func(self Object, args Tuple) (Object, error) {
		s := self.(*ModuleSpec)
		origin := "None"
		if s.origin != nil {
			origin = string(s.origin.(String))
		}
		return String("ModuleSpec(name='" + s.name + "', origin='" + origin + "')"), nil
	}, 0, "Return repr(self)."))
}
