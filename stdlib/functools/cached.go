// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// functools.cached_property.

package functools

import (
	"github.com/vishnukv64/gpython/py"
)

const cached_property_doc = `functools.cached_property

Transform a method of a class into a property whose value is computed ONCE and
then cached as a normal attribute for the life of the instance.

The computation is deferred until the attribute is first read, and the result is
stored in the instance's __dict__ - which is what makes a second read find it
without calling the function again.  Deleting the attribute recomputes it, as in
CPython.

The name the decorator is bound to is supplied by __set_name__ when the class
body finishes, which is how the result is filed under the right key.`

// cachedProperty is the Go value behind a cached_property.
//
// It is a DESCRIPTOR: __get__ computes on demand and caches, and __set_name__
// records the attribute's name - which the class statement calls once, after the
// body has run.
type cachedProperty struct {
	fn   py.Object
	name string
	doc  string
}

// newCachedProperty is the constructor, so "@cached_property" produces one.
//
// The type is built with NewTypeX because NewType leaves New nil, and nil New
// means "cannot create instances" - the decorator could then never be applied.
func newCachedProperty(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := &cachedProperty{}
	if len(args) > 0 {
		c.fn = args[0]
	}
	if v, ok := kwargs.Get("doc"); ok {
		c.doc, _ = py.StrAsString(v)
	}
	return c, nil
}

var cachedPropertyType = py.NewTypeX("functools.cached_property",
	"A property whose value is computed once and cached on the instance.", newCachedProperty, nil)

func (c *cachedProperty) Type() *py.Type { return cachedPropertyType }

// M__get__ is the GO method the descriptor protocol calls.
//
// The interface I__get__ is what the attribute lookup consults on an instance,
// and it matches methods implemented in GO only.  Registering __get__ in the
// type's Dict made it callable by name - "d.__get__(obj)" worked - but the
// lookup never reached it, so "obj.v" returned the descriptor itself.
func (c *cachedProperty) M__get__(instance, owner py.Object) (py.Object, error) {
	// Read off the CLASS: the descriptor itself is what CPython returns.
	if instance == nil || instance == py.None {
		return c, nil
	}
	// The instance's own namespace is consulted first, which is what makes the
	// second read cheap and "del obj.attr" invalidate the cache.
	if dict, ok := instanceDict(instance); ok && c.name != "" {
		if v, found := dict.Get(c.name); found {
			return v, nil
		}
	}
	if c.fn == nil {
		return nil, py.ExceptionNewf(py.TypeError, "cached_property has no function")
	}
	val, err := py.Call(c.fn, py.Tuple{instance}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	if dict, ok := instanceDict(instance); ok && c.name != "" {
		dict.Set(c.name, val)
	}
	return val, nil
}

// instanceDict returns an instance's namespace, which is where the cache lives.
//
// A python-level instance is a *Type whose Dict IS its namespace, which is the
// same arrangement dict subclassing relies on.
func instanceDict(instance py.Object) (py.StringDict, bool) {
	if I, ok := instance.(py.IGetDict); ok {
		d := I.GetDict()
		// A nil underlying map cannot hold a cache entry.
		return d, d.Ptr() != 0
	}
	return py.StringDict{}, false
}

func init() {
	cachedPropertyType.Dict.Set("__init__", py.MustNewMethod("__init__", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		c, ok := self.(*cachedProperty)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a cached_property")
		}
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "cached_property() takes a function")
		}
		c.fn = args[0]
		if v, ok := kwargs.Get("doc"); ok {
			c.doc, _ = py.StrAsString(v)
		}
		return py.None, nil
	}, 0, "__init__(func, doc=None)"))

	// __set_name__ is called by the class statement once the body has run, and
	// it is how the property learns the attribute name it was bound to.
	cachedPropertyType.Dict.Set("__set_name__", py.MustNewMethod("__set_name__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c, ok := self.(*cachedProperty)
		if !ok || len(args) < 2 {
			return py.None, nil
		}
		name, err := py.StrAsString(args[1])
		if err != nil {
			return nil, err
		}
		c.name = name
		return py.None, nil
	}, 0, "Record the attribute name this property was bound to."))

	// A cached_property is NOT a data descriptor: without __set__ / __delete__
	// an instance attribute SHADOWS it, which is precisely what lets the cache
	// be an ordinary attribute.
	cachedPropertyType.Dict.Set("func", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if c, ok := self.(*cachedProperty); ok && c.fn != nil {
				return c.fn, nil
			}
			return py.None, nil
		},
		Doc: "The function this property wraps.",
	})
	cachedPropertyType.Dict.Set("attrname", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if c, ok := self.(*cachedProperty); ok {
				return py.String(c.name), nil
			}
			return py.None, nil
		},
		Doc: "The attribute name this property was bound to.",
	})
	cachedPropertyType.Dict.Set("__doc__", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if c, ok := self.(*cachedProperty); ok && c.doc != "" {
				return py.String(c.doc), nil
			}
			return py.String(cached_property_doc), nil
		},
		Doc: "The docstring, from the wrapped function unless overridden.",
	})
}
