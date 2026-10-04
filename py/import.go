// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Import modules

package py

import (
	"strings"
)

func Import(ctx Context, names ...string) error {
	for _, name := range names {
		_, err := ImportModuleLevelObject(ctx, name, NewStringDict(), NewStringDict(), nil, 0)
		if err != nil {
			return err
		}
	}
	return nil
}

// The workings of __import__
//
// __import__(name, globals=None, locals=None, fromlist=(), level=0)
//
// This function is invoked by the import statement. It can be
// replaced (by importing the builtins module and assigning to
// builtins.__import__) in order to change semantics of the import
// statement, but doing so is strongly discouraged as it is usually
// simpler to use import hooks (see PEP 302) to attain the same goals
// and does not cause issues with code which assumes the default
// import implementation is in use. Direct use of __import__() is also
// discouraged in favor of importlib.import_module().
//
// The function imports the module name, potentially using the given
// globals and locals to determine how to interpret the name in a
// package context. The fromlist gives the names of objects or
// submodules that should be imported from the module given by
// name. The standard implementation does not use its locals argument
// at all, and uses its globals only to determine the package context
// of the import statement.
//
// level specifies whether to use absolute or relative imports. 0 (the
// default) means only perform absolute imports. Positive values for
// level indicate the number of parent directories to search relative
// to the directory of the module calling __import__() (see PEP 328
// for the details).
//
// When the name variable is of the form package.module, normally, the
// top-level package (the name up till the first dot) is returned, not
// the module named by name. However, when a non-empty fromlist
// argument is given, the module named by name is returned.
//
// For example, the statement import spam results in bytecode
// resembling the following code:
//
// spam = __import__('spam', globals(), locals(), [], 0)
// The statement import spam.ham results in this call:
//
// spam = __import__('spam.ham', globals(), locals(), [], 0)
//
// Note how __import__() returns the toplevel module here because this
// is the object that is bound to a name by the import statement.
//
// On the other hand, the statement from spam.ham import eggs, sausage
// as saus results in
//
// _temp = __import__('spam.ham', globals(), locals(), ['eggs', 'sausage'], 0)
// eggs = _temp.eggs
// saus = _temp.sausage
//
// Here, the spam.ham module is returned from __import__(). From this
// object, the names to import are retrieved and assigned to their
// respective names.
//
// If you simply want to import a module (potentially within a
// package) by name, use importlib.import_module().
//
// Changed in version 3.3: Negative values for level are no longer
// supported (which also changes the default value to 0).
func ImportModuleLevelObject(ctx Context, name string, globals, locals StringDict, fromlist Tuple, level int) (Object, error) {
	if globals.IsNil() {
		globals = NewStringDict()
	}

	// Resolve explicit relative imports against the importing module's package
	if level > 0 {
		absName, err := resolveRelativeImport(name, level, globals)
		if err != nil {
			return nil, err
		}
		name = absName
	} else {
		name = strings.TrimPrefix(name, ".")
	}

	if name == "" {
		return nil, ExceptionNewf(ValueError, "Empty module name")
	}

	// Import the module and every one of its parent packages.  "import a.b.c"
	// binds "a", but each package on the way must be loaded first and the
	// child bound onto its parent as an attribute.
	parts := strings.Split(name, ".")
	var module *Module
	var err error
	var parent *Module
	for i := range parts {
		module, err = importDotted(ctx, strings.Join(parts[:i+1], "."))
		if err != nil {
			return nil, err
		}
		if parent != nil {
			parent.Globals.Set(parts[i], module)
		}
		parent = module
	}

	// "from pkg import a, b" -- import any of the names that are submodules
	if err := ensureFromlist(ctx, module, name, fromlist); err != nil {
		return nil, err
	}

	if len(fromlist) == 0 {
		// Plain "import a.b.c" returns the top level package "a"
		return importOne(ctx, parts[0])
	}

	return module, nil
}

