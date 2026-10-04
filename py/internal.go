// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Internal interface for use from Go
//
// See arithmetic.go for the auto generated stuff

package py

import (
	"fmt"
	"reflect"
	"strings"
)

// AttributeName converts an Object to a string, raising a TypeError
// if it wasn't a String
func AttributeName(keyObj Object) (string, error) {
	if key, ok := keyObj.(String); ok {
		return string(key), nil
	}
	return "", ExceptionNewf(TypeError, "attribute name must be string, not '%s'", keyObj.Type().Name)
}

// Bool is called to implement truth value testing and the built-in
// operation bool(); should return False or True. When this method is
// not defined, __len__() is called, if it is defined, and the object
// is considered true if its result is nonzero. If a class defines
// neither __len__() nor __bool__(), all its instances are considered
// true.
func MakeBool(a Object) (Object, error) {
	if _, ok := a.(Bool); ok {
		return a, nil
	}

	// A payload-carrying instance is truthy exactly when its container is:
	// "bool(T())" on an empty tuple subclass must be False.  It answered True
	// because the lookup below found the identity __bool__ every object has.
	//
	// A __bool__ the CLASS defines still wins, because that is a *Function and
	// is checked first.
	if payload, ok := payloadOf(a); ok {
		if ty, isInst := a.(*Type); isInst && ty.LookupPython("__bool__") == nil {
			return MakeBool(payload)
		}
	}

	if A, ok := a.(I__bool__); ok {
		res, err := A.M__bool__()
		if err != nil {
			return nil, err
		}
		if res != NotImplemented {
			return res, nil
		}
	}

	if B, ok := a.(I__len__); ok {
		res, err := B.M__len__()
		if err != nil {
			return nil, err
		}
		if res != NotImplemented {
			return MakeBool(res)
		}
	}

	return True, nil
}

// Turns a into a go int if possible
func MakeGoInt(a Object) (int, error) {
	a, err := MakeInt(a)
	if err != nil {
		return 0, err
	}
	A, ok := a.(IGoInt)
	if ok {
		return A.GoInt()
	}
	return 0, ExceptionNewf(TypeError, "'%v' object cannot be interpreted as a go integer", a.Type().Name)
}

// Turns a into a go int64 if possible
func MakeGoInt64(a Object) (int64, error) {
	a, err := MakeInt(a)
	if err != nil {
		return 0, err
	}
	A, ok := a.(IGoInt64)
	if ok {
		return A.GoInt64()
	}
	return 0, ExceptionNewf(TypeError, "'%v' object cannot be interpreted as a go int64", a.Type().Name)
}

// Index the python Object returning an Int
//
// Will raise TypeError if Index can't be run on this object
func Index(a Object) (Int, error) {
	if A, ok := a.(I__index__); ok {
		return A.M__index__()
	}

	if A, ok, err := TypeCall0(a, "__index__"); ok {
		if err != nil {
			return 0, err
		}

		if res, ok := A.(Int); ok {
			return res, nil
		}

		return 0, ExceptionNewf(TypeError, "__index__ returned non-int: (type %s)", A.Type().Name)
	}

	return 0, ExceptionNewf(TypeError, "unsupported operand type(s) for index: '%s'", a.Type().Name)
}

// Index the python Object returning an int
//
// Will raise TypeError if Index can't be run on this object
//
// or IndexError if the Int won't fit!
func IndexInt(a Object) (int, error) {
	i, err := Index(a)
	if err != nil {
		return 0, err
	}
	intI := int(i)

	// Int might not fit in an int
	if Int(intI) != i {
		return 0, ExceptionNewf(IndexError, "cannot fit %d into an index-sized integer", i)
	}

	return intI, nil
}

