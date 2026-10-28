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
	"sort"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Support for enumerations.`

// EnumMember is one member of an enumeration.
type EnumMember struct {
	value py.Object
	name  string
	enum  *py.Type
	index int
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

// StrEnum members behave as strings: they compare and hash as their value and
// print as it.
var StrEnumType = py.NewTypeX("enum.StrEnum", "An enum whose members are also strs.", nil, nil)

// Flag and IntFlag are provided for importability; they behave as Enum here.
var FlagType = py.NewTypeX("enum.Flag", "An enum of bit flags.", nil, nil)

var IntFlagType = py.NewTypeX("enum.IntFlag", "An enum of bit flags that are ints.", nil, nil)

// auto() is a marker the builder replaces with the next integer.  It is a
// callable object so that both "auto" and "auto()" work at a class body.
type autoFunc struct{}

var autoType = py.NewTypeX("enum.auto", "Instances are replaced with an appropriate value in Enum class suites.", nil, nil)

func (a *autoFunc) Type() *py.Type { return autoType }

func (a *autoFunc) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 0 || kwargs.Len() != 0 {
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
	StrEnumType.Flags |= enumFlags
	StrEnumType.Base = py.StringType
	// IntEnum and IntFlag are also ints.
	IntEnumType.Base = py.IntType
	IntFlagType.Base = py.IntType

	// The members are attributes of the enum class, and the class itself
	// carries the usual introspection names.
	EnumMemberType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*EnumMember).name), nil },
	})
	EnumMemberType.Dict.Set("value", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*EnumMember).value, nil },
	})

	// The hash/eq/index protocol has to be reachable through the type's own
	// Dict, not merely as Go methods on *EnumMember.  GetAttrString finds a
	// Go M__hash__ by reflection, so "hash(member)" worked, but using a member
	// as a dict key or set element goes through Type.CallMethod, which walks
	// the MRO Dict - and there "__hash__" was absent, so
	// "{ControlType.BELL: 1}" raised "unhashable type: 'enum.member'".
	// rich/control.py builds exactly such a dict.
	EnumMemberType.Dict.Set("__hash__", py.MustNewMethod("__hash__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*EnumMember).M__hash__()
	}, 0, "Return the hash of a member."))
	EnumMemberType.Dict.Set("__eq__", py.MustNewMethod("__eq__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__eq__", 1, 1, &other); err != nil {
			return nil, err
		}
		return self.(*EnumMember).M__eq__(other)
	}, 0, "Return self==value."))
	EnumMemberType.Dict.Set("__ne__", py.MustNewMethod("__ne__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__ne__", 1, 1, &other); err != nil {
			return nil, err
		}
		return self.(*EnumMember).M__ne__(other)
	}, 0, "Return self!=value."))

	for _, t := range []*py.Type{EnumType, IntEnumType, FlagType, IntFlagType, StrEnumType} {
		// A METH_CLASS method read off the class is handed no self, so each
		// class gets a method that closes over itself.
		t := t
		t.Dict.Set("_convert_", py.MustNewMethod("_convert_", func(_ py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			return enumConvert(t, args, kwargs)
		}, py.METH_CLASS, "Create a new Enum subclass that replaces a collection of global constants."))
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

	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "Enum", Value: EnumType},
		py.DictEntry{Key: "IntEnum", Value: IntEnumType},
		py.DictEntry{Key: "StrEnum", Value: StrEnumType},
		py.DictEntry{Key: "_simple_enum", Value: py.MustNewMethod("_simple_enum", enumSimple, 0, "Class decorator that converts a normal class into an enumeration.")},
		py.DictEntry{Key: "Flag", Value: FlagType},
		py.DictEntry{Key: "IntFlag", Value: IntFlagType},
		py.DictEntry{Key: "EnumMeta", Value: EnumMetaType},
		py.DictEntry{Key: "auto", Value: &autoFunc{}},
		py.DictEntry{Key: "unique", Value: py.MustNewMethod("unique", enumUnique, 0, "Class decorator that ensures at most one name per value.")},
		py.DictEntry{Key: "verify", Value: py.MustNewMethod("verify", enumVerify, 0, "Class decorator that checks the given constraints.")},
		py.DictEntry{Key: "member", Value: py.MustNewMethod("member", enumMemberPassthrough, 0, "Mark an attribute as a member.")},
		py.DictEntry{Key: "nonmember", Value: py.MustNewMethod("nonmember", enumMemberPassthrough, 0, "Mark an attribute as not a member.")},
		py.DictEntry{Key: "global_enum", Value: py.MustNewMethod("global_enum", enumMemberPassthrough, 0, "Export the members to the module namespace.")},
		py.DictEntry{Key: "show_flag_values", Value: py.MustNewMethod("show_flag_values", enumMemberPassthrough, 0, "Return a list of all power-of-two integers.")},
	)
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "enum",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// enumConvert is Enum._convert_(name, module, filter, source=None, *, boundary=None, as_global=False).
func enumConvert(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var name, module, filter, source, boundary, asGlobal py.Object = nil, nil, nil, py.None, py.None, py.False
	err := py.ParseTupleAndKeywords(args, kwargs, "OOO|O$OO:_convert_", []string{"name", "module", "filter", "source", "boundary", "as_global"},
		&name, &module, &filter, &source, &boundary, &asGlobal)
	if err != nil {
		return nil, err
	}
	modName, ok := module.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "_convert_() argument 'module' must be str")
	}
	frame := py.CurrentFrame()
	if frame == nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "_convert_: no running frame")
	}
	mod, err := frame.Context.GetModule(string(modName))
	if err != nil {
		return nil, py.ExceptionNewf(py.KeyError, "%q", string(modName))
	}
	src := mod.Globals
	if source != py.None {
		d, ok := source.(py.StringDict)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "_convert_: source must be a dict")
		}
		src = d
	}
	var picked []string
	for _, k := range src.Keys() {
		r, err := py.Call(filter, py.Tuple{py.String(k)}, py.NewStringDict())
		if err != nil {
			return nil, err
		}
		if ok, err := py.ObjectIsTrue(r); err != nil {
			return nil, err
		} else if ok {
			picked = append(picked, k)
		}
	}
	// CPython orders converted members by (value, name).
	var sortErr error
	sort.SliceStable(picked, func(i, j int) bool {
		a := py.Tuple{src.GetOrNil(picked[i]), py.String(picked[i])}
		b := py.Tuple{src.GetOrNil(picked[j]), py.String(picked[j])}
		lt, err := py.Lt(a, b)
		if err != nil {
			sortErr = err
			return false
		}
		return lt == py.True
	})
	if sortErr != nil {
		return nil, sortErr
	}
	ns := py.NewStringDict()
	for _, k := range picked {
		ns.Set(k, src.GetOrNil(k))
	}
	cls, err := buildEnumClass(string(name.(py.String)), []py.Object{self}, ns)
	if err != nil {
		return nil, err
	}
	t := cls.(*py.Type)
	t.Dict.Set("__module__", modName)
	for _, k := range ns.Keys() {
		mod.Globals.Set(k, t.Dict.GetOrNil(k))
	}
	mod.Globals.Set(string(name.(py.String)), t)
	return t, nil
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
			} else if baseType == StrEnumType {
				cls.Base = py.StringType
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
		if _, ok := ns.GetOrNil(key).(*py.Function); ok {
			return true
		}
		switch ns.GetOrNil(key).(type) {
		case *py.Property, *py.StaticMethod, *py.ClassMethod:
			return true
		}
		return false
	}

	// A class body that defines __new__ and gives tuple values (pstats.SortKey)
	// means "(value, *aliases)": __new__ builds the member in CPython; here the
	// first item is the value and the others are extra names it answers to.
	_, hasNew := ns.GetOrNil("__new__").(*py.Function)
	members := []*EnumMember{}
	memberNames := []string{}
	byValue := map[string]*EnumMember{}
	autoValue := int64(1)

	// Members keep definition order: the class-body namespace is insertion-ordered.
	for _, key := range ns.Keys() {
		value := ns.GetOrNil(key)
		if isMeta(key) {
			if !(hasNew && key == "__new__") {
				cls.Dict.Set(key, value)
			}
			continue
		}
		var aliasValues py.Tuple
		if t, ok := value.(py.Tuple); ok && hasNew && len(t) > 0 {
			value, aliasValues = t[0], t[1:]
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

		// A duplicate value makes an alias, as Python does.  A value that
		// cannot be keyed - a plain object() - simply has no alias to be a
		// duplicate of, so it becomes its own member rather than raising
		// "unhashable type": that is what "X = object()" in an enum body is
		// for, and click's sentinel enum relies on it.
		encoded, err := py.DictKey(value)
		if err == nil {
			if existing, ok := byValue[encoded]; ok {
				cls.Dict.Set(key, existing)
				continue
			}
		} else {
			// Key it by name instead, so two distinct objects with the same
			// name cannot collide but the lookup below still works.
			encoded = "\x00name\x00" + name + "." + key
		}
		member := &EnumMember{value: value, name: key, enum: cls, index: len(members)}
		byValue[encoded] = member
		for _, a := range aliasValues {
			if k, err := py.DictKey(a); err == nil {
				byValue[k] = member
			}
		}
		members = append(members, member)
		memberNames = append(memberNames, key)
		cls.Dict.Set(key, member)
	}

	// __members__: the name-to-member mapping, aliases included.
	all := py.NewStringDict()
	for _, n := range memberNames {
		all.Set(n, cls.Dict.GetOrNil(n))
	}
	cls.Dict.Set("__members__", all)

	// The members cannot be stored in the class Dict: that holds py.Object
	// values, and these are Go values.  They live in a side table instead.
	enumDataFor[cls] = &enumData{members: members, byValue: byValue, names: memberNames, repr: cls.Dict.GetOrNil("__repr__")}
	installClassMethods(cls)
	return cls, nil
}

// enumData is what an enum class knows about itself beyond its attributes.
type enumData struct {
	members []*EnumMember
	byValue map[string]*EnumMember
	names   []string
	// repr is the __repr__ the class body defined, which the class-level
	// __repr__ installed by installClassMethods would otherwise replace.
	repr py.Object
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

// installClassMethods gives the enum class the behaviour Python code expects
// of it: iteration over the members, member lookup by name and by value, and
// the "in" test.
//
// The VM reaches these on the class object itself, so they are methods of the
// class's own Dict: that path binds correctly, unlike a method reached
// through a member instance.
func installClassMethods(cls *py.Type) {
	cls.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
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
	}, 0, "Iterate over the members, in definition order."))

	cls.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		if d := dataOf(self.(*py.Type)); d != nil {
			return py.Int(len(d.members)), nil
		}
		return py.Int(0), nil
	}, 0, "Number of members."))

	cls.Dict.Set("__contains__", py.MustNewMethod("__contains__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var item py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__contains__", 1, 1, &item); err != nil {
			return nil, err
		}
		m, ok := item.(*EnumMember)
		if ok && m.enum == self {
			return py.True, nil
		}
		return py.False, nil
	}, 0, "Whether the value is a member of this enum."))

	cls.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		t := self.(*py.Type)
		if name, ok := key.(py.String); ok {
			if m, ok := t.Dict.GetOrNil(string(name)).(*EnumMember); ok {
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
	}, 0, "Look a member up by name or by value."))

	cls.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String("<enum '" + self.(*py.Type).Name + "'>"), nil
	}, 0, "Return repr(self)."))

	// The class is called to look a member up by value: Color(1).
	cls.New = func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "Enum", 1, 1, &value); err != nil {
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

// M__getattr__ reaches what the class body defined (methods, properties,
// classmethods, staticmethods, plain attributes) through the member.
func (m *EnumMember) M__getattr__(name string) (py.Object, error) {
	if m.enum.Base == py.StringType && m.enum.Dict.GetOrNil(name) == nil && StrEnumType.Dict.GetOrNil(name) == nil {
		// str methods are the value's: Color.RED.upper(), .startswith(...)
		if v, err := py.GetAttrString(m.value, name); err == nil {
			return v, nil
		}
	}
	if v := m.enum.Lookup(name); v != nil {
		if g, ok := v.(py.I__get__); ok {
			return g.M__get__(m, m.enum)
		}
		return v, nil
	}
	return nil, py.ExceptionNewf(py.AttributeError, "'%s' object has no attribute '%s'", m.enum.Name, name)
}

// override calls a user-defined dunder from the class body, if there is one.
func (m *EnumMember) override(name string) (py.Object, bool, error) {
	var v py.Object
	if name == "__repr__" {
		if d := dataOf(m.enum); d != nil {
			v = d.repr
		}
	} else {
		v = m.enum.Lookup(name)
	}
	f, ok := v.(*py.Function)
	if !ok {
		return nil, false, nil
	}
	r, err := py.Call(f, py.Tuple{m}, py.NewStringDict())
	return r, true, err
}

// memberRepr is the "Color.RED" form Python prints.
func (m *EnumMember) M__repr__() (py.Object, error) {
	if r, ok, err := m.override("__repr__"); ok {
		return r, err
	}
	return py.String("<" + m.enum.Name + "." + m.name + ": " + debugStr(m.value) + ">"), nil
}

func (m *EnumMember) M__str__() (py.Object, error) {
	if r, ok, err := m.override("__str__"); ok {
		return r, err
	}
	if m.proxies() {
		// An IntEnum or StrEnum prints like the value it is, as in Python.
		return py.Str(m.value)
	}
	return py.String(m.enum.Name + "." + m.name), nil
}

func (m *EnumMember) M__eq__(other py.Object) (py.Object, error) {
	o, ok := other.(*EnumMember)
	if !ok {
		if m.proxies() {
			// An IntEnum or StrEnum compares equal to its value.
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
	if m.proxies() {
		if n, ok := py.HashValue(m.value); ok {
			return py.Int(n), nil
		}
	}
	return py.Int(int64(m.index) + 1), nil
}

// proxies reports whether the member stands in for its int or str value.
func (m *EnumMember) proxies() bool {
	return m.enum.Base == py.IntType || m.enum.Base == py.StringType
}

// DictKeyObject makes an IntEnum member share its int's dict entry.
func (m *EnumMember) DictKeyObject() py.Object {
	if m.proxies() {
		return m.value
	}
	return m
}

var _ py.IDictKeyProxy = (*EnumMember)(nil)

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
	_ py.I__getattr__ = (*EnumMember)(nil)
	_ py.I__repr__    = (*EnumMember)(nil)
	_ py.I__str__     = (*EnumMember)(nil)
	_ py.I__eq__      = (*EnumMember)(nil)
	_ py.I__ne__      = (*EnumMember)(nil)
	_ py.I__hash__    = (*EnumMember)(nil)
	_ py.I__index__   = (*EnumMember)(nil)
)

// enumSimple is enum._simple_enum(etype): a class decorator that turns a plain
// class into an enum of that type, as pstats.SortKey is written.
func enumSimple(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var etype py.Object = EnumType
	if len(args) > 0 {
		etype = args[0]
	}
	return py.MustNewMethod("_simple_enum_decorator", func(_ py.Object, a py.Tuple) (py.Object, error) {
		if len(a) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "_simple_enum decorator takes one class")
		}
		cls, ok := a[0].(*py.Type)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "_simple_enum decorates a class, not %s", a[0].Type().Name)
		}
		ns := py.NewStringDict()
		for _, k := range cls.Dict.Keys() {
			switch k {
			case "__dict__", "__weakref__":
				continue
			}
			ns.Set(k, cls.Dict.GetOrNil(k))
		}
		built, err := buildEnumClass(cls.Name, []py.Object{etype}, ns)
		if err != nil {
			return nil, err
		}
		return built, nil
	}, 0, ""), nil
}
