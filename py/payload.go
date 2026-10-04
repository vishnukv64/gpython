// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The value an instance of a Python subclass of a builtin container carries.
//
// An instance of a python-level class is a *Type whose namespace is a Dict, so
// "class D(dict)" works: the instance's Dict IS the mapping.  A SEQUENCE has
// positional items and a namespace has nowhere to put them, so
// "class L(list); L([1, 2])" produced an object whose len() raised, and
// "class _TokenType(tuple)" - pygments' token type, which pip needs - was not a
// tuple at all, so "Token.Text" raised AttributeError.
//
// Type.Payload holds the value the native constructor produced.  These helpers
// read it, and the four container protocols in the interpreter's shared entry
// points - Len, Iter, GetItem and the arithmetic - unwrap it there, so no
// Python attribute name is shadowed for objects that have no payload.
//
// Nothing here is registered on a type's namespace.  An earlier attempt did
// that and hijacked Len/Iter for every object, because a *Type is both a class
// and an instance and a name registered on ObjectType is inherited by both.

package py

// payloadOf returns the container an instance carries, and whether it has one.
//
// A CLASS never has one: Payload is set only where a value has been constructed,
// and Type.Alloc - which builds a class - leaves it nil.  That is what keeps
// "the class has a payload" from being indistinguishable from "an instance has
// one".
// PayloadOf returns the native value a python-level instance of a subclass
// carries, which is how a derived type's own Go value is reached from a shared
// helper that knows only the base.
func PayloadOf(self Object) (Object, bool) {
	return payloadOf(self)
}

func payloadOf(self Object) (Object, bool) {
	t, ok := self.(*Type)
	if !ok || t.Payload == nil {
		return nil, false
	}
	return t.Payload, true
}

// hasPayload reports whether an object is a payload-carrying instance.
func hasPayload(o Object) bool {
	_, ok := payloadOf(o)
	return ok
}

// payloadLen answers len() from the payload.
func payloadLen(self Object) (Object, error) {
	payload, ok := payloadOf(self)
	if !ok {
		return nil, ExceptionNewf(TypeError, "object of type '%s' has no len()", self.Type().Name)
	}
	return Len(payload)
}

// payloadGetItem answers "self[key]" from the payload.
func payloadGetItem(self Object, key Object) (Object, error) {
	payload, ok := payloadOf(self)
	if !ok {
		return nil, ExceptionNewf(TypeError, "'%s' object is not subscriptable", self.Type().Name)
	}
	return GetItem(payload, key)
}

// payloadContains answers "x in self" from the payload.
func payloadContains(self Object, item Object) (Object, error) {
	payload, ok := payloadOf(self)
	if !ok {
		return False, nil
	}
	found, err := SequenceContains(payload, item)
	if err != nil {
		return nil, err
	}
	return NewBool(found), nil
}

// rewrapPayload gives a freshly computed container value the class it should
// have, when an operation on a subclass instance produced one.
//
// The value is the same data; only the identity changes, which is what lets
// "a + (val,)" on a tuple subclass be an instance of that subclass rather than
// of tuple.  A value that is not a container of the expected shape is returned
// as it is, so an operation that legitimately produces something else is not
// coerced.
func rewrapPayload(cls *Type, value Object) Object {
	if cls == nil || cls.Name == "" {
		return value
	}
	if value.Type() == cls {
		return value
	}
	switch value.(type) {
	case Tuple, *List, *Set:
	default:
		return value
	}
	return &Type{
		ObjectType: cls,
		Base:       cls,
		Dict:       NewStringDict(),
		Payload:    unwrapPayload(value),
	}
}
