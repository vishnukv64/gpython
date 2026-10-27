// Async generators (PEP 525): "async def" functions containing "yield".

package py

import (
	"fmt"
	"sync"
)

// AsyncGenWrappedValue marks a value produced by "yield" inside an async
// generator, to tell it apart from a suspension caused by "await" (which
// travels up to the event loop untouched).
type AsyncGenWrappedValue struct{ Value Object }

var AsyncGenWrappedValueType = NewType("async_generator_wrapped_value", "")

func (o *AsyncGenWrappedValue) Type() *Type { return AsyncGenWrappedValueType }

// sys.set_asyncgen_hooks state (process-wide; CPython keeps it per thread).
var (
	asyncgenHookMu sync.Mutex
	asyncgenFirst  Object = None
	asyncgenFinal  Object = None
)

// SetAsyncGenHooks installs the hooks; nil leaves one unchanged.
func SetAsyncGenHooks(first, final Object) {
	asyncgenHookMu.Lock()
	defer asyncgenHookMu.Unlock()
	if first != nil {
		asyncgenFirst = first
	}
	if final != nil {
		asyncgenFinal = final
	}
}

// GetAsyncGenHooks returns (firstiter, finalizer).
func GetAsyncGenHooks() (Object, Object) {
	asyncgenHookMu.Lock()
	defer asyncgenHookMu.Unlock()
	return asyncgenFirst, asyncgenFinal
}

// AsyncGenerator is the object an async generator function returns.
type AsyncGenerator struct {
	Generator
	hooksInit bool
	finalizer Object
	closed    bool // aclose()d or exhausted
	running   bool // an asend/athrow awaitable is in progress
}

var AsyncGeneratorType = NewType("async_generator", "async generator object")

func NewAsyncGenerator(frame *Frame) *AsyncGenerator {
	return &AsyncGenerator{Generator: Generator{Frame: frame, Code: frame.Code}}
}

func (o *AsyncGenerator) Type() *Type { return AsyncGeneratorType }

func (o *AsyncGenerator) M__repr__() (Object, error) {
	return String(fmt.Sprintf("<async_generator object %s at %p>", o.Frame.Code.Name, o)), nil
}

func (o *AsyncGenerator) M__iter__() (Object, error) {
	return nil, ExceptionNewf(TypeError, "'async_generator' object is not iterable")
}

func (o *AsyncGenerator) M__next__() (Object, error) {
	return nil, ExceptionNewf(TypeError, "'async_generator' object is not an iterator")
}

func (o *AsyncGenerator) M__aiter__() (Object, error) { return o, nil }

func (o *AsyncGenerator) M__anext__() (Object, error) { return o.newAwaitable(agSend, None) }

func (o *AsyncGenerator) done() bool {
	return o.closed || (o.Frame.Lasti != 0 && !o.Frame.Yielded)
}

func (o *AsyncGenerator) initHooks() error {
	if o.hooksInit {
		return nil
	}
	o.hooksInit = true
	first, final := GetAsyncGenHooks()
	o.finalizer = final
	if first != None {
		_, err := Call(first, Tuple{o}, StringDict{})
		return err
	}
	return nil
}

const (
	agSend = iota
	agThrow
	agClose
)

func (o *AsyncGenerator) newAwaitable(kind int, arg Object) (Object, error) {
	if err := o.initHooks(); err != nil {
		return nil, err
	}
	return &asyncGenAwaitable{gen: o, kind: kind, arg: arg}, nil
}

func init() {
	ag := func(self Object) *AsyncGenerator { return self.(*AsyncGenerator) }
	AsyncGeneratorType.Dict.Set("asend", MustNewMethod("asend", func(self, v Object) (Object, error) {
		return ag(self).newAwaitable(agSend, v)
	}, 0, "asend(v) -> send 'v' in generator."))
	AsyncGeneratorType.Dict.Set("athrow", MustNewMethod("athrow", func(self Object, args Tuple) (Object, error) {
		if len(args) == 0 {
			return nil, ExceptionNewf(TypeError, "athrow expected at least 1 argument, got 0")
		}
		// args aliases the caller's value stack, which is reused before the awaitable runs
		return ag(self).newAwaitable(agThrow, append(Tuple(nil), args...))
	}, 0, "athrow(value) -> raise exception in generator."))
	AsyncGeneratorType.Dict.Set("aclose", MustNewMethod("aclose", func(self Object) (Object, error) {
		return ag(self).newAwaitable(agClose, None)
	}, 0, "aclose() -> raise GeneratorExit inside generator."))

	AsyncGenAwaitableType.Dict.Set("send", MustNewMethod("send", func(self, v Object) (Object, error) {
		return self.(*asyncGenAwaitable).Send(v)
	}, 0, ""))
	AsyncGenAwaitableType.Dict.Set("throw", MustNewMethod("throw", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(*asyncGenAwaitable).Throw(args)
	}, 0, ""))
	AsyncGenAwaitableType.Dict.Set("close", MustNewMethod("close", func(self Object) (Object, error) {
		self.(*asyncGenAwaitable).state = agsClosed
		return None, nil
	}, 0, ""))
}