// As IndexInt but if index is -ve addresses it from the end
//
// If index is out of range throws IndexError
func IndexIntCheck(a Object, max int) (int, error) {
	i, err := IndexInt(a)
	if err != nil {
		return 0, err
	}
	if i < 0 {
		i += max
	}
	if i < 0 || i >= max {
		return 0, ExceptionNewf(IndexError, "index out of range")
	}
	return i, nil
}

// IndexIntCheckNamed is IndexIntCheck, but names the sequence in the error the
// way CPython does: "list index out of range", "tuple index out of range",
// "string index out of range".
//
// A bare "index out of range" is what a program sees in a traceback, and it
// does not say which object was subscripted.
func IndexIntCheckNamed(a Object, max int, what string) (int, error) {
	i, err := IndexIntCheck(a, max)
	if err != nil {
		if e, ok := err.(*Exception); ok && e.Base == IndexError && what != "" {
			return 0, ExceptionNewf(IndexError, "%s index out of range", what)
		}
		return 0, err
	}
	return i, nil
}

// attributeOwner names the object an AttributeError is about, in CPython's
// form: an instance is "'V' object", a class is "type object 'V'", and a module
// is "module 'os'".
func attributeOwner(self Object) string {
	if self == nil {
		return "NoneType object"
	}
	name := self.Type().Name
	// A python-level CLASS is a *Type whose Name is set; an instance of one is
	// a *Type whose Name is empty.  The two must not read alike.
	if t, ok := self.(*Type); ok {
		if t.Name != "" {
			return "type object '" + t.Name + "'"
		}
	}
	if _, ok := self.(*Module); ok {
		return "module '" + name + "'"
	}
	return "'" + name + "' object"
}

// Returns the number of items of a sequence or mapping
func Len(self Object) (Object, error) {
	// An instance of a Python subclass of a builtin container carries its value
	// in Payload, and the length is the VALUE's.  Unwrapping here covers every
	// caller - the Go path and the method path alike - because this is the one
	// place both route through.
	if payload, ok := payloadOf(self); ok {
		return Len(payload)
	}
	if I, ok := self.(I__len__); ok {
		return I.M__len__()
	} else if res, ok, err := TypeCall0(self, "__len__"); ok {
		return res, err
	}
	return nil, ExceptionNewf(TypeError, "object of type '%s' has no len()", self.Type().Name)
}

// Return the result of not a
func Not(a Object) (Object, error) {
	b, err := MakeBool(a)
	if err != nil {
		return nil, err
	}
	switch b {
	case False:
		return True, nil
	case True:
		return False, nil
	}
	return nil, ExceptionNewf(TypeError, "bool() didn't return True or False")
}

// Calls function fnObj with args and kwargs in a new vm (or directly
// if Go code)
//
// kwargs should be nil if not required
//
// fnObj must be a callable type such as *py.Method or *py.Function
//
// The result is returned
func Call(fn Object, args Tuple, kwargs StringDict) (Object, error) {
	// An instance of a python class is represented as a Type with an EMPTY
	// NAME - see the FIXME in Type.M__repr__ - so it is a *Type and would
	// otherwise be caught by Type.M__call__ below, which is the constructor.
	// "h(5)" on a class defining __call__ therefore failed with "cannot
	// create '' instances".  Such an object is callable only if its CLASS
	// defines __call__, looked up as a descriptor so self is supplied.
	if t, ok := fn.(*Type); ok && t.Name == "" {
		if res, ok, err := TypeCall(fn, "__call__", append(Tuple{fn}, args...), kwargs); ok {
			return res, err
		}
		return nil, ExceptionNewf(TypeError, "'%s' object is not callable", t.Type().Name)
	}
	if I, ok := fn.(I__call__); ok {
		return I.M__call__(args, kwargs)
	}
	return nil, ExceptionNewf(TypeError, "'%s' object is not callable", fn.Type().Name)
}

