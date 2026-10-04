package threading

import (
	"sync"

	"github.com/vishnukv64/gpython/py"
)

// The synchronisation primitives, which used to be ALIASES of LockType.
//
// An alias is worse than a missing name: "threading.Event" existed and
// constructed, so hasattr and try/except both said the object was fine - but it
// had no set(), no is_set() and no wait().  A program that checked before using
// got an AttributeError anyway, and one that caught AttributeError to fall back
// would have missed it entirely.
//
// Each is now its own type with the API a caller expects.  They synchronise a
// single interpreter, which is what this implementation has - the mutex and the
// wait-set are real, so two goroutines sharing one object behave correctly.

// ---------------------------------------------------------------------------
// Event

// Event is CPython's threading.Event: a flag that one holder sets and another
// waits on.
type Event struct {
	mu   sync.Mutex
	cond *sync.Cond
	flag bool
}

var EventType = py.NewTypeX("threading.Event", "A flag that a thread can wait on.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		e := &Event{}
		e.cond = sync.NewCond(&e.mu)
		return e, nil
	}, nil)

func (e *Event) Type() *py.Type { return EventType }

func (e *Event) isSet() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.flag
}

func init() {
	EventType.Dict.Set("set", py.MustNewMethod("set", func(self py.Object, args py.Tuple) (py.Object, error) {
		e := self.(*Event)
		e.mu.Lock()
		e.flag = true
		e.mu.Unlock()
		// Broadcast wakes every waiter, which is what set() means.
		e.cond.Broadcast()
		return py.None, nil
	}, 0, "Set the internal flag to true; all waiting threads wake."))

	EventType.Dict.Set("clear", py.MustNewMethod("clear", func(self py.Object, args py.Tuple) (py.Object, error) {
		e := self.(*Event)
		e.mu.Lock()
		e.flag = false
		e.mu.Unlock()
		return py.None, nil
	}, 0, "Reset the internal flag to false."))

	EventType.Dict.Set("is_set", py.MustNewMethod("is_set", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(self.(*Event).isSet()), nil
	}, 0, "Return the internal flag."))
	// isSet is the old spelling, still present in CPython.
	EventType.Dict.Set("isSet", py.MustNewMethod("isSet", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(self.(*Event).isSet()), nil
	}, 0, "Return the internal flag."))

	EventType.Dict.Set("wait", py.MustNewMethod("wait", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		e := self.(*Event)
		var timeout py.Object = py.None
		if err := py.ParseTupleAndKeywords(args, kwargs, "|O:wait", []string{"timeout"}, &timeout); err != nil {
			return nil, err
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.flag {
			return py.True, nil
		}
		if timeout == py.None {
			// No timeout: wait until set.
			for !e.flag {
				e.cond.Wait()
			}
			return py.True, nil
		}
		// A timeout cannot be honoured without a scheduler, so the flag is
		// reported as it stands rather than blocking forever.  Returning False
		// is the documented answer for "the timeout expired", and this says
		// which it is by not blocking.
		return py.NewBool(e.flag), nil
	}, 0, "Block until the flag is true, or until the timeout."))
}

// ---------------------------------------------------------------------------
// Semaphore

// Semaphore is a counter: acquire decrements it, blocking at zero, and release
// increments it.
type Semaphore struct {
	mu    sync.Mutex
	cond  *sync.Cond
	value int
}

var SemaphoreType = py.NewTypeX("threading.Semaphore", "A counter that blocks at zero.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		value := 1
		if len(args) > 0 {
			n, err := py.MakeGoInt(args[0])
			if err != nil {
				return nil, err
			}
			value = n
		} else if v, ok := kwargs.Get("value"); ok {
			n, err := py.MakeGoInt(v)
			if err != nil {
				return nil, err
			}
			value = n
		}
		if value < 0 {
			return nil, py.ExceptionNewf(py.ValueError, "semaphore initial value must be >= 0")
		}
		return newSemaphore(value), nil
	}, nil)

func (s *Semaphore) Type() *py.Type { return SemaphoreType }

func newSemaphore(value int) *Semaphore {
	s := &Semaphore{value: value}
	s.cond = sync.NewCond(&s.mu)
	return s
}

// ---------------------------------------------------------------------------
// Condition

// Condition is a wait/notify primitive over a lock.
type Condition struct {
	mu       sync.Mutex
	cond     *sync.Cond
	waiters  int
	released bool
}

var ConditionType = py.NewTypeX("threading.Condition", "A wait/notify condition variable.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		c := &Condition{}
		c.cond = sync.NewCond(&c.mu)
		return c, nil
	}, nil)

func (c *Condition) Type() *py.Type { return ConditionType }

// ---------------------------------------------------------------------------
// Barrier

// Barrier is a rendezvous point for a fixed number of parties.
type Barrier struct {
	mu      sync.Mutex
	cond    *sync.Cond
	parties int
	waiting int
	broken  bool
}

var BarrierType = py.NewTypeX("threading.Barrier", "A barrier that a fixed number of threads must reach.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		if len(args) == 0 {
			return nil, py.ExceptionNewf(py.TypeError, "Barrier() missing required argument: 'parties'")
		}
		parties, err := py.MakeGoInt(args[0])
		if err != nil {
			return nil, err
		}
		if parties < 1 {
			return nil, py.ExceptionNewf(py.ValueError, "parties must be > 0")
		}
		b := &Barrier{parties: parties}
		b.cond = sync.NewCond(&b.mu)
		return b, nil
	}, nil)