var AsyncGenAwaitableType = NewType("async_generator_asend", "async generator awaitable")

const (
	agsInit = iota
	agsIter
	agsClosed
)

// asyncGenAwaitable is what asend()/athrow()/aclose()/__anext__() return.
type asyncGenAwaitable struct {
	gen   *AsyncGenerator
	kind  int
	arg   Object // asend: value; athrow: args tuple
	state int
}

func (a *asyncGenAwaitable) Type() *Type                 { return AsyncGenAwaitableType }
func (a *asyncGenAwaitable) M__await__() (Object, error) { return a, nil }
func (a *asyncGenAwaitable) M__iter__() (Object, error)  { return a, nil }
func (a *asyncGenAwaitable) M__next__() (Object, error)  { return a.Send(None) }

func (a *asyncGenAwaitable) Throw(args Tuple) (Object, error) {
	if a.state == agsClosed {
		return nil, ExceptionNewf(RuntimeError, "cannot reuse already awaited aclose()/athrow()")
	}
	return a.step(func(g *AsyncGenerator) (Object, error) { return g.Generator.Throw(args, StringDict{}) })
}

func (a *asyncGenAwaitable) Send(arg Object) (Object, error) {
	g := a.gen
	switch a.state {
	case agsClosed:
		return nil, ExceptionNewf(RuntimeError, "cannot reuse already awaited aclose()/athrow()")
	case agsInit:
		what := [...]string{"anext()", "athrow()", "aclose()"}[a.kind]
		if g.running {
			return nil, ExceptionNewf(RuntimeError, "%s: asynchronous generator is already running", what)
		}
		a.state = agsIter
		if g.done() {
			a.state = agsClosed
			switch a.kind {
			case agSend:
				return nil, StopAsyncIteration
			case agClose:
				return nil, StopIteration
			default:
				return nil, throwValue(a.arg.(Tuple))
			}
		}
		switch a.kind {
		case agThrow:
			args := a.arg.(Tuple)
			return a.step(func(g *AsyncGenerator) (Object, error) {
				if g.Frame.Lasti == 0 {
					g.closed = true
					return nil, throwValue(args)
				}
				return g.Generator.Throw(args, StringDict{})
			})
		case agClose:
			if g.Frame.Lasti == 0 {
				g.closed = true
				a.state = agsClosed
				return nil, StopIteration
			}
			return a.step(func(g *AsyncGenerator) (Object, error) {
				return g.Generator.Throw(Tuple{GeneratorExitInstance()}, StringDict{})
			})
		}
		if arg == None {
			arg = a.arg
		}
	}
	return a.step(func(g *AsyncGenerator) (Object, error) { return g.Generator.Send(arg) })
}

// throwValue turns athrow's args into the error to raise.
func throwValue(args Tuple) error {
	switch v := args[0].(type) {
	case *Exception:
		return v
	case *Type:
		if len(args) > 1 {
			if e, err := ExceptionNew(v, args[1:2], NewStringDict()); err == nil {
				return e.(*Exception)
			}
		}
		return MakeException(v)
	}
	return ExceptionNewf(TypeError, "exceptions must be classes or instances")
}

// step runs one resumption of the generator and translates the outcome.
func (a *asyncGenAwaitable) step(run func(*AsyncGenerator) (Object, error)) (Object, error) {
	g := a.gen
	g.running = true
	res, err := run(g)
	g.running = false
	finish := func() { a.state = agsClosed }
	if err == nil {
		if _, ok := res.(*AsyncGenWrappedValue); !ok {
			g.running = true // suspended on an await: still in progress
		}
		if w, ok := res.(*AsyncGenWrappedValue); ok {
			finish()
			if a.kind == agClose {
				return nil, ExceptionNewf(RuntimeError, "async generator ignored GeneratorExit")
			}
			return nil, stopIterationWith(w.Value)
		}
		return res, nil // an await suspension: pass up to the event loop
	}
	finish()
	switch {
	case IsException(StopIteration, err):
		// the body returned
		g.closed = true
		if a.kind == agClose {
			return nil, StopIteration
		}
		return nil, StopAsyncIteration
	case isGeneratorExit(err) && a.kind == agClose:
		g.closed = true
		return nil, StopIteration
	case IsException(StopAsyncIteration, err):
		g.closed = true
		return nil, ExceptionNewf(RuntimeError, "async generator raised StopAsyncIteration")
	}
	g.closed = g.Frame.Lasti != 0 && !g.Frame.Yielded
	return nil, err
}
