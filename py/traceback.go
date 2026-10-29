// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Traceback objects

package py

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// A python Traceback object
type Traceback struct {
	Next   *Traceback
	Frame  *Frame
	Lasti  int32
	Lineno int32
}

var TracebackType = NewType("traceback", "A python traceback")

// Type of this object
func (o *Traceback) Type() *Type {
	return TracebackType
}

// Make a new traceback
func NewTraceback(next *Traceback, frame *Frame, lasti, lineno int32) *Traceback {
	return &Traceback{
		Next:   next,
		Frame:  frame,
		Lasti:  lasti,
		Lineno: lineno,
	}
}

/*
Traceback (most recent call last):
  File "throws.py", line 8, in <module>
    main()
  File "throws.py", line 5, in main
    throws()
  File "throws.py", line 2, in throws
    raise RuntimeError('this is the error message')
RuntimeError: this is the error message
*/

// Dump a traceback for tb to w
func (tb *Traceback) TracebackDump(w io.Writer) {
	for _, line := range tb.FormatLines() {
		io.WriteString(w, line)
	}
}

// FormatLines renders the frames of tb the way CPython's StackSummary.format
// does: a File line and the stripped source line per frame, with a run of more
// than three identical frames (recursion) cut to three and a
// "[Previous line repeated N more times]" line.  Where the compiler recorded a
// source range, the source and "~~~^^^" caret lines follow CPython's.
func (tb *Traceback) FormatLines() []string {
	var out []string
	var lastFile, lastName string
	var lastLine int32 = -1
	count := 0
	flush := func() {
		if count > 3 {
			n := count - 3
			s := "s"
			if n == 1 {
				s = ""
			}
			out = append(out, fmt.Sprintf("  [Previous line repeated %d more time%s]\n", n, s))
		}
	}
	scope := ""
	for ; tb != nil; tb = tb.Next {
		code := tb.Frame.Code
		// CPython inlines list, set and dict comprehensions (PEP 709): they
		// have no frame of their own, so the frame that calls one is shown
		// at the failing position inside it.  Here they do have one, so the
		// calling entry is dropped and the comprehension's is shown under
		// the caller's name.
		if tb.Next != nil && isInlinedComprehension(tb.Next.Frame.Code) && tb.Next.Frame.Code.Filename == code.Filename {
			if scope == "" {
				scope = code.Name
			}
			continue
		}
		name := code.Name
		if isInlinedComprehension(code) {
			if scope != "" {
				name = scope
			}
		}
		scope = ""
		if code.Filename != lastFile || tb.Lineno != lastLine || name != lastName {
			flush()
			lastFile, lastLine, lastName = code.Filename, tb.Lineno, name
			count = 0
		}
		count++
		if count > 3 {
			continue
		}
		out = append(out, fmt.Sprintf("  File \"%s\", line %d, in %s\n", code.Filename, tb.Lineno, name))
		line, endLine, col, endCol, hasCols := code.PositionAt(max(tb.Lasti-1, 0))
		if line != int(tb.Lineno) || endLine < line {
			// The table and the line number disagree (a frame traced past
			// its last instruction): fall back to the plain source line.
			line, endLine, hasCols = int(tb.Lineno), int(tb.Lineno), false
		}
		out = append(out, frameSourceLines(code.Filename, line, endLine, col, endCol, hasCols)...)
	}
	flush()
	return out
}

// SourceLine reads line lineno (1-based) of filename, without its newline.
//
// CPython omits the source line when it cannot read the file (a string
// compiled with a "<...>" name, or a file since deleted), so a failure is not
// an error.
// ponytail: re-reads the file per frame; add a cache if deep tracebacks are slow.
func SourceLine(filename string, lineno int) (string, bool) {
	if filename == "" || strings.HasPrefix(filename, "<") || lineno <= 0 {
		return "", false
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(data), "\n")
	if lineno > len(lines) {
		return "", false
	}
	return strings.TrimSuffix(lines[lineno-1], "\r"), true
}

// Dumps a traceback to stderr
func TracebackDump(err interface{}) {
	switch e := err.(type) {
	case ExceptionInfo:
		e.TracebackDump(os.Stderr)
	case *ExceptionInfo:
		e.TracebackDump(os.Stderr)
	case *Exception:
		fmt.Fprintf(os.Stderr, "Exception %v\n", e)
		fmt.Fprintf(os.Stderr, "-- No traceback available --\n")
	default:
		fmt.Fprintf(os.Stderr, "Error %v\n", err)
		fmt.Fprintf(os.Stderr, "-- No traceback available --\n")
	}
}

// Properties
func init() {
	TracebackType.Dict.Set("__tb_next__", &Property{
		Fget: func(self Object) (Object, error) {
			next := self.(*Traceback).Next
			if next == nil {
				return None, nil
			}
			return next, nil
		},
	})
	TracebackType.Dict.Set("__tb_frame__", &Property{
		Fget: func(self Object) (Object, error) {
			return self.(*Traceback).Frame, nil
		},
	})
	TracebackType.Dict.Set("__tb_lasti__", &Property{
		Fget: func(self Object) (Object, error) {
			return Int(self.(*Traceback).Lasti), nil
		},
	})
	TracebackType.Dict.Set("__tb_lineno__", &Property{
		Fget: func(self Object) (Object, error) {
			return Int(self.(*Traceback).Lineno), nil
		},
	})
}

// Make sure it satisfies the interface
var _ Object = (*Traceback)(nil)

// isInlinedComprehension reports whether code is the body of a list, set or
// dict comprehension, which CPython runs in the frame of its caller.
func isInlinedComprehension(code *Code) bool {
	switch code.Name {
	case "<listcomp>", "<setcomp>", "<dictcomp>":
		return true
	}
	return false
}
