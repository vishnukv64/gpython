// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package futures

import (
	"time"

	"github.com/vishnukv64/gpython/py"
)

// restArgs copies the arguments that follow fn into a fresh tuple, and copies
// the keyword mapping too.
//
// The caller's tuple is the VM's own argument slice and is reused for the next
// call, so keeping a subslice of it made every queued task see the LAST call's
// arguments: "[pool.submit(f, i) for i in range(4)]" ran f(3) four times.  The
// values are read here, on the calling goroutine, before control returns to
// the VM.
func restArgs(args py.Tuple) py.Tuple {
	out := make(py.Tuple, len(args))
	copy(out, args)
	return out
}

// ---------------------------------------------------------------------------
// wait

// wait blocks until the futures satisfy return_when, and answers the
// (done, not_done) pair of sets.
func waitFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fsObj py.Object
	timeout := py.Object(py.None)
	returnWhen := py.Object(py.String(allCompleted))
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:wait",
		[]string{"fs", "timeout", "return_when"}, &fsObj, &timeout, &returnWhen); err != nil {
		return nil, err
	}
	futures, err := futureList(fsObj)
	if err != nil {
		return nil, err
	}
	when, err := py.StrAsString(returnWhen)
	if err != nil {
		return nil, err
	}
	// CPython rejects a duplicate future, because (done, not_done) must
	// partition the input.
	if err := rejectDuplicates(futures); err != nil {
		return nil, err
	}

	var deadline time.Time
	if timeout != nil && timeout != py.None {
		secs, err := py.FloatAsFloat64(timeout)
		if err != nil {
			return nil, err
		}
		deadline = time.Now().Add(time.Duration(secs * float64(time.Second)))
	}

	done, notDone := partition(futures)
	for {
		if satisfied(done, notDone, futures, when) {
			return doneNotDone(done, notDone), nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return doneNotDone(done, notDone), nil
		}
		if len(notDone) == 0 {
			return doneNotDone(done, notDone), nil
		}
		// One millisecond is short enough that a timeout is honoured to
		// within a millisecond and long enough not to spin.
		time.Sleep(time.Millisecond)
		done, notDone = partition(futures)
	}
}

// satisfied reports whether return_when is met.  FIRST_EXCEPTION counts a
// future that was cancelled as an exception, matching CPython: a cancelled
// future is one the caller will not get a value from.
func satisfied(done, notDone []*Future, all []*Future, when string) bool {
	switch when {
	case firstCompleted:
		return len(done) != 0
	case firstException:
		if len(notDone) == 0 {
			return true
		}
		for _, f := range done {
			if f.raised() {
				return true
			}
		}
		return false
	default: // allCompleted
		return len(notDone) == 0
	}
}

// partition splits the futures into those that are finished and the rest.
func partition(futures []*Future) (done, notDone []*Future) {
	for _, f := range futures {
		if f.doneState() {
			done = append(done, f)
		} else {
			notDone = append(notDone, f)
		}
	}
	return done, notDone
}

// doneNotDone builds the (done, not_done) pair of sets that wait returns.
func doneNotDone(done, notDone []*Future) py.Object {
	asObjects := func(fs []*Future) *py.Set {
		items := make([]py.Object, len(fs))
		for i, f := range fs {
			items[i] = f
		}
		return py.NewSetFromItems(items)
	}
	return py.Tuple{asObjects(done), asObjects(notDone)}
}

// futureList accepts either an iterable of futures or a single future and
// answers the slice they form.
func futureList(o py.Object) ([]*Future, error) {
	// A bare Future, which CPython accepts.
	if f, ok := o.(*Future); ok {
		return []*Future{f}, nil
	}
	items, err := py.SequenceList(o)
	if err != nil {
		return nil, err
	}
	futures := make([]*Future, len(items.Items))
	for i, item := range items.Items {
		f, ok := item.(*Future)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError,
				"expected a Future, got %s", item.Type().Name)
		}
		futures[i] = f
	}
	return futures, nil
}

// rejectDuplicates reports the first future passed more than once, which
// CPython refuses because the done/not_done split would be ambiguous.
func rejectDuplicates(futures []*Future) error {
	for i, a := range futures {
		for j, b := range futures {
			if i != j && a == b {
				return py.ExceptionNewf(py.TypeError,
					"duplicate future argument: %v", a)
			}
		}
	}
	return nil
}

