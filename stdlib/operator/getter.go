// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package operator

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// AttrGetterType is operator.attrgetter: the callable returned by
// attrgetter("a", "b.c") fetches those attributes from the object it is
// called with, and a name containing a '.' is a chain of nested lookups.
var AttrGetterType = py.NewTypeX("operator.attrgetter", "attrgetter(attr, ...) -> attrgetter object\n\nReturn a callable object that fetches the given attribute(s) from its operand.", attrGetterNew, nil)

// ItemGetterType is operator.itemgetter: the callable returned by
// itemgetter(1, "k") indexes its operand with each item in turn.
var ItemGetterType = py.NewTypeX("operator.itemgetter", "itemgetter(item, ...) -> itemgetter object\n\nReturn a callable object that fetches the given item(s) from its operand.", itemGetterNew, nil)

// MethodCallerType is operator.methodcaller: the callable returned by
// methodcaller("name", args...) calls that method, with those extra
// arguments, on the object it is called with.
var MethodCallerType = py.NewTypeX("operator.methodcaller", "methodcaller(name, ...) -> methodcaller object\n\nReturn a callable object that calls the given method on its operand.", methodCallerNew, nil)

// attrGetter is the object attrgetter() returns.
type attrGetter struct {
	// names is the chain of attributes for one argument, so that "a.b" is two
	// lookups rather than one attribute literally named "a.b".
	names [][]string
}

func attrGetterNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "attrgetter expected 1 argument, got 0")
	}
	g := &attrGetter{names: make([][]string, len(args))}
	for i, a := range args {
		name, err := py.StrAsString(a)
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "attribute name must be a string")
		}
		g.names[i] = strings.Split(name, ".")
	}
	return g, nil
}

func (g *attrGetter) Type() *py.Type { return AttrGetterType }

func (g *attrGetter) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var obj py.Object
	if err := py.UnpackTuple(args, kwargs, "attrgetter", 1, 1, &obj); err != nil {
		return nil, err
	}
	// A single name returns the value itself; several return a tuple of them.
	if len(g.names) == 1 {
		return g.lookup(obj, g.names[0])
	}
	items := make([]py.Object, len(g.names))
	for i, name := range g.names {
		v, err := g.lookup(obj, name)
		if err != nil {
			return nil, err
		}
		items[i] = v
	}
	return py.Tuple(items), nil
}

func (g *attrGetter) lookup(obj py.Object, path []string) (py.Object, error) {
	var err error
	for _, name := range path {
		obj, err = py.GetAttrString(obj, name)
		if err != nil {
			return nil, err
		}
	}
	return obj, nil
}

func (g *attrGetter) M__repr__() (py.Object, error) {
	parts := make([]string, len(g.names))
	for i, path := range g.names {
		parts[i] = reprShort(py.String(strings.Join(path, ".")))
	}
	return py.String("operator.attrgetter(" + strings.Join(parts, ", ") + ")"), nil
}

var _ py.I__call__ = (*attrGetter)(nil)
var _ py.I__repr__ = (*attrGetter)(nil)

// itemGetter is the object itemgetter() returns.
type itemGetter struct {
	items []py.Object
}

func itemGetterNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "itemgetter expected 1 argument, got 0")
	}
	return &itemGetter{items: append([]py.Object{}, args...)}, nil
}

func (g *itemGetter) Type() *py.Type { return ItemGetterType }

func (g *itemGetter) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var obj py.Object
	if err := py.UnpackTuple(args, kwargs, "itemgetter", 1, 1, &obj); err != nil {
		return nil, err
	}
	res, err := py.GetItem(obj, g.items[0])
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (g *itemGetter) M__repr__() (py.Object, error) {
	parts := make([]string, len(g.items))
	for i, item := range g.items {
		parts[i] = reprShort(item)
	}
	return py.String("operator.itemgetter(" + strings.Join(parts, ", ") + ")"), nil
}

var _ py.I__call__ = (*itemGetter)(nil)
var _ py.I__repr__ = (*itemGetter)(nil)

// methodCaller is the object methodcaller() returns.
type methodCaller struct {
	name string
	args py.Tuple
	kw   py.StringDict
}

func methodCallerNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "methodcaller needs at least one argument, the method name")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "method name must be a string")
	}
	return &methodCaller{name: name, args: append(py.Tuple{}, args[1:]...), kw: py.NewStringDict()}, nil
}

func (m *methodCaller) Type() *py.Type { return MethodCallerType }

// M__call__ looks the method up on the operand and calls it with the extra
// arguments the methodcaller was built with.
func (m *methodCaller) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var obj py.Object
	if err := py.UnpackTuple(args, kwargs, "methodcaller", 1, 1, &obj); err != nil {
		return nil, err
	}
	f, err := py.GetAttrString(obj, m.name)
	if err != nil {
		return nil, err
	}
	return py.Call(f, m.args, m.kw)
}

func (m *methodCaller) M__repr__() (py.Object, error) {
	parts := []string{m.name}
	for _, a := range m.args {
		parts = append(parts, reprShort(a))
	}
	return py.String("operator.methodcaller(" + strings.Join(parts, ", ") + ")"), nil
}

var _ py.I__call__ = (*methodCaller)(nil)
var _ py.I__repr__ = (*methodCaller)(nil)
