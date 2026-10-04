// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Type objects - these make objects

// FIXME should be caching the expensive lookups in the superclasses
// and using the cache clearing machinery to clear the caches when the
// heirachy changes

// FIXME should make Mro and Bases be []*Type

package py

import (
	"fmt"
	"log"
	"reflect"
)

// Type flags (tp_flags)
//
// These flags are used to extend the type structure in a backwards-compatible
// fashion. Extensions can use the flags to indicate (and test) when a given
// type structure contains a new feature. The Python core will use these when
// introducing new functionality between major revisions (to avoid mid-version
// changes in the PYTHON_API_VERSION).
//
// Arbitration of the flag bit positions will need to be coordinated among
// all extension writers who publically release their extensions (this will
// be fewer than you might expect!)..
//
// Most flags were removed as of Python 3.0 to make room for new flags.  (Some
// flags are not for backwards compatibility but to indicate the presence of an
// optional feature; these flags remain of course.)
//
// Type definitions should use TPFLAGS_DEFAULT for their tp_flags value.
//
// Code can use PyType_HasFeature(type_ob, flag_value) to test whether the
// given type object has a specified feature.

const (
	// Set if the type object is dynamically allocated
	TPFLAGS_HEAPTYPE uint = 1 << 9

	// Set if the type allows subclassing
	TPFLAGS_BASETYPE uint = 1 << 10

	// Set if the type is 'ready' -- fully initialized
	TPFLAGS_READY uint = 1 << 12

	// Set while the type is being 'readied', to prevent recursive ready calls
	TPFLAGS_READYING uint = 1 << 13

	// Objects support garbage collection (see objimp.h)
	TPFLAGS_HAVE_GC uint = 1 << 14

	// Objects support type attribute cache
	TPFLAGS_HAVE_VERSION_TAG  uint = 1 << 18
	TPFLAGS_VALID_VERSION_TAG uint = 1 << 19

	// Type is abstract and cannot be instantiated
	TPFLAGS_IS_ABSTRACT uint = 1 << 20

	// These flags are used to determine if a type is a subclass.
	TPFLAGS_INT_SUBCLASS      uint = 1 << 23
	TPFLAGS_LONG_SUBCLASS     uint = 1 << 24
	TPFLAGS_LIST_SUBCLASS     uint = 1 << 25
	TPFLAGS_TUPLE_SUBCLASS    uint = 1 << 26
	TPFLAGS_BYTES_SUBCLASS    uint = 1 << 27
	TPFLAGS_UNICODE_SUBCLASS  uint = 1 << 28
	TPFLAGS_DICT_SUBCLASS     uint = 1 << 29
	TPFLAGS_BASE_EXC_SUBCLASS uint = 1 << 30
	TPFLAGS_TYPE_SUBCLASS     uint = 1 << 31

	TPFLAGS_DEFAULT = TPFLAGS_HAVE_VERSION_TAG
)

type NewFunc func(metatype *Type, args Tuple, kwargs StringDict) (Object, error)

type InitFunc func(self Object, args Tuple, kwargs StringDict) error

type Type struct {
	ObjectType *Type  // Type of this object -- FIXME this is redundant in Base?
	Name       string // For printing, in format "<module>.<name>"
	Doc        string // Documentation string
	//	Methods    StringDict // *PyMethodDef
	//	Members    StringDict // *PyMemberDef
	//	Getset     *PyGetSetDef
	Base *Type
	Dict StringDict
	//	Dictoffset int
	Bases Tuple
	Mro   Tuple // method resolution order
	//	Cache      Object
	//	Subclasses Tuple
	//	Weaklist   Tuple
	New      NewFunc
	Init     InitFunc
	Flags    uint // Flags to define presence of optional/expanded features
	Qualname string

	// NoSubclass is set by a native type that declines to be a BASE even
	// though its own base accepts subclassing.  NewType inherits flags from
	// the superclass, so list and set would otherwise inherit BASETYPE from
	// object and accept a subclass; and clearing the flag in an init() does
	// not stick, because Ready() assigns the flags afterwards.  The base-type
	// check reads this field, which is the one place that decides.
	NoSubclass bool

	// Payload is where an INSTANCE of a Python subclass of a builtin container
	// keeps its data.
	//
	// An instance of a python-level class is a *Type whose namespace is a Dict,
	// so "class D(dict)" works - the instance's Dict IS the mapping.  A
	// SEQUENCE has positional items and a namespace has nowhere to put them, so
	// "class L(list); L([1, 2])" produced an object whose len() raised, and
	// "class _TokenType(tuple)" - pygments' token type, which pip needs - was not
	// a tuple at all, so "Token.Text" raised AttributeError.
	//
	// This field holds the value the native constructor produced, and the
	// container protocols read it.  It is nil on every class and on every
	// ordinary python-level instance, which is what distinguishes an instance
	// that carries data from one that does not.
	//
	// An instance is NOT marked by any other field: Name is empty for every
	// instance and ObjectType points at the class, so Payload is the one thing
	// that says "a builtin's data lives here".
	Payload Object

	/*
	   Py_ssize_t tp_basicsize, tp_itemsize; // For allocation

	   // Methods to implement standard operations

	   destructor tp_dealloc;
	   printfunc tp_print;
	   getattrfunc tp_getattr;
	   setattrfunc tp_setattr;
	   void *tp_reserved; // formerly known as tp_compare
	   reprfunc tp_repr;

	   // Method suites for standard classes

	   PyNumberMethods *tp_as_number;
	   PySequenceMethods *tp_as_sequence;
	   PyMappingMethods *tp_as_mapping;

	   // More standard operations (here for binary compatibility)

	   hashfunc tp_hash;
	   ternaryfunc tp_call;
	   reprfunc tp_str;
	   getattrofunc tp_getattro;
	   setattrofunc tp_setattro;

	   // Functions to access object as input/output buffer
	   PyBufferProcs *tp_as_buffer;

	   // Flags to define presence of optional/expanded features
	   unsigned long tp_flags;

	   const char *tp_doc; // Documentation string

	   // Assigned meaning in release 2.0
	   // call function for all accessible objects
	   traverseproc tp_traverse;

	   // delete references to contained objects
	   inquiry tp_clear;

	   // Assigned meaning in release 2.1
	   // rich comparisons
	   richcmpfunc tp_richcompare;

	   // weak reference enabler
	   Py_ssize_t tp_weaklistoffset;

	   // Iterators
	   getiterfunc tp_iter;
	   iternextfunc tp_iternext;

	   // Attribute descriptor and subclassing stuff
	   struct PyMethodDef *tp_methods;
	   struct PyMemberDef *tp_members;
	   struct PyGetSetDef *tp_getset;
	   struct _typeobject *tp_base;
	   PyObject *tp_dict;
	   descrgetfunc tp_descr_get;
	   descrsetfunc tp_descr_set;
	   Py_ssize_t tp_dictoffset;
	   initproc tp_init;
	   allocfunc tp_alloc;
	   newfunc tp_new;
	   freefunc tp_free; // Low-level free-memory routine
	   inquiry tp_is_gc; // For PyObject_IS_GC
	   PyObject *tp_bases;
	   PyObject *tp_mro; // method resolution order
	   PyObject *tp_cache;
	   PyObject *tp_subclasses;
	   PyObject *tp_weaklist;
	   destructor tp_del;

	   // Type attribute cache version tag. Added in version 2.6
	   unsigned int tp_version_tag;

	   destructor tp_finalize;
	*/
}

var TypeType *Type = &Type{
	Name: "type",
	Doc:  "type(object) -> the object's type\ntype(name, bases, dict) -> a new type",
	// A metaclass derives from type - "class Sentinel(type)" - so type has to
	// accept subclassing, exactly as object does.  Without the flag EVERY
	// metaclass was refused with "type 'type' is not an acceptable base type",
	// which is what stopped h11: its Sentinel is a metaclass and its whole
	// state machine is keyed by instances of it.
	Flags: TPFLAGS_BASETYPE,
	Dict:  NewStringDict(),
}

var ObjectType = &Type{
	Name:  "object",
	Doc:   "The most base type",
	Flags: TPFLAGS_BASETYPE,
	Dict:  NewStringDict(),
}

func init() {
	// Initialised like this to avoid initialisation loops
	TypeType.New = TypeNew
	TypeType.Init = TypeInit
	TypeType.ObjectType = TypeType
	ObjectType.New = ObjectNew
	ObjectType.Init = ObjectInit
	ObjectType.ObjectType = TypeType
	err := TypeType.Ready()
	if err != nil {
		log.Fatal(err)
	}
	err = ObjectType.Ready()
	if err != nil {
		log.Fatal(err)
	}
}

// Type of this object
func (t *Type) Type() *Type {
	return t.ObjectType
}

// Satistfy error interface
func (t *Type) Error() string {
	return t.Name
}

// Get the Dict
func (t *Type) GetDict() StringDict {
	return t.Dict
}

// SetDict replaces an instance's namespace, which is what assigning to
// __dict__ does.  A Type that is a CLASS must not be re-pointed at another
// dict this way - its Dict IS its class namespace - so this refuses for a
// named type and is only reached for an instance (an empty Name).
func (t *Type) SetDict(d StringDict) {
	if t.Name != "" {
		// A class's namespace is replaced through the class, not by
		// assigning to an instance's __dict__; leave it alone rather than
		// silently swapping a class's own dict out from under it.
		return
	}
	t.Dict = d
}

// delayedReady holds types waiting to be intialised
var delayedReady = []*Type{}

// delayedReadyPending records whether delayedReady has entries that arrived
// after TypeMakeReady last drained the queue.
//
// TypeMakeReady runs once, during this package's init, so a type declared by any
// later-initialising package (every stdlib module) is appended to delayedReady
// but never readied: it keeps an empty Mro and an incomplete dict copy.  Ready()
// is idempotent, so a type made this way is readied lazily on first use instead,
// in TypeEnsureReady.
var delayedReadyPending bool

// ensureReadyHook lets Lookup drain the delayed-ready queue without Go
// reporting an initialisation cycle between this file and the type
// constructors.  It is set in init, which runs before anything can call
// Lookup.
var ensureReadyHook func()

func init() {
	ensureReadyHook = func() {
		if err := TypeEnsureReady(); err != nil {
			// A type that fails to ready is skipped rather than fatal: the
			// alternative is that one bad stdlib type makes every attribute
			// lookup in the process panic.
			_ = err
		}
	}
}

// TypeDelayReady stores the list of types to initialise
//
// Call MakeReady when all initialised
func TypeDelayReady(t *Type) {
	delayedReady = append(delayedReady, t)
	delayedReadyPending = true
}

// TypeEnsureReady readies every type queued since the last drain.
//
// It is called before an operation that depends on a type's Mro being populated,
// so that types created by packages which initialise after this one are usable.
// Ready() is a no-op for an already-ready type, so calling this repeatedly is
// cheap; the pending flag keeps the common case off the queue entirely.
func TypeEnsureReady() error {
	if !delayedReadyPending {
		return nil
	}
	return TypeMakeReady()
}

// TypeMakeReady readies all the types
func TypeMakeReady() (err error) {
	for _, t := range delayedReady {
		err = t.Ready()
		if err != nil {
			return fmt.Errorf("Error initialising go type %s: %v", t.Name, err)
		}
	}
	delayedReady = nil
	delayedReadyPending = false
	return nil
}