// importOne imports the module with the given absolute dotted name, returning
// an already loaded instance when there is one.
// metaPathFind consults sys.meta_path, in order, for a finder that can supply
// the named module.
//
// A finder is any object with find_spec(name, path, target) - the PEP 451
// protocol - or the older find_module(name, path).  An error raised INSIDE a
// finder is returned rather than turned into "module not found": a broken
// finder must be visible, not silently skipped.
//
// A loader that supplies code is executed and the module returned.  A finder
// that only identifies the module - a namespace package - leaves an empty
// module recorded under that name, which is what CPython does with no source to
// run.
func metaPathFind(ctx Context, name string) (*Module, error) {
	metaPath := metaPathList(ctx)
	if metaPath == nil {
		return nil, nil
	}
	iter, err := Iter(metaPath)
	if err != nil {
		return nil, nil
	}
	for {
		finder, err := Next(iter)
		if err != nil {
			if IsException(StopIteration, err) {
				return nil, nil
			}
			return nil, err
		}
		spec, err := callFinder(finder, name)
		if err != nil {
			return nil, err
		}
		if spec == nil || spec == None {
			continue
		}
		return moduleFromSpec(ctx, name, spec)
	}
}

// attrOrNil reads an attribute, giving nil when the object does not have one.
//
// A finder or loader implements SOME of the protocol, not all of it: a finder
// may have find_spec but no find_module, and a loader may have get_source but
// no get_code.  The absent method is the signal to try the other.
func attrOrNil(o Object, name string) Object {
	v, err := GetAttrString(o, name)
	if err != nil || v == nil {
		return nil
	}
	return v
}

// metaPathList reads sys.meta_path, or nil when there is none.
func metaPathList(ctx Context) Object {
	// DURING STARTUP sys is not registered yet, and the first imports run
	// before it is - so a missing sys means "no user finders", not an error.
	// MustGetModule PANICS in that case, which took the whole process down
	// before a single line of the program ran.
	sysMod, err := ctx.GetModule("sys")
	if err != nil || sysMod == nil {
		return nil
	}
	list, ok := sysMod.Globals.Get("meta_path")
	if !ok {
		return nil
	}
	return list
}

// callFinder asks one finder for a spec, trying find_spec then find_module.
func callFinder(finder Object, name string) (Object, error) {
	if findSpec := attrOrNil(finder, "find_spec"); findSpec != nil {
		return Call(findSpec, Tuple{String(name), None, None}, NewStringDict())
	}
	findModule := attrOrNil(finder, "find_module")
	if findModule == nil {
		return nil, nil
	}
	res, err := Call(findModule, Tuple{String(name), None}, NewStringDict())
	if err != nil {
		return nil, err
	}
	if res == None || res == nil {
		return nil, nil
	}
	// The older protocol answers with a LOADER, which stands in for the spec.
	return res, nil
}

// moduleFromSpec turns what a finder returned into a module.
func moduleFromSpec(ctx Context, name string, spec Object) (*Module, error) {
	loader := attrOrNil(spec, "loader")
	if loader == nil {
		// find_module's answer IS the loader.
		loader = spec
	}
	codeDesc := "<meta_path:" + name + ">"
	if o := attrOrNil(spec, "origin"); o != nil {
		if s, err := StrAsString(o); err == nil && s != "" {
			codeDesc = s
		}
	}

	// A PEP 451 loader with exec_module fills the module in itself, which is
	// the modern protocol and the one a finder written today implements.
	if execModule := attrOrNil(loader, "exec_module"); execModule != nil {
		if module, err := moduleFromSpecAttrs(ctx, name, spec, codeDesc); err == nil {
			if _, err := Call(execModule, Tuple{module}, NewStringDict()); err != nil {
				return nil, err
			}
			return module, nil
		}
	}

	// Otherwise a loader with get_code or get_source, whose code is compiled
	// and run here.
	code, err := loadCodeFrom(loader, name)
	if err != nil {
		return nil, err
	}
	if code == nil {
		// No code to run, so the module is recorded as an empty one.  A later
		// import of the same name then finds it rather than searching again.
		return moduleFromSpecAttrs(ctx, name, spec, codeDesc)
	}
	return RunCode(ctx, code, codeDesc, name)
}

