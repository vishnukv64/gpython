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

// payloadUnlessOverridden is payloadOf for a protocol the class may implement
// in Python: "class S(str): def __len__" must win over the carried value's.
func payloadUnlessOverridden(self Object, dunder string) (Object, bool) {
	p, ok := payloadOf(self)
	if ok && self.(*Type).LookupPython(dunder) != nil {
		return nil, false
	}
	return p, ok
}

func payloadOf(self Object) (Object, bool) {
	t, ok := self.(*Type)
	if !ok || t.Payload == nil {
		return nil, false
	}
	return t.Payload, true
}

// hasPayload reports whether an object is a payload-carrying instance.

// payloadLen answers len() from the payload.

// payloadGetItem answers "self[key]" from the payload.

// payloadContains answers "x in self" from the payload.

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

// scalarOf returns the native str/int/float/bytes/complex an instance of a
// Python subclass of one carries.  It is the single test for "this argument is
// really a builtin scalar in a subclass wrapper".
func scalarOf(o Object) (Object, bool) {
	t, ok := o.(*Type)
	if !ok || t.Payload == nil {
		return nil, false
	}
	switch t.Payload.(type) {
	case String, Int, *BigInt, Float, Bytes, Complex:
		return t.Payload, true
	}
	return nil, false
}

// UnwrapScalar returns the builtin value behind a scalar-subclass instance, or
// o itself.  Native code that type-asserts its argument uses it where a value
// may reach it without passing through Method.Call (list items, say).
func UnwrapScalar(o Object) Object {
	if p, ok := scalarOf(o); ok {
		return p
	}
	return o
}

// unwrapScalarArgs hands a native function the builtin value of every
// scalar-subclass argument.  Nothing is copied unless one is found.
func unwrapScalarArgs(args Tuple) Tuple {
	var out Tuple
	for i, a := range args {
		if p, ok := scalarOf(a); ok {
			if out == nil {
				out = append(Tuple(nil), args...)
			}
			out[i] = p
		}
	}
	if out == nil {
		return args
	}
	return out
}

// seqKey unwraps an int-subclass subscript for a builtin sequence, whose native
// index code type-asserts it.  Mappings keep the key as given (they store it).
func seqKey(self, key Object) Object {
	switch self.(type) {
	case *List, Tuple, String, Bytes, *ByteArray, *MemoryView:
		if p, ok := scalarOf(key); ok {
			switch p.(type) {
			case Int, *BigInt:
				return p
			}
		}
	}
	return key
}

func unwrapScalarKwargs(kw StringDict) StringDict {
	var out StringDict
	for _, e := range kw.Items() {
		if p, ok := scalarOf(e.Value); ok {
			if out.IsNil() {
				out = kw.Copy()
			}
			out.Set(e.Key, p)
		}
	}
	if out.IsNil() {
		return kw
	}
	return out
}

// keepArgsNames are native functions that must see (or store) the instance
// itself: introspection, formatting, and anything that keeps its argument.
// A method whose receiver is a builtin scalar (str.index) never keeps: a str
// stores nothing, and its methods type-assert their arguments.
var keepArgsNames = map[string]bool{
	"boundmethod": true, // types.MethodType(f, instance) keeps the instance
	"isinstance":  true, "issubclass": true, "type": true, "id": true, "repr": true,
	"str": true, "format": true, "print": true, "hash": true, "ascii": true,
	"getattr": true, "setattr": true, "hasattr": true, "delattr": true,
	"__getattribute__": true, "__setattr__": true, "__delattr__": true, "__format__": true,
	"__str__": true, "__repr__": true, "__eq__": true, "__ne__": true, "__hash__": true,
	"__init__": true, "__new__": true, "__init_subclass__": true,
	"append": true, "appendleft": true, "insert": true, "extend": true, "extendleft": true,
	"add": true, "setdefault": true, "__setitem__": true, "__contains__": true,
	"update": true, "put": true, "put_nowait": true, "push": true, "heappush": true,
	"max": true, "min": true, "sorted": true, "reversed": true, "next": true,
	"__reduce__": true, "__reduce_ex__": true, "__copy__": true, "__deepcopy__": true,
	// protocol-driven builtins: they call the instance's own __len__, __iter__...
	"len": true, "iter": true, "abs": true, "divmod": true, "pow": true,
	"sum": true, "any": true, "all": true, "list": true, "tuple": true, "set": true,
	"frozenset": true, "enumerate": true, "zip": true, "map": true, "filter": true,
	"bool": true, "dict": true,
	"copy": true, "deepcopy": true,
	"vars": true, "dir": true, "callable": true, "super": true, "partial": true,
	"__getitem__": true, "__delitem__": true, "fromkeys": true,
}

