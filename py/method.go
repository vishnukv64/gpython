// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Method objects
//
// This is about the type 'builtin_function_or_method', not Python
// methods in user-defined classes.  See class.go for the latter.

package py

import (
	"fmt"
)

// Types for methods

// Called with self and a tuple of args
type PyCFunction func(self Object, args Tuple) (Object, error)

// Called with self, a tuple of args and a stringdic of kwargs
type PyCFunctionWithKeywords func(self Object, args Tuple, kwargs StringDict) (Object, error)

// Called with self only
type PyCFunctionNoArgs func(Object) (Object, error)

// Called with one (unnamed) parameter only
type PyCFunction1Arg func(Object, Object) (Object, error)

const (
	// These two constants are not used to indicate the calling convention
	// but the binding when use with methods of classes. These may not be
	// used for functions defined for modules. At most one of these flags
	// may be set for any given method.

	// The method will be passed the type object as the first parameter
	// rather than an instance of the type. This is used to create class
	// methods, similar to what is created when using the classmethod()
	// built-in function.
	METH_CLASS = 0x0010

	// The method will be passed NULL as the first parameter rather than
	// an instance of the type. This is used to create static methods,
	// similar to what is created when using the staticmethod() built-in
	// function.
	METH_STATIC = 0x0020

	// One other constant controls whether a method is loaded in
	// place of another definition with the same method name.

	// The method will be loaded in place of existing definitions. Without
	// METH_COEXIST, the default is to skip repeated definitions. Since
	// slot wrappers are loaded before the method table, the existence of
	// a sq_contains slot, for example, would generate a wrapped method
	// named __contains__() and preclude the loading of a corresponding
	// PyCFunction with the same name. With the flag defined, the
	// PyCFunction will be loaded in place of the wrapper object and will
	// co-exist with the slot. This is helpful because calls to
	// PyCFunctions are optimized more than wrapper object calls.
	METH_COEXIST = 0x0040
)

// A python Method object
type Method struct {
	// Name of this function
	Name string
	// Doc string
	Doc string
	// Flags - see METH_* flags
	Flags int
	// Go function implementation
	method interface{}
	// Parent module of this method
	Module *Module
	// Unbound marks a Method reached through its CLASS rather than an
	// instance - "str.upper" - where CPython has an unbound method
	// descriptor.  Its first argument supplies the instance, as
	// str.upper(s) is s.upper(); see callUnbound.
	Unbound bool
}

// Internal method types implemented within eval.go
type InternalMethod int

const (
	InternalMethodNone InternalMethod = iota
	InternalMethodGlobals
	InternalMethodLocals
	InternalMethodImport
	InternalMethodEval
	InternalMethodExec
	InternalMethodVars
	InternalMethodDir
	InternalMethodGetFrame
)

var MethodType = NewType("method", "method object")

// Type of this object
func (o *Method) Type() *Type {
	return MethodType
}

// Define a new method
func NewMethod(name string, method interface{}, flags int, doc string) (*Method, error) {
	// have to write out the function arguments - can't use the
	// type aliases as they are different types :-(
	switch method.(type) {
	case func(self Object, args Tuple) (Object, error):
	case func(self Object, args Tuple, kwargs StringDict) (Object, error):
	case func(Object) (Object, error):
	case func(Object, Object) (Object, error):
	case InternalMethod:
	default:
		return nil, ExceptionNewf(SystemError, "Unknown function type for NewMethod %q, %T", name, method)
	}
	return &Method{
		Name:   name,
		Doc:    doc,
		Flags:  flags,
		method: method,
	}, nil
}

// As NewMethod but panics on error
func MustNewMethod(name string, method interface{}, flags int, doc string) *Method {
	m, err := NewMethod(name, method, flags, doc)
	if err != nil {
		panic(err)
	}
	return m
}

// Returns the InternalMethod type of this method
func (m *Method) Internal() InternalMethod {
	if internalMethod, ok := m.method.(InternalMethod); ok {
		return internalMethod
	}
	return InternalMethodNone
}

