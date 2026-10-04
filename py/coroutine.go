// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Coroutine objects (PEP 492): what calling an "async def" function returns.

package py

import "fmt"

// A python coroutine object, returned by calling an "async def" function.
//
// It shares ALL the frame mechanics with the generator (the coroutine body
// IS executed as a suspended frame, driven by send/throw), while presenting
// the "coroutine" type and the coroutine protocol: it is NOT an iterator
// (no __iter__/__next__), and its __await__ makes it awaitable.
type Coroutine struct {
	Generator
	// Awaited records that this coroutine has been awaited (or is being
	// awaited): CPython's coro->cr_await.  A second "await" on it raises
	// "cannot reuse already awaited coroutine".
	Awaited bool
}

var CoroutineType = NewType("coroutine", "coroutine object")

func init() {
	CoroutineType.Dict.Set("send", MustNewMethod("send", func(self Object, value Object) (Object, error) {
		return coroutineOf(self).Send(value)
	}, 0, "send(arg) -> send 'arg' into coroutine,\nreturn next yielded value or raise StopIteration."))
	CoroutineType.Dict.Set("throw", MustNewMethod("throw", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return coroutineOf(self).Throw(args, kwargs)
	}, 0, "throw(typ[,val[,tb]]) -> raise exception in coroutine,\nreturn next yielded value or raise StopIteration."))
	CoroutineType.Dict.Set("close", MustNewMethod("close", func(self Object) (Object, error) {
		return coroutineOf(self).Close()
	}, 0, "close() -> raise GeneratorExit inside coroutine."))
}

// coroutineOf unwraps the *Coroutine a coroutine method was called on.  The
// type assertion is on the OUTER type because the Coroutine embeds the
// Generator's whole state: asserting *Generator would silently accept the
// embedded part and drop the coroutine's own invariants (Awaited).
func coroutineOf(o Object) *Coroutine {
	if c, ok := o.(*Coroutine); ok {
		return c
	}
	panic(fmt.Sprintf("coroutine method on %T", o))
}

// Type of this object
func (o *Coroutine) Type() *Type {
	return CoroutineType
}

// M__repr__ mirrors CPython: "<coroutine object f at 0x...>".
func (o *Coroutine) M__repr__() (Object, error) {
	name := ""
	if o.Frame != nil && o.Frame.Code != nil {
		name = o.Frame.Code.Name
	}
	return String(fmt.Sprintf("<coroutine object %s at %p>", name, o)), nil
}

// Coroutine.Send mirrors Generator.Send with coroutine error wording.
func (it *Coroutine) Send(arg Object) (Object, error) {
	if it.Running {
		return nil, ExceptionNewf(ValueError, "coroutine already executing")
	}
	if it.Frame.Lasti == 0 {
		if arg != None {
			return nil, ExceptionNewf(TypeError, "can't send non-None value to a just-started coroutine")
		}
	} else {
		// If already returned a non yield value then stop
		if !it.Frame.Yielded {
			if it.Awaited {
				return nil, ExceptionNewf(RuntimeError, "cannot reuse already awaited coroutine")
			}
			return nil, StopIteration
		}
		it.Frame.Stack = append(it.Frame.Stack, arg)
	}
	// CPython's send() sets cr_await (coroutines may not be driven twice -
	// CPython 3.14 raises "cannot reuse already awaited coroutine" on any
	// send after the coroutine has finished, awaited or not).
	it.Awaited = true
	it.Running = true
	res, err := VmRunFrame(it.Frame)
	it.Running = false
	if err != nil {
		return nil, err
	}
	if it.Frame.Yielded {
		return res, nil
	}
	// The coroutine RETURNED: its return value rides StopIteration.value,
	// which is exactly what "await coro" reads out.
	return nil, stopIterationWith(res)
}

// M__await__ makes a coroutine awaitable: "await coro" drives it via the
// generator protocol, and the coroutine itself answers __next__ so a
// driver can send values in.
func (it *Coroutine) M__await__() (Object, error) {
	if it.Running {
		return nil, ExceptionNewf(RuntimeError, "coroutine is being awaited already")
	}
	if it.Frame.Lasti == 0 {
		it.Awaited = true
		return it, nil
	}
	if it.Frame.Yielded {
		return it, nil
	}
	return nil, ExceptionNewf(RuntimeError, "cannot reuse already awaited coroutine")
}

// Coroutine deliberately overrides the iterator protocol the Generator
// embeds: "next(c)" is a TypeError "coroutine is not an iterator" and
// "iter(c)"/"for x in c:" is TypeError "coroutine is not iterable" in
// CPython.  The await driving goes through Send (see do_YIELD_FROM and
// do_FOR_ITER_AEXPR), not through the iterator protocol.
func (o *Coroutine) M__iter__() (Object, error) {
	return nil, ExceptionNewf(TypeError, "'coroutine' object is not iterable")
}

func (o *Coroutine) M__next__() (Object, error) {
	return nil, ExceptionNewf(TypeError, "'coroutine' object is not an iterator")
}

// Coroutine.Close is Generator.Close plus the coroutine's "already awaited"
// state: a closed coroutine may not be sent into again, which CPython
// reports as "cannot reuse already awaited coroutine" (close sets the
// equivalent of cr_await).
func (it *Coroutine) Close() (Object, error) {
	res, err := it.Generator.Close()
	it.Awaited = true
	return res, err
}

// Define a new coroutine owning the ready to run frame
func NewCoroutine(frame *Frame) *Coroutine {
	return &Coroutine{Generator: Generator{
		Frame:   frame,
		Running: false,
		Code:    frame.Code,
	}}
}
