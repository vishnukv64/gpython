// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Property object

package py

// A python Property object
type Property struct {
	Fget func(self Object) (Object, error)
	Fset func(self, value Object) error
	Fdel func(self Object) error
	Doc  string
}

var PropertyType = NewTypeX("property", `property(fget=None, fset=None, fdel=None, doc=None)

Property attribute.

fget is a function to be used for getting an attribute value, and
likewise fset is a function for setting an attribute value.
fdel is a function of one argument for deleting the attribute.
If doc is given, it is used as the attribute's docstring; otherwise
the docstring of fget is used.`, PropertyNew, nil)

// PropertyNew implements property(fget=None, fset=None, fdel=None, doc=None).
// The @property decorator form is the one-argument call property(fget).
func PropertyNew(metatype *Type, args Tuple, kwargs StringDict) (res Object, err error) {
	var (
		fget, fset, fdel, doc Object
	)
	err = ParseTupleAndKeywords(args, kwargs, "O|OOO", []string{"fget", "fset", "fdel", "doc"}, &fget, &fset, &fdel, &doc)
	if err != nil {
		return nil, err
	}

	p := &Property{}

	if fget != nil && fget != None {
		fn := fget
		p.Fget = func(self Object) (Object, error) {
			return Call(fn, Tuple{self}, nil)
		}
	}
	if fset != nil && fset != None {
		fn := fset
		p.Fset = func(self, value Object) error {
			_, err := Call(fn, Tuple{self, value}, nil)
			return err
		}
	}
	if fdel != nil && fdel != None {
		fn := fdel
		p.Fdel = func(self Object) error {
			_, err := Call(fn, Tuple{self}, nil)
			return err
		}
	}
	if docStr, ok := doc.(String); ok {
		p.Doc = string(docStr)
	}

	return p, nil
}

// Type of this object
func (o *Property) Type() *Type {
	return PropertyType
}

func (p *Property) M__get__(instance, owner Object) (Object, error) {
	// Reading a property on the class itself yields the property object;
	// the getter is only invoked for an instance.
	if instance == nil || instance == None {
		return p, nil
	}
	if p.Fget == nil {
		return nil, ExceptionNewf(AttributeError, "can't get attribute")
	}
	return p.Fget(instance)
}

func (p *Property) M__set__(instance, value Object) (Object, error) {
	if p.Fset == nil {
		return nil, ExceptionNewf(AttributeError, "can't set attribute")
	}
	return None, p.Fset(instance, value)
}

func (p *Property) M__delete__(instance Object) (Object, error) {
	if p.Fdel == nil {
		return nil, ExceptionNewf(AttributeError, "can't delete attribute")
	}
	return None, p.Fdel(instance)
}

// Properties
func init() {
	PropertyType.Dict["__doc__"] = &Property{
		Fget: func(self Object) (Object, error) {
			return String(self.(*Property).Doc), nil
		},
	}
}

// Interfaces
var _ I__get__ = (*Property)(nil)
var _ I__set__ = (*Property)(nil)
var _ I__delete__ = (*Property)(nil)
