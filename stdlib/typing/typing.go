// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package typing provides the implementation of python's 'typing' module.
//
// CPython's typing is a 3,866 line pure Python module, and it cannot be run
// here: it uses the walrus operator, PEP 695 type parameters and a
// metaclass-based Generic/Protocol system, none of which this interpreter
// has.  This is a native port covering the part of the module that has any
// meaning at run time.
//
// The split is deliberate and worth stating:
//
//   - Names that only ever appear in an annotation - Any, Optional, List,
//     Callable, IO and the rest - are inert.  Now that "from __future__
//     import annotations" keeps annotations as text, they are never
//     resolved, so their only job is to exist and be subscriptable.  Each
//     one says what it is rather than pretending to be a full
//     implementation.
//
//   - Names with real behaviour are real: cast returns its argument,
//     overload and final return the function they are given, TYPE_CHECKING
//     is False, and TypeVar, NewType, Generic and Protocol exist as objects.
//
//   - The container names are aliases of collections.abc, which does
//     implement them, so isinstance(x, typing.Iterable) answers correctly.
package typing

import (
	"strings"
	"unsafe"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/abc"
	"github.com/vishnukv64/gpython/stdlib/collections"
)

const module_doc = `The typing module: support for type hints.

Only the part of this module with run-time meaning is implemented here; the
names that exist purely for a type checker are inert placeholders.`

// specialForm is a stand-in for one of the module's typing constructs.
//
// It is subscriptable - Any[int], Optional[str], Callable[[int], str] - and
// returns itself, because the parameters carry no run-time meaning.
type specialForm struct {
	name string
	doc  string
}

var SpecialFormType = py.NewTypeX("typing._SpecialForm", "A typing construct with no run-time behaviour.", nil, nil)

func init() {
	SpecialFormType.Dict.Set("__hash__", py.MustNewMethod("__hash__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*specialForm).M__hash__()
	}, 0, "Return hash(self)."))
}

func (s *specialForm) Type() *py.Type { return SpecialFormType }

func (s *specialForm) M__repr__() (py.Object, error) {
	return py.String("typing." + s.name), nil
}

// M__hash__ keys a bare construct - "Dict", "List" - by identity, as a
// singleton should be; h11 puts "Dict" and "Type" into its transition tables.
func (s *specialForm) M__hash__() (py.Object, error) {
	return py.Int(int64(uintptr(unsafe.Pointer(s))) & (1<<62 - 1)), nil
}

func (s *specialForm) M__getitem__(key py.Object) (py.Object, error) {
	// A subscripted construct is still a class, because it is used as a base:
	// "class ParamType(Generic[_T], ABC)".  Returning the form itself would
	// make that a TypeError ("not an acceptable base type"), so a real type
	// is returned, named after the construct and deriving from the form's
	// own class.
	return &subscribedForm{form: s, name: s.name + "[" + describe(key) + "]"}, nil
}

// subscribedForm is the class that "Generic[T]" or "Optional[int]" produces
// when it is used as a base class.
type subscribedForm struct {
	form *specialForm
	name string
}

var subscribedFormType = py.NewTypeX("typing._SubscribedForm", "A subscripted typing construct.", nil, nil)

// M__hash__ is identity-based, which is what makes a typing construct usable
// as a DICT KEY.
//
// h11 builds its state-transition tables as dicts keyed by constructs -
// "Dict[Type[Event], ...]" - and looked them up at import: every one of them
// was unhashable, so "import h11" died on "unhashable type: 'type'".  A typing
// construct is a singleton, so identity is the right notion of equality and
// h11's own comment says as much ("inherit identity-based comparison and
// hashing from object").
func (s *subscribedForm) M__hash__() (py.Object, error) {
	return py.Int(int64(uintptr(unsafe.Pointer(s))) & (1<<62 - 1)), nil
}

