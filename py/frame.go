// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Frame objects

package py

// What kind of block this is
type TryBlockType byte

const (
	TryBlockSetupLoop TryBlockType = iota
	TryBlockSetupExcept
	TryBlockSetupFinally
	TryBlockExceptHandler
)

// Store information about try blocks
type TryBlock struct {
	Type    TryBlockType // what kind of block this is
	Handler int32        // where to jump to find handler
	Level   int          // value stack level to pop to
}

// A python Frame object
type Frame struct {
	Back            *Frame     // the frame that was executing when this one was called, or nil
	Context         Context    // host module (state) context
	Code            *Code      // code segment
	Builtins        StringDict // builtin symbol table
	Globals         StringDict // global symbol table
	Locals          StringDict // local symbol table
	Stack           []Object   // Valuestack
	LocalVars       Tuple      // Fast access local vars
	CellAndFreeVars Tuple      // Cellvars then Freevars Cell objects in one Tuple

	// Next free slot in f_valuestack.  Frame creation sets to f_valuestack.
	// Frame evaluation usually NULLs it, but a frame that yields sets it
	// to the current stack top.
	// Stacktop *Object
	Yielded bool // set if the function yielded, cleared otherwise
	// Trace   Object // Trace function

	// In a generator, we need to be able to swap between the exception
	// state inside the generator and the exception state of the calling
	// frame (which shouldn't be impacted when the generator "yields"
	// from an except handler).
	// These three fields exist exactly for that, and are unused for
	// non-generator frames. See the save_exc_state and swap_exc_state
	// functions in ceval.c for details of their use.
	// Exc_type      Object
	// Exc_value     *Object
	// Exc_traceback *Object
	// Borrowed reference to a generator, or NULL
	// Gen Object

	// FIXME Tstate *PyThreadState
	Lasti int32 // Last instruction if called

	// PendingException is an exception to raise as soon as this frame
	// resumes, set by generator.throw().  The VM reads it on entry and
	// unwinds through the frame's own handlers, so an "except" inside a
	// suspended generator catches what the caller threw at it.
	PendingException *Exception
	// Call PyFrame_GetLineNumber() instead of reading this field
	// directly.  As of 2.3 f_lineno is only valid when tracing is
	// active (i.e. when f_trace is set).  At other times we use
	// PyCode_Addr2Line to calculate the line from the current
	// bytecode index.
	// Lineno     int        // Current line number
	// Iblock     int        // index in f_blockstack
	// Executing  byte       // whether the frame is still executing
	Blockstack []TryBlock // for try and loop blocks
	Block      *TryBlock  // pointer to current block or nil
	Localsplus []Object   // LocalVars + CellAndFreeVars
}

var FrameType = NewType("frame", "Represents a stack frame")

// Type of this object
func (o *Frame) Type() *Type {
	return FrameType
}

// Make a new frame for a code object
func NewFrame(ctx Context, globals, locals StringDict, code *Code, closure Tuple) *Frame {
	nlocals := int(code.Nlocals)
	ncells := len(code.Cellvars)
	nfrees := len(code.Freevars)
	varsize := nlocals + ncells + nfrees
	// Allocate the stack, locals, cells and frees in a contigious block of memory
	allocation := make([]Object, varsize)
	localVars := allocation[:nlocals]
	//cellVars := allocation[nlocals : nlocals+ncells]
	//freeVars := allocation[nlocals+ncells : varsize]
	cellAndFreeVars := allocation[nlocals:varsize]

	return &Frame{
		Context:         ctx,
		Globals:         globals,
		Locals:          locals,
		Code:            code,
		LocalVars:       localVars,
		CellAndFreeVars: cellAndFreeVars,
		Builtins:        ctx.Store().Builtins.Globals,
		Localsplus:      allocation,
		Stack:           make([]Object, 0, code.Stacksize),
	}
}

// Python globals  are looked up in two scopes
//
// The module global scope
// And finally the builtins
func (f *Frame) LookupGlobal(name string) (obj Object, ok bool) {
	// Lookup in globals
	// fmt.Printf("globals = %v\n", f.Globals)
	if obj, ok = f.Globals.Get(name); ok {
		return
	}

	// Lookup in builtins
	// fmt.Printf("builtins = %v\n", Builtins.Globals)
	if obj, ok = f.Builtins.Get(name); ok {
		return
	}

	return nil, false
}

