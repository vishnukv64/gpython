// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Module objects

package py

import (
	"fmt"
	"sort"
	"sync"
)

type ModuleFlags int32

const (
	// ShareModule signals that an embedded module is threadsafe and read-only, meaninging it could be shared across multiple py.Context instances (for efficiency).
	// Otherwise, ModuleImpl will create a separate py.Module instance for each py.Context that imports it.
	// This should be used with extreme caution since any module mutation (write) means possible cross-context data corruption.
	ShareModule ModuleFlags = 0x01

	MainModuleName = "__main__"
)

// ModuleInfo contains info and about a module and can specify flags that affect how it is imported into a py.Context
type ModuleInfo struct {
	Name     string // __name__ (if nil, "__main__" is used)
	Doc      string // __doc__
	FileDesc string // __file__
	Flags    ModuleFlags
}

// ModuleImpl is used for modules that are ready to be imported into a py.Context.
// The model is that a ModuleImpl is read-only and instantiates a Module into a py.Context when imported.
//
// By convention, .Code is executed when a module instance is initialized. If nil,
// then .CodeBuf or .CodeSrc will be auto-compiled to set .Code.
type ModuleImpl struct {
	Info    ModuleInfo
	Methods []*Method  // Module-bound global method functions
	Globals StringDict // Module-bound global variables
	CodeSrc string     // Module code body (source code to be compiled)
	CodeBuf []byte     // Module code body (serialized py.Code object)
	Code    *Code      // Module code body
	// PreSetGlobals, when set, is given the module's globals BEFORE the body
	// runs, so a module can be given __spec__ (or anything else) in time for
	// its first statement to use it.
	PreSetGlobals   func(StringDict)
	OnContextClosed func(*Module) // Callback for when a py.Context is closing to release resources
}

// ModuleStore is a container of Module imported into an owning py.Context.
type ModuleStore struct {
	// Registry of installed modules
	modules map[string]*Module
	// moduleLike holds objects substituted into sys.modules that are NOT
	// *Module: a subclass of types.ModuleType, which a Python program builds
	// when it replaces a module at run time.  pygments does exactly that
	//
	//	newmod = _automodule(__name__)   # a types.ModuleType subclass
	//	sys.modules[__name__] = newmod
	//
	// Keeping them in a map of their own means GetModule keeps its *Module
	// contract for the import machinery - which needs a real Module to compile
	// into - while sys.modules can still carry what the program put there.
	moduleLike map[string]Object
	// Builtin module
	Builtins *Module
	// this should be the frozen module importlib/_bootstrap.py generated
	// by Modules/_freeze_importlib.c into Python/importlib.h
	Importlib *Module

	// moduleMu guards the module registry.  A context is not meant to be
	// entered by two goroutines at once, but a shared context is a mistake
	// that is easy to make and the race detector catches it here: two
	// goroutines initialising modules wrote this map concurrently.  Holding
	// a lock is cheap next to that, and it makes the mistake correct rather
	// than silently corrupting the registry.
	moduleMu sync.Mutex
}

func RegisterModule(module *ModuleImpl) {
	gRuntime.RegisterModule(module)
}

func GetModuleImpl(moduleName string) *ModuleImpl {
	gRuntime.mu.RLock()
	defer gRuntime.mu.RUnlock()
	impl := gRuntime.ModuleImpls[moduleName]
	return impl
}

type Runtime struct {
	mu          sync.RWMutex
	ModuleImpls map[string]*ModuleImpl
}

var gRuntime = Runtime{
	ModuleImpls: make(map[string]*ModuleImpl),
}

func (rt *Runtime) RegisterModule(impl *ModuleImpl) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.ModuleImpls[impl.Info.Name] = impl
}

func NewModuleStore() *ModuleStore {
	return &ModuleStore{
		modules: make(map[string]*Module),
	}
}

// Module is a runtime instance of a ModuleImpl bound to the py.Context that imported it.
type Module struct {
	ModuleImpl *ModuleImpl // Parent implementation of this Module instance
	Globals    StringDict  // Initialized from ModuleImpl.Globals
	Context    Context     // Parent context that "owns" this Module instance
}

