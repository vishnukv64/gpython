// Fast paths for exact int/float operands.  Each returns ok=false whenever the
// generic protocol could behave differently (overflow, zero divisor, mixed or
// subclassed operands), so semantics are unchanged.

package vm

import (
	"math/bits"

	"github.com/vishnukv64/gpython/py"
)

const (
	fastAdd = iota
	fastSub
	fastMul
	fastMod
)

func fastArith(op int, a, b py.Object) (py.Object, bool) {
	switch x := a.(type) {
	case py.Int:
		y, ok := b.(py.Int)
		if !ok {
			return nil, false
		}
		switch op {
		case fastAdd:
			s := x + y
			if (x >= 0) == (y >= 0) && (s >= 0) != (x >= 0) {
				return nil, false
			}
			return py.NewInt(int64(s)), true
		case fastSub:
			d := x - y
			if (x >= 0) != (y >= 0) && (d >= 0) != (x >= 0) {
				return nil, false
			}
			return py.NewInt(int64(d)), true
		case fastMul:
			if x > -(1<<31) && x < 1<<31 && y > -(1<<31) && y < 1<<31 {
				return py.NewInt(int64(x * y)), true
			}
			hi, lo := bits.Mul64(uint64(x), uint64(y))
			// Only the non-negative, in-range case is cheap to verify.
			if x >= 0 && y >= 0 && hi == 0 && lo < 1<<63 {
				return py.NewInt(int64(lo)), true
			}
		case fastMod:
			if y > 0 && x >= 0 {
				return py.NewInt(int64(x % y)), true
			}
		}
	case py.Float:
		y, ok := b.(py.Float)
		if !ok {
			return nil, false
		}
		switch op {
		case fastAdd:
			return x + y, true
		case fastSub:
			return x - y, true
		case fastMul:
			return x * y, true
		}
	}
	return nil, false
}

// fastCompare handles exact int/int and float/float rich comparisons.
func fastCompare(op int32, a, b py.Object) (py.Object, bool) {
	switch x := a.(type) {
	case py.Int:
		y, ok := b.(py.Int)
		if !ok {
			return nil, false
		}
		switch op {
		case PyCmp_LT:
			return py.NewBool(x < y), true
		case PyCmp_LE:
			return py.NewBool(x <= y), true
		case PyCmp_EQ:
			return py.NewBool(x == y), true
		case PyCmp_NE:
			return py.NewBool(x != y), true
		case PyCmp_GT:
			return py.NewBool(x > y), true
		case PyCmp_GE:
			return py.NewBool(x >= y), true
		}
	case py.Float:
		y, ok := b.(py.Float)
		if !ok {
			return nil, false
		}
		switch op {
		case PyCmp_LT:
			return py.NewBool(x < y), true
		case PyCmp_LE:
			return py.NewBool(x <= y), true
		case PyCmp_EQ:
			return py.NewBool(x == y), true
		case PyCmp_NE:
			return py.NewBool(x != y), true
		case PyCmp_GT:
			return py.NewBool(x > y), true
		case PyCmp_GE:
			return py.NewBool(x >= y), true
		}
	}
	return nil, false
}

// binArith is the shared body of the BINARY_* and INPLACE_* arithmetic ops.
func (vm *Vm) binArith(op int, slow func(a, b py.Object) (py.Object, error)) error {
	st := vm.frame.Stack
	n := len(st)
	a, b := st[n-2], st[n-1]
	if r, ok := fastArith(op, a, b); ok {
		st[n-2] = r
		vm.frame.Stack = st[:n-1]
		return nil
	}
	vm.frame.Stack = st[:n-1]
	return vm.setTopAndCheckErr(slow(a, b))
}

// truth reports the truth of exact bool/int/None without calling __bool__.
func truth(o py.Object) (val, ok bool) {
	switch x := o.(type) {
	case py.Bool:
		return bool(x), true
	case py.Int:
		return x != 0, true
	case py.NoneType:
		return false, true
	}
	return false, false
}
