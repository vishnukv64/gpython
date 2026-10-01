// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package contextlib provides the implementation of python's 'contextlib'
// module: utilities for the with statement.
//
// contextmanager is the part that carries weight, and it works the way it
// does in CPython: the decorated function is driven as a generator, the with
// body's exception is thrown into it at the yield, and what it yields is
// returned as the value of "as".  That needs generator.throw, which is why
// that was implemented first.
package contextlib

import (
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Utilities for with-statement contexts.`

// GeneratorContextManager is what a @contextmanager function returns: an
// object whose __enter__ advances the generator and whose __exit__ throws
// the body's exception back into it.
type GeneratorContextManager struct {
	gen *py.Generator
}

var GeneratorContextManagerType = py.NewType("contextlib._GeneratorContextManager", "Helper for @contextmanager decorator.")

func (g *GeneratorContextManager) Type() *py.Type { return GeneratorContextManagerType }

// enter starts the generator and returns what it yielded.
func (g *GeneratorContextManager) enter() (py.Object, error) {
	value, err := g.gen.Send(py.None)
	if err != nil {
		if py.IsException(py.StopIteration, err) {
			return nil, py.ExceptionNewf(py.RuntimeError, "generator didn't yield")
		}
		return nil, err
	}
	return value, nil
}

// exit throws the body's exception into the generator, or advances it when
// there was none.  It returns whether the exception was suppressed, which is
// the value of the generator's return.
func (g *GeneratorContextManager) exit(excType, excValue, traceback py.Object) (py.Object, error) {
	if excType == py.None || excType == nil {
		// No exception: the generator should finish.
		_, err := g.gen.Send(py.None)
		if err != nil {
			if py.IsException(py.StopIteration, err) {
				return py.False, nil
			}
			return nil, err
		}
		return nil, py.ExceptionNewf(py.RuntimeError, "generator didn't stop")
	}

	// An exception: throw it in.  The generator either catches it and stops
	// (suppressing), catches it and yields (a RuntimeError, as in CPython),
	// or does not catch it and the same exception comes back out.
	typ, ok := excType.(*py.Type)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "exception type must be a class")
	}
	value, err := g.gen.Throw(py.Tuple{typ, excValue}, nil)
	if err != nil {
		if py.IsException(py.StopIteration, err) {
			// The generator handled it and finished: suppressed.
			return py.True, nil
		}
		if py.IsException(typ, err) {
			// The generator did not handle it; return False so the with
			// statement re-raises it, rather than propagating from here.
			return py.False, nil
		}
		return nil, err
	}
	_ = value
	return nil, py.ExceptionNewf(py.RuntimeError, "generator didn't stop after throw()")
}

func init() {
	GeneratorContextManagerType.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*GeneratorContextManager).enter()
	}, 0, "Start the generator and return what it yielded.")

	GeneratorContextManagerType.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var excType, excValue = py.Object(py.None), py.Object(py.None)
		var traceback py.Object = py.None
		if err := py.UnpackTuple(args, nil, "__exit__", 0, 3, &excType, &excValue, &traceback); err != nil {
			return nil, err
		}
		return self.(*GeneratorContextManager).exit(excType, excValue, traceback)
	}, 0, "Throw the body's exception into the generator.")

	globals := py.StringDict{}

	globals["contextmanager"] = py.MustNewMethod("contextmanager", contextmanager, 0, contextmanager_doc)
	globals["AbstractContextManager"] = AbstractContextManagerType
	globals["AbstractAsyncContextManager"] = AbstractContextManagerType
	globals["ExitStack"] = ExitStackType
	globals["nullcontext"] = py.MustNewMethod("nullcontext", nullcontext, 0, "Context manager that does no additional processing.")
	globals["closing"] = py.MustNewMethod("closing", closing, 0, "Context to automatically close something at the end of a block.")
	globals["redirect_stdout"] = py.MustNewMethod("redirect_stdout", redirectStdout, 0, "Context manager for temporarily redirecting sys.stdout.")
	globals["redirect_stderr"] = py.MustNewMethod("redirect_stderr", redirectStderr, 0, "Context manager for temporarily redirecting sys.stderr.")
	globals["suppress"] = py.MustNewMethod("suppress", suppress, 0, "Context manager to suppress specified exceptions.")
	globals["ContextDecorator"] = ContextDecoratorType

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "contextlib",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

const contextmanager_doc = `@contextmanager

Decorator that converts a generator function into a context manager factory.

The function must yield exactly once: everything before the yield runs on
entry and the yielded value becomes the "as" target, and everything after
runs on exit, with the body's exception thrown in at the yield.`

func contextmanager(self py.Object, args py.Tuple) (py.Object, error) {
	var fn py.Object
	if err := py.UnpackTuple(args, nil, "contextmanager", 1, 1, &fn); err != nil {
		return nil, err
	}
	if _, ok := fn.(*py.Function); !ok {
		return nil, py.ExceptionNewf(py.TypeError, "contextmanager() argument must be a generator function")
	}
	return &contextmanagerFactory{fn: fn}, nil
}

// contextmanagerFactory is the decorated function: calling it produces the
// context manager.
type contextmanagerFactory struct {
	fn py.Object
}

var contextmanagerFactoryType = py.NewType("contextlib.contextmanager", "A context manager factory.")

func (c *contextmanagerFactory) Type() *py.Type { return contextmanagerFactoryType }

func (c *contextmanagerFactory) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	genObj, err := py.Call(c.fn, args, kwargs)
	if err != nil {
		return nil, err
	}
	gen, ok := genObj.(*py.Generator)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "contextmanager() decorated function must be a generator function")
	}
	return &GeneratorContextManager{gen: gen}, nil
}