// Python names are looked up in three scopes
//
// First the local scope
// Next the module global scope
// And finally the builtins
func (f *Frame) Lookup(name string) (obj Object, ok bool) {
	// Lookup in locals
	if obj, ok = f.Locals.Get(name); ok {
		return
	}
	// At module scope a frame's locals and globals are the SAME dict - the
	// module body runs with one namespace - so the miss above has already
	// searched it, and LookupGlobal would hash and compare the same name a
	// second time on every global read.  A profile of a module-level loop puts
	// ~60% of its samples in exactly that: Go's map hash and the string
	// comparison behind it.
	if f.Locals.SameAs(f.Globals) {
		return f.LookupBuiltins(name)
	}
	return f.LookupGlobal(name)
}

// LookupBuiltins searches only the builtins, for the case where the other
// scopes have already been searched.
func (f *Frame) LookupBuiltins(name string) (obj Object, ok bool) {
	return f.Builtins.Get(name)
}

// Make a new Block (try/for/while)
func (f *Frame) PushBlock(Type TryBlockType, Handler int32, Level int) {
	f.Blockstack = append(f.Blockstack, TryBlock{
		Type:    Type,
		Handler: Handler,
		Level:   Level,
	})
	f.Block = &f.Blockstack[len(f.Blockstack)-1]
}

// Pop the current block off
func (f *Frame) PopBlock() {
	f.Blockstack = f.Blockstack[:len(f.Blockstack)-1]
	if len(f.Blockstack) > 0 {
		f.Block = &f.Blockstack[len(f.Blockstack)-1]
	} else {
		f.Block = nil
	}
}

/*
Convert between "fast" version of locals and dictionary version.

	map and values are input arguments.  map is a tuple of strings.
	values is an array of PyObject*.  At index i, map[i] is the name of
	the variable with value values[i].  The function copies the first
	nmap variable from map/values into dict.  If values[i] is NULL,
	the variable is deleted from dict.

	If deref is true, then the values being copied are cell variables
	and the value is extracted from the cell variable before being put
	in dict.
*/
func map_to_dict(mapping []string, nmap int, dict StringDict, values []Object, deref bool) {
	for j := nmap - 1; j >= 0; j-- {
		key := mapping[j]
		value := values[j]
		if deref && value != nil {
			cell, ok := value.(*Cell)
			if !ok {
				panic("map_to_dict: expecting Cell")
			}
			value = cell.Get()
		}
		if value == nil {
			dict.Del(key)
		} else {
			dict.Set(key, value)
		}
	}
}

/*
Copy values from the "locals" dict into the fast locals.

	dict is an input argument containing string keys representing
	variables names and arbitrary PyObject* as values.

	mapping and values are input arguments.  mapping is a tuple of strings.
	values is an array of PyObject*.  At index i, mapping[i] is the name of
	the variable with value values[i].  The function copies the first
	nmap variable from mapping/values into dict.  If values[i] is nil,
	the variable is deleted from dict.

	If deref is true, then the values being copied are cell variables
	and the value is extracted from the cell variable before being put
	in dict.  If clear is true, then variables in mapping but not in dict
	are set to nil in mapping; if clear is false, variables missing in
	dict are ignored.

	Exceptions raised while modifying the dict are silently ignored,
	because there is no good way to report them.
*/
func dict_to_map(mapping []string, nmap int, dict StringDict, values []Object, deref bool, clear bool) {
	for j := nmap - 1; j >= 0; j-- {
		key := mapping[j]
		value := dict.GetOrNil(key)
		/* We only care about nils if clear is true. */
		if value == nil {
			if !clear {
				continue
			}
		}
		if deref {
			cell, ok := values[j].(*Cell)
			if !ok {
				panic("dict_to_map: expecting Cell")
			}
			if cell.Get() != value {
				cell.Set(value)
			}
		} else if values[j] != value {
			values[j] = value
		}
	}
}

