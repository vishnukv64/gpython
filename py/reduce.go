// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import "sync"

// object.__reduce_ex__, __reduce__ and __getstate__ are what pickle and copy
// ask every instance for.  They are written in Python on top of copyreg - the
// same helpers CPython's C versions call - and compiled on first use, so a
// program that never pickles pays nothing.
const reduceSrc = `
import copyreg

def getstate(self):
    d = getattr(self, "__dict__", None)
    state = d if d else None
    names = copyreg._slotnames(type(self))
    if names:
        slots = {}
        for n in names:
            try:
                slots[n] = getattr(self, n)
            except AttributeError:
                pass
        if slots:
            state = (state, slots)
    return state

def reduce01(self):
    # copyreg._reduce_ex tests the heap-type flag, which types here lack.
    cls = type(self)
    base = object
    for b in cls.__mro__:
        if b.__module__ == "builtins":
            base = b
            break
    state = None if base is object else base(self)
    d = getattr(self, "__dict__", None)
    return copyreg._reconstructor, (cls, base, state), (d if d else None)

def reduce(self):
    return reduce01(self)

def reduce_ex(self, proto):
    cls = type(self)
    if _owner(cls, "__reduce__") is not object:
        return self.__reduce__()
    if proto < 2:
        return reduce01(self)
    kwargs = None
    if hasattr(cls, "__getnewargs_ex__"):
        args, kwargs = self.__getnewargs_ex__()
    elif hasattr(cls, "__getnewargs__"):
        args = self.__getnewargs__()
    else:
        args = ()
    if kwargs:
        newobj, newargs = copyreg.__newobj_ex__, (cls, args, kwargs)
    else:
        newobj, newargs = copyreg.__newobj__, (cls,) + tuple(args)
    listitems = iter(self) if isinstance(self, list) else None
    dictitems = iter(self.items()) if isinstance(self, dict) else None
    return newobj, newargs, self.__getstate__(), listitems, dictitems
`

var (
	reduceOnce sync.Once
	reduceMod  *Module
	reduceErr  error
)

// reduceCall runs the named helper from reduceSrc.
func reduceCall(name string, args ...Object) (Object, error) {
	reduceOnce.Do(func() {
		f := CurrentFrame()
		if f == nil {
			reduceErr = ExceptionNewf(RuntimeError, "no running frame")
			return
		}
		// _owner(cls, name): the first class in cls's MRO that defines name in
		// its own namespace.  Python code cannot ask that of the exception
		// types, whose __dict__ is the instance-dict property.
		owner := MustNewMethod("_owner", func(_ Object, args Tuple) (Object, error) {
			cls, ok := args[0].(*Type)
			if !ok || len(args) != 2 {
				return None, nil
			}
			name, _ := args[1].(String)
			for _, c := range cls.Mro {
				if t, ok := c.(*Type); ok {
					if _, found := t.Dict.Get(string(name)); found {
						return t, nil
					}
				}
			}
			return None, nil
		}, 0, "_owner(cls, name)")
		reduceMod, reduceErr = RunSrc(f.Context, "", "<reduce>", "_gpython_reduce")
		if reduceErr != nil {
			return
		}
		reduceMod.Globals.Set("_owner", owner)
		_, reduceErr = RunSrc(f.Context, reduceSrc, "<reduce>", reduceMod)
	})
	if reduceErr != nil {
		return nil, reduceErr
	}
	return Call(reduceMod.Globals.GetOrNil(name), Tuple(args), NewStringDict())
}

// registerReduce is called from type.go's init, once ObjectType's dict is final.
func registerReduce() {
	ObjectType.Dict.Set("__reduce_ex__", MustNewMethod("__reduce_ex__", func(self Object, args Tuple) (Object, error) {
		if len(args) != 1 {
			return nil, ExceptionNewf(TypeError, "__reduce_ex__() takes exactly one argument (%d given)", len(args))
		}
		return reduceCall("reduce_ex", self, args[0])
	}, 0, "Helper for pickle."))
	ObjectType.Dict.Set("__reduce__", MustNewMethod("__reduce__", func(self Object, args Tuple) (Object, error) {
		return reduceCall("reduce", self)
	}, 0, "Helper for pickle."))
	ObjectType.Dict.Set("__getstate__", MustNewMethod("__getstate__", func(self Object, args Tuple) (Object, error) {
		return reduceCall("getstate", self)
	}, 0, "Helper for pickle."))
}
