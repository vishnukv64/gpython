// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package futures provides python's 'concurrent.futures' module.
//
// ThreadPoolExecutor runs its tasks on real OS-backed goroutines, the same
// way this interpreter's threading.Thread does, so a task submitted here
// genuinely runs in parallel with the submitting thread.  There is no GIL
// (see the caveat on threading.Thread), so two tasks in the pool really do
// execute at once.
//
// That caveat applies here as it does to threading: with no GIL, a task that
// reads or writes a shared module global while the submitting thread touches
// the same one is a genuine data race, and "go test -race" reports it.  The
// same is already true of threading.Thread alone.  Python-level locks are the
// way to order such a pair.
//
// ProcessPoolExecutor is NOT implemented and raises NotImplementedError when
// constructed.  This interpreter is one process with no fork and no way to
// run py code in a second one, so it cannot be done; a ThreadPoolExecutor in
// disguise would be worse than the refusal, because code would silently stop
// getting process isolation and might pass - the tests it was written to
// satisfy - while sharing memory it assumed was private.
package futures

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Execute computations asynchronously using threads.

Classes:
    Future              -- Represents the result of an asynchronous computation.
    Executor            -- Abstract base class for executors.
    ThreadPoolExecutor  -- Executor using a pool of threads.

Functions:
    as_completed        -- Return an iterator over completed futures.
    wait                -- Wait for the futures to complete.

Exceptions:
    TimeoutError        -- Raised when a future does not complete in time.
    CancelledError      -- Raised when a future is cancelled.
    InvalidStateError   -- Raised when an operation is invalid for a future's state.
    BrokenExecutor      -- Raised when an executor cannot schedule work.

