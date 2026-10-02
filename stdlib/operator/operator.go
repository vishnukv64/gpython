// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package operator provides python's 'operator' module: functions
// corresponding to Python's intrinsic operators.
//
// The set of functions here is the one pip and its vendored packages reach
// for: the comparison and arithmetic operators, and the three generalized
// lookup helpers attrgetter, itemgetter and methodcaller, which the vendored
// rich and resolvelib use as sort keys.
//
// The in-place operators (iadd and friends) are NOT provided: they need to
// rebind the caller's local variable, which a function call cannot do.  A
// caller that needs them writes the augmented assignment.
package operator

import (
	"reflect"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Operator Interface.

This module exports a set of functions corresponding to the intrinsic
operators of Python.  For example, operator.add(x, y) is equivalent to the
expression x+y.  The function names are those used for special methods;
variants without leading and trailing '__' are also provided for convenience.`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "operator",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			// Comparisons.
			py.MustNewMethod("lt", binop(func(a, b py.Object) (py.Object, error) { return py.Lt(a, b) }), 0, "lt(a, b) -- Same as a<b."),
			py.MustNewMethod("le", binop(func(a, b py.Object) (py.Object, error) { return py.Le(a, b) }), 0, "le(a, b) -- Same as a<=b."),
			py.MustNewMethod("eq", binop(func(a, b py.Object) (py.Object, error) { return py.Eq(a, b) }), 0, "eq(a, b) -- Same as a==b."),
			py.MustNewMethod("ne", binop(func(a, b py.Object) (py.Object, error) { return py.Ne(a, b) }), 0, "ne(a, b) -- Same as a!=b."),
			py.MustNewMethod("gt", binop(func(a, b py.Object) (py.Object, error) { return py.Gt(a, b) }), 0, "gt(a, b) -- Same as a>b."),
			py.MustNewMethod("ge", binop(func(a, b py.Object) (py.Object, error) { return py.Ge(a, b) }), 0, "ge(a, b) -- Same as a>=b."),
			py.MustNewMethod("is_", binop(func(a, b py.Object) (py.Object, error) { return py.NewBool(identical(a, b)), nil }), 0, "is_(a, b) -- Same as a is b."),
			py.MustNewMethod("is_not", binop(func(a, b py.Object) (py.Object, error) { return py.NewBool(!identical(a, b)), nil }), 0, "is_not(a, b) -- Same as a is not b."),
			py.MustNewMethod("is_none", unop(func(a py.Object) (py.Object, error) { return py.NewBool(a == py.None), nil }), 0, "is_none(a) -- Same as a is None."),
			py.MustNewMethod("is_not_none", unop(func(a py.Object) (py.Object, error) { return py.NewBool(a != py.None), nil }), 0, "is_not_none(a) -- Same as a is not None."),
			py.MustNewMethod("truth", unop(truth), 0, "truth(a) -- Return True if a is true, False otherwise."),
			py.MustNewMethod("not_", unop(not_), 0, "not_(a) -- Same as not a."),

			// Arithmetic.
			py.MustNewMethod("abs", unop(func(a py.Object) (py.Object, error) { return py.Abs(a) }), 0, "abs(a) -- Same as abs(a)."),
			py.MustNewMethod("add", binop(func(a, b py.Object) (py.Object, error) { return py.Add(a, b) }), 0, "add(a, b) -- Same as a + b."),
			py.MustNewMethod("and_", binop(func(a, b py.Object) (py.Object, error) { return py.And(a, b) }), 0, "and_(a, b) -- Same as a & b."),
			py.MustNewMethod("floordiv", binop(func(a, b py.Object) (py.Object, error) { return py.FloorDiv(a, b) }), 0, "floordiv(a, b) -- Same as a // b."),
			py.MustNewMethod("index", unop(index), 0, "index(a) -- Same as a.__index__()."),
			py.MustNewMethod("inv", unop(func(a py.Object) (py.Object, error) { return py.Invert(a) }), 0, "inv(a) -- Same as ~a."),
			py.MustNewMethod("invert", unop(func(a py.Object) (py.Object, error) { return py.Invert(a) }), 0, "invert(a) -- Same as ~a."),
			py.MustNewMethod("lshift", binop(func(a, b py.Object) (py.Object, error) { return py.Lshift(a, b) }), 0, "lshift(a, b) -- Same as a << b."),
			py.MustNewMethod("mod", binop(func(a, b py.Object) (py.Object, error) { return py.Mod(a, b) }), 0, "mod(a, b) -- Same as a % b."),
			py.MustNewMethod("mul", binop(func(a, b py.Object) (py.Object, error) { return py.Mul(a, b) }), 0, "mul(a, b) -- Same as a * b."),
			py.MustNewMethod("matmul", binop(matmul), 0, "matmul(a, b) -- Same as a @ b."),
			py.MustNewMethod("neg", unop(func(a py.Object) (py.Object, error) { return py.Neg(a) }), 0, "neg(a) -- Same as -a."),
			py.MustNewMethod("or_", binop(func(a, b py.Object) (py.Object, error) { return py.Or(a, b) }), 0, "or_(a, b) -- Same as a | b."),
			py.MustNewMethod("pos", unop(func(a py.Object) (py.Object, error) { return py.Pos(a) }), 0, "pos(a) -- Same as +a."),
			py.MustNewMethod("pow", pow, 0, "pow(a, b) -- Same as a ** b."),
			py.MustNewMethod("rshift", binop(func(a, b py.Object) (py.Object, error) { return py.Rshift(a, b) }), 0, "rshift(a, b) -- Same as a >> b."),
			py.MustNewMethod("sub", binop(func(a, b py.Object) (py.Object, error) { return py.Sub(a, b) }), 0, "sub(a, b) -- Same as a - b."),
			py.MustNewMethod("truediv", binop(func(a, b py.Object) (py.Object, error) { return py.TrueDiv(a, b) }), 0, "truediv(a, b) -- Same as a / b."),
			py.MustNewMethod("xor", binop(func(a, b py.Object) (py.Object, error) { return py.Xor(a, b) }), 0, "xor(a, b) -- Same as a ^ b."),

			// Sequence operations.
			py.MustNewMethod("concat", binop(concat), 0, "concat(a, b) -- Same as a + b, for a and b sequences."),
			py.MustNewMethod("contains", binop(contains), 0, "contains(a, b) -- Same as b in a (note reversed operands)."),
			py.MustNewMethod("countOf", binop(countOf), 0, "countOf(a, b) -- Return the number of times b occurs in a."),
			py.MustNewMethod("indexOf", binop(indexOf), 0, "indexOf(a, b) -- Return the first index of b in a."),
			py.MustNewMethod("getitem", binop(func(a, b py.Object) (py.Object, error) { return py.GetItem(a, b) }), 0, "getitem(a, b) -- Same as a[b]."),
			py.MustNewMethod("setitem", func(self py.Object, args py.Tuple) (py.Object, error) { return setitem(args) }, 0, "setitem(a, b, c) -- Same as a[b] = c."),
			py.MustNewMethod("delitem", binop(func(a, b py.Object) (py.Object, error) { return py.DelItem(a, b) }), 0, "delitem(a, b) -- Same as del a[b]."),
			py.MustNewMethod("length_hint", lengthHint, 0, length_hint_doc),

			// Miscellaneous.
			py.MustNewMethod("call", call, 0, "call(obj, /, *args, **kwargs) -- Same as obj(*args, **kwargs)."),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "attrgetter", Value: AttrGetterType},
			py.DictEntry{Key: "itemgetter", Value: ItemGetterType},
			py.DictEntry{Key: "methodcaller", Value: MethodCallerType},
		),
	})
}

// binop adapts a two-argument Go function into a module method.
func binop(f func(a, b py.Object) (py.Object, error)) func(py.Object, py.Tuple) (py.Object, error) {
	return func(self py.Object, args py.Tuple) (py.Object, error) {
		var a, b py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "operator", 2, 2, &a, &b); err != nil {
			return nil, err
		}
		return f(a, b)
	}
}

// unop adapts a one-argument Go function into a module method.
func unop(f func(a py.Object) (py.Object, error)) func(py.Object, py.Tuple) (py.Object, error) {
	return func(self py.Object, args py.Tuple) (py.Object, error) {
		var a py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "operator", 1, 1, &a); err != nil {
			return nil, err
		}
		return f(a)
	}
}

// identical reports whether a and b are the same object.
//
// This is the "is" operator.  It cannot be Go's ==: the dynamic types that
// reach here include the slice-backed ones (Tuple, List, Bytes), for which Go
// has no == at all.  reflect identifies a slice, map or func by the address of
// its data, which is exactly object identity for these representations.
func identical(a, b py.Object) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	switch ta.Kind() {
	case reflect.Slice, reflect.Map, reflect.Func:
		if ta.Comparable() {
			return a == b
		}
		va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
		return va.Pointer() == vb.Pointer()
	default:
		return ta.Comparable() && a == b
	}
}

func truth(a py.Object) (py.Object, error) {
	b, err := py.ObjectIsTrue(a)
	if err != nil {
		return nil, err
	}
	return py.NewBool(b), nil
}

func not_(a py.Object) (py.Object, error) {
	b, err := py.ObjectIsTrue(a)
	if err != nil {
		return nil, err
	}
	return py.NewBool(!b), nil
}

// index is operator.index: the __index__ result, and a TypeError for a value
// that has no __index__ - which is why len() semantics are not used here.
func index(a py.Object) (py.Object, error) {
	n, err := py.Index(a)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "'%s' object cannot be interpreted as an integer", a.Type().Name)
	}
	return n, nil
}

// pow implements the two-argument form of operator.pow.  The three-argument
// modular form is a method on int and is reached through __pow__, not here.
func pow(self py.Object, args py.Tuple) (py.Object, error) {
	var a, b py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "pow", 2, 2, &a, &b); err != nil {
		return nil, err
	}
	return py.Pow(a, b, py.None)
}

// matmul has no counterpart in this interpreter: the type slots carry no
// __matmul__, so no object can support @.  The function exists so that a
// caller can import it, and it raises the same TypeError CPython raises for
// operands that do not implement the operator.
func matmul(a, b py.Object) (py.Object, error) {
	return nil, py.ExceptionNewf(py.TypeError, "unsupported operand type(s) for @: '%s' and '%s'", a.Type().Name, b.Type().Name)
}

func concat(a, b py.Object) (py.Object, error) {
	switch a.(type) {
	case py.String, py.Bytes, py.Tuple, *py.List:
		return py.Add(a, b)
	}
	return nil, py.ExceptionNewf(py.TypeError, "cannot concatenate '%s' and '%s' objects", a.Type().Name, b.Type().Name)
}

func contains(a, b py.Object) (py.Object, error) {
	found, err := py.SequenceContains(a, b)
	if err != nil {
		return nil, err
	}
	return py.NewBool(found), nil
}

// countOf counts occurrences of b in a.  CPython calls a.count(b); the same
// result comes from iterating, which also works for the types that have no
// count method.
func countOf(a, b py.Object) (py.Object, error) {
	if c, err := py.GetAttrString(a, "count"); err == nil {
		return py.Call(c, py.Tuple{b}, py.StringDict{})
	}
	iter, err := py.Iter(a)
	if err != nil {
		return nil, err
	}
	n := 0
	for {
		item, err := py.Next(iter)
		if err == py.StopIteration {
			break
		}
		if err != nil {
			return nil, err
		}
		eq, err := py.Eq(item, b)
		if err != nil {
			return nil, err
		}
		if ok, _ := py.ObjectIsTrue(eq); ok {
			n++
		}
	}
	return py.Int(n), nil
}

// indexOf returns the first index of b in a, raising ValueError when it is
// absent, as the sequence index method does.
func indexOf(a, b py.Object) (py.Object, error) {
	if f, err := py.GetAttrString(a, "index"); err == nil {
		return py.Call(f, py.Tuple{b}, py.StringDict{})
	}
	iter, err := py.Iter(a)
	if err != nil {
		return nil, err
	}
	i := 0
	for {
		item, err := py.Next(iter)
		if err == py.StopIteration {
			break
		}
		if err != nil {
			return nil, err
		}
		eq, err := py.Eq(item, b)
		if err != nil {
			return nil, err
		}
		if ok, _ := py.ObjectIsTrue(eq); ok {
			return py.Int(i), nil
		}
		i++
	}
	return nil, py.ExceptionNewf(py.ValueError, "%s is not in sequence", reprShort(b))
}

func setitem(args py.Tuple) (py.Object, error) {
	var a, b, c py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "setitem", 3, 3, &a, &b, &c); err != nil {
		return nil, err
	}
	if _, err := py.SetItem(a, b, c); err != nil {
		return nil, err
	}
	return py.None, nil
}

const length_hint_doc = `length_hint(obj, default=0) -> int

Return an estimate of the number of items in obj.  This is useful for
presizing containers when building from an iterable.  If the object supports
len(), the result is exact; otherwise the default is returned.`

// lengthHint is operator.length_hint: the exact length where the object has
// one, otherwise the type's __length_hint__, otherwise default.
func lengthHint(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var obj py.Object
	default_ := py.Object(py.Int(0))
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:length_hint", []string{"obj", "default"}, &obj, &default_); err != nil {
		return nil, err
	}
	if _, ok := default_.(py.Int); !ok {
		return nil, py.ExceptionNewf(py.TypeError, "'%s' object cannot be interpreted as an integer", default_.Type().Name)
	}
	if n, err := py.Len(obj); err == nil {
		return n, nil
	}
	if hint, err := py.GetAttrString(obj, "__length_hint__"); err == nil {
		if v, err := py.Call(hint, py.Tuple{}, py.StringDict{}); err == nil && v != py.NotImplemented {
			n, ok := v.(py.Int)
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "__length_hint__ must be integer, not %s", v.Type().Name)
			}
			if n < 0 {
				return nil, py.ExceptionNewf(py.ValueError, "__length_hint__() should return >= 0")
			}
			return n, nil
		}
	}
	return default_, nil
}

// call is operator.call(obj, *args, **kwargs), which is obj(*args, **kwargs).
func call(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "call() missing 1 required positional argument: 'obj'")
	}
	return py.Call(args[0], args[1:], kwargs)
}

// reprShort renders a value for an error message, falling back to the type
// name when its repr raises.
func reprShort(o py.Object) string {
	if s, err := py.ReprAsString(o); err == nil {
		return s
	}
	return o.Type().Name
}

// attrName splits an attrgetter name on '.', so that "a.b" is a nested lookup
// rather than an attribute literally called "a.b".
func attrName(name string) []string {
	return strings.Split(name, ".")
}
