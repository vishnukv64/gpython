// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package collections

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const chainmap_doc = `A ChainMap groups multiple dicts (or other mappings) together
to create a single, updateable view.

The underlying mappings are stored in a list.  That list is public and can
be accessed or updated using the *maps* attribute.  Lookups search the
underlying mappings successively until a key is found.  In contrast,
writes, updates, and deletions only operate on the first mapping.`

// ChainMap searches a list of mappings in order.  It is a port of CPython's
// ChainMap: every operation goes through the mappings' own protocol, so any
// mapping (not just a dict) can be chained.
type ChainMap struct {
	maps *py.List
	// cls and attrs make a Python subclass work; see DefaultDict.
	cls   *py.Type
	attrs py.StringDict
}

var ChainMapType = py.NewTypeX("collections.ChainMap", chainmap_doc, chainMapNew, nil)

func (c *ChainMap) Type() *py.Type {
	if c.cls != nil {
		return c.cls
	}
	return ChainMapType
}

// GetDict is the instance __dict__ of a subclass instance.
func (c *ChainMap) GetDict() py.StringDict { return c.attrs }

func chainMapNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if kwargs.Len() > 0 {
		return nil, py.ExceptionNewf(py.TypeError, "ChainMap() takes no keyword arguments")
	}
	c := &ChainMap{maps: py.NewListFromItems(append([]py.Object(nil), args...))}
	if len(args) == 0 {
		c.maps.Items = []py.Object{py.NewStringDict()}
	}
	if metatype != nil && metatype.Name != "" && metatype.Name != "collections.ChainMap" {
		c.cls, c.attrs = metatype, py.NewStringDict()
	}
	return c, nil
}

// newLike builds a ChainMap of the receiver's class over maps.
func (c *ChainMap) newLike(maps ...py.Object) *ChainMap {
	return &ChainMap{maps: py.NewListFromItems(maps), cls: c.cls, attrs: attrsFor(c)}
}

func attrsFor(c *ChainMap) py.StringDict {
	if c.cls == nil {
		return py.StringDict{}
	}
	return py.NewStringDict()
}

func callMethod(obj py.Object, name string, args ...py.Object) (py.Object, error) {
	m, err := py.GetAttrString(obj, name)
	if err != nil {
		return nil, err
	}
	return py.Call(m, py.Tuple(args), py.StringDict{})
}

// keyError is KeyError(key): the argument is kept as given, so str() shows its repr.
func keyError(key py.Object) error {
	exc, _ := py.ExceptionNew(py.KeyError, py.Tuple{key}, py.StringDict{})
	return exc.(error)
}

func isKeyError(err error) bool { return err != nil && py.IsException(py.KeyError, err) }

func contains(m, key py.Object) (bool, error) {
	r, err := callMethod(m, "__contains__", key)
	if err != nil {
		return false, err
	}
	return py.ObjectIsTrue(r)
}

func (c *ChainMap) M__getitem__(key py.Object) (py.Object, error) {
	for _, m := range c.maps.Items {
		v, err := py.GetItem(m, key)
		if err == nil {
			return v, nil
		}
		if !isKeyError(err) {
			return nil, err
		}
	}
	// A subclass may define __missing__ (CPython's ChainMap calls it).
	if c.cls != nil {
		if fn := c.cls.LookupPython("__missing__"); fn != nil {
			return py.Call(py.NewBoundMethod(c, fn), py.Tuple{key}, py.StringDict{})
		}
	}
	return nil, keyError(key)
}

func (c *ChainMap) M__setitem__(key, value py.Object) (py.Object, error) {
	_, err := py.SetItem(c.maps.Items[0], key, value)
	return py.None, err
}

func (c *ChainMap) M__delitem__(key py.Object) (py.Object, error) {
	_, err := py.DelItem(c.maps.Items[0], key)
	if isKeyError(err) {
		return nil, py.ExceptionNewf(py.KeyError, "Key not found in the first mapping: %s", mustRepr(key))
	}
	return py.None, err
}

func (c *ChainMap) M__contains__(key py.Object) (py.Object, error) {
	for _, m := range c.maps.Items {
		ok, err := contains(m, key)
		if err != nil {
			return nil, err
		}
		if ok {
			return py.True, nil
		}
	}
	return py.False, nil
}

// order is the keys in CPython's iteration order: the last mapping's keys
// first, each key where it first appears.
func (c *ChainMap) order() ([]py.Object, error) {
	seen := py.NewStringDict()
	var keys []py.Object
	for i := len(c.maps.Items) - 1; i >= 0; i-- {
		var inner error
		err := py.Iterate(c.maps.Items[i], func(k py.Object) bool {
			enc, err := py.DictKey(k)
			if err != nil {
				inner = err
				return true
			}
			if _, dup := seen.Get(enc); !dup {
				seen.Set(enc, py.True)
				keys = append(keys, k)
			}
			return false
		})
		if err != nil {
			return nil, err
		}
		if inner != nil {
			return nil, inner
		}
	}
	return keys, nil
}

