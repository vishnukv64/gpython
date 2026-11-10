// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

// reversedTuple walks items from the end.
func reversedTuple(items []Object) Object {
	out := make(Tuple, len(items))
	for i, o := range items {
		out[len(items)-1-i] = o
	}
	return NewIterator(out)
}

// M__reversed__ iterates the keys from the most recently inserted (PEP 585,
// Python 3.8: dicts and their views are reversible).
func (d StringDict) M__reversed__() (Object, error) {
	it, err := d.M__iter__()
	if err != nil {
		return nil, err
	}
	keys, err := SequenceTuple(it)
	if err != nil {
		return nil, err
	}
	return reversedTuple(keys), nil
}

func (v *dictView) M__reversed__() (Object, error) {
	return reversedTuple(v.entries()), nil
}

// builtins.reversed finds __reversed__ in the type dictionary, not through
// the Go interface, so register both.
func init() {
	reg := func(t *Type) {
		t.Dict.Set("__reversed__", MustNewMethod("__reversed__", func(self Object, args Tuple) (Object, error) {
			// Called through the type, the instance is the first argument.
			if _, isType := self.(*Type); isType && len(args) > 0 {
				self = args[0]
			}
			if r, ok := self.(I__reversed__); ok {
				return r.M__reversed__()
			}
			// A dict subclass instance keeps its storage behind dictStorage.
			storage, err := dictStorage(self)
			if err != nil {
				return nil, err
			}
			return storage.M__reversed__()
		}, 0, "Return a reverse iterator."))
	}
	reg(dictKeysType)
	reg(dictValuesType)
	reg(dictItemsType)
	reg(StringDictType)
	reg(DictType)
}
