// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package enum

import "github.com/vishnukv64/gpython/py"

// An IntEnum member is an int: arithmetic, ordering and conversion act on its
// value, as in CPython (signal.SIGINT is used as -signal.SIGINT, 1 << flag).

// intValue returns the member's integer value, or ok=false for a plain Enum.
func (m *EnumMember) intValue() (py.Object, bool) {
	return m.value, m.enum.Base == py.IntType
}

// unwrap turns an IntEnum operand into its value.
func unwrap(o py.Object) py.Object {
	if e, ok := o.(*EnumMember); ok && e.enum.Base == py.IntType {
		return e.value
	}
	return o
}

func (m *EnumMember) unary(f func(py.Object) (py.Object, error)) (py.Object, error) {
	if v, ok := m.intValue(); ok {
		return f(v)
	}
	return py.NotImplemented, nil
}

func (m *EnumMember) binary(f func(a, b py.Object) (py.Object, error), o py.Object, swap bool) (py.Object, error) {
	v, ok := m.intValue()
	if !ok {
		return py.NotImplemented, nil
	}
	if swap {
		return f(unwrap(o), v)
	}
	return f(v, unwrap(o))
}

func (m *EnumMember) M__neg__() (py.Object, error)             { return m.unary(py.Neg) }
func (m *EnumMember) M__pos__() (py.Object, error)             { return m.unary(py.Pos) }
func (m *EnumMember) M__abs__() (py.Object, error)             { return m.unary(py.Abs) }
func (m *EnumMember) M__invert__() (py.Object, error)          { return m.unary(py.Invert) }
func (m *EnumMember) M__int__() (py.Object, error)             { return m.unary(py.MakeInt) }
func (m *EnumMember) M__float__() (py.Object, error)           { return m.unary(py.MakeFloat) }
func (m *EnumMember) M__lt__(o py.Object) (py.Object, error)   { return m.binary(py.Lt, o, false) }
func (m *EnumMember) M__le__(o py.Object) (py.Object, error)   { return m.binary(py.Le, o, false) }
func (m *EnumMember) M__gt__(o py.Object) (py.Object, error)   { return m.binary(py.Gt, o, false) }
func (m *EnumMember) M__ge__(o py.Object) (py.Object, error)   { return m.binary(py.Ge, o, false) }
func (m *EnumMember) M__add__(o py.Object) (py.Object, error)  { return m.binary(py.Add, o, false) }
func (m *EnumMember) M__radd__(o py.Object) (py.Object, error) { return m.binary(py.Add, o, true) }
func (m *EnumMember) M__sub__(o py.Object) (py.Object, error)  { return m.binary(py.Sub, o, false) }
func (m *EnumMember) M__rsub__(o py.Object) (py.Object, error) { return m.binary(py.Sub, o, true) }
func (m *EnumMember) M__mul__(o py.Object) (py.Object, error)  { return m.binary(py.Mul, o, false) }
func (m *EnumMember) M__rmul__(o py.Object) (py.Object, error) { return m.binary(py.Mul, o, true) }
func (m *EnumMember) M__truediv__(o py.Object) (py.Object, error) {
	return m.binary(py.TrueDiv, o, false)
}
func (m *EnumMember) M__rtruediv__(o py.Object) (py.Object, error) {
	return m.binary(py.TrueDiv, o, true)
}
func (m *EnumMember) M__floordiv__(o py.Object) (py.Object, error) {
	return m.binary(py.FloorDiv, o, false)
}
func (m *EnumMember) M__rfloordiv__(o py.Object) (py.Object, error) {
	return m.binary(py.FloorDiv, o, true)
}
func (m *EnumMember) M__mod__(o py.Object) (py.Object, error)  { return m.binary(py.Mod, o, false) }
func (m *EnumMember) M__rmod__(o py.Object) (py.Object, error) { return m.binary(py.Mod, o, true) }
func (m *EnumMember) M__lshift__(o py.Object) (py.Object, error) {
	return m.binary(py.Lshift, o, false)
}
func (m *EnumMember) M__rlshift__(o py.Object) (py.Object, error) {
	return m.binary(py.Lshift, o, true)
}
func (m *EnumMember) M__rshift__(o py.Object) (py.Object, error) {
	return m.binary(py.Rshift, o, false)
}
func (m *EnumMember) M__rrshift__(o py.Object) (py.Object, error) {
	return m.binary(py.Rshift, o, true)
}
func (m *EnumMember) M__and__(o py.Object) (py.Object, error)  { return m.binary(py.And, o, false) }
func (m *EnumMember) M__rand__(o py.Object) (py.Object, error) { return m.binary(py.And, o, true) }
func (m *EnumMember) M__xor__(o py.Object) (py.Object, error)  { return m.binary(py.Xor, o, false) }
func (m *EnumMember) M__rxor__(o py.Object) (py.Object, error) { return m.binary(py.Xor, o, true) }
func (m *EnumMember) M__or__(o py.Object) (py.Object, error)   { return m.binary(py.Or, o, false) }
func (m *EnumMember) M__ror__(o py.Object) (py.Object, error)  { return m.binary(py.Or, o, true) }

var (
	_ py.I__neg__       = (*EnumMember)(nil)
	_ py.I__pos__       = (*EnumMember)(nil)
	_ py.I__abs__       = (*EnumMember)(nil)
	_ py.I__invert__    = (*EnumMember)(nil)
	_ py.I__int__       = (*EnumMember)(nil)
	_ py.I__float__     = (*EnumMember)(nil)
	_ py.I__lt__        = (*EnumMember)(nil)
	_ py.I__le__        = (*EnumMember)(nil)
	_ py.I__gt__        = (*EnumMember)(nil)
	_ py.I__ge__        = (*EnumMember)(nil)
	_ py.I__add__       = (*EnumMember)(nil)
	_ py.I__sub__       = (*EnumMember)(nil)
	_ py.I__mul__       = (*EnumMember)(nil)
	_ py.I__truediv__   = (*EnumMember)(nil)
	_ py.I__floordiv__  = (*EnumMember)(nil)
	_ py.I__mod__       = (*EnumMember)(nil)
	_ py.I__lshift__    = (*EnumMember)(nil)
	_ py.I__rshift__    = (*EnumMember)(nil)
	_ py.I__and__       = (*EnumMember)(nil)
	_ py.I__xor__       = (*EnumMember)(nil)
	_ py.I__or__        = (*EnumMember)(nil)
	_ py.I__radd__      = (*EnumMember)(nil)
	_ py.I__rsub__      = (*EnumMember)(nil)
	_ py.I__rmul__      = (*EnumMember)(nil)
	_ py.I__rtruediv__  = (*EnumMember)(nil)
	_ py.I__rfloordiv__ = (*EnumMember)(nil)
	_ py.I__rmod__      = (*EnumMember)(nil)
	_ py.I__rlshift__   = (*EnumMember)(nil)
	_ py.I__rrshift__   = (*EnumMember)(nil)
	_ py.I__rand__      = (*EnumMember)(nil)
	_ py.I__rxor__      = (*EnumMember)(nil)
	_ py.I__ror__       = (*EnumMember)(nil)
)
