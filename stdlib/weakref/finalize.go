package weakref

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"unsafe"
	"weak"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/atexit"
)

// finalize is weakref.finalize: a callback that runs once, when its object is
// collected, when it is called, or at exit.
//
// Go's collector decides when "collected" is.  A runtime finalizer runs on its
// own goroutine, so it only queues the finalize object; the callbacks run on
// the interpreter goroutine from RunPending, which gc.collect() calls.  Unlike
// CPython there is no refcount, so a dropped object is not finalized until a
// collection happens (or at exit, for atexit=True).
type finalize struct {
	mu     sync.Mutex
	alive  bool
	atexit bool
	index  int
	// ptr keys the registry; wp/typ rebuild the referent for peek/detach
	// without keeping it alive (wp goes nil once the object is unreachable).
	ptr    uintptr
	wp     weak.Pointer[byte]
	typ    reflect.Type
	fn     py.Object
	args   py.Tuple
	kwargs py.StringDict
}

var (
	finMu       sync.Mutex
	finRegistry = map[uintptr][]*finalize{}
	finPending  []*finalize
	finCounter  int
	exitHook    = py.MustNewMethod("_exitfunc", func(py.Object) (py.Object, error) {
		exitFunc()
		return py.None, nil
	}, 0, "")
)

var FinalizeType = py.NewTypeX("weakref.finalize", "Class for finalization of weakrefable objects.", finalizeNew, nil)

func (f *finalize) Type() *py.Type { return FinalizeType }

func finalizeNew(_ *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 2 {
		if len(args) == 1 {
			return nil, py.ExceptionNewf(py.TypeError, "finalize.__init__() missing 1 required positional argument: 'func'")
		}
		return nil, py.ExceptionNewf(py.TypeError, "finalize.__init__() missing 2 required positional arguments: 'obj' and 'func'")
	}
	obj, fn := args[0], args[1]
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil, py.ExceptionNewf(py.TypeError, "cannot create weak reference to '%s' object", obj.Type().Name)
	}
	f := &finalize{alive: true, atexit: true, ptr: v.Pointer(), wp: weak.Make((*byte)(v.UnsafePointer())), typ: v.Type(), fn: fn, args: append(py.Tuple{}, args[2:]...), kwargs: kwargs.Copy()}
	finMu.Lock()
	if _, ok := finRegistry[f.ptr]; !ok {
		if err := setFinalizer(obj); err != nil {
			finMu.Unlock()
			return nil, py.ExceptionNewf(py.TypeError, "cannot create weak reference to '%s' object", obj.Type().Name)
		}
	}
	finRegistry[f.ptr] = append(finRegistry[f.ptr], f)
	finCounter++
	f.index = finCounter
	if !atexit.Registered(exitHook) {
		atexit.Register(exitHook)
	}
	finMu.Unlock()
	return f, nil
}

// setFinalizer panics for a pointer that is interior or already has a
// finalizer; those objects cannot be tracked.
func setFinalizer(obj py.Object) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	runtime.SetFinalizer(obj, func(x py.Object) {
		p := reflect.ValueOf(x).Pointer()
		finMu.Lock()
		fins := finRegistry[p]
		delete(finRegistry, p)
		for _, f := range fins {
			f.mu.Lock()
			if f.alive {
				finPending = append(finPending, f)
			}
			f.mu.Unlock()
		}
		finMu.Unlock()
	})
	return nil
}

// RunPending runs the callbacks of finalizers whose objects Go has collected.
// It must be called on the interpreter goroutine.
func RunPending() {
	finMu.Lock()
	pending := finPending
	finPending = nil
	finMu.Unlock()
	sort.Slice(pending, func(i, j int) bool { return pending[i].index < pending[j].index })
	for _, f := range pending {
		if _, err := f.run(); err != nil {
			py.TracebackDump(err)
		}
	}
}

// exitFunc is finalize._exitfunc: the atexit=True finalizers still alive, last
// created first.
func exitFunc() {
	finMu.Lock()
	var all []*finalize
	for _, fins := range finRegistry {
		for _, f := range fins {
			if f.atexit {
				all = append(all, f)
			}
		}
	}
	for _, f := range finPending {
		if f.atexit {
			all = append(all, f)
		}
	}
	finPending = nil
	finMu.Unlock()
	sort.Slice(all, func(i, j int) bool { return all[i].index > all[j].index })
	for _, f := range all {
		if _, err := f.run(); err != nil {
			py.TracebackDump(err)
		}
	}
}

// take marks f dead and unregisters it; false if it already was.
func (f *finalize) take() bool {
	finMu.Lock()
	defer finMu.Unlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.alive {
		return false
	}
	f.alive = false
	fins := finRegistry[f.ptr]
	for i, g := range fins {
		if g == f {
			finRegistry[f.ptr] = append(fins[:i:i], fins[i+1:]...)
			break
		}
	}
	return true
}

func (f *finalize) run() (py.Object, error) {
	if !f.take() {
		return py.None, nil
	}
	return py.Call(f.fn, f.args, f.kwargs)
}

func (f *finalize) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) > 1 || kwargs.Len() != 0 {
		return nil, py.ExceptionNewf(py.TypeError, "finalize.__call__() takes from 1 to 2 positional arguments but %d were given", len(args)+1)
	}
	return f.run()
}

func (f *finalize) info() (py.Tuple, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.wp.Value()
	if !f.alive || p == nil {
		return nil, false
	}
	obj := reflect.NewAt(f.typ.Elem(), unsafe.Pointer(p)).Interface().(py.Object)
	return py.Tuple{obj, f.fn, f.args, f.kwargs}, true
}

func (f *finalize) M__repr__() (py.Object, error) {
	id := fmt.Sprintf("<finalize object at %#x", uintptr(unsafe.Pointer(f)))
	t, ok := f.info()
	if !ok {
		return py.String(id + "; dead>"), nil
	}
	return py.String(fmt.Sprintf("%s; for '%s' at %#x>", id, t[0].Type().Name, f.ptr)), nil
}

func init() {
	FinalizeType.Dict.Set("alive", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		f := self.(*finalize)
		f.mu.Lock()
		defer f.mu.Unlock()
		return py.NewBool(f.alive), nil
	}})
	FinalizeType.Dict.Set("atexit", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			f := self.(*finalize)
			f.mu.Lock()
			defer f.mu.Unlock()
			return py.NewBool(f.alive && f.atexit), nil
		},
		Fset: func(self, value py.Object) error {
			b, err := py.ObjectIsTrue(value)
			if err != nil {
				return err
			}
			f := self.(*finalize)
			f.mu.Lock()
			f.atexit = b
			f.mu.Unlock()
			return nil
		},
	})
	FinalizeType.Dict.Set("detach", py.MustNewMethod("detach", func(self py.Object) (py.Object, error) {
		f := self.(*finalize)
		t, ok := f.info()
		if !ok || !f.take() {
			return py.None, nil
		}
		return t, nil
	}, 0, "If alive then mark as dead and return (obj, func, args, kwargs); if dead then return None."))
	FinalizeType.Dict.Set("peek", py.MustNewMethod("peek", func(self py.Object) (py.Object, error) {
		if t, ok := self.(*finalize).info(); ok {
			return t, nil
		}
		return py.None, nil
	}, 0, "If alive then return (obj, func, args, kwargs); if dead then return None."))
}