func init() {
	err := TypeMakeReady()
	if err != nil {
		log.Fatal(err)
	}

	// Type metadata readable from Python (cls.__name__, cls.__doc__, ...)
	//
	// These go on ObjectType as well as TypeType because the two are both
	// metatypes here: a type made by the NewType function is a TypeType, but
	// one made with ObjectType.NewType - which is how most of the builtin
	// types are declared - is an ObjectType.  Putting them only on TypeType
	// is why "int.__name__" raised while "bool.__name__" worked.
	//
	// ObjectType's dict is also the dict of the "object" class, so an
	// instance would find these through its base chain.  Each getter
	// therefore checks that it was handed a type and raises AttributeError
	// otherwise, which is exactly what CPython reports for "x.__name__".
	typeMeta := func(name string, doc string, get func(t *Type) Object) {
		d := &typeMetaGetter{name: name, get: get}
		TypeType.Dict.Set(name, d)
		ObjectType.Dict.Set(name, d)
	}
	typeMeta("__name__", "The name of the type.", func(t *Type) Object { return String(t.Name) })
	typeMeta("__qualname__", "The qualified name of the type.", func(t *Type) Object { return String(t.Name) })
	typeMeta("__doc__", "The documentation string of the type.", func(t *Type) Object { return String(t.Doc) })
	typeMeta("__bases__", "The base classes of the type.", func(t *Type) Object { return t.Bases })
	// __mro__ is a standard attribute of every class, and inspect.getmro is
	// literally "cls.__mro__".  It was absent, so a program walking a class's
	// MRO - pkg_resources' _find_adapter does exactly that - could not.
	typeMeta("__mro__", "The method resolution order of the type.", func(t *Type) Object {
		// A type declared by a stdlib module is queued rather than readied at
		// init time, so the queue is drained here before the MRO is read - and
		// the single-parent fallback below is never the answer when a
		// grandparent exists.
		if len(t.Mro) == 0 {
			_ = TypeEnsureReady()
		}
		if len(t.Mro) != 0 {
			return t.Mro
		}
		// A type whose MRO was never built still has itself in it.
		return Tuple{Object(t)}
	})
	typeMeta("__dict__", "The namespace of the type.", func(t *Type) Object { return t.Dict })

	// __class__ is a descriptor registered on both metatypes for the same
	// reason as the metadata above: an instance of a builtin must be able to
	// ask for its class, and "(5).__class__" was an AttributeError.
	ObjectType.Dict.Set("__class__", &classGetter{})
	TypeType.Dict.Set("__class__", &classGetter{})

	// object.__init__ exists and takes no arguments, which is what makes
	// "super().__init__()" work in a class whose base is a native type - such
	// as urllib3's HTTPConnection, which chains to http.client.HTTPConnection.
	// It is registered on ObjectType.Dict, the "object" class's namespace, so
	// every type finds it through its base chain when it does not define one
	// of its own.  It is deliberately permissive about arguments: this
	// interpreter calls the MRO's __init__ on every construction, so rejecting
	// them here would break "class C: pass" when C is instantiated with any
	// argument the base chain has already consumed.
	// object.__init_subclass__ is the no-op every class inherits; a base that
	// defines its own overrides it, and the class statement's keywords reach
	// whichever one is found.  It was absent entirely, so
	// "class Bad(Plain, nope=1)" silently accepted a keyword CPython rejects.
	ObjectType.Dict.Set("__init_subclass__", MustNewMethod("__init_subclass__", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		// The first argument is the class being CREATED - that is what this
		// hook is called with - so the message names it, as CPython's does.
		if kwargs.Len() > 0 {
			name := ""
			if len(args) > 0 {
				if t, ok := args[0].(*Type); ok {
					name = t.Name + "."
				}
			}
			return nil, ExceptionNewf(TypeError, "%s__init_subclass__() takes no keyword arguments", name)
		}
		return None, nil
	}, 0, "This method is called when a class is subclassed."))

	ObjectType.Dict.Set("__init__", MustNewMethod("__init__", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return None, nil
	}, 0, "Initialize self.  See help(type(self)) for accurate signature."))

	// __new__ is registered on the OBJECT type's namespace, so every class
	// inherits a callable one.  "tuple.__new__(cls, args)" is how a Python
	// subclass of a builtin constructs itself - pygments' Token type does
	// exactly that - and without a native __new__ to call, its __new__ raised
	// "'object' has no attribute '__new__'" and the class could not be built.
	//
	// The descriptor form is what makes it useful: "tuple.__new__(T, ...)" must
	// build a T, and only the owner knows that.
	ObjectType.Dict.Set("__new__", &nativeNew{})

	// A type is hashable by identity, as in CPython: "{str: 1, bytes: 2}" is
	// ordinary code (requests builds HEADER_VALIDATORS that way).  Without a
	// __hash__ the dict key encoder rejected every type as "unhashable type:
	// 'object'".  The hash is the type's own pointer, and it is registered on
	// both metatypes (a native type is an ObjectType, a python class a
	// TypeType) - but NOT on a plain instance, which keeps lists and dicts
	// unhashable.
	typeHash := MustNewMethod("__hash__", func(self Object, args Tuple) (Object, error) {
		t, ok := self.(*Type)
		if !ok {
			return nil, ExceptionNewf(TypeError, "descriptor '__hash__' requires a 'type' object")
		}
		return Int(int64(reflect.ValueOf(t).Pointer())), nil
	}, 0, "Return hash(self).")
	TypeType.Dict.Set("__hash__", typeHash)
	ObjectType.Dict.Set("__hash__", typeHash)
}

// typeMetaGetter is the descriptor behind cls.__name__ and friends.
//
// It is deliberately NOT a property: reading a property on a class yields
// the property object, but reading one of these yields the answer, so that
// "object.__name__" is the string "object" and not a descriptor.
//
// The subject is the instance when that is a type - "int.__name__" arrives
// as (int, type) - and otherwise the owner, which is how "object.__name__"
// arrives, since object's attribute is found on object itself.
type typeMetaGetter struct {
	name string
	get  func(t *Type) Object
}

func (d *typeMetaGetter) Type() *Type { return ObjectType }

func (d *typeMetaGetter) M__get__(instance, owner Object) (Object, error) {
	if t, ok := instance.(*Type); ok {
		return d.get(t), nil
	}
	if instance == nil || instance == None {
		if t, ok := owner.(*Type); ok {
			return d.get(t), nil
		}
	}
	// An instance of a class that inherits from object - "(5).__name__".
	return nil, ExceptionNewf(AttributeError, "'%s' object has no attribute '%s'", instance.Type().Name, d.name)
}

var _ I__get__ = (*typeMetaGetter)(nil)

// classGetter implements __class__.
//
// For an instance it is the class the instance was made from, so
// "(5).__class__" is int.  For a type it is the metatype, so
// "int.__class__" is type - and the lookup for a type arrives with the
// metatype as the owner, which is what makes that answer.
type classGetter struct{}

func (d *classGetter) Type() *Type { return ObjectType }

func (d *classGetter) M__get__(instance, owner Object) (Object, error) {
	if _, ok := instance.(*Type); ok {
		if t, ok := owner.(*Type); ok {
			return t, nil
		}
	}
	if instance == nil || instance == None {
		return owner, nil
	}
	return instance.Type(), nil
}

var _ I__get__ = (*classGetter)(nil)

// Make a new type from a name
//
// For making Go types
func NewType(Name string, Doc string) *Type {
	t := &Type{
		ObjectType: TypeType,
		Name:       Name,
		Doc:        Doc,
		Dict:       NewStringDict(),
		// A type created this way models a PYTHON-level class, and in CPython
		// a python-level class accepts subclasses.  Without the flag,
		// "class MyLogger(logging.Logger)" raised "type 'logging.Logger' is
		// not an acceptable base type" - which is how pip._internal failed to
		// import, since it derives a Logger to add its own levels.
		Flags: TPFLAGS_BASETYPE,
	}
	TypeDelayReady(t)
	return t
}

// Make a new type with constructors
//
// For making Go types
func NewTypeX(Name string, Doc string, New NewFunc, Init InitFunc) *Type {
	t := &Type{
		ObjectType: TypeType,
		Name:       Name,
		Doc:        Doc,
		New:        New,
		Init:       Init,
		Dict:       NewStringDict(),
		// Subclassable, for the same reason NewType is: a type made this way
		// models a python-level class, and CPython lets those be derived from.
		//
		// This was missing, so every type built with NewTypeX was refused as a
		// base - "type 'typing._Final' is not an acceptable base type" stopped
		// typing_extensions, which opens with
		// "class _SpecialForm(typing._Final, _root=True)".  A native type that
		// genuinely cannot be subclassed (list, set) opts out with NoSubclass,
		// which is the one place that decides; the flag must not be the
		// default refusal.
		Flags: TPFLAGS_BASETYPE,
	}
	TypeDelayReady(t)
	return t
}

// Make a subclass of a type
//
// For making Go types
func (t *Type) NewTypeFlags(Name string, Doc string, New NewFunc, Init InitFunc, Flags uint) *Type {
	// inherit constructors
	if New == nil {
		New = t.New
	}
	if Init == nil {
		Init = t.Init
	}
	// FIXME inherit more stuff
	//
	// TPFLAGS_READY and TPFLAGS_READYING must NOT be inherited: they describe
	// whether *this* type has been readied, and Ready() returns immediately when
	// READY is set.  Inheriting READY from the base left every NewType subclass
	// with an empty Mro, so an except clause naming a grandparent of the raised
	// class did not match - binascii.Error derives from ValueError, and
	// 'except Exception' has to catch it.
	Flags &^= TPFLAGS_READY | TPFLAGS_READYING
	tt := &Type{
		ObjectType: t,
		Name:       Name,
		Doc:        Doc,
		New:        New,
		Init:       Init,
		Flags:      Flags,
		Dict:       NewStringDict(),
		Bases:      Tuple{t},
	}
	TypeDelayReady(tt)
	return tt
}

// Make a subclass of a type
//
// For making Go types
func (t *Type) NewType(Name string, Doc string, New NewFunc, Init InitFunc) *Type {
	// Inherit flags from superclass
	// FIXME not sure this is correct!
	return t.NewTypeFlags(Name, Doc, New, Init, t.Flags)
}

// Determine the most derived metatype.
func (metatype *Type) CalculateMetaclass(bases Tuple) (*Type, error) {
	// Determine the proper metatype to deal with this,
	// and check for metatype conflicts while we're at it.
	// Note that if some other metatype wins to contract,
	// it's possible that its instances are not types. */

	winner := metatype
	for _, tmp := range bases {
		tmptype := tmp.Type()
		if winner.IsSubtype(tmptype) {
			continue
		}
		if tmptype.IsSubtype(winner) {
			winner = tmptype
			continue
		}
		// A base whose metatype is NOT a metaclass imposes no constraint.
		//
		// This interpreter does not model metaclasses faithfully: a builtin
		// type's metatype is ObjectType, and a NewType subclass carries its
		// BASE as its metatype, so Exception's is BaseException.  Neither is a
		// subtype of type, so treating them as competing metaclasses rejected
		// every ordinary class statement that inherited a builtin - "class
		// MyErr(Exception): pass" failed with "metaclass conflict: the
		// metaclass of a derived class must be a (non-strict) subclass of the
		// metaclasses of all its bases".
		//
		// A type with no real metaclass is just a class; only a genuine
		// metaclass (a subtype of type) can win or conflict.
		if !tmptype.IsSubtype(TypeType) {
			continue
		}
		if !winner.IsSubtype(TypeType) {
			winner = tmptype
			continue
		}
		// else:
		return nil, ExceptionNewf(TypeError, "metaclass conflict: the metaclass of a derived class must be a (non-strict) subclass of the metaclasses of all its bases")
	}
	return winner, nil
}

// type test with subclassing support
// reads a IsSubtype of b
func (a *Type) IsSubtype(b *Type) bool {
	mro := a.Mro
	if len(mro) == 0 {
		// A type whose Mro is still empty has not been readied.  TypeMakeReady
		// runs once, during this package's init, so a type declared by any
		// later-initialising package - every stdlib module - is queued but
		// never readied.  Drain that queue now so the MRO is available; the
		// fallback below walks Base only and would miss a grandparent.
		if err := TypeEnsureReady(); err == nil {
			mro = a.Mro
		}
	}
	if len(mro) != 0 {
		// Deal with multiple inheritance without recursion
		// by walking the MRO tuple
		for _, baseObj := range mro {
			base := baseObj.(*Type)
			if base == b {
				return true
			}
		}
		return false
	} else {
		// a is not completely initilized yet; follow tp_base
		for {
			if a == b {
				return true
			}
			a = a.Base
			if a == nil {
				break
			}
		}
		return b == ObjectType
	}
}

