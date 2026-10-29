package typing

import (
	"github.com/vishnukv64/gpython/py"
)

// Type parameters: TypeVar, TypeVarTuple, ParamSpec and TypeAliasType (PEP
// 695, 696).  The compiler lowers "def f[T]", "class C[T]" and "type X = ..."
// to calls of the _gpy_* functions below (ast/typeparams.go); the classes
// themselves are also what typing.TypeVar(...) and friends construct.

// lazy is a value computed on first use by calling a thunk: the bound,
// constraints and default of a type parameter, and the value of an alias.
type lazy struct {
	thunk py.Object
	val   py.Object
	done  bool
}

func (l *lazy) get() (py.Object, error) {
	if !l.done {
		v, err := py.Call(l.thunk, py.Tuple{}, py.StringDict{})
		if err != nil {
			return nil, err
		}
		l.val, l.done = v, true
	}
	return l.val, nil
}

// noDefault is typing.NoDefault.
var noDefault = form("NoDefault")

// eagerLazy wraps a value that exists already.
func eagerLazy(v py.Object) *lazy { return &lazy{val: v, done: true} }

// typeParam is the state the three parameter kinds share.
type typeParam struct {
	name        string
	dflt        *lazy // nil: no default
	covariant   bool
	contravar   bool
	inferVar    bool
	bound       *lazy // TypeVar only
	constraints *lazy // TypeVar only
}

var (
	TypeVarType      = py.NewTypeX("typing.TypeVar", "Type variable.", typeVarNew, nil)
	TypeVarTupleType = py.NewTypeX("typing.TypeVarTuple", "Type variable tuple.", typeVarTupleNew, nil)
	ParamSpecType    = py.NewTypeX("typing.ParamSpec", "Parameter specification variable.", paramSpecNew, nil)
	paramSpecArgs    = py.NewTypeX("typing.ParamSpecArgs", "The args for a ParamSpec object.", nil, nil)
	paramSpecKwargs  = py.NewTypeX("typing.ParamSpecKwargs", "The kwargs for a ParamSpec object.", nil, nil)
)

// TypeVarObj is a TypeVar.
type TypeVarObj struct{ typeParam }

// TypeVarTupleObj is a TypeVarTuple.
type TypeVarTupleObj struct{ typeParam }

// ParamSpecObj is a ParamSpec.
type ParamSpecObj struct{ typeParam }

func (t *TypeVarObj) Type() *py.Type      { return TypeVarType }
func (t *TypeVarTupleObj) Type() *py.Type { return TypeVarTupleType }
func (t *ParamSpecObj) Type() *py.Type    { return ParamSpecType }

func variancePrefix(p *typeParam) string {
	switch {
	case p.inferVar:
		return ""
	case p.covariant:
		return "+"
	case p.contravar:
		return "-"
	}
	return "~"
}

func (t *TypeVarObj) M__repr__() (py.Object, error) {
	return py.String(variancePrefix(&t.typeParam) + t.name), nil
}
func (t *ParamSpecObj) M__repr__() (py.Object, error) {
	return py.String(variancePrefix(&t.typeParam) + t.name), nil
}
func (t *TypeVarTupleObj) M__repr__() (py.Object, error) { return py.String(t.name), nil }

func (t *TypeVarObj) M__or__(o py.Object) (py.Object, error)  { return form("Union"), nil }
func (t *TypeVarObj) M__ror__(o py.Object) (py.Object, error) { return form("Union"), nil }
func (t *ParamSpecObj) M__or__(o py.Object) (py.Object, error) {
	return form("Union"), nil
}
func (t *ParamSpecObj) M__ror__(o py.Object) (py.Object, error) {
	return form("Union"), nil
}

// M__iter__ makes "*Ts" work: it yields an Unpack[Ts] stand-in.
func (t *TypeVarTupleObj) M__iter__() (py.Object, error) {
	return py.NewIterator(py.Tuple{form("Unpack[" + t.name + "]")}), nil
}