var ModuleType = NewType("module", "module object")

// A module's __dict__ IS its global namespace, and it must be a real, writable
// mapping: "mod.__dict__['x'] = 1" and "mod.__dict__.update(other)" are how a
// module rewrites itself at run time.
//
// pygments does exactly that - it builds a replacement module and does
//
//	newmod.__dict__.update(oldmod.__dict__)
//	sys.modules[__name__] = newmod
//
// so without __dict__ the lexer package raised "'module' object has no
// attribute '__dict__'" and pip could not import pygments.
func init() {
	ModuleType.Dict.Set("__dict__", &Property{
		Fget: func(self Object) (Object, error) {
			// A *Module carries its globals directly.  A SUBCLASS of
			// types.ModuleType - "class _automodule(types.ModuleType)", which
			// pygments defines - is a *Type instance instead, and its namespace
			// is the dict an instance carries.  Answering None for that case
			// made "newmod.__dict__.update(...)" fail with "'NoneType' has no
			// attribute 'update'".
			if m, ok := self.(*Module); ok {
				return m.Globals, nil
			}
			if I, ok := self.(IGetDict); ok {
				return I.GetDict(), nil
			}
			return None, nil
		},
		Doc: "The module's namespace: its globals, as a live mapping.",
	})
}

// Type of this object
func (o *Module) Type() *Type {
	return ModuleType
}

func (m *Module) M__repr__() (Object, error) {
	name, ok := m.Globals.GetOrNil("__name__").(String)
	if !ok {
		name = "???"
	}
	return String(fmt.Sprintf("<module %s>", string(name))), nil
}

// Get the Dict
func (m *Module) GetDict() StringDict {
	return m.Globals
}

// Calls a named method of a module
func (m *Module) Call(name string, args Tuple, kwargs StringDict) (Object, error) {
	attr, err := GetAttrString(m, name)
	if err != nil {
		return nil, err
	}
	return Call(attr, args, kwargs)
}

// Interfaces
var _ IGetDict = (*Module)(nil)

// NewModule adds a new Module instance to this ModuleStore.
// Each given Method prototype is used to create a new "live" Method bound this the newly created Module.
// This func also sets appropriate module global attribs based on the given ModuleInfo (e.g. __name__).
func (store *ModuleStore) NewModule(ctx Context, impl *ModuleImpl) (*Module, error) {
	// Serialise registrations.  This covers the insert AND the sys.modules
	// view install at the end of the function, which must not be observed
	// half-done.
	store.moduleMu.Lock()
	defer store.moduleMu.Unlock()

	name := impl.Info.Name
	if name == "" {
		name = MainModuleName
	}
	m := &Module{
		ModuleImpl: impl,
		Globals:    impl.Globals.Copy(),
		Context:    ctx,
	}
	// Insert the methods into the module dictionary
	// Copy each method an insert each "live" with a ptr back to the module (which can also lead us to the host Context)
	for _, method := range impl.Methods {
		methodInst := new(Method)
		*methodInst = *method
		methodInst.Module = m
		m.Globals.Set(method.Name, methodInst)
	}
	// Set some module globals
	m.Globals.Set("__name__", String(name))
	m.Globals.Set("__doc__", String(impl.Info.Doc))
	m.Globals.Set("__package__", None)
	if len(impl.Info.FileDesc) > 0 {
		m.Globals.Set("__file__", String(impl.Info.FileDesc))
	}
	if impl.PreSetGlobals != nil {
		impl.PreSetGlobals(m.Globals)
	}

	// Register the module
	store.modules[name] = m
	if store.moduleLike == nil {
		store.moduleLike = map[string]Object{}
	}
	delete(store.moduleLike, name)

	// sys.modules is a LIVE view of this registry.  It is installed here
	// because this is where the store is at hand; the sys module's own
	// "modules" entry is an empty dict that nothing updated, so
	// "sys.modules["__main__"]" raised KeyError.
	if name == "sys" {
		m.Globals.Set("modules", NewModulesView(store))
	}
	// Make a note of some modules
	switch name {
	case "builtins":
		store.Builtins = m
	case "importlib":
		store.Importlib = m
	}
	// fmt.Printf("Registered module %q\n", moduleName)
	return m, nil
}