// Call type()
// nativeNew is the __new__ of a type whose constructor is implemented in Go.
//
// It exists so that "tuple.__new__(cls, args)" works, which is how a Python
// subclass of a builtin constructs its own instance.  It is a DESCRIPTOR: the
// class it is reached through supplies the type, so "tuple.__new__(T, ...)"
// builds a T rather than a tuple.
type nativeNew struct{}

func (n *nativeNew) Type() *Type { return TypeType }

func (n *nativeNew) M__get__(instance, owner Object) (Object, error) { return n, nil }

func (n *nativeNew) M__call__(args Tuple, kwargs StringDict) (Object, error) {
	if len(args) == 0 {
		return nil, ExceptionNewf(TypeError, "__new__() takes at least 1 argument")
	}
	t, ok := args[0].(*Type)
	if !ok {
		return nil, ExceptionNewf(TypeError, "__new__() requires a type as its first argument")
	}
	if t.New == nil {
		return nil, ExceptionNewf(TypeError, "cannot create '%s' instances", t.Name)
	}
	return t.New(t, args[1:], kwargs)
}

// payloadContainers are the builtin containers whose whole content can live in
// an instance's Payload.
//
// A dict subclass is NOT among them: an instance's Dict already IS the mapping,
// and the dict machinery reaches it through dictStorage, which is the
// arrangement that already makes "class D(dict)" work.  A set, list and tuple
// have positional contents and nothing to read them from, so their subclass
// instances read the Payload.
var payloadContainers = []*Type{TupleType, ListType, SetType}

// IsClassObject reports whether an object is a CLASS rather than an instance.
//
// This interpreter represents a python-level instance as a *Type with an empty
// Name, so "obj.(*Type)" is TRUE for ordinary instances - which made
// inspect.isclass(x) true for every python-level object, and rich's render
// therefore refused to call x.__rich_console__ and reported that a Text was
// "not renderable".  The Name is what tells the two apart.
func IsClassObject(obj Object) bool {
	t, ok := obj.(*Type)
	return ok && t.Name != ""
}

// isUserContainerSubclass reports whether t is a class the program declared that
// derives from a builtin container, and value is that container's own value.
func isUserContainerSubclass(t *Type, value Object) bool {
	if t == nil || t.Name == "" {
		// A class always has a name; an instance does not.
		return false
	}
	// Do not wrap a value that is ALREADY an instance of t - a constructor that
	// honoured its class has nothing left to do.
	if value.Type() == t {
		return false
	}
	ot := value.Type()
	for _, c := range payloadContainers {
		if ot == c && t.IsSubtype(c) {
			return true
		}
	}
	return false
}

// unwrapPayload returns the container behind a value, so that wrapping a value
// that already carries a payload does not nest two of them.
func unwrapPayload(o Object) Object {
	if p, ok := payloadOf(o); ok {
		return p
	}
	return o
}

// LookupPython returns the named attribute when it is a PYTHON-defined
// function - a *Function - walking this type's MRO.  nil otherwise.
//
// It exists so that a Python override such as __new__ can be told apart from a
// native one, which is a Go method that the native constructor already calls.
func (t *Type) LookupPython(name string) Object {
	if v := t.Lookup(name); v != nil {
		if _, ok := v.(*Function); ok {
			return v
		}
	}
	return nil
}

func (t *Type) M__call__(args Tuple, kwargs StringDict) (Object, error) {
	if t.New == nil {
		return nil, ExceptionNewf(TypeError, "cannot create '%s' instances", t.Name)
	}

	// A __new__ DEFINED IN PYTHON governs construction.  Without this the
	// inherited Go constructor ran instead and the override was ignored
	// entirely: "class T(tuple): def __new__(cls, *a): ..." produced a plain
	// tuple, so "T()" was not a T at all and every attribute it declares raised
	// AttributeError.  pygments' Token type is built that way, so pip could not
	// import pygments.
	//
	// The lookup is on the class's own MRO, and only a *Function - something
	// written in Python - is honoured.  A native __new__ is the Go New the code
	// below already calls, so consulting it here would recurse.
	if t.Name != "" {
		if newFn := t.LookupPython("__new__"); newFn != nil {
			obj, err := Call(newFn, append(Tuple{t}, args...), kwargs)
			if err != nil {
				return nil, err
			}
			// __init__ runs only when the result really is an instance of this
			// class, which is the rule CPython applies.
			if !obj.Type().IsSubtype(t) {
				return obj, nil
			}
			objType := obj.Type()
			if objType.Init != nil {
				if err := objType.Init(obj, args, kwargs); err != nil {
					return nil, err
				}
			}
			return obj, nil
		}
	}

	obj, err := t.New(t, args, kwargs)
	if err != nil {
		return nil, err
	}
	// A Python subclass of a BUILTIN CONTAINER carries the value its native
	// constructor produced.  Without this the value was of the BASE type:
	// "class _TokenType(tuple); _TokenType()" was a plain tuple, so "Token.Text"
	// raised AttributeError and pygments' token table could not be built.
	//
	// The condition is deliberately narrow.  It fires only when the value's type
	// is a builtin container this class DERIVES FROM, and the class is one the
	// program declared - so a native constructor that honoured its argument (a
	// dict subclass, say) is untouched, and a __new__ returning something else
	// entirely is left alone, as CPython does.
	if isUserContainerSubclass(t, obj) {
		obj = &Type{
			ObjectType: t,
			Base:       t,
			Dict:       NewStringDict(),
			Payload:    unwrapPayload(obj),
		}
	}
	// Ugly exception: when the call was type(something),
	// don't call tp_init on the result.
	if t == TypeType && len(args) == 1 && kwargs.Len() == 0 {
		return obj, nil
	}
	// If the returned object is not an instance of type,
	// it won't be initialized.
	if !obj.Type().IsSubtype(t) {
		return obj, nil
	}
	objType := obj.Type()
	if objType.Init != nil {
		err = objType.Init(obj, args, kwargs)
		if err != nil {
			return nil, err
		}
	}
	return obj, nil
}

// Internal API to look for a name through the MRO.
// This returns a borrowed reference, and doesn't set an exception,
// returning nil instead
func (t *Type) Lookup(name string) Object {
	// Py_ssize_t i, n;
	// PyObject *mro, *res, *base, *dict;
	// unsigned int h;

	// FIXME caching
	// if (MCACHE_CACHEABLE_NAME(name) &&
	//     PyType_HasFeature(type, TPFLAGS_VALID_VERSION_TAG)) {
	//     // fast path
	//     h = MCACHE_HASH_METHOD(type, name);
	//     if (method_cache[h].version == type->tp_version_tag &&
	//         method_cache[h].name == name)
	//         return method_cache[h].value;
	// }

	// Look in tp_dict of types in MRO
	mro := t.Mro

	// If mro is nil, the type is either not yet initialized
	// by PyType_Ready(), or already cleared by type_clear().
	if mro == nil {
		// TypeMakeReady runs once, during THIS package's init, so a type
		// declared by any package that initialises later - which is every
		// stdlib module - is queued in delayedReady and would never be
		// readied.  Its Mro stays empty, and the walk below would find
		// nothing in its bases: "threading.local.__name__" and
		// "threading.local.__bases__" raised AttributeError while a class
		// statement in the same process worked.  Drain the queue first.
		//
		// The call goes through a variable because Go's initialisation
		// analysis otherwise reports a cycle: draining the queue reaches the
		// type constructors, and those reach this file.
		if ensureReadyHook != nil {
			ensureReadyHook()
			mro = t.Mro
		}
	}
	if mro == nil {
		// Still not readied - the fallback is the type's own Dict, which is
		// what lets "Generic[int]" work: Generic's __class_getitem__ lives
		// there and the type has no MRO.
		if res, ok := t.Dict.Get(name); ok {
			return res
		}
		if base := t.Base; base != nil {
			return base.Lookup(name)
		}
		return nil
	}

	var res Object
	// keep a strong reference to mro because type->tp_mro can be replaced
	// during PyDict_GetItem(dict, name)
	for _, baseObj := range mro {
		base := baseObj.(*Type)
		var ok bool
		res, ok = base.Dict.Get(name)
		if ok {
			break
		}
	}

	// FIXME caching
	// if (MCACHE_CACHEABLE_NAME(name) && assign_version_tag(type)) {
	//     h = MCACHE_HASH_METHOD(type, name);
	//     method_cache[h].version = type->tp_version_tag;
	//     method_cache[h].value = res;  /* borrowed */
	//     Py_INCREF(name);
	//     Py_DECREF(method_cache[h].name);
	//     method_cache[h].name = name;
	// }

	return res
}

// Get an attribute from the type of a go type
//
// Doesn't call __getattr__ etc
//
// # Returns nil if not found
//
// Doesn't look in the instance dictionary
//
// FIXME this isn't totally correct!
// as we are ignoring getattribute etc
// See _PyObject_GenericGetAttrWithDict in object.c
func (t *Type) NativeGetAttrOrNil(name string) Object {
	// Look in type Dict
	if res, ok := t.Dict.Get(name); ok {
		return res
	}
	// Now look through base classes etc
	return t.Lookup(name)
}

// Get an attribute from the type
//
// Doesn't call __getattr__ etc
//
// # Returns nil if not found
//
// FIXME this isn't totally correct!
// as we are ignoring getattribute etc
// See _PyObject_GenericGetAttrWithDict in object.c
func (t *Type) GetAttrOrNil(name string) Object {
	// Look in instance dictionary first
	if res, ok := t.Dict.Get(name); ok {
		return res
	}
	// Then look in type Dict
	if res, ok := t.Type().Dict.Get(name); ok {
		return res
	}
	// Now look through base classes etc
	return t.Lookup(name)
}

// Calls method on name
//
// If method not found returns (nil, false, nil)
//
// If method found returns (object, true, err)
//
// May raise exceptions if calling the method failed
func (t *Type) CallMethod(name string, args Tuple, kwargs StringDict) (Object, bool, error) {
	// Look up the method through the MRO, not only in this type's own Dict.
	//
	// GetAttrOrNil consults the type's Dict and then its METATYPE, which is
	// right for an instance's attributes but wrong here: a SUBCLASS inherits
	// its base's methods, and they live in the base's Dict.  So
	// "class D(dict): pass; d["a"] = 1" raised "'D' object does not support
	// item assignment" - the inherited __setitem__ was never found - and the
	// same would have hit any operator inherited from a builtin.
	fn := t.GetAttrOrNil(name)
	if own := t.Lookup(name); own != nil {
		fn = own
	}
	if fn == nil {
		return nil, false, nil
	}
	// A method registered in the type's Dict has to be given its self
	// explicitly: py.Call would treat the first argument as an ordinary
	// positional one, and the method would receive the wrong object (or
	// none) as self.  This is the path that makes len(), repr(), 'in' and
	// subscripting work on the instances of a natively defined type.
	if m, ok := fn.(*Method); ok && len(args) > 0 {
		res, err := m.Call(args[0], args[1:])
		return res, true, err
	}
	// A python function defined in the class body is bound the same way:
	// going through __get__ supplies self.  Without this the function was
	// called with the arguments shifted up by one, so "g[3]" on a class
	// defining __getitem__ returned the instance rather than calling the
	// method, and "__call__" was never found at all.
	if len(args) > 0 {
		if d, ok := fn.(I__get__); ok {
			bound, err := d.M__get__(args[0], t)
			if err != nil {
				return nil, true, err
			}
			// A plain function and a staticmethod both decline to bind, and
			// answer with themselves - in which case self is NOT passed.
			if bound != nil && bound != fn {
				res, err := Call(bound, args[1:], kwargs)
				return res, true, err
			}
			res, err := Call(fn, args[1:], kwargs)
			return res, true, err
		}
	}
	res, err := Call(fn, args, kwargs)
	return res, true, err
}