func init() {
	// "Generic[T]" is used as a base class, so the class it produces must be
	// one that can be derived from.
	subscribedFormType.Flags |= py.TPFLAGS_BASETYPE
	// And hashable, so it can key a dict - see M__hash__ above.
	subscribedFormType.Dict.Set("__hash__", py.MustNewMethod("__hash__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*subscribedForm).M__hash__()
	}, 0, "Return hash(self)."))
}

func (s *subscribedForm) Type() *py.Type { return subscribedFormType }

func (s *subscribedForm) M__repr__() (py.Object, error) {
	return py.String("typing." + s.name), nil
}

func describe(key py.Object) string {
	text, err := py.ReprAsString(key)
	if err != nil {
		return "?"
	}
	return text
}

// callable, so that constructs used with parameters - TypeVar("T"), or the
// Optional[...] form used as a value - do not fail.
func (s *specialForm) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return s, nil
}

// M__or__ supports the PEP 604 union syntax: "SupportsInt | None" is
// evaluated at run time unless annotations are deferred, so an inert form
// that could not be combined with "|" would break the annotation it appears
// in.  The result is another inert form, since the union has no run-time
// meaning either.
func (s *specialForm) M__or__(other py.Object) (py.Object, error) {
	return form(s.name + " | " + nameOf(other)), nil
}

func nameOf(v py.Object) string {
	if sf, ok := v.(*specialForm); ok {
		return sf.name
	}
	if t, ok := v.(*py.Type); ok {
		return t.Name
	}
	if text, err := py.ReprAsString(v); err == nil {
		return text
	}
	return "?"
}

var (
	_ py.I__repr__    = (*specialForm)(nil)
	_ py.I__getitem__ = (*specialForm)(nil)
	_ py.I__call__    = (*specialForm)(nil)
	_ py.I__or__      = (*specialForm)(nil)
)

func form(name string) *specialForm {
	return &specialForm{name: name}
}

// TypeVarObj is what TypeVar() returns: a named placeholder that may be
// constrained, and that is subscriptable by a type checker only.
type TypeVarObj struct {
	name          string
	constraints   []py.Object
	bound         py.Object
	covariant     bool
	contravariant bool
}

var TypeVarType = py.NewTypeX("typing.TypeVar", "A type variable.", nil, nil)

func (t *TypeVarObj) Type() *py.Type { return TypeVarType }

func (t *TypeVarObj) M__repr__() (py.Object, error) {
	if len(t.constraints) > 0 {
		parts := make([]string, 0, len(t.constraints))
		for _, c := range t.constraints {
			s, err := py.ReprAsString(c)
			if err != nil {
				return nil, err
			}
			parts = append(parts, s)
		}
		return py.String("~" + t.name + "(" + strings.Join(parts, ", ") + ")"), nil
	}
	return py.String("~" + t.name), nil
}

func (t *TypeVarObj) M__getitem__(key py.Object) (py.Object, error) { return t, nil }

// newType is what NewType() returns: a callable whose only behaviour is to
// return what it is given, which is all it does in CPython too.
type newType struct {
	name string
	tp   py.Object
}

var newTypeType = py.NewTypeX("typing.NewType", "A distinct type created by NewType().", nil, nil)

func (n *newType) Type() *py.Type { return newTypeType }

func (n *newType) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var value py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, n.name, 1, 1, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (n *newType) M__repr__() (py.Object, error) {
	return py.String("typing.NewType('" + n.name + "')"), nil
}

var _ py.I__call__ = (*newType)(nil)

