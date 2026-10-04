// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Evaluate opcodes
package vm

/*
FIXME

cpython keeps a zombie frame on each code object to speed up execution
of a code object so a frame doesn't have to be allocated and
deallocated each time which seems like a good idea.  If we want to
work with go routines then it might have to be more sophisticated.

FIXME could make the stack be permanently allocated and just keep a
pointer into it rather than using append etc...

If we are caching the frames need to make sure we clear the stack
objects so they can be GCed
*/

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

const (
	nameErrorMsg         = "name '%s' is not defined"
	globalNameErrorMsg   = "global name '%s' is not defined"
	unboundLocalErrorMsg = "local variable '%s' referenced before assignment"
	unboundFreeErrorMsg  = "free variable '%s' referenced before assignment in enclosing scope"
	cannotCatchMsg       = "catching '%s' that does not inherit from BaseException is not allowed"
)

const debugging = false

// Debug print
func debugf(format string, a ...interface{}) {
	fmt.Printf(format, a...)
}

// Stack operations
func (vm *Vm) STACK_LEVEL() int             { return len(vm.frame.Stack) }
func (vm *Vm) TOP() py.Object               { return vm.frame.Stack[len(vm.frame.Stack)-1] }
func (vm *Vm) SECOND() py.Object            { return vm.frame.Stack[len(vm.frame.Stack)-2] }
func (vm *Vm) THIRD() py.Object             { return vm.frame.Stack[len(vm.frame.Stack)-3] }
func (vm *Vm) FOURTH() py.Object            { return vm.frame.Stack[len(vm.frame.Stack)-4] }
func (vm *Vm) PEEK(n int) py.Object         { return vm.frame.Stack[len(vm.frame.Stack)-n] }
func (vm *Vm) SET_TOP(v py.Object)          { vm.frame.Stack[len(vm.frame.Stack)-1] = v }
func (vm *Vm) SET_SECOND(v py.Object)       { vm.frame.Stack[len(vm.frame.Stack)-2] = v }
func (vm *Vm) SET_THIRD(v py.Object)        { vm.frame.Stack[len(vm.frame.Stack)-3] = v }
func (vm *Vm) SET_FOURTH(v py.Object)       { vm.frame.Stack[len(vm.frame.Stack)-4] = v }
func (vm *Vm) SET_VALUE(n int, v py.Object) { vm.frame.Stack[len(vm.frame.Stack)-(n)] = (v) }
func (vm *Vm) DROP()                        { vm.frame.Stack = vm.frame.Stack[:len(vm.frame.Stack)-1] }
func (vm *Vm) DROPN(n int)                  { vm.frame.Stack = vm.frame.Stack[:len(vm.frame.Stack)-n] }

// Pop from top of vm stack
func (vm *Vm) POP() py.Object {
	// FIXME what if empty?
	out := vm.frame.Stack[len(vm.frame.Stack)-1]
	vm.frame.Stack = vm.frame.Stack[:len(vm.frame.Stack)-1]
	return out
}

// Push to top of vm stack
func (vm *Vm) PUSH(obj py.Object) {
	vm.frame.Stack = append(vm.frame.Stack, obj)
}

// Push items to top of vm stack
func (vm *Vm) EXTEND(items py.Tuple) {
	vm.frame.Stack = append(vm.frame.Stack, items...)
}

// Push items to top of vm stack in reverse order
func (vm *Vm) EXTEND_REVERSED(items py.Tuple) {
	start := len(vm.frame.Stack)
	vm.frame.Stack = append(vm.frame.Stack, items...)
	py.Tuple(vm.frame.Stack[start:]).Reverse()
}

// Adds a traceback to the exc passed in for the current vm state
func (vm *Vm) AddTraceback(exc *py.ExceptionInfo) {
	exc.Traceback = &py.Traceback{
		Next:   exc.Traceback,
		Frame:  vm.frame,
		Lasti:  vm.frame.Lasti,
		Lineno: vm.frame.Code.Addr2Line(vm.frame.Lasti),
	}
}

// Set an exception in the VM
//
// The exception must be a valid exception instance (eg as returned by
// py.MakeException)
//
// It sets vm.curexc.* and sets vm.why to whyException
func (vm *Vm) SetException(exception py.Object) {
	vm.curexc.Value = exception
	vm.curexc.Type = exception.Type()
	vm.curexc.Traceback = nil
	vm.AddTraceback(&vm.curexc)
	vm.why = whyException
	// Publish it as the exception now being handled, so that sys.exc_info()
	// and traceback.format_exc() work inside an except block even when the
	// exception came from a Go-level method rather than a Python "raise".
	//
	// SetCurrentException was called ONLY on the raise statement, so
	// "try: [][5] / except IndexError: sys.exc_info()" gave None - and with it
	// traceback.format_exc() returned "NoneType: None" for every error raised
	// by the interpreter itself.  That is how pip reported its own failures,
	// so a broken run looked like no error at all.
	if exc, ok := exception.(*py.Exception); ok {
		py.SetCurrentException(exc)
	}
}

// Check for an exception (panic)
//
// Should be called with the result of recover
func (vm *Vm) CheckExceptionRecover(r interface{}) {
	// If what was raised was an ExceptionInfo the stuff this into the current vm
	if exc, ok := r.(py.ExceptionInfo); ok {
		vm.curexc = exc
		vm.AddTraceback(&vm.curexc)
		vm.why = whyException
		if debugging {
			debugf("*** Propagating exception: %s\n", exc.Error())
		}
	} else {
		// Coerce whatever was raised into a *Exception
		vm.SetException(py.MakeException(r))
		if debugging {
			debugf("*** Exception raised %v\n", r)
			debug.PrintStack()
		}
	}
}

// Check for an exception (panic)
//
// Must be called as a defer function
func (vm *Vm) CheckException() {
	if r := recover(); r != nil {
		if debugging {
			debugf("*** Panic recovered %v\n", r)
		}
		vm.CheckExceptionRecover(r)
	}
}

// Illegal instruction
func do_ILLEGAL(vm *Vm, arg int32) error {
	return py.ExceptionNewf(py.SystemError, "Illegal opcode")
}

// Do nothing code. Used as a placeholder by the bytecode optimizer.
func do_NOP(vm *Vm, arg int32) error {
	return nil
}

// Removes the top-of-stack (TOS) item.
func do_POP_TOP(vm *Vm, arg int32) error {
	vm.DROPN(1)
	return nil
}

// Swaps the two top-most stack items.
func do_ROT_TWO(vm *Vm, arg int32) error {
	top := vm.TOP()
	second := vm.SECOND()
	vm.SET_TOP(second)
	vm.SET_SECOND(top)
	return nil
}

// Lifts second and third stack item one position up, moves top down
// to position three.
func do_ROT_THREE(vm *Vm, arg int32) error {
	top := vm.TOP()
	second := vm.SECOND()
	third := vm.THIRD()
	vm.SET_TOP(second)
	vm.SET_SECOND(third)
	vm.SET_THIRD(top)
	return nil
}

// Duplicates the reference on top of the stack.
func do_DUP_TOP(vm *Vm, arg int32) error {
	vm.PUSH(vm.TOP())
	return nil
}

// Duplicates the top two reference on top of the stack.
func do_DUP_TOP_TWO(vm *Vm, arg int32) error {
	top := vm.TOP()
	second := vm.SECOND()
	vm.PUSH(second)
	vm.PUSH(top)
	return nil
}

// Unary Operations take the top of the stack, apply the operation,
// and push the result back on the stack.

// Set the Top and return the error
func (vm *Vm) setTopAndCheckErr(obj py.Object, err error) error {
	if err == nil {
		vm.SET_TOP(obj)
	}
	return err
}

// Implements TOS = +TOS.
func do_UNARY_POSITIVE(vm *Vm, arg int32) error {
	return vm.setTopAndCheckErr(py.Pos(vm.TOP()))
}

// Implements TOS = -TOS.
func do_UNARY_NEGATIVE(vm *Vm, arg int32) error {
	return vm.setTopAndCheckErr(py.Neg(vm.TOP()))
}

// Implements TOS = not TOS.
func do_UNARY_NOT(vm *Vm, arg int32) error {
	return vm.setTopAndCheckErr(py.Not(vm.TOP()))
}

// Implements TOS = ~TOS.
func do_UNARY_INVERT(vm *Vm, arg int32) error {
	return vm.setTopAndCheckErr(py.Invert(vm.TOP()))
}

// Implements TOS = iter(TOS).
func do_GET_ITER(vm *Vm, arg int32) error {
	return vm.setTopAndCheckErr(py.Iter(vm.TOP()))
}

// Binary operations remove the top of the stack (TOS) and the second
// top-most stack item (TOS1) from the stack. They perform the
// operation, and put the result back on the stack.

// Implements TOS = TOS1 ** TOS.
func do_BINARY_POWER(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.Pow(a, b, py.None))
}

// Implements TOS = TOS1 * TOS.
func do_BINARY_MULTIPLY(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.Mul(a, b))
}

// Implements TOS = TOS1 // TOS.
func do_BINARY_FLOOR_DIVIDE(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.FloorDiv(a, b))
}

// Implements TOS = TOS1 / TOS when from __future__ import division is
// in effect.
func do_BINARY_TRUE_DIVIDE(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.TrueDiv(a, b))
}

// Implements TOS = TOS1 % TOS.
func do_BINARY_MODULO(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.Mod(a, b))
}

// Implements TOS = TOS1 + TOS.
func do_BINARY_ADD(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	// Fastpath: two ints add without going through the operator protocol.
	//
	// CPython does exactly this in its main loop, and the reason is visible in
	// a profile: the general path asserts two interfaces, calls M__add__, and
	// allocates a fresh Int for the result.  "x = x + i" in a loop pays that
	// on every iteration, and the allocation shows up as mallocgcTiny.
	//
	// The fallthrough is unchanged, so a subclass, a float, or a type with
	// its own __radd__ takes the slower path exactly as before.
	if ai, ok := a.(py.Int); ok {
		if bi, ok := b.(py.Int); ok {
			sum := ai + bi
			// Only take the fast path when the result is the same as the
			// protocol would produce: an overflow becomes a long in CPython,
			// and this interpreter has a BigInt behind the protocol.
			if !(ai > 0 && bi > 0 && sum < 0 || ai < 0 && bi < 0 && sum > 0) {
				vm.SET_TOP(sum)
				return nil
			}
		}
	}
	return vm.setTopAndCheckErr(py.Add(a, b))
}

// Implements TOS = TOS1 - TOS.
func do_BINARY_SUBTRACT(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.Sub(a, b))
}

// Implements TOS = TOS1[TOS].
func do_BINARY_SUBSCR(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.GetItem(a, b))
}

// Implements TOS = TOS1 << TOS.
func do_BINARY_LSHIFT(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.Lshift(a, b))
}

// Implements TOS = TOS1 >> TOS.
func do_BINARY_RSHIFT(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.Rshift(a, b))
}

// Implements TOS = TOS1 & TOS.
func do_BINARY_AND(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.And(a, b))
}

// Implements TOS = TOS1 ^ TOS.
func do_BINARY_XOR(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.Xor(a, b))
}

// Implements TOS = TOS1 | TOS.
func do_BINARY_OR(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.Or(a, b))
}

// In-place operations are like binary operations, in that they remove
// TOS and TOS1, and push the result back on the stack, but the
// operation is done in-place when TOS1 supports it, and the resulting
// TOS may be (but does not have to be) the original TOS1.

// Implements in-place TOS = TOS1 ** TOS.
func do_INPLACE_POWER(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.IPow(a, b, py.None))
}

// Implements in-place TOS = TOS1 * TOS.
func do_INPLACE_MULTIPLY(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.IMul(a, b))
}

// Implements in-place TOS = TOS1 // TOS.
func do_INPLACE_FLOOR_DIVIDE(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.IFloorDiv(a, b))
}

// Implements in-place TOS = TOS1 / TOS when from __future__ import
// division is in effect.
func do_INPLACE_TRUE_DIVIDE(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.ITrueDiv(a, b))
}

// Implements in-place TOS = TOS1 % TOS.
func do_INPLACE_MODULO(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.Mod(a, b))
}

// Implements in-place TOS = TOS1 + TOS.
func do_INPLACE_ADD(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.IAdd(a, b))
}

// Implements in-place TOS = TOS1 - TOS.
func do_INPLACE_SUBTRACT(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.ISub(a, b))
}

// Implements in-place TOS = TOS1 << TOS.
func do_INPLACE_LSHIFT(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.ILshift(a, b))
}

// Implements in-place TOS = TOS1 >> TOS.
func do_INPLACE_RSHIFT(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.IRshift(a, b))
}

