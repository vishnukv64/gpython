// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import "strings"

// GenericAlias is types.GenericAlias, a parameterised class such as
// list[int]: CPython's Objects/genericaliasobject.c.  The standard library
// spells "this class is subscriptable" as
//
//	__class_getitem__ = classmethod(GenericAlias)
//
// (_weakrefset, and through it abc, numbers, decimal, difflib), so without it
// none of those import.
type GenericAlias struct {
	Origin Object
	Args   Tuple
}

var GenericAliasType = NewTypeX("types.GenericAlias",
	"Represent a PEP 585 generic type\n\nE.g. for t = list[int], t.__origin__ is list and t.__args__ is (int,).",
	genericAliasNew, nil)

func (g *GenericAlias) Type() *Type { return GenericAliasType }

func genericAliasNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	if kwargs.Len() != 0 {
		return nil, ExceptionNewf(TypeError, "GenericAlias() takes no keyword arguments")
	}
	if len(args) != 2 {
		return nil, ExceptionNewf(TypeError, "GenericAlias expected 2 arguments, got %d", len(args))
	}
	return NewGenericAlias(args[0], args[1]), nil
}

// NewGenericAlias builds origin[args]; a non-tuple args is a single argument,
// as in CPython.
func NewGenericAlias(origin, args Object) *GenericAlias {
	t, ok := args.(Tuple)
	if !ok {
		t = Tuple{args}
	}
	return &GenericAlias{Origin: origin, Args: t}
}

// typeRepr is CPython's _Py_typing_type_repr: a class by its (qualified)
// name, Ellipsis as "...", anything else by repr().
func typeRepr(o Object) (string, error) {
	if o == Ellipsis {
		return "...", nil
	}
	if t, ok := o.(*Type); ok && t.Name != "" {
		name := t.Name
		if q, ok := t.Dict.GetOrNil("__qualname__").(String); ok {
			name = string(q)
		}
		mod, _ := t.Dict.GetOrNil("__module__").(String)
		if mod == "" || mod == "builtins" || strings.Contains(name, ".") {
			return name, nil
		}
		return string(mod) + "." + name, nil
	}
	return ReprAsString(o)
}

func (g *GenericAlias) M__repr__() (Object, error) {
	var b strings.Builder
	r, err := typeRepr(g.Origin)
	if err != nil {
		return nil, err
	}
	b.WriteString(r)
	b.WriteByte('[')
	if len(g.Args) == 0 {
		b.WriteString("()")
	}
	for i, a := range g.Args {
		if i > 0 {
			b.WriteString(", ")
		}
		r, err := typeRepr(a)
		if err != nil {
			return nil, err
		}
		b.WriteString(r)
	}
	b.WriteByte(']')
	return String(b.String()), nil
}

// M__call__ constructs an instance of the origin: list[int]() is [].
func (g *GenericAlias) M__call__(args Tuple, kwargs StringDict) (Object, error) {
	return Call(g.Origin, args, kwargs)
}

// M__mro_entries__ makes "class C(list[int])" subclass list.
func (g *GenericAlias) M__mro_entries__(bases Object) (Object, error) {
	return Tuple{g.Origin}, nil
}

func (g *GenericAlias) M__eq__(other Object) (Object, error) {
	o, ok := other.(*GenericAlias)
	if !ok {
		return NotImplemented, nil
	}
	if eq, err := Eq(g.Origin, o.Origin); err != nil || eq != True {
		return eq, err
	}
	return Eq(g.Args, o.Args)
}

func (g *GenericAlias) M__ne__(other Object) (Object, error) {
	eq, err := g.M__eq__(other)
	if err != nil || eq == NotImplemented {
		return eq, err
	}
	return Not(eq)
}

func (g *GenericAlias) M__hash__() (Object, error) {
	ho, ok1 := HashValue(g.Origin)
	ha, ok2 := HashValue(g.Args)
	if !ok1 || !ok2 {
		return nil, ExceptionNewf(TypeError, "unhashable type: 'GenericAlias'")
	}
	return Int(ho ^ ha), nil
}

// TypeParam marks a type parameter (typing.TypeVar) that a subscript of a
// generic alias substitutes, as CPython detects by __typing_subst__.
type TypeParam interface {
	IsTypeParam()
}

// parameters is __parameters__: the distinct type parameters in the args, in
// first-seen order, looking inside nested aliases and Callable's arg lists.
func (g *GenericAlias) parameters() Tuple {
	var out Tuple
	var walk func(o Object)
	walk = func(o Object) {
		switch v := o.(type) {
		case TypeParam:
			for _, p := range out {
				if p == o {
					return
				}
			}
			out = append(out, o)
		case *GenericAlias:
			for _, a := range v.Args {
				walk(a)
			}
		case Tuple:
			for _, a := range v {
				walk(a)
			}
		case *List:
			for _, a := range v.Items {
				walk(a)
			}
		}
	}
	for _, a := range g.Args {
		walk(a)
	}
	if out == nil {
		return Tuple{}
	}
	return out
}

// M__getitem__ substitutes the alias's type parameters: dict[T, int][str] is
// dict[str, int].  ponytail: TypeVar only; ParamSpec/TypeVarTuple unpacking is not done.
func (g *GenericAlias) M__getitem__(key Object) (Object, error) {
	params := g.parameters()
	if len(params) == 0 {
		r, _ := g.M__repr__()
		return nil, ExceptionNewf(TypeError, "%s is not a generic class", r)
	}
	args, ok := key.(Tuple)
	if !ok {
		args = Tuple{key}
	}
	if len(args) != len(params) {
		r, _ := g.M__repr__()
		which := "many"
		if len(args) < len(params) {
			which = "few"
		}
		return nil, ExceptionNewf(TypeError, "Too %s arguments for %s; actual %d, expected %d", which, r, len(args), len(params))
	}
	var subst func(o Object) Object
	subst = func(o Object) Object {
		switch v := o.(type) {
		case TypeParam:
			for i, p := range params {
				if p == o {
					return args[i]
				}
			}
		case *GenericAlias:
			na := make(Tuple, len(v.Args))
			for i, a := range v.Args {
				na[i] = subst(a)
			}
			return &GenericAlias{Origin: v.Origin, Args: na}
		case Tuple:
			na := make(Tuple, len(v))
			for i, a := range v {
				na[i] = subst(a)
			}
			return na
		case *List:
			na := make([]Object, len(v.Items))
			for i, a := range v.Items {
				na[i] = subst(a)
			}
			return NewListFromItems(na)
		}
		return o
	}
	na := make(Tuple, len(g.Args))
	for i, a := range g.Args {
		na[i] = subst(a)
	}
	return &GenericAlias{Origin: g.Origin, Args: na}, nil
}

// M__getattr__ forwards every attribute the alias does not define to its
// origin, as CPython does for non-dunder names: list[int].append is
// list.append.
func (g *GenericAlias) M__getattr__(name string) (Object, error) {
	switch name {
	case "__origin__":
		return g.Origin, nil
	case "__args__":
		return g.Args, nil
	case "__parameters__":
		return g.parameters(), nil
	}
	if strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
		return nil, ExceptionNewf(AttributeError, "'types.GenericAlias' object has no attribute '%s'", name)
	}
	return GetAttrString(g.Origin, name)
}
