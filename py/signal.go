package py

import "sync/atomic"

// SignalPending is set (cheaply, from any goroutine) when a Python-level
// signal handler is waiting to run.  The VM loop polls it between bytecodes
// and calls RunSignalHandlers, which the _signal module installs.
var SignalPending atomic.Bool

// RunSignalHandlers runs the queued handlers; a non-nil error is raised in the
// running frame.  Nil until _signal is imported.
var RunSignalHandlers func() error

// SignalSent, when set by _signal, is called after a process signals itself so
// an installed Python handler has been queued before the call returns.
var SignalSent func(sig int)
