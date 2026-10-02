// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package atexit provides the implementation of python's 'atexit' module.
//
// Handlers registered here run when the interpreter terminates normally -
// which includes a SystemExit, since that is how sys.exit() leaves a
// program - in last-in-first-out order.  An unhandled exception aborts the
// process without running them, and os._exit never reaches the hook at all,
// both as under CPython.
//
// The hook is installed into py by RunExitFuncs, which the interpreter calls
// once the program has finished.
package atexit

import (
	"github.com/vishnukv64/gpython/py"
)

const atexit_doc = `allow programmer to define multiple exit functions to be executed upon normal
program termination.

Calling sys.exit() also triggers the call of these functions, but os._exit()
does not, so no resources are freed or files flushed properly after it.`

// gHandlers is the LIFO list of registered callables: the one registered last
// runs first.
var gHandlers []py.Object

const register_doc = `register(func, *args, **kwargs)

Register a function to be executed upon normal program termination.

    func - function to be called at exit
    args - optional arguments to pass to func
    kwargs - optional keyword arguments to pass to func

func is returned to facilitate usage as a decorator.`

func register(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "register() takes at least 1 argument (0 given)")
	}
	callable := args[0]
	// Defer the "not callable" check to exit time, exactly as CPython does:
	// a name bound to a non-callable is only rejected when the handler runs.
	bound := &exitHandler{fn: callable, args: args[1:], kwargs: kwargs.Copy()}
	gHandlers = append(gHandlers, bound)
	return callable, nil
}

const unregister_doc = `unregister(func)

Unregister an exit function which was previously registered using
atexit.register.

    func - function to be unregistered

Returns the unregistered function, or None when it was not registered.`

func unregister(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "unregister", 1, 1); err != nil {
		return nil, err
	}
	target := args[0]
	for i := len(gHandlers) - 1; i >= 0; i-- {
		h, ok := gHandlers[i].(*exitHandler)
		if !ok {
			continue
		}
		// Functions are compared by identity: the interpreter's "==" does
		// not define it for two function objects, and CPython's unregister
		// matches the very object that was registered.
		same, err := py.Eq(h.fn, target)
		if err != nil {
			same = py.Bool(h.fn == target)
		}
		equal, err := py.ObjectIsTrue(same)
		if err != nil {
			return nil, err
		}
		if equal {
			gHandlers = append(gHandlers[:i], gHandlers[i+1:]...)
			return target, nil
		}
	}
	return py.None, nil
}

const runExitfuncs_doc = `_run_exitfuncs()

Run any registered exit functions, in reverse order of registration.`

func runExitfuncs(self py.Object, args py.Tuple) (py.Object, error) {
	if err := checkArgs(args, py.StringDict{}, "_run_exitfuncs", 0, 0); err != nil {
		return nil, err
	}
	runHandlers()
	return py.None, nil
}

const clear_doc = `_clear()

Clear the list of previously registered exit functions; testing only.`

func clear(self py.Object, args py.Tuple) (py.Object, error) {
	if err := checkArgs(args, py.StringDict{}, "_clear", 0, 0); err != nil {
		return nil, err
	}
	gHandlers = nil
	return py.None, nil
}

// handlerType is the type of the internal registration records.  They are
// never handed to Python code (register returns the user's own callable),
// but the list they live in has to hold py.Objects.
var handlerType = py.NewType("atexit._Handler", "An atexit registration record.")

// exitHandler is one registered callback together with the arguments to call
// it with.
type exitHandler struct {
	fn     py.Object
	args   py.Tuple
	kwargs py.StringDict
}

func (h *exitHandler) Type() *py.Type { return handlerType }

func (h *exitHandler) M__repr__() (py.Object, error) {
	return py.String("<atexit handler>"), nil
}

// runHandlers runs every pending handler in LIFO order.  A handler that
// raises does not stop the others; the error is reported and the run
// continues, which is what CPython's call_ll_exitfuncs does.
func runHandlers() {
	// Take the list first: a handler may register another, and CPython does
	// not run handlers registered during shutdown.
	pending := gHandlers
	gHandlers = nil
	for i := len(pending) - 1; i >= 0; i-- {
		h, ok := pending[i].(*exitHandler)
		if !ok {
			continue
		}
		if _, err := py.Call(h.fn, h.args, h.kwargs); err != nil {
			py.TracebackDump(err)
		}
	}
}

// RunExitFuncs runs any handlers registered with the atexit module.  It is
// called by the interpreter once the program has finished, and is safe to
// call when the module was never imported.
func RunExitFuncs() {
	runHandlers()
}

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "atexit",
			Doc:  atexit_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("register", register, 0, register_doc),
			py.MustNewMethod("unregister", unregister, 0, unregister_doc),
			py.MustNewMethod("_run_exitfuncs", runExitfuncs, 0, runExitfuncs_doc),
			py.MustNewMethod("_clear", clear, 0, clear_doc),
		},
	})
}

// checkArgs validates the positional and keyword argument counts of a
// method whose body reads its arguments directly, rather than unpacking
// them into named variables.  min may be negative, meaning "no lower
// bound".
func checkArgs(args py.Tuple, kwargs py.StringDict, name string, min, max int) error {
	if kwargs.Len() != 0 {
		return py.ExceptionNewf(py.TypeError, "%s() takes no keyword arguments", name)
	}
	n := len(args)
	if min >= 0 && n < min {
		return py.ExceptionNewf(py.TypeError, "%s() takes at least %d argument%s (%d given)", name, min, plural(min), n)
	}
	if max >= 0 && n > max {
		if min == max {
			return py.ExceptionNewf(py.TypeError, "%s() takes exactly %d argument%s (%d given)", name, max, plural(max), n)
		}
		return py.ExceptionNewf(py.TypeError, "%s() takes at most %d argument%s (%d given)", name, max, plural(max), n)
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
