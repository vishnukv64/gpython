// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"sync"
	"sync/atomic"

	"github.com/petermattis/goid"
)

// Goroutine identity.  Every goroutine that runs Python concurrently with
// others registers with EnterGoroutine and gets a small logical id; everything
// else is goroutine 1.  While nothing is registered GoroutineID is one atomic
// load; once a thread exists it is a map lookup keyed by the runtime's own
// goroutine id, read by petermattis/goid in a few nanoseconds.  (It used to be
// parsed out of runtime.Stack, which walks the whole Go stack: ~35us at depth
// 200, on every Go-to-Python call, which made any threaded program ~35x
// slower - poetry's schema compiles went from 0.02s to 5s.)
var (
	registered atomic.Int32
	nextID     atomic.Uint64
	byRuntime  sync.Map // runtime goroutine id -> logical id
)

// GoroutineID returns the calling goroutine's logical id: 1 for the main (or
// any unregistered) goroutine.
func GoroutineID() uint64 {
	if registered.Load() == 0 {
		return 1
	}
	if id, ok := byRuntime.Load(runtimeGoID()); ok {
		return id.(uint64)
	}
	return 1
}

// EnterGoroutine registers the calling goroutine as a Python thread; call it
// first thing in the goroutine and defer the returned func.  A goroutine that
// runs Python while another does, without registering, shares id 1 and so its
// frame and exception state.
func EnterGoroutine() (leave func()) {
	rid := runtimeGoID()
	byRuntime.Store(rid, nextID.Add(1)+1)
	registered.Add(1)
	return func() {
		byRuntime.Delete(rid)
		registered.Add(-1)
	}
}

// runtimeGoID is the runtime's id for the calling goroutine.
func runtimeGoID() uint64 { return uint64(goid.Get()) }
