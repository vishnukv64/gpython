// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// Goroutine identity.  Go exposes no goroutine id, and the only way to read
// one (runtime.Stack) walks the whole Go stack, so its cost grows with call
// depth - it dominated pip install.  Instead every goroutine that runs Python
// concurrently with others registers with EnterGoroutine and gets a small
// logical id; everything else is goroutine 1.  While nothing is registered
// GoroutineID is one atomic load.
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
// frame and exception state.  ponytail: slow runtime.Stack path while any
// thread is live; make it a per-thread context if that matters.
func EnterGoroutine() (leave func()) {
	rid := runtimeGoID()
	byRuntime.Store(rid, nextID.Add(1)+1)
	registered.Add(1)
	return func() {
		byRuntime.Delete(rid)
		registered.Add(-1)
	}
}

// runtimeGoID parses the id from the stack header "goroutine 123 [running]:".
func runtimeGoID() uint64 {
	var buf [32]byte
	s := buf[:runtime.Stack(buf[:], false)]
	var id uint64
	for _, c := range s[len("goroutine "):] {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}
