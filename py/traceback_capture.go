// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Capturing a traceback into a Python-visible exception.
//
// The interpreter already built *Traceback values internally - AddTraceback in
// the vm walks the frames - but nothing ever made one reachable from Python:
// "e.__traceback__" raised AttributeError, and sys.exc_info()[2] was None.  So
// "traceback" had nothing to work from, and neither had anything else that
// wants to inspect where an exception came from.
//
// This lives in py rather than vm deliberately.  The running frame is already
// reachable from here - the vm installs CurrentFrame for the benefit of super()
// - so a raise can build its traceback without the vm package calling back in,
// and without py importing vm.

package py

// CaptureTraceback builds the traceback for a raise happening now.
//
// The frames are walked with Back, which goes from the frame executing now out
// to the outermost one - inner to outer - while a traceback runs the other way,
// outermost first, which is the order CPython's repr and traceback's formatting
// expect ("Traceback (most recent call last)" is only true in that order).  The
// walk is therefore reversed into the chain: each frame is prepended to the
// one built so far, so the last frame walked ends up first.
func CaptureTraceback() *Traceback {
	var frames []*Frame
	for f := currentFrame(); f != nil; f = f.Back {
		frames = append(frames, f)
	}
	var tb *Traceback
	for _, f := range frames {
		lineno := int32(0)
		if f.Code != nil {
			lineno = f.Code.Addr2Line(f.Lasti)
		}
		tb = &Traceback{Next: tb, Frame: f, Lasti: f.Lasti, Lineno: lineno}
	}
	return tb
}

// SetExceptionTraceback records the traceback on an exception, as CPython's
// PyException_SetTraceback does.
//
// It is only ever set once, when the exception is first raised: re-raising or
// chaining must not overwrite the place the exception originally came from,
// which is the same rule CPython follows.
func SetExceptionTraceback(e *Exception, tb *Traceback) {
	if e == nil || tb == nil {
		return
	}
	if existing, ok := e.Traceback.(*Traceback); ok && existing != nil {
		return
	}
	e.Traceback = tb
}

// TracebackOf returns an exception's traceback, or nil.
func TracebackOf(e *Exception) *Traceback {
	if e == nil {
		return nil
	}
	tb, _ := e.Traceback.(*Traceback)
	return tb
}

// SetCurrentExceptionFromValue republishes the exception being handled from a vm
// exception VALUE, which is whatever object the interpreter is carrying and so
// is not necessarily an *Exception.
//
// When an except block ends the vm restores the exception state that was in
// effect BEFORE the handler; this makes the Python-visible state agree with it.
// Without it, sys.exc_info() reported a finished handler's exception forever,
// and a nested handler destroyed the outer one's state instead of restoring it.
func SetCurrentExceptionFromValue(v Object) {
	if v == nil || v == None {
		SetCurrentException(nil)
		return
	}
	if e, ok := v.(*Exception); ok {
		SetCurrentException(e)
		return
	}
	SetCurrentException(MakeException(v))
}

func init() {
	// "__traceback__" is what a Python program reads to find out where an
	// exception came from - rich's traceback renderer, and therefore pip's
	// error output, is built on it.
	ExceptionType.Dict.Set("__traceback__", &Property{
		Fget: func(self Object) (Object, error) {
			e, ok := self.(*Exception)
			if !ok {
				return None, nil
			}
			if tb, ok := e.Traceback.(*Traceback); ok && tb != nil {
				return tb, nil
			}
			return None, nil
		},
		Fset: func(self Object, value Object) error {
			e, ok := self.(*Exception)
			if !ok {
				return ExceptionNewf(TypeError, "cannot set __traceback__ on this object")
			}
			if value == None {
				e.Traceback = nil
				return nil
			}
			tb, ok := value.(*Traceback)
			if !ok {
				return ExceptionNewf(TypeError, "__traceback__ must be a traceback or None")
			}
			e.Traceback = tb
			return nil
		},
		Doc: "The traceback for the exception, or None if it has not been raised.",
	})

	// with_traceback is the explicit form of the same thing.
	ExceptionType.Dict.Set("with_traceback", MustNewMethod("with_traceback", func(self Object, args Tuple) (Object, error) {
		e, ok := self.(*Exception)
		if !ok {
			return nil, ExceptionNewf(TypeError, "with_traceback() requires an exception")
		}
		var tb Object
		if err := UnpackTuple(args, StringDict{}, "with_traceback", 1, 1, &tb); err != nil {
			return nil, err
		}
		if tb != None {
			if _, ok := tb.(*Traceback); !ok {
				return nil, ExceptionNewf(TypeError, "with_traceback() argument must be a traceback or None")
			}
		}
		e.Traceback = tb
		return e, nil
	}, 0, "Sets the traceback on the exception, and returns the exception."))

	// The traceback's own attributes, for code that walks one.
	TracebackType.Dict.Set("tb_frame", &Property{
		Fget: func(self Object) (Object, error) {
			tb, ok := self.(*Traceback)
			if !ok || tb.Frame == nil {
				return None, nil
			}
			return tb.Frame, nil
		},
		Doc: "The frame this traceback entry refers to.",
	})
	TracebackType.Dict.Set("tb_next", &Property{
		Fget: func(self Object) (Object, error) {
			tb, ok := self.(*Traceback)
			if !ok || tb.Next == nil {
				return None, nil
			}
			return tb.Next, nil
		},
		Doc: "The next traceback entry, or None at the innermost frame.",
	})
	TracebackType.Dict.Set("tb_lasti", &Property{
		Fget: func(self Object) (Object, error) {
			tb, ok := self.(*Traceback)
			if !ok {
				return Int(0), nil
			}
			return Int(tb.Lasti), nil
		},
		Doc: "The index of the last attempted instruction.",
	})
	TracebackType.Dict.Set("tb_lineno", &Property{
		Fget: func(self Object) (Object, error) {
			tb, ok := self.(*Traceback)
			if !ok {
				return Int(0), nil
			}
			return Int(tb.Lineno), nil
		},
		Doc: "The line number this traceback entry was on.",
	})
}
