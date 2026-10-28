package py

import (
	"sync"
	"sync/atomic"
)

// SignalPending is set (cheaply, from any goroutine) when a Python-level
// signal handler is waiting to run.  The VM loop polls it between bytecodes
// and calls RunSignalHandlers, which the _signal module installs.
var SignalPending atomic.Bool

// Slow is true while a signal handler is pending or a trace/profile function is
// set.  The interpreter loop tests this one flag per instruction and takes the
// out-of-line work only when it is up, so the common case costs one load.
// Keep it in step through SetSignalPending and SetTrace/SetProfile.
var Slow atomic.Bool

var slowMu sync.Mutex

func updateSlow() {
	slowMu.Lock()
	Slow.Store(SignalPending.Load() || TracingGlobal.Load())
	slowMu.Unlock()
}

// SetSignalPending sets SignalPending and keeps Slow in step.
func SetSignalPending(v bool) {
	SignalPending.Store(v)
	updateSlow()
}

// RunSignalHandlers runs the queued handlers; a non-nil error is raised in the
// running frame.  Nil until _signal is imported.
var RunSignalHandlers func() error

// SignalSent, when set by _signal, is called after a process signals itself so
// an installed Python handler has been queued before the call returns.
var SignalSent func(sig int)