// M__get__ makes a decorated function work as a method, binding self.
func (c *contextmanagerFactory) M__get__(instance, owner py.Object) (py.Object, error) {
	if instance == nil || instance == py.None {
		return c, nil
	}
	return &boundContextmanagerFactory{factory: c, self: instance}, nil
}

type boundContextmanagerFactory struct {
	factory *contextmanagerFactory
	self    py.Object
}

func (b *boundContextmanagerFactory) Type() *py.Type { return contextmanagerFactoryType }

func (b *boundContextmanagerFactory) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	full := append(py.Tuple{b.self}, args...)
	return b.factory.M__call__(full, kwargs)
}

var (
	_ py.I__call__ = (*contextmanagerFactory)(nil)
	_ py.I__get__  = (*contextmanagerFactory)(nil)
	_ py.I__call__ = (*boundContextmanagerFactory)(nil)
	_ py.I__get__  = (*GeneratorContextManager)(nil)
)

// M__get__ lets a context manager produced by a method be bound like one.
func (g *GeneratorContextManager) M__get__(instance, owner py.Object) (py.Object, error) {
	return g, nil
}

// AbstractContextManager is the base class of the context managers, used as
// an annotation and occasionally as a base.
var AbstractContextManagerType = py.NewType("contextlib.AbstractContextManager", "Abstract base class for context managers.")

// ContextDecorator is the base class for context managers usable as
// decorators.
var ContextDecoratorType = py.NewType("contextlib.ContextDecorator", "A base class or mixin that enables context managers to work as decorators.")

func init() {
	AbstractContextManagerType.Flags |= py.TPFLAGS_BASETYPE
	ContextDecoratorType.Flags |= py.TPFLAGS_BASETYPE
}

// ---------------------------------------------------------------------------
// ExitStack

const exitstack_doc = `Context manager for dynamic management of a stack of exit callbacks.

Callbacks are unwound in reverse order on exit, and the body's exception is
passed to each of them, so a callback may suppress it by returning True.`

// ExitStack is a stack of cleanup callbacks, unwound on exit.
type ExitStack struct {
	callbacks []exitCallback
}

type exitCallback struct {
	fn   py.Object
	args py.Tuple
	kwds py.StringDict
}

var ExitStackType = py.NewTypeX("contextlib.ExitStack", exitstack_doc, func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return &ExitStack{}, nil
}, nil)

func (s *ExitStack) Type() *py.Type { return ExitStackType }

