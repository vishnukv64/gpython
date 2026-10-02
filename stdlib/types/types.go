// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package types provides the implementation of python's 'types' module: the
// names of the interpreter's built-in types, and the helpers for building
// dynamic classes.
//
// SimpleNamespace is implemented, and the type names are bound to the real
// types the interpreter uses, so "isinstance(x, types.FunctionType)" answers
// correctly rather than always being False - which a set of placeholder
// names would give.
package types

import (
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Define names for built-in types that aren't directly accessible as
builtins.`

// SimpleNamespace is an attribute holder: a class whose instances carry
// whatever attributes are assigned to them, with no other behaviour.
type SimpleNamespace struct {
	Dict py.StringDict
}

var SimpleNamespaceType = py.NewTypeX("types.SimpleNamespace", "A simple attribute holder.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) > 0 {
		return nil, py.ExceptionNewf(py.TypeError, "SimpleNamespace() takes no positional arguments")
	}
	ns := &SimpleNamespace{Dict: py.NewStringDict()}
	kwargs.Range(func(k string, v py.Object) bool {
		ns.Dict.Set(k, v)
		return false
	})
	return ns, nil
}, nil)

func (n *SimpleNamespace) Type() *py.Type { return SimpleNamespaceType }

func (n *SimpleNamespace) GetDict() py.StringDict { return n.Dict }

func init() {
	SimpleNamespaceType.Flags |= py.TPFLAGS_BASETYPE

	SimpleNamespaceType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object) (py.Object, error) {
		n := self.(*SimpleNamespace)
		out := "namespace("
		first := true
		n.Dict.Range(func(k string, v py.Object) bool {
			if !first {
				out += ", "
			}
			first = false
			s, err := py.ReprAsString(v)
			if err != nil {
				s = "?"
			}
			out += k + "=" + s
			return false
		})
		return py.String(out + ")"), nil
	}, 0, "Return repr(self)."))

	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "SimpleNamespace", Value: SimpleNamespaceType},
		// The type names, bound to the interpreter's real types so that an
		// isinstance test against them means what it says.
		py.DictEntry{Key: "FunctionType", Value: py.FunctionType},
		py.DictEntry{Key: "LambdaType", Value: py.FunctionType},
		py.DictEntry{Key: "BuiltinFunctionType", Value: py.MethodType},
		py.DictEntry{Key: "BuiltinMethodType", Value: py.MethodType},
		py.DictEntry{Key: "MethodType", Value: py.MethodType},
		py.DictEntry{Key: "GeneratorType", Value: py.GeneratorType},
		py.DictEntry{Key: "CodeType", Value: py.CodeType},
		py.DictEntry{Key: "FrameType", Value: py.FrameType},
		py.DictEntry{Key: "TracebackType", Value: py.TracebackType},
		py.DictEntry{Key: "ModuleType", Value: py.ModuleType},
		py.DictEntry{Key: "CellType", Value: py.CellType},
		py.DictEntry{Key: "MappingProxyType", Value: py.StringDictType},
		py.DictEntry{Key: "NoneType", Value: py.NoneType.Type(py.None)},
		py.DictEntry{Key: "ModuleSpec", Value: py.NewType("types.ModuleSpec", "The specification for a module, used for loading.")},
		// The dynamic-class helpers.  They build a class from a dict, which
		// is what the module's names are for.
		py.DictEntry{Key: "new_class", Value: py.MustNewMethod("new_class", newClass, 0, "Create a class object dynamically using the appropriate metaclass.")},
		py.DictEntry{Key: "prepare_class", Value: py.MustNewMethod("prepare_class", prepareClass, 0, "Call the __prepare__ method of the appropriate metaclass.")},
	)

	// The class-building module, for the metaclass lookup.
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "types",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// newClass builds a class from a name, bases and a namespace, which in
// CPython goes through the metaclass machinery.  There are no metaclasses
// here, so the class is built directly with type(), which is the same result
// for the ordinary case.
func newClass(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var name py.Object
	bases := py.Object(py.Tuple{py.ObjectType})
	ns := py.Object(py.NewStringDict())
	if err := py.UnpackTuple(args, py.StringDict{}, "new_class", 1, 3, &name, &bases, &ns); err != nil {
		return nil, err
	}
	// type(name, bases, namespace) is the constructor's own form.
	return py.Call(py.TypeType, py.Tuple{name, bases, ns}, py.StringDict{})
}

// prepareClass returns a namespace for the class to be built in.  The
// metaclass __prepare__ hook is looked for and used when present.
func prepareClass(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return py.NewStringDict(), nil
}