// moduleFromSpecAttrs builds the module a spec describes, applying the spec's
// own attributes the way CPython's _init_module_attrs does.
//
// The attributes matter to the loader: exec_module reads module.__name__, and a
// loader that looks at __spec__ or __path__ must find them.
func moduleFromSpecAttrs(ctx Context, name string, spec Object, codeDesc string) (*Module, error) {
	mod, err := ctx.Store().NewModule(ctx, &ModuleImpl{
		Info: ModuleInfo{Name: name, FileDesc: codeDesc},
	})
	if err != nil {
		return nil, err
	}
	mod.Globals.Set("__name__", String(name))
	if spec != nil {
		mod.Globals.Set("__spec__", spec)
	}
	if origin := attrOrNil(spec, "origin"); origin != nil && origin != None {
		mod.Globals.Set("__file__", origin)
	}
	// submodule_search_locations makes the module a PACKAGE; without it a
	// loader that imports its own submodules cannot.
	if locs := attrOrNil(spec, "submodule_search_locations"); locs != nil && locs != None {
		mod.Globals.Set("__path__", locs)
	}
	return mod, nil
}

// loadCodeFrom asks a loader for a code object, by either protocol method.
func loadCodeFrom(loader Object, name string) (*Code, error) {
	if getCode := attrOrNil(loader, "get_code"); getCode != nil {
		res, err := Call(getCode, Tuple{String(name)}, NewStringDict())
		if err != nil {
			return nil, err
		}
		if res == nil || res == None {
			return nil, nil
		}
		if code, ok := res.(*Code); ok {
			return code, nil
		}
	}
	getSource := attrOrNil(loader, "get_source")
	if getSource == nil {
		return nil, nil
	}
	res, err := Call(getSource, Tuple{String(name)}, NewStringDict())
	if err != nil {
		return nil, err
	}
	src, err := StrAsString(res)
	if err != nil || src == "" {
		return nil, nil
	}
	return Compile(src, "<meta_path:"+name+">", ExecMode, 0, true)
}

func importOne(ctx Context, name string) (*Module, error) {
	// Module already loaded - return that
	if module, err := ctx.GetModule(name); err == nil {
		return module, nil
	}

	// sys.meta_path comes BEFORE the built-in resolution, which is CPython's
	// order: its own builtin and file finders are entries IN meta_path, so a
	// program that inserts a finder at the front gets first refusal.  A finder
	// that raises surfaces its error here rather than being passed over - the
	// point of inserting one is to be believed.
	if m, err := metaPathFind(ctx, name); err != nil {
		return nil, err
	} else if m != nil {
		return m, nil
	}

	// Registered embedded module that has not been loaded into this ctx yet
	if impl := GetModuleImpl(name); impl != nil {
		return ctx.ModuleInit(impl)
	}

	path, isPkg, err := findModule(ctx, name)
	if err != nil {
		return nil, err
	}
	return initModuleFromPath(ctx, name, path, isPkg)
}

// importDotted imports a name that may not have a file of its own, by
// resolving its parent and looking the leaf up as an attribute of it.
//
// That is what makes "import os.path" work when os.path is an attribute of
// os rather than a module on sys.path, which is how os.path is defined
// everywhere outside this interpreter - and the same shape covers
// "import pkg.sub" for a submodule bound onto its package.
func importDotted(ctx Context, name string) (*Module, error) {
	if module, err := ctx.GetModule(name); err == nil {
		return module, nil
	}
	// sys.meta_path is consulted BEFORE the embedded modules, which is
	// CPython's order: its builtin finder is an entry ON meta_path, so a
	// program that inserts its own finder gets first refusal - including over
	// a name this interpreter has an embedded module for.  Putting the
	// embedded check first made an inserted finder unable to override one, so
	// "import netrc" resolved the embedded module and a finder that was
	// supposed to raise was never reached.
	if m, err := metaPathFind(ctx, name); err != nil {
		return nil, err
	} else if m != nil {
		return m, nil
	}
	if impl := GetModuleImpl(name); impl != nil {
		return ctx.ModuleInit(impl)
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		parent, err := importDotted(ctx, name[:i])
		if err != nil {
			return nil, err
		}
		leaf := name[i+1:]
		if child, ok := parent.Globals.GetOrNil(leaf).(*Module); ok {
			return child, nil
		}
		// A module the parent exposes but that has not been registered under
		// its dotted name: give it that name so a later import finds it.
		if child, err := ctx.Store().GetModule(name); err == nil {
			return child, nil
		}
	}
	return importOne(ctx, name)
}