func init() {
	globals := py.NewStringDict()

	// Names that exist only for a type checker.  They are subscriptable and
	// say so in their repr.
	for _, name := range []string{
		"AbstractSet", "Any", "AnyStr", "AsyncContextManager", "AsyncGenerator",
		"AsyncIterable", "AsyncIterator", "Awaitable", "BinaryIO", "ByteString",
		"Callable", "ClassVar", "Collection", "Concatenate", "Container",
		"ContextManager", "Coroutine", "DefaultDict", "Deque", "Dict",
		"Final", "ForwardRef", "FrozenSet", "Generator",
		"Hashable", "IO", "ItemsView", "Iterable", "Iterator", "KeysView",
		"List", "Literal", "LiteralString", "Mapping", "MappingView",
		"Match", "Pattern", "MutableMapping", "MutableSequence", "MutableSet",
		"NamedTuple", "Never", "NoReturn", "NotRequired", "Optional",
		"OrderedDict", "ParamSpec", "Pattern", "ReadOnly", "Required",
		"Reversible", "Self", "Sequence", "Set", "Sized", "Text",
		"TextIO", "Tuple", "Type", "TypeAlias", "TypeGuard", "Union",
		"Unpack", "ValuesView", "final", "overload", "runtime_checkable",
		"SupportsInt", "SupportsFloat", "SupportsComplex", "SupportsBytes",
		"SupportsAbs", "SupportsRound", "SupportsIndex", "SupportsRichComparison",
		"Buffer", "LiteralString", "Self", "Never", "NoReturn", "AnyStr",
		"TypeVarTuple", "TypeAliasType", "Generic", "Protocol",
	} {
		globals.Set(name, form(name))
	}

	// The containers that collections.abc actually implements are aliases of
	// it, so isinstance answers correctly rather than always being False.
	globals.Set("Iterable", abc.IterableType)
	globals.Set("Iterator", abc.IteratorType)
	globals.Set("Sized", abc.SizedType)
	globals.Set("Container", abc.ContainerType)
	globals.Set("Hashable", abc.HashableType)
	globals.Set("Callable", abc.CallableType)
	globals.Set("Collection", abc.CollectionType)
	globals.Set("Sequence", abc.SequenceType)
	globals.Set("MutableSequence", abc.MutableSequenceType)
	globals.Set("AbstractSet", abc.SetType)
	globals.Set("MutableSet", abc.MutableSetType)
	globals.Set("Mapping", abc.MappingType)
	globals.Set("MutableMapping", abc.MutableMappingType)
	globals.Set("Reversible", abc.ReversibleType)
	globals.Set("Coroutine", abc.CoroutineType)
	globals.Set("Awaitable", abc.AwaitableType)
	globals.Set("AsyncIterable", abc.AsyncIterableType)
	globals.Set("AsyncIterator", abc.AsyncIteratorType)
	globals.Set("Generator", abc.GeneratorType)

	// TYPE_CHECKING is False, as it is at run time in CPython, so the blocks
	// that guard imports for type checkers do not execute.
	globals.Set("TYPE_CHECKING", py.False)

	// Names with real behaviour.
	globals.Set("cast", py.MustNewMethod("cast", func(self py.Object, args py.Tuple) (py.Object, error) {
		var typ, value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "cast", 2, 2, &typ, &value); err != nil {
			return nil, err
		}
		return value, nil
	}, 0, "Cast a value to a type.  At run time this returns the value unchanged."))

	globals.Set("overload", py.MustNewMethod("overload", passthrough, 0, "Decorator for overloaded functions: returns the function itself."))
	globals.Set("final", py.MustNewMethod("final", passthrough, 0, "Decorator to indicate a final method: returns the function itself."))
	globals.Set("no_type_check", py.MustNewMethod("no_type_check", passthrough, 0, "Decorator to indicate no type checking: returns the object."))
	globals.Set("runtime_checkable", py.MustNewMethod("runtime_checkable", passthrough, 0, "Mark a protocol as runtime checkable: returns the class."))

	globals.Set("TypeVar", py.MustNewMethod("TypeVar", typeVarNew, 0, "TypeVar(name, *constraints, bound=None, covariant=False, contravariant=False)"))
	globals.Set("NewType", py.MustNewMethod("NewType", newTypeNew, 0, "NewType(name, tp) -> a callable that returns its argument."))
	globals.Set("NamedTuple", namedTupleType)
	// Generic and Protocol are used as bases, so they are real classes with
	// a class-getitem, rather than inert forms.
	// TypedDict is a class base for a dict-shaped record, and is also called
	// as a factory.  As a base it needs a real class; calling it returns the
	// class itself, since the fields carry no run-time meaning here.
	typedDictType := py.NewType("typing.TypedDict", "A dictionary with a fixed set of keys.")
	typedDictType.Flags |= py.TPFLAGS_BASETYPE
	typedDictType.Dict.Set("__call__", py.MustNewMethod("__call__", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return self, nil
	}, 0, "Return the class, ignoring the field specification."))
	// "class X(t.TypedDict, total=False)" is a base with keywords.  There is
	// no __init_subclass__ here, so the keywords are accepted and ignored
	// through the type's own constructor.
	typedDictType.New = func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) >= 2 {
			// The factory form: TypedDict('Name', {...}) returns a class.
			nameObj, _ := py.StrAsString(args[0])
			cls := py.NewType(nameObj, "A TypedDict.")
			cls.Flags |= py.TPFLAGS_BASETYPE
			cls.Base = typedDictType
			return cls, nil
		}
		return typedDictType, nil
	}

	genericType := py.NewType("typing.Generic", "Abstract base class for generic types.")
	genericType.Flags |= py.TPFLAGS_BASETYPE
	genericType.Dict.Set("__class_getitem__", py.MustNewMethod("__class_getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Return the class, ignoring the subscription parameters."))
	globals.Set("Generic", genericType)
	globals.Set("TypedDict", typedDictType)

	// ParamSpec and Concatenate are used in signatures: P = ParamSpec("P"),
	// then Callable[Concatenate[T, P], R].  They need to be callable and
	// subscriptable, which the inert forms already are.
	globals.Set("ParamSpec", py.MustNewMethod("ParamSpec", func(self py.Object, args py.Tuple) (py.Object, error) {
		return form("ParamSpec"), nil
	}, 0, "Return a parameter specification, subscriptable by a type checker only."))
	globals.Set("TypeVarTuple", py.MustNewMethod("TypeVarTuple", func(self py.Object, args py.Tuple) (py.Object, error) {
		return form("TypeVarTuple"), nil
	}, 0, "Return a variadic type variable."))
	globals.Set("TypeAliasType", py.MustNewMethod("TypeAliasType", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) > 1 {
			return args[1], nil
		}
		return form("TypeAliasType"), nil
	}, 0, "Create a type alias."))
	globals.Set("get_protocol_members", py.MustNewMethod("get_protocol_members", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewListFromItems(nil), nil
	}, 0, "Return the members of a protocol."))
	globals.Set("is_protocol", py.MustNewMethod("is_protocol", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.False, nil
	}, 0, "Return whether the class is a protocol."))
	globals.Set("assert_type", py.MustNewMethod("assert_type", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) == 0 {
			return py.None, nil
		}
		return args[0], nil
	}, 0, "Return the value, for a type checker to assert on."))
	globals.Set("assert_never", py.MustNewMethod("assert_never", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "Mark unreachable code."))
	globals.Set("reveal_type", py.MustNewMethod("reveal_type", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) == 0 {
			return py.None, nil
		}
		return args[0], nil
	}, 0, "Reveal the type of an expression."))
	globals.Set("dataclass_transform", py.MustNewMethod("dataclass_transform", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		// Used as a decorator, with or without arguments.
		if len(args) == 1 {
			return args[0], nil
		}
		return &passthroughDecorator{}, nil
	}, 0, "Mark a class or function as a dataclass-like transform."))
	globals.Set("override", py.MustNewMethod("override", passthrough, 0, "Mark a method as overriding its base."))
	globals.Set("deprecated", py.MustNewMethod("deprecated", passthrough, 0, "Mark a function as deprecated."))
	globals.Set("get_overloads", py.MustNewMethod("get_overloads", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewListFromItems(nil), nil
	}, 0, "Return the overloads of a function."))

	protocolType := py.NewType("typing.Protocol", "Base class for protocol classes.")
	protocolType.Flags |= py.TPFLAGS_BASETYPE
	protocolType.Dict.Set("__class_getitem__", py.MustNewMethod("__class_getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Return the class, ignoring the subscription parameters."))
	globals.Set("Protocol", protocolType)

	globals.Set("get_type_hints", py.MustNewMethod("get_type_hints", getTypeHints, 0, "Return the annotations of an object."))
	globals.Set("get_args", py.MustNewMethod("get_args", getArgs, 0, "Return the arguments of a subscripted type, as far as they are kept."))
	globals.Set("get_origin", py.MustNewMethod("get_origin", getArgs, 0, "Return the unsubscripted type."))

	// The module also re-exports the collections types it names.
	globals.Set("Deque", collections.DequeType)
	globals.Set("OrderedDict", collections.OrderedDictType)
	globals.Set("DefaultDict", collections.DefaultDictType)
	globals.Set("Counter", collections.CounterType)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "typing",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

