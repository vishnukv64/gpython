// Inline attribute caches for instances of Python classes.

package py

import (
	"sync/atomic"
)

// typeEpoch advances whenever a class namespace that an attribute cache has
// consulted changes, and whenever a class's MRO is (re)built.  An AttrCache is
// valid only for the epoch it was filled in, which is the per-type version
// check collapsed to one global counter: classes are almost never mutated
// once a program is running, and a stale answer is never possible.
var typeEpoch atomic.Uint64

// TypeEpoch returns the current class-mutation epoch.
func TypeEpoch() uint64 { return typeEpoch.Load() }

// Cache kinds: how "obj.name" resolves for instances of AttrCache.Cls.
const (
	AttrSlow   = iota // not cacheable: ask GetAttrString
	AttrAbsent        // not on the class: the instance dict or nothing
	AttrFunc          // a plain function on the class: bound unless the instance dict shadows it
	AttrData          // a Go data descriptor (property, slot): wins over the instance dict
	AttrPlain         // a non-descriptor class attribute, shadowed by the instance dict
)

// AttrCache is an immutable inline-cache entry for one attribute name.
type AttrCache struct {
	Cls   *Type
	Epoch uint64
	Kind  int
	Attr  Object
}

// isDataDescriptor reports whether o has __set__ or __delete__, so that it
// outranks the instance dict.
func isDataDescriptor(o Object) bool {
	switch o.(type) {
	case I__set__, I__delete__:
		return true
	}
	if t, ok := o.(*Type); ok && t.Name == "" && t.ObjectType != nil {
		return t.ObjectType.Lookup("__set__") != nil || t.ObjectType.Lookup("__delete__") != nil
	}
	return false
}

func isNonDataDescriptor(o Object) bool {
	if _, ok := o.(I__get__); ok {
		return true
	}
	if t, ok := o.(*Type); ok && t.Name == "" && t.ObjectType != nil {
		return t.ObjectType.Lookup("__get__") != nil
	}
	return false
}

// FillAttrCache classifies "ty.key" for the instance ty of a Python class.
func FillAttrCache(ty *Type, key string) *AttrCache {
	c := &AttrCache{Cls: ty.ObjectType, Kind: AttrSlow, Epoch: typeEpoch.Load()}
	cls := ty.ObjectType
	if ty.Name != "" || cls == nil || ty.Payload != nil || len(cls.Mro) == 0 ||
		(len(key) >= 2 && key[:2] == "__") || cls.derivesFromDict() {
		return c
	}
	for _, m := range cls.Mro {
		if mt, ok := m.(*Type); ok && !mt.Dict.IsNil() {
			mt.Dict.watch()
		}
	}
	c.Epoch = typeEpoch.Load()
	if cls.Lookup("__getattribute__") != Object(objectGetattribute) {
		return c
	}
	own := cls.Lookup(key)
	switch {
	case own == nil:
		c.Kind = AttrAbsent
	case isDataDescriptor(own):
		if _, ok := own.(I__get__); ok {
			if _, ok := own.(I__set__); ok {
				c.Kind, c.Attr = AttrData, own
			}
		}
	case isNonDataDescriptor(own):
		if _, ok := own.(*Function); ok {
			c.Kind, c.Attr = AttrFunc, own
		}
	default:
		c.Kind, c.Attr = AttrPlain, own
	}
	return c
}

// watch marks the dict as a class namespace whose mutations advance typeEpoch.
func (d StringDict) watch() {
	if d.shared != nil {
		atomic.StoreUint32(&d.shared.watched, 1)
	}
}

// isAttributeError is set at init: naming IsException directly from the
// attribute lookup would be an initialisation cycle.
var isAttributeError func(error) bool

func init() {
	isAttributeError = func(err error) bool { return IsException(AttributeError, err) }
}
