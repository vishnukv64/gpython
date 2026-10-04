package py

import (
	"strings"
)

// The dict views.  dict.keys(), dict.values() and dict.items() return LIVE
// objects, not lists: they support len(), "in", and iteration, they reflect a
// mutation made after they were created, and a keys view behaves as a SET -
// "d.keys() & other", "d.keys() == d2.keys()".
//
// Returning an iterator instead - which is what this interpreter did - meant
// len() and "in" raised TypeError, and a program that asked a dict how many
// keys it had could not.
type dictView struct {
	d    StringDict
	kind dictViewKind
}

type dictViewKind int

const (
	viewKeys dictViewKind = iota
	viewValues
	viewItems
)

var (
	dictKeysType   = NewType("dict_keys", "A set-like view of a dictionary's keys.")
	dictValuesType = NewType("dict_values", "A view of a dictionary's values.")
	dictItemsType  = NewType("dict_items", "A view of a dictionary's items.")
)

func (v *dictView) Type() *Type {
	switch v.kind {
	case viewKeys:
		return dictKeysType
	case viewValues:
		return dictValuesType
	default:
		return dictItemsType
	}
}

// storageOf returns the string-keyed storage of any dict-shaped object, or a
// zero dict when there is none - so a view can be built without a second error
// path in every caller.
func storageOf(self Object) StringDict {
	storage, err := dictStorage(self)
	if err != nil {
		return StringDict{}
	}
	return storage
}

// entries reads the view's current contents.  Reading on every call is what
// makes the view LIVE.
func (v *dictView) entries() []Object {
	storage, err := dictStorage(v.d)
	if err != nil {
		return nil
	}
	keys := storage.Keys()
	out := make([]Object, 0, len(keys))
	for _, encoded := range keys {
		key, err := storage.DecodeKey(encoded)
		if err != nil {
			continue
		}
		switch v.kind {
		case viewKeys:
			out = append(out, key)
		case viewValues:
			if val, ok := storage.Get(encoded); ok {
				out = append(out, val)
			}
		default:
			val, _ := storage.Get(encoded)
			out = append(out, Tuple{key, val})
		}
	}
	return out
}

func (v *dictView) M__len__() (Object, error) {
	storage, err := dictStorage(v.d)
	if err != nil {
		return Int(0), nil
	}
	return Int(storage.Len()), nil
}

func (v *dictView) M__iter__() (Object, error) {
	return NewIterator(Tuple(v.entries())), nil
}

func (v *dictView) M__contains__(key Object) (Object, error) {
	switch v.kind {
	case viewKeys:
		if _, ok := v.lookup(key); ok {
			return True, nil
		}
		return False, nil
	case viewValues:
		for _, item := range v.entries() {
			eq, err := Eq(item, key)
			if err == nil && eq == True {
				return True, nil
			}
		}
		return False, nil
	default:
		pair, ok := key.(Tuple)
		if !ok || len(pair) != 2 {
			// A non-pair can never be an item, which CPython reports as False
			// rather than raising.
			return False, nil
		}
		if val, found := v.lookup(pair[0]); found {
			eq, err := Eq(val, pair[1])
			if err == nil && eq == True {
				return True, nil
			}
		}
		return False, nil
	}
}

// lookup reads one key out of the dict the view belongs to.
func (v *dictView) lookup(key Object) (Object, bool) {
	storage, err := dictStorage(v.d)
	if err != nil {
		return nil, false
	}
	var buf []byte
	if err := appendKey(&buf, key); err != nil {
		return nil, false
	}
	return storage.Get(string(buf))
}

func (v *dictView) M__repr__() (Object, error) {
	parts := make([]string, 0, 8)
	for _, item := range v.entries() {
		s, err := ReprAsString(item)
		if err != nil {
			return nil, err
		}
		parts = append(parts, s)
	}
	name := map[dictViewKind]string{
		viewKeys: "dict_keys", viewValues: "dict_values", viewItems: "dict_items",
	}[v.kind]
	return String(name + "([" + strings.Join(parts, ", ") + "])"), nil
}

// The set operations a KEYS view supports, which is what makes it set-like.
// The result is a real set, as in CPython.

func (v *dictView) asSet() (*Set, error) {
	s := NewSet()
	for _, item := range v.entries() {
		s.Add(item)
	}
	return s, nil
}

func (v *dictView) M__and__(other Object) (Object, error) {
	mine, err := v.asSet()
	if err != nil {
		return nil, err
	}
	return mine.M__and__(other)
}

func (v *dictView) M__or__(other Object) (Object, error) {
	mine, err := v.asSet()
	if err != nil {
		return nil, err
	}
	return mine.M__or__(other)
}

func (v *dictView) M__sub__(other Object) (Object, error) {
	mine, err := v.asSet()
	if err != nil {
		return nil, err
	}
	return mine.M__sub__(other)
}