// Straight port of the python code
//
// This calls functins from _bootstrap.py which is a frozen module
//
// Too much functionality for the moment
func XImportModuleLevelObject(ctx Context, nameObj, given_globals, locals, given_fromlist Object, level int) (Object, error) {
	var abs_name string
	var builtins_import Object
	var final_mod Object
	var mod Object
	var PackageObj Object
	var Package string
	var globals StringDict
	var fromlist Tuple
	var ok bool
	var name string
	var err error
	store := ctx.Store()

	// Make sure to use default values so as to not have
	// PyObject_CallMethodObjArgs() truncate the parameter list because of a
	// nil argument.
	if given_globals == nil {
		globals = NewStringDict()
	} else {
		// Only have to care what given_globals is if it will be used
		// for something.
		globals, ok = given_globals.(StringDict)
		if level > 0 && !ok {
			return nil, ExceptionNewf(TypeError, "globals must be a dict")
		}
	}

	if given_fromlist == nil || given_fromlist == None {
		fromlist = Tuple{}
	} else {
		fromlist, err = SequenceTuple(given_fromlist)
		if err != nil {
			return nil, err
		}
	}
	if nameObj == nil {
		return nil, ExceptionNewf(ValueError, "Empty module name")
	}

	// The below code is importlib.__import__() & _gcd_import(), ported to Go
	// for added performance.

	_, ok = nameObj.(String)
	if !ok {
		return nil, ExceptionNewf(TypeError, "module name must be a string")
	}
	name = string(nameObj.(String))

	if level < 0 {
		return nil, ExceptionNewf(ValueError, "level must be >= 0")
	} else if level > 0 {
		PackageObj, ok = globals.Get("__package__")
		if ok && PackageObj != None {
			if _, ok = PackageObj.(String); !ok {
				return nil, ExceptionNewf(TypeError, "package must be a string")
			}
			Package = string(PackageObj.(String))
		} else {
			PackageObj, ok = globals.Get("__name__")
			if !ok {
				return nil, ExceptionNewf(KeyError, "'__name__' not in globals")
			} else if _, ok = PackageObj.(String); !ok {
				return nil, ExceptionNewf(TypeError, "__name__ must be a string")
			}
			Package = string(PackageObj.(String))

			if _, ok = globals.Get("__path__"); !ok {
				i := strings.LastIndex(string(Package), ".")
				if i < 0 {
					Package = ""
				} else {
					Package = Package[:i]
				}
			}
		}

		if _, err = ctx.GetModule(string(Package)); err != nil {
			return nil, ExceptionNewf(SystemError, "Parent module %q not loaded, cannot perform relative import", Package)
		}
	} else { // level == 0 */
		if len(name) == 0 {
			return nil, ExceptionNewf(ValueError, "Empty module name")
		}
		Package = ""
	}

	if level > 0 {
		last_dot := len(Package)
		var base string
		level_up := 1

		for level_up = 1; level_up < level; level_up += 1 {
			last_dot = strings.LastIndex(string(Package[:last_dot]), ".")
			if last_dot < 0 {
				return nil, ExceptionNewf(ValueError, "attempted relative import beyond top-level Package")
			}
		}

		base = Package[:last_dot]

		if len(name) > 0 {
			abs_name = strings.Join([]string{base, name}, ".")
		} else {
			abs_name = base
		}
	} else {
		abs_name = name
	}

	// FIXME _PyImport_AcquireLock()

	// From this point forward, goto error_with_unlock!
	builtins_import, ok = globals.Get("__import__")
	if !ok {
		builtins_import, ok = store.Builtins.Globals.Get("__import__")
		if !ok {
			return nil, ExceptionNewf(ImportError, "__import__ not found")
		}
	}

	mod, err = ctx.GetModule(abs_name)
	if err != nil || mod == None {
		return nil, ExceptionNewf(ImportError, "import of %q halted; None in sys.modules", abs_name)
	} else if err == nil {
		var value Object
		var err error
		initializing := false

		// Optimization: only call _bootstrap._lock_unlock_module() if
		// __initializing__ is true.
		// NOTE: because of this, __initializing__ must be set *before*
		// stuffing the new module in sys.modules.

		value, err = GetAttrString(mod, "__initializing__")
		if err == nil {
			x, err := MakeBool(value)
			if err != nil {
				return nil, err
			}
			initializing = bool(x.(Bool))
		}
		if initializing {
			// _bootstrap._lock_unlock_module() releases the import lock */
			_, err = store.Importlib.Call("_lock_unlock_module", Tuple{String(abs_name)}, NewStringDict())
			if err != nil {
				return nil, err
			}
			//	} else { // not initializing
			// FIXME locking
			// if _PyImport_ReleaseLock() < 0 {
			// 	return nil, ExceptionNewf(RuntimeError, "not holding the import lock")
			// }
		}
	} else {
		// _bootstrap._find_and_load() releases the import lock
		mod, err = store.Importlib.Call("_find_and_load", Tuple{String(abs_name), builtins_import}, NewStringDict())
		if err != nil {
			return nil, err
		}
	}
	// From now on we don't hold the import lock anymore.

	if len(fromlist) == 0 {
		if level == 0 || len(name) > 0 {
			i := strings.Index(name, ".")
			if i < 0 {
				// No dot in module name, simple exit
				final_mod = mod
				goto error
			}
			front := name[:1]

			if level == 0 {
				var err error
				final_mod, err = Call(builtins_import, Tuple{String(front)}, NewStringDict())
				if err != nil {
					return nil, err
				}
			} else {
				cut_off := len(name) - len(front)
				abs_name_len := len(abs_name)
				to_return := abs_name[:abs_name_len-cut_off]
				final_mod, err = ctx.GetModule(to_return)
				if err != nil {
					return nil, ExceptionNewf(KeyError, "%q not in sys.modules as expected", to_return)
				}
			}
		} else {
			final_mod = mod
		}
	} else {
		final_mod, err = store.Importlib.Call("_handle_fromlist", Tuple{mod, fromlist, builtins_import}, NewStringDict())
		if err != nil {
			return nil, err
		}

	}
	goto error

	//error_with_unlock:
	// FIXME defer?
	// if _PyImport_ReleaseLock() < 0 {
	// 	return nil, ExceptionNewf(RuntimeError, "not holding the import lock"
	// }
error:
	// FIXME defer?
	// if final_mod == nil {
	// 	remove_importlib_frames()
	// }
	return final_mod, nil
}