// Modules returns the live registry of loaded modules, keyed by name.
//
// It is what sys.modules must be: a real mapping of every module the context
// has loaded, including "__main__".  The builtin sys module had an EMPTY dict
// of its own, so "sys.modules["__main__"]" raised KeyError and any library
// that looks itself up there - click does, to find the program name - failed.
func (store *ModuleStore) Modules() map[string]*Module {
	return store.modules
}

// ModuleLike returns the object registered under a name in sys.modules, which
// may be a *Module or a program-supplied substitute.
func (store *ModuleStore) ModuleLike(name string) (Object, bool) {
	store.moduleMu.Lock()
	defer store.moduleMu.Unlock()
	if m, ok := store.modules[name]; ok {
		return m, true
	}
	if o, ok := store.moduleLike[name]; ok {
		return o, true
	}
	return nil, false
}

// SetModuleLike registers a module substitute.  A *Module goes into the real
// registry, so the import system finds it as usual; anything else - a Python
// subclass of types.ModuleType - goes into the overlay.
func (store *ModuleStore) SetModuleLike(name string, value Object) {
	store.moduleMu.Lock()
	defer store.moduleMu.Unlock()
	if store.moduleLike == nil {
		store.moduleLike = map[string]Object{}
	}
	if m, ok := value.(*Module); ok {
		store.modules[name] = m
		delete(store.moduleLike, name)
		return
	}
	store.moduleLike[name] = value
}

// Gets a module
func (store *ModuleStore) GetModule(name string) (*Module, error) {
	store.moduleMu.Lock()
	m, ok := store.modules[name]
	store.moduleMu.Unlock()
	if !ok {
		return nil, ExceptionNewf(ImportError, "Module '%s' not found", name)
	}
	return m, nil
}

// Gets a module or panics
func (store *ModuleStore) MustGetModule(name string) *Module {
	m, err := store.GetModule(name)
	if err != nil {
		panic(err)
	}
	return m
}

// OnContextClosed signals all module instances that the parent py.Context has closed
func (store *ModuleStore) OnContextClosed() {
	for _, m := range store.modules {
		if m.ModuleImpl.OnContextClosed != nil {
			m.ModuleImpl.OnContextClosed(m)
		}
	}
}

// GetModuleImplOrNil returns the registered module of that name, loading it
// if it has not been loaded yet, or nil when nothing registers the name.
//
// It is for one embedded module that needs another's objects without an
// import statement, which is not possible at package level because that
// would be an import cycle.
func GetModuleImplOrNil(name string) *Module {
	impl := GetModuleImpl(name)
	if impl == nil {
		return nil
	}
	mod, err := NewModuleStore().NewModule(nil, impl)
	if err != nil {
		return nil
	}
	return mod
}

// RegisterModuleAlias makes alias importable as another already registered
// module, so that both names resolve to the one module object.  "os.path is
// posixpath" then holds, as it does in CPython, instead of the two names
// producing two modules that merely behave alike.
func RegisterModuleAlias(alias, target string) {
	gRuntime.RegisterModuleAlias(alias, target)
}

// GetModuleImplOrNilFor is the alias-aware form used by callers that need a
// specific registered module built outside a context.

// RegisterModuleAlias makes alias importable as the module already
// registered under target, so both names resolve to one ModuleImpl - and so
// to one module object once loaded.  It is what makes "os.path is posixpath"
// hold rather than the two names producing two modules that behave alike.
func (rt *Runtime) RegisterModuleAlias(alias, target string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if impl, ok := rt.ModuleImpls[target]; ok {
		rt.ModuleImpls[alias] = impl
	}
}

// ModulesView is sys.modules: a live mapping over the module store's registry.
//
// A plain dict would have to be kept in step by hand, and it was not - it was
// simply empty, so "sys.modules[\"__main__\"]" raised KeyError for any library
// that looks itself up there.  Reading through to the registry means every
// module the context has loaded is visible, including __main__, with nothing
// to keep in sync.
type ModulesView struct {
	store *ModuleStore
}

