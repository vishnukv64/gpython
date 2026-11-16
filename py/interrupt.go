package py

import (
	"sync"
	"sync/atomic"
)

// InterruptFlag is a request, made by a host, to stop the code running on a
// Context. Request may be called from any goroutine.
//
// The interpreter loop tests the flag before every instruction (one atomic
// load). When it is set, the loop clears it and raises KeyboardInterrupt in the
// running frame, so the frame's own try/except can see it as usual. Blocking
// builtins that take an interruptible wait (time.sleep) return early.
//
// A request applies only to code running when it is delivered: the context
// clears the flag when a run starts and when it ends, so a late request cannot
// stop the next run.
type InterruptFlag struct {
	requested atomic.Bool

	mu   sync.Mutex
	wake chan struct{} // closed by Request, nil until someone waits
}

// Request asks the code running on the context to stop.
func (f *InterruptFlag) Request() {
	f.mu.Lock()
	f.requested.Store(true)
	if f.wake != nil {
		close(f.wake)
		f.wake = nil
	}
	f.mu.Unlock()
}

// Requested reports whether a request is pending. It is a single atomic load,
// cheap enough for the interpreter loop to test on every instruction.
func (f *InterruptFlag) Requested() bool {
	return f.requested.Load()
}

// Take clears a pending request and reports whether there was one.
func (f *InterruptFlag) Take() bool {
	return f.requested.CompareAndSwap(true, false)
}

// Clear drops any pending request.
func (f *InterruptFlag) Clear() {
	f.mu.Lock()
	f.requested.Store(false)
	f.mu.Unlock()
}

// Wake returns a channel that is closed by the next Request. If a request is
// already pending, it is closed already.
func (f *InterruptFlag) Wake() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.requested.Load() {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	if f.wake == nil {
		f.wake = make(chan struct{})
	}
	return f.wake
}