// push records a callback to run on exit.
func (s *ExitStack) push(fn py.Object, args py.Tuple, kwds py.StringDict) py.Object {
	s.callbacks = append(s.callbacks, exitCallback{fn: fn, args: args, kwds: kwds})
	// The callback itself is returned, which is what lets
	// "stack.callback(f)()" work as a decorator.
	return fn
}

// unwind runs the callbacks in reverse order, stopping as soon as one
// suppresses the exception.
func (s *ExitStack) unwind(excType, excValue, traceback py.Object) (py.Object, error) {
	for i := len(s.callbacks) - 1; i >= 0; i-- {
		cb := s.callbacks[i]
		if excType == py.None || excType == nil {
			if _, err := py.Call(cb.fn, cb.args, cb.kwds); err != nil {
				return nil, err
			}
			continue
		}
		all := append(append(py.Tuple{}, cb.args...), excType, excValue, traceback)
		res, err := py.Call(cb.fn, all, cb.kwds)
		if err != nil {
			return nil, err
		}
		if res == py.True {
			return py.True, nil
		}
	}
	if excType != py.None && excType != nil {
		return py.False, nil
	}
	return py.None, nil
}

func init() {
	ExitStackType.Dict["push"] = py.MustNewMethod("push", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*ExitStack)
		var cm py.Object
		if err := py.UnpackTuple(args, nil, "push", 1, 1, &cm); err != nil {
			return nil, err
		}
		enter, err := py.GetAttrString(cm, "__enter__")
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "object does not support the context manager protocol")
		}
		exit, err := py.GetAttrString(cm, "__exit__")
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "object does not support the context manager protocol")
		}
		value, err := py.Call(enter, py.Tuple{}, nil)
		if err != nil {
			return nil, err
		}
		s.push(exit, py.Tuple{}, nil)
		return value, nil
	}, 0, "Enter a context manager and register its exit on the stack.")

	ExitStackType.Dict["enter_context"] = ExitStackType.Dict["push"]

	ExitStackType.Dict["callback"] = py.MustNewMethod("callback", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*ExitStack)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "callback() needs at least one argument")
		}
		return s.push(args[0], args[1:], nil), nil
	}, 0, "Register a callback to be called on exit.")

	ExitStackType.Dict["push_async_callback"] = ExitStackType.Dict["callback"]

	ExitStackType.Dict["close"] = py.MustNewMethod("close", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*ExitStack)
		s.callbacks = nil
		return py.None, nil
	}, 0, "Immediately unwind the stack without an exception.")

	ExitStackType.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Return the stack itself.")

	ExitStackType.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		// The with statement passes three arguments; a direct call may pass
		// none, which means the same as (None, None, None).
		var excType, excValue, traceback = py.Object(py.None), py.Object(py.None), py.Object(py.None)
		if err := py.UnpackTuple(args, nil, "__exit__", 0, 3, &excType, &excValue, &traceback); err != nil {
			return nil, err
		}
		s := self.(*ExitStack)
		res, err := s.unwind(excType, excValue, traceback)
		// The stack is not reusable after it has been unwound.
		s.callbacks = nil
		return res, err
	}, 0, "Unwind the stack, passing the exception to each callback.")
}

// ---------------------------------------------------------------------------
// nullcontext, closing, suppress, redirect_stdout

const nullcontext_doc = `nullcontext([enter_result]) -> ContextManager

Return a context manager that returns enter_result from __enter__, but
otherwise does nothing.`

func nullcontext(self py.Object, args py.Tuple) (py.Object, error) {
	var value py.Object = py.None
	if err := py.UnpackTuple(args, nil, "nullcontext", 0, 1, &value); err != nil {
		return nil, err
	}
	return &nullContext{value: value}, nil
}

type nullContext struct{ value py.Object }

var nullContextType = py.NewType("contextlib.nullcontext", nullcontext_doc)

func (n *nullContext) Type() *py.Type { return nullContextType }

func init() {
	nullContextType.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*nullContext).value, nil
	}, 0, "Return the value the context was created with.")
	nullContextType.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.False, nil
	}, 0, "Do nothing.")
}

const closing_doc = `closing(thing)

Return a context manager that closes thing upon completion of the block.`