// Implements in-place TOS = TOS1 & TOS.
func do_INPLACE_AND(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.IAnd(a, b))
}

// Implements in-place TOS = TOS1 ^ TOS.
func do_INPLACE_XOR(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.IXor(a, b))
}

// Implements in-place TOS = TOS1 | TOS.
func do_INPLACE_OR(vm *Vm, arg int32) error {
	b := vm.POP()
	a := vm.TOP()
	return vm.setTopAndCheckErr(py.IOr(a, b))
}

// Implements TOS1[TOS] = TOS2.
func do_STORE_SUBSCR(vm *Vm, arg int32) error {
	w := vm.TOP()
	v := vm.SECOND()
	u := vm.THIRD()
	vm.DROPN(3)
	// v[w] = u
	_, err := py.SetItem(v, w, u)
	if err != nil {
		return err
	}
	return nil
}

// Implements del TOS1[TOS].
func do_DELETE_SUBSCR(vm *Vm, arg int32) error {
	sub := vm.TOP()
	container := vm.SECOND()
	vm.DROPN(2)
	/* del v[w] */
	_, err := py.DelItem(container, sub)
	if err != nil {
		return err
	}
	return nil
}

// Miscellaneous opcodes.

// PrintExpr controls where the output of PRINT_EXPR goes which is
// used in the REPL
var PrintExpr = func(out string) {
	_, _ = os.Stdout.WriteString(out + "\n")
}

// Implements the expression statement for the interactive mode. TOS
// is removed from the stack and printed. In non-interactive mode, an
// expression statement is terminated with POP_STACK.
func do_PRINT_EXPR(vm *Vm, arg int32) error {
	// FIXME this should be calling sys.displayhook

	// Print value except if None
	// After printing, also assign to '_'
	// Before, set '_' to None to avoid recursion
	value := vm.POP()
	vm.frame.Globals.Set("_", py.None)
	if value != py.None {
		repr, err := py.Repr(value)
		if err != nil {
			return err
		}
		PrintExpr(fmt.Sprint(repr))
	}
	vm.frame.Globals.Set("_", value)
	return nil
}

// Terminates a loop due to a break statement.
func do_BREAK_LOOP(vm *Vm, arg int32) error {
	vm.why = whyBreak
	return nil
}

// Continues a loop due to a continue statement. target is the address
// to jump to (which should be a FOR_ITER instruction).
func do_CONTINUE_LOOP(vm *Vm, target int32) error {
	vm.retval = py.Int(target)
	vm.why = whyContinue
	return nil
}

// Iterate v argcnt times and store the results on the stack (via decreasing
// sp).  Return 1 for success, 0 if error.
//
// If argcntafter == -1, do a simple unpack. If it is >= 0, do an unpack
// with a variable target.
func unpack_iterable(vm *Vm, v py.Object, argcnt int, argcntafter int, sp int) error {
	it, err := py.Iter(v)
	if err != nil {
		return err
	}
	i := 0
	for i = 0; i < argcnt; i++ {
		w, err := py.Next(it)
		if err != nil {
			/* Iterator done, via error or exhaustion. */
			return py.ExceptionNewf(py.ValueError, "need more than %d value(s) to unpack", i)
		}
		sp--
		vm.frame.Stack[sp] = w
	}

	if argcntafter == -1 {
		/* We better have exhausted the iterator now. */
		_, finished := py.Next(it)
		if finished != nil {
			return nil
		}
		return py.ExceptionNewf(py.ValueError, "too many values to unpack (expected %d)", argcnt)
	}

	l, err := py.SequenceList(it)
	if err != nil {
		return err
	}
	sp--
	vm.frame.Stack[sp] = l
	i++

	ll := l.Len()
	if ll < argcntafter {
		return py.ExceptionNewf(py.ValueError, "need more than %d values to unpack", argcnt+ll)
	}

	/* Pop the "after-variable" args off the list. */
	for j := argcntafter; j > 0; j-- {
		sp--
		vm.frame.Stack[sp], err = l.M__getitem__(py.Int(ll - j))
		if err != nil {
			return err
		}
	}
	/* Resize the list. */
	l.Resize(ll - argcntafter)
	return nil
}

// Implements assignment with a starred target: Unpacks an iterable in
// TOS into individual values, where the total number of values can be
// smaller than the number of items in the iterable: one the new
// values will be a list of all leftover items.
//
// The low byte of counts is the number of values before the list
// value, the high byte of counts the number of values after it. The
// resulting values are put onto the stack right-to-left.
func do_UNPACK_EX(vm *Vm, counts int32) error {
	before := int(counts & 0xFF)
	after := int(counts >> 8)
	totalargs := 1 + before + after
	seq := vm.POP()
	sp := vm.STACK_LEVEL()
	vm.EXTEND(make([]py.Object, totalargs))
	return unpack_iterable(vm, seq, before, after, sp+totalargs)
}

// Calls set.add(TOS1[-i], TOS). Used to implement set comprehensions.
func do_SET_ADD(vm *Vm, i int32) error {
	w := vm.POP()
	v := vm.PEEK(int(i))
	// Propagate an unhashable element: "{[1]}" must raise instead of
	// silently omitting the element.
	return v.(*py.Set).AddErr(w)
}

// Calls list.append(TOS[-i], TOS). Used to implement list
// comprehensions. While the appended value is popped off, the list
// object remains on the stack so that it is available for further
// iterations of the loop.
func do_LIST_APPEND(vm *Vm, i int32) error {
	w := vm.POP()
	v := vm.PEEK(int(i))
	if l, ok := v.(*py.List); ok {
		l.Append(w)
		return nil
	}
	// The same opcode grows a TUPLE in a tuple display: a tuple cannot be
	// mutated, so the element is appended to a copy which replaces the one
	// on the stack.  Without this "(*a, b)" panicked with "interface
	// conversion: py.Object is py.Tuple, not *py.List".
	if t, ok := v.(py.Tuple); ok {
		merged := make(py.Tuple, 0, len(t)+1)
		merged = append(merged, t...)
		merged = append(merged, w)
		vm.SET_VALUE(int(i), merged)
		return nil
	}
	return py.ExceptionNewf(py.SystemError, "LIST_APPEND: expected a list or tuple, got %s", v.Type().Name)
}

// Extends the list at TOS1[-i] with the iterable at TOS, as in
// "[*a, b]".  Used to implement PEP 448 unpacking in a list display.
// do_TUPLE_EXTEND implements TUPLE_EXTEND, the tuple counterpart of
// LIST_EXTEND.  It exists because a tuple cannot be mutated in place: the
// compiler builds a fresh tuple and the concatenation produces a new one,
// which is then stored back over the old at TOS1[-i].
func do_TUPLE_EXTEND(vm *Vm, i int32) error {
	iterable := vm.POP()
	tupleObj := vm.PEEK(int(i))
	tuple, ok := tupleObj.(py.Tuple)
	if !ok {
		return py.ExceptionNewf(py.SystemError, "TUPLE_EXTEND: expected a tuple, got %s", tupleObj.Type().Name)
	}
	items, err := py.SequenceList(iterable)
	if err != nil {
		return err
	}
	merged := make(py.Tuple, 0, len(tuple)+len(items.Items))
	merged = append(merged, tuple...)
	merged = append(merged, items.Items...)
	vm.SET_VALUE(int(i), merged)
	return nil
}

func do_LIST_EXTEND(vm *Vm, i int32) error {
	iterable := vm.POP()
	listObj := vm.PEEK(int(i))
	list, ok := listObj.(*py.List)
	if !ok {
		return py.ExceptionNewf(py.SystemError, "LIST_EXTEND: expected a list, got %s", listObj.Type().Name)
	}
	items, err := py.SequenceList(iterable)
	if err != nil {
		return err
	}
	list.Items = append(list.Items, items.Items...)
	return nil
}

// Updates the dict at TOS1[-i] with the mapping at TOS, as in "{**a, b: c}".
// Used to implement PEP 448 unpacking in a dict display.
func do_DICT_UPDATE(vm *Vm, i int32) error {
	source := vm.POP()
	target := vm.PEEK(int(i))
	dict, ok := target.(py.StringDict)
	if !ok {
		return py.ExceptionNewf(py.SystemError, "DICT_UPDATE: expected a dict, got %s", target.Type().Name)
	}
	// The mapping is read through the dict protocol, so any mapping whose
	// items can be iterated updates the target.
	src, ok := source.(py.IGetDict)
	if !ok {
		return py.ExceptionNewf(py.TypeError, "'%s' object is not a mapping", source.Type().Name)
	}
	// Through the source dict's DecodeKey, which can recover a key whose type
	// defines __hash__; DictKeyDecode cannot, and "{**d}" of such a key would
	// raise "corrupt dict key".
	srcDict := src.GetDict()
	var updErr error
	srcDict.Range(func(encoded string, value py.Object) bool {
		key, err := srcDict.DecodeKey(encoded)
		if err != nil {
			updErr = err
			return true
		}
		if _, err := dict.M__setitem__(key, value); err != nil {
			updErr = err
			return true
		}
		return false
	})
	return updErr
}

// do_LIST_EXTEND_MAPPING implements LIST_EXTEND_MAPPING.  It extends the list
// at TOS1[-i] with the key/value pairs of the mapping at TOS, flattened as
// [k1, v1, k2, v2, ...], which is the keyword-argument list a CALL_FUNCTION_EX
// consumes.  A mapping is read through the same dict protocol as DICT_UPDATE,
// and a non-string key raises here, as CPython does when the call is built.
func do_LIST_EXTEND_MAPPING(vm *Vm, i int32) error {
	source := vm.POP()
	listObj := vm.PEEK(int(i))
	list, ok := listObj.(*py.List)
	if !ok {
		return py.ExceptionNewf(py.SystemError, "LIST_EXTEND_MAPPING: expected a list, got %s", listObj.Type().Name)
	}
	src, ok := source.(py.IGetDict)
	if !ok {
		return py.ExceptionNewf(py.TypeError, "'%s' object is not a mapping", source.Type().Name)
	}
	srcDict := src.GetDict()
	var updErr error
	srcDict.Range(func(encoded string, value py.Object) bool {
		key, err := srcDict.DecodeKey(encoded)
		if err != nil {
			updErr = err
			return true
		}
		k, ok := key.(py.String)
		if !ok {
			updErr = py.ExceptionNewf(py.TypeError, "keywords must be strings")
			return true
		}
		list.Items = append(list.Items, k, value)
		return false
	})
	return updErr
}

// Calls dict.setitem(TOS1[-i], TOS, TOS1). Used to implement dict comprehensions.
func do_MAP_ADD(vm *Vm, i int32) error {
	key := vm.TOP()
	value := vm.SECOND()
	vm.DROPN(2)
	dictObj := vm.PEEK(int(i))
	dict, err := py.DictCheckExact(dictObj)
	if err != nil {
		return err
	}
	_, err = dict.M__setitem__(key, value)
	return err
}

// Returns with TOS to the caller of the function.
func do_RETURN_VALUE(vm *Vm, arg int32) error {
	vm.retval = vm.POP()
	vm.frame.Yielded = false
	vm.why = whyReturn
	return nil
}

// Pops TOS and delegates to it as a subiterator from a generator.
func do_YIELD_FROM(vm *Vm, arg int32) error {

	var retval py.Object
	var err error
	u := vm.POP()
	x := vm.TOP()
	// send u to x
	if u == py.None {
		retval, err = py.Next(x)
	} else {
		retval, err = py.Send(x, u)

	}
	if err != nil {
		if !py.IsException(py.StopIteration, err) {
			return err
		}
		return nil
	}
	// x remains on stack, retval is value to be yielded
	// FIXME vm.frame.Stacktop = stack_pointer
	//why = whyYield
	// and repeat...
	vm.frame.Lasti--

	vm.retval = retval
	vm.frame.Yielded = true
	vm.why = whyYield
	return nil
}

// Pops TOS and yields it from a generator.
func do_YIELD_VALUE(vm *Vm, arg int32) error {
	vm.retval = vm.POP()
	vm.frame.Yielded = true
	vm.why = whyYield
	return nil
}

