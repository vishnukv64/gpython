// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Module objects

package py

import (
	"fmt"
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
	Info            ModuleInfo
	Methods         []*Method     // Module-bound global method functions
	Globals         StringDict    // Module-bound global variables
	CodeSrc         string        // Module code body (source code to be compiled)
	CodeBuf         []byte        // Module code body (serialized py.Code object)
	Code            *Code         // Module code body
	OnContextClosed func(*Module) // Callback for when a py.Context is closing to release resources
}

// ModuleStore is a container of Module imported into an owning py.Context.
type ModuleStore struct {
	// Registry of installed modules
	modules map[string]*Module
	// Builtin module
	Builtins *Module
	// this should be the frozen module importlib/_bootstrap.py generated
	// by Modules/_freeze_importlib.c into Python/importlib.h
	Importlib *Module

	// frameStack is the chain of frames currently executing in this
	// context.  It is what gives a frame its Back pointer, so that Python
	// code can walk out to its caller (inspect.currentframe().f_back).
	//
	// It is guarded because a context may be shared by goroutines: the
	// frame a goroutine is executing is its own, and the lock only covers
	// the bookkeeping.
	frameMu    sync.Mutex
	frameStack []*Frame
}

// PushFrame records a frame as executing and links it to its caller.
func (s *ModuleStore) PushFrame(f *Frame) {
	s.frameMu.Lock()
	defer s.frameMu.Unlock()
	if n := len(s.frameStack); n > 0 {
		f.Back = s.frameStack[n-1]
	}
	s.frameStack = append(s.frameStack, f)
}

// PopFrame removes a frame that has finished executing.  It drops the
// last entry rather than searching for f, because frames finish in the
// order they start; the identity check guards against a mismatch.
//
// The frame keeps its Back pointer.  A frame that has been returned to the
// caller (sys._getframe() handed out inside a call) stays walkable, which
// is what CPython does - and it is why CPython documents that keeping a
// frame creates a reference cycle.
func (s *ModuleStore) PopFrame(f *Frame) {
	s.frameMu.Lock()
	defer s.frameMu.Unlock()
	if n := len(s.frameStack); n > 0 && s.frameStack[n-1] == f {
		s.frameStack = s.frameStack[:n-1]
	}
}

// CurrentFrame returns the frame that is executing, or nil when the
// interpreter is between calls.
func (s *ModuleStore) CurrentFrame() *Frame {
	s.frameMu.Lock()
	defer s.frameMu.Unlock()
	if n := len(s.frameStack); n > 0 {
		return s.frameStack[n-1]
	}
	return nil
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
	// Register the module
	store.modules[name] = m
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

// Gets a module
func (store *ModuleStore) GetModule(name string) (*Module, error) {
	m, ok := store.modules[name]
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