// GetItem
func GetItem(self Object, key Object) (Object, error) {
	// The payload first, so "class L(list); L([1,2])[0]" is the element the list
	// holds rather than an item lookup on the instance's namespace.
	if payload, ok := payloadOf(self); ok {
		return GetItem(payload, key)
	}
	// "X[params]" on a class is __class_getitem__ in Python 3.7 and later.
	// The classes that define it accept any parameters - the abstract base
	// classes do, and the parameters only matter to a type checker - so the
	// class itself is the result.
	if t, ok := self.(*Type); ok {
		// "X[params]" on a CLASS is __class_getitem__ in Python 3.7 and later.
		// The classes that define it accept any parameters - the abstract base
		// classes do, and the parameters only matter to a type checker - so the
		// class itself is the result.
		//
		// Lookup walks the MRO; GetAttrOrNil does not, and only looking at the
		// class's own dict made "class C(CompositeParamType[T])" fail when
		// CompositeParamType's BASE was the thing providing the hook.
		//
		// ONLY for a class, not for an instance.  An instance of a python-level
		// class is a *Type with an EMPTY Name, and once dict is subclassable
		// such an instance inherits dict's __class_getitem__, so
		// "class D(dict): pass; d['a']" returned d ITSELF instead of the value
		// - the hook fired for the instance because the check was on the Go
		// type alone.
		if t.Name != "" {
			if t.Lookup("__class_getitem__") != nil {
				return self, nil
			}
			// PEP 585: a BUILTIN type is subscriptable on the class itself, and
			// the class is the result.  "deque[int]" and "defaultdict[str, int]"
			// are written as annotation defaults and get evaluated, so they must
			// answer "[" rather than fall through to __getitem__ and be treated
			// as an item lookup - which panicked with "interface conversion:
			// py.Object is *py.Type, not *collections.Deque" and took the whole
			// process down inside rich.
			//
			// The test is that the implementation is a Go *Method, i.e. a
			// builtin: a class written in Python keeps its __getitem__ as a Dict
			// entry holding a *Function, and for that a subscript really IS an
			// item lookup.
			if m, isMethod := t.Lookup("__getitem__").(*Method); isMethod && m != nil {
				return self, nil
			}
		}
	}
	if I, ok := self.(I__getitem__); ok {
		return I.M__getitem__(key)
	} else if res, ok, err := TypeCall1(self, "__getitem__", key); ok {
		return res, err
	}
	return nil, ExceptionNewf(TypeError, "'%s' object is not subscriptable", self.Type().Name)
}

// SetItem
func SetItem(self Object, key Object, value Object) (Object, error) {
	// The payload's element is the one assigned: "L([1])[0] = 9" is
	// list.__setitem__ on the list the instance carries.
	if payload, ok := payloadOf(self); ok {
		return SetItem(payload, key, value)
	}
	if I, ok := self.(I__setitem__); ok {
		return I.M__setitem__(key, value)
	} else if res, ok, err := TypeCall2(self, "__setitem__", key, value); ok {
		return res, err
	}

	return nil, ExceptionNewf(TypeError, "'%s' object does not support item assignment", self.Type().Name)
}

// Delitem
func DelItem(self Object, key Object) (Object, error) {
	if payload, ok := payloadOf(self); ok {
		return DelItem(payload, key)
	}
	if I, ok := self.(I__delitem__); ok {
		return I.M__delitem__(key)
	} else if res, ok, err := TypeCall1(self, "__delitem__", key); ok {
		return res, err
	}
	return nil, ExceptionNewf(TypeError, "'%s' object does not support item deletion", self.Type().Name)
}