func (p *typeParam) common(name string) (py.Object, bool, error) {
	switch name {
	case "__name__":
		return py.String(p.name), true, nil
	case "__default__":
		if p.dflt == nil {
			return noDefault, true, nil
		}
		v, err := p.dflt.get()
		return v, true, err
	case "has_default":
		return py.MustNewMethod("has_default", func(self py.Object) (py.Object, error) {
			return py.NewBool(p.dflt != nil), nil
		}, 0, "Return whether the parameter has a default."), true, nil
	case "__module__":
		return py.String("typing"), true, nil
	}
	return nil, false, nil
}

func (t *TypeVarObj) M__getattribute__(name string) (py.Object, error) {
	if v, ok, err := t.common(name); ok {
		return v, err
	}
	switch name {
	case "__bound__":
		if t.bound == nil {
			return py.None, nil
		}
		return t.bound.get()
	case "__constraints__":
		if t.constraints == nil {
			return py.Tuple{}, nil
		}
		return t.constraints.get()
	case "__covariant__":
		return py.NewBool(t.covariant), nil
	case "__contravariant__":
		return py.NewBool(t.contravar), nil
	case "__infer_variance__":
		return py.NewBool(t.inferVar), nil
	}
	return defaultAttr(t, name)
}

func (t *TypeVarTupleObj) M__getattribute__(name string) (py.Object, error) {
	if v, ok, err := t.common(name); ok {
		return v, err
	}
	return defaultAttr(t, name)
}

func (t *ParamSpecObj) M__getattribute__(name string) (py.Object, error) {
	if v, ok, err := t.common(name); ok {
		return v, err
	}
	switch name {
	case "args":
		return &paramSpecPart{t, false}, nil
	case "kwargs":
		return &paramSpecPart{t, true}, nil
	case "__bound__":
		return py.None, nil
	case "__covariant__":
		return py.NewBool(t.covariant), nil
	case "__contravariant__":
		return py.NewBool(t.contravar), nil
	case "__infer_variance__":
		return py.NewBool(t.inferVar), nil
	}
	return defaultAttr(t, name)
}

// paramSpecPart is P.args or P.kwargs.
type paramSpecPart struct {
	origin *ParamSpecObj
	kw     bool
}

func (p *paramSpecPart) Type() *py.Type {
	if p.kw {
		return paramSpecKwargs
	}
	return paramSpecArgs
}

func (p *paramSpecPart) M__repr__() (py.Object, error) {
	if p.kw {
		return py.String(p.origin.name + ".kwargs"), nil
	}
	return py.String(p.origin.name + ".args"), nil
}

func (p *paramSpecPart) M__getattribute__(name string) (py.Object, error) {
	if name == "__origin__" {
		return p.origin, nil
	}
	return defaultAttr(p, name)
}

// defaultAttr is object.__getattribute__: the ordinary lookup, for what the
// type does not answer itself.
func defaultAttr(self py.Object, name string) (py.Object, error) {
	return py.ObjectGetAttribute(self, name)
}

func flag(kwargs py.StringDict, key string) bool {
	v, ok := kwargs.Get(key)
	return ok && v == py.True
}

func nameArg(fn string, args py.Tuple) (string, error) {
	if len(args) < 1 {
		return "", py.ExceptionNewf(py.TypeError, "%s() missing required argument 'name' (pos 1)", fn)
	}
	name, ok := args[0].(py.String)
	if !ok {
		return "", py.ExceptionNewf(py.TypeError, "%s() argument 'name' must be str, not %s", fn, args[0].Type().Name)
	}
	return string(name), nil
}

func defaultArg(p *typeParam, kwargs py.StringDict) {
	if d, ok := kwargs.Get("default"); ok && d != noDefault {
		p.dflt = eagerLazy(d)
	}
}

func typeVarNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	name, err := nameArg("TypeVar", args)
	if err != nil {
		return nil, err
	}
	tv := &TypeVarObj{typeParam{name: name}}
	if len(args) > 1 {
		if len(args) == 2 {
			return nil, py.ExceptionNewf(py.TypeError, "A single constraint is not allowed")
		}
		tv.constraints = eagerLazy(append(py.Tuple{}, args[1:]...))
	}
	if bound, ok := kwargs.Get("bound"); ok && bound != py.None {
		if tv.constraints != nil {
			return nil, py.ExceptionNewf(py.TypeError, "Constraints cannot be combined with bound=...")
		}
		tv.bound = eagerLazy(bound)
	}
	tv.covariant = flag(kwargs, "covariant")
	tv.contravar = flag(kwargs, "contravariant")
	tv.inferVar = flag(kwargs, "infer_variance")
	if tv.covariant && tv.contravar {
		return nil, py.ExceptionNewf(py.ValueError, "Bivariant type variables are not supported.")
	}
	if tv.inferVar && (tv.covariant || tv.contravar) {
		return nil, py.ExceptionNewf(py.ValueError, "Variance cannot be specified with infer_variance.")
	}
	defaultArg(&tv.typeParam, kwargs)
	return tv, nil
}

func typeVarTupleNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "TypeVarTuple() takes exactly one argument (%d given)", len(args))
	}
	name, err := nameArg("TypeVarTuple", args)
	if err != nil {
		return nil, err
	}
	t := &TypeVarTupleObj{typeParam{name: name}}
	defaultArg(&t.typeParam, kwargs)
	return t, nil
}

func paramSpecNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "ParamSpec() takes exactly one argument (%d given)", len(args))
	}
	name, err := nameArg("ParamSpec", args)
	if err != nil {
		return nil, err
	}
	p := &ParamSpecObj{typeParam{name: name}}
	p.covariant = flag(kwargs, "covariant")
	p.contravar = flag(kwargs, "contravariant")
	p.inferVar = flag(kwargs, "infer_variance")
	defaultArg(&p.typeParam, kwargs)
	return p, nil
}

// TypeAliasObj is a TypeAliasType: what "type X = ..." binds.
type TypeAliasObj struct {
	name   string
	value  *lazy
	params py.Tuple
	module py.Object
}

var TypeAliasTypeType = py.NewTypeX("typing.TypeAliasType", "Type alias.", typeAliasNew, nil)

func (a *TypeAliasObj) Type() *py.Type { return TypeAliasTypeType }

func (a *TypeAliasObj) M__repr__() (py.Object, error) { return py.String(a.name), nil }

func (a *TypeAliasObj) M__getattribute__(name string) (py.Object, error) {
	switch name {
	case "__name__":
		return py.String(a.name), nil
	case "__value__":
		return a.value.get()
	case "__type_params__", "__parameters__":
		return a.params, nil
	case "__module__":
		if a.module == nil {
			return py.None, nil
		}
		return a.module, nil
	}
	return defaultAttr(a, name)
}

func (a *TypeAliasObj) M__getitem__(key py.Object) (py.Object, error) {
	if len(a.params) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "Only generic type aliases are subscriptable")
	}
	args, ok := key.(py.Tuple)
	if !ok {
		args = py.Tuple{key}
	}
	return &py.GenericAlias{Origin: a, Args: args}, nil
}

func (a *TypeAliasObj) M__or__(o py.Object) (py.Object, error)  { return form("Union"), nil }
func (a *TypeAliasObj) M__ror__(o py.Object) (py.Object, error) { return form("Union"), nil }

func typeAliasNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var name, value py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "TypeAliasType", 2, 2, &name, &value); err != nil {
		return nil, err
	}
	s, ok := name.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "TypeAliasType() argument 'name' must be str, not %s", name.Type().Name)
	}
	params := py.Tuple{}
	if tp, ok := kwargs.Get("type_params"); ok {
		t, ok := tp.(py.Tuple)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "type_params must be a tuple")
		}
		params = t
	}
	return &TypeAliasObj{name: string(s), value: eagerLazy(value), params: params}, nil
}

