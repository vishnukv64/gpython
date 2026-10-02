// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue provides the implementation of python's 'queue' module.
//
// A Queue is a real blocking queue: put() and get() wait on a Go sync.Cond,
// so a consumer in one goroutine genuinely blocks until a producer in another
// puts an item.  The interpreter runs threading.Thread.start's target in a Go
// goroutine, so this maps directly onto the interpreter's threads.  Queue,
// LifoQueue and PriorityQueue differ only in the order get() returns:
// FIFO, LIFO and heap order respectively.
package queue

import (
	"container/heap"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `A multi-producer, multi-consumer queue.

This module implements multi-producer, multi-consumer queues.  put() and get()
block on the queue's condition variable, so a consumer waits for a producer
rather than spinning.
`

var (
	EmptyType = py.ExceptionType.NewType("queue.Empty",
		"Raised by Queue.get_nowait() when the queue is empty.", nil, nil)
	FullType = py.ExceptionType.NewType("queue.Full",
		"Raised by Queue.put_nowait() when the queue is full.", nil, nil)
)

// queueType is the Python-visible queue class.  All three variants share it;
// ordering is selected by the kind field.
var queueType = py.NewTypeX("queue.Queue",
	"A queue: put(), get(), task_done() and join().", queueNew, nil)

var (
	LifoQueueType = py.NewTypeX("queue.LifoQueue",
		"A LIFO queue: get() returns the most recently added item.", lifoNew, nil)
	PriorityQueueType = py.NewTypeX("queue.PriorityQueue",
		"A priority queue: get() returns the smallest item.", priorityNew, nil)
)

// kind selects the ordering the queue applies on get.
type kind int

const (
	kindFIFO kind = iota
	kindLIFO
	kindPriority
)

// queue is the blocking queue itself.  The mutex guards everything below it;
// the condition broadcasts when the contents change, so a blocked get or put
// re-checks its predicate.
type queue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond

	kind     kind
	maxsize  int
	items    []py.Object // FIFO/LIFO storage
	heap     prioHeap    // priority storage
	unfinish int64
	Dict     py.StringDict
}

func (q *queue) Type() *py.Type         { return queueType }
func (q *queue) GetDict() py.StringDict { return q.Dict }

// prioHeap orders py.Objects by their Python "less than".  Its Less calls back
// into the interpreter, which is why ordering is not done with sort.Slice:
// heap operations need the comparison at each sift.
type prioHeap struct {
	items []py.Object
	q     *queue
}

func (h prioHeap) Len() int { return len(h.items) }
func (h prioHeap) Less(i, j int) bool {
	res, err := py.Lt(h.items[i], h.items[j])
	if err != nil {
		return false
	}
	lt, err := py.ObjectIsTrue(res)
	if err != nil {
		return false
	}
	return lt
}
func (h prioHeap) Swap(i, j int)       { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *prioHeap) Push(x interface{}) { h.items = append(h.items, x.(py.Object)) }
func (h *prioHeap) Pop() interface{} {
	old := h.items
	n := len(old)
	it := old[n-1]
	h.items = old[:n-1]
	return it
}

func queueNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var maxsize py.Object = py.Int(0)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:__init__",
		[]string{"maxsize"}, &maxsize); err != nil {
		return nil, err
	}
	n, err := py.IndexInt(maxsize)
	if err != nil {
		return nil, err
	}
	q := &queue{kind: kindFIFO, maxsize: n, Dict: py.NewStringDict()}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	q.heap.q = q
	return q, nil
}

// qsize reports the number of items.  The caller must hold q.mu.
func (q *queue) qsize() int {
	if q.kind == kindPriority {
		return len(q.heap.items)
	}
	return len(q.items)
}

func queueQSize(self py.Object, args py.Tuple) (py.Object, error) {
	q := self.(*queue)
	q.mu.Lock()
	defer q.mu.Unlock()
	return py.Int(q.qsize()), nil
}

func queueEmpty(self py.Object, args py.Tuple) (py.Object, error) {
	q := self.(*queue)
	q.mu.Lock()
	defer q.mu.Unlock()
	return py.Bool(q.qsize() == 0), nil
}

func queueFull(self py.Object, args py.Tuple) (py.Object, error) {
	q := self.(*queue)
	q.mu.Lock()
	defer q.mu.Unlock()
	return py.Bool(q.maxsize > 0 && q.qsize() >= q.maxsize), nil
}

// putNowait adds an item without blocking, reporting whether it fit.
func (q *queue) putNowait(item py.Object) bool {
	if q.maxsize > 0 && q.qsize() >= q.maxsize {
		return false
	}
	if q.kind == kindPriority {
		heap.Push(&q.heap, item)
	} else {
		q.items = append(q.items, item)
	}
	q.notEmpty.Signal()
	return true
}

func queuePutNowait(self py.Object, args py.Tuple) (py.Object, error) {
	q := self.(*queue)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "put_nowait() takes exactly one argument")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.putNowait(args[0]) {
		return nil, py.ExceptionNewf(FullType, "Full")
	}
	return py.None, nil
}

