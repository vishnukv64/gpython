// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package enum provides the implementation of python's 'enum' module.
//
// Python builds an enum with a metaclass, and metaclasses are not supported
// here: "class Meta(type)" is refused with "type 'type' is not an acceptable
// base type", and __init_subclass__ is never called.  The class body is
// therefore handed to BuildEnumClass from __build_class__ instead, which is
// the one place that sees a class and its namespace before the class exists.
//
// The result is a class whose members are attributes, which is how Python
// code uses an enum: Color.RED, Color.RED.name, Color.RED.value, iteration,
// lookup by value and comparison of members.
package enum

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Support for enumerations.`

// EnumMember is one member of an enumeration.
type EnumMember struct {
	value  py.Object
	name   string
	enum   *py.Type
	index  int
	parent py.Object // the member this one aliases, when it is an alias
}

var EnumMemberType = py.NewTypeX("enum.member", "A single enum member.", nil, nil)

func (m *EnumMember) Type() *py.Type { return EnumMemberType }

// EnumMetaType is the type of an enum class itself.
var EnumMetaType = py.NewTypeX("enum.EnumMeta", "The type of an enum class.", nil, nil)

// EnumType is the base class that user enums derive from.  Its instances are
// enum classes, so its constructor is EnumMetaType.
var EnumType = py.NewTypeX("enum.Enum", "Create a collection of name/value pairs.", nil, nil)

// IntEnum members behave as integers, which is what click depends on.
var IntEnumType = py.NewTypeX("enum.IntEnum", "An enum whose members are also ints.", nil, nil)

// Flag and IntFlag are provided for importability; they behave as Enum here.
var FlagType = py.NewTypeX("enum.Flag", "An enum of bit flags.", nil, nil)

var IntFlagType = py.NewTypeX("enum.IntFlag", "An enum of bit flags that are ints.", nil, nil)

// auto() is a marker the builder replaces with the next integer.  It is a
// callable object so that both "auto" and "auto()" work at a class body.
type autoFunc struct{}

var autoType = py.NewTypeX("enum.auto", "Instances are replaced with an appropriate value in Enum class suites.", nil, nil)

func (a *autoFunc) Type() *py.Type { return autoType }

func (a *autoFunc) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 0 || len(kwargs) != 0 {
		return nil, py.ExceptionNewf(py.TypeError, "auto() takes no arguments")
	}
	return &autoFunc{}, nil
}

func (a *autoFunc) M__repr__() (py.Object, error) { return py.String("auto()"), nil }

var _ py.I__call__ = (*autoFunc)(nil)

// isAuto reports whether a value is an auto() marker.
func isAuto(value py.Object) bool {
	_, ok := value.(*autoFunc)
	return ok
}

// isIntEnumValue reports whether the enum members should also be integers,
// which is the case for IntEnum and IntFlag.
func isIntEnumValue(bases []py.Object) bool {
	for _, base := range bases {
		if base == IntEnumType || base == IntFlagType {
			return true
		}
	}
	return false
}

// enumFlags marks a class as an enum, which is how __build_class__ knows to
// hand its namespace to BuildEnumClass.
const enumFlags = py.TPFLAGS_ENUM

func init() {
	EnumType.Flags |= enumFlags
	IntEnumType.Flags |= enumFlags
	FlagType.Flags |= enumFlags
	IntFlagType.Flags |= enumFlags
	// IntEnum and IntFlag are also ints.
	IntEnumType.Base = py.IntType
	IntFlagType.Base = py.IntType

	// The members are attributes of the enum class, and the class itself
	// carries the usual introspection names.
	EnumMemberType.Dict["name"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*EnumMember).name), nil },
	}
	EnumMemberType.Dict["value"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*EnumMember).value, nil },
	}

	py.BuildEnumClass = buildEnumClass

	// An enum member is an instance of the enum class for isinstance, which
	// cannot walk from the member's own type to the class it belongs to.
	py.ABCHooks = append(py.ABCHooks, func(obj py.Object, class *py.Type) bool {
		if m, ok := obj.(*EnumMember); ok {
			return m.enum == class
		}
		return false
	})

	globals := py.StringDict{
		"Enum":             EnumType,
		"IntEnum":          IntEnumType,
		"Flag":             FlagType,
		"IntFlag":          IntFlagType,
		"EnumMeta":         EnumMetaType,
		"auto":             &autoFunc{},
		"unique":           py.MustNewMethod("unique", enumUnique, 0, "Class decorator that ensures at most one name per value."),
		"verify":           py.MustNewMethod("verify", enumVerify, 0, "Class decorator that checks the given constraints."),
		"member":           py.MustNewMethod("member", enumMemberPassthrough, 0, "Mark an attribute as a member."),
		"nonmember":        py.MustNewMethod("nonmember", enumMemberPassthrough, 0, "Mark an attribute as not a member."),
		"global_enum":      py.MustNewMethod("global_enum", enumMemberPassthrough, 0, "Export the members to the module namespace."),
		"show_flag_values": py.MustNewMethod("show_flag_values", enumMemberPassthrough, 0, "Return a list of all power-of-two integers."),
	}
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "enum",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

func enumUnique(fn py.Object) (py.Object, error) { return fn, nil }

func enumVerify(fn py.Object) (py.Object, error) { return fn, nil }

func enumMemberPassthrough(fn py.Object) (py.Object, error) { return fn, nil }

// buildEnumClass turns the namespace of a class deriving from Enum into an
// enum class.
//
// The namespace is the class body as executed: the members are the plain
// attributes, everything else (methods, docstring, dunder names) is copied
// onto the class.  A member whose value duplicates an earlier one becomes an
// alias for it, as in Python.
func buildEnumClass(name string, bases []py.Object, ns py.StringDict) (py.Object, error) {
	cls := py.NewType(name, "")
	cls.ObjectType = py.TypeType
	cls.Flags |= enumFlags
	for _, base := range bases {
		if baseType, ok := base.(*py.Type); ok {
			if baseType == IntEnumType || baseType == IntFlagType {
				cls.Base = py.IntType
			} else {
				cls.Base = baseType
			}
		}
	}

	asInt := isIntEnumValue(bases)

	// Names that are not members: dunders, methods, descriptors and the
	// names Python treats as documentation or annotations.
	isMeta := func(key string) bool {
		if strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__") {
			return true
		}
		if key == "_ignore_" || key == "_order_" {
			return true
		}
		if _, ok := ns[key].(*py.Function); ok {
			return true
		}
		switch ns[key].(type) {
		case *py.Property, *py.StaticMethod, *py.ClassMethod:
			return true
		}
		return false
	}

	members := []*EnumMember{}
	memberNames := []string{}
	byValue := map[string]*EnumMember{}
	autoValue := int64(1)

	// Members keep the order they were written in, so iterate the sorted
	// keys of a temporary list rather than map order... but the namespace is
	// a map with no order.  Member order therefore follows the order the
	// class body produced the names, which the compiler records for us in
	// the insertion-ordered string dict used for a class body; failing that,
	// sorted order is deterministic and at least stable.
	for _, key := range orderedNames(ns) {
		value := ns[key]
		if isMeta(key) {
			cls.Dict[key] = value
			continue
		}
		if isAuto(value) {
			value = py.Int(autoValue)
		}
		if asInt {
			if _, ok := value.(py.Int); !ok {
				// IntEnum members must be integers.
				n, err := py.IndexInt(value)
				if err != nil {
					return nil, py.ExceptionNewf(py.ValueError, "%s.%s is not an integer", name, key)
				}
				value = py.Int(n)
			}
		}
		if autoValue <= mustInt(value) {
			autoValue = mustInt(value) + 1
		}

		encoded, err := py.DictKey(value)
		if err != nil {
			return nil, err
		}
		if existing, ok := byValue[encoded]; ok {
			// A duplicate value makes an alias, as Python does.
			cls.Dict[key] = existing
			continue
		}
		member := &EnumMember{value: value, name: key, enum: cls, index: len(members)}
		byValue[encoded] = member
		members = append(members, member)
		memberNames = append(memberNames, key)
		cls.Dict[key] = member
	}

	// The members cannot be stored in the class Dict: that holds py.Object
	// values, and these are Go values.  They live in a side table instead.
	enumDataFor[cls] = &enumData{members: members, byValue: byValue, names: memberNames}
	installClassMethods(cls)
	return cls, nil
}

// enumData is what an enum class knows about itself beyond its attributes.
type enumData struct {
	members []*EnumMember
	byValue map[string]*EnumMember
	names   []string
}

// enumDataFor maps an enum class to its members.  A map keyed by the class
// is used rather than a field on Type, which is shared by every class.
var enumDataFor = map[*py.Type]*enumData{}

// dataOf returns the enum data for a class, if it is an enum.
func dataOf(t *py.Type) *enumData {
	return enumDataFor[t]
}

// mustInt returns the integer value of a member value, or 0 when it is not
// an integer, which only matters for picking the next auto() number.
func mustInt(value py.Object) int64 {
	if i, ok := value.(py.Int); ok {
		return int64(i)
	}
	return 0
}

// orderedNames returns the namespace keys, putting the ones that are members
// in their natural order.  A class body namespace is unordered here, so the
// member order is the sorted order of the names - deterministic, and enough
// for the names Python code compares against.
func orderedNames(ns py.StringDict) []string {
	names := make([]string, 0, len(ns))
	for key := range ns {
		names = append(names, key)
	}
	// Sort with dunders and private names first, then the members, so the
	// class attributes are set up before the members reference them.
	sortStrings(names)
	return names
}

func sortStrings(items []string) {
	// A small insertion sort: the namespaces are tiny, and this avoids
	// pulling in sort and its closure allocation for a handful of strings.
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j] < items[j-1]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// installClassMethods gives the enum class the behaviour Python code expects
// of it: iteration over the members, member lookup by name and by value, and
// the "in" test.
//
// The VM reaches these on the class object itself, so they are methods of the
// class's own Dict: that path binds correctly, unlike a method reached
// through a member instance.
func installClassMethods(cls *py.Type) {
	cls.Dict["__iter__"] = py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		t := self.(*py.Type)
		d := dataOf(t)
		if d == nil {
			return py.NewIterator(py.Tuple{}), nil
		}
		items := make(py.Tuple, len(d.members))
		for i, m := range d.members {
			items[i] = m
		}
		return py.NewIterator(items), nil
	}, 0, "Iterate over the members, in definition order.")

	cls.Dict["__len__"] = py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		if d := dataOf(self.(*py.Type)); d != nil {
			return py.Int(len(d.members)), nil
		}
		return py.Int(0), nil
	}, 0, "Number of members.")

	cls.Dict["__contains__"] = py.MustNewMethod("__contains__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var item py.Object
		if err := py.UnpackTuple(args, nil, "__contains__", 1, 1, &item); err != nil {
			return nil, err
		}
		m, ok := item.(*EnumMember)
		if ok && m.enum == self {
			return py.True, nil
		}
		return py.False, nil
	}, 0, "Whether the value is a member of this enum.")

	cls.Dict["__getitem__"] = py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key py.Object
		if err := py.UnpackTuple(args, nil, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		t := self.(*py.Type)
		if name, ok := key.(py.String); ok {
			if m, ok := t.Dict[string(name)].(*EnumMember); ok {
				return m, nil
			}
			return nil, py.ExceptionNewf(py.KeyError, "%v", key)
		}
		// A value looks up its member, which is Enum(2) in Python.
		encoded, err := py.DictKey(key)
		if err == nil {
			if d := dataOf(t); d != nil {
				if m, ok := d.byValue[encoded]; ok {
					return m, nil
				}
			}
		}
		return nil, py.ExceptionNewf(py.KeyError, "%v", key)
	}, 0, "Look a member up by name or by value.")

	cls.Dict["__repr__"] = py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String("<enum '" + self.(*py.Type).Name + "'>"), nil
	}, 0, "Return repr(self).")

	// The class is called to look a member up by value: Color(1).
	cls.New = func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var value py.Object
		if err := py.UnpackTuple(args, nil, "Enum", 1, 1, &value); err != nil {
			return nil, err
		}
		encoded, err := py.DictKey(value)
		if err != nil {
			return nil, err
		}
		if d := dataOf(cls); d != nil {
			if m, ok := d.byValue[encoded]; ok {
				return m, nil
			}
		}
		return nil, py.ExceptionNewf(py.ValueError, "%v is not a valid %s", value, cls.Name)
	}
}

// memberRepr is the "Color.RED" form Python prints.
func (m *EnumMember) M__repr__() (py.Object, error) {
	return py.String("<" + m.enum.Name + "." + m.name + ": " + debugStr(m.value) + ">"), nil
}

func (m *EnumMember) M__str__() (py.Object, error) {
	if m.enum.Base == py.IntType {
		// An IntEnum prints like the int it is, as in Python.
		return py.Str(m.value)
	}
	return py.String(m.enum.Name + "." + m.name), nil
}

func (m *EnumMember) M__eq__(other py.Object) (py.Object, error) {
	o, ok := other.(*EnumMember)
	if !ok {
		if m.enum.Base == py.IntType {
			// An IntEnum compares equal to its integer value.
			return py.Eq(m.value, other)
		}
		return py.False, nil
	}
	return py.NewBool(o == m), nil
}

func (m *EnumMember) M__ne__(other py.Object) (py.Object, error) {
	eq, err := m.M__eq__(other)
	if err != nil {
		return nil, err
	}
	return py.Not(eq)
}

func (m *EnumMember) M__hash__() (py.Object, error) {
	return py.Int(int64(m.index) + 1), nil
}

// M__index__ makes an IntEnum usable wherever an integer is expected.
func (m *EnumMember) M__index__() (py.Int, error) {
	n, err := py.IndexInt(m.value)
	if err != nil {
		return 0, py.ExceptionNewf(py.TypeError, "%s is not an integer", m.name)
	}
	return py.Int(n), nil
}

func debugStr(v py.Object) string {
	s, err := py.ReprAsString(v)
	if err != nil {
		return "?"
	}
	return s
}

var (
	_ py.I__repr__  = (*EnumMember)(nil)
	_ py.I__str__   = (*EnumMember)(nil)
	_ py.I__eq__    = (*EnumMember)(nil)
	_ py.I__ne__    = (*EnumMember)(nil)
	_ py.I__hash__  = (*EnumMember)(nil)
	_ py.I__index__ = (*EnumMember)(nil)
)