// scalarKind reports whether o is a bare builtin scalar value.
func scalarKind(o Object) (Object, bool) {
	switch o.(type) {
	case String, Int, *BigInt, Float, Bytes, Complex:
		return o, true
	}
	return nil, false
}

// scalarCtorDunders maps a builtin scalar type to the conversion hooks its
// constructor honours on an argument: int(x) calls x.__int__ even when x is an
// int subclass, so a Python override must reach it as the instance.
var scalarCtorDunders = []struct {
	t       *Type
	dunders []string
}{
	{IntType, []string{"__int__", "__index__", "__trunc__"}},
	{BigIntType, []string{"__int__", "__index__", "__trunc__"}},
	{FloatType, []string{"__float__", "__index__"}},
	{StringType, []string{"__str__"}},
	{BytesType, []string{"__bytes__"}},
	{ComplexType, []string{"__complex__"}},
}

// ctorArgs unwraps scalar-subclass arguments for the native constructor of a
// builtin scalar type (or a subclass of one).  The first argument keeps its
// wrapper when its class overrides the conversion hook the constructor uses.
func ctorArgs(t *Type, args Tuple, kwargs StringDict) (Tuple, StringDict) {
	for _, c := range scalarCtorDunders {
		if t == BoolType || !t.IsSubtype(c.t) {
			continue
		}
		var first Object
		if len(args) > 0 {
			first = args[0]
			if ft, ok := first.(*Type); ok && ft.Payload != nil {
				if c.t == StringType {
					// str(x) is repr(x) for a number subclass: leave it to Str.
					if _, isBytes := ft.Payload.(Bytes); !isBytes && !isStrPayload(ft.Payload) {
						return append(Tuple{first}, unwrapScalarArgs(args[1:])...), unwrapScalarKwargs(kwargs)
					}
				}
				for _, d := range c.dunders {
					if ft.LookupPython(d) != nil {
						rest := unwrapScalarArgs(args[1:])
						return append(Tuple{first}, rest...), unwrapScalarKwargs(kwargs)
					}
				}
			}
		}
		return unwrapScalarArgs(args), unwrapScalarKwargs(kwargs)
	}
	// Any other native type (pathlib.Path, io.BytesIO, re.compile...) asserts
	// its arguments just as a native function does.  A class with a Python
	// __init__ (or __new__, handled by the caller) receives them untouched, and
	// so do types that keep what they are given.
	if t.LookupPython("__init__") == nil && !keepArgsNames[t.Name] && !t.IsSubtype(BaseException) && !t.IsSubtype(TupleType) && !t.IsSubtype(DictType) && !t.IsSubtype(StringDictType) && t.Name != "deque" {
		return unwrapScalarArgs(args), unwrapScalarKwargs(kwargs)
	}
	return args, kwargs
}

// containerPayloadOf is payloadOf for tuple/list/set instances.  Those unwrap
// unconditionally; a scalar instance must first give its class's Python
// overrides a chance, which scalarOperands does.
func containerPayloadOf(o Object) (Object, bool) {
	p, ok := payloadOf(o)
	if !ok {
		return nil, false
	}
	if _, scalar := scalarKind(p); scalar {
		return nil, false
	}
	return p, true
}

// scalarOperands unwraps the scalar-subclass operands of a binary operation
// whose class defines neither the forward nor the reflected hook in Python.
func scalarOperands(a, b Object, fwd, rev string) (Object, Object, bool) {
	x, okA := scalarUnlessOverridden(a, fwd, rev)
	y, okB := scalarUnlessOverridden(b, fwd, rev)
	if !okA && !okB {
		return nil, nil, false
	}
	if !okA {
		x = a
	}
	if !okB {
		y = b
	}
	return x, y, true
}

func scalarUnlessOverridden(o Object, fwd, rev string) (Object, bool) {
	p, ok := scalarOf(o)
	if !ok {
		return nil, false
	}
	t := o.(*Type)
	if t.LookupPython(fwd) != nil || t.LookupPython(rev) != nil {
		return nil, false
	}
	return p, true
}

func isStrPayload(p Object) bool { _, ok := p.(String); return ok }