func registerTypeParams(globals py.StringDict) {
	globals.Set("TypeVar", TypeVarType)
	globals.Set("TypeVarTuple", TypeVarTupleType)
	globals.Set("ParamSpec", ParamSpecType)
	globals.Set("ParamSpecArgs", paramSpecArgs)
	globals.Set("ParamSpecKwargs", paramSpecKwargs)
	globals.Set("TypeAliasType", TypeAliasTypeType)
	globals.Set("NoDefault", noDefault)

	thunkOrNone := func(o py.Object) *lazy {
		if o == py.None {
			return nil
		}
		return &lazy{thunk: o}
	}
	def := func(name string, n int, f func(a py.Tuple) py.Object) {
		globals.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
			if len(args) != n {
				return nil, py.ExceptionNewf(py.TypeError, "%s() takes %d arguments", name, n)
			}
			return f(args), nil
		}, 0, "Internal: lowering of PEP 695 syntax."))
	}
	def("_gpy_typevar", 4, func(a py.Tuple) py.Object {
		tv := &TypeVarObj{typeParam{name: string(a[0].(py.String)), inferVar: true, dflt: thunkOrNone(a[3])}}
		switch a[1] {
		case py.Int(1):
			tv.bound = thunkOrNone(a[2])
		case py.Int(2):
			tv.constraints = thunkOrNone(a[2])
		}
		return tv
	})
	def("_gpy_typevartuple", 2, func(a py.Tuple) py.Object {
		return &TypeVarTupleObj{typeParam{name: string(a[0].(py.String)), dflt: thunkOrNone(a[1])}}
	})
	def("_gpy_paramspec", 2, func(a py.Tuple) py.Object {
		return &ParamSpecObj{typeParam{name: string(a[0].(py.String)), inferVar: true, dflt: thunkOrNone(a[1])}}
	})
	def("_gpy_alias", 4, func(a py.Tuple) py.Object {
		return &TypeAliasObj{name: string(a[0].(py.String)), value: &lazy{thunk: a[1]}, params: a[2].(py.Tuple), module: a[3]}
	})
	globals.Set("_gpy_annotate", py.MustNewMethod("_gpy_annotate", gpyAnnotate, 0, "Internal: body of a PEP 649 __annotate__ function."))
	globals.Set("_gpy_generic", py.MustNewMethod("_gpy_generic", func(self py.Object, args py.Tuple) (py.Object, error) {
		var params py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "_gpy_generic", 1, 1, &params); err != nil {
			return nil, err
		}
		var key py.Object = params
		if t := params.(py.Tuple); len(t) == 1 {
			key = t[0]
		}
		return py.GetItem(GenericType, key)
	}, 0, "Internal: Generic[params], the implicit base of a generic class."))
}

// IsTypeParam lets a generic alias find and substitute these parameters.
func (t *TypeVarObj) IsTypeParam() {}

// gpyAnnotate is the body of every compiler-made __annotate__ function:
// (format, names, thunks, sources) -> the annotations dict.  FORWARDREF
// (3) turns an annotation that raises NameError into a ForwardRef of its
// source text, STRING (4) gives the source text, and the VALUE formats
// (1, 2) evaluate and propagate errors.
func gpyAnnotate(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 5 {
		return nil, py.ExceptionNewf(py.TypeError, "_gpy_annotate() takes 5 arguments")
	}
	format, names, thunks, srcs, fwd := args[0], args[1].(py.Tuple), args[2].(py.Tuple), args[3].(py.Tuple), args[4]
	f, err0 := py.MakeGoInt(format)
	if err0 != nil || f < 1 || f > 4 {
		return nil, py.ExceptionNewf(py.NotImplementedError, "")
	}
	out := py.NewStringDict()
	for i, name := range names {
		var v py.Object
		var err error
		switch f {
		case 4:
			v = srcs[i]
		default:
			v, err = py.Call(thunks[i], py.Tuple{}, py.StringDict{})
			if err != nil && f == 3 && py.IsException(py.NameError, err) {
				var cls py.Object
				if cls, err = py.Call(fwd, py.Tuple{}, py.StringDict{}); err == nil {
					v, err = py.Call(cls, py.Tuple{srcs[i]}, py.StringDict{})
				}
			}
		}
		if err != nil {
			return nil, err
		}
		out.Set(string(name.(py.String)), v)
	}
	return out, nil
}