// Loads all symbols not starting with '_' directly from the module
// TOS to the local namespace. The module is popped after loading all
// names. This opcode implements from module import *.
func do_IMPORT_STAR(vm *Vm, arg int32) error {
	vm.frame.FastToLocals()
	from := vm.POP()
	module := from.(*py.Module)
	if all, ok := module.Globals.Get("__all__"); ok {
		var loopErr error
		iterErr := py.Iterate(all, func(item py.Object) bool {
			name, err := py.AttributeName(item)
			if err != nil {
				loopErr = err
				return true
			}
			value, err := py.GetAttrString(module, name)
			if err != nil {
				loopErr = err
				return true
			}
			vm.frame.Locals.Set(name, value)
			return false
		})
		if iterErr != nil {
			return iterErr
		}
		if loopErr != nil {
			return loopErr
		}
	} else {
		module.Globals.Range(func(name string, value py.Object) bool {
			if !strings.HasPrefix(name, "_") {
				vm.frame.Locals.Set(name, value)
			}
			return false
		})
	}
	vm.frame.LocalsToFast(false)
	return nil
}

// Removes one block from the block stack. Per frame, there is a stack
// of blocks, denoting nested loops, try statements, and such.
func do_POP_BLOCK(vm *Vm, arg int32) error {
	vm.frame.PopBlock()
	return nil
}

// Removes one block from the block stack. The popped block must be an
// exception handler block, as implicitly created when entering an
// except handler. In addition to popping extraneous values from the
// frame stack, the last three popped values are used to restore the
// exception state.
func do_POP_EXCEPT(vm *Vm, arg int32) error {
	frame := vm.frame
	b := vm.frame.Block
	frame.PopBlock()
	if b.Type != py.TryBlockExceptHandler {
		return py.ExceptionNewf(py.SystemError, "popped block is not an except handler")
	}
	// Leaving the handler restores the exception that was being handled
	// OUTSIDE it, which is the one saved on entry - or nothing at all when
	// this was the outermost handler.  Without this sys.exc_info() kept
	// reporting a handler that had already finished.
	if n := len(vm.excStack); n > 0 {
		vm.exc = vm.excStack[n-1]
		vm.excStack = vm.excStack[:n-1]
	} else {
		vm.exc = py.ExceptionInfo{}
	}
	py.SetCurrentExceptionFromValue(vm.exc.Value)
	vm.UnwindExceptHandler(frame, b)
	return nil
}

// Terminates a finally clause. The interpreter recalls whether the
// exception has to be re-raised, or whether the function returns, and
// continues with the outer-next block.
func do_END_FINALLY(vm *Vm, arg int32) error {
	v := vm.POP()
	if debugging {
		debugf("END_FINALLY v=%#v\n", v)
	}
	if v == py.None {
		// None exception
		if debugging {
			debugf(" END_FINALLY: None\n")
		}
	} else if vInt, ok := v.(py.Int); ok {
		vm.why = vmStatus(vInt)
		if debugging {
			debugf(" END_FINALLY: Int %v\n", vm.why)
		}
		switch vm.why {
		case whyYield:
			panic("vm: Unexpected whyYield in END_FINALLY")
		case whyException:
			panic("vm: Unexpected whyException in END_FINALLY")
		case whyReturn, whyContinue:
			vm.retval = vm.POP()
		case whySilenced:
			// An exception was silenced by 'with', we must
			// manually unwind the EXCEPT_HANDLER block which was
			// created when the exception was caught, otherwise
			// the stack will be in an inconsistent state.
			frame := vm.frame
			b := vm.frame.Block
			frame.PopBlock()
			if b.Type != py.TryBlockExceptHandler {
				panic("vm: Expecting EXCEPT_HANDLER in END_FINALLY")
			}
			vm.UnwindExceptHandler(frame, b)
			vm.why = whyNot
		}
	} else if py.ExceptionClassCheck(v) {
		w := vm.POP()
		u := vm.POP()
		if debugging {
			debugf(" END_FINALLY: Exc %v, Type %v, Traceback %v\n", v, w, u)
		}
		// FIXME PyErr_Restore(v, w, u)
		vm.curexc.Type, _ = v.(*py.Type)
		vm.curexc.Value = w
		vm.curexc.Traceback, _ = u.(*py.Traceback)
		vm.why = whyException
	} else {
		return py.ExceptionNewf(py.SystemError, "'finally' pops bad exception %#v", v)
	}
	if debugging {
		debugf("END_FINALLY: vm.why = %v\n", vm.why)
	}
	return nil
}

// Loads the __build_class__ helper function to the stack which
// creates a new class object.
func do_LOAD_BUILD_CLASS(vm *Vm, arg int32) error {
	vm.PUSH(vm.context.Store().Builtins.Globals.GetOrNil("__build_class__"))
	return nil
}

// This opcode performs several operations before a with block
// starts. First, it loads __exit__( ) from the context manager and
// pushes it onto the stack for later use by WITH_CLEANUP. Then,
// __enter__( ) is called, and a finally block pointing to delta is
// pushed. Finally, the result of calling the enter method is pushed
// onto the stack. The next opcode will either ignore it (POP_TOP), or
// store it in (a) variable(s) (STORE_FAST, STORE_NAME, or
// UNPACK_SEQUENCE).
func do_SETUP_WITH(vm *Vm, delta int32) error {
	mgr := vm.TOP()
	// exit := py.ObjectLookupSpecial(mgr, "__exit__")
	exit, err := py.GetAttrString(mgr, "__exit__")
	if err != nil {
		return err
	}
	vm.SET_TOP(exit)
	// enter := py.ObjectLookupSpecial(mgr, "__enter__")
	enter, err := py.GetAttrString(mgr, "__enter__")
	if err != nil {
		return err
	}
	res, err := py.Call(enter, nil, py.StringDict{}) // FIXME method for this?
	if err != nil {
		return err
	}
	// Setup the finally block before pushing the result of __enter__ on the stack.
	vm.frame.PushBlock(py.TryBlockSetupFinally, vm.frame.Lasti+delta, vm.STACK_LEVEL())
	vm.PUSH(res)
	return nil
}

// Cleans up the stack when a with statement block exits. On top of
// the stack are 1–3 values indicating how/why the finally clause was
// entered:
//
// TOP = None
// (TOP, SECOND) = (WHY_{RETURN,CONTINUE}), retval
// TOP = WHY_*; no retval below it
// (TOP, SECOND, THIRD) = exc_info( )
// Under them is EXIT, the context manager’s __exit__( ) bound method.
//
// In the last case, EXIT(TOP, SECOND, THIRD) is called, otherwise
// EXIT(None, None, None).
//
// EXIT is removed from the stack, leaving the values above it in the
// same order. In addition, if the stack represents an exception, and
// the function call returns a ‘true’ value, this information is
// “zapped”, to prevent END_FINALLY from re-raising the
// exception. (But non-local gotos should still be resumed.)
func do_WITH_CLEANUP(vm *Vm, arg int32) error {
	var exit_func py.Object

	exc := vm.TOP()
	var val py.Object = py.None
	var tb py.Object = py.None
	if exc == py.None {
		vm.DROP()
		exit_func = vm.TOP()
		vm.SET_TOP(exc)
	} else if excInt, ok := exc.(py.Int); ok {
		vm.DROP()
		switch vmStatus(excInt) {
		case whyReturn, whyContinue:
			/* Retval in TOP. */
			exit_func = vm.SECOND()
			vm.SET_SECOND(vm.TOP())
			vm.SET_TOP(exc)
		default:
			exit_func = vm.TOP()
			vm.SET_TOP(exc)
		}
		exc = py.None
	} else {
		val = vm.SECOND()
		tb = vm.THIRD()
		tp2 := vm.FOURTH()
		exc2 := vm.PEEK(5)
		tb2 := vm.PEEK(6)
		exit_func = vm.PEEK(7)
		vm.SET_VALUE(7, tb2)
		vm.SET_VALUE(6, exc2)
		vm.SET_VALUE(5, tp2)
		/* UNWIND_EXCEPT_HANDLER will pop this off. */
		vm.SET_FOURTH(nil)
		/* We just shifted the stack down, so we have
		   to tell the except handler block that the
		   values are lower than it expects. */
		block := vm.frame.Block
		if block.Type != py.TryBlockExceptHandler {
			panic("vm: WITH_CLEANUP expecting TryBlockExceptHandler")
		}
		block.Level--
	}
	/* XXX Not the fastest way to call it... */
	res, err := py.Call(exit_func, []py.Object{exc, val, tb}, py.StringDict{})
	if err != nil {
		return err
	}

	wasErr := false
	if exc != py.None {
		wasErr = res == py.True
	}
	if wasErr {
		/* There was an exception and a True return */
		vm.PUSH(py.Int(whySilenced))
	}
	return nil
}

// All of the following opcodes expect arguments. An argument is two bytes, with the more significant byte last.

// Implements name = TOS. namei is the index of name in the attribute
// co_names of the code object. The compiler tries to use STORE_FAST
// or STORE_GLOBAL if possible.
func do_STORE_NAME(vm *Vm, namei int32) error {
	if debugging {
		debugf("STORE_NAME %v\n", vm.frame.Code.Names[namei])
	}
	vm.frame.Locals.Set(vm.frame.Code.Names[namei], vm.POP())
	return nil
}

// Implements del name, where namei is the index into co_names
// attribute of the code object.
func do_DELETE_NAME(vm *Vm, namei int32) error {
	name := vm.frame.Code.Names[namei]
	if _, ok := vm.frame.Locals.Get(name); !ok {
		return py.ExceptionNewf(py.NameError, nameErrorMsg, name)
	} else {
		vm.frame.Locals.Del(name)
	}
	return nil
}

// Unpacks TOS into count individual values, which are put onto the
// stack right-to-left.
func do_UNPACK_SEQUENCE(vm *Vm, count int32) error {
	it := vm.POP()
	args := int(count)
	if tuple, ok := it.(py.Tuple); ok && len(tuple) == args {
		vm.EXTEND_REVERSED(tuple)
	} else if list, ok := it.(*py.List); ok && list.Len() == args {
		vm.EXTEND_REVERSED(list.Items)
	} else {
		sp := vm.STACK_LEVEL()
		vm.EXTEND(make([]py.Object, args))
		return unpack_iterable(vm, it, args, -1, sp+args)
	}
	return nil
}

// Implements TOS.name = TOS1, where namei is the index of name in
// co_names.
func do_STORE_ATTR(vm *Vm, namei int32) error {
	w := vm.frame.Code.Names[namei]
	v := vm.TOP()
	u := vm.SECOND()
	vm.DROPN(2)
	_, err := py.SetAttrString(v, w, u) /* v.w = u */
	if err != nil {
		return err
	}
	return nil
}

// Implements del TOS.name, using namei as index into co_names.
func do_DELETE_ATTR(vm *Vm, namei int32) error {
	return py.DeleteAttrString(vm.POP(), vm.frame.Code.Names[namei])
}

// Works as STORE_NAME, but stores the name as a global.
func do_STORE_GLOBAL(vm *Vm, namei int32) error {
	vm.frame.Globals.Set(vm.frame.Code.Names[namei], vm.POP())
	return nil
}

// Works as DELETE_NAME, but deletes a global name.
func do_DELETE_GLOBAL(vm *Vm, namei int32) error {
	name := vm.frame.Code.Names[namei]
	if _, ok := vm.frame.Globals.Get(name); !ok {
		return py.ExceptionNewf(py.NameError, nameErrorMsg, name)
	} else {
		vm.frame.Globals.Del(name)
	}
	return nil
}

// Pushes co_consts[consti] onto the stack.
func do_LOAD_CONST(vm *Vm, consti int32) error {
	vm.PUSH(vm.frame.Code.Consts[consti])
	// if debugging { debugf("LOAD_CONST %v\n", vm.TOP()) }
	return nil
}

// Pushes the value associated with co_names[namei] onto the stack.
func do_LOAD_NAME(vm *Vm, namei int32) error {
	name := vm.frame.Code.Names[namei]
	if debugging {
		debugf("LOAD_NAME %v\n", name)
	}
	obj, ok := vm.frame.Lookup(name)
	if !ok {
		return py.ExceptionNewf(py.NameError, nameErrorMsg, name)
	} else {
		vm.PUSH(obj)
	}
	return nil
}

// Creates a tuple consuming count items from the stack, and pushes
// the resulting tuple onto the stack.
func do_BUILD_TUPLE(vm *Vm, count int32) error {
	tuple := make(py.Tuple, count)
	copy(tuple, vm.frame.Stack[len(vm.frame.Stack)-int(count):])
	vm.DROPN(int(count))
	vm.PUSH(tuple)
	return nil
}

// Works as BUILD_TUPLE, but creates a set.
func do_BUILD_SET(vm *Vm, count int32) error {
	set, err := py.NewSetFromItemsErr(vm.frame.Stack[len(vm.frame.Stack)-int(count):])
	if err != nil {
		return err
	}
	vm.DROPN(int(count))
	vm.PUSH(set)
	return nil
}