func (v *dictView) M__xor__(other Object) (Object, error) {
	mine, err := v.asSet()
	if err != nil {
		return nil, err
	}
	return mine.M__xor__(other)
}

// The REFLECTED set operations, which is what makes "set & d.keys()" work:
// the left operand is a set that does not know about dict_keys, so the right
// operand's reflected method is what answers.  Without these,
// "d2.keys() & d.keys()" raised unsupported-operand even though the method on
// the view's own side existed.

func (v *dictView) M__rand__(other Object) (Object, error) { return v.M__and__(other) }
func (v *dictView) M__ror__(other Object) (Object, error)  { return v.M__or__(other) }
func (v *dictView) M__rxor__(other Object) (Object, error) { return v.M__xor__(other) }

// M__eq__ compares a keys view with another view or a set, by membership -
// which is what CPython does, and why "d.keys() == d2.keys()" is True for two
// dicts with the same keys in a different order.
func (v *dictView) M__eq__(other Object) (Object, error) {
	otherSet, ok := other.(interface{ M__eq__(Object) (Object, error) })
	_ = otherSet
	if !ok {
		return NotImplemented, nil
	}
	mine, err := v.asSet()
	if err != nil {
		return nil, err
	}
	switch o := other.(type) {
	case *dictView:
		if o.kind != viewKeys {
			return NotImplemented, nil
		}
		theirs, err := o.asSet()
		if err != nil {
			return nil, err
		}
		return mine.M__eq__(theirs)
	case *Set, *FrozenSet:
		return mine.M__eq__(o)
	}
	return NotImplemented, nil
}

// init registers the three view types.
//
// A view holds the dict it was made from, so a mutation after creation is
// visible - which is the whole difference between a view and a list.
func init() {
	for _, t := range []*Type{dictKeysType, dictValuesType, dictItemsType} {
		t.Dict.Set("__len__", MustNewMethod("__len__", func(self Object, args Tuple) (Object, error) {
			return self.(*dictView).M__len__()
		}, 0, "The number of entries, read live."))
		t.Dict.Set("__iter__", MustNewMethod("__iter__", func(self Object, args Tuple) (Object, error) {
			return self.(*dictView).M__iter__()
		}, 0, "Iterate the current entries."))
		t.Dict.Set("__repr__", MustNewMethod("__repr__", func(self Object, args Tuple) (Object, error) {
			return self.(*dictView).M__repr__()
		}, 0, "The repr, read live."))
	}

	dictKeysType.Dict.Set("__contains__", MustNewMethod("__contains__", func(self Object, args Tuple) (Object, error) {
		v := self.(*dictView)
		return v.M__contains__(args[0])
	}, 0, "Membership, read live."))
	for _, t := range []*Type{dictValuesType, dictItemsType} {
		t.Dict.Set("__contains__", MustNewMethod("__contains__", func(self Object, args Tuple) (Object, error) {
			v := self.(*dictView)
			return v.M__contains__(args[0])
		}, 0, "Membership, read live."))
	}

	// The set operations belong to the KEYS view, and only to it.
	for name, fn := range map[string]func(*dictView, Object) (Object, error){
		"__and__": (*dictView).M__and__,
		"__or__":  (*dictView).M__or__,
		"__sub__": (*dictView).M__sub__,
		"__xor__": (*dictView).M__xor__,
	} {
		name, fn := name, fn
		dictKeysType.Dict.Set(name, MustNewMethod(name, func(self Object, args Tuple) (Object, error) {
			return fn(self.(*dictView), args[0])
		}, 0, "A set operation on a keys view."))
	}
	for name, fn := range map[string]func(*dictView, Object) (Object, error){
		"__rand__": (*dictView).M__rand__,
		"__ror__":  (*dictView).M__ror__,
		"__rxor__": (*dictView).M__rxor__,
	} {
		name, fn := name, fn
		dictKeysType.Dict.Set(name, MustNewMethod(name, func(self Object, args Tuple) (Object, error) {
			return fn(self.(*dictView), args[0])
		}, 0, "A reflected set operation on a keys view."))
	}
	dictKeysType.Dict.Set("__eq__", MustNewMethod("__eq__", func(self Object, args Tuple) (Object, error) {
		return self.(*dictView).M__eq__(args[0])
	}, 0, "Set-like equality."))
	// A keys view is hashable in CPython only when its contents are; a frozenset
	// of the keys is the honest answer to "can it be used as a key".
	dictKeysType.Dict.Set("__hash__", MustNewMethod("__hash__", func(self Object, args Tuple) (Object, error) {
		return nil, ExceptionNewf(TypeError, "unhashable type: 'dict_keys'")
	}, 0, "A keys view is unhashable."))
}
