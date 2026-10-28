// __slots__: slot descriptors and the no-__dict__ rule.
//
// An instance of a Python class is a *Type whose Dict is its namespace.  A
// slot's value is kept apart from it, in Type.slotVals, so it never shows up
// in __dict__ of a subclass that has one.  What makes a slotted class
// different from an ordinary one is that it has no __dict__: assigning an
// undeclared name is an AttributeError, and the instance dict is not
// reachable as an attribute.

package py

import (
	"sort"
	"strings"
)

var MemberDescriptorType = NewType("member_descriptor", "Descriptor for a __slots__ entry.")

type slotDescriptor struct {
	name  string // the (mangled) attribute name
	owner *Type
}

func (*slotDescriptor) Type() *Type { return MemberDescriptorType }

func (s *slotDescriptor) instance(o Object) (*Type, error) {
	if t, ok := o.(*Type); ok && t.Name == "" {
		return t, nil
	}
	return nil, ExceptionNewf(TypeError, "descriptor '%s' for '%s' objects doesn't apply to a '%s' object",
		s.name, s.owner.shortName(), o.Type().Name)
}

func (s *slotDescriptor) M__get__(instance, owner Object) (Object, error) {
	if instance == nil || instance == None {
		return s, nil
	}
	t, err := s.instance(instance)
	if err != nil {
		return nil, err
	}
	if v, ok := t.slotVals[s.name]; ok {
		return v, nil
	}
	return nil, ExceptionNewf(AttributeError, "'%s' object has no attribute '%s'", t.ObjectType.shortName(), s.name)
}

func (s *slotDescriptor) M__set__(instance, value Object) (Object, error) {
	t, err := s.instance(instance)
	if err != nil {
		return nil, err
	}
	if t.slotVals == nil {
		t.slotVals = map[string]Object{}
	}
	t.slotVals[s.name] = value
	return None, nil
}

func (s *slotDescriptor) M__delete__(instance Object) (Object, error) {
	t, err := s.instance(instance)
	if err != nil {
		return nil, err
	}
	if _, ok := t.slotVals[s.name]; !ok {
		return nil, ExceptionNewf(AttributeError, "%s", s.name)
	}
	delete(t.slotVals, s.name)
	return None, nil
}

func (s *slotDescriptor) M__repr__() (Object, error) {
	return String("<member '" + s.name + "' of '" + s.owner.shortName() + "' objects>"), nil
}

var (
	_ I__get__    = (*slotDescriptor)(nil)
	_ I__set__    = (*slotDescriptor)(nil)
	_ I__delete__ = (*slotDescriptor)(nil)
	_ I__repr__   = (*slotDescriptor)(nil)
)

func (t *Type) shortName() string {
	if i := strings.LastIndexByte(t.Name, '.'); i > 0 {
		return t.Name[i+1:]
	}
	return t.Name
}

// mangle is _Py_Mangle for a __slots__ entry: __x in class C becomes _C__x.
func mangle(class, name string) string {
	if !strings.HasPrefix(name, "__") || strings.HasSuffix(name, "__") || strings.Contains(name, ".") {
		return name
	}
	class = strings.TrimLeft(class, "_")
	if class == "" {
		return name
	}
	return "_" + class + name
}

func validSlotName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		letter := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127
		if !(letter || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// setupSlots interprets a class body's __slots__: it adds the slot
// descriptors to dict and reports whether instances have no __dict__.
func setupSlots(className string, bases Tuple, slots Object, dict StringDict) (noDict bool, err error) {
	var names []string
	if s, ok := slots.(String); ok {
		names = []string{string(s)}
	} else {
		var bad error
		if err = Iterate(slots, func(item Object) bool {
			s, ok := item.(String)
			if !ok {
				bad = ExceptionNewf(TypeError, "__slots__ items must be strings, not '%s'", item.Type().Name)
				return true
			}
			names = append(names, string(s))
			return false
		}); err != nil {
			return false, err
		}
		if bad != nil {
			return false, bad
		}
	}

	baseHasDict := false
	for _, b := range bases {
		if bt, ok := b.(*Type); ok && bt != ObjectType && !bt.NoDict {
			baseHasDict = true
		}
	}
	addDict, addWeak := false, false
	var slotNames []string
	for _, n := range names {
		if !validSlotName(n) {
			return false, ExceptionNewf(TypeError, "__slots__ must be identifiers")
		}
		switch n {
		case "__dict__":
			if baseHasDict || addDict {
				return false, ExceptionNewf(TypeError, "__dict__ slot disallowed: we already got one")
			}
			addDict = true
		case "__weakref__":
			if addWeak {
				return false, ExceptionNewf(TypeError, "__weakref__ slot disallowed: either we already got one, or __itemsize__ != 0")
			}
			addWeak = true // every object here can be weakly referenced
		default:
			slotNames = append(slotNames, mangle(className, n))
		}
	}
	sort.Strings(slotNames)
	owner := &Type{Name: className}
	for _, n := range slotNames {
		if _, clash := dict.Get(n); clash {
			return false, ExceptionNewf(ValueError, "'%s' in __slots__ conflicts with class variable", n)
		}
		dict.Set(n, &slotDescriptor{name: n, owner: owner})
	}
	return !baseHasDict && !addDict, nil
}