// Works as BUILD_TUPLE, but creates a list.
func do_BUILD_LIST(vm *Vm, count int32) error {
	list := py.NewListFromItems(vm.frame.Stack[len(vm.frame.Stack)-int(count):])
	vm.DROPN(int(count))
	vm.PUSH(list)
	return nil
}

// Pushes a new dictionary object onto the stack. The dictionary is
// pre-sized to hold count entries.
func do_BUILD_MAP(vm *Vm, count int32) error {
	vm.PUSH(py.NewStringDictSized(int(count)))
	return nil
}

// Replaces TOS with getattr(TOS, co_names[namei]).
func do_LOAD_ATTR(vm *Vm, namei int32) error {
	return vm.setTopAndCheckErr(py.GetAttrString(vm.TOP(), vm.frame.Code.Names[namei]))
}

// Performs a Boolean operation. The operation name can be found in
// cmp_op[opname].
func do_COMPARE_OP(vm *Vm, opname int32) error {
	b := vm.POP()
	a := vm.TOP()
	var r py.Object
	var err error
	switch opname {
	case PyCmp_LT:
		r, err = py.Lt(a, b)
	case PyCmp_LE:
		r, err = py.Le(a, b)
	case PyCmp_EQ:
		r, err = py.Eq(a, b)
	case PyCmp_NE:
		r, err = py.Ne(a, b)
	case PyCmp_GT:
		r, err = py.Gt(a, b)
	case PyCmp_GE:
		r, err = py.Ge(a, b)
	case PyCmp_IN:
		var in bool
		in, err = py.SequenceContains(b, a)
		r = py.NewBool(in)
	case PyCmp_NOT_IN:
		var in bool
		in, err = py.SequenceContains(b, a)
		r = py.NewBool(!in)
	case PyCmp_IS:
		r = py.NewBool(a == b)
	case PyCmp_IS_NOT:
		r = py.NewBool(a != b)
	case PyCmp_EXC_MATCH:
		if bTuple, ok := b.(py.Tuple); ok {
			for _, exc := range bTuple {
				if !py.ExceptionClassCheck(exc) {
					return py.ExceptionNewf(py.TypeError, cannotCatchMsg, exc.Type().Name)
				}
			}
		} else {
			if !py.ExceptionClassCheck(b) {
				return py.ExceptionNewf(py.TypeError, cannotCatchMsg, b.Type().Name)
			}
		}
		r = py.NewBool(py.ExceptionGivenMatches(a, b))
	default:
		panic(fmt.Sprintf("vm: Unknown COMPARE_OP %v", opname))
	}
	if err != nil {
		return err
	}
	vm.SET_TOP(r)
	return nil
}

// Imports the module co_names[namei]. TOS and TOS1 are popped and
// provide the fromlist and level arguments of __import__( ). The
// module object is pushed onto the stack. The current namespace is
// not affected: for a proper import statement, a subsequent
// STORE_FAST instruction modifies the namespace.
func do_IMPORT_NAME(vm *Vm, namei int32) error {
	name := py.String(vm.frame.Code.Names[namei])
	__import__, ok := vm.frame.Builtins.Get("__import__")
	if !ok {
		return py.ExceptionNewf(py.ImportError, "__import__ not found")
	}
	v := vm.POP()
	u := vm.TOP()
	var locals py.Object = py.None
	if !vm.frame.Locals.IsNil() {
		locals = vm.frame.Locals
	}
	var args py.Tuple
	if _, ok := u.(py.Int); ok {
		args = py.Tuple{name, vm.frame.Globals, locals, v, u}
	} else {
		args = py.Tuple{name, vm.frame.Globals, locals, v}
	}
	x, err := callInternal(__import__, args, py.StringDict{}, vm.frame)
	if err != nil {
		return err
	}
	vm.SET_TOP(x)
	return nil
}

// Loads the attribute co_names[namei] from the module found in
// TOS. The resulting object is pushed onto the stack, to be
// subsequently stored by a STORE_FAST instruction.
func do_IMPORT_FROM(vm *Vm, namei int32) error {
	name := vm.frame.Code.Names[namei]
	module := vm.TOP()
	res, err := py.GetAttrString(module, name)
	if err != nil {
		// Catch AttributeError and rethrow as ImportError
		if py.IsException(py.AttributeError, err) {
			err = py.ExceptionNewf(py.ImportError, "cannot import name %s", name)
		}
		return err
	}
	vm.PUSH(res)
	return nil
}

// Increments bytecode counter by delta.
func do_JUMP_FORWARD(vm *Vm, delta int32) error {
	vm.frame.Lasti += delta
	return nil
}

// If TOS is true, sets the bytecode counter to target. TOS is popped.
func do_POP_JUMP_IF_TRUE(vm *Vm, target int32) error {
	b, err := py.MakeBool(vm.POP())
	if err != nil {
		return err
	}
	if b.(py.Bool) {
		vm.frame.Lasti = target
	}
	return nil
}

// If TOS is false, sets the bytecode counter to target. TOS is popped.
func do_POP_JUMP_IF_FALSE(vm *Vm, target int32) error {
	b, err := py.MakeBool(vm.POP())
	if err != nil {
		return err
	}
	if !b.(py.Bool) {
		vm.frame.Lasti = target
	}
	return nil
}

// If TOS is true, sets the bytecode counter to target and leaves TOS
// on the stack. Otherwise (TOS is false), TOS is popped.
func do_JUMP_IF_TRUE_OR_POP(vm *Vm, target int32) error {
	b, err := py.MakeBool(vm.TOP())
	if err != nil {
		return err
	}
	if b.(py.Bool) {
		vm.frame.Lasti = target
	} else {
		vm.DROP()
	}
	return nil
}

// If TOS is false, sets the bytecode counter to target and leaves TOS
// on the stack. Otherwise (TOS is true), TOS is popped.
func do_JUMP_IF_FALSE_OR_POP(vm *Vm, target int32) error {
	b, err := py.MakeBool(vm.TOP())
	if err != nil {
		return err
	}
	if !b.(py.Bool) {
		vm.frame.Lasti = target
	} else {
		vm.DROP()
	}
	return nil
}

// Set bytecode counter to target.
func do_JUMP_ABSOLUTE(vm *Vm, target int32) error {
	vm.frame.Lasti = target
	return nil
}

// TOS is an iterator. Call its next( ) method. If this yields a new
// value, push it on the stack (leaving the iterator below it). If the
// iterator indicates it is exhausted TOS is popped, and the bytecode
// counter is incremented by delta.
func do_FOR_ITER(vm *Vm, delta int32) error {
	r, finished := py.Next(vm.TOP())
	if finished != nil {
		vm.DROP()
		vm.frame.Lasti += delta
	} else {
		vm.PUSH(r)
	}
	return nil
}

// Loads the global named co_names[namei] onto the stack.
func do_LOAD_GLOBAL(vm *Vm, namei int32) error {
	name := vm.frame.Code.Names[namei]
	if debugging {
		debugf("LOAD_GLOBAL %v\n", name)
	}
	obj, ok := vm.frame.LookupGlobal(name)
	if !ok {
		return py.ExceptionNewf(py.NameError, nameErrorMsg, name)
	} else {
		vm.PUSH(obj)
	}
	return nil
}

// Pushes a block for a loop onto the block stack. The block spans
// from the current instruction with a size of delta bytes.
func do_SETUP_LOOP(vm *Vm, delta int32) error {
	vm.frame.PushBlock(py.TryBlockSetupLoop, vm.frame.Lasti+delta, vm.STACK_LEVEL())
	return nil
}

// Pushes a try block from a try-except clause onto the block
// stack. delta points to the first except block.
func do_SETUP_EXCEPT(vm *Vm, delta int32) error {
	vm.frame.PushBlock(py.TryBlockSetupExcept, vm.frame.Lasti+delta, vm.STACK_LEVEL())
	return nil
}

// Pushes a try block from a try-except clause onto the block
// stack. delta points to the finally block.
func do_SETUP_FINALLY(vm *Vm, delta int32) error {
	vm.frame.PushBlock(py.TryBlockSetupFinally, vm.frame.Lasti+delta, vm.STACK_LEVEL())
	return nil
}

// Store a key and value pair in a dictionary. Pops the key and value
// while leaving the dictionary on the stack.
func do_STORE_MAP(vm *Vm, arg int32) error {
	key := vm.TOP()
	value := vm.SECOND()
	dictObj := vm.THIRD()
	vm.DROPN(2)
	dict, err := py.DictCheckExact(dictObj)
	if err != nil {
		return err
	}
	_, err = dict.M__setitem__(key, value)
	return err
}

// Pushes a reference to the local co_varnames[var_num] onto the stack.
func do_LOAD_FAST(vm *Vm, var_num int32) error {
	value := vm.frame.LocalVars[var_num]
	if value != nil {
		vm.PUSH(value)
	} else {
		varname := vm.frame.Code.Varnames[var_num]
		return py.ExceptionNewf(py.NameError, nameErrorMsg, varname)
		// FIXME ceval.c says this, but it python3.4 returns the above
		// return py.ExceptionNewf(py.UnboundLocalError, unboundLocalErrorMsg, varname)
	}
	return nil
}

// Stores TOS into the local co_varnames[var_num].
func do_STORE_FAST(vm *Vm, var_num int32) error {
	vm.frame.LocalVars[var_num] = vm.POP()
	return nil
}

// Deletes local co_varnames[var_num].
func do_DELETE_FAST(vm *Vm, var_num int32) error {
	if vm.frame.LocalVars[var_num] == nil {
		varname := vm.frame.Code.Varnames[var_num]
		return py.ExceptionNewf(py.NameError, nameErrorMsg, varname)
		// FIXME ceval.c says this return py.ExceptionNewf(py.UnboundLocalError, unboundLocalErrorMsg, varname)
	} else {
		vm.frame.LocalVars[var_num] = nil
	}
	return nil
}

// Name of slot for LOAD_CLOSURE / LOAD_DEREF / etc
//
// Returns name of variable and bool, true for free var, false for
// cell var
func _var_name(vm *Vm, i int32) (string, bool) {
	cellvars := vm.frame.Code.Cellvars
	if int(i) < len(cellvars) {
		return cellvars[i], false
	}
	return vm.frame.Code.Freevars[int(i)-len(cellvars)], true
}

// Pushes a reference to the cell contained in slot i of the cell and
// free variable storage. The name of the variable is co_cellvars[i]
// if i is less than the length of co_cellvars. Otherwise it is
// co_freevars[i - len(co_cellvars)].
func do_LOAD_CLOSURE(vm *Vm, i int32) error {
	vm.PUSH(vm.frame.CellAndFreeVars[i])
	return nil
}

// writes the correct errors for an unbound deref
func unboundDeref(vm *Vm, i int32) error {
	varname, free := _var_name(vm, i)
	if free {
		return py.ExceptionNewf(py.NameError, unboundFreeErrorMsg, varname)
	} else {
		return py.ExceptionNewf(py.UnboundLocalError, unboundLocalErrorMsg, varname)
	}
}

// Loads the cell contained in slot i of the cell and free variable
// storage. Pushes a reference to the object the cell contains on the
// stack.
func do_LOAD_DEREF(vm *Vm, i int32) error {
	res := vm.frame.CellAndFreeVars[i].(*py.Cell).Get()
	if res == nil {
		return unboundDeref(vm, i)
	}
	vm.PUSH(res)
	return nil
}

// Much like LOAD_DEREF but first checks the locals dictionary before
// consulting the cell. This is used for loading free variables in
// class bodies.
func do_LOAD_CLASSDEREF(vm *Vm, i int32) error {
	name, _ := _var_name(vm, i)

	// Lookup in locals
	if obj, ok := vm.frame.Locals.Get(name); ok {
		vm.PUSH(obj)
	}
	// If that failed look at the cell
	res := vm.frame.CellAndFreeVars[i].(*py.Cell).Get()
	if res == nil {
		return unboundDeref(vm, i)
	} else {
		vm.PUSH(res)
	}
	return nil
}

// Stores TOS into the cell contained in slot i of the cell and free
// variable storage.
func do_STORE_DEREF(vm *Vm, i int32) error {
	cell := vm.frame.CellAndFreeVars[i].(*py.Cell)
	cell.Set(vm.POP())
	return nil
}

// Empties the cell contained in slot i of the cell and free variable
// storage. Used by the del statement.
func do_DELETE_DEREF(vm *Vm, i int32) error {
	cell := vm.frame.CellAndFreeVars[i].(*py.Cell)
	if cell.Get() == nil {
		return unboundDeref(vm, i)
	}
	cell.Delete()
	return nil
}

