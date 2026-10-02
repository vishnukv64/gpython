// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Bool objects

package py

type Bool bool

var (
	BoolType = NewType("bool", "bool(x) -> bool\n\nReturns True when the argument x is true, False otherwise.\nThe builtins True and False are the only two instances of the class bool.\nThe class bool is a subclass of the class int, and cannot be subclassed.")
	// Some well known bools
	False = Bool(false)
	True  = Bool(true)
)

// Type of this object
func (s Bool) Type() *Type {
	return BoolType
}

// Make a new bool - returns the canonical True and False values
func NewBool(t bool) Bool {
	if t {
		return True
	}
	return False
}

func (a Bool) M__bool__() (Object, error) {
	return a, nil
}

func (a Bool) M__index__() (Int, error) {
	if a {
		return Int(1), nil
	}
	return Int(0), nil
}

func (a Bool) M__str__() (Object, error) {
	return a.M__repr__()
}

func (a Bool) M__repr__() (Object, error) {
	if a {
		return String("True"), nil
	}
	return String("False"), nil
}

// Convert an Object to an Bool
//
// Returns ok as to whether the conversion worked or not
func convertToBool(other Object) (Bool, bool) {
	switch b := other.(type) {
	case Bool:
		return b, true
	case Int:
		switch b {
		case 0:
			return False, true
		case 1:
			return True, true
		default:
			return False, false
		}
	case Float:
		switch b {
		case 0:
			return False, true
		case 1:
			return True, true
		default:
			return False, false
		}
	}
	return False, false
}

func (a Bool) M__eq__(other Object) (Object, error) {
	if b, ok := convertToBool(other); ok {
		return NewBool(a == b), nil
	}
	return False, nil
}

func (a Bool) M__ne__(other Object) (Object, error) {
	if b, ok := convertToBool(other); ok {
		return NewBool(a != b), nil
	}
	return True, nil
}

// Bool is a subclass of int, so it answers the integer operators too.  They
// delegate to Int, which is what makes "True & True" work: bitwise and,
// or and xor on two bools raised "unsupported operand type(s)" because Bool
// implemented none of them.  The result is an Int, as in CPython - "True & 1"
// is 1, not True.
func (a Bool) asInt() Int {
	if a {
		return Int(1)
	}
	return Int(0)
}

// int(True) is 1 and int(False) is 0.  Bool is a subclass of int, so MakeInt
// has to be able to convert it: it asks for __int__ first, and without this
// "int(True)" raised "unsupported operand type(s) for int: 'bool'".
func (a Bool) M__int__() (Object, error) { return a.asInt(), nil }

// Bool is a subclass of int, so it answers the ARITHMETIC operators too.
// True + True is 2 and True * 3 is 3, as CPython has it.  Only the bitwise
// family was implemented, so "True + True" raised "unsupported operand
// type(s) for +: 'bool' and 'bool'" - and bool values arrive in arithmetic
// constantly, from comparisons and from any()/all().
func (a Bool) M__add__(other Object) (Object, error)      { return a.asInt().M__add__(other) }
func (a Bool) M__radd__(other Object) (Object, error)     { return a.asInt().M__radd__(other) }
func (a Bool) M__sub__(other Object) (Object, error)      { return a.asInt().M__sub__(other) }
func (a Bool) M__rsub__(other Object) (Object, error)     { return a.asInt().M__rsub__(other) }
func (a Bool) M__mul__(other Object) (Object, error)      { return a.asInt().M__mul__(other) }
func (a Bool) M__rmul__(other Object) (Object, error)     { return a.asInt().M__rmul__(other) }
func (a Bool) M__floordiv__(other Object) (Object, error) { return a.asInt().M__floordiv__(other) }
func (a Bool) M__rfloordiv__(other Object) (Object, error) {
	return a.asInt().M__rfloordiv__(other)
}
func (a Bool) M__mod__(other Object) (Object, error)     { return a.asInt().M__mod__(other) }
func (a Bool) M__rmod__(other Object) (Object, error)    { return a.asInt().M__rmod__(other) }
func (a Bool) M__truediv__(other Object) (Object, error) { return a.asInt().M__truediv__(other) }
func (a Bool) M__rtruediv__(other Object) (Object, error) {
	return a.asInt().M__rtruediv__(other)
}
func (a Bool) M__neg__() (Object, error) { return a.asInt().M__neg__() }
func (a Bool) M__pos__() (Object, error) { return a.asInt().M__pos__() }
func (a Bool) M__abs__() (Object, error) { return a.asInt().M__abs__() }

func (a Bool) M__and__(other Object) (Object, error)  { return a.asInt().M__and__(other) }
func (a Bool) M__rand__(other Object) (Object, error) { return a.asInt().M__rand__(other) }
func (a Bool) M__iand__(other Object) (Object, error) { return a.asInt().M__iand__(other) }
func (a Bool) M__or__(other Object) (Object, error)   { return a.asInt().M__or__(other) }
func (a Bool) M__ror__(other Object) (Object, error)  { return a.asInt().M__ror__(other) }
func (a Bool) M__ior__(other Object) (Object, error)  { return a.asInt().M__ior__(other) }
func (a Bool) M__xor__(other Object) (Object, error)  { return a.asInt().M__xor__(other) }
func (a Bool) M__rxor__(other Object) (Object, error) { return a.asInt().M__rxor__(other) }
func (a Bool) M__ixor__(other Object) (Object, error) { return a.asInt().M__ixor__(other) }
func (a Bool) M__invert__() (Object, error)           { return a.asInt().M__invert__() }

func notEq(eq Object, err error) (Object, error) {
	if err != nil {
		return nil, err
	}
	if eq == NotImplemented {
		return eq, nil
	}
	return Not(eq)
}

// Check interface is satisfied
var _ I__bool__ = Bool(false)
var _ I__index__ = Bool(false)
var _ I__and__ = Bool(false)
var _ I__rand__ = Bool(false)
var _ I__iand__ = Bool(false)
var _ I__or__ = Bool(false)
var _ I__ror__ = Bool(false)
var _ I__ior__ = Bool(false)
var _ I__xor__ = Bool(false)
var _ I__rxor__ = Bool(false)
var _ I__ixor__ = Bool(false)
var _ I__int__ = Bool(false)
var _ I__str__ = Bool(false)
var _ I__repr__ = Bool(false)
var _ I__eq__ = Bool(false)
var _ I__ne__ = Bool(false)

func init() {
	// bool is a subclass of int in Python: issubclass(bool, int) is True and
	// True == 1, so bool must sit under int in the MRO.
	BoolType.Base = IntType

	// bool(x) asks its argument for the truth, the same rule "if x" uses.
	// Without this the type had no constructor at all and "bool(x)" failed
	// with "cannot create 'bool' instances".  There are only ever two bool
	// objects, so NewBool always returns one of them.
	BoolType.New = func(t *Type, args Tuple, kwargs StringDict) (Object, error) {
		var x Object = False
		if len(args) > 0 {
			x = args[0]
		}
		if len(args) > 1 {
			return nil, ExceptionNewf(TypeError, "bool expected at most 1 argument, got %d", len(args))
		}
		b, err := MakeBool(x)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
}
