// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Generator objects

package py

// A python Generator object
type Generator struct {
	// Note: gi_frame can be NULL if the generator is "finished"
	Frame *Frame

	// True if generator is being executed.
	Running bool

	// The code object backing the generator
	Code *Code

	// List of weak reference.
	Weakreflist Object
}

var GeneratorType = NewType("generator", "generator object")

func init() {
	// FIXME would like to do this with introspection
	GeneratorType.Dict.Set("send", MustNewMethod("send", func(self Object, value Object) (Object, error) {
		return self.(*Generator).Send(value)
	}, 0, "send(arg) -> send 'arg' into generator,\nreturn next yielded value or raise StopIteration."))
	GeneratorType.Dict.Set("throw", MustNewMethod("throw", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(*Generator).Throw(args, kwargs)
	}, 0, "throw(typ[,val[,tb]]) -> raise exception in generator,\nreturn next yielded value or raise StopIteration."))
	GeneratorType.Dict.Set("close", MustNewMethod("close", func(self Object) (Object, error) {
		return self.(*Generator).Close()
	}, 0, "close() -> raise GeneratorExit inside generator."))
}

// Type of this object
func (o *Generator) Type() *Type {
	return GeneratorType
}

// Define a new generator
func NewGenerator(frame *Frame) *Generator {
	g := &Generator{
		Frame:   frame,
		Running: false,
		Code:    frame.Code,
	}
	return g
}

func (it *Generator) M__iter__() (Object, error) {
	return it, nil
}

// generator.__next__()
//
// Starts the execution of a generator function or resumes it at the
// last executed yield expression. When a generator function is
// resumed with a __next__() method, the current yield expression
// always evaluates to None. The execution then continues to the next
// yield expression, where the generator is suspended again, and the
// value of the expression_list is returned to next()‘s caller. If the
// generator exits without yielding another value, a StopIteration
// exception is raised.
//
// This method is normally called implicitly, e.g. by a for loop, or by the built-in next() function.
func (it *Generator) M__next__() (Object, error) {
	return it.Send(None)
}

// generator.send(value)
//
// Resumes the execution and “sends” a value into the generator
// function. The value argument becomes the result of the current
// yield expression. The send() method returns the next value yielded
// by the generator, or raises StopIteration if the generator exits
// without yielding another value. When send() is called to start the
// generator, it must be called with None as the argument, because
// there is no yield expression that could receive the value.
func (it *Generator) Send(arg Object) (Object, error) {
	if it.Running {
		return nil, ExceptionNewf(ValueError, "generator already executing")
	}
	if it.Frame.Lasti == 0 {
		if arg != None {
			return nil, ExceptionNewf(TypeError, "can't send non-None value to a just-started generator")
		}
	} else {
		// If already returned a non yield value then stop
		if !it.Frame.Yielded {
			return nil, StopIteration
		}
		// Push arg onto the frame's value stack
		it.Frame.Stack = append(it.Frame.Stack, arg)
	}
	it.Running = true
	res, err := VmRunFrame(it.Frame)
	it.Running = false
	if err != nil {
		return nil, err
	}
	if it.Frame.Yielded {
		return res, nil
	}
	return nil, StopIteration
}

// generator.throw(type[, value[, traceback]])
//
// Raises an exception of type type at the point where generator was
// paused, and returns the next value yielded by the generator
// function. If the generator exits without yielding another value, a
// StopIteration exception is raised. If the generator function does
// not catch the passed-in exception, or raises a different exception,
// then that exception propagates to the caller.
// generator.throw(typ[, value[, traceback]])
//
// Raises an exception at the point where the generator is paused, and
// returns the next value it yields.  If the generator does not catch it, the
// exception propagates to the caller.
//
// The mechanism is to set the frame's exception and jump to the same unwind
// path the VM takes for a raised exception (whyException), which is what lets
// an "except" inside the generator catch it.
func (it *Generator) Throw(args Tuple, kwargs StringDict) (Object, error) {
	if it.Running {
		return nil, ExceptionNewf(ValueError, "generator already executing")
	}
	if !it.Frame.Yielded {
		// The generator has finished or never started: the exception is
		// simply raised in the caller, which is what CPython does.
		return it.throwIntoCaller(args)
	}

	var exc *Exception
	switch len(args) {
	case 0:
		return nil, ExceptionNewf(TypeError, "throw() takes at least 1 argument")
	case 1:
		// The single argument may be an exception instance or a class.
		switch v := args[0].(type) {
		case *Exception:
			exc = v
		case *Type:
			newExc, err := ExceptionNew(v, Tuple{}, NewStringDict())
			if err != nil {
				return nil, err
			}
			exc, _ = newExc.(*Exception)
		default:
			return nil, ExceptionNewf(TypeError, "exceptions must be classes or instances")
		}
	default:
		typ, ok := args[0].(*Type)
		if !ok {
			return nil, ExceptionNewf(TypeError, "exceptions must be classes or instances")
		}
		// throw(typ, value): when the value is already an exception
		// instance it IS the exception to raise - rebuilding one from it
		// would make the instance its sole argument, so str(e) would try to
		// format an exception into a string and fail.  Otherwise the
		// remaining arguments are the new exception's arguments, passed
		// through as they are, since ExceptionNew takes the args tuple.
		if instance, ok := args[1].(*Exception); ok {
			exc = instance
		} else {
			newExc, err := ExceptionNew(typ, args[1:], NewStringDict())
			if err != nil {
				return nil, err
			}
			exc, _ = newExc.(*Exception)
		}
	}

	it.Frame.Yielded = false
	it.Running = true
	it.Frame.PendingException = exc

	res, err := VmRunFrame(it.Frame)
	it.Running = false
	it.Frame.PendingException = nil
	if err == GeneratorExit || err == StopIteration {
		return nil, StopIteration
	}
	if err != nil {
		return nil, err
	}
	if it.Frame.Yielded {
		return res, nil
	}
	return nil, StopIteration
}

// throwIntoCaller raises the given exception in the calling code, for a
// generator that is not suspended.
func (it *Generator) throwIntoCaller(args Tuple) (Object, error) {
	if len(args) == 0 {
		return nil, ExceptionNewf(TypeError, "throw() takes at least 1 argument")
	}
	switch v := args[0].(type) {
	case *Exception:
		return nil, v
	case *Type:
		newExc, err := ExceptionNew(v, Tuple{}, NewStringDict())
		if err != nil {
			return nil, err
		}
		if e, ok := newExc.(*Exception); ok {
			return nil, e
		}
		return nil, ExceptionNewf(TypeError, "exceptions must be classes or instances")
	}
	return nil, ExceptionNewf(TypeError, "exceptions must be classes or instances")
}

// generator.close()
//
// Raises a GeneratorExit at the point where the generator function
// was paused. If the generator function then raises StopIteration (by
// exiting normally, or due to already being closed) or GeneratorExit
// (by not catching the exception), close returns to its caller. If
// the generator yields a value, a RuntimeError is raised. If the
// generator raises any other exception, it is propagated to the
// caller. close() does nothing if the generator has already exited
// due to an exception or normal exit.
func (it *Generator) Close() (Object, error) {
	return nil, NotImplementedError
}

// Check interface is satisfied
var _ I_generator = (*Generator)(nil)