// Logic for the raise statement
func (vm *Vm) raise(exc, cause py.Object) error {
	if exc == nil {
		// raise (with no parameters == re-raise)
		if !vm.exc.IsSet() {
			return py.ExceptionNewf(py.RuntimeError, "No active exception to reraise")
		} else {
			// Resignal the exception
			vm.curexc = vm.exc
			// Signal the existing exception again
			vm.why = whyException

		}
	} else {
		// raise <instance>
		// raise <type>
		excException := py.MakeException(exc)
		// Record it for sys.exc_info().  This is where the interpreter knows
		// which exception is in play, and nothing clears it until another
		// raise, so an except clause calling exc_info() sees the exception it
		// is handling.
		py.SetCurrentException(excException)
		if debugging {
			debugf("raise: excException = %v\n", excException)
		}
		if cause != nil {
			excException.Cause = py.MakeException(cause)
		}
		return excException
	}
	return nil
}

// Raises an exception. argc indicates the number of parameters to the
// raise statement, ranging from 0 to 3. The handler will find the
// traceback as TOS2, the parameter as TOS1, and the exception as TOS.
func do_RAISE_VARARGS(vm *Vm, argc int32) error {
	var cause, exc py.Object
	switch argc {
	case 2:
		cause = vm.POP()
		fallthrough
	case 1:
		exc = vm.POP()
	case 0:
	default:
		panic("vm: Bad RAISE_VARARGS argc")
	}
	return vm.raise(exc, cause)
}

// Calls a function. The low byte of argc indicates the number of
// positional parameters, the high byte the number of keyword
// parameters. On the stack, the opcode finds the keyword parameters
// first. For each keyword argument, the value is on top of the
// key. Below the keyword parameters, the positional parameters are on
// the stack, with the right-most parameter on top. Below the
// parameters, the function object to call is on the stack. Pops all
// function arguments, and the function itself off the stack, and
// pushes the return value.
func do_CALL_FUNCTION(vm *Vm, argc int32) error {
	return vm.Call(argc, nil, nil)
}

// Implementation for MAKE_FUNCTION and MAKE_CLOSURE
func _make_function(vm *Vm, argc int32, opcode OpCode) error {
	posdefaults := argc & 0xff
	kwdefaults := (argc >> 8) & 0xff
	num_annotations := (argc >> 16) & 0x7fff
	qualname := vm.POP()
	code := vm.POP()

	codeObj, ok := code.(*py.Code)
	if !ok {
		return py.ExceptionNewf(py.SystemError, "MAKE_FUNCTION: code must be a code object, not %s", code.Type().Name)
	}

	// Python 3.3-3.10 push the function name (not the qualname) and
	// CPython tolerates it being any object, so only convert real strings.
	var qualnameStr string
	if q, ok := qualname.(py.String); ok {
		qualnameStr = string(q)
	}
	function := py.NewFunction(vm.context, codeObj, vm.frame.Globals, qualnameStr)

	if opcode == MAKE_CLOSURE {
		closure, ok := vm.POP().(py.Tuple)
		if !ok {
			return py.ExceptionNewf(py.SystemError, "MAKE_CLOSURE: closure must be a tuple")
		}
		function.Closure = closure
	}

	if num_annotations > 0 {
		names, ok := vm.POP().(py.Tuple) // names of args with annotations
		if !ok {
			return py.ExceptionNewf(py.SystemError, "MAKE_FUNCTION: annotations must be a tuple")
		}
		anns := py.NewStringDict()
		if int32(len(names))+1 != num_annotations {
			return py.ExceptionNewf(py.SystemError, "MAKE_FUNCTION: num_annotations wrong - corrupt bytecode?")
		}
		// The names come off the stack in DECLARATION order, and the values
		// above the last name in the same order - so both are read forwards.
		//
		// Walking the names BACKWARDS (which this did) reversed the insertion
		// order of an ordered dict, so f.__annotations__ listed "return" first
		// and the parameters after it: "def f(width: float, height: float) ->
		// float" gave ['return', 'height', 'width'] where CPython gives
		// ['width', 'height', 'return'].
		values := make(py.Tuple, len(names))
		for i := range names {
			name, ok := names[i].(py.String)
			if !ok {
				return py.ExceptionNewf(py.SystemError, "MAKE_FUNCTION: annotation name must be a string")
			}
			values[i] = py.Object(name)
		}
		// Pop the values, then re-read them in order: the stack gives the LAST
		// name's value first.
		popped := make(py.Tuple, len(names))
		for i := len(names) - 1; i >= 0; i-- {
			popped[i] = vm.POP()
		}
		for i, n := range values {
			anns.Set(string(n.(py.String)), popped[i])
		}
		function.Annotations = anns
	}

	if kwdefaults > 0 {
		// The stack holds them in SOURCE order - (name, value) per entry, pushed
		// left to right - so they are popped into a slice and filed in that
		// order.  Filling the dict as they are popped reversed it, because a
		// dict is insertion-ordered: "def h(*a, p, q=1, r=2)" reported
		// {'r': 2, 'q': 1} where CPython reports {'q': 1, 'r': 2}.
		type kwDefault struct {
			name  string
			value py.Object
		}
		pairs := make([]kwDefault, kwdefaults)
		for i := int(kwdefaults) - 1; i >= 0; i-- {
			v := vm.POP()   // default value
			key := vm.POP() // kw only arg name
			keyStr, ok := key.(py.String)
			if !ok {
				return py.ExceptionNewf(py.SystemError, "MAKE_FUNCTION: keyword argument name must be a string")
			}
			pairs[i] = kwDefault{name: string(keyStr), value: v}
		}
		defs := py.NewStringDict()
		for _, p := range pairs {
			defs.Set(p.name, p.value)
		}
		function.KwDefaults = defs
	}

	if posdefaults > 0 {
		defs := make(py.Tuple, posdefaults)
		for posdefaults--; posdefaults >= 0; posdefaults-- {
			defs[posdefaults] = vm.POP()
		}
		function.Defaults = defs
	}

	vm.PUSH(function)
	return nil
}

// Pushes a new function object on the stack. TOS is the code
// associated with the function. The function object is defined to
// have argc default parameters, which are found below TOS.
//
// FIXME these docs are slightly wrong.
func do_MAKE_FUNCTION(vm *Vm, argc int32) error {
	return _make_function(vm, argc, MAKE_FUNCTION)
}

// Creates a new function object, sets its func_closure slot, and
// pushes it on the stack. TOS is the code associated with the
// function, TOS1 the tuple containing cells for the closure’s free
// variables. The function also has argc default parameters, which are
// found below the cells.
func do_MAKE_CLOSURE(vm *Vm, argc int32) error {
	return _make_function(vm, argc, MAKE_CLOSURE)
}

// Pushes a slice object on the stack. argc must be 2 or 3. If it is
// 2, slice(TOS1, TOS) is pushed; if it is 3, slice(TOS2, TOS1, TOS)
// is pushed. See the slice( ) built-in function for more information.
func do_BUILD_SLICE(vm *Vm, argc int32) error {
	var step py.Object
	switch argc {
	case 2:
		step = py.None
	case 3:
		step = vm.POP()
	default:
		panic("vm: Bad value for argc in BUILD_SLICE")
	}
	stop := vm.POP()
	start := vm.TOP()
	x := py.NewSlice(start, stop, step)
	vm.SET_TOP(x)
	return nil
}

// positionalOnlyPassedByKeyword lists the positional-only parameters that a
// call supplied by keyword, comma-separated, or "" when none.
//
// It lists ALL of them rather than the first, because that is what CPython's
// message does: calling divide(a=1, b=2) for "def divide(a, b, /)" names both.
func positionalOnlyPassedByKeyword(co *py.Code, kws py.StringDict) string {
	if co.Posonlyargcount <= 0 {
		return ""
	}
	var found []string
	for i := 0; i < int(co.Posonlyargcount) && i < len(co.Varnames); i++ {
		if _, ok := kws.Get(co.Varnames[i]); ok {
			found = append(found, co.Varnames[i])
		}
	}
	return strings.Join(found, ", ")
}

// Prefixes any opcode which has an argument too big to fit into the
// default two bytes. ext holds two additional bytes which, taken
// together with the subsequent opcode’s argument, comprise a
// four-byte argument, ext being the two most-significant bytes.
func do_EXTENDED_ARG(vm *Vm, ext int32) error {
	vm.ext = ext
	vm.extended = true
	return nil
}

// Calls a function. argc is interpreted as in CALL_FUNCTION. The top
// element on the stack contains the variable argument list, followed
// by keyword and positional arguments.
func do_CALL_FUNCTION_VAR(vm *Vm, argc int32) error {
	args := vm.POP()
	return vm.Call(argc, args, nil)
}

// Calls a function. argc is interpreted as in CALL_FUNCTION. The top
// element on the stack contains the keyword arguments dictionary,
// followed by explicit keyword and positional arguments.
func do_CALL_FUNCTION_KW(vm *Vm, argc int32) error {
	kwargs := vm.POP()
	return vm.Call(argc, nil, kwargs)
}

// Calls a function. argc is interpreted as in CALL_FUNCTION. The top
// element on the stack contains the keyword arguments dictionary,
// followed by the variable-arguments tuple, followed by explicit
// keyword and positional arguments.
func do_CALL_FUNCTION_VAR_KW(vm *Vm, argc int32) error {
	kwargs := vm.POP()
	args := vm.POP()
	return vm.Call(argc, args, kwargs)
}

// do_CALL_FUNCTION_EX implements CALL_FUNCTION_EX.  The stack holds the
// callable, the positional argument sequence (a tuple or list) and the
// keyword argument list, which is a flat list of alternating key, value
// objects as built by LIST_EXTEND_MAPPING.  A separate list, rather than a
// merged dict, is what lets a duplicate keyword be reported against the
// callee's own name: the callee is still on the stack when the merge runs,
// and a dict could not have held the duplicate to begin with.
func do_CALL_FUNCTION_EX(vm *Vm, _ int32) error {
	kwlistObj := vm.POP()
	argsObj := vm.POP()
	fn := vm.POP()

	var args py.Tuple
	switch x := argsObj.(type) {
	case py.Tuple:
		args = x
	case *py.List:
		// A list is what BUILD_LIST produces; it is flattened into a tuple
		// so the callee sees an ordinary argument tuple.
		args = py.Tuple(x.Items)
	default:
		return py.ExceptionNewf(py.SystemError, "CALL_FUNCTION_EX: argument list must be a tuple, not %s", argsObj.Type().Name)
	}

	const multipleValues = "%s%s got multiple values for keyword argument '%s'"
	var kwargs py.StringDict
	if kwlist, ok := kwlistObj.(*py.List); ok && len(kwlist.Items) > 0 {
		if len(kwlist.Items)%2 != 0 {
			panic("vm: Odd length keyword list")
		}
		kwargs = py.NewStringDict()
		for i := 0; i < len(kwlist.Items); i += 2 {
			kPy, ok := kwlist.Items[i].(py.String)
			if !ok {
				return py.ExceptionNewf(py.TypeError, "keywords must be strings")
			}
			k := string(kPy)
			if _, found := kwargs.Get(k); found {
				return py.ExceptionNewf(py.TypeError, multipleValues, EvalGetFuncName(fn), EvalGetFuncDesc(fn), k)
			}
			kwargs.Set(k, kwlist.Items[i+1])
		}
	}

	obj, err := callInternal(fn, args, kwargs, vm.frame)
	if err != nil {
		return err
	}
	vm.PUSH(obj)
	return nil
}

// EvalGetFuncName returns the name of the function object passed in
func EvalGetFuncName(fn py.Object) string {
	switch x := fn.(type) {
	case *py.Method:
		return x.Name
	case *py.Function:
		return x.Name
	default:
		return fn.Type().Name
	}
}

// EvalGetFuncDesc returns a description of the arguments for the
// function object
func EvalGetFuncDesc(fn py.Object) string {
	switch fn.(type) {
	case *py.Method:
		return "()"
	case *py.Function:
		return "()"
	default:
		return " object"
	}
}