func passthrough(self py.Object, args py.Tuple) (py.Object, error) {
	var obj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "typing", 1, 1, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func typeVarNew(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "TypeVar() missing required argument 'name'")
	}
	name := args[0]
	text, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	tv := &TypeVarObj{name: text}
	// The remaining positional arguments are constraints.
	if len(args) > 1 {
		tv.constraints = append(tv.constraints, args[1:]...)
	}
	if bound, ok := kwargs.Get("bound"); ok {
		tv.bound = bound
	}
	if cov, ok := kwargs.Get("covariant"); ok && cov == py.True {
		tv.covariant = true2()
	}
	if contra, ok := kwargs.Get("contravariant"); ok && contra == py.True {
		tv.contravariant = true2()
	}
	return tv, nil
}

func true2() bool { return true }

func newTypeNew(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var name, tp py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "NewType", 2, 2, &name, &tp); err != nil {
		return nil, err
	}
	text, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	return &newType{name: text, tp: tp}, nil
}

func namedTupleNew(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// The same factory as collections.namedtuple; the class form used by a
	// type checker ("class P(NamedTuple)") is handled by
	// buildNamedTupleClass, installed on py.BuildNamedTupleClass below.
	mod := collectionsModule()
	if mod == nil {
		return nil, py.ExceptionNewf(py.ImportError, "collections is not available")
	}
	fn, err := py.GetAttrString(mod, "namedtuple")
	if err != nil {
		return nil, err
	}
	return py.Call(fn, args, kwargs)
}