func queuePut(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	q := self.(*queue)
	var item py.Object
	var block py.Object = py.Bool(true)
	var timeout py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:put",
		[]string{"item", "block", "timeout"}, &item, &block, &timeout); err != nil {
		return nil, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.maxsize <= 0 || q.qsize() < q.maxsize {
		q.putNowait(item)
		return py.None, nil
	}
	if !truthy(block) {
		return nil, py.ExceptionNewf(FullType, "Full")
	}
	if timeout != py.None {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"queue.Queue.put: a timeout on a full queue is not implemented; "+
				"put(block=True) with no timeout waits on the queue's condition variable")
	}
	for q.maxsize > 0 && q.qsize() >= q.maxsize {
		q.notFull.Wait()
	}
	q.putNowait(item)
	return py.None, nil
}

// getNowait removes and returns an item, reporting whether one was available.
func (q *queue) getNowait() (py.Object, bool) {
	if q.qsize() == 0 {
		return nil, false
	}
	var item py.Object
	switch q.kind {
	case kindFIFO:
		item = q.items[0]
		q.items = q.items[1:]
	case kindLIFO:
		n := len(q.items) - 1
		item = q.items[n]
		q.items = q.items[:n]
	case kindPriority:
		item = heap.Pop(&q.heap).(py.Object)
	}
	q.unfinish++
	q.notFull.Signal()
	return item, true
}

func queueGetNowait(self py.Object, args py.Tuple) (py.Object, error) {
	q := self.(*queue)
	q.mu.Lock()
	defer q.mu.Unlock()
	item, ok := q.getNowait()
	if !ok {
		return nil, py.ExceptionNewf(EmptyType, "Empty")
	}
	return item, nil
}

func queueGet(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	q := self.(*queue)
	var block py.Object = py.Bool(true)
	var timeout py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:get",
		[]string{"block", "timeout"}, &block, &timeout); err != nil {
		return nil, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.qsize() > 0 {
		item, _ := q.getNowait()
		return item, nil
	}
	if !truthy(block) {
		return nil, py.ExceptionNewf(EmptyType, "Empty")
	}
	if timeout != py.None {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"queue.Queue.get: a timeout on an empty queue is not implemented; "+
				"get(block=True) with no timeout waits on the queue's condition variable")
	}
	for q.qsize() == 0 {
		q.notEmpty.Wait()
	}
	item, _ := q.getNowait()
	return item, nil
}

func queueTaskDone(self py.Object, args py.Tuple) (py.Object, error) {
	q := self.(*queue)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.unfinish <= 0 {
		return nil, py.ExceptionNewf(py.ValueError, "task_done() called too many times")
	}
	q.unfinish--
	if q.unfinish == 0 {
		q.notEmpty.Broadcast() // join() waits on this condition
	}
	return py.None, nil
}

func queueJoin(self py.Object, args py.Tuple) (py.Object, error) {
	q := self.(*queue)
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.unfinish > 0 {
		q.notEmpty.Wait()
	}
	return py.None, nil
}

func truthy(v py.Object) bool {
	if v == nil || v == py.None {
		return false
	}
	b, err := py.ObjectIsTrue(v)
	if err != nil {
		return false
	}
	return b
}

// lifoNew and priorityNew build the two ordering variants.  CPython derives
// LifoQueue and PriorityQueue from Queue and overrides _get/_put; here the
// ordering is a field, so the new functions set it directly.
func lifoNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	obj, err := queueNew(metatype, args, kwargs)
	if err != nil {
		return nil, err
	}
	obj.(*queue).kind = kindLIFO
	return obj, nil
}

func priorityNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	obj, err := queueNew(metatype, args, kwargs)
	if err != nil {
		return nil, err
	}
	obj.(*queue).kind = kindPriority
	return obj, nil
}

func init() {
	for _, m := range []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"put", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return queuePut(self, args, kw)
		}, "Put an item into the queue."},
		{"get", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return queueGet(self, args, kw)
		}, "Remove and return an item from the queue."},
		{"put_nowait", queuePutNowait, "Put an item into the queue without blocking."},
		{"get_nowait", queueGetNowait, "Remove and return an item without blocking."},
		{"qsize", queueQSize, "Return the approximate size of the queue."},
		{"empty", queueEmpty, "Return True if the queue is empty."},
		{"full", queueFull, "Return True if the queue is full."},
		{"task_done", queueTaskDone, "Indicate that a formerly enqueued task is complete."},
		{"join", queueJoin, "Block until all items in the queue have been gotten and processed."},
	} {
		queueType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}
	queueType.Dict.Set("maxsize", py.Int(0))

	globals := py.NewStringDict()
	globals.Set("Empty", EmptyType)
	globals.Set("Full", FullType)
	globals.Set("Queue", queueType)
	globals.Set("LifoQueue", LifoQueueType)
	globals.Set("PriorityQueue", PriorityQueueType)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "queue",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