// Calls a type method on obj
//
// If the method isn't found on the object returns (nil, false, nil)
//
// Otherwise returns (object, true, err)
//
// May raise exceptions if calling the method fails
func TypeCall(self Object, name string, args Tuple, kwargs StringDict) (Object, bool, error) {
	// Any object answers for itself through its class.  Handling only a
	// *Type here is why "g[3]" returned the instance itself for a class that
	// defines __getitem__: the lookup reported "not found", so GetItem fell
	// through to its error path rather than calling the method.
	t, ok := self.(*Type)
	if !ok {
		t = self.Type()
	}
	return t.CallMethod(name, args, kwargs)
}

// Calls TypeCall with 0 arguments
func TypeCall0(self Object, name string) (Object, bool, error) {
	return TypeCall(self, name, Tuple{self}, NewStringDict())
}

// Calls TypeCall with 1 argument
func TypeCall1(self Object, name string, arg Object) (Object, bool, error) {
	return TypeCall(self, name, Tuple{self, arg}, NewStringDict())
}

// Calls TypeCall with 2 arguments
func TypeCall2(self Object, name string, arg1, arg2 Object) (Object, bool, error) {
	return TypeCall(self, name, Tuple{self, arg1, arg2}, NewStringDict())
}

// Internal routines to do a method lookup in the type
// without looking in the instance dictionary
// (so we can't use PyObject_GetAttr) but still binding
// it to the instance.  The arguments are the object,
// the method name as a C string, and the address of a
// static variable used to cache the interned Python string.
//
// Two variants:
//
// - lookup_maybe() returns nil without raising an exception
//
//	when the _PyType_Lookup() call fails;
//
// - lookup_method() always raises an exception upon errors.
func lookup_maybe(self Object, attr string) Object {
	res := self.Type().Lookup(attr)
	// FIXME descriptor lookup
	// if (res != nil) {
	// 	descrgetfunc f;
	// 	if ((f = Py_TYPE(res)->tp_descr_get) == nil) {
	// 		Py_INCREF(res);
	// 	}else{
	// 		res = f(res, self, (PyObject *)(Py_TYPE(self)));
	// 	}
	// }
	return res
}

// func lookup_method(self Object, attr string) Object {
// 	res := lookup_maybe(self, attr)
// 	if res == nil {
// 		// FIXME PyErr_SetObject(PyExc_AttributeError, attrid->object);
// 		return ExceptionNewf(AttributeError, "'%s' object has no attribute '%s'", self.Type().Name, attr)
// 	}
// 	return res
// }

// Method resolution order algorithm C3 described in
// "A Monotonic Superclass Linearization for Dylan",
// by Kim Barrett, Bob Cassel, Paul Haahr,
// David A. Moon, Keith Playford, and P. Tucker Withington.
// (OOPSLA 1996)
//
// Some notes about the rules implied by C3:
//
// No duplicate bases.
// It isn't legal to repeat a class in a list of base classes.
//
// The next three properties are the 3 constraints in "C3".
//
// Local precendece order.
// If A precedes B in C's MRO, then A will precede B in the MRO of all
// subclasses of C.
//
// Monotonicity.
// The MRO of a class must be an extension without reordering of the
// MRO of each of its superclasses.
//
// Extended Precedence Graph (EPG).
// Linearization is consistent if there is a path in the EPG from
// each class to all its successors in the linearization.  See
// the paper for definition of EPG.

func tail_contains(list *List, whence int, o Object) bool {
	for j := whence + 1; j < len(list.Items); j++ {
		if list.Items[j] == o {
			return true
		}
	}
	return false
}

func class_name(cls Object) string {
	name := ObjectGetAttr(cls, "__name__")
	if name == nil {
		name = ObjectRepr(cls)
	}
	nameString, ok := name.(String)
	if !ok {
		return ""
	}
	return string(nameString)
}

func check_duplicates(list *List) error {
	// Let's use a quadratic time algorithm,
	// assuming that the bases lists is short.
	for i := range list.Items {
		o := list.Items[i]
		for j := i + 1; j < len(list.Items); j++ {
			if list.Items[j] == o {
				return ExceptionNewf(TypeError, "duplicate base class %s", class_name(o))
			}
		}
	}
	return nil
}

// Raise a TypeError for an MRO order disagreement.
//
// It's hard to produce a good error message.  In the absence of better
// insight into error reporting, report the classes that were candidates
// to be put next into the MRO.  There is some conflict between the
// order in which they should be put in the MRO, but it's hard to
// diagnose what constraint can't be satisfied.
func set_mro_error(to_merge *List, remain []int) error {
	return ExceptionNewf(TypeError, "mro is wonky")
	/* FIXME implement this!
	       Py_ssize_t i, n, off, to_merge_size;
	       char buf[1000];
	       PyObject *k, *v;
	       PyObject *set = PyDict_New();
	       if (!set) return;

	       to_merge_size = PyList_GET_SIZE(to_merge);
	       for (i = 0; i < to_merge_size; i++) {
	           PyObject *L = PyList_GET_ITEM(to_merge, i);
	           if (remain[i] < PyList_GET_SIZE(L)) {
	               PyObject *c = PyList_GET_ITEM(L, remain[i]);
	               if (PyDict_SetItem(set, c, Py_None) < 0) {
	                   Py_DECREF(set);
	                   return;
	               }
	           }
	       }
	       n = PyDict_Size(set);

	       off = PyOS_snprintf(buf, sizeof(buf), "Cannot create a \
	   consistent method resolution\norder (MRO) for bases");
	       i = 0;
	       while (PyDict_Next(set, &i, &k, &v) && (size_t)off < sizeof(buf)) {
	           PyObject *name = class_name(k);
	           char *name_str;
	           if (name != nil) {
	               name_str = _PyUnicode_AsString(name);
	               if (name_str == nil)
	                   name_str = "?";
	           } else
	               name_str = "?";
	           off += PyOS_snprintf(buf + off, sizeof(buf) - off, " %s", name_str);
	           Py_XDECREF(name);
	           if (--n && (size_t)(off+1) < sizeof(buf)) {
	               buf[off++] = ',';
	               buf[off] = '\0';
	           }
	       }
	       PyErr_SetString(PyExc_TypeError, buf);
	       Py_DECREF(set);
	*/
}

func pmerge(acc, to_merge *List) error {
	// Py_ssize_t i, j, to_merge_size, empty_cnt;
	// int *remain;
	// int ok;

	to_merge_size := len(to_merge.Items)

	// remain stores an index into each sublist of to_merge.
	// remain[i] is the index of the next base in to_merge[i]
	// that is not included in acc.
	remain := make([]int, to_merge_size)

again:
	empty_cnt := 0
	for i := 0; i < to_merge_size; i++ {
		cur_list := to_merge.Items[i].(*List)

		if remain[i] >= len(cur_list.Items) {
			empty_cnt++
			continue
		}

		// Choose next candidate for MRO.
		//
		// The input sequences alone can determine the choice.
		// If not, choose the class which appears in the MRO
		// of the earliest direct superclass of the new class.

		candidate := cur_list.Items[remain[i]]
		for j := 0; j < to_merge_size; j++ {
			j_lst := to_merge.Items[j].(*List)
			if tail_contains(j_lst, remain[j], candidate) {
				goto skip // continue outer loop
			}
		}
		acc.Append(candidate)
		for j := 0; j < to_merge_size; j++ {
			j_lst := to_merge.Items[j].(*List)
			if remain[j] < len(j_lst.Items) && j_lst.Items[remain[j]] == candidate {
				remain[j]++
			}
		}
		goto again
	skip:
	}

	if empty_cnt == to_merge_size {
		return nil
	}
	return set_mro_error(to_merge, remain)
}