// raised reports whether the future finished with an exception or was
// cancelled.
func (f *Future) raised() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exc != nil || f.state == stateCancelled
}

// ---------------------------------------------------------------------------
// as_completed

// asCompleted is the as_completed() builtin.
func asCompleted(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fsObj py.Object
	timeout := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:as_completed",
		[]string{"fs", "timeout"}, &fsObj, &timeout); err != nil {
		return nil, err
	}
	futures, err := futureList(fsObj)
	if err != nil {
		return nil, err
	}
	if err := rejectDuplicates(futures); err != nil {
		return nil, err
	}
	return &asCompletedIterator{pending: futures, timeout: timeout, Dict: py.NewStringDict()}, nil
}

// ---------------------------------------------------------------------------
// registration

func init() {
	// ---- Future
	FutureType.Dict.Set("result", py.MustNewMethod("result", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		timeout := py.Object(py.None)
		if err := py.ParseTupleAndKeywords(args, kwargs, "|O:result",
			[]string{"timeout"}, &timeout); err != nil {
			return nil, err
		}
		res, exc, err := self.(*Future).awaitResult(timeout)
		if err != nil {
			return nil, err
		}
		if exc != nil {
			return nil, exc
		}
		return res, nil
	}, 0, "Return the result of the call which this future represents."))

	FutureType.Dict.Set("exception", py.MustNewMethod("exception", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		timeout := py.Object(py.None)
		if err := py.ParseTupleAndKeywords(args, kwargs, "|O:exception",
			[]string{"timeout"}, &timeout); err != nil {
			return nil, err
		}
		f := self.(*Future)
		res, exc, err := f.awaitResult(timeout)
		if err != nil {
			return nil, err
		}
		if exc != nil {
			return exc, nil
		}
		if f.cancelled() {
			return py.ExceptionNewf(CancelledError, ""), nil
		}
		_ = res
		return py.None, nil
	}, 0, "Return the exception raised by the call, or None if it completed without raising."))

	FutureType.Dict.Set("done", py.MustNewMethod("done", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(self.(*Future).doneState()), nil
	}, 0, "Return True if the future was cancelled or finished."))

	FutureType.Dict.Set("cancelled", py.MustNewMethod("cancelled", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(self.(*Future).cancelled()), nil
	}, 0, "Return True if the future was cancelled."))

	FutureType.Dict.Set("running", py.MustNewMethod("running", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(self.(*Future).running()), nil
	}, 0, "Return True while the call is being executed and cannot be cancelled."))

	FutureType.Dict.Set("cancel", py.MustNewMethod("cancel", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(self.(*Future).cancel()), nil
	}, 0, "Cancel the future if possible, returning True on success."))

	FutureType.Dict.Set("add_done_callback", py.MustNewMethod("add_done_callback", func(self py.Object, args py.Tuple) (py.Object, error) {
		var fn py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "add_done_callback", 1, 1, &fn); err != nil {
			return nil, err
		}
		if err := self.(*Future).addCallback(fn); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Attach a callable that will be called with this future when it completes."))

	FutureType.Dict.Set("set_result", py.MustNewMethod("set_result", func(self py.Object, args py.Tuple) (py.Object, error) {
		var value py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "set_result", 1, 1, &value); err != nil {
			return nil, err
		}
		if err := self.(*Future).setResult(value); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Mark the future as finished and record its result."))

	FutureType.Dict.Set("set_exception", py.MustNewMethod("set_exception", func(self py.Object, args py.Tuple) (py.Object, error) {
		var exc py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "set_exception", 1, 1, &exc); err != nil {
			return nil, err
		}
		if err := self.(*Future).setException(py.MakeException(exc)); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Mark the future as finished and record the exception it raised."))

	FutureType.Dict.Set("__hash__", py.MustNewMethod("__hash__", func(self py.Object, args py.Tuple) (py.Object, error) {
		// Futures hash by identity, so they can be members of the sets that
		// wait() returns.  Without this they were unhashable, and every future
		// was silently dropped from those sets.
		return py.Int(self.(*Future).id), nil
	}, 0, "Return hash(self)."))

	FutureType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		f := self.(*Future)
		f.mu.Lock()
		defer f.mu.Unlock()
		state := "pending"
		if f.state == stateFinished {
			state = "finished"
		}
		if f.state == stateRunning {
			state = "running"
		}
		if f.state == stateCancelled {
			state = "cancelled"
		}
		return py.String("<Future at 0x0 state=" + state + ">"), nil
	}, 0, "Return repr(self)."))

	// ---- Executor
	ExecutorType.Dict.Set("submit", py.MustNewMethod("submit", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if err := py.UnpackTuple(args, kwargs, "submit", 1, -1); err != nil {
			return nil, err
		}
		// The base class does not implement submit, as in CPython.
		_, ok := self.(*ThreadPoolExecutor)
		if !ok {
			return nil, py.ExceptionNewf(py.NotImplementedError,
				"Executor.submit is abstract; use ThreadPoolExecutor")
		}
		fn := args[0]
		future, err := self.(*ThreadPoolExecutor).submit(fn, restArgs(args[1:]), kwargs)
		if err != nil {
			return nil, err
		}
		return future, nil
	}, 0, "Submit fn(*args, **kwargs) and return a Future representing the call."))

	ExecutorType.Dict.Set("map", py.MustNewMethod("map", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) == 0 {
			return nil, py.ExceptionNewf(py.TypeError, "map() missing required argument 'fn'")
		}
		var timeout py.Object = py.None
		// map(fn, *iterables, timeout=None, chunksize=1).  chunksize exists for
		// the process pool and is accepted and ignored, as CPython's thread
		// pool does with it.
		if v, ok := kwargs.Get("timeout"); ok {
			timeout = v
		}
		iterables := []py.Object(args[1:])
		if len(iterables) == 0 {
			return nil, py.ExceptionNewf(py.TypeError, "map() requires at least one iterable")
		}
		e, ok := self.(*ThreadPoolExecutor)
		if !ok {
			return nil, py.ExceptionNewf(py.NotImplementedError,
				"Executor.map is abstract; use ThreadPoolExecutor")
		}
		return e.mapFn(args[0], iterables, timeout)
	}, 0, "Return an iterator equivalent to map(fn, iter), applying fn to the arguments drawn from the iterables."))

	ExecutorType.Dict.Set("shutdown", py.MustNewMethod("shutdown", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		wait := py.Object(py.True)
		if err := py.ParseTupleAndKeywords(args, kwargs, "|O:shutdown",
			[]string{"wait"}, &wait); err != nil {
			return nil, err
		}
		w, err := py.ObjectIsTrue(wait)
		if err != nil {
			return nil, err
		}
		if e, ok := self.(*ThreadPoolExecutor); ok {
			if err := e.shutdown(w); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}, 0, "Clean up the resources associated with this executor."))

	// The context-manager protocol, which CPython's Executor implements in
	// terms of shutdown.
	ExecutorType.Dict.Set("__enter__", py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Enter the context; return the executor."))
	ExecutorType.Dict.Set("__exit__", py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		if e, ok := self.(*ThreadPoolExecutor); ok {
			if err := e.shutdown(true); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}, 0, "Shut the executor down and wait for its work."))

	// ---- ThreadPoolExecutor
	ThreadPoolExecutorType.Dict.Set("submit", py.MustNewMethod("submit", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) == 0 {
			return nil, py.ExceptionNewf(py.TypeError, "submit() missing required argument 'fn'")
		}
		future, err := self.(*ThreadPoolExecutor).submit(args[0], restArgs(args[1:]), kwargs)
		if err != nil {
			return nil, err
		}
		return future, nil
	}, 0, "Submit fn(*args, **kwargs) and return a Future representing the call."))

	ThreadPoolExecutorType.Dict.Set("map", py.MustNewMethod("map", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) < 2 {
			return nil, py.ExceptionNewf(py.TypeError, "map() requires a function and at least one iterable")
		}
		var timeout py.Object = py.None
		if v, ok := kwargs.Get("timeout"); ok {
			timeout = v
		}
		return self.(*ThreadPoolExecutor).mapFn(args[0], []py.Object(args[1:]), timeout)
	}, 0, "Return an iterator equivalent to map(fn, iter)."))

	ThreadPoolExecutorType.Dict.Set("shutdown", py.MustNewMethod("shutdown", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		wait := py.Object(py.True)
		if err := py.ParseTupleAndKeywords(args, kwargs, "|O:shutdown",
			[]string{"wait"}, &wait); err != nil {
			return nil, err
		}
		w, err := py.ObjectIsTrue(wait)
		if err != nil {
			return nil, err
		}
		if err := self.(*ThreadPoolExecutor).shutdown(w); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Clean up the resources associated with this executor."))

	ThreadPoolExecutorType.Dict.Set("__enter__", py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Enter the context; return the executor."))
	ThreadPoolExecutorType.Dict.Set("__exit__", py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		if err := self.(*ThreadPoolExecutor).shutdown(true); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Shut the executor down and wait for its work."))

	// ---- ProcessPoolExecutor
	ProcessPoolExecutorType.Dict.Set("__init__", py.MustNewMethod("__init__", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"ProcessPoolExecutor is not implemented: this interpreter is a single process and cannot fork, so there is no way to run a task in another process. Use ThreadPoolExecutor instead.")
	}, 0, "Not implemented; always raises NotImplementedError."))

	// ---- the module
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "Future", Value: FutureType},
		py.DictEntry{Key: "Executor", Value: ExecutorType},
		py.DictEntry{Key: "ThreadPoolExecutor", Value: ThreadPoolExecutorType},
		py.DictEntry{Key: "ProcessPoolExecutor", Value: ProcessPoolExecutorType},
		py.DictEntry{Key: "CancelledError", Value: CancelledError},
		py.DictEntry{Key: "InvalidStateError", Value: InvalidStateError},
		py.DictEntry{Key: "BrokenExecutor", Value: BrokenExecutor},
		// TimeoutError is the BUILTIN, as in CPython: the except clauses
		// written for the module and for the builtin must catch one another's
		// exceptions.
		py.DictEntry{Key: "TimeoutError", Value: py.TimeoutError},
		py.DictEntry{Key: "FIRST_COMPLETED", Value: py.String(firstCompleted)},
		py.DictEntry{Key: "FIRST_EXCEPTION", Value: py.String(firstException)},
		py.DictEntry{Key: "ALL_COMPLETED", Value: py.String(allCompleted)},
		py.DictEntry{Key: "wait", Value: py.MustNewMethod("wait", waitFn, 0, "Wait for the futures to complete, returning the (done, not_done) pair.")},
		py.DictEntry{Key: "as_completed", Value: py.MustNewMethod("as_completed", asCompleted, 0, "Iterate over the futures as they complete, in completion order.")},
	)
	globals.Set("__all__", py.NewListFromItems([]py.Object{
		py.String("FIRST_COMPLETED"), py.String("FIRST_EXCEPTION"),
		py.String("ALL_COMPLETED"), py.String("CancelledError"),
		py.String("TimeoutError"), py.String("InvalidStateError"),
		py.String("BrokenExecutor"), py.String("Future"),
		py.String("Executor"), py.String("wait"), py.String("as_completed"),
		py.String("ProcessPoolExecutor"), py.String("ThreadPoolExecutor"),
	}))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "concurrent.futures",
			Doc:  module_doc,
		},
		Globals: globals,
	})

	// concurrent.futures._base carries the exception types under the names
	// their __module__ reports, and Error, which 3.14 no longer re-exports
	// from concurrent.futures but libraries still import from _base.
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "concurrent.futures._base",
			Doc:  "Base classes and exception types for concurrent.futures.",
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "Error", Value: Error},
			py.DictEntry{Key: "CancelledError", Value: CancelledError},
			py.DictEntry{Key: "TimeoutError", Value: py.TimeoutError},
			py.DictEntry{Key: "InvalidStateError", Value: InvalidStateError},
			py.DictEntry{Key: "BrokenExecutor", Value: BrokenExecutor},
			py.DictEntry{Key: "Future", Value: FutureType},
			py.DictEntry{Key: "Executor", Value: ExecutorType},
			py.DictEntry{Key: "FIRST_COMPLETED", Value: py.String(firstCompleted)},
			py.DictEntry{Key: "FIRST_EXCEPTION", Value: py.String(firstException)},
			py.DictEntry{Key: "ALL_COMPLETED", Value: py.String(allCompleted)},
			py.DictEntry{Key: "wait", Value: py.MustNewMethod("wait", waitFn, 0, "Wait for the futures to complete.")},
			py.DictEntry{Key: "as_completed", Value: py.MustNewMethod("as_completed", asCompleted, 0, "Iterate over the futures as they complete.")},
		),
	})
}
