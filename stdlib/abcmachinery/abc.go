// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package abc provides the implementation of python's 'abc' module: the
// machinery for abstract base classes.
//
// Python builds this on ABCMeta, a metaclass, and metaclasses are not
// supported here: "class Meta(type)" is refused with "type 'type' is not an
// acceptable base type".  What is here instead is the part that has a
// run-time effect for code that uses the module in the ordinary way:
//
//   - ABC is a real class, so "class Foo(ABC)" works and Foo is a normal
//     class with ABC among its bases.
//   - ABCMeta is the type() metaclass, so "metaclass=ABCMeta" is accepted
//     and builds an ordinary class.
//   - @abstractmethod and friends mark the function, so
//     __isabstractmethod__ reads correctly, which is what code that
//     inspects for abstract methods checks.
//
// What is NOT enforced: instantiating a class that still has abstract
// methods.  CPython refuses; here it succeeds, because detection needs the
// metaclass hook that does not exist.  That is a real difference and is
// stated rather than hidden.
package abc

import (
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Abstract Base Classes

This module provides the infrastructure for defining abstract base classes
(ABCs) in Python, as outlined in PEP 3119.`

// ABC is the helper class that gives a class the abc machinery: a class
// deriving from it is an abstract base class.
var ABCType = py.NewType("abc.ABC", "Helper class that provides a standard way to create an ABC using inheritance.")

// ABCMeta is the metaclass for ABCs.  It is type() here: the metaclass
// behaviour that would mark a class abstract cannot be installed, so the
// name maps onto the ordinary class constructor and "metaclass=ABCMeta"
// builds a normal class instead of failing.
var ABCMetaType = py.TypeType

// markAbstract records the abstract marker on whatever it is given, which is
// a function for @abstractmethod and a property for @abstractproperty.
func markAbstract(obj py.Object, name string) (py.Object, error) {
	switch o := obj.(type) {
	case *py.Function:
		if o.Dict.IsNil() {
			o.Dict = py.NewStringDict()
		}
		o.Dict.Set("__isabstractmethod__", py.True)
		return o, nil
	case *py.Property:
		o.Abstract = trueValue()
		return o, nil
	}
	// Marking something else - a plain callable - is not an error in
	// CPython, so it is returned unchanged rather than raising.
	return obj, nil
}

func trueValue() bool { return true }

func init() {
	// A class is only subclassable here when it carries this flag, and ABC
	// exists to be subclassed: "class Foo(abc.ABC)".
	ABCType.Flags |= py.TPFLAGS_BASETYPE

	// __isabstractmethod__ is what code that inspects for abstract methods
	// reads; a property reports it from the marker set above.
	py.PropertyType.Dict.Set("__isabstractmethod__", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if self.(*py.Property).Abstract {
				return py.True, nil
			}
			return py.False, nil
		},
	})

	ABCType.Dict.Set("__class_getitem__", py.MustNewMethod("__class_getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		// "ABC[T]" is accepted, as in the annotations that use it.
		return self, nil
	}, 0, "Return the class, ignoring the subscription parameters."))

	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "ABC", Value: ABCType},
		py.DictEntry{Key: "ABCMeta", Value: ABCMetaType},
	)

	globals.Set("abstractmethod", py.MustNewMethod("abstractmethod", func(self py.Object, args py.Tuple) (py.Object, error) {
		var fn py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "abstractmethod", 1, 1, &fn); err != nil {
			return nil, err
		}
		return markAbstract(fn, "__isabstractmethod__")
	}, 0, "A decorator indicating abstract methods."))

	globals.Set("abstractproperty", py.MustNewMethod("abstractproperty", func(self py.Object, args py.Tuple) (py.Object, error) {
		var fn py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "abstractproperty", 1, 1, &fn); err != nil {
			return nil, err
		}
		return markAbstract(fn, "__isabstractmethod__")
	}, 0, "A decorator indicating abstract properties."))

	globals.Set("abstractclassmethod", py.MustNewMethod("abstractclassmethod", func(self py.Object, args py.Tuple) (py.Object, error) {
		var fn py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "abstractclassmethod", 1, 1, &fn); err != nil {
			return nil, err
		}
		return markAbstract(fn, "__isabstractmethod__")
	}, 0, "A decorator indicating abstract classmethods."))

	globals.Set("abstractstaticmethod", py.MustNewMethod("abstractstaticmethod", func(self py.Object, args py.Tuple) (py.Object, error) {
		var fn py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "abstractstaticmethod", 1, 1, &fn); err != nil {
			return nil, err
		}
		return markAbstract(fn, "__isabstractmethod__")
	}, 0, "A decorator indicating abstract staticmethods."))

	globals.Set("get_cache_token", py.MustNewMethod("get_cache_token", func(self py.Object, args py.Tuple) (py.Object, error) {
		if err := py.UnpackTuple(args, py.StringDict{}, "get_cache_token", 0, 0); err != nil {
			return nil, err
		}
		// There is no ABC registry to version, so the token is a constant.
		return py.Int(1), nil
	}, 0, "Return the current ABC cache token."))

	globals.Set("update_abstractmethods", py.MustNewMethod("update_abstractmethods", func(self py.Object, args py.Tuple) (py.Object, error) {
		var cls py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "update_abstractmethods", 1, 1, &cls); err != nil {
			return nil, err
		}
		// Nothing to recompute: abstractness is not tracked per class here.
		return cls, nil
	}, 0, "Recalculate the abstract method set of a class."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "abc",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