func (t *Type) mro_implementation() (Object, error) {
	// Py_ssize_t i, n;
	// int ok;
	// PyObject *bases, *result;
	// PyObject *to_merge, *bases_aslist;
	var err error

	if t.Dict.IsNil() {
		err = t.Ready()
		if err != nil {
			return nil, err
		}
	}

	// Find a superclass linearization that honors the constraints
	// of the explicit lists of bases and the constraints implied by
	// each base class.
	//
	// to_merge is a list of lists, where each list is a superclass
	// linearization implied by a base class.  The last element of
	// to_merge is the declared list of bases.

	bases := t.Bases
	n := len(bases)
	to_merge := NewListSized(n + 1)

	for i := range bases {
		base := bases[i].(*Type)
		parentMRO, err := SequenceList(base.Mro)
		if err != nil {
			return nil, err
		}
		to_merge.Items[i] = parentMRO
	}

	bases_aslist, err := SequenceList(bases)
	if err != nil {
		return nil, err
	}

	// This is just a basic sanity check.
	err = check_duplicates(bases_aslist)
	if err != nil {
		return nil, err
	}

	to_merge.Items[n] = bases_aslist

	result := NewListFromItems([]Object{t})

	err = pmerge(result, to_merge)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (t *Type) mro_internal() (err error) {
	// PyObject *mro, *result, *tuple;
	var result Object
	checkit := false

	if t == TypeType {
		result, err = t.mro_implementation()
		if err != nil {
			return err
		}
	} else {
		checkit = true
		// FIXME this is what it was originally
		// but we haven't put mro in slots or anything
		// mro := lookup_method(t, "mro")
		mro := lookup_maybe(t, "mro")
		if mro == nil {
			// Default to internal implementation
			result, err = t.mro_implementation()
			if err != nil {
				return err
			}
		} else {
			result, err = Call(mro, nil, NewStringDict())
			if err != nil {
				return err
			}
		}
	}
	tuple, err := SequenceTuple(result)
	if err != nil {
		return err
	}
	if checkit {
		// Py_ssize_t i, len;
		// PyObject *cls;
		// PyTypeObject *solid;

		solid := t.solid_base()

		for i := range tuple {
			cls := tuple[i]
			t, ok := cls.(*Type)
			if !ok {
				return ExceptionNewf(TypeError, "mro() returned a non-class ('%s')", cls.Type().Name)
			}
			if !solid.IsSubtype(t.solid_base()) {
				return ExceptionNewf(TypeError, "mro() returned base with unsuitable layout ('%.500s')", cls.Type().Name)
			}
		}
	}
	t.Mro = tuple

	// FIXME t.type_mro_modified(t.Mro)
	// corner case: the super class might have been hidden
	// from the custom MRO
	// FIXME t.type_mro_modified(t.Bases)

	// FIXME t.Modified()
	return nil
}

func (t *Type) inherit_special(base *Type) {
	//     /* Copying basicsize is connected to the GC flags */
	//     if (!(type->tp_flags & TPFLAGS_HAVE_GC) &&
	//         (base->tp_flags & TPFLAGS_HAVE_GC) &&
	//         (!type->tp_traverse && !type->tp_clear)) {
	//         type->tp_flags |= TPFLAGS_HAVE_GC;
	//         if (type->tp_traverse == nil)
	//             type->tp_traverse = base->tp_traverse;
	//         if (type->tp_clear == nil)
	//             type->tp_clear = base->tp_clear;
	//     }
	//     {
	//         /* The condition below could use some explanation.
	//            It appears that tp_new is not inherited for static types
	//            whose base class is 'object'; this seems to be a precaution
	//            so that old extension types don't suddenly become
	//            callable (object.__new__ wouldn't insure the invariants
	//            that the extension type's own factory function ensures).
	//            Heap types, of course, are under our control, so they do
	//            inherit tp_new; static extension types that specify some
	//            other built-in type as the default also
	//            inherit object.__new__. */
	//         if (base != &PyBaseObject_Type ||
	//             (type->tp_flags & TPFLAGS_HEAPTYPE)) {
	//             if (type->tp_new == nil)
	//                 type->tp_new = base->tp_new;
	//         }
	//     }
	//     if (type->tp_basicsize == 0)
	//         type->tp_basicsize = base->tp_basicsize;

	//     /* Copy other non-function slots */

	// #undef COPYVAL
	// #define COPYVAL(SLOT) \
	//     if (type->SLOT == 0) type->SLOT = base->SLOT

	//     COPYVAL(tp_itemsize);
	//     COPYVAL(tp_weaklistoffset);
	//     COPYVAL(tp_dictoffset);

	// Setup fast subclass flags
	switch {
	case base.IsSubtype(BaseException):
		t.Flags |= TPFLAGS_BASE_EXC_SUBCLASS
	case base.IsSubtype(TypeType):
		t.Flags |= TPFLAGS_TYPE_SUBCLASS
	case base.IsSubtype(IntType):
		t.Flags |= TPFLAGS_LONG_SUBCLASS
	case base.IsSubtype(BigIntType):
		t.Flags |= TPFLAGS_LONG_SUBCLASS
	case base.IsSubtype(BytesType):
		t.Flags |= TPFLAGS_BYTES_SUBCLASS
	case base.IsSubtype(StringType):
		t.Flags |= TPFLAGS_UNICODE_SUBCLASS
	case base.IsSubtype(TupleType):
		t.Flags |= TPFLAGS_TUPLE_SUBCLASS
	case base.IsSubtype(ListType):
		t.Flags |= TPFLAGS_LIST_SUBCLASS
	case base.IsSubtype(DictType):
		t.Flags |= TPFLAGS_DICT_SUBCLASS
	}

}

func add_subclass(base, t *Type) {
	// Py_ssize_t i;
	// int result;
	// PyObject *list, *ref, *newobj;

	// list = base->tp_subclasses;
	// if (list == nil) {
	//     base->tp_subclasses = list = PyList_New(0);
	//     if (list == nil)
	//         return -1;
	// }
	// assert(PyList_Check(list));
	// newobj = PyWeakref_NewRef((PyObject *)type, nil);
	// i = PyList_GET_SIZE(list);
	// while (--i >= 0) {
	//     ref = PyList_GET_ITEM(list, i);
	//     assert(PyWeakref_CheckRef(ref));
	//     if (PyWeakref_GET_OBJECT(ref) == Py_None)
	//         return PyList_SetItem(list, i, newobj);
	// }
	// result = PyList_Append(list, newobj);
	// Py_DECREF(newobj);
	// return result;
}

// func remove_subclass(base, t *Type) {
// 	// Py_ssize_t i;
// 	// PyObject *list, *ref;
//
// 	// list = base->tp_subclasses;
// 	// if (list == nil) {
// 	//     return;
// 	// }
// 	// assert(PyList_Check(list));
// 	// i = PyList_GET_SIZE(list);
// 	// while (--i >= 0) {
// 	//     ref = PyList_GET_ITEM(list, i);
// 	//     assert(PyWeakref_CheckRef(ref));
// 	//     if (PyWeakref_GET_OBJECT(ref) == (PyObject*)type) {
// 	//         /* this can't fail, right? */
// 	//         PySequence_DelItem(list, i);
// 	//         return;
// 	//     }
// 	// }
// }

// Ready the type for use
//
// Returns an error on problems
func (t *Type) Ready() error {
	// PyObject *dict, *bases;
	// PyTypeObject *base;
	// Py_ssize_t i, n;
	var err error

	if t.Flags&TPFLAGS_READY != 0 {
		if t.Dict.IsNil() {
			return ExceptionNewf(SystemError, "Type.Ready is Ready but Dict is nil")
		}
		return nil
	}
	if t.Flags&TPFLAGS_READYING != 0 {
		return ExceptionNewf(SystemError, "Type.Ready already readying")
	}

	t.Flags |= TPFLAGS_READYING

	// Initialize tp_base (defaults to BaseObject unless that's us)
	base := t.Base
	if base == nil && t != ObjectType {
		base = ObjectType
		t.Base = base
	}

	// Now the only way base can still be nil is if type is
	// ObjectType.

	// Initialize the base class
	if base != nil && base.Dict.IsNil() {
		err = base.Ready()
		if err != nil {
			return err
		}
	}

	// Initialize ob_type if nil.      This means extensions that want to be
	// compilable separately on Windows can call PyType_Ready() instead of
	// initializing the ob_type field of their type objects.
	// The test for base != nil is really unnecessary, since base is only
	// nil when type is ObjectType, and we know its ob_type is
	// not nil (it's initialized to &PyType_Type).      But coverity doesn't
	// know that.

	// FIXME - this can't work with the current Type scheme
	// if t.Type() == nil && base != nil {
	// 	Py_TYPE(t) = Py_TYPE(base)
	// }

	// Initialize tp_bases
	bases := t.Bases
	if bases == nil {
		if base == nil {
			bases = Tuple{}
		} else {
			bases = Tuple{base}
		}
		t.Bases = bases
	}

	// Initialize tp_dict
	dict := t.Dict
	if dict.IsNil() {
		dict = NewStringDict()
		t.Dict = dict
	}

	// Add type-specific descriptors to tp_dict
	// FIXME not doing this
	// if add_operators(t) < 0 {
	// 	goto error
	// }
	// if t.Methods != nil {
	// 	if add_methods(t, t.Methods) < 0 {
	// 		goto error
	// 	}
	// }
	// if t.Members != nil {
	// 	if add_members(t, t.Members) < 0 {
	// 		goto error
	// 	}
	// }
	// if t.Getset != nil {
	// 	if add_getset(t, t.Getset) < 0 {
	// 		goto error
	// 	}
	// }

	// Calculate method resolution order
	err = t.mro_internal()
	if err != nil {
		return err
	}

	// Inherit special flags from dominant base
	if t.Base != nil {
		t.inherit_special(t.Base)
	}

	// A class that inherits from an exception is itself an exception, and the
	// except clause and raise both check exactly this flag.
	//
	// inherit_special above does the CPython job for a NewType subclass of an
	// exception, but a class built by a "class MyErr(Exception)" statement has
	// object as its dominant base, so it never saw the exception flag: raise
	// said "exceptions must derive from BaseException" and isinstance said
	// False.  Walking the declared bases covers that case, and runs here where
	// BaseException is certain to exist.
	if t.Flags&TPFLAGS_BASE_EXC_SUBCLASS == 0 {
		for _, b := range t.Bases {
			if bt, ok := b.(*Type); ok && bt.IsSubtype(BaseException) {
				t.Flags |= TPFLAGS_BASE_EXC_SUBCLASS
				break
			}
		}
	}

	// Initialize tp_dict properly
	bases = t.Mro
	if bases == nil {
		panic("Type.Ready: bases is nil")
	}
	// Ignore slots
	// for i := 1; i < len(bases); i++ {
	// 	b, ok := bases[i].(*Type)
	// 	if ok {
	// 		inherit_slots(t, b)
	// 	}
	// }

	// if the type dictionary doesn't contain a __doc__, set it from
	// the tp_doc slot.
	if _, ok := t.Dict.Get("__doc__"); !ok {
		if t.Doc != "" {
			t.Dict.Set("__doc__", String(t.Doc))
		} else {
			t.Dict.Set("__doc__", None)
		}
	}

	// Link into each base class's list of subclasses
	bases = t.Bases
	for i := range bases {
		b, ok := bases[i].(*Type)
		if ok {
			add_subclass(b, t)
		}
	}

	// All done -- set the ready flag
	if t.Dict.IsNil() {
		panic("Type.Ready Dict is nil")
	}
	t.Flags = (t.Flags &^ TPFLAGS_READYING) | TPFLAGS_READY
	return nil
}

func (t *Type) extra_ivars(base *Type) bool {
	return false
	/* FIXME implement this
	   	t_size := t.Basicsize;
	   	b_size := base.Basicsize;

	       assert(t_size >= b_size); // Else type smaller than base!
	       if (t.Itemsize || base.Itemsize) {
	           // If itemsize is involved, stricter rules
	           return t_size != b_size ||
	               t.Itemsize != base.Itemsize;
	       }
	       if (t.Weaklistoffset && base.Weaklistoffset == 0 &&
	           t.Weaklistoffset + sizeof(PyObject *) == t_size &&
	           t.Flags & TPFLAGS_HEAPTYPE)
	           t_size -= sizeof(PyObject *);
	       if (t.Dictoffset && base.Dictoffset == 0 &&
	           t.Dictoffset + sizeof(PyObject *) == t_size &&
	           t.Flags & TPFLAGS_HEAPTYPE)
	           t_size -= sizeof(PyObject *);

	       return t_size != b_size;
	*/
}

func (t *Type) solid_base() *Type {
	var base *Type

	if t.Base != nil {
		base = t.Base.solid_base()
	} else {
		base = ObjectType
	}
	if t.extra_ivars(base) {
		return t
	} else {
		return base
	}
}

// Calculate the best base amongst multiple base classes.
// This is the first one that's on the path to the "solid base".
func best_base(bases Tuple) (*Type, error) {
	// Py_ssize_t i, n;
	// PyTypeObject *base, *winner, *candidate, *base_i;
	// PyObject *base_proto;
	var err error

	if len(bases) == 0 {
		panic("best_base: no bases supplied")
	}
	var base *Type
	var winner *Type
	for i := range bases {
		base_i, ok := bases[i].(*Type)
		if !ok {
			return nil, ExceptionNewf(TypeError, "bases must be types")
		}
		if base_i.Dict.IsNil() {
			err = base_i.Ready()
			if err != nil {
				return nil, err
			}
		}
		candidate := base_i.solid_base()
		if winner == nil {
			winner = candidate
			base = base_i
		} else if winner.IsSubtype(candidate) {
		} else if candidate.IsSubtype(winner) {
			winner = candidate
			base = base_i
		} else {
			return nil, ExceptionNewf(TypeError, "multiple bases have instance lay-out conflict")
		}
	}
	if base == nil {
		return nil, ExceptionNewf(SystemError, "best_base: none found")
	}
	return base, nil
}

// Generic object allocator
func (t *Type) Alloc() *Type {
	// Set the type of the new object to this type
	obj := &Type{
		ObjectType: t,
		Base:       t,
		Dict:       NewStringDict(),
	}
	return obj
}

// Create a new type
func TypeNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	// fmt.Printf("TypeNew(type=%q, args=%v, kwargs=%v\n", metatype.Name, args, kwargs)
	var nameObj, basesObj, orig_dictObj Object
	var new_type, base, winner *Type
	// PyHeapTypeObject et;
	// PyMemberDef mp;
	// Py_ssize_t i, nbases, nslots, slotoffset, add_dict, add_weak;
	// _Py_IDENTIFIER(__qualname__);
	// _Py_IDENTIFIER(__slots__);

	// Special case: type(x) should return x.ob_type
	if metatype != nil && len(args) == 1 && kwargs.Len() == 0 {
		return args[0].Type(), nil
	}

	// The class statement's own keyword arguments - everything besides
	// name/bases/dict - are meant for the base's __init_subclass__, and both
	// the arity check below and ParseTupleAndKeywords reject anything outside
	// name/bases/dict.  Take them out before either sees them.
	classKeywords := NewStringDict()
	if !kwargs.IsNil() {
		for _, e := range kwargs.Items() {
			switch e.Key {
			case "name", "bases", "dict", "metaclass":
			default:
				classKeywords.Set(e.Key, e.Value)
				kwargs.Del(e.Key)
			}
		}
	}

	// SF bug 475327 -- if that didn't trigger, we need 3
	// arguments. but PyArg_ParseTupleAndKeywords below may give
	// a msg saying type() needs exactly 3.
	if len(args)+kwargs.Len() != 3 {
		return nil, ExceptionNewf(TypeError, "type() takes 1 or 3 arguments")
	}

	// Check arguments: (name, bases, dict)
	err := ParseTupleAndKeywords(args, kwargs, "UOO:type", []string{"name", "bases", "dict"},
		&nameObj,
		&basesObj,
		&orig_dictObj)
	if err != nil {
		return nil, err
	}
	name := nameObj.(String)
	// COPY the bases.  A type assertion on a Tuple ALIASES the caller's
	// backing array, and the caller is the VM passing its argument tuple -
	// whose storage is reused.  The class's Bases then changed under it:
	// "class C(object): pass" followed by ANY annotated definition turned
	// C.__bases__ into the function's name, because the annotated def reused
	// that stack slot and wrote over the shared array in place.
	// Tested: "class C(object): pass; def f() -> int: pass; print(C.__bases__)"
	// printed ('f').
	bases := make(Tuple, len(basesObj.(Tuple)))
	copy(bases, basesObj.(Tuple))
	orig_dict := orig_dictObj.(StringDict)

	// A base need not be a class: PEP 560 lets an object say what it should be
	// REPLACED BY, through __mro_entries__.  A subscripted typing alias uses
	// this - the bases of "class NullFile(IO[str])" are IO and Generic, not the
	// alias - and without it every such class statement failed with "bases
	// must be types".  rich declares exactly that shape, and pip renders
	// through rich.
	expanded := make(Tuple, 0, len(bases))
	var mroErr error
	for _, b := range bases {
		if _, isType := b.(*Type); isType {
			expanded = append(expanded, b)
			continue
		}
		getter, err := GetAttrString(b, "__mro_entries__")
		if err != nil {
			// No hook: leave it alone so the existing "bases must be types"
			// error names the real problem.
			expanded = append(expanded, b)
			continue
		}
		// The hook is called with the ORIGINAL bases tuple, as CPython does.
		res, err := Call(getter, Tuple{bases}, NewStringDict())
		if err != nil {
			mroErr = err
			break
		}
		t, ok := res.(Tuple)
		if !ok {
			mroErr = ExceptionNewf(TypeError,
				"__mro_entries__ must return a tuple")
			break
		}
		expanded = append(expanded, t...)
	}
	if mroErr != nil {
		return nil, mroErr
	}
	bases = expanded

	// Determine the proper metatype to deal with this:
	winner, err = metatype.CalculateMetaclass(bases)
	if err != nil {
		return nil, err
	}

	if winner != metatype {
		//if winner.New != TypeNew { // Pass it to the winner
		// FIXME Nasty hack since you can't compare function pointers in Go
		if fmt.Sprintf("%p", winner.New) != fmt.Sprintf("%p", TypeNew) { // Pass it to the winner
			return winner.New(winner, args, kwargs)
		}
		metatype = winner
	}

	// Adjust for empty tuple bases
	if len(bases) == 0 {
		bases = Tuple{Object(ObjectType)}
	}

	// Calculate best base, and check that all bases are type objects
	base, err = best_base(bases)
	if err != nil {
		return nil, err
	}
	// A type may DECLINE to be a base even though its own base is one.
	//
	// NewType inherits flags from the superclass, so ListType inherits
	// BASETYPE from ObjectType - and clearing it in an init() does not stick,
	// because Ready() sets the flags on the type afterwards.  An explicit
	// marker on the type is read here instead, which is the one place that
	// decides.
	if base.Flags&TPFLAGS_BASETYPE == 0 || base.NoSubclass {
		// "type" is allowed as a base so that "metaclass=" can be spelled the
		// Python 2 way - "class C(object, metaclass=M)" needs M to derive
		// from type - and so that a class statement with a metaclass on a
		// plain type works.  Nothing here dispatches on the metaclass, so the
		// result is an ordinary class; see the note in __build_class__.
		if base != TypeType {
			return nil, ExceptionNewf(TypeError, "type '%s' is not an acceptable base type", base.Name)
		}
	}

	dict := orig_dict.Copy()

	// Check for a __slots__ sequence variable in dict, and count it
	slots, haveSlots := dict.Get("__slots__")
	nslots := 0
	// add_dict := 0
	// add_weak := 0
	// may_add_dict = base.tp_dictoffset == 0
	// may_add_weak = base.tp_weaklistoffset == 0 && base.tp_itemsize == 0
	if !haveSlots {
		// if may_add_dict {
		// 	add_dict++
		// }
		// if may_add_weak {
		// 	add_weak++
		// }
	} else {
		// __slots__ is IGNORED, deliberately.
		//
		// It is an optimisation: it tells CPython to store the named
		// attributes in preallocated slots instead of a per-instance dict.
		// The observable difference is that a typo'd attribute raises
		// AttributeError rather than silently creating one, and that the
		// instances have no __dict__.  Neither matters to code that uses the
		// attributes it declared, and every class still WORKS.
		//
		// Refusing it - which is what this did, with "Can't do __slots__
		// yet" - is not an option: __slots__ is common in ordinary library
		// code, and a class that cannot be defined at all is far worse than
		// one that is a little more permissive than it asked to be.  The
		// FIXME below is the note this replaces.
		_ = slots
		/* FIXME ignore slots for the moment
		// Have slots

		// Make it into a tuple
		if PyUnicode_Check(slots) {
			slots = PyTuple_Pack(1, slots)
		} else {
			slots = PySequence_Tuple(slots)
		}
		if slots == nil {
			goto error
		}
		assert(PyTuple_Check(slots))

		// Are slots allowed?
		nslots = PyTuple_GET_SIZE(slots)
		if nslots > 0 && base.tp_itemsize != 0 {
			PyErr_Format(PyExc_TypeError,
				"nonempty __slots__ not supported for subtype of '%s'",
				base.tp_name)
			goto error
		}

		// Check for valid slot names and two special cases
		for i = 0; i < nslots; i++ {
			PyObject * tmp = PyTuple_GET_ITEM(slots, i)
			if !valid_identifier(tmp) {
				goto error
			}
			assert(PyUnicode_Check(tmp))
			if _PyUnicode_CompareWithId(tmp, &PyId___dict__) == 0 {
				if !may_add_dict || add_dict {
					PyErr_SetString(PyExc_TypeError,
						"__dict__ slot disallowed: we already got one")
					goto error
				}
				add_dict++
			}
			if PyUnicode_CompareWithASCIIString(tmp, "__weakref__") == 0 {
				if !may_add_weak || add_weak {
					PyErr_SetString(PyExc_TypeError,
						"__weakref__ slot disallowed: either we already got one, or __itemsize__ != 0")
					goto error
				}
				add_weak++
			}
		}

		// Copy slots into a list, mangle names and sort them.
		// Sorted names are needed for __class__ assignment.
		// Convert them back to tuple at the end.

		newslots = PyList_New(nslots - add_dict - add_weak)
		for i, j = 0, 0; i < nslots; i++ {
			tmp = PyTuple_GET_ITEM(slots, i)
			if (add_dict &&
				_PyUnicode_CompareWithId(tmp, &PyId___dict__) == 0) ||
				(add_weak &&
					PyUnicode_CompareWithASCIIString(tmp, "__weakref__") == 0) {
				continue
			}
			tmp = _Py_Mangle(name, tmp)
			if !tmp {
				goto error
			}
			PyList_SET_ITEM(newslots, j, tmp)
			if PyDict_GetItem(dict, tmp) {
				PyErr_Format(PyExc_ValueError,
					"%R in __slots__ conflicts with class variable",
					tmp)
				goto error
			}
			j++
		}
		assert(j == nslots-add_dict-add_weak)
		nslots = j
		Py_CLEAR(slots)
		if PyList_Sort(newslots) == -1 {
			goto error
		}
		slots = PyList_AsTuple(newslots)

		// Secondary bases may provide weakrefs or dict
		if nbases > 1 &&
			((may_add_dict && !add_dict) ||
				(may_add_weak && !add_weak)) {
			for i = 0; i < nbases; i++ {
				tmp = PyTuple_GET_ITEM(bases, i)
				if tmp == base {
					continue // Skip primary base
				}
				// assert(PyType_Check(tmp));
				tmptype = tmp.(*Type)
				if may_add_dict && !add_dict &&
					tmptype.tp_dictoffset != 0 {
					add_dict++
				}
				if may_add_weak && !add_weak &&
					tmptype.tp_weaklistoffset != 0 {
					add_weak++
				}
				if may_add_dict && !add_dict {
					continue
				}
				if may_add_weak && !add_weak {
					continue
				}
				// Nothing more to check
				break
			}
		}
		*/
	}

	// Allocate the type object
	_ = nslots // FIXME
	new_type = metatype.Alloc()
	// A class's native constructor is exposed as __new__, so that
	// "tuple.__new__(cls, args)" - which is how a Python subclass of a builtin
	// constructs itself - actually works.  Only when the class body does NOT
	// define one: a Python __new__ is kept as written and honoured by
	// Type.M__call__.
	if _, hasOwnNew := new_type.Dict.Get("__new__"); !hasOwnNew {
		new_type.Dict.Set("__new__", &nativeNew{})
	}

	// A class INHERITS its base's constructor, which is what makes a subclass
	// of a builtin behave like the builtin: without this, "class D(dict): pass;
	// D({'a': 1})" produced an EMPTY dict, because the hardcoded ObjectNew
	// knows nothing about dict arguments.  CPython does the same by leaving
	// tp_new/tp_init to be inherited from tp_base.
	new_type.New = ObjectNew   // FIXME metatype.New // FIXME?
	new_type.Init = ObjectInit // FIXME metatype.New // FIXME?
	if base != nil {
		if base.New != nil {
			new_type.New = base.New
		}
		if base.Init != nil {
			new_type.Init = base.Init
		}
	}

	// Keep name and slots alive in the extended type object
	et := new_type
	et.Name = string(name)
	// FIXME et.Slots = slots
	slots = nil

	// Initialize tp_flags
	new_type.Flags = TPFLAGS_DEFAULT | TPFLAGS_HEAPTYPE | TPFLAGS_BASETYPE

	// Set tp_base and tp_bases
	new_type.Bases = bases
	bases = nil
	new_type.Base = base

	// Initialize tp_dict from passed-in dict
	new_type.Dict = dict
	// fmt.Printf("New type dict is %v\n", dict)

	// Set __module__ in the dict, from the __name__ of the globals the class
	// statement is executing in.
	//
	// This used to print "*** FIXME need to get the current vm globals
	// somehow" to stderr and set nothing, so every class lacked __module__ -
	// visible in pip, which printed the line twice on a successful run.  The
	// frame executing right now is the one running the class body, so its
	// globals are exactly what CPython reads here.
	if _, ok := dict.Get("__module__"); !ok {
		if frame := currentFrame(); frame != nil {
			if name, ok := frame.Globals.Get("__name__"); ok {
				dict.Set("__module__", name)
			}
		}
	}

	// Set ht_qualname to dict['__qualname__'] if available, else to
	// __name__.  The __qualname__ accessor will look for ht_qualname.
	if qualname, ok := dict.Get("__qualname__"); ok {
		if Qualname, ok := qualname.(String); !ok {
			return nil, ExceptionNewf(TypeError, "type __qualname__ must be a str, not %s", qualname.Type().Name)
		} else {
			et.Qualname = string(Qualname)
		}
		dict.Del("__qualname__")
	} else {
		et.Qualname = et.Name
	}

	// Set tp_doc to a copy of dict['__doc__'], if the latter is there
	// and is a string.  The __doc__ accessor will first look for tp_doc;
	// if that fails, it will still look into __dict__.
	if doc, ok := dict.Get("__doc__"); ok {
		if Doc, ok := doc.(String); ok {
			new_type.Doc = string(Doc)
		}
	}

	// Special-case __new__: if it's a plain function,
	// make it a static function
	// FIXME
	// tmp = dict["__new__"]
	// if tmp != nil && PyFunction_Check(tmp) {
	// 	tmp = PyStaticMethod_New(tmp)
	// 	if _PyDict_SetItemId(dict, &PyId___new__, tmp) < 0 {
	// 		goto error
	// 	}
	// }

	/*
		// Add descriptors for custom slots from __slots__, or for __dict__
		mp = PyHeapType_GET_MEMBERS(et)
		slotoffset = base.tp_basicsize
		if et.ht_slots != nil {
			for i = 0; i < nslots; i++ {
				mp.name = _PyUnicode_AsString(
					PyTuple_GET_ITEM(et.ht_slots, i))
				mp.new_type = T_OBJECT_EX
				mp.offset = slotoffset

				// __dict__ and __weakref__ are already filtered out
				assert(strcmp(mp.name, "__dict__") != 0)
				assert(strcmp(mp.name, "__weakref__") != 0)

				slotoffset += 1 // FIXME sizeof(PyObject *);
				mp++
			}
		}
		if add_dict {
			// if (base.tp_itemsize)
			//     new_type.tp_dictoffset = -sizeof(PyObject *);
			// else
			//     new_type.tp_dictoffset = slotoffset;
			slotoffset += 1 // sizeof(PyObject *);
		}
		if new_type.tp_dictoffset {
			et.ht_cached_keys = _PyDict_NewKeysForClass()
		}
		if add_weak {
			assert(!base.tp_itemsize)
			new_type.tp_weaklistoffset = slotoffset
			slotoffset += 1 // FIXME sizeof(PyObject *);
		}
		new_type.tp_basicsize = slotoffset
		new_type.tp_itemsize = base.tp_itemsize
		new_type.tp_members = PyHeapType_GET_MEMBERS(et)

		if new_type.tp_weaklistoffset && new_type.tp_dictoffset {
			new_type.tp_getset = subtype_getsets_full
		} else if new_type.tp_weaklistoffset && !new_type.tp_dictoffset {
			new_type.tp_getset = subtype_getsets_weakref_only
		} else if !new_type.tp_weaklistoffset && new_type.tp_dictoffset {
			new_type.tp_getset = subtype_getsets_dict_only
		} else {
			new_type.tp_getset = nil
		}

		// Special case some slots
		if new_type.tp_dictoffset != 0 || nslots > 0 {
			if base.tp_getattr == nil && base.tp_getattro == nil {
				new_type.tp_getattro = PyObject_GenericGetAttr
			}
			if base.tp_setattr == nil && base.tp_setattro == nil {
				new_type.tp_setattro = PyObject_GenericSetAttr
			}
		}
		new_type.tp_dealloc = subtype_dealloc

		// Enable GC unless there are really no instance variables possible
		if !(new_type.tp_basicsize == sizeof(PyObject) &&
			new_type.tp_itemsize == 0) {
			new_type.tp_flags |= TPFLAGS_HAVE_GC
		}

		// Always override allocation strategy to use regular heap
		new_type.tp_alloc = PyType_GenericAlloc
		if new_type.tp_flags & TPFLAGS_HAVE_GC {
			new_type.tp_free = PyObject_GC_Del
			new_type.tp_traverse = subtype_traverse
			new_type.tp_clear = subtype_clear
		} else {
			new_type.tp_free = PyObject_Del
		}

	*/
	// Initialize the rest
	err = new_type.Ready()
	if err != nil {
		return nil, err
	}

	// __set_name__ is called once, for every descriptor in the class body that
	// defines it, after the class exists.  It is how a descriptor learns the
	// attribute name it was bound to - functools.cached_property uses it to
	// know which key to cache under, and without the call its attrname was
	// empty and nothing was ever cached.
	//
	// The name here is the one in the CLASS BODY, which is what the descriptor
	// needs, and it is called with the class being created as the owner.
	// The class's OWN dict, which is what holds the descriptors from the body -
	// orig_dict has already been copied into it by this point.
	runSetName(new_type, new_type.Dict)

	// __init_subclass__ is called on the BASE, once, when a subclass is
	// created - it is the hook by which a base class learns about and
	// configures its subclasses, and the counterpart of __set_name__.
	//
	// Any keyword arguments the class statement carried besides name/bases/dict
	// are passed straight through: "class C(Base, kind='x')" reaches
	// Base.__init_subclass__(kind='x').
	if err := runInitSubclass(new_type, classKeywords); err != nil {
		return nil, err
	}

	// Put the proper slots in place
	// fixup_slot_dispatchers(new_type)

	return new_type, nil
}