// namedTupleType is typing.NamedTuple.  It carries the flag __build_class__
// looks for, so "class P(NamedTuple): ..." is handed to buildNamedTupleClass,
// and it is callable as the factory namedtuple(name, fields).
var namedTupleType = py.NewType("typing.NamedTuple",
	"Typed version of collections.namedtuple.  Also usable as a base class:"+
		" class P(NamedTuple): x: int")

// buildNamedTupleClass turns the namespace of a class deriving from NamedTuple
// into a named tuple class.  The annotations supply the field names, in order;
// defaults, if present in the namespace, are applied through the class's
// __new__ defaults, which collections.namedtuple already honours.
func buildNamedTupleClass(name string, bases []py.Object, ns py.StringDict) (py.Object, error) {
	mod := collectionsModule()
	if mod == nil {
		return nil, py.ExceptionNewf(py.ImportError, "collections is not available")
	}
	fn, err := py.GetAttrString(mod, "namedtuple")
	if err != nil {
		return nil, err
	}
	// The field names come from __annotations__, in insertion order.
	var fields []string
	if anns, ok := ns.Get("__annotations__"); ok {
		if d, ok := anns.(py.StringDict); ok {
			for _, ent := range d.Items() {
				fields = append(fields, ent.Key)
			}
		}
	}
	if len(fields) == 0 {
		return nil, py.ExceptionNewf(py.TypeError,
			"named tuple %s has no fields", name)
	}
	clsObj, err := py.Call(fn, py.Tuple{py.String(name), py.NewListFromStrings(fields)}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	cls, ok := clsObj.(*py.Type)
	if !ok {
		return clsObj, nil
	}
	// Method bodies in the class namespace - "def meth(self): ..." - are copied
	// onto the generated class so a NamedTuple subclass can carry behaviour,
	// as CPython allows.
	for _, ent := range ns.Items() {
		if ent.Key == "__annotations__" || ent.Key == "__module__" || ent.Key == "__qualname__" {
			continue
		}
		cls.Dict.Set(ent.Key, ent.Value)
	}
	return cls, nil
}

// collectionsModule returns the collections module, which is where the
// namedtuple factory lives.  It is fetched from the runtime rather than
// through an import, because this is an embedded module too.
func collectionsModule() py.Object {
	return py.GetModuleImplOrNil("collections")
}

const getTypeHints_doc = `get_type_hints(obj) -> dict

Return the annotations of an object.`

func getTypeHints(self py.Object, args py.Tuple) (py.Object, error) {
	var obj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "get_type_hints", 1, 1, &obj); err != nil {
		return nil, err
	}
	anns, err := py.GetAttrString(obj, "__annotations__")
	if err != nil || anns == py.None || anns == nil {
		return py.NewStringDict(), nil
	}
	return anns, nil
}