// Call the method with the given arguments
func (m *Method) Call(self Object, args Tuple) (Object, error) {
	// A native method on an instance of a Python subclass of a builtin container
	// receives the CONTAINER, not the wrapper.  "L([1,2]).append(3)" is
	// list.append, and the list it appends to is the value the instance carries;
	// without this the method asserted *List on a *Type and PANICKED with
	// "interface conversion: py.Object is *py.Type, not *py.List".
	//
	// This is the one place a native receiver is bound, so unwrapping here
	// covers every container method at once.  A method written in PYTHON is a
	// *Function rather than a *Method and never reaches this, so an override
	// still sees the instance it was written for.
	if payload, ok := payloadOf(self); ok {
		self = payload
	}
	switch f := m.method.(type) {
	case func(self Object, args Tuple) (Object, error):
		return f(self, args)
	case func(self Object, args Tuple, kwargs StringDict) (Object, error):
		return f(self, args, NewStringDict())
	case func(Object) (Object, error):
		if len(args) != 0 {
			return nil, ExceptionNewf(TypeError, "%s() takes no arguments (%d given)", m.Name, len(args))
		}
		return f(self)
	case func(Object, Object) (Object, error):
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "%s() takes exactly 1 argument (%d given)", m.Name, len(args))
		}
		return f(self, args[0])
	}
	panic(fmt.Sprintf("Unknown method type: %T", m.method))
}

// Call the method with the given arguments
func (m *Method) CallWithKeywords(self Object, args Tuple, kwargs StringDict) (Object, error) {
	if kwargs.Len() == 0 {
		return m.Call(self, args)
	}
	switch f := m.method.(type) {
	case func(self Object, args Tuple, kwargs StringDict) (Object, error):
		return f(self, args, kwargs)
	case func(self Object, args Tuple) (Object, error),
		func(Object) (Object, error),
		func(Object, Object) (Object, error):
		return nil, ExceptionNewf(TypeError, "%s() takes no keyword arguments", m.Name)
	}
	panic(fmt.Sprintf("Unknown method type: %T", m.method))
}

// Return a new Method with the bound method passed in, or an error
//
// This needs to convert the methods into internally callable python
// methods
func newBoundMethod(name string, fn interface{}) (Object, error) {
	m := &Method{
		Name: name,
	}
	switch f := fn.(type) {
	case func(args Tuple) (Object, error):
		m.method = func(_ Object, args Tuple) (Object, error) {
			return f(args)
		}
	// M__call__(args Tuple, kwargs StringDict) (Object, error)
	case func(args Tuple, kwargs StringDict) (Object, error):
		m.method = func(_ Object, args Tuple, kwargs StringDict) (Object, error) {
			return f(args, kwargs)
		}
	// M__str__() (Object, error)
	case func() (Object, error):
		m.method = func(_ Object) (Object, error) {
			return f()
		}
	// M__add__(other Object) (Object, error)
	case func(Object) (Object, error):
		m.method = func(_ Object, other Object) (Object, error) {
			return f(other)
		}
	// M__getattr__(name string) (Object, error)
	case func(string) (Object, error):
		m.method = func(_ Object, stringObject Object) (Object, error) {
			name, err := StrAsString(stringObject)
			if err != nil {
				return nil, err
			}
			return f(name)
		}
	// M__get__(instance, owner Object) (Object, error)
	case func(Object, Object) (Object, error):
		m.method = func(_ Object, args Tuple) (Object, error) {
			var a, b Object
			err := UnpackTuple(args, NewStringDict(), name, 2, 2, &a, &b)
			if err != nil {
				return nil, err
			}
			return f(a, b)
		}
	// M__new__(cls, args, kwargs Object) (Object, error)
	case func(Object, Object, Object) (Object, error):
		m.method = func(_ Object, args Tuple) (Object, error) {
			var a, b, c Object
			err := UnpackTuple(args, NewStringDict(), name, 3, 3, &a, &b, &c)
			if err != nil {
				return nil, err
			}
			return f(a, b, c)
		}
	default:
		return nil, fmt.Errorf("unknown bound method type for %q: %T", name, fn)
	}
	return m, nil
}