// The actual import code
func BuiltinImport(ctx Context, self Object, args Tuple, kwargs StringDict, currentGlobal StringDict) (Object, error) {
	kwlist := []string{"name", "globals", "locals", "fromlist", "level"}
	var name Object
	var globals Object = currentGlobal
	var locals Object = NewStringDict()
	var fromlist Object = Tuple{}
	var fromlistTuple Tuple
	var level Object = Int(0)

	err := ParseTupleAndKeywords(args, kwargs, "U|OOOi:__import__", kwlist, &name, &globals, &locals, &fromlist, &level)
	if err != nil {
		return nil, err
	}
	levelObj, ok := level.(Int)
	if !ok {
		return nil, ExceptionNewf(TypeError, "__import__() argument 5 must be int, not %s", level.Type().Name)
	}
	levelInt, err := levelObj.GoInt()
	if err != nil {
		return nil, err
	}

	globalsDict, ok := globals.(StringDict)
	if !ok {
		if levelInt > 0 {
			return nil, ExceptionNewf(TypeError, "globals must be a dict")
		}
		globalsDict = NewStringDict()
	}

	localsDict, ok := locals.(StringDict)
	if !ok {
		localsDict = NewStringDict()
	}

	fromlistTuple = Tuple{}
	if fromlist != None {
		fromlistTuple, err = SequenceTuple(fromlist)
		if err != nil {
			return nil, err
		}
	}

	return ImportModuleLevelObject(ctx, string(name.(String)), globalsDict, localsDict, fromlistTuple, levelInt)
}
