package vm

// except* (PEP 654).
//
// try/except* compiles to a handler that leaves [orig, raised, rest] on the
// value stack (under the saved exception state, like any except handler):
//
//	orig    the exception that was caught
//	raised  a list of (exception, reraised) pairs, one per clause body that
//	        ended by raising
//	rest    what no except* clause has matched yet, or None
//
// EG_PREP builds it, EG_MATCH splits `rest` for one clause, EG_RECORD notes
// the exception a clause body raised and EG_RERAISE combines everything into
// the exception to raise once all clauses have run (or None).

import (
	"github.com/vishnukv64/gpython/py"
)

func (vm *Vm) baseGroupType() *py.Type {
	t, _ := vm.frame.Builtins.Get("BaseExceptionGroup")
	bt, _ := t.(*py.Type)
	return bt
}

func (vm *Vm) isGroup(o py.Object) bool {
	return o.Type().IsSubtype(vm.baseGroupType())
}

// do_EG_PREP: [tb, val, exc] -> [val, raised, val]
func do_EG_PREP(vm *Vm, arg int32) error {
	vm.POP()
	val := vm.POP()
	vm.POP()
	vm.PUSH(val)
	vm.PUSH(py.NewList())
	vm.PUSH(val)
	return nil
}

// do_EG_MATCH: [.., rest, E] -> [.., rest', match]
func do_EG_MATCH(vm *Vm, arg int32) error {
	spec := vm.POP()
	rest := vm.TOP()
	check := func(c py.Object) error {
		if !py.ExceptionClassCheck(c) {
			return py.ExceptionNewf(py.TypeError, "catching classes that do not inherit from BaseException is not allowed")
		}
		if t, ok := c.(*py.Type); ok && t.IsSubtype(vm.baseGroupType()) {
			return py.ExceptionNewf(py.TypeError, "catching ExceptionGroup with except* is not allowed. Use except instead.")
		}
		return nil
	}
	if tup, ok := spec.(py.Tuple); ok {
		for _, c := range tup {
			if err := check(c); err != nil {
				return err
			}
		}
	} else if err := check(spec); err != nil {
		return err
	}
	var match py.Object = py.None
	switch {
	case rest == py.None:
	case !vm.isGroup(rest):
		if py.ExceptionGivenMatches(rest, spec) {
			g, err := py.Call(vm.baseGroupType(), py.Tuple{py.String(""), py.Tuple{rest}}, py.StringDict{})
			if err != nil {
				return err
			}
			match, rest = g, py.None
		}
	default:
		split, err := py.GetAttrString(rest, "split")
		if err != nil {
			return err
		}
		res, err := py.Call(split, py.Tuple{spec}, py.StringDict{})
		if err != nil {
			return err
		}
		pair := res.(py.Tuple)
		match, rest = pair[0], pair[1]
	}
	vm.SET_TOP(rest)
	vm.PUSH(match)
	if e, ok := match.(*py.Exception); ok {
		tb, _ := e.Traceback.(*py.Traceback)
		vm.exc = py.ExceptionInfo{Type: e.Type(), Value: e, Traceback: tb}
		vm.frame.Handling = e
		py.SetCurrentExceptionFromValue(e)
	}
	return nil
}

// do_EG_RECORD: [orig, raised, rest, saved*3, tb, val, exc] -> [.., saved*3]
// It runs in the handler that wraps a clause body, after that body raised.
func do_EG_RECORD(vm *Vm, arg int32) error {
	vm.POP()
	val := vm.POP()
	vm.POP()
	lvl := vm.frame.Block.Level
	raised := vm.frame.Stack[lvl-2].(*py.List)
	reraised := vm.bareRaised == val && vm.bareRaisedFrame == vm.frame
	vm.bareRaised, vm.bareRaisedFrame = nil, nil
	raised.Append(py.Tuple{val, py.NewBool(reraised)})
	return nil
}

func (vm *Vm) leafIDs(o py.Object, into map[py.Object]bool) error {
	if !vm.isGroup(o) {
		into[o] = true
		return nil
	}
	excs, err := py.GetAttrString(o, "exceptions")
	if err != nil {
		return err
	}
	it, err := py.Iter(excs)
	if err != nil {
		return err
	}
	for {
		e, err := py.Next(it)
		if err != nil {
			if py.IsException(py.StopIteration, err) {
				return nil
			}
			return err
		}
		if err := vm.leafIDs(e, into); err != nil {
			return err
		}
	}
}

// do_EG_RERAISE: [orig, raised, rest] -> [] and either raises or continues.
func do_EG_RERAISE(vm *Vm, arg int32) error {
	rest := vm.POP()
	raised := vm.POP().(*py.List)
	orig := vm.POP()
	if !vm.isGroup(orig) {
		if len(raised.Items) > 0 {
			return raiseValue(raised.Items[0].(py.Tuple)[0])
		}
		if rest != py.None {
			return raiseValue(rest)
		}
		return nil
	}
	keepIDs := map[py.Object]bool{}
	if rest != py.None {
		if err := vm.leafIDs(rest, keepIDs); err != nil {
			return err
		}
	}
	var items []py.Object
	for _, it := range raised.Items {
		pair := it.(py.Tuple)
		if pair[1] == py.True {
			if err := vm.leafIDs(pair[0], keepIDs); err != nil {
				return err
			}
		} else {
			items = append(items, pair[0])
		}
	}
	var keep py.Object = py.None
	if len(keepIDs) > 0 {
		split, err := py.GetAttrString(orig, "subgroup")
		if err != nil {
			return err
		}
		pred := py.MustNewMethod("keep", func(self, leaf py.Object) (py.Object, error) {
			return py.NewBool(keepIDs[leaf]), nil
		}, 0, "")
		if keep, err = py.Call(split, py.Tuple{pred}, py.StringDict{}); err != nil {
			return err
		}
	}
	if keep != py.None {
		items = append(items, keep)
	}
	switch len(items) {
	case 0:
		return nil
	case 1:
		return raiseValue(items[0])
	}
	g, err := py.Call(vm.baseGroupType(), py.Tuple{py.String(""), py.NewListFromItems(items)}, py.StringDict{})
	if err != nil {
		return err
	}
	return raiseValue(g)
}

// raiseValue raises v without chaining a __context__: the combined exception
// leaves the handler, it is not raised inside it.
func raiseValue(v py.Object) error {
	e := py.MakeException(v)
	tb, _ := e.Traceback.(*py.Traceback)
	return py.ExceptionInfo{Type: e.Type(), Value: e, Traceback: tb}
}