// GetAttrString - returns the result or an err to be raised if not found
//
// If not found err will be an AttributeError
func GetAttrString(self Object, key string) (res Object, err error) {
	// Call __getattribute__ if it exists.
	//
	// For an INSTANCE that means whatever its class defines, which is the
	// point of the hook.  For a CLASS it does NOT: a class-level
	// __getattribute__ governs its instances, never the class itself, so
	// reading "type.__name__" must not run "type.__getattribute__".  The
	// distinction matters because this branch is reached before the ordinary
	// lookup - so a type that defines __getattribute__, as threading.local
	// does, otherwise answered EVERY attribute through it and lost its own
	// metadata: "threading.local.__name__" raised AttributeError.
	if I, ok := self.(I__getattribute__); ok {
		if _, isType := self.(*Type); !isType {
			return I.M__getattribute__(key)
		}
	}
	if _, isType := self.(*Type); !isType {
		if r, found, err2 := TypeCall1(self, "__getattribute__", Object(String(key))); found {
			return r, err2
		}
	}

	// Look up any __special__ methods as M__special__ and return a bound method.
	//
	// Not for an instance of a python class: those are represented as a Type
	// with an empty Name, so this would answer with the interpreter's own
	// M__call__ (the type constructor) where the user's __call__ was meant -
	// "h.__call__(5)" built a new object instead of calling the method.  The
	// ordinary lookup below finds the user's method.
	if _, isInstance := self.(*Type); !isInstance || self.(*Type).Name != "" {
		if len(key) >= 5 && strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__") {
			objectValue := reflect.ValueOf(self)
			methodValue := objectValue.MethodByName("M" + key)
			if methodValue.IsValid() {
				return newBoundMethod(key, methodValue.Interface())
			}
		}
	}

	// Look in the instance dictionary if it exists
	if I, ok := self.(IGetDict); ok {
		dict := I.GetDict()
		res, ok = dict.Get(key)
		if ok {
			// A type's dict is its class namespace, and class attributes
			// still go through the descriptor protocol: a staticmethod in
			// the class body yields the plain function, a classmethod a
			// method bound to the class, and a property the property
			// object itself (its getter is only run for an instance).
			if _, isType := self.(*Type); isType {
				if I, ok := res.(I__get__); ok {
					return I.M__get__(None, self)
				}
			}
			return res, err
		}
	}

	// Now look in type's dictionary etc
	t := self.Type()

	// A PURE-PYTHON instance is a *Type with an empty Name, and its attributes
	// must come from its class's MRO before the metatype is consulted: the
	// metatype describes the CLASS, not the instance.  Without this ordering an
	// attribute the metatype happens to have shadowed the instance's own.
	//
	// This does NOT fix a subclass of a NATIVE type - "class E(Filter)" - whose
	// instance is a Go struct rather than a *Type.  There type(e) reports the
	// native base and the base's methods win over the subclass's overrides,
	// which is a limitation of the object model, not of this lookup.  See the
	// note in py/type.go.
	if ty, isInstance := self.(*Type); isInstance && ty.Name == "" && ty.ObjectType != nil {
		t = ty.ObjectType
		if own := t.Lookup(key); own != nil {
			if I, ok := own.(I__get__); ok {
				return I.M__get__(self, t)
			}
			return own, nil
		}
	}

	res = t.NativeGetAttrOrNil(key)

	// A class's attributes live in its OWN bases before its metatype's:
	// "Sub.attr" has to reach Base.attr.  The metatype knows nothing of the
	// class's bases, and the metatype's result usually wins anyway because
	// Name, Doc, Dict and the rest are there - so the class's own lookup has
	// to be consulted whenever the metatype did not find anything, which is
	// exactly the inherited case.  Instances are untouched: their
	// t.Type().Lookup(key) already found the attribute, so res is not nil
	// and this is skipped.
	if res == nil {
		if ty, isType := self.(*Type); isType {
			if own := ty.Lookup(key); own != nil {
				// Bind as a CLASS access: a staticmethod yields the plain
				// function, a classmethod a method bound to the class.
				if I, ok := own.(I__get__); ok {
					return I.M__get__(None, self)
				}
				return own, nil
			}
		}
	}

	if res != nil {
		// Call __get__ which creates bound methods, reads properties etc
		if I, ok := res.(I__get__); ok {
			res, err = I.M__get__(self, t)
		}
		return res, err
	}

	// And now only if not found call __getattr__
	if I, ok := self.(I__getattr__); ok {
		return I.M__getattr__(key)
	} else if res, ok, err = TypeCall1(self, "__getattr__", Object(String(key))); ok {
		return res, err
	}

	// Not found - return nil
	// The name of the CLASS, not the metatype: for a class this prints
	// "type object 'V' has no attribute 'x'", which is CPython's wording and
	// says WHICH class - "type has no attribute 'x'" names nothing useful.
	return nil, ExceptionNewf(AttributeError, "%s has no attribute '%s'", attributeOwner(self), key)
}

