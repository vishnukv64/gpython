// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package traceback implements the traceback module: formatting an exception and
// the frames it passed through.
//
// The rendered output is CPython's, line for line, down to the "Traceback (most
// recent call last):" header and the two-space indent before "File".  Anything
// that renders an exception - pip's own error reporting, rich, a test runner -
// compares against that text, so matching it is the point of the module.

package traceback

import (
	"io"
	"os"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Extract, format and print information about Python stack traces.`

// frameSummary is the (filename, lineno, name, line) shape CPython's
// FrameSummary carries, kept as a plain struct because the module only needs
// the fields the formatters read.
type frameSummary struct {
	filename string
	lineno   int
	name     string
	line     string
}

func (f frameSummary) Type() *py.Type { return frameSummaryType }

var frameSummaryType = py.NewType("traceback.FrameSummary", "A single frame in a traceback.")

var frameSummaryFields = []string{"filename", "lineno", "name", "line"}

func init() {
	for i, name := range frameSummaryFields {
		idx := i
		fieldName := name
		frameSummaryType.Dict.Set(name, &py.Property{
			Fget: func(self py.Object) (py.Object, error) {
				fs, ok := self.(frameSummary)
				if !ok {
					return py.None, nil
				}
				switch idx {
				case 0:
					return py.String(fs.filename), nil
				case 1:
					return py.Int(fs.lineno), nil
				case 2:
					return py.String(fs.name), nil
				default:
					return py.String(fs.line), nil
				}
			},
			Doc: "The " + fieldName + " of this frame.",
		})
	}
	frameSummaryType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		fs, ok := self.(frameSummary)
		if !ok {
			return py.String("<traceback.FrameSummary>"), nil
		}
		return py.String("<FrameSummary file " + fs.filename + ", line " + itoa(fs.lineno) + " in " + fs.name + ">"), nil
	}, 0, "Return repr(self)."))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// exceptionText is the "ValueError: boom" line, without the trailing newline.
func exceptionText(e *py.Exception) string {
	t := e.Type()
	name := "Exception"
	if t != nil {
		name = t.Name
	}
	// An exception with no arguments has no colon - CPython prints just the
	// name.  str() already knows this, so it is the single source of truth.
	s, err := py.StrAsString(e)
	if err != nil || s == "" {
		return name
	}
	return name + ": " + s
}

// formatFrame is the "  File \"x.py\", line 3, in name" line plus the source
// line, when the source can be read.
func formatFrame(tb *py.Traceback) []string {
	if tb == nil || tb.Frame == nil || tb.Frame.Code == nil {
		return nil
	}
	code := tb.Frame.Code
	filename := code.Filename
	lineno := int(tb.Lineno)
	if lineno == 0 {
		lineno = int(code.Firstlineno)
	}
	out := []string{"  File \"" + filename + "\", line " + itoa(lineno) + ", in " + code.Name + "\n"}
	if line, ok := sourceLine(filename, lineno); ok {
		text := strings.TrimSpace(line)
		if text != "" {
			out = append(out, "    "+text+"\n")
		}
	}
	return out
}

// sourceLine reads one line from the file a frame came from.
//
// CPython omits the source line when it cannot read the file (a module built
// from a string, or a file since deleted), so a failure here is not an error.
func sourceLine(filename string, lineno int) (string, bool) {
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
	return lines[lineno-1], true
}

// formatExceptionInto writes the whole rendering of an exception, its
// traceback and its cause chain, and returns the number of bytes written.
func formatExceptionInto(w io.Writer, e *py.Exception, tb *py.Traceback, seen map[*py.Exception]bool) {
	if e == nil {
		return
	}
	if seen == nil {
		seen = map[*py.Exception]bool{}
	}
	if seen[e] {
		return
	}
	seen[e] = true

	// A chained exception prints its cause first, then the context, exactly as
	// CPython orders them; "Traceback (most recent call last):" precedes each.
	if cause, ok := e.Cause.(*py.Exception); ok && cause != nil {
		formatExceptionInto(w, cause, py.TracebackOf(cause), seen)
		io.WriteString(w, "\nThe above exception was the direct cause of the following exception:\n\n")
	} else if ctx, ok := e.Context.(*py.Exception); ok && ctx != nil && !e.SuppressContext {
		formatExceptionInto(w, ctx, py.TracebackOf(ctx), seen)
		io.WriteString(w, "\nDuring handling of the above exception, another exception occurred:\n\n")
	}

	if tb != nil {
		io.WriteString(w, "Traceback (most recent call last):\n")
		for t := tb; t != nil; t = t.Next {
			for _, line := range formatFrame(t) {
				io.WriteString(w, line)
			}
		}
	}
	io.WriteString(w, exceptionText(e)+"\n")
}

// splitLines turns a rendering into the list-of-strings CPython returns.
func splitLines(s string) py.Object {
	if s == "" {
		return py.NewList()
	}
	lines := strings.SplitAfter(s, "\n")
	out := py.NewList()
	for _, l := range lines {
		if l == "" {
			continue
		}
		out.Append(py.String(l))
	}
	return out
}

// resolveExcArgs works out (value, traceback) from the several forms CPython
// accepts: format_exception(exc), format_exception(type, value, tb), and
// format_exc() with nothing at all.
func resolveExcArgs(fname string, args py.Tuple) (*py.Exception, *py.Traceback, error) {
	switch len(args) {
	case 0:
		// CPython uses the exception being handled; outside a handler it uses
		// None as the value, which renders "NoneType: None".
		exc := py.CurrentException()
		if exc == nil {
			return nil, nil, nil
		}
		return exc, py.TracebackOf(exc), nil
	case 1:
		switch v := args[0].(type) {
		case *py.Exception:
			// Prefer the traceback the exception carries, which is where it
			// was raised and is what CPython uses.
			return v, py.TracebackOf(v), nil
		case *py.Type:
			if v.Flags&py.TPFLAGS_BASE_EXC_SUBCLASS != 0 {
				return py.MakeException(v), nil, nil
			}
			return nil, nil, py.ExceptionNewf(py.TypeError,
				"Exception expected for value, %s found", v.Name)
		default:
			return nil, nil, py.ExceptionNewf(py.TypeError,
				"Exception expected for value, %s found", args[0].Type().Name)
		}
	case 3:
		value, ok := args[1].(py.Object)
		if !ok {
			value = py.None
		}
		var exc *py.Exception
		if value == py.None {
			exc = nil
		} else {
			exc = py.MakeException(value)
		}
		if t, ok := args[2].(*py.Traceback); ok {
			return exc, t, nil
		}
		return exc, py.TracebackOf(exc), nil
	default:
		return nil, nil, py.ExceptionNewf(py.TypeError,
			"%s() takes at most 3 arguments (%d given)", fname, len(args))
	}
}

func formatException(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	exc, tb, err := resolveExcArgs("format_exception", args)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	formatExceptionInto(&sb, exc, tb, nil)
	return splitLines(sb.String()), nil
}

func formatExceptionOnly(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// CPython accepts format_exception_only(exc) and the older
	// format_exception_only(type, value) - traceback.format_exception_only
	// is called both ways in the wild, and pip/_internal/utils/temp_dir.py
	// uses the two-argument form.
	var exc *py.Exception
	switch len(args) {
	case 0:
		exc = py.CurrentException()
	case 1:
		e, _, err := resolveExcArgs("format_exception_only", args)
		if err != nil {
			return nil, err
		}
		exc = e
	case 2:
		value := args[1]
		if value == py.None {
			exc = nil
		} else {
			exc = py.MakeException(value)
		}
	default:
		return nil, py.ExceptionNewf(py.TypeError,
			"format_exception_only() takes at most 2 arguments (%d given)", len(args))
	}
	if exc == nil {
		return splitLines("NoneType: None\n"), nil
	}
	return splitLines(exceptionText(exc) + "\n"), nil
}

func formatExc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	exc, tb, err := resolveExcArgs("format_exc", py.Tuple{})
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	if exc == nil {
		return py.String("NoneType: None\n"), nil
	}
	formatExceptionInto(&sb, exc, tb, nil)
	return py.String(sb.String()), nil
}

func printExc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		limit py.Object = py.None
		file  py.Object = py.None
		chain py.Object = py.True
	)
	// print_exc(limit=None, file=None, chain=True)
	if len(args) > 0 {
		limit = args[0]
	}
	if len(args) > 1 {
		file = args[1]
	}
	if len(args) > 2 {
		chain = args[2]
	}
	_ = limit
	_ = chain
	exc, tb, err := resolveExcArgs("print_exc", py.Tuple{})
	if err != nil {
		return nil, err
	}
	if exc == nil {
		return py.None, nil
	}
	var sb strings.Builder
	formatExceptionInto(&sb, exc, tb, nil)
	if file == py.None {
		if _, err := os.Stderr.WriteString(sb.String()); err != nil {
			return nil, err
		}
		return py.None, nil
	}
	if _, err := py.Call(file, py.Tuple{py.String(sb.String())}, py.NewStringDict()); err != nil {
		return nil, err
	}
	return py.None, nil
}

// formatStack renders just the frames, with no exception.
func formatStack(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var f py.Object = py.None
	if len(args) > 0 {
		f = args[0]
	}
	if f == py.None {
		return py.String(""), nil
	}
	frame, ok := f.(*py.Frame)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "format_stack() requires a frame")
	}
	var sb strings.Builder
	for cur := frame; cur != nil; cur = cur.Back {
		tb := &py.Traceback{Frame: cur, Lineno: cur.Code.Addr2Line(cur.Lasti)}
		for _, line := range formatFrame(tb) {
			sb.WriteString(line)
		}
	}
	return splitLines(sb.String()), nil
}

// walkTb yields (frame, lineno) pairs from a traceback, which is the shape
// traceback.walk_tb has and what rich renders from.
func walkTb(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var tbObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "walk_tb", 1, 1, &tbObj); err != nil {
		return nil, err
	}
	pairs := make([]py.Object, 0, 8)
	tb, _ := tbObj.(*py.Traceback)
	for t := tb; t != nil; t = t.Next {
		if t.Frame == nil {
			continue
		}
		pairs = append(pairs, py.Tuple{t.Frame, py.Int(t.Lineno)})
	}
	return py.NewListFromItems(pairs), nil
}

// extractTb turns a traceback into the list of FrameSummary CPython returns.
func extractTb(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var tbObj py.Object = py.None
	if len(args) > 0 {
		tbObj = args[0]
	}
	frames := make([]py.Object, 0, 8)
	tb, _ := tbObj.(*py.Traceback)
	for t := tb; t != nil; t = t.Next {
		if t.Frame == nil || t.Frame.Code == nil {
			continue
		}
		line := ""
		if l, ok := sourceLine(t.Frame.Code.Filename, int(t.Lineno)); ok {
			line = strings.TrimSpace(l)
		}
		frames = append(frames, frameSummary{
			filename: t.Frame.Code.Filename,
			lineno:   int(t.Lineno),
			name:     t.Frame.Code.Name,
			line:     line,
		})
	}
	return py.NewListFromItems(frames), nil
}

func init() {
	globals := py.NewStringDict()
	globals.Set("__doc__", py.String(module_doc))

	globals.Set("format_exception", py.MustNewMethod("format_exception", formatException, 0, "Format and return the specified exception information as a list of strings."))
	globals.Set("format_exception_only", py.MustNewMethod("format_exception_only", formatExceptionOnly, 0, "Format the exception part of a traceback."))
	globals.Set("format_exc", py.MustNewMethod("format_exc", formatExc, 0, "Like print_exc() but return a string."))
	globals.Set("print_exc", py.MustNewMethod("print_exc", printExc, 0, "Shorthand for print_exception(*sys.exc_info())."))
	globals.Set("format_stack", py.MustNewMethod("format_stack", formatStack, 0, "Format a stack trace and its source code, as a list of strings."))
	globals.Set("walk_tb", py.MustNewMethod("walk_tb", walkTb, 0, "Walk a traceback, yielding each frame and line number."))
	globals.Set("extract_tb", py.MustNewMethod("extract_tb", extractTb, 0, "Return a list of FrameSummary for a traceback."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			FileDesc: "",
			Name:     "traceback",
			Doc:      module_doc,
		},
		Methods: nil,
		Globals: globals,
	})
}