// As py.Call but takes an interpreter Frame object
//
// Used to implement some interpreter magic like locals(), globals() etc
func callInternal(fn py.Object, args py.Tuple, kwargs py.StringDict, f *py.Frame) (py.Object, error) {
	if method, ok := fn.(*py.Method); ok {
		switch x := method.Internal(); x {
		case py.InternalMethodNone:
		case py.InternalMethodGlobals:
			return f.Globals, nil
		case py.InternalMethodLocals:
			f.FastToLocals()
			return f.Locals, nil
		case py.InternalMethodImport:
			return py.BuiltinImport(f.Context, nil, args, kwargs, f.Globals)
		case py.InternalMethodEval:
			f.FastToLocals()
			return builtinEval(f.Context, args, kwargs, f.Locals, f.Globals, f.Builtins)
		case py.InternalMethodExec:
			f.FastToLocals()
			return builtinExec(f.Context, args, kwargs, f.Locals, f.Globals, f.Builtins)
		case py.InternalMethodVars:
			if kwargs.Len() > 0 {
				return nil, py.ExceptionNewf(py.TypeError, "vars() takes no keyword arguments")
			}
			switch len(args) {
			case 0:
				f.FastToLocals()
				return f.Locals, nil
			case 1:
				attr, err := py.GetAttrString(args[0], "__dict__")
				if err != nil {
					return nil, err
				}
				return attr, nil
			default:
				return nil, py.ExceptionNewf(py.TypeError, "vars() takes at most 1 argument (%d given)", len(args))
			}
		case py.InternalMethodDir:
			return builtinDir(f, args)
		case py.InternalMethodGetFrame:
			return builtinGetFrame(f, args)
		default:
			return nil, py.ExceptionNewf(py.SystemError, "Internal method %v not found", x)
		}
	}
	return py.Call(fn, args, kwargs)
}

// builtinGetFrame implements sys._getframe([depth]).  It lives here because
// the frame stack belongs to the running interpreter, not to any module.
func builtinGetFrame(f *py.Frame, args py.Tuple) (py.Object, error) {
	depth := 0
	if len(args) > 1 {
		return nil, py.ExceptionNewf(py.TypeError, "_getframe expected at most 1 argument, got %d", len(args))
	}
	if len(args) == 1 {
		n, err := py.IndexInt(args[0])
		if err != nil {
			return nil, err
		}
		depth = n
	}
	// CPython counts from the frame that called _getframe, as seen by
	// Python; here the caller of the internal method is the Python frame, so
	// start there and walk out.
	at := f.Context.Store().CurrentFrame()
	for i := 0; i < depth && at != nil; i++ {
		at = at.Back
	}
	if at == nil {
		return nil, py.ExceptionNewf(py.ValueError, "call stack is not deep enough")
	}
	return at, nil
}