// GetAttrErr - returns the result or an err to be raised if not found
//
// If not found an AttributeError will be returned
func GetAttr(self Object, keyObj Object) (res Object, err error) {
	key, err := AttributeName(keyObj)
	if err != nil {
		return nil, err
	}
	return GetAttrString(self, key)
}

// IDictSetter is implemented by an object whose instance namespace can be
// replaced wholesale, which is what "obj.__dict__ = other" does.
type IDictSetter interface {
	SetDict(StringDict)
}

// SetAttrString
func SetAttrString(self Object, key string, value Object) (Object, error) {
	// First look in type's dictionary etc for a property that could
	// be set - do this before looking in the instance dictionary
	setter := self.Type().NativeGetAttrOrNil(key)
	if setter != nil {
		// Call __set__ which writes properties etc
		if I, ok := setter.(I__set__); ok {
			return I.M__set__(self, value)
		}
	}

	// If we have __setattr__ then use that
	if I, ok := self.(I__setattr__); ok {
		return I.M__setattr__(key, value)
	} else if res, ok, err := TypeCall2(self, "__setattr__", String(key), value); ok {
		return res, err
	}

	// Otherwise set the attribute in the instance dictionary if
	// possible
	if I, ok := self.(IGetDict); ok {
		dict := I.GetDict()
		if dict.IsNil() {
			return nil, ExceptionNewf(SystemError, "nil Dict in %s", self.Type().Name)
		}
		// Assigning __dict__ REPLACES the instance's namespace; it is not an
		// attribute named "__dict__".  Treated as an ordinary key it made the
		// namespace itself invisible: "a.__dict__ = {...}" then showed the
		// mapping but "a.x" still raised, because lookup reads the namespace
		// and the mapping had been filed inside it.  rich copies
		// ConsoleOptions exactly this way - __new__ then __dict__ = ... - so
		// every copied options object was empty of its fields.
		if key == "__dict__" {
			newDict, ok := value.(StringDict)
			if !ok {
				return nil, ExceptionNewf(TypeError, "__dict__ must be set to a dict, not %s", value.Type().Name)
			}
			if setter, ok := self.(IDictSetter); ok {
				setter.SetDict(newDict)
				return None, nil
			}
		}
		dict.Set(key, value)
		return None, nil
	}

	// If not blow up
	return nil, ExceptionNewf(AttributeError, "'%s' object has no attribute '%s'", self.Type().Name, key)
}

// SetAttr
func SetAttr(self Object, keyObj Object, value Object) (Object, error) {
	key, err := AttributeName(keyObj)
	if err != nil {
		return nil, err
	}
	return SetAttrString(self, key, value)
}

