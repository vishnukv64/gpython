// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"os"
	"path/filepath"
	"strings"
)

type CompileMode string

const (
	ExecMode   CompileMode = "exec"   // Compile a module
	EvalMode   CompileMode = "eval"   // Compile an expression
	SingleMode CompileMode = "single" // Compile a single (interactive) statement
)

// Context is a gpython environment instance container, providing a high-level mechanism
// for multiple python interpreters to run concurrently without restriction.
//
// Context instances maintain completely independent environments, namely the modules that
// have been imported and their state.  Modules imported into a Context are instanced
// from a parent ModuleImpl.  For example, since Contexts each have their
// own sys module instance, each can set sys.path differently and independently.
//
// If you access a Context from multiple groutines, you are responsible that access is not concurrent,
// with the exception of Close() and Done().
//
// See examples/multi-context and examples/embedding.
type Context interface {

	// Resolves then compiles (if applicable) the given file system pathname into a py.Code ready to be executed.
	ResolveAndCompile(pathname string, opts CompileOpts) (CompileOut, error)

	// Creates a new py.Module instance and initializes ModuleImpl's code in the new module (if applicable).
	ModuleInit(impl *ModuleImpl) (*Module, error)

	// RunCode is a lower-level invocation to execute the given py.Code.
	// Blocks until execution is complete.
	RunCode(code *Code, globals, locals StringDict, closure Tuple) (result Object, err error)

	// Returns the named module for this context (or an error if not found)
	GetModule(moduleName string) (*Module, error)

	// Gereric access to this context's modules / state.
	Store() *ModuleStore

	// Close signals this context is about to go out of scope and any internal resources should be released.
	// Code execution on a py.Context that has been closed will result in an error.
	Close() error

	// Done returns a signal that can be used to detect when this Context has fully closed / completed.
	// If Close() is called while execution in progress, Done() will not signal until execution is complete.
	Done() <-chan struct{}
}

// CompileOpts specifies options for high-level compilation.
type CompileOpts struct {
	UseSysPaths bool   // If set, sys.path will be used to resolve relative pathnames
	CurDir      string // If non-empty, this is the path of the current working directory.  If empty, os.Getwd() is used.
}

// CompileOut the output of high-level compilation -- e.g. ResolveAndCompile()
type CompileOut struct {
	SrcPathname string // Resolved pathname the .py file that was compiled (if applicable)
	PycPathname string // Pathname of the .pyc file read and/or written (if applicable)
	FileDesc    string // Pathname to be used for a a module's "__file__" attrib
	Code        *Code  // Read/Output code object ready for execution
}

// DefaultCoreSysPaths specify default search paths for module sys
// This can be changed during runtime and plays nice with others using DefaultContextOpts()
var DefaultCoreSysPaths = []string{
	".",
	"lib",
	// The standard library embedded in the binary.  It comes after the local
	// entries, so a file on disk can still shadow it.
	EmbeddedLibRoot,
}

// DefaultAuxSysPaths are secondary default search paths for module sys.
// This can be changed during runtime and plays nice with others using DefaultContextOpts()
// They are separated from the default core paths since they the more likley thing you will want to completely replace when using gpython.
var DefaultAuxSysPaths = []string{
	"/usr/lib/python3.4",
	"/usr/local/lib/python3.4/dist-packages",
	"/usr/lib/python3/dist-packages",
}

// ContextOpts specifies fundamental environment and input settings for creating a new py.Context
type ContextOpts struct {
	SysArgs  []string // sys.argv initializer
	SysPaths []string // sys.path initializer
}

var (
	// DefaultContextOpts should be the default opts created for py.NewContext.
	// Calling this ensure that you future proof you code for suggested/default settings.
	DefaultContextOpts = func() ContextOpts {
		opts := ContextOpts{
			SysPaths: DefaultCoreSysPaths,
		}
		opts.SysPaths = append(opts.SysPaths, DefaultAuxSysPaths...)
		// A PYTHONPATH is searched before the built-in defaults, as in CPython.
		if pythonPath := os.Getenv("PYTHONPATH"); pythonPath != "" {
			paths := filepath.SplitList(pythonPath)
			opts.SysPaths = append(paths, opts.SysPaths...)
		}
		return opts
	}

	// NewContext is a high-level call to create a new gpython interpreter context.
	// See type Context interface.
	NewContext func(opts ContextOpts) Context

	// Compiles a python buffer into a py.Code object.
	// Returns a py.Code object or otherwise an error.
	Compile func(src, srcDesc string, mode CompileMode, flags int, dont_inherit bool) (*Code, error)

	// InputHook is an optional function that can be set to provide a custom input
	// mechanism for the input() builtin. If nil, input() reads from sys.stdin.
	// This is used by the REPL to integrate with the liner library.
	InputHook func(prompt string) (string, error)
)

// RunFile resolves the given pathname, compiles as needed, executes the code in the given module, and returns the Module to indicate success.
//
// See RunCode() for description of inModule.
func RunFile(ctx Context, pathname string, opts CompileOpts, inModule interface{}) (*Module, error) {
	return runFile(ctx, pathname, opts, inModule)
}