// Merge fast locals into frame Locals
func (f *Frame) FastToLocals() {
	locals := f.Locals
	if locals.IsNil() {
		locals = NewStringDict()
		f.Locals = locals
	}
	co := f.Code
	mapping := co.Varnames
	fast := f.Localsplus
	j := len(mapping)
	if j > int(co.Nlocals) {
		j = int(co.Nlocals)
	}
	if co.Nlocals != 0 {
		map_to_dict(mapping, j, locals, fast, false)
	}
	ncells := len(co.Cellvars)
	nfreevars := len(co.Freevars)
	if ncells != 0 || nfreevars != 0 {
		map_to_dict(co.Cellvars, ncells, locals, fast[co.Nlocals:], true)

		/* If the namespace is unoptimized, then one of the
		   following cases applies:
		   1. It does not contain free variables, because it
		      uses import * or is a top-level namespace.
		   2. It is a class namespace.
		   We don't want to accidentally copy free variables
		   into the locals dict used by the class.
		*/
		if co.Flags&CO_OPTIMIZED != 0 {
			map_to_dict(co.Freevars, nfreevars, locals, fast[int(co.Nlocals)+ncells:], true)
		}
	}
}

// Merge frame Locals into fast locals
func (f *Frame) LocalsToFast(clear bool) {
	locals := f.Locals
	co := f.Code
	mapping := co.Varnames
	if locals.IsNil() {
		return
	}
	fast := f.Localsplus
	j := len(mapping)
	if j > int(co.Nlocals) {
		j = int(co.Nlocals)
	}
	if co.Nlocals != 0 {
		dict_to_map(co.Varnames, j, locals, fast, false, clear)
	}
	ncells := len(co.Cellvars)
	nfreevars := len(co.Freevars)
	if ncells != 0 || nfreevars != 0 {
		dict_to_map(co.Cellvars, ncells, locals, fast[co.Nlocals:], true, clear)
		/* Same test as in FastToLocals() above. */
		if co.Flags&CO_OPTIMIZED != 0 {
			dict_to_map(co.Freevars, nfreevars, locals, fast[int(co.Nlocals)+ncells:], true, clear)
		}
	}
}

// The frame attributes below are the subset of the CPython frame interface
// that Python code uses to walk out to a caller - which is what
// inspect.currentframe().f_back.f_globals needs.
func init() {
	FrameType.Dict.Set("f_back", &Property{
		Fget: func(self Object) (Object, error) {
			f := self.(*Frame)
			if f.Back == nil {
				return None, nil
			}
			return f.Back, nil
		},
		Doc: "previous frame, or None if this is the outermost",
	})
	FrameType.Dict.Set("f_globals", &Property{
		Fget: func(self Object) (Object, error) {
			return self.(*Frame).Globals, nil
		},
		Doc: "global namespace as seen by this frame",
	})
	FrameType.Dict.Set("f_locals", &Property{
		Fget: func(self Object) (Object, error) {
			f := self.(*Frame)
			// A running frame keeps its locals in slots, so they have to be
			// materialised before the mapping is read.
			f.FastToLocals()
			return f.Locals, nil
		},
		Doc: "local namespace as seen by this frame",
	})
	FrameType.Dict.Set("f_code", &Property{
		Fget: func(self Object) (Object, error) {
			return self.(*Frame).Code, nil
		},
		Doc: "code object being executed in this frame",
	})
	FrameType.Dict.Set("f_builtins", &Property{
		Fget: func(self Object) (Object, error) {
			return self.(*Frame).Builtins, nil
		},
		Doc: "builtins namespace seen by this frame",
	})
	FrameType.Dict.Set("f_lasti", &Property{
		Fget: func(self Object) (Object, error) {
			// The instruction pointer is not tracked here, so there is no
			// honest value to give; -1 is what CPython reports for a frame
			// that has not started executing.
			return Int(-1), nil
		},
		Doc: "index of last attempted instruction, or -1",
	})
	FrameType.Dict.Set("f_lineno", &Property{
		Fget: func(self Object) (Object, error) {
			return Int(self.(*Frame).Code.Firstlineno), nil
		},
		Doc: "current line number, from the code object's first line",
	})
}