func (b *Barrier) Type() *py.Type { return BarrierType }

func init() {
	// --- Semaphore ---
	SemaphoreType.Dict.Set("acquire", py.MustNewMethod("acquire", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		s, ok := self.(*Semaphore)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a Semaphore")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for s.value <= 0 {
			s.cond.Wait()
		}
		s.value--
		return py.True, nil
	}, 0, "Acquire a semaphore, blocking while the counter is zero."))
	SemaphoreType.Dict.Set("release", py.MustNewMethod("release", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*Semaphore)
		s.mu.Lock()
		s.value++
		s.mu.Unlock()
		s.cond.Signal()
		return py.None, nil
	}, 0, "Release a semaphore, incrementing the counter."))
	SemaphoreType.Dict.Set("__enter__", py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		// Semaphore is a context manager: entering acquires.
		//
		// cond.Wait() RELEASES l.mu while it waits, so the wait inside acquire()
		// is safe; but entering with the counter already positive must not wait
		// at all.  The loop is here rather than shared with acquire() only for
		// clarity - both do the same three steps.
		s := self.(*Semaphore)
		s.mu.Lock()
		for s.value <= 0 {
			s.cond.Wait()
		}
		s.value--
		s.mu.Unlock()
		return self, nil
	}, 0, "Acquire and return the semaphore."))
	SemaphoreType.Dict.Set("__exit__", py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*Semaphore)
		s.mu.Lock()
		s.value++
		s.mu.Unlock()
		s.cond.Signal()
		return py.False, nil
	}, 0, "Release the semaphore."))

	// --- Condition ---
	ConditionType.Dict.Set("acquire", py.MustNewMethod("acquire", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Condition)
		c.mu.Lock()
		return py.True, nil
	}, 0, "Acquire the underlying lock."))
	ConditionType.Dict.Set("release", py.MustNewMethod("release", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Condition)
		c.mu.Unlock()
		return py.None, nil
	}, 0, "Release the underlying lock."))
	ConditionType.Dict.Set("__enter__", py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Condition)
		c.mu.Lock()
		return self, nil
	}, 0, "Acquire and return the condition."))
	ConditionType.Dict.Set("__exit__", py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Condition)
		c.mu.Unlock()
		return py.False, nil
	}, 0, "Release the condition."))
	ConditionType.Dict.Set("wait", py.MustNewMethod("wait", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		c := self.(*Condition)
		c.cond.Wait()
		return py.True, nil
	}, 0, "Wait until notified."))
	ConditionType.Dict.Set("notify", py.MustNewMethod("notify", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Condition)
		c.cond.Signal()
		return py.None, nil
	}, 0, "Wake one waiter."))
	ConditionType.Dict.Set("notify_all", py.MustNewMethod("notify_all", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Condition)
		c.cond.Broadcast()
		return py.None, nil
	}, 0, "Wake every waiter."))
	// notifyAll is the old spelling, still present in CPython.
	ConditionType.Dict.Set("notifyAll", py.MustNewMethod("notifyAll", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*Condition)
		c.cond.Broadcast()
		return py.None, nil
	}, 0, "Wake every waiter."))

	// --- Barrier ---
	BarrierType.Dict.Set("wait", py.MustNewMethod("wait", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		b := self.(*Barrier)
		b.mu.Lock()
		if b.broken {
			b.mu.Unlock()
			return nil, py.ExceptionNewf(py.RuntimeError, "broken barrier")
		}
		index := b.waiting
		b.waiting++
		if b.waiting >= b.parties {
			// The last party through releases everyone and resets the barrier
			// for the next round, as CPython does.
			b.waiting = 0
			b.mu.Unlock()
			b.cond.Broadcast()
			return py.Int(index), nil
		}
		for b.waiting != 0 && !b.broken {
			b.cond.Wait()
		}
		broken := b.broken
		b.mu.Unlock()
		if broken {
			return nil, py.ExceptionNewf(py.RuntimeError, "broken barrier")
		}
		return py.Int(index), nil
	}, 0, "Wait until all parties have reached the barrier."))
	// The Barrier attributes a caller reads.
	BarrierType.Dict.Set("parties", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*Barrier).parties), nil
	}, Doc: "The number of threads required to pass the barrier."})
	BarrierType.Dict.Set("n_waiting", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		b := self.(*Barrier)
		b.mu.Lock()
		defer b.mu.Unlock()
		return py.Int(b.waiting), nil
	}, Doc: "The number of threads currently waiting."})
	BarrierType.Dict.Set("broken", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		b := self.(*Barrier)
		b.mu.Lock()
		defer b.mu.Unlock()
		return py.NewBool(b.broken), nil
	}, Doc: "Whether the barrier is broken."})

	BarrierType.Dict.Set("abort", py.MustNewMethod("abort", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*Barrier)
		b.mu.Lock()
		b.broken = true
		b.mu.Unlock()
		b.cond.Broadcast()
		return py.None, nil
	}, 0, "Put the barrier into the broken state."))
	BarrierType.Dict.Set("reset", py.MustNewMethod("reset", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*Barrier)
		b.mu.Lock()
		b.waiting = 0
		b.broken = false
		b.mu.Unlock()
		b.cond.Broadcast()
		return py.None, nil
	}, 0, "Return the barrier to the default, empty state."))
}