// Call a method
//
// bound reports whether this Method already carries its instance - see
// newBoundMethod, whose closure ignores the self it is passed.
func (m *Method) M__call__(args Tuple, kwargs StringDict) (Object, error) {
	// An unbound method takes its instance from the first argument.  A bound
	// one - including the closure newBoundMethod builds, which ignores the
	// self it is handed - carries it already.
	if m.Unbound {
		return m.callUnbound(args, kwargs)
	}
	self := Object(m.Module)
	if !kwargs.IsNil() {
		return m.CallWithKeywords(self, args, kwargs)
	}
	return m.Call(self, args)
}

// callUnbound calls a Method that was NOT reached through an instance -
// "str.upper" plucked off the class, or "min(xs, key=str.upper)".
//
// Such a method must not manufacture a self.  It used to pass the method's
// MODULE as self, so the implementation received a *py.Module where it asserted
// a String and the whole process died with "interface conversion: py.Object is
// *py.Module, not py.String" - not an exception, a panic.
//
// CPython's equivalent is an unbound method descriptor, and there the INSTANCE
// comes from the first argument: str.upper(s) is s.upper().  That is what
// happens here - the first argument becomes self, the rest the arguments.  With
// no argument at all there is no instance to work on, so it says so rather than
// being handed something arbitrary.
func (m *Method) callUnbound(args Tuple, kwargs StringDict) (Object, error) {
	if len(args) == 0 {
		return nil, ExceptionNewf(TypeError, "unbound method %s() needs an argument", m.Name)
	}
	self := args[0]
	reduced := args[1:]
	if !kwargs.IsNil() {
		return m.CallWithKeywords(self, reduced, kwargs)
	}
	return m.Call(self, reduced)
}

// Read a method from a class which makes a bound method
func (m *Method) M__get__(instance, owner Object) (Object, error) {
	if instance != None {
		return NewBoundMethod(instance, m), nil
	}
	// Read off the CLASS, so there is no instance yet.
	//
	// Whether that makes the method UNBOUND depends on whether it is an
	// instance method or a classmethod, and METH_CLASS is how this codebase
	// already spells the latter.  The signature cannot tell them apart: both
	// are written "func(self Object, args Tuple, ...)", and bytes.fromhex
	// simply ignores the self it is handed while str.upper uses it.
	//
	// Marking every class read as unbound ate the first argument of
	// bytes.fromhex, float.fromhex, int.from_bytes and date.fromisoformat at
	// once - each is registered as an instance method for want of a flag.
	if m.Flags&METH_CLASS != 0 || m.Flags&METH_STATIC != 0 {
		return m, nil
	}
	// Otherwise CPython hands back an unbound method descriptor, and the caller
	// supplies the instance as the first argument - str.upper(s) is s.upper().
	// Copying the Method keeps the shared one unmarked.
	unbound := *m
	unbound.Unbound = true
	return &unbound, nil
}

// FIXME this should be the default?
func (m *Method) M__eq__(other Object) (Object, error) {
	if otherMethod, ok := other.(*Method); ok && m == otherMethod {
		return True, nil
	}
	return False, nil
}

// FIXME this should be the default?
func (m *Method) M__ne__(other Object) (Object, error) {
	if otherMethod, ok := other.(*Method); ok && m == otherMethod {
		return False, nil
	}
	return True, nil
}

// Make sure it satisfies the interface
var _ Object = (*Method)(nil)
var _ I__call__ = (*Method)(nil)
var _ I__get__ = (*Method)(nil)
var _ I__eq__ = (*Method)(nil)
var _ I__ne__ = (*Method)(nil)