// DeleteAttrString
func DeleteAttrString(self Object, key string) error {
	// First look in type's dictionary etc for a property that could
	// be set - do this before looking in the instance dictionary
	deleter := self.Type().NativeGetAttrOrNil(key)
	if deleter != nil {
		// Call __set__ which writes properties etc
		if I, ok := deleter.(I__delete__); ok {
			_, err := I.M__delete__(self)
			return err
		}
	}

	// If we have __delattr__ then use that
	if I, ok := self.(I__delattr__); ok {
		_, err := I.M__delattr__(key)
		return err
	} else if _, ok, err := TypeCall1(self, "__delattr__", String(key)); ok {
		return err
	}

	// Otherwise delete the attribute from the instance dictionary
	// if possible
	if I, ok := self.(IGetDict); ok {
		dict := I.GetDict()
		if dict.IsNil() {
			return ExceptionNewf(SystemError, "nil Dict in %s", self.Type().Name)
		}
		if _, ok := dict.Get(key); ok {
			dict.Del(key)
			return nil
		}
	}

	// If not blow up
	return ExceptionNewf(AttributeError, "'%s' object has no attribute '%s'", self.Type().Name, key)
}

// DeleteAttr
func DeleteAttr(self Object, keyObj Object) error {
	key, err := AttributeName(keyObj)
	if err != nil {
		return err
	}
	return DeleteAttrString(self, key)
}

// Calls __str__ on the object
//
// Calls __repr__ on the object or returns a sensible default
func Repr(self Object) (Object, error) {
	// A nil Object is an internal error, not a Python-level mistake, but it
	// must still not SEGFAULT the host process: the panic below came from a
	// nil field read on a dataclasses.Field, whose type is unset until the
	// decorator fills it in.
	if self == nil {
		return nil, ExceptionNewf(RuntimeError, "Repr called with a nil object")
	}
	if I, ok := self.(I__repr__); ok {
		return I.M__repr__()
	} else if res, ok, err := TypeCall0(self, "__repr__"); ok {
		return res, err
	}
	// CPython's fallback shape is "<name object at 0x...>", not "<name instance
	// at ...>".  The word matters: a program that scrapes or asserts on a repr
	// - a test framework, a doctest - reads it.
	return String(fmt.Sprintf("<%s object at %p>", self.Type().Name, self)), nil
}

// DebugRepr - see Repr but returns the repr or error as a string
func DebugRepr(self Object) string {
	res, err := Repr(self)
	if err != nil {
		return fmt.Sprintf("Repr(%s) returned %v", self.Type().Name, err)
	}
	str, ok := res.(String)
	if !ok {
		return fmt.Sprintf("Repr(%s) didn't return a string", self.Type().Name)
	}
	return string(str)
}

// Calls __str__ on the object and if not found calls __repr__
func Str(self Object) (Object, error) {
	if I, ok := self.(I__str__); ok {
		return I.M__str__()
	} else if res, ok, err := TypeCall0(self, "__str__"); ok {
		return res, err
	}
	return Repr(self)
}

// Returns object as a string
//
// Calls Str then makes sure the output is a string
func StrAsString(self Object) (string, error) {
	res, err := Str(self)
	if err != nil {
		return "", err
	}
	str, ok := res.(String)
	if !ok {
		return "", ExceptionNewf(TypeError, "result of __str__ must be string, not '%s'", res.Type().Name)
	}
	return string(str), nil
}

// Returns object as a string
//
// Calls Repr then makes sure the output is a string
func ReprAsString(self Object) (string, error) {
	res, err := Repr(self)
	if err != nil {
		return "", err
	}
	str, ok := res.(String)
	if !ok {
		return "", ExceptionNewf(TypeError, "result of __repr__ must be string, not '%s'", res.Type().Name)
	}
	return string(str), nil
}

// Returns an iterator object
//
// Call __Iter__ Returns an iterator object
//
// If object is sequence object, create an iterator
func Iter(self Object) (res Object, err error) {
	if payload, ok := payloadOf(self); ok {
		return Iter(payload)
	}
	if I, ok := self.(I__iter__); ok {
		return I.M__iter__()
	} else if res, ok, err = TypeCall0(self, "__iter__"); ok {
		return res, err
	}
	if ObjectIsSequence(self) {
		return NewIterator(self), nil
	}
	return nil, ExceptionNewf(TypeError, "'%s' object is not iterable", self.Type().Name)
}
