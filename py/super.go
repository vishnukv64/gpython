// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

// super: the proxy that looks a method up starting after a given class in
// the instance's MRO.
//
// Python implements the no-argument form with a compiler-generated
// __class__ cell plus the first argument of the method, both of which this
// interpreter already produces: the compiler emits the cell (see
// compile/compile.go) and SuperType is handed the frame so it can read them
// back.  See SuperNew for that path.

// Super is a proxy over an instance and the class whose methods are being
// skipped.
type Super struct {
	// typ is the class to start the search after.
	typ *Type
	// obj is the instance the attribute is bound to, when there is one.
	obj Object
}

var SuperType = NewTypeX("super", `super() -> same as super(__class__, <first argument>)
super(type) -> unbound super object
super(type, obj) -> bound super object; requires isinstance(obj, type)

Return a proxy object that delegates method calls to a parent or sibling
class of type.`, SuperNew, nil)

// Type of this Super object
func (s *Super) Type() *Type {
	return SuperType
}

func (s *Super) M__repr__() (Object, error) {
	if s.obj == nil || s.obj == None {
		return String("<super: <class '" + s.typ.Name + "'>>"), nil
	}
	return String("<super: <class '" + s.typ.Name + "'>, <" + typeNameOf(s.obj) + " object>>"), nil
}

// currentFrame is the frame executing right now, which is what the
// no-argument form of super() reads.
var currentFrame = func() *Frame { return nil }

// SetCurrentFrame installs the accessor the vm provides, so that this
// package can reach the running frame without importing it.
func SetCurrentFrame(fn func() *Frame) { currentFrame = fn }

// typeNameOf is the instance's class name, for the repr.
func typeNameOf(obj Object) string {
	if t := obj.Type(); t != nil {
		return t.Name
	}
	return "?"
}

// lookup finds the attribute by walking the MRO of the instance's class,
// starting at the entry after the one this proxy skips.
func (s *Super) lookup(name string) (Object, bool, error) {
	start := s.typ
	var mro Object
	if s.obj != nil && s.obj != None {
		mro = s.obj.Type().Mro
	} else {
		mro = s.typ.Mro
	}
	entries, err := SequenceList(mro)
	if err != nil {
		return nil, false, nil
	}

	// Skip everything up to and including the class this proxy was built
	// for, then take the next class that defines the name.
	reached := false

	for _, entry := range entries.Items {
		entryType, ok := entry.(*Type)
		if !ok {
			continue
		}
		if !reached {
			if entryType == start {
				reached = true
			}
			continue
		}
		// getattr finds the attribute and binds it, which is what makes
		// super().method() receive self.
		var target Object = entryType
		if s.obj != nil && s.obj != None {
			// A descriptor found on the class binds to the instance.
			if fn := entryType.Lookup(name); fn != nil {
				if getter, ok := fn.(I__get__); ok {
					res, err := getter.M__get__(s.obj, entryType)
					if err != nil {
						return nil, false, err
					}
					return res, true, nil
				}
				return fn, true, nil
			}
			continue
		}
		if fn := target.(*Type).Lookup(name); fn != nil {
			return fn, true, nil
		}
	}
	return nil, false, nil
}

// M__getattribute__ makes the proxy answer attribute lookups from the MRO.
func (s *Super) M__getattribute__(name string) (Object, error) {
	res, ok, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	if ok {
		return res, nil
	}
	return nil, ExceptionNewf(AttributeError, "'super' object has no attribute '%s'", name)
}

// M__getattr__ backs up __getattribute__ for the paths that use it.
func (s *Super) M__getattr__(name string) (Object, error) {
	res, ok, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	if ok {
		return res, nil
	}
	return nil, ExceptionNewf(AttributeError, "'super' object has no attribute '%s'", name)
}