// ModulesViewType is the type of sys.modules.
//
// It is named "dict" because that is what CPython reports for sys.modules,
// and code does check.
var ModulesViewType = NewType("dict", "A mapping of module name to the module object.")

func (m *ModulesView) Type() *Type { return ModulesViewType }

// NewModulesView returns a live view of store's registry.
func NewModulesView(store *ModuleStore) *ModulesView {
	return &ModulesView{store: store}
}

func (m *ModulesView) M__len__() (Object, error) {
	return Int(len(m.store.Modules())), nil
}

func (m *ModulesView) M__contains__(item Object) (Object, error) {
	name, err := StrAsString(item)
	if err != nil {
		return nil, err
	}
	_, ok := m.store.ModuleLike(name)
	return NewBool(ok), nil
}

func (m *ModulesView) M__getitem__(key Object) (Object, error) {
	name, err := StrAsString(key)
	if err != nil {
		return nil, err
	}
	mod, ok := m.store.ModuleLike(name)
	if !ok {
		return nil, ExceptionNewf(KeyError, "'%s'", name)
	}
	return mod, nil
}

// M__setitem__ lets a module be registered by name, which is what
// "sys.modules[name] = mod" does in library code and in importlib.
func (m *ModulesView) M__setitem__(key, value Object) (Object, error) {
	name, err := StrAsString(key)
	if err != nil {
		return nil, err
	}
	// A module SUBSTITUTE is accepted: CPython allows anything with a module
	// shape, and pygments puts a "types.ModuleType" subclass here.  Refusing it
	// meant "sys.modules[__name__] = newmod" raised and pip could not import
	// pygments at all.
	if _, ok := value.(*Module); !ok {
		if _, isType := value.(*Type); !isType {
			return nil, ExceptionNewf(TypeError, "sys.modules values must be modules, not '%s'", value.Type().Name)
		}
	}
	m.store.SetModuleLike(name, value)
	return None, nil
}

func (m *ModulesView) M__delitem__(key Object) (Object, error) {
	name, err := StrAsString(key)
	if err != nil {
		return nil, err
	}
	if _, ok := m.store.Modules()[name]; !ok {
		return nil, ExceptionNewf(KeyError, "'%s'", name)
	}
	delete(m.store.Modules(), name)
	return None, nil
}

func (m *ModulesView) M__iter__() (Object, error) {
	names := m.store.ModuleNames()
	items := make(Tuple, 0, len(names))
	for _, n := range names {
		items = append(items, String(n))
	}
	return NewIterator(items), nil
}

func init() {
	ModulesViewType.Dict.Set("get", MustNewMethod("get", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		var key, def Object = nil, None
		if err := UnpackTuple(args, kwargs, "get", 1, 2, &key, &def); err != nil {
			return nil, err
		}
		name, err := StrAsString(key)
		if err != nil {
			return nil, err
		}
		if mod, ok := m0(self).store.Modules()[name]; ok {
			return mod, nil
		}
		return def, nil
	}, 0, "get(name[, default]) -> the module, or default"))
	ModulesViewType.Dict.Set("keys", MustNewMethod("keys", func(self Object, args Tuple) (Object, error) {
		names := m0(self).store.ModuleNames()
		items := make([]Object, 0, len(names))
		for _, n := range names {
			items = append(items, String(n))
		}
		return NewListFromItems(items), nil
	}, 0, "keys() -> the names of the loaded modules"))
}

// m0 asserts the receiver, so the method bodies above stay readable.
func m0(self Object) *ModulesView { return self.(*ModulesView) }

// ModuleNames returns the registered names in sorted order, so that iterating
// sys.modules is stable from one run to the next.
func (store *ModuleStore) ModuleNames() []string {
	names := make([]string, 0, len(store.modules))
	for n := range store.modules {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Check the interfaces are satisfied.
var _ I__len__ = (*ModulesView)(nil)
var _ I__getitem__ = (*ModulesView)(nil)
var _ I__setitem__ = (*ModulesView)(nil)
var _ I__delitem__ = (*ModulesView)(nil)
var _ I__iter__ = (*ModulesView)(nil)
var _ I__contains__ = (*ModulesView)(nil)
