// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"reflect"
	"sync"
	"unsafe"
)

// Nesting guards for containers that can reach themselves.
//
// A list, dict or set may contain itself, so repr and comparison recurse
// without bound.  CPython guards repr with Py_ReprEnter/Py_ReprLeave, which
// prints "[...]" for a container already being repr'd on this thread, and
// guards comparison with the C recursion limit.  The state is per goroutine:
// each goroutine running Python has its own repr stack and comparison depth,
// so concurrent contexts never see each other's containers.  All access goes
// through nestMu.
//
// ponytail: one global mutex over a per-goroutine map; contention only matters
// when many threads compare or repr nested containers at once.  Per-goroutine
// state on GoState would remove the lock if that shows up in a profile.
type nestState struct {
	repr  []unsafe.Pointer // containers whose repr is in progress, innermost last
	depth int              // nested container comparisons in progress
}

var (
	nestMu sync.Mutex
	nests  = map[uint64]*nestState{}
)

// identityKey is the address of a mutable container, which is what identity
// means for it.  ok is false for objects with no stable address here (strings,
// numbers); those compare by value and cannot form cycles.
func identityKey(o Object) (unsafe.Pointer, bool) {
	switch x := o.(type) {
	case *List:
		return unsafe.Pointer(x), true
	case *Set:
		return unsafe.Pointer(x), true
	case *FrozenSet:
		return unsafe.Pointer(x), true
	case Tuple:
		if len(x) == 0 {
			return nil, false
		}
		return unsafe.Pointer(unsafe.SliceData(x)), true
	case StringDict:
		return reflect.ValueOf(x.m).UnsafePointer(), x.m != nil
	}
	return nil, false
}

// sameItem reports whether a and b are the same object, the shortcut CPython
// takes before comparing container items: [x] == [x] is True even when x
// contains itself, because x is compared with x.
func sameItem(a, b Object) bool {
	ka, ok := identityKey(a)
	if !ok {
		return false
	}
	kb, ok := identityKey(b)
	return ok && ka == kb
}

// nestingOf returns the calling goroutine's nesting state, creating it on
// first use.  Callers hold nestMu.
func nestingOf() *nestState {
	id := GoroutineID()
	st := nests[id]
	if st == nil {
		st = &nestState{}
		nests[id] = st
	}
	return st
}

// ReprEnter marks the container with key as being repr'd on this goroutine.
// ok is false when it already is, which means the repr recurses into itself;
// the caller then prints its "..." placeholder and must not call leave.
func ReprEnter(key unsafe.Pointer) (leave func(), ok bool) {
	nestMu.Lock()
	defer nestMu.Unlock()
	st := nestingOf()
	for _, k := range st.repr {
		if k == key {
			return nil, false
		}
	}
	st.repr = append(st.repr, key)
	gid := GoroutineID()
	return func() {
		nestMu.Lock()
		defer nestMu.Unlock()
		st := nests[gid]
		if st == nil {
			return
		}
		st.repr = st.repr[:len(st.repr)-1]
		if st.depth == 0 && len(st.repr) == 0 {
			delete(nests, gid)
		}
	}, true
}

func noop() {}

// enterRepr guards the repr of container o.  recursing is true when o is
// already being repr'd on this goroutine; the caller then prints its
// placeholder and must not call leave.  Objects without an identity cannot
// recurse, so they get a no-op leave.
func enterRepr(o Object) (leave func(), recursing bool) {
	k, has := identityKey(o)
	if !has {
		return noop, false
	}
	leave, ok := ReprEnter(k)
	return leave, !ok
}

// compareEnter counts one more nested container comparison on this goroutine.
// It fails with RecursionError at RecursionLimit instead of letting the Go
// stack grow without bound, as CPython does with its C recursion limit.
func compareEnter() (leave func(), err error) {
	nestMu.Lock()
	defer nestMu.Unlock()
	st := nestingOf()
	if int64(st.depth) >= RecursionLimit.Load() {
		return nil, ExceptionNewf(RecursionError, "maximum recursion depth exceeded in comparison")
	}
	st.depth++
	gid := GoroutineID()
	return func() {
		nestMu.Lock()
		defer nestMu.Unlock()
		st := nests[gid]
		if st == nil {
			return
		}
		st.depth--
		if st.depth == 0 && len(st.repr) == 0 {
			delete(nests, gid)
		}
	}, nil
}

// itemsEqual is CPython's PyObject_RichCompareBool(a, b, Py_EQ): identical
// objects are equal without asking __eq__, otherwise it is Eq's answer.
func itemsEqual(a, b Object) (Object, error) {
	if sameItem(a, b) {
		return True, nil
	}
	return Eq(a, b)
}