// runInitSubclass calls __init_subclass__ on the nearest BASE that defines it.
//
// It is looked up on the bases, not on the new class: the hook is how a base
// configures each class derived from it, and CPython does not call it for the
// class that declares it.  Any keyword arguments in the class statement are
// passed straight through ("class C(Base, kind='x')").
func runInitSubclass(cls *Type, classKeywords StringDict) error {
	// The hook comes from the bases.  Base is not always set - a class whose
	// only base is object has a nil Base - so the bases themselves are what
	// this walks, which is also what CPython does.
	var base *Type
	for _, b := range cls.Bases {
		if bt, ok := b.(*Type); ok && bt != cls {
			base = bt
			break
		}
	}
	if base == nil {
		return nil
	}
	hook := base.Lookup("__init_subclass__")
	if hook == nil {
		return nil
	}
	// object.__init_subclass__ is the no-op default.  It is found through the
	// base's MRO - Plain above inherits it from object - so the test is on the
	// HOOK, not on the base.  It takes no keyword arguments, and CPython
	// REJECTS a class keyword that would have to reach it:
	// "class Bad(Plain, nope=1)" is a TypeError, not an ignored keyword.
	if hook == objectInitSubclass {
		if classKeywords.Len() > 0 {
			return ExceptionNewf(TypeError, "%s.__init_subclass__() takes no keyword arguments", cls.Name)
		}
		return nil
	}
	_, err := Call(hook, Tuple{cls}, classKeywords)
	return err
}