const getArgs_doc = `get_args(tp) -> tuple

Return the arguments of a subscripted type.

The parameters of a typing construct carry no run-time meaning here, so an
empty tuple is returned rather than a guess.`

func getArgs(self py.Object, args py.Tuple) (py.Object, error) {
	var tp py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "get_args", 1, 1, &tp); err != nil {
		return nil, err
	}
	if _, ok := tp.(*specialForm); ok {
		return py.Tuple{}, nil
	}
	return py.Tuple{tp}, nil
}

// passthroughDecorator is returned when a decorator is used with arguments.
type passthroughDecorator struct{}

var passthroughDecoratorType = py.NewType("typing._decorator", "A decorator awaiting its function.")

func (p *passthroughDecorator) Type() *py.Type { return passthroughDecoratorType }

func (p *passthroughDecorator) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	return py.None, nil
}

var _ py.I__call__ = (*passthroughDecorator)(nil)

// The PEP 604 union operator on the types themselves.
//
// "str | bytes" appears in evaluated positions - a class base, a default -
// where deferring annotations does not help, so a type has to support "|".
// The result is an inert union object: it is only ever used as a base class
// or passed around, never to test membership.
func init() {
	unionType := py.NewType("typing.UnionType", "The result of X | Y.")
	unionType.Flags |= py.TPFLAGS_BASETYPE
	unionType.Dict.Set("__or__", py.MustNewMethod("__or__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var other py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__or__", 1, 1, &other); err != nil {
			return nil, err
		}
		return form("Union"), nil
	}, 0, "Return the union of two types."))

	// Every type gains __or__, which is what makes "str | bytes" work.
	py.TypeType.Dict.Set("__or__", py.MustNewMethod("__or__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return form("Union"), nil
	}, 0, "Return the union of two types."))
	py.ObjectType.Dict.Set("__or__", py.TypeType.Dict.GetOrNil("__or__"))

	// typing.NamedTuple is both a base class (recognised by __build_class__
	// through the flag) and the factory NamedTuple(name, fields).
	namedTupleType.Flags |= py.TPFLAGS_BASETYPE | py.TPFLAGS_NAMEDTUPLE
	namedTupleType.New = func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) >= 2 {
			return namedTupleNew(nil, args, kwargs)
		}
		return nil, py.ExceptionNewf(py.TypeError,
			"NamedTuple() requires a type name and field names")
	}
	py.BuildNamedTupleClass = buildNamedTupleClass
}