// runFile is RunFile with the module specification passed through unchanged.
func runFile(ctx Context, pathname string, opts CompileOpts, inModule interface{}) (*Module, error) {
	out, err := ctx.ResolveAndCompile(pathname, opts)
	if err != nil {
		return nil, err
	}

	return RunCode(ctx, out.Code, out.FileDesc, inModule)
}

// RunFileAs runs a file as a module of the given name, giving it __spec__ the
// way an import would.
//
// "python -m pkg" executes pkg/__main__.py as __main__, and CPython gives that
// module a spec naming the PACKAGE - which is how pip's __main__.py decides
// whether it is running from a wheel.  RunFile alone leaves __spec__ unset.
func RunFileAs(ctx Context, pathname string, opts CompileOpts, moduleName string) (*Module, error) {
	return RunFileAsNamed(ctx, pathname, opts, moduleName, moduleName)
}

// runAs carries the two names a "-m" run needs, so that RunCode can give the
// module a __name__ different from its __spec__.name.
type runAs struct {
	RunName  string
	SpecName string
}

// RunFileAsNamed runs a file as the module runName, while giving it a spec
// naming specName.
//
// The two differ for "-m pkg": __name__ is "__main__", because that is what
// the module is being run AS, but __spec__.name is "pkg.__main__", because
// that is what the module IS.  CPython keeps them separate, and both are
// observable - pip's __main__.py switches on __name__ and reads the spec.
func RunFileAsNamed(ctx Context, pathname string, opts CompileOpts, runName, specName string) (*Module, error) {
	mod, err := runFile(ctx, pathname, opts, runAs{RunName: runName, SpecName: specName})
	if err != nil {
		return nil, err
	}
	mod.Globals.Set("__file__", String(pathname))
	return mod, nil
}

// RunSrc compiles the given python buffer and executes it within the given module and returns the Module to indicate success.
//
// See RunCode() for description of inModule.
func RunSrc(ctx Context, pySrc string, pySrcDesc string, inModule interface{}) (*Module, error) {
	if pySrcDesc == "" {
		pySrcDesc = "<run>"
	}
	// ExecMode, not SingleMode: "-c" is a program, not a line of interactive
	// input, and SingleMode compiles exactly ONE statement - everything after
	// the first newline was silently dropped.  "gpython -c 'a\nb'" printed a
	// and exited 0, with no error and no traceback, which is the worst way for
	// a program to be wrong.  CPython compiles -c with Py_file_input.
	code, err := Compile(pySrc+"\n", pySrcDesc, ExecMode, 0, true)
	if err != nil {
		return nil, err
	}

	return RunCode(ctx, code, pySrcDesc, inModule)
}

// RunCode executes the given code object within the given module and returns the Module to indicate success.
//
// If inModule is a *Module, then the code is run in that module.
//
// If inModule is nil, the code is run in a new __main__ module (and the new Module is returned).
//
// If inModule is a string, the code is run in a new module with the given name (and the new Module is returned).
func RunCode(ctx Context, code *Code, codeDesc string, inModule interface{}) (*Module, error) {
	var (
		module     *Module
		moduleName string
		specName   string
		err        error
	)

	createNew := false
	switch mod := inModule.(type) {

	case runAs:
		// "-m pkg": the module is run AS __main__ but IS pkg.__main__.
		moduleName = mod.RunName
		specName = mod.SpecName
		createNew = true
	case string:
		moduleName = mod
		specName = mod
		createNew = true
	case nil:
		createNew = true
	case *Module:
		_, err = ctx.RunCode(code, mod.Globals, mod.Globals, nil)
		module = mod
	default:
		err = ExceptionNewf(TypeError, "unsupported module type: %v", inModule)
	}

	if err == nil && createNew {
		// __package__ is the module's PARENT package, which is what a relative
		// import resolves against (PEP 328).  It was left as None, so
		// "python -m pkg.mod" could not run a file whose first statement is
		// "from .sub import helper" - CPython sets it to "pkg".  The rule is
		// the same as for an ordinary import: the dotted name minus its last
		// component.
		pkg := specName
		if i := strings.LastIndex(pkg, "."); i >= 0 {
			pkg = pkg[:i]
		} else {
			pkg = ""
		}
		moduleImpl := ModuleImpl{
			Info: ModuleInfo{
				Name:     moduleName,
				FileDesc: codeDesc,
			},
			Code: code,
			// __spec__ has to be in place BEFORE the body runs: pip's
			// __main__.py tests it on its first statement, so setting it
			// afterwards is too late.
			PreSetGlobals: func(g StringDict) {
				g.Set("__spec__", NewModuleSpec(specName, String(codeDesc), false))
				g.Set("__package__", String(pkg))
			},
		}
		module, err = ctx.ModuleInit(&moduleImpl)
	}

	if err != nil {
		return nil, err
	}

	return module, nil
}