// SuperNew implements both forms of super().
//
// With no arguments it reads the __class__ cell of the calling frame and the
// frame's first local variable, which is self - the two things the
// no-argument form is defined to use.  With arguments it takes the class and
// the object directly.
func SuperNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	if len(args) == 0 {
		// The no-argument form needs the calling frame.
		frame := currentFrame()
		if frame == nil {
			return nil, ExceptionNewf(RuntimeError, "super(): no current frame")
		}
		typ, self, err := classAndSelf(frame)
		if err != nil {
			return nil, err
		}
		return &Super{typ: typ, obj: self}, nil
	}
	if len(args) == 1 {
		typ, ok := args[0].(*Type)
		if !ok {
			return nil, ExceptionNewf(TypeError, "super() argument 1 must be a type")
		}
		return &Super{typ: typ}, nil
	}
	typ, ok := args[0].(*Type)
	if !ok {
		return nil, ExceptionNewf(TypeError, "super() argument 1 must be a type")
	}
	obj := args[1]
	// The instance must be an instance of the class, as in CPython.
	if obj != None && obj != nil {
		if !isInstanceOf(obj, typ) {
			return nil, ExceptionNewf(TypeError, "super(type, obj): obj must be an instance or subtype of type")
		}
	}
	return &Super{typ: typ, obj: obj}, nil
}

// isInstanceOf reports whether obj's class derives from typ.
func isInstanceOf(obj Object, typ *Type) bool {
	for t := obj.Type(); t != nil; t = t.Base {
		if t == typ {
			return true
		}
	}
	return false
}

// classAndSelf reads __class__ and the first argument out of a frame.
func classAndSelf(frame *Frame) (*Type, Object, error) {
	// __class__ is a free variable of the method, held in a cell.
	var typ *Type
	for i, name := range frame.Code.Freevars {
		if name == "__class__" {
			idx := len(frame.Code.Cellvars) + i
			if idx < len(frame.CellAndFreeVars) {
				if cell, ok := frame.CellAndFreeVars[idx].(*Cell); ok && cell.Get() != nil {
					if t, ok := cell.Get().(*Type); ok {
						typ = t
					}
				}
			}
		}
	}
	if typ == nil {
		return nil, nil, ExceptionNewf(RuntimeError, "super(): no arguments and no __class__ cell; super() only works inside a method")
	}

	// The first argument is self, which is the first local slot.
	//
	// It may have been MOVED into a cell: a method whose body mentions
	// __class__ - which is what super() does - has self as a cell variable, and
	// the frame builder clears the local slot once the cell owns the value.  So
	// the CELL is consulted when the local is empty, or "super()" reported "no
	// first argument" in exactly the methods that use it.
	var self Object
	if len(frame.LocalVars) > 0 {
		self = frame.LocalVars[0]
	}
	if self == nil {
		self = cellArg(frame, 0)
	}
	if self == nil {
		return nil, nil, ExceptionNewf(RuntimeError, "super(): no arguments and no first argument; super() only works inside a method")
	}
	return typ, self, nil
}

// cellArg returns the value of an argument that was moved into a cell.
//
// Code.Cell2arg records, per cell, which argument it was made from; an argument
// that is not a cell reports CO_CELL_NOT_AN_ARG.
//
// This exists because a method whose body mentions __class__ - which is what
// super() does - has its arguments as cell variables, and the frame builder
// CLEARS the local slot once a cell owns the value.  Reading LocalVars alone
// therefore found nothing in exactly the methods that use super().
func cellArg(frame *Frame, arg int) Object {
	if frame.Code.Cell2arg == nil {
		return nil
	}
	for i, a := range frame.Code.Cell2arg {
		if int(a) != arg {
			continue
		}
		// Cells come first in this storage, indexed by the cell variable's own
		// position - the same i that LOAD_DEREF uses.
		if i >= len(frame.CellAndFreeVars) {
			return nil
		}
		if c, ok := frame.CellAndFreeVars[i].(*Cell); ok {
			return c.Get()
		}
	}
	return nil
}
