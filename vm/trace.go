package vm

import "github.com/vishnukv64/gpython/py"

// sys.settrace / sys.setprofile support.  The interpreter loop pays for it
// with one test of frame.Tracing per instruction and of py.TracingGlobal per
// frame entry and exit; everything else lives here.
//
// Events: 'call' when a frame starts (or a generator resumes), 'line' before
// the first instruction of a new line or after a backward jump, 'exception'
// when an exception reaches a frame, 'return' when it returns, yields or is
// left by an exception.  setprofile gets 'call' and 'return' for Python
// frames; there are no c_call / c_return / c_exception events and no opcode
// events.

// callTrace calls fn(frame, event, arg) with the frame marked as inside a
// trace call, so nothing it runs generates events.  line is the f_lineno to
// show during the call.
func callTrace(frame *py.Frame, fn py.Object, event string, arg py.Object, line int32) (py.Object, error) {
	ts := frame.TraceState()
	frame.InTrace, ts.EventLine = true, line
	res, err := py.Call(fn, py.Tuple{frame, py.String(event), arg}, py.StringDict{})
	frame.InTrace, ts.EventLine = false, 0
	return res, err
}

// traceResult applies a trace function's result: a non-None result becomes
// the frame's f_trace; an error switches tracing off and is returned.
func traceResult(frame *py.Frame, res py.Object, err error) error {
	if err != nil {
		py.SetTrace(nil)
		frame.DropTraceState()
		return err
	}
	if res != py.None && res != nil {
		frame.TraceState().Trace, frame.Tracing = res, true
	}
	return nil
}

// traceEnter fires the 'call' events for a frame that is about to run.
func traceEnter(frame, prev *py.Frame) error {
	frame.InTrace = prev != nil && prev.InTrace
	if frame.InTrace {
		return nil
	}
	line := frame.Code.Firstlineno
	if frame.Lasti > 0 { // a generator resuming
		line = frame.CurrentLine()
	}
	if fn := py.TraceFunc(); fn != nil {
		res, err := callTrace(frame, fn, "call", py.None, line)
		if err := traceResult(frame, res, err); err != nil {
			return err
		}
	}
	if fn := py.ProfileFunc(); fn != nil {
		if _, err := callTrace(frame, fn, "call", py.None, line); err != nil {
			py.SetProfile(nil)
			return err
		}
	}
	return nil
}

// traceLine fires a 'line' event if the instruction about to run starts a new
// line or follows a backward jump.
func traceLine(frame *py.Frame) error {
	fn := py.TraceFunc()
	if fn == nil || frame.InTrace {
		return nil
	}
	line := frame.Code.Addr2Line(frame.Lasti)
	ts := frame.TraceState()
	back := frame.Lasti <= ts.Lasti
	ts.Lasti = frame.Lasti
	if line == ts.Line && !back {
		return nil
	}
	ts.Line = line
	res, err := callTrace(frame, ts.Trace, "line", py.None, line)
	return traceResult(frame, res, err)
}

// traceException fires the 'exception' event with (type, value, traceback).
func traceException(frame *py.Frame, exc py.ExceptionInfo) error {
	if py.TraceFunc() == nil || frame.InTrace {
		return nil
	}
	orNone := func(o py.Object) py.Object {
		if o == nil {
			return py.None
		}
		return o
	}
	var tb py.Object = py.None
	if exc.Traceback != nil {
		tb = exc.Traceback
	}
	arg := py.Tuple{orNone(exc.Type), orNone(exc.Value), tb}
	res, err := callTrace(frame, frame.TraceState().Trace, "exception", arg, frame.CurrentLine())
	return traceResult(frame, res, err)
}

// traceLeave fires the 'return' events as a frame returns, yields or is left
// by an exception (arg None).  A failure in a trace function replaces the
// frame's result.
func traceLeave(frame *py.Frame, retval py.Object, raised bool) error {
	if frame.InTrace {
		return nil
	}
	var arg py.Object = py.None
	if !raised && retval != nil {
		arg = retval
	}
	if frame.Tracing {
		if fn := py.TraceFunc(); fn != nil {
			res, err := callTrace(frame, frame.TraceState().Trace, "return", arg, frame.CurrentLine())
			if err := traceResult(frame, res, err); err != nil {
				return err
			}
		}
	}
	if fn := py.ProfileFunc(); fn != nil {
		if _, err := callTrace(frame, fn, "return", arg, frame.CurrentLine()); err != nil {
			py.SetProfile(nil)
			return err
		}
	}
	return nil
}