func (c *ChainMap) M__iter__() (py.Object, error) {
	keys, err := c.order()
	if err != nil {
		return nil, err
	}
	return newIteratorFromItems(keys), nil
}

func (c *ChainMap) M__len__() (py.Object, error) {
	keys, err := c.order()
	if err != nil {
		return nil, err
	}
	return py.Int(len(keys)), nil
}

func (c *ChainMap) M__bool__() (py.Object, error) {
	for _, m := range c.maps.Items {
		ok, err := py.ObjectIsTrue(m)
		if err != nil {
			return nil, err
		}
		if ok {
			return py.True, nil
		}
	}
	return py.False, nil
}

// snapshot is the merged mapping as a dict, in iteration order.
func (c *ChainMap) snapshot() (py.Object, error) {
	keys, err := c.order()
	if err != nil {
		return nil, err
	}
	d := py.NewStringDict()
	for _, k := range keys {
		v, err := c.M__getitem__(k)
		if err != nil {
			return nil, err
		}
		if _, err := py.SetItem(d, k, v); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func (c *ChainMap) M__repr__() (py.Object, error) {
	parts := make([]string, len(c.maps.Items))
	for i, m := range c.maps.Items {
		s, err := py.ReprAsString(m)
		if err != nil {
			return nil, err
		}
		parts[i] = s
	}
	return py.String(c.Type().Name[strings.LastIndex(c.Type().Name, ".")+1:] + "(" + strings.Join(parts, ", ") + ")"), nil
}

func (c *ChainMap) M__eq__(other py.Object) (py.Object, error) {
	if _, ok := other.(py.StringDict); !ok {
		if _, ok := other.(*ChainMap); !ok {
			return py.NotImplemented, nil
		}
	}
	a, err := c.snapshot()
	if err != nil {
		return nil, err
	}
	if o, ok := other.(*ChainMap); ok {
		if other, err = o.snapshot(); err != nil {
			return nil, err
		}
	}
	return py.Eq(a, other)
}

func (c *ChainMap) copyChain() (*ChainMap, error) {
	first, err := callMethod(c.maps.Items[0], "copy")
	if err != nil {
		return nil, err
	}
	return c.newLike(append([]py.Object{first}, c.maps.Items[1:]...)...), nil
}

func (c *ChainMap) M__or__(other py.Object) (py.Object, error) {
	if _, ok := other.(py.StringDict); !ok {
		if _, ok := other.(*ChainMap); !ok {
			return py.NotImplemented, nil
		}
	}
	m, err := c.copyChain()
	if err != nil {
		return nil, err
	}
	if _, err := callMethod(m.maps.Items[0], "update", other); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *ChainMap) M__ror__(other py.Object) (py.Object, error) {
	if _, ok := other.(py.StringDict); !ok {
		return py.NotImplemented, nil
	}
	m, err := callMethod(py.StringDictType, "__call__", other)
	if err != nil {
		return nil, err
	}
	for i := len(c.maps.Items) - 1; i >= 0; i-- {
		if _, err := callMethod(m, "update", c.maps.Items[i]); err != nil {
			return nil, err
		}
	}
	return c.newLike(m), nil
}

func (c *ChainMap) M__ior__(other py.Object) (py.Object, error) {
	if _, err := callMethod(c.maps.Items[0], "update", other); err != nil {
		return nil, err
	}
	return c, nil
}

func init() {
	d := ChainMapType.Dict
	method := func(name, doc string, fn func(c *ChainMap, args py.Tuple, kwargs py.StringDict) (py.Object, error)) {
		d.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			c, ok := self.(*ChainMap)
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "descriptor '%s' requires a 'collections.ChainMap' object but received a '%s'", name, self.Type().Name)
			}
			return fn(c, args, kwargs)
		}, 0, doc))
	}
	d.Set("maps", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*ChainMap).maps, nil },
		Fset: func(self, v py.Object) error {
			l, err := py.SequenceList(v)
			if err != nil {
				return err
			}
			self.(*ChainMap).maps = l
			return nil
		},
		Doc: "The list of mappings searched, first to last.",
	})
	d.Set("parents", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			c := self.(*ChainMap)
			return c.newLike(append([]py.Object(nil), c.maps.Items[1:]...)...), nil
		},
		Doc: "New ChainMap from maps[1:].",
	})
	method("__missing__", "Raise KeyError(key).", func(c *ChainMap, args py.Tuple, _ py.StringDict) (py.Object, error) {
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "__missing__() takes exactly one argument")
		}
		return nil, keyError(args[0])
	})
	method("get", "Return self[key] if key is in the chain, else default.", func(c *ChainMap, args py.Tuple, _ py.StringDict) (py.Object, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, py.ExceptionNewf(py.TypeError, "get expected 1 or 2 arguments, got %d", len(args))
		}
		in, err := c.M__contains__(args[0])
		if err != nil {
			return nil, err
		}
		if in == py.True {
			return c.M__getitem__(args[0])
		}
		if len(args) == 2 {
			return args[1], nil
		}
		return py.None, nil
	})
	method("keys", "Keys of the merged mapping.", func(c *ChainMap, _ py.Tuple, _ py.StringDict) (py.Object, error) {
		s, err := c.snapshot()
		if err != nil {
			return nil, err
		}
		return callMethod(s, "keys")
	})
	method("values", "Values of the merged mapping.", func(c *ChainMap, _ py.Tuple, _ py.StringDict) (py.Object, error) {
		s, err := c.snapshot()
		if err != nil {
			return nil, err
		}
		return callMethod(s, "values")
	})
	method("items", "Items of the merged mapping.", func(c *ChainMap, _ py.Tuple, _ py.StringDict) (py.Object, error) {
		s, err := c.snapshot()
		if err != nil {
			return nil, err
		}
		return callMethod(s, "items")
	})
	method("copy", "New ChainMap or subclass with a new copy of maps[0] and refs to maps[1:].", func(c *ChainMap, _ py.Tuple, _ py.StringDict) (py.Object, error) {
		return c.copyChain()
	})
	d.Set("__copy__", d.GetOrNil("copy"))
	method("new_child", "New ChainMap with a new map followed by all previous maps.", func(c *ChainMap, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) > 1 {
			return nil, py.ExceptionNewf(py.TypeError, "new_child() takes at most 1 positional argument")
		}
		var m py.Object = py.NewStringDict()
		if len(args) == 1 && args[0] != py.None {
			m = args[0]
		}
		if kwargs.Len() > 0 {
			if _, err := py.Call(mustAttr(m, "update"), py.Tuple{}, kwargs); err != nil {
				return nil, err
			}
		}
		return c.newLike(append([]py.Object{m}, c.maps.Items...)...), nil
	})
	method("popitem", "Remove and return an item pair from maps[0]. Raise KeyError is maps[0] is empty.", func(c *ChainMap, _ py.Tuple, _ py.StringDict) (py.Object, error) {
		r, err := callMethod(c.maps.Items[0], "popitem")
		if isKeyError(err) {
			return nil, py.ExceptionNewf(py.KeyError, "No keys found in the first mapping.")
		}
		return r, err
	})
	method("pop", "Remove *key* from maps[0] and return its value. Raise KeyError if *key* not in maps[0].", func(c *ChainMap, args py.Tuple, _ py.StringDict) (py.Object, error) {
		r, err := callMethod(c.maps.Items[0], "pop", args...)
		if isKeyError(err) {
			return nil, py.ExceptionNewf(py.KeyError, "Key not found in the first mapping: %s", mustRepr(args[0]))
		}
		return r, err
	})
	method("clear", "Clear maps[0], leaving maps[1:] intact.", func(c *ChainMap, _ py.Tuple, _ py.StringDict) (py.Object, error) {
		return callMethod(c.maps.Items[0], "clear")
	})
	method("setdefault", "Return self[key]; if key is missing, set it to default first.", func(c *ChainMap, args py.Tuple, _ py.StringDict) (py.Object, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, py.ExceptionNewf(py.TypeError, "setdefault expected 1 or 2 arguments, got %d", len(args))
		}
		in, err := c.M__contains__(args[0])
		if err != nil {
			return nil, err
		}
		if in == py.True {
			return c.M__getitem__(args[0])
		}
		var def py.Object = py.None
		if len(args) == 2 {
			def = args[1]
		}
		if _, err := c.M__setitem__(args[0], def); err != nil {
			return nil, err
		}
		return def, nil
	})
	method("update", "Update maps[0] from a mapping, an iterable of pairs and/or keywords.", func(c *ChainMap, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return py.Call(mustAttr(c.maps.Items[0], "update"), args, kwargs)
	})
	d.Set("fromkeys", py.MustNewMethod("fromkeys", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, py.ExceptionNewf(py.TypeError, "fromkeys expected 1 or 2 arguments, got %d", len(args))
		}
		m, err := callMethod(py.StringDictType, "fromkeys", args...)
		if err != nil {
			return nil, err
		}
		cls := ChainMapType
		if t, ok := self.(*py.Type); ok && t.IsSubtype(ChainMapType) {
			cls = t
		}
		return chainMapNew(cls, py.Tuple{m}, py.StringDict{})
	}, py.METH_CLASS, "Create a new ChainMap with keys from iterable and values set to value."))
}
