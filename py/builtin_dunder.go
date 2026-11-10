package py

// Python code reads the operator slots off a builtin class - list.__add__,
// str.__mod__, int.__lt__ - as method descriptors.  The Go types implement
// the operators as methods that are not entries of the class's namespace, so
// the lookup finds nothing.  builtinDunder answers it with a method that
// applies the operator, for the builtin types whose instances support it.

type dunderBinary func(a, b Object) (Object, error)

// binaryDunder is the operator behind __name__ (without the underscores).
// A function rather than a table: the operators reach the type machinery, so
// a package-level map of them is an initialization cycle.
func binaryDunder(name string) dunderBinary {
	switch name {
	case "add":
		return Add
	case "sub":
		return Sub
	case "mul":
		return Mul
	case "truediv":
		return TrueDiv
	case "floordiv":
		return FloorDiv
	case "mod":
		return Mod
	case "lshift":
		return Lshift
	case "rshift":
		return Rshift
	case "and":
		return And
	case "or":
		return Or
	case "xor":
		return Xor
	case "lt":
		return Lt
	case "le":
		return Le
	case "gt":
		return Gt
	case "ge":
		return Ge
	}
	return nil
}

func unaryDunder(name string) func(Object) (Object, error) {
	switch name {
	case "neg":
		return Neg
	case "pos":
		return Pos
	case "abs":
		return Abs
	case "invert":
		return Invert
	}
	return nil
}

// dunderKinds says which builtin types get which dunders (by name sans __).
func dunderSupported(t *Type, name string) bool {
	numeric := t == IntType || t == FloatType || t == ComplexType || t == BoolType
	sequence := t == ListType || t == TupleType || t == StringType || t == BytesType || t == ByteArrayType
	set := t == SetType || t == FrozenSetType
	switch name {
	case "add", "mul", "getitem", "contains", "len", "iter":
		if name == "add" && numeric {
			return true
		}
		if name == "mul" && numeric {
			return true
		}
		if name == "add" && set {
			return false
		}
		if name == "mul" && set {
			return false
		}
		return sequence || (set && (name == "contains" || name == "len" || name == "iter")) || (t == RangeType && name != "add" && name != "mul")
	case "sub", "truediv", "floordiv", "pow", "neg", "pos", "abs":
		if set {
			return name == "sub"
		}
		return numeric && (name != "truediv" || true)
	case "mod":
		return numeric || t == StringType || t == BytesType
	case "lshift", "rshift", "invert":
		return t == IntType || t == BoolType
	case "and", "or", "xor":
		return t == IntType || t == BoolType || set
	case "lt", "le", "gt", "ge":
		return (numeric && t != ComplexType) || sequence || set
	}
	return false
}

func init() {
	builtinDunderHook = builtinDunder
	builtinDictViewHook = builtinDictView
}

// builtinDunder returns t.<key> for a dunder the builtin type t supports but
// does not list in its dict, or nil.
func builtinDunder(t *Type, key string) Object {
	if len(key) < 5 || key[:2] != "__" || key[len(key)-2:] != "__" || t.Name == "" {
		return nil
	}
	name := key[2 : len(key)-2]
	if name == "dir" && (t == ObjectType || t == TypeType) {
		m, err := NewMethod(key, func(self Object, args Tuple) (Object, error) {
			return DirHook(self)
		}, 0, "Default dir() implementation.")
		if err != nil {
			return nil
		}
		m.Unbound = true
		return m
	}
	rev := false
	if len(name) > 1 && name[0] == 'r' {
		if binaryDunder(name[1:]) != nil {
			name, rev = name[1:], true
		}
	}
	if !dunderSupported(t, name) {
		return nil
	}
	m, err := NewMethod(key, func(self Object, args Tuple) (Object, error) {
		return applyDunder(name, rev, self, args)
	}, 0, "Operator slot of "+t.Name+".")
	if err != nil {
		return nil
	}
	m.Unbound = true
	return m
}

func applyDunder(name string, rev bool, self Object, args Tuple) (Object, error) {
	if f := binaryDunder(name); f != nil {
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "expected 1 argument, got %d", len(args))
		}
		if rev {
			return f(args[0], self)
		}
		return f(self, args[0])
	}
	if f := unaryDunder(name); f != nil {
		return f(self)
	}
	switch name {
	case "getitem":
		return GetItem(self, args[0])
	case "len":
		return Len(self)
	case "iter":
		return Iter(self)
	case "pow":
		var mod Object = None
		if len(args) > 1 {
			mod = args[1]
		}
		return Pow(self, args[0], mod)
	case "contains":
		it, err := Iter(self)
		if err != nil {
			return nil, err
		}
		for {
			v, err := Next(it)
			if err != nil {
				if IsException(StopIteration, err) {
					return False, nil
				}
				return nil, err
			}
			if eq, err := Eq(v, args[0]); err == nil && eq == True {
				return True, nil
			}
		}
	}
	return nil, ExceptionNewf(TypeError, "unsupported operator")
}

// dunderNames are the operator slots builtinDunder can supply.
var dunderNames = []string{"add", "radd", "sub", "rsub", "mul", "rmul", "truediv", "rtruediv", "floordiv", "rfloordiv",
	"mod", "rmod", "pow", "lshift", "rlshift", "rshift", "rrshift", "and", "rand", "or", "ror", "xor", "rxor",
	"lt", "le", "gt", "ge", "neg", "pos", "abs", "invert", "getitem", "contains", "len", "iter"}

// builtinDictView is t.__dict__ for a builtin class: its namespace plus the
// operator slots it supports but keeps in Go, as a copy (a builtin's
// namespace is not edited).  ok is false when nothing is added.
func builtinDictView(t *Type) (StringDict, bool) {
	var view StringDict
	for _, n := range dunderNames {
		key := "__" + n + "__"
		if t.Dict.Has(key) {
			continue
		}
		if m := builtinDunder(t, key); m != nil {
			if view.IsNil() {
				view = t.Dict.Copy()
			}
			view.Set(key, m)
		}
	}
	return view, !view.IsNil()
}

// DirHook is dir(obj), installed by the vm package that implements it.
var DirHook func(Object) (Object, error)