// objectInitSubclass is object's own no-op __init_subclass__, which Python
// resolves to when no base overrides it.
var objectInitSubclass = ObjectType.Lookup("__init_subclass__")

// runSetName calls __set_name__ on every descriptor in a class body that
// defines one.
//
// A failure is returned to the caller rather than swallowed: CPython propagates
// it, and a descriptor that could not record its name is broken in a way the
// program would otherwise only see much later.
func runSetName(cls *Type, body StringDict) {
	entries := body.Items()
	for _, ent := range entries {
		setName, err := GetAttrString(ent.Value, "__set_name__")
		if err != nil {
			continue
		}
		// Each call is independent; a failure in one does not stop the rest,
		// matching CPython, which collects and re-raises at the end.
		_, _ = Call(setName, Tuple{cls, String(ent.Key)}, NewStringDict())
	}
}

func TypeInit(cls Object, args Tuple, kwargs StringDict) error {
	if kwargs.Len() != 0 {
		return ExceptionNewf(TypeError, "type.__init__() takes no keyword arguments")
	}

	if len(args) != 1 && len(args) != 3 {
		return ExceptionNewf(TypeError, "type.__init__() takes 1 or 3 arguments")
	}

	// Call object.__init__(self) now.
	// XXX Could call super(type, cls).__init__() but what's the point?
	return ObjectInit(cls, nil, NewStringDict())
}

// The base type of all types (eventually)... except itself.

// You may wonder why object.__new__() only complains about arguments
// when object.__init__() is not overridden, and vice versa.
//
// Consider the use cases:
//
// 1. When neither is overridden, we want to hear complaints about
//    excess (i.e., any) arguments, since their presence could
//    indicate there's a bug.
//
// 2. When defining an Immutable type, we are likely to override only
//    __new__(), since __init__() is called too late to initialize an
//    Immutable object.  Since __new__() defines the signature for the
//    type, it would be a pain to have to override __init__() just to
//    stop it from complaining about excess arguments.
//
// 3. When defining a Mutable type, we are likely to override only
//    __init__().  So here the converse reasoning applies: we don't
//    want to have to override __new__() just to stop it from
//    complaining.
//
// 4. When __init__() is overridden, and the subclass __init__() calls
//    object.__init__(), the latter should complain about excess
//    arguments; ditto for __new__().
//
// Use cases 2 and 3 make it unattractive to unconditionally check for
// excess arguments.  The best solution that addresses all four use
// cases is as follows: __init__() complains about excess arguments
// unless __new__() is overridden and __init__() is not overridden
// (IOW, if __init__() is overridden or __new__() is not overridden);
// symmetrically, __new__() complains about excess arguments unless
// __init__() is overridden and __new__() is not overridden
// (IOW, if __new__() is overridden or __init__() is not overridden).
//
// However, for backwards compatibility, this breaks too much code.
// Therefore, in 2.6, we'll *warn* about excess arguments when both
// methods are overridden; for all other cases we'll use the above
// rules.

// Return true if any arguments supplied
func excess_args(args Tuple, kwargs StringDict) bool {
	return len(args) != 0 || kwargs.Len() != 0
}

func ObjectInit(self Object, args Tuple, kwargs StringDict) error {
	t := self.Type()
	// FIXME bodge to compare function pointers
	// if excess_args(args, kwargs) && (fmt.Sprintf("%p", t.New) == fmt.Sprintf("%p", ObjectNew) || fmt.Sprintf("%p", t.Init) != fmt.Sprintf("%p", ObjectInit)) {
	// 	return ExceptionNewf(TypeError, "object.__init__() takes no parameters")
	// }

	// FIXME this isn't correct probably
	// Check args for object()
	if t == ObjectType && excess_args(args, kwargs) {
		return ExceptionNewf(TypeError, "object.__init__() takes no parameters")
	}

	// Call the __init__ method if it exists
	// FIXME this isn't the way cpython does it - it adjusts the function pointers
	//
	// The guard used to be "self is a *Type", which is the PYTHON-level instance
	// representation - and every instance of a native type (a threading.local, a
	// list, a logging.Logger) is a Go struct instead.  So a Python __init__ on a
	// subclass of a native type never ran at all: the instance was built by the
	// native constructor and its fields were never set.
	//
	// The real question is whether the object's CLASS defines an __init__ that
	// is not object's own, so ask that instead.
	if init := t.GetAttrOrNil("__init__"); init != nil && !isObjectInit(init) {
		// A native method taken straight from the type's Dict is UNBOUND, so
		// calling it with self among the arguments did not bind it: the
		// implementation was handed the method's MODULE as self instead.  A
		// subclass of a native type that INHERITS __init__ therefore could not
		// be constructed at all - "class E(logging.Filter): pass; E()" raised
		// "not a Filter", because the base __init__ got a *py.Module where it
		// asserted its own type.  A method reached through the instance (the
		// Python-level __init__ case) is already bound and keeps the old path.
		if m, isMethod := init.(*Method); isMethod && !m.Unbound {
			if _, err := m.CallWithKeywords(self, args, kwargs); err != nil {
				return err
			}
			return nil
		}
		newArgs := make(Tuple, len(args)+1)
		newArgs[0] = self
		copy(newArgs[1:], args)
		if _, err := Call(init, newArgs, kwargs); err != nil {
			return err
		}
	}
	return nil
}

// isObjectInit reports whether an __init__ is object's own, which must not be
// re-entered from here.
//
// Compared by IDENTITY against the attribute on object: a class that does not
// define __init__ looks it up and finds object's, and calling it from here
// would recurse.
func isObjectInit(init Object) bool {
	return init == ObjectType.GetAttrOrNil("__init__")
}