// builtinDir implements dir().  It lives here rather than in the builtin
// package because the no-argument form needs the interpreter frame.
func builtinDir(f *py.Frame, args py.Tuple) (py.Object, error) {
	if len(args) > 1 {
		return nil, py.ExceptionNewf(py.TypeError, "dir expected at most 1 argument, got %d", len(args))
	}

	if len(args) == 0 {
		f.FastToLocals()
		names := make([]string, 0, f.Locals.Len())
		for _, name := range f.Locals.Keys() {
			names = append(names, name)
		}
		sort.Strings(names)
		return dirList(names), nil
	}

	obj := args[0]
	seen := map[string]bool{}

	// Attributes supplied by the type and everything it inherits.
	for t := obj.Type(); t != nil; t = t.Base {
		for _, name := range t.Dict.Keys() {
			seen[name] = true
		}
	}
	// Attributes carried by the object itself.
	if d, ok := obj.(py.IGetDict); ok {
		for _, name := range d.GetDict().Keys() {
			seen[name] = true
		}
	}
	if m, ok := obj.(*py.Module); ok {
		for _, name := range m.Globals.Keys() {
			seen[name] = true
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return dirList(names), nil
}

func dirList(names []string) *py.List {
	items := make([]py.Object, len(names))
	for i, name := range names {
		items[i] = py.String(name)
	}
	return py.NewListFromItems(items)
}

// Implements a function call - see CALL_FUNCTION for a description of
// how the arguments are arranged.
//
// # Optionally pass in args and kwargs
//
// The result is put on the stack
func (vm *Vm) Call(argc int32, starArgs py.Object, starKwargs py.Object) error {
	// if debugging { debugf("Stack: %v\n", vm.frame.Stack) }
	// if debugging { debugf("Locals: %v\n", vm.frame.Locals) }
	// if debugging { debugf("Globals: %v\n", vm.frame.Globals) }

	// Get the arguments off the stack
	nargs := int(argc & 0xFF)
	nkwargs := int((argc >> 8) & 0xFF)
	p, q := len(vm.frame.Stack)-2*nkwargs, len(vm.frame.Stack)
	kwargsTuple := vm.frame.Stack[p:q]
	p, q = p-nargs, p
	args := py.Tuple(vm.frame.Stack[p:q])
	p, q = p-1, p
	fn := vm.frame.Stack[p]
	// Drop everything off the stack
	vm.frame.Stack = vm.frame.Stack[:p]

	const multipleValues = "%s%s got multiple values for keyword argument '%s'"

	// if debugging { debugf("Call %T %v with args = %v, kwargsTuple = %v\n", fnObj, fnObj, args, kwargsTuple) }
	var kwargs py.StringDict
	if len(kwargsTuple) > 0 {
		// Convert kwargsTuple into dictionary
		if len(kwargsTuple)%2 != 0 {
			panic("vm: Odd length kwargsTuple")
		}
		kwargs = py.NewStringDict()
		for i := 0; i < len(kwargsTuple); i += 2 {
			kPy, ok := kwargsTuple[i].(py.String)
			if !ok {
				return py.ExceptionNewf(py.TypeError, "keywords must be strings")
			}
			k := string(kPy)
			v := kwargsTuple[i+1]
			if _, ok := kwargs.Get(k); ok {
				return py.ExceptionNewf(py.TypeError, multipleValues, EvalGetFuncName(fn), EvalGetFuncDesc(fn), k)
			}
			kwargs.Set(k, v)
		}
	}

	// Update with starKwargs if any
	if starKwargs != nil {
		if kwargs.IsNil() {
			kwargs = py.NewStringDict()
		}
		// FIXME should be some sort of dictionary iterator...
		starKwargsDict, ok := starKwargs.(py.StringDict)
		if !ok {
			return py.ExceptionNewf(py.SystemError, "FIXME can't use %T as **kwargs", starKwargs)
		}
		for _, __e := range starKwargsDict.Items() {
			k := __e.Key
			v := __e.Value

			if _, ok := kwargs.Get(k); ok {
				return py.ExceptionNewf(py.TypeError, multipleValues, EvalGetFuncName(fn), EvalGetFuncDesc(fn), k)
			}
			kwargs.Set(k, v)
		}
	}

	// Update with starArgs if any
	if starArgs != nil {
		// Copy the args off the stack if there are any
		args = append([]py.Object(nil), args...)
		err := py.Iterate(starArgs, func(item py.Object) bool {
			args = append(args, item)
			return false
		})
		if err != nil {
			return err
		}
	}

	// log.Printf("%s(args=%#v, kwargs=%#v)", EvalGetFuncName(fn), args, kwargs)
	// Call the function pushing the return on the stack
	obj, err := callInternal(fn, args, kwargs, vm.frame)
	if err != nil {
		return err
	}
	vm.PUSH(obj)
	return nil
}

// Unwinds the stack for a block
func (vm *Vm) UnwindBlock(frame *py.Frame, block *py.TryBlock) {
	if vm.STACK_LEVEL() > block.Level {
		frame.Stack = frame.Stack[:block.Level]
	}
}

// Unwinds the stack in the presence of an exception
func (vm *Vm) UnwindExceptHandler(frame *py.Frame, block *py.TryBlock) {
	if debugging {
		debugf("** UnwindExceptHandler stack depth %v\n", vm.STACK_LEVEL())
	}
	if vm.STACK_LEVEL() < block.Level+3 {
		panic("vm: Couldn't find traceback on stack")
	} else {
		frame.Stack = frame.Stack[:block.Level+3]
	}
	if debugging {
		debugf("** UnwindExceptHandler stack depth now %v\n", vm.STACK_LEVEL())
	}
	vm.exc.Type, _ = vm.POP().(*py.Type)
	vm.exc.Value = vm.POP()
	vm.exc.Traceback, _ = vm.POP().(*py.Traceback)
	if debugging {
		debugf("** UnwindExceptHandler exc = (type: %v, value: %v, traceback: %v)\n", vm.exc.Type, vm.exc.Value, vm.exc.Traceback)
	}
}

// Run the virtual machine on a Frame object
//
// FIXME figure out how we are going to signal exceptions!
//
// Returns an Object and an error.  The error will be a py.ExceptionInfo
//
// This is the equivalent of PyEval_EvalFrame
func RunFrame(frame *py.Frame) (res py.Object, err error) {
	// Every frame that executes passes through here - a function call, a
	// class body, a module body, and a generator each time it is resumed -
	// so this is the one place that needs to keep the chain of executing
	// frames correct.  The defer covers every exit: a return, a raised
	// exception, or a yield that suspends the frame.
	store := frame.Context.Store()
	store.PushFrame(frame)
	// The executing frame is recorded PER GOROUTINE, not in a package-level
	// variable: two interpreter contexts running in two goroutines each need
	// their own "current frame", and a shared variable made them fight over
	// one - a data race on this exact line, and the wrong frame handed to
	// super() when they interleaved.
	prevFrame := gCurrentFrame.get()
	gCurrentFrame.set(frame)
	defer func() {
		gCurrentFrame.set(prevFrame)
		store.PopFrame(frame)
	}()

	var vm = Vm{
		frame:   frame,
		context: frame.Context,
	}

	// FIXME need to do this to save the old exeption when we
	// yield from a generator.  Should save it in the Frame though
	// (see slots in the frame)

	// FIXME make a test for this!

	// FIXME
	// if (co->co_flags & CO_GENERATOR) {
	//     if (!throwflag && f->f_exc_type != NULL && f->f_exc_type != Py_None) {
	//         /* We were in an except handler when we left,
	//            restore the exception state which was put aside
	//            (see YIELD_VALUE). */
	//         swap_exc_state(tstate, f);
	//     }
	//     else
	//         save_exc_state(tstate, f);
	// }

	if int(frame.Lasti) >= len(frame.Code.Code) {
		return nil, py.ExceptionNewf(py.SystemError, "vm: instruction out of range - code most likely finished already")
	}

	var opcode OpCode
	var arg int32
	opcodes := frame.Code.Code
	for vm.why == whyNot {
		// A pending exception means generator.throw() was called: it is
		// raised here, at the point the frame resumes, so the frame's own
		// handlers see it exactly as if the generator had raised it.  This
		// has to happen inside the loop: the block-unwinding below runs at
		// the end of each iteration, and setting why before the loop would
		// stop the loop from ever running.
		injected := false
		if frame.PendingException != nil {
			pending := frame.PendingException
			frame.PendingException = nil
			vm.SetException(pending)
			injected = true
		}
		if debugging {
			debugf("* %4d:", frame.Lasti)
		}
		if injected {
			// Do not execute an instruction with the exception pending: go
			// straight to the unwinding at the end of the loop body, which
			// is what finds the frame's own exception handler.
			goto unwind
		}
		opcode = OpCode(opcodes[frame.Lasti])
		frame.Lasti++
		if opcode.HAS_ARG() {
			arg = int32(opcodes[frame.Lasti])
			frame.Lasti++
			arg += int32(opcodes[frame.Lasti]) << 8
			frame.Lasti++
			if vm.extended {
				arg += vm.ext << 16
			}
			if debugging {
				debugf(" %v(%d)\n", opcode, arg)
			}
		} else {
			if debugging {
				debugf(" %v\n", opcode)
			}
		}
		vm.extended = false
		// Dispatch on the opcode with a switch rather than an indirect call
		// through a function table (jumpTable, now gone).  Go has no computed
		// goto, which is what CPython uses, so a switch is the equivalent.
		//
		// It is a SMALL win, and the honest number matters: a microbenchmark
		// with one arithmetic operation per handler gave 8.2x, but that is
		// measuring the call overhead itself.  With realistic handler bodies
		// it is 1.8x, and end to end on a 3,000,000-iteration loop it is
		// 0.452-0.463s -> 0.443-0.453s, about 2-3%.  Do not expect an 8x here.
		//
		// The cases match the old jumpTable entries exactly - that was checked
		// by generating both mappings and diffing them - and do_ILLEGAL is the
		// default, so an unknown opcode still reports rather than silently
		// doing nothing.
		switch opcode {
		case POP_TOP:
			err = do_POP_TOP(&vm, arg)
		case ROT_TWO:
			err = do_ROT_TWO(&vm, arg)
		case ROT_THREE:
			err = do_ROT_THREE(&vm, arg)
		case DUP_TOP:
			err = do_DUP_TOP(&vm, arg)
		case DUP_TOP_TWO:
			err = do_DUP_TOP_TWO(&vm, arg)
		case NOP:
			err = do_NOP(&vm, arg)
		case UNARY_POSITIVE:
			err = do_UNARY_POSITIVE(&vm, arg)
		case UNARY_NEGATIVE:
			err = do_UNARY_NEGATIVE(&vm, arg)
		case UNARY_NOT:
			err = do_UNARY_NOT(&vm, arg)
		case UNARY_INVERT:
			err = do_UNARY_INVERT(&vm, arg)
		case BINARY_POWER:
			err = do_BINARY_POWER(&vm, arg)
		case BINARY_MULTIPLY:
			err = do_BINARY_MULTIPLY(&vm, arg)
		case BINARY_MODULO:
			err = do_BINARY_MODULO(&vm, arg)
		case BINARY_ADD:
			err = do_BINARY_ADD(&vm, arg)
		case BINARY_SUBTRACT:
			err = do_BINARY_SUBTRACT(&vm, arg)
		case BINARY_SUBSCR:
			err = do_BINARY_SUBSCR(&vm, arg)
		case BINARY_FLOOR_DIVIDE:
			err = do_BINARY_FLOOR_DIVIDE(&vm, arg)
		case BINARY_TRUE_DIVIDE:
			err = do_BINARY_TRUE_DIVIDE(&vm, arg)
		case INPLACE_FLOOR_DIVIDE:
			err = do_INPLACE_FLOOR_DIVIDE(&vm, arg)
		case INPLACE_TRUE_DIVIDE:
			err = do_INPLACE_TRUE_DIVIDE(&vm, arg)
		case STORE_MAP:
			err = do_STORE_MAP(&vm, arg)
		case INPLACE_ADD:
			err = do_INPLACE_ADD(&vm, arg)
		case INPLACE_SUBTRACT:
			err = do_INPLACE_SUBTRACT(&vm, arg)
		case INPLACE_MULTIPLY:
			err = do_INPLACE_MULTIPLY(&vm, arg)
		case INPLACE_MODULO:
			err = do_INPLACE_MODULO(&vm, arg)
		case STORE_SUBSCR:
			err = do_STORE_SUBSCR(&vm, arg)
		case DELETE_SUBSCR:
			err = do_DELETE_SUBSCR(&vm, arg)
		case BINARY_LSHIFT:
			err = do_BINARY_LSHIFT(&vm, arg)
		case BINARY_RSHIFT:
			err = do_BINARY_RSHIFT(&vm, arg)
		case BINARY_AND:
			err = do_BINARY_AND(&vm, arg)
		case BINARY_XOR:
			err = do_BINARY_XOR(&vm, arg)
		case BINARY_OR:
			err = do_BINARY_OR(&vm, arg)
		case INPLACE_POWER:
			err = do_INPLACE_POWER(&vm, arg)
		case GET_ITER:
			err = do_GET_ITER(&vm, arg)
		case PRINT_EXPR:
			err = do_PRINT_EXPR(&vm, arg)
		case LOAD_BUILD_CLASS:
			err = do_LOAD_BUILD_CLASS(&vm, arg)
		case YIELD_FROM:
			err = do_YIELD_FROM(&vm, arg)
		case INPLACE_LSHIFT:
			err = do_INPLACE_LSHIFT(&vm, arg)
		case INPLACE_RSHIFT:
			err = do_INPLACE_RSHIFT(&vm, arg)
		case INPLACE_AND:
			err = do_INPLACE_AND(&vm, arg)
		case INPLACE_XOR:
			err = do_INPLACE_XOR(&vm, arg)
		case INPLACE_OR:
			err = do_INPLACE_OR(&vm, arg)
		case BREAK_LOOP:
			err = do_BREAK_LOOP(&vm, arg)
		case WITH_CLEANUP:
			err = do_WITH_CLEANUP(&vm, arg)
		case RETURN_VALUE:
			err = do_RETURN_VALUE(&vm, arg)
		case IMPORT_STAR:
			err = do_IMPORT_STAR(&vm, arg)
		case YIELD_VALUE:
			err = do_YIELD_VALUE(&vm, arg)
		case POP_BLOCK:
			err = do_POP_BLOCK(&vm, arg)
		case END_FINALLY:
			err = do_END_FINALLY(&vm, arg)
		case POP_EXCEPT:
			err = do_POP_EXCEPT(&vm, arg)
		case STORE_NAME:
			err = do_STORE_NAME(&vm, arg)
		case DELETE_NAME:
			err = do_DELETE_NAME(&vm, arg)
		case UNPACK_SEQUENCE:
			err = do_UNPACK_SEQUENCE(&vm, arg)
		case FOR_ITER:
			err = do_FOR_ITER(&vm, arg)
		case UNPACK_EX:
			err = do_UNPACK_EX(&vm, arg)
		case STORE_ATTR:
			err = do_STORE_ATTR(&vm, arg)
		case DELETE_ATTR:
			err = do_DELETE_ATTR(&vm, arg)
		case STORE_GLOBAL:
			err = do_STORE_GLOBAL(&vm, arg)
		case DELETE_GLOBAL:
			err = do_DELETE_GLOBAL(&vm, arg)
		case LOAD_CONST:
			err = do_LOAD_CONST(&vm, arg)
		case LOAD_NAME:
			err = do_LOAD_NAME(&vm, arg)
		case BUILD_TUPLE:
			err = do_BUILD_TUPLE(&vm, arg)
		case BUILD_LIST:
			err = do_BUILD_LIST(&vm, arg)
		case BUILD_SET:
			err = do_BUILD_SET(&vm, arg)
		case BUILD_MAP:
			err = do_BUILD_MAP(&vm, arg)
		case LOAD_ATTR:
			err = do_LOAD_ATTR(&vm, arg)
		case COMPARE_OP:
			err = do_COMPARE_OP(&vm, arg)
		case IMPORT_NAME:
			err = do_IMPORT_NAME(&vm, arg)
		case IMPORT_FROM:
			err = do_IMPORT_FROM(&vm, arg)
		case JUMP_FORWARD:
			err = do_JUMP_FORWARD(&vm, arg)
		case JUMP_IF_FALSE_OR_POP:
			err = do_JUMP_IF_FALSE_OR_POP(&vm, arg)
		case JUMP_IF_TRUE_OR_POP:
			err = do_JUMP_IF_TRUE_OR_POP(&vm, arg)
		case JUMP_ABSOLUTE:
			err = do_JUMP_ABSOLUTE(&vm, arg)
		case POP_JUMP_IF_FALSE:
			err = do_POP_JUMP_IF_FALSE(&vm, arg)
		case POP_JUMP_IF_TRUE:
			err = do_POP_JUMP_IF_TRUE(&vm, arg)
		case LOAD_GLOBAL:
			err = do_LOAD_GLOBAL(&vm, arg)
		case CONTINUE_LOOP:
			err = do_CONTINUE_LOOP(&vm, arg)
		case SETUP_LOOP:
			err = do_SETUP_LOOP(&vm, arg)
		case SETUP_EXCEPT:
			err = do_SETUP_EXCEPT(&vm, arg)
		case SETUP_FINALLY:
			err = do_SETUP_FINALLY(&vm, arg)
		case LOAD_FAST:
			err = do_LOAD_FAST(&vm, arg)
		case STORE_FAST:
			err = do_STORE_FAST(&vm, arg)
		case DELETE_FAST:
			err = do_DELETE_FAST(&vm, arg)
		case RAISE_VARARGS:
			err = do_RAISE_VARARGS(&vm, arg)
		case CALL_FUNCTION:
			err = do_CALL_FUNCTION(&vm, arg)
		case MAKE_FUNCTION:
			err = do_MAKE_FUNCTION(&vm, arg)
		case BUILD_SLICE:
			err = do_BUILD_SLICE(&vm, arg)
		case MAKE_CLOSURE:
			err = do_MAKE_CLOSURE(&vm, arg)
		case LOAD_CLOSURE:
			err = do_LOAD_CLOSURE(&vm, arg)
		case LOAD_DEREF:
			err = do_LOAD_DEREF(&vm, arg)
		case STORE_DEREF:
			err = do_STORE_DEREF(&vm, arg)
		case DELETE_DEREF:
			err = do_DELETE_DEREF(&vm, arg)
		case CALL_FUNCTION_VAR:
			err = do_CALL_FUNCTION_VAR(&vm, arg)
		case CALL_FUNCTION_KW:
			err = do_CALL_FUNCTION_KW(&vm, arg)
		case CALL_FUNCTION_VAR_KW:
			err = do_CALL_FUNCTION_VAR_KW(&vm, arg)
		case CALL_FUNCTION_EX:
			err = do_CALL_FUNCTION_EX(&vm, arg)
		case SETUP_WITH:
			err = do_SETUP_WITH(&vm, arg)
		case EXTENDED_ARG:
			err = do_EXTENDED_ARG(&vm, arg)
		case LIST_APPEND:
			err = do_LIST_APPEND(&vm, arg)
		case LIST_EXTEND:
			err = do_LIST_EXTEND(&vm, arg)
		case TUPLE_EXTEND:
			err = do_TUPLE_EXTEND(&vm, arg)
		case DICT_UPDATE:
			err = do_DICT_UPDATE(&vm, arg)
		case LIST_EXTEND_MAPPING:
			err = do_LIST_EXTEND_MAPPING(&vm, arg)
		case SET_ADD:
			err = do_SET_ADD(&vm, arg)
		case MAP_ADD:
			err = do_MAP_ADD(&vm, arg)
		case LOAD_CLASSDEREF:
			err = do_LOAD_CLASSDEREF(&vm, arg)
		default:
			err = do_ILLEGAL(&vm, arg)
		}
		if err != nil {
			// FIXME shouldn't be doing this - just use err?
			if errExcInfo, ok := err.(py.ExceptionInfo); ok {
				vm.curexc = errExcInfo
				vm.AddTraceback(&vm.curexc)
				vm.why = whyException
			} else {
				vm.SetException(py.MakeException(err))
			}
		}
		if debugging {
			debugf("* Stack = %#v\n", frame.Stack)
			// if len(frame.Stack) > 0 {
			// 	if t, ok := vm.TOP().(*py.Type); ok {
			// 		if debugging { debugf(" * TOP = %#v\n", t) }
			// 	}
			// }
		}
		if vm.why == whyYield {
			goto fast_yield
		}

	unwind:
		// Something exceptional has happened - unwind the block stack
		// and find out what
		for vm.why != whyNot && frame.Block != nil {
			// Peek at the current block.
			b := frame.Block
			if debugging {
				debugf("*** Unwinding %#v vm %#v\n", b, vm)
			}

			if b.Type == py.TryBlockSetupLoop && vm.why == whyContinue {
				vm.why = whyNot
				dest := vm.retval.(py.Int)
				frame.Lasti = int32(dest)
				break
			}

			// Now we have to pop the block.
			frame.PopBlock()

			if b.Type == py.TryBlockExceptHandler {
				if debugging {
					debugf("*** EXCEPT_HANDLER\n")
				}
				vm.UnwindExceptHandler(frame, b)
				continue
			}
			vm.UnwindBlock(frame, b)
			if b.Type == py.TryBlockSetupLoop && vm.why == whyBreak {
				if debugging {
					debugf("*** Loop\n")
				}
				vm.why = whyNot
				frame.Lasti = b.Handler
				break
			}
			if vm.why == whyException && (b.Type == py.TryBlockSetupExcept || b.Type == py.TryBlockSetupFinally) {
				if debugging {
					debugf("*** Exception\n")
				}
				handler := b.Handler
				// This invalidates b
				frame.PushBlock(py.TryBlockExceptHandler, -1, vm.STACK_LEVEL())
				vm.PUSH(vm.exc.Traceback)
				vm.PUSH(vm.exc.Value)
				if vm.exc.Type == nil {
					vm.PUSH(py.None)
				} else {
					vm.PUSH(vm.exc.Type) // can be nil
				}
				// FIXME PyErr_Fetch(&exc, &val, &tb)
				exc := vm.curexc.Type
				val := vm.curexc.Value
				tb := vm.curexc.Traceback
				vm.curexc.Type = nil
				vm.curexc.Value = nil
				vm.curexc.Traceback = nil
				// Make the raw exception data
				// available to the handler,
				// so a program can emulate the
				// Python main loop.
				// FIXME PyErr_NormalizeException(exc, &val, &tb)
				// FIXME PyException_SetTraceback(val, tb)
				// Entering a handler SAVES the state of the one that was
				// active outside it, so leaving this one can put it back.
				vm.excStack = append(vm.excStack, vm.exc)
				vm.exc.Type = exc
				vm.exc.Value = val
				vm.exc.Traceback = tb
				py.SetCurrentExceptionFromValue(val)
				vm.PUSH(tb)
				vm.PUSH(val)
				if exc == nil {
					vm.PUSH(py.None)
				} else {
					vm.PUSH(exc)
				}
				vm.why = whyNot
				frame.Lasti = handler
				break
			}
			if b.Type == py.TryBlockSetupFinally {
				if vm.why == whyReturn || vm.why == whyContinue {
					vm.PUSH(vm.retval)
				}
				vm.PUSH(py.Int(vm.why))
				vm.why = whyNot
				frame.Lasti = b.Handler
				break
			}
		}
	}
	if debugging {
		debugf("EXIT with %v\n", vm.why)
	}
	if vm.why != whyReturn {
		vm.retval = nil
	}
	if vm.retval == nil && !vm.curexc.IsSet() {
		panic("vm: no result or exception")
	}
	if vm.retval != nil && vm.curexc.IsSet() {
		panic("vm: result and exception")
	}

fast_yield:
	// FIXME
	// if (co->co_flags & CO_GENERATOR) {
	//     /* The purpose of this block is to put aside the generator's exception
	//        state and restore that of the calling frame. If the current
	//        exception state is from the caller, we clear the exception values
	//        on the generator frame, so they are not swapped back in latter. The
	//        origin of the current exception state is determined by checking for
	//        except handler blocks, which we must be in iff a new exception
	//        state came into existence in this frame. (An uncaught exception
	//        would have why == WHY_EXCEPTION, and we wouldn't be here). */
	//     int i;
	//     for (i = 0; i < f->f_iblock; i++)
	//         if (f->f_blockstack[i].b_type == EXCEPT_HANDLER)
	//             break;
	//     if (i == f->f_iblock)
	//         /* We did not create this exception. */
	//         restore_and_clear_exc_state(tstate, f);
	//     else
	//         swap_exc_state(tstate, f);
	// }

	if vm.curexc.IsSet() {
		return vm.retval, vm.curexc
	}
	return vm.retval, nil
}

// Chooses trueString if flag is true, falseString otherwise
func chooseString(flag bool, trueString, falseString string) string {
	if flag {
		return trueString
	}
	return falseString
}

// Returns a plural suffix "s" or ""
func pluralSuffix(plural bool) string {
	if plural {
		return "s"
	}
	return ""
}

// Format and error for missing arguments
func formatMissing(kind string, co *py.Code, names []string) error {
	var name_str string
	/* Deal with the joys of natural language. */
	switch len(names) {
	case 0:
		panic("vm: format_missing: no names")
	case 1:
		name_str = "'" + names[0] + "'"
	case 2:
		name_str = fmt.Sprintf("'%s' and '%s'", names[len(names)-2], names[len(names)-1])
	default:
		tail := fmt.Sprintf(", '%s', and '%s'", names[len(names)-2], names[len(names)-1])
		// Stitch everything up into a nice comma-separated list.
		name_str = "'" + strings.Join(names[:len(names)-2], "', '") + "'" + tail
	}
	return py.ExceptionNewf(py.TypeError,
		"%s() missing %d required %s argument%s: %s",
		co.Name,
		len(names),
		kind,
		pluralSuffix(len(names) != 1),
		name_str)
}

// Format an error for missing arguments
func missingArguments(co *py.Code, missing, defcount int, fastlocals []py.Object) error {
	positional := defcount != -1
	kind := chooseString(positional, "positional", "keyword-only")
	var missing_names []string

	/* Compute the names of the arguments that are missing. */
	var start, end int
	if positional {
		start = 0
		end = int(co.Argcount) - defcount
	} else {
		start = int(co.Argcount)
		end = start + int(co.Kwonlyargcount)
	}
	for i := start; i < end; i++ {
		if fastlocals[i] == nil {
			name := co.Varnames[i]
			missing_names = append(missing_names, name)
		}
	}
	return formatMissing(kind, co, missing_names)
}

// Format an error for too many positional arguments
func tooManyPositional(co *py.Code, given, defcount int, fastlocals []py.Object) error {
	kwonly_given := 0

	//assert((co.Flags & CO_VARARGS) == 0)
	/* Count missing keyword-only args. */
	for i := co.Argcount; i < co.Argcount+co.Kwonlyargcount; i++ {
		if fastlocals[i] != nil {
			kwonly_given++
		}
	}
	var plural bool
	var sig string
	var kwonly_sig string
	if defcount != 0 {
		atleast := int(co.Argcount) - defcount
		plural = true
		sig = fmt.Sprintf("from %d to %d", atleast, co.Argcount)
	} else {
		plural = co.Argcount != 1
		sig = fmt.Sprintf("%d", co.Argcount)
	}
	if kwonly_given != 0 {
		kwonly_sig = fmt.Sprintf(" positional argument%s (and %d keyword-only argument%s)", pluralSuffix(given != 1), kwonly_given, pluralSuffix(kwonly_given != 1))
	}
	return py.ExceptionNewf(py.TypeError,
		"%s() takes %s positional argument%s but %d%s %s given",
		co.Name,
		sig,
		pluralSuffix(plural),
		given,
		kwonly_sig,
		chooseString(given == 1 && kwonly_given == 0, "was", "were"))
}

// EvalCode runs a new virtual machine on a Code object.
//
// # Any parameters are expected to have been decoded into locals
//
// Returns an Object and an error.  The error will be a py.ExceptionInfo
//
// This is the equivalent of PyEval_EvalCode with closure support
func EvalCode(ctx py.Context, co *py.Code, globals, locals py.StringDict, args []py.Object, kws py.StringDict, defs []py.Object, kwdefs py.StringDict, closure py.Tuple) (retval py.Object, err error) {
	total_args := int(co.Argcount + co.Kwonlyargcount)
	n := len(args)
	var kwdict py.StringDict

	if globals.IsNil() {
		return nil, py.ExceptionNewf(py.SystemError, "PyEval_EvalCodeEx: nil globals")
	}

	//assert(tstate != nil)
	//assert(globals != nil)
	// f = PyFrame_New(tstate, co, globals, locals)
	f := py.NewFrame(ctx, globals, locals, co, closure) // FIXME extra closure parameter?

	fastlocals := f.Localsplus
	freevars := f.CellAndFreeVars

	/* Parse arguments. */
	if co.Flags&py.CO_VARKEYWORDS != 0 {
		kwdict = py.NewStringDict()
		i := total_args
		if co.Flags&py.CO_VARARGS != 0 {
			i++
		}
		fastlocals[i] = kwdict
	}
	if len(args) > int(co.Argcount) {
		n = int(co.Argcount)
	}
	for i := 0; i < n; i++ {
		fastlocals[i] = args[i]
	}
	if co.Flags&py.CO_VARARGS != 0 {
		u := make(py.Tuple, len(args)-n)
		fastlocals[total_args] = u
		for i := n; i < len(args); i++ {
			u[i-n] = args[i]
		}
	}
	for _, __e := range kws.Items() {
		keyword := __e.Key
		value := __e.Value

		j := 0
		// Positional-only arguments are matched by position and must not be
		// passed by keyword (PEP 570): skip them when looking for a name.
		for ; j < total_args; j++ {
			if co.Posonlyargcount > 0 && j < int(co.Posonlyargcount) {
				continue
			}
			if co.Varnames[j] == keyword {
				goto kw_found
			}
		}
		if j >= total_args && kwdict.IsNil() {
			// A name that IS a positional-only parameter gets CPython's own
			// message, which says the argument exists but may not be passed by
			// keyword - naming all such parameters, not just this one:
			//   "divide() got some positional-only arguments passed as keyword
			//    arguments: 'a, b'"
			// "unexpected keyword argument" is misleading there, because the
			// name is not unexpected at all.
			if po := positionalOnlyPassedByKeyword(co, kws); po != "" {
				return nil, py.ExceptionNewf(py.TypeError,
					"%s() got some positional-only arguments passed as keyword arguments: '%s'",
					co.Name, po)
			}
			return nil, py.ExceptionNewf(py.TypeError, "%s() got an unexpected keyword argument '%s'", co.Name, keyword)
		}
		kwdict.Set(keyword, value)
		continue
	kw_found:
		if fastlocals[j] != nil {
			return nil, py.ExceptionNewf(py.TypeError, "%s() got multiple values for argument '%s'", co.Name, keyword)
		}
		fastlocals[j] = value
	}
	if len(args) > int(co.Argcount) && co.Flags&py.CO_VARARGS == 0 {
		return nil, tooManyPositional(co, len(args), len(defs), fastlocals)
	}
	if len(args) < int(co.Argcount) {
		m := int(co.Argcount) - len(defs)
		missing := 0
		for i := len(args); i < m; i++ {
			if fastlocals[i] == nil {
				missing++
			}
		}
		if missing != 0 {
			return nil, missingArguments(co, missing, len(defs), fastlocals)
		}
		i := 0
		if n > m {
			i = n - m
		}
		for ; i < len(defs); i++ {
			if fastlocals[m+i] == nil {
				fastlocals[m+i] = defs[i]
			}
		}
	}
	if co.Kwonlyargcount > 0 {
		missing := 0
		for i := int(co.Argcount); i < total_args; i++ {
			if fastlocals[i] != nil {
				continue
			}
			name := co.Varnames[i]
			if !kwdefs.IsNil() {
				if def, ok := kwdefs.Get(name); ok {
					fastlocals[i] = def
					continue
				}
			}
			missing++
		}
		if missing != 0 {
			return nil, missingArguments(co, missing, -1, fastlocals)
		}
	}

	/* Allocate and initialize storage for cell vars, and copy free
	   vars into frame. */
	for i := 0; i < len(co.Cellvars); i++ {
		/* Possibly account for the cell variable being an argument. */
		var c *py.Cell
		if co.Cell2arg != nil && co.Cell2arg[i] != py.CO_CELL_NOT_AN_ARG {
			c = py.NewCell(fastlocals[co.Cell2arg[i]])
			/* Clear the local copy. */
			fastlocals[co.Cell2arg[i]] = nil
		} else {
			c = py.NewCell(nil)
		}
		fastlocals[int(co.Nlocals)+i] = c
		//freevars[i] = c
	}
	for i := 0; i < len(co.Freevars); i++ {
		freevars[len(co.Cellvars)+i] = closure[i]
	}

	if co.Flags&py.CO_GENERATOR != 0 {
		/* Create a new generator that owns the ready to run frame
		 * and return that as the value. */
		return py.NewGenerator(f), nil
	}

	return RunFrame(f)
}

// Write the py global to avoid circular import
func init() {
	py.VmEvalCode = EvalCode
	py.VmRunFrame = RunFrame
}

// init wires the frame accessor for the no-argument form of super(), which
// needs the frame that is executing to read __class__ and self.
func init() {
	py.SetCurrentFrame(func() *py.Frame {
		return gCurrentFrame.get()
	})
	py.CurrentGoroutineID = goroutineID
}

// gCurrentFrame is the frame THIS goroutine is executing, recorded by RunFrame
// so that py code can read it without importing this package.
//
// It is goroutine-local rather than a plain variable so that two interpreter
// contexts in two goroutines each have their own.  Go has no goroutine-local
// storage, so the value is kept in a map keyed by the goroutine's id, which
// is obtained unsupported-ly but is the only handle available; the map is
// mutex-guarded, and an entry is removed when the goroutine's outermost frame
// exits so the map cannot grow without bound.
var gCurrentFrame = &frameSlot{frames: map[uint64]*py.Frame{}}

type frameSlot struct {
	mu     sync.Mutex
	frames map[uint64]*py.Frame
}

func (f *frameSlot) get() *py.Frame {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.frames[goroutineID()]
}

func (f *frameSlot) set(frame *py.Frame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := goroutineID()
	if frame == nil {
		delete(f.frames, id)
		return
	}
	f.frames[id] = frame
}

// goroutineID returns the calling goroutine's id.
//
// Go deliberately provides no supported way to get this: goroutine-local
// storage is discouraged because it does not compose with the scheduler, and
// the runtime does not expose the id.  It is needed here because "the frame
// currently executing" is genuinely per-goroutine state with no context to
// hang it off - a builtin method receives only its arguments, so super() has
// nothing else to ask.
//
// The implementation reads the id from the goroutine's own stack header,
// which the runtime writes as "goroutine N [status]:" at a documented
// location.  This is a well-known trick rather than a contract, so it is
// confined to this one function: if a future Go release changes the header
// the parse below returns 0 for every goroutine, which degrades to the old
// shared-variable behaviour (correct in the single-threaded case, racy in the
// concurrent one) rather than crashing.
func goroutineID() uint64 {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	// The header is "goroutine 123 [running]:"; parse the digits after the
	// space.
	s := buf[:n]
	const prefix = "goroutine "
	if !bytes.HasPrefix(s, []byte(prefix)) {
		return 0
	}
	s = s[len(prefix):]
	var id uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}