func closing(self py.Object, args py.Tuple) (py.Object, error) {
	var thing py.Object
	if err := py.UnpackTuple(args, nil, "closing", 1, 1, &thing); err != nil {
		return nil, err
	}
	return &closingContext{thing: thing}, nil
}

type closingContext struct{ thing py.Object }

var closingContextType = py.NewType("contextlib.closing", closing_doc)

func (c *closingContext) Type() *py.Type { return closingContextType }

func init() {
	closingContextType.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*closingContext).thing, nil
	}, 0, "Return the wrapped object.")

	closingContextType.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*closingContext)
		close, err := py.GetAttrString(c.thing, "close")
		if err != nil {
			// An object with no close() is closed as far as this can tell,
			// which is what CPython reports for a missing attribute.
			return nil, err
		}
		if _, err := py.Call(close, py.Tuple{}, nil); err != nil {
			return nil, err
		}
		return py.False, nil
	}, 0, "Close the wrapped object.")
}

const suppress_doc = `suppress(*exceptions)

Context manager to suppress the specified exceptions.`

func suppress(self py.Object, args py.Tuple) (py.Object, error) {
	// The argument tuple must be COPIED: the tuple a call is given can be a
	// slice of the interpreter's value stack, which is cleared when the call
	// returns, so keeping it left the exception types reading as nil and
	// nothing was ever suppressed.
	exceptions := make(py.Tuple, len(args))
	copy(exceptions, args)
	return &suppressContext{exceptions: exceptions}, nil
}

type suppressContext struct{ exceptions py.Tuple }

var suppressContextType = py.NewType("contextlib.suppress", suppress_doc)

func (s *suppressContext) Type() *py.Type { return suppressContextType }

func init() {
	suppressContextType.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "Do nothing.")

	suppressContextType.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*suppressContext)
		var excType, excValue, traceback py.Object
		if err := py.UnpackTuple(args, nil, "__exit__", 0, 3, &excType, &excValue, &traceback); err != nil {
			return nil, err
		}
		if excType == py.None || excType == nil {
			return py.False, nil
		}
		typ, ok := excType.(*py.Type)
		if !ok {
			return py.False, nil
		}
		for _, wanted := range s.exceptions {
			if wantedType, ok := wanted.(*py.Type); ok {
				for t := typ; t != nil; t = t.Base {
					if t == wantedType {
						return py.True, nil
					}
				}
			}
		}
		return py.False, nil
	}, 0, "Suppress the exception if it is one of the listed types.")
}

// redirectStdout and redirectStderr swap the sys stream for the block.
func redirectStdout(self py.Object, args py.Tuple) (py.Object, error) {
	return redirectStream("stdout", args)
}

func redirectStderr(self py.Object, args py.Tuple) (py.Object, error) {
	return redirectStream("stderr", args)
}

func redirectStream(name string, args py.Tuple) (py.Object, error) {
	var target py.Object
	if err := py.UnpackTuple(args, nil, "redirect", 1, 1, &target); err != nil {
		return nil, err
	}
	return &redirectContext{name: name, new: target}, nil
}

type redirectContext struct {
	name  string
	new   py.Object
	old   py.Object
	saved bool
}

var redirectContextType = py.NewType("contextlib._RedirectStream", "Context manager for temporarily redirecting a stream.")

func (r *redirectContext) Type() *py.Type { return redirectContextType }

func (r *redirectContext) setStream(value py.Object) error {
	mod := py.GetModuleImplOrNil("sys")
	if mod == nil {
		return py.ExceptionNewf(py.RuntimeError, "sys is not available")
	}
	if !r.saved {
		r.old = mod.Globals[r.name]
		r.saved = true
	}
	mod.Globals[r.name] = value
	return nil
}

func init() {
	redirectContextType.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		r := self.(*redirectContext)
		if err := r.setStream(r.new); err != nil {
			return nil, err
		}
		return r.new, nil
	}, 0, "Redirect the stream and return the new one.")

	redirectContextType.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		r := self.(*redirectContext)
		if r.old != nil {
			if err := r.setStream(r.old); err != nil {
				return nil, err
			}
		}
		return py.False, nil
	}, 0, "Restore the previous stream.")
}