func ObjectNew(t *Type, args Tuple, kwargs StringDict) (Object, error) {
	// FIXME bodge to compare function pointers
	// if excess_args(args, kwargs) && (fmt.Sprintf("%p", t.Init) == fmt.Sprintf("%p", ObjectInit) || fmt.Sprintf("%p", t.New) != fmt.Sprintf("%p", ObjectNew)) {
	// 	return ExceptionNewf(TypeError, "object() takes no parameters")
	// }

	// FIXME this isn't correct probably
	// Check arguments to new only for object
	if t == ObjectType && excess_args(args, kwargs) {
		return nil, ExceptionNewf(TypeError, "object() takes no parameters")
	}

	// FIXME abstrac ty pes
	// if (type->tp_flags & TPFLAGS_IS_ABSTRACT) {
	// 	PyObject *abstract_methods = NULL;
	// 	PyObject *builtins;
	// 	PyObject *sorted;
	// 	PyObject *sorted_methods = NULL;
	// 	PyObject *joined = NULL;
	// 	PyObject *comma;
	// 	_Py_static_string(comma_id, ", ");
	// 	_Py_IDENTIFIER(sorted);

	// 	// Compute ", ".join(sorted(type.__abstractmethods__))
	// 	// into joined.
	// 	abstract_methods = type_abstractmethods(type, NULL);
	// 	if (abstract_methods == NULL) {
	// 		goto error;
	// 	}
	// 	builtins = PyEval_GetBuiltins();
	// 	if (builtins == NULL) {
	// 		goto error;
	// 	}
	// 	sorted = _PyDict_GetItemId(builtins, &PyId_sorted);
	// 	if (sorted == NULL) {
	// 		goto error;
	// 	}
	// 	sorted_methods = PyObject_CallFunctionObjArgs(sorted,
	// 		abstract_methods,
	// 		NULL);
	// 	if (sorted_methods == NULL) {
	// 		goto error;
	// 	}
	// 	comma = _PyUnicode_FromId(&comma_id);
	// 	if (comma == NULL) {
	// 		goto error;
	// 	}
	// 	joined = PyUnicode_Join(comma, sorted_methods);
	// 	if (joined == NULL) {
	// 		goto error;
	// 	}

	// 	PyErr_Format(PyExc_TypeError,
	// 		"Can't instantiate abstract class %s "
	// 		"with abstract methods %U",
	// 		type->tp_name,
	// 		joined);
	// error:
	// 	Py_XDECREF(joined);
	// 	Py_XDECREF(sorted_methods);
	// 	Py_XDECREF(abstract_methods);
	// 	return NULL;
	// }
	return t.Alloc(), nil
}

// FIXME this should be the default?
func (ty *Type) M__eq__(other Object) (Object, error) {
	// An INSTANCE looks its __eq__ up through its class, so a user's method
	// must be the one that decides.  "Name == \"\"" is how this codebase tells
	// an instance from a class - the same test M__str__ uses a few lines down,
	// and for the same reason.
	//
	// Without it this method's identity comparison swallowed the operator:
	// "class K: def __eq__(self, o): return True" gave K() == K() False, and
	// every list, tuple and set holding such objects inherited the lie,
	// because Eq consulted the interface this satisfies before ever looking
	// for a Python method.
	if ty.Name == "" {
		if res, found, err := ty.CallMethod("__eq__", Tuple{ty, other}, NewStringDict()); err != nil {
			return nil, err
		} else if found && res != NotImplemented {
			return res, nil
		}
	}
	if otherTy, ok := other.(*Type); ok && ty == otherTy {
		return True, nil
	}
	return False, nil
}

// FIXME this should be the default?
func (ty *Type) M__ne__(other Object) (Object, error) {
	// As for M__eq__ above.
	if ty.Name == "" {
		if res, found, err := ty.CallMethod("__ne__", Tuple{ty, other}, NewStringDict()); err != nil {
			return nil, err
		} else if found && res != NotImplemented {
			return res, nil
		}
	}
	if otherTy, ok := other.(*Type); ok && ty == otherTy {
		return False, nil
	}
	return True, nil
}

func (ty *Type) M__str__() (Object, error) {
	// An INSTANCE looks its __str__ up through its class, which is how
	// "str(A())" reaches a user's method.  A CLASS's own __str__ describes
	// its instances, so it must not be handed the class as self - that is
	// what made "repr(BytesIO)" call BytesIO.__repr__ with a *Type and panic
	// on its type assertion.  "Name == \"\"" is how this codebase already
	// tells an instance from a class; see the FIXME below.
	if ty.Name == "" {
		if res, ok, err := ty.CallMethod("__str__", Tuple{ty}, NewStringDict()); ok {
			return res, err
		}
	}
	return ty.M__repr__()
}

func (ty *Type) M__repr__() (Object, error) {
	if ty.Name == "" {
		// A Python __repr__ on the class wins; then the payload's.
		//
		// pygments' Token defines __repr__, and its children print as
		// "Token.Text" because of this.  A plain subclass with none prints as
		// its container, which is what "class L(list)" should do.
		if fn := ty.LookupPython("__repr__"); fn != nil {
			if res, err := Call(fn, Tuple{ty}, NewStringDict()); err == nil {
				return res, nil
			}
		}
		if payload, ok := payloadOf(ty); ok {
			return Repr(payload)
		}
		if res, ok, err := ty.CallMethod("__repr__", Tuple{ty}, NewStringDict()); ok {
			return res, err
		}
	}
	if ty.Name == "" {
		// FIXME not a good way to tell objects from classes!
		return String(fmt.Sprintf("<%s object at %p>", ty.Type().Name, ty)), nil
	}
	// A class repr is QUALIFIED with its module: "<class '__main__.MyError'>".
	// The module is the class's own __module__, which the class statement sets
	// from the globals it ran in - so a class knows where it was defined
	// without this needing to guess.
	//
	// A builtin type has no __module__ and prints bare, which is also what
	// CPython does: "<class 'int'>".
	name := ty.Name
	if mod, ok := ty.Dict.Get("__module__"); ok {
		if ms, ok := mod.(String); ok && string(ms) != "" && string(ms) != "builtins" {
			name = string(ms) + "." + name
		}
	}
	return String(fmt.Sprintf("<class '%s'>", name)), nil

}

// Make sure it satisfies the interface
var _ Object = (*Type)(nil)
var _ I__call__ = (*Type)(nil)
var _ IGetDict = (*Type)(nil)
var _ I__repr__ = (*Type)(nil)
var _ I__str__ = (*Type)(nil)

// ABCHooks are consulted by isinstance for objects whose concrete type does
// not derive from the class being tested for.
//
// isinstance() cannot know about structural conformance on its own - a list
// is Iterable without deriving from the Iterable class, an enum member is an
// Enum without deriving from it - and those classes live in standard library
// packages that must not be imported from here, so they install their checks
// at init time.  It is a slice so that several modules can contribute.
var ABCHooks []func(obj Object, class *Type) bool

// sequenceABC is collections.abc.Sequence, published by that module so that
// code in this package can test for it without importing a stdlib package.
var sequenceABC *Type

// SetSequenceABC records collections.abc.Sequence.
func SetSequenceABC(t *Type) { sequenceABC = t }

// SequenceABC returns collections.abc.Sequence, or nil before it is published.
func SequenceABC() *Type { return sequenceABC }

// BuildEnumClass, when set, turns the body of a class that derives from an
// enum into an enum class.  Python does this with a metaclass, and
// metaclasses are not supported here, so __build_class__ offers the hook
// instead.  It is installed by the enum module.
var BuildEnumClass func(name string, bases []Object, ns StringDict) (Object, error)

// BuildNamedTupleClass, when set, turns the body of a class that derives from
// typing.NamedTuple into a named tuple class.  CPython does this with
// __mro_entries__ on the NamedTuple base (PEP 560); the hook here is the same
// shape as BuildEnumClass, and is installed by the typing module.
var BuildNamedTupleClass func(name string, bases []Object, ns StringDict) (Object, error)

// IsEnumBase reports whether the type is an enum class, which is how
// __build_class__ recognises the bases that need BuildEnumClass.
func (t *Type) IsEnumBase() bool {
	return t.Flags&TPFLAGS_ENUM != 0
}

// IsNamedTupleBase reports whether the type is typing.NamedTuple, which is how
// __build_class__ recognises the base that needs BuildNamedTupleClass.
func (t *Type) IsNamedTupleBase() bool {
	return t.Flags&TPFLAGS_NAMEDTUPLE != 0
}

// TPFLAGS_ENUM marks a class that derives from enum.Enum.
const TPFLAGS_ENUM uint = 1 << 20

// TPFLAGS_NAMEDTUPLE marks typing.NamedTuple, the base whose subclasses are
// built by BuildNamedTupleClass.
const TPFLAGS_NAMEDTUPLE uint = 1 << 21

// M__or__ gives "type | type" a result.
//
// PEP 604 unions appear in evaluated positions - a class base, a default
// value - where deferring annotations does not help, so the type object has
// to answer "|" rather than raising.  The result is a union type that exists
// to be used as a base or passed around; it is not a run-time membership
// test.
func (t *Type) M__or__(other Object) (Object, error) {
	return unionOf(t, other)
}

// unionOf builds (or reuses) the union type of two types.
func unionOf(a, b Object) (Object, error) {
	name := "Union"
	if at, ok := a.(*Type); ok {
		name += "[" + at.Name
	}
	if bt, ok := b.(*Type); ok {
		name += ", " + bt.Name
	}
	name += "]"

	union := NewType("typing."+name, "A union of types, from X | Y.")
	union.Flags |= TPFLAGS_BASETYPE
	// The union is a set of types rather than one, so deriving from it is
	// allowed and gives a class whose base is the union.
	if at, ok := a.(*Type); ok {
		union.Base = at
	} else if bt, ok := b.(*Type); ok {
		union.Base = bt
	}
	return union, nil
}

// UnionType is the type of a union made from "|".
var UnionType = NewType("typing.UnionType", "The type of a union made with X | Y.")

func init() {
	// __or__ on a type produces a union, which is what makes
	// "class C(str | bytes)" work.  It is set on TypeType so every type has
	// it, and the Go interface above is what the VM actually reaches.
	orMethod := MustNewMethod("__or__", func(self Object, args Tuple) (Object, error) {
		var other Object
		if err := UnpackTuple(args, NewStringDict(), "__or__", 1, 1, &other); err != nil {
			return nil, err
		}
		return unionOf(self, other)
	}, 0, "Return the union of two types.")
	if !TypeType.Dict.IsNil() {
		TypeType.Dict.Set("__or__", orMethod)
	}
}

// PEP 585: the builtin container types are subscriptable.
//
// "tuple[int, ...]" and "list[str]" appear in evaluated positions - a class
// base, a default value - where deferring annotations does not help, so the
// builtin types have to answer "[" rather than raising.  The parameters only
// matter to a type checker, so the class itself is the result, exactly as it
// is for the abstract base classes.
func init() {
	getitem := MustNewMethod("__class_getitem__", func(self Object, args Tuple) (Object, error) {
		return self, nil
	}, 0, "Return the class, ignoring the subscription parameters.")

	// ObjectType is deliberately NOT in this list: every class's MRO includes
	// object, so putting the hook there made EVERY class subscriptable-as-
	// class and shadowed __getitem__, which is why "g[3]" on a class defining
	// __getitem__ returned the instance itself.  CPython raises TypeError for
	// "object[int]".
	//
	// TypeType IS included, because "type[X]" is legal - since PEP 585 a
	// generic alias may name a type, and typing_extensions writes
	// "type[Warning]" in an annotation that gets evaluated.
	for _, t := range []*Type{
		ListType, TupleType, DictType, SetType, FrozenSetType,
		StringType, BytesType, IntType, FloatType, BoolType,
		SliceType, ComplexType, TypeType, StringDictType,
	} {
		if t != nil && !t.Dict.IsNil() {
			t.Dict.Set("__class_getitem__", getitem)
		}
	}
}
