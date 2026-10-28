package py

import (
	"sync"
	"sync/atomic"
)

// The trace and profile functions sys.settrace and sys.setprofile install.
// They are process-wide here (CPython keeps them per thread); the VM asks
// TracingGlobal once per frame, never per instruction.
var (
	traceFunc   atomic.Pointer[Object]
	profileFunc atomic.Pointer[Object]
	// TracingGlobal is true while either function is set.
	TracingGlobal atomic.Bool
)

func store(p *atomic.Pointer[Object], f Object) {
	if f == nil || f == None {
		p.Store(nil)
	} else {
		p.Store(&f)
	}
	TracingGlobal.Store(traceFunc.Load() != nil || profileFunc.Load() != nil)
	updateSlow()
}

// SetTrace installs (or with None removes) the sys.settrace function.
func SetTrace(f Object) { store(&traceFunc, f) }

// SetProfile installs (or with None removes) the sys.setprofile function.
func SetProfile(f Object) { store(&profileFunc, f) }

// TraceFunc returns the sys.settrace function, or nil.
func TraceFunc() Object {
	if p := traceFunc.Load(); p != nil {
		return *p
	}
	return nil
}

// ProfileFunc returns the sys.setprofile function, or nil.
func ProfileFunc() Object {
	if p := profileFunc.Load(); p != nil {
		return *p
	}
	return nil
}

// TraceState is the per-frame tracing state.  It sits in a side table because
// almost no frame is traced and a field per frame would be paid by every call.
type TraceState struct {
	Trace     Object // f_trace
	EventLine int32  // f_lineno while a trace event runs, else 0
	Line      int32  // line of the last instruction a line event was considered for
	Lasti     int32  // offset of that instruction
}

var traceStates = struct {
	sync.Mutex
	m map[*Frame]*TraceState
}{m: map[*Frame]*TraceState{}}

func (f *Frame) traceStateOrNil() *TraceState {
	traceStates.Lock()
	defer traceStates.Unlock()
	return traceStates.m[f]
}

// TraceState returns the frame's tracing state, creating it.
func (f *Frame) TraceState() *TraceState {
	traceStates.Lock()
	defer traceStates.Unlock()
	ts := traceStates.m[f]
	if ts == nil {
		ts = &TraceState{}
		traceStates.m[f] = ts
	}
	return ts
}

// DropTraceState clears f_trace and forgets the frame's tracing state.
func (f *Frame) DropTraceState() {
	f.Tracing = false
	traceStates.Lock()
	delete(traceStates.m, f)
	traceStates.Unlock()
}