Not implemented:
    ProcessPoolExecutor.  This interpreter has a single address space and no
    fork, so a real process pool is impossible.  Constructing one raises
    NotImplementedError rather than silently running the work in threads,
    which would give back a result that is not the one the caller asked for.
    InterpreterPoolExecutor is likewise absent.`

// The return_when constants, which CPython compares by identity.
const (
	firstCompleted = "FIRST_COMPLETED"
	firstException = "FIRST_EXCEPTION"
	allCompleted   = "ALL_COMPLETED"
)

// ---------------------------------------------------------------------------
// exception types

var (
	// Error is the base of CancelledError and InvalidStateError, as in
	// CPython's concurrent.futures._base.  It is not re-exported from
	// concurrent.futures in 3.14, so it lives on the _base module below.
	Error = py.ExceptionType.NewType("concurrent.futures._base.Error",
		"Base class for all future-related exceptions.", nil, nil)
	// CancelledError is raised when result() is called on a cancelled future.
	CancelledError = Error.NewType("concurrent.futures._base.CancelledError",
		"The Future was cancelled.", nil, nil)
	// InvalidStateError is raised when an operation is not valid in the
	// future's state.
	InvalidStateError = Error.NewType("concurrent.futures._base.InvalidStateError",
		"The operation is not valid in the current state.", nil, nil)
	// BrokenExecutor is raised when an executor cannot schedule work.
	BrokenExecutor = py.RuntimeError.NewType("concurrent.futures._base.BrokenExecutor",
		"Executor is broken.", nil, nil)
)

// ---------------------------------------------------------------------------
// Future

// pending, running, cancelled and finished are the states CPython gives a
// Future, reported by the state predicates.
type futureState int

const (
	statePending futureState = iota
	stateRunning
	stateCancelled
	stateFinished
)

// futureSeq hands each Future a distinct identity, which is what its __hash__
// reports.  A dict or set encodes a member by its type name plus its hash, so
// two futures sharing a hash would collide; CPython hashes them by identity
// for the same reason.
var futureSeq int64

// Future is concurrent.futures.Future.
//
// All of its state is guarded by mu.  The completion channel is closed
// exactly once, by whoever finishes the future, and waiters select on it, so
// a waiter is woken by the transition rather than by polling.
type Future struct {
	mu        sync.Mutex
	state     futureState
	result    py.Object
	exc       *py.Exception
	callbacks []py.Object // add_done_callback targets, run on completion
	done      chan struct{}
	id        int64 // identity, reported by __hash__
}

var FutureType = py.NewTypeX("concurrent.futures.Future",
	"Represents the result of an asynchronous computation.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return newFuture(), nil
	}, nil)

func newFuture() *Future {
	f := &Future{
		state:  statePending,
		result: py.None,
		done:   make(chan struct{}),
	}
	f.id = atomic.AddInt64(&futureSeq, 1)
	return f
}

func (f *Future) Type() *py.Type { return FutureType }

// finish records the outcome and wakes every waiter.  It is called with mu
// held by the caller.
func (f *Future) finish(result py.Object, exc *py.Exception) {
	f.state = stateFinished
	f.result = result
	f.exc = exc
	close(f.done)
}

// runCallbacks invokes add_done_callback targets.  They run OUTSIDE the lock,
// because a callback is arbitrary py code and may call back into the future.
func (f *Future) runCallbacks() {
	f.mu.Lock()
	callbacks := f.callbacks
	f.callbacks = nil
	f.mu.Unlock()
	for _, cb := range callbacks {
		if _, err := py.Call(cb, py.Tuple{f}, py.StringDict{}); err != nil {
			// CPython logs this to the futures logger; there is no caller to
			// hand it to, so it is reported on stderr as threading does.
			fmt.Fprintf(os.Stderr, "Exception in future callback: %v\n", err)
		}
	}
}

// setResult completes the future with a value.
func (f *Future) setResult(value py.Object) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == stateCancelled || f.state == stateFinished {
		return py.ExceptionNewf(InvalidStateError, "invalid state")
	}
	f.finish(value, nil)
	return nil
}

// setException completes the future with an exception.
func (f *Future) setException(exc *py.Exception) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == stateCancelled || f.state == stateFinished {
		return py.ExceptionNewf(InvalidStateError, "invalid state")
	}
	f.finish(py.None, exc)
	return nil
}

func (f *Future) cancelled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state == stateCancelled
}

func (f *Future) running() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state == stateRunning
}

func (f *Future) doneState() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state == stateCancelled || f.state == stateFinished
}

// cancel attempts to cancel the future.  It succeeds only while the work has
// not started, which is the contract callers rely on to tell "I stopped it"
// from "it had already begun".
func (f *Future) cancel() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == stateRunning || f.state == stateFinished {
		return false
	}
	if f.state == stateCancelled {
		return true
	}
	f.finish(py.None, nil)
	f.state = stateCancelled
	return true
}

// exceptionFrom turns a Go error raised by py code into the exception the
// future must report.
//
// It has to unwrap py.ExceptionInfo itself: that type satisfies error, so the
// generic py.MakeException would take its "case error" branch and answer with
// a SystemError, and "except ZeroDivisionError" would not catch it.
func exceptionFrom(err error) *py.Exception {
	switch e := err.(type) {
	case py.ExceptionInfo:
		if e.Value != nil {
			return py.MakeException(e.Value)
		}
	case *py.ExceptionInfo:
		if e.Value != nil {
			return py.MakeException(e.Value)
		}
	case *py.Exception:
		return e
	}
	return py.MakeException(err)
}

// startRunning moves a pending future to running, and reports whether the
// work should go ahead.  It answers false when the future was cancelled while
// it waited in the queue.
func (f *Future) startRunning() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state != statePending {
		return false
	}
	f.state = stateRunning
	return true
}

// awaitResult waits for the future to finish and returns its outcome.
//
// timeout is a float in seconds or None.  A zero or negative timeout polls
// once, and the value -1 (the sentinel for a non-blocking call) does the same.
func (f *Future) awaitResult(timeout py.Object) (py.Object, *py.Exception, error) {
	f.mu.Lock()
	already := f.state == stateCancelled || f.state == stateFinished
	f.mu.Unlock()
	if already {
		return f.outcome()
	}
	if timeout != nil && timeout != py.None {
		secs, err := py.FloatAsFloat64(timeout)
		if err != nil {
			return nil, nil, err
		}
		if secs <= 0 {
			// A non-blocking attempt: it may have finished since the check.
			if f.doneState() {
				return f.outcome()
			}
			return nil, nil, py.ExceptionNewf(py.TimeoutError, "")
		}
		timer := time.NewTimer(time.Duration(secs * float64(time.Second)))
		defer timer.Stop()
		select {
		case <-f.done:
		case <-timer.C:
			return nil, nil, py.ExceptionNewf(py.TimeoutError, "")
		}
		return f.outcome()
	}
	<-f.done
	return f.outcome()
}

// outcome reads the finished future's result, translating the state into what
// result() must raise.
func (f *Future) outcome() (py.Object, *py.Exception, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == stateCancelled {
		return nil, nil, py.ExceptionNewf(CancelledError, "")
	}
	return f.result, f.exc, nil
}

func (f *Future) addCallback(cb py.Object) error {
	f.mu.Lock()
	if f.state == stateCancelled || f.state == stateFinished {
		f.mu.Unlock()
		// Already done: run it now, as CPython does.
		if _, err := py.Call(cb, py.Tuple{f}, py.StringDict{}); err != nil {
			return err
		}
		return nil
	}
	f.callbacks = append(f.callbacks, cb)
	f.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// Executor

// Executor is the abstract base class.  It is instantiable here as CPython's
// is (the module's own subclasses override its two methods), but its submit
// and map raise NotImplementedError from the Go side.
type Executor struct {
	Dict py.StringDict
}

var ExecutorType = py.NewTypeX("concurrent.futures.Executor",
	"Abstract base class for executors.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return &Executor{Dict: py.NewStringDict()}, nil
	}, nil)

func (e *Executor) Type() *py.Type { return ExecutorType }

// ---------------------------------------------------------------------------
// ThreadPoolExecutor

// ThreadPoolExecutor is a pool of worker goroutines.
//
// Work is handed to the workers over a buffered channel and the pool closes
// the channel on shutdown.  Each worker runs py code in its own goroutine,
// which this interpreter supports: the current frame and the current
// exception are keyed by goroutine id.
type ThreadPoolExecutor struct {
	mu      sync.Mutex
	work    chan *workItem
	workers int
	closed  bool
	broken  *py.Exception
	wg      sync.WaitGroup
	// initializer runs once in each worker before it takes work.
	initializer py.Object
	initargs    py.Tuple
	Dict        py.StringDict
}

// workItem is one submitted call, with the future its result is delivered to.
type workItem struct {
	fn     py.Object
	args   py.Tuple
	kwargs py.StringDict
	future *Future
}

var ThreadPoolExecutorType = ExecutorType.NewType("concurrent.futures.ThreadPoolExecutor",
	"An Executor subclass that executes calls asynchronously using a pool of at most max_workers threads.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		maxWorkers := 0
		if v, ok := kwargs.Get("max_workers"); ok && v != py.None {
			n, err := py.IndexInt(v)
			if err != nil {
				return nil, err
			}
			maxWorkers = n
		}
		if len(args) > 1 {
			return nil, py.ExceptionNewf(py.TypeError,
				"ThreadPoolExecutor() takes at most 1 positional argument")
		}
		if len(args) == 1 {
			n, err := py.IndexInt(args[0])
			if err != nil {
				return nil, err
			}
			maxWorkers = n
		}
		if maxWorkers <= 0 {
			// CPython defaults to min(32, os.cpu_count() + 4).  There is no
			// os.cpu_count here, so the Go runtime's count stands in for it.
			maxWorkers = runtime.NumCPU() + 4
			if maxWorkers > 32 {
				maxWorkers = 32
			}
		}
		initializer := py.Object(py.None)
		if v, ok := kwargs.Get("initializer"); ok {
			initializer = v
		}
		initargs := py.Tuple{}
		if v, ok := kwargs.Get("initargs"); ok {
			items, err := py.SequenceList(v)
			if err != nil {
				return nil, err
			}
			initargs = py.Tuple(items.Items)
		}
		return newThreadPool(maxWorkers, initializer, initargs), nil
	}, nil)

func (e *ThreadPoolExecutor) Type() *py.Type { return ThreadPoolExecutorType }

// newThreadPool constructs a pool and starts its workers.
func newThreadPool(maxWorkers int, initializer py.Object, initargs py.Tuple) *ThreadPoolExecutor {
	e := &ThreadPoolExecutor{
		work:        make(chan *workItem, maxWorkers*8),
		workers:     maxWorkers,
		initializer: initializer,
		initargs:    initargs,
		Dict:        py.NewStringDict(),
	}
	for i := 0; i < maxWorkers; i++ {
		e.wg.Add(1)
		go e.worker()
	}
	return e
}

// worker is one pool thread.  It takes work until the channel is closed by
// shutdown.
func (e *ThreadPoolExecutor) worker() {
	defer e.wg.Done()
	if e.initializer != nil && e.initializer != py.None {
		if _, err := py.Call(e.initializer, e.initargs, py.StringDict{}); err != nil {
			// An initializer that fails breaks the executor, as in CPython.
			e.mu.Lock()
			if e.broken == nil {
				e.broken = py.ExceptionNewf(BrokenExecutor,
					"initializer failed: %v", err)
			}
			e.mu.Unlock()
			return
		}
	}
	for item := range e.work {
		// The future becomes running only when a worker actually takes the
		// item.  Marking it running at submit time made cancel() fail for work
		// that was still queued, which is the case callers cancel for.
		if !item.future.startRunning() {
			// It was cancelled while queued: its outcome is already decided.
			item.future.runCallbacks()
			continue
		}
		result, err := py.Call(item.fn, item.args, item.kwargs)
		if err != nil {
			_ = item.future.setException(exceptionFrom(err))
		} else {
			_ = item.future.setResult(result)
		}
		item.future.runCallbacks()
	}
}

// submit queues fn(*args, **kwargs) and answers the future it will be
// delivered to.
func (e *ThreadPoolExecutor) submit(fn py.Object, args py.Tuple, kwargs py.StringDict) (*Future, error) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil, py.ExceptionNewf(py.RuntimeError,
			"cannot schedule new futures after shutdown")
	}
	if e.broken != nil {
		broken := e.broken
		e.mu.Unlock()
		return nil, broken
	}
	work := e.work
	e.mu.Unlock()

	future := newFuture()
	item := &workItem{fn: fn, args: args, kwargs: kwargs, future: future}

	// Sending must not block past a shutdown that has already closed the
	// channel: the mutex above is released before the send, so the send can
	// race with shutdown's close.  recover() turns the resulting panic into
	// the error CPython raises for it.
	var sendErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				sendErr = py.ExceptionNewf(py.RuntimeError,
					"cannot schedule new futures after shutdown")
			}
		}()
		work <- item
	}()
	if sendErr != nil {
		return nil, sendErr
	}
	return future, nil
}

// map applies fn to the arguments drawn from the iterables, yielding the
// results in order.
//
// The timeout applies to the collection of the results, not to any one
// call, which is CPython's meaning.
func (e *ThreadPoolExecutor) mapFn(fn py.Object, iterables []py.Object, timeout py.Object) (py.Object, error) {
	if len(iterables) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "map() requires at least one iterable")
	}
	// The iterables are consumed in lockstep, as zip() would consume them.
	iters := make([]py.Object, len(iterables))
	for i, it := range iterables {
		iter, err := py.Iter(it)
		if err != nil {
			return nil, err
		}
		iters[i] = iter
	}
	futures := []py.Object{}
	args := make([]py.Object, len(iters))
	nextFn := func(it py.Object) (py.Object, bool, error) {
		next, err := py.GetAttrString(it, "__next__")
		if err != nil {
			return nil, false, err
		}
		res, err := py.Call(next, py.Tuple{}, py.StringDict{})
		if err != nil {
			if py.IsException(py.StopIteration, err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		return res, true, nil
	}
	for {
		stop := false
		for i := range iters {
			v, ok, err := nextFn(iters[i])
			if err != nil {
				return nil, err
			}
			if !ok {
				stop = true
				break
			}
			args[i] = v
		}
		if stop {
			break
		}
		future, err := e.submit(fn, append(py.Tuple{}, args...), py.StringDict{})
		if err != nil {
			return nil, err
		}
		futures = append(futures, future)
	}
	return newResultIterator(futures, timeout), nil
}

// shutdown stops accepting work and waits for the running tasks.
func (e *ThreadPoolExecutor) shutdown(wait bool) error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
	} else {
		e.closed = true
		close(e.work)
		e.mu.Unlock()
	}
	if wait {
		e.wg.Wait()
	}
	return nil
}

// ---------------------------------------------------------------------------
// ProcessPoolExecutor

var ProcessPoolExecutorType = ExecutorType.NewType("concurrent.futures.ProcessPoolExecutor",
	"A process pool.  NOT IMPLEMENTED by this interpreter.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"ProcessPoolExecutor is not implemented: this interpreter is a single process and cannot fork, so there is no way to run a task in another process. Use ThreadPoolExecutor instead.")
	}, nil)

// ---------------------------------------------------------------------------
// iterators over futures

// resultIterator is the object map() returns: it yields each future's result
// in submission order, raising as result() would.
type resultIterator struct {
	futures []py.Object
	index   int
	timeout py.Object
	Dict    py.StringDict
}

var resultIteratorType = py.NewType("concurrent.futures._map_iterator",
	"Iterator over a ThreadPoolExecutor.map() result.")

func (it *resultIterator) Type() *py.Type { return resultIteratorType }

func newResultIterator(futures []py.Object, timeout py.Object) *resultIterator {
	return &resultIterator{futures: futures, timeout: timeout, Dict: py.NewStringDict()}
}

func (it *resultIterator) M__iter__() (py.Object, error) { return it, nil }

func (it *resultIterator) M__next__() (py.Object, error) {
	if it.index >= len(it.futures) {
		return nil, py.StopIteration
	}
	future := it.futures[it.index]
	it.index++
	f, ok := future.(*Future)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "map() produced a non-Future")
	}
	res, exc, err := f.awaitResult(it.timeout)
	if err != nil {
		return nil, err
	}
	if exc != nil {
		return nil, exc
	}
	return res, nil
}

// asCompletedIterator is the object as_completed() returns: it yields each
// future as it finishes, in completion order.
type asCompletedIterator struct {
	pending  []*Future
	finished []*Future
	timeout  py.Object
	Dict     py.StringDict
}

var asCompletedType = py.NewType("concurrent.futures._as_completed_iterator",
	"Iterator over futures as they complete.")

func (it *asCompletedIterator) Type() *py.Type { return asCompletedType }

func (it *asCompletedIterator) M__iter__() (py.Object, error) { return it, nil }

func (it *asCompletedIterator) M__next__() (py.Object, error) {
	if len(it.finished) != 0 {
		f := it.finished[0]
		it.finished = it.finished[1:]
		return f, nil
	}
	if len(it.pending) == 0 {
		return nil, py.StopIteration
	}
	// Poll the pending futures until one is done.  CPython uses a wait() with
	// a condition variable; polling is simpler and the same observable
	// behaviour, at the cost of waking every millisecond.
	deadline := time.Time{}
	if it.timeout != nil && it.timeout != py.None {
		secs, err := py.FloatAsFloat64(it.timeout)
		if err != nil {
			return nil, err
		}
		if secs <= 0 {
			return nil, py.ExceptionNewf(py.TimeoutError, "")
		}
		deadline = time.Now().Add(time.Duration(secs * float64(time.Second)))
	}
	for {
		var done []*Future
		var rest []*Future
		for _, f := range it.pending {
			if f.doneState() {
				done = append(done, f)
			} else {
				rest = append(rest, f)
			}
		}
		if len(done) != 0 {
			it.pending = rest
			it.finished = append(it.finished, done...)
			f := it.finished[0]
			it.finished = it.finished[1:]
			return f, nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, py.ExceptionNewf(py.TimeoutError, "")
		}
		time.Sleep(time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// module functions
