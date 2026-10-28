// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inspect

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// This file is inspect's source lookup: getfile, getsourcefile, findsource,
// getsourcelines and getsource.  CPython finds where a block ends by running
// tokenize over it; gpython has no tokenizer to call, so blockEnd below scans
// logical lines itself.

// sourceMethods are registered into the module by init in inspect.go.
func sourceMethods() []py.DictEntry {
	one := func(name string, fn func(py.Object, py.Object) (py.Object, error)) py.DictEntry {
		return py.DictEntry{Key: name, Value: py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
			var obj py.Object
			if err := py.UnpackTuple(args, py.StringDict{}, name, 1, 1, &obj); err != nil {
				return nil, err
			}
			return fn(self, obj)
		}, 0, name+"(object) -> source lookup, as in CPython's inspect.")}
	}
	return []py.DictEntry{
		one("getfile", getfile),
		one("getsourcefile", getsourcefile),
		one("getsource", getsource),
		one("getsourcelines", getsourcelines),
		one("findsource", findsource),
	}
}

func typeErr(format string, a ...any) error {
	return py.ExceptionNewf(py.TypeError, format, a...)
}

func isInstance(obj py.Object, t *py.Type) bool { return obj.Type().IsSubtype(t) }

func attr(obj py.Object, name string) (py.Object, bool) {
	v, err := py.GetAttrString(obj, name)
	return v, err == nil
}

// getfile is inspect.getfile: the file an object was defined in.
func getfile(self, obj py.Object) (py.Object, error) {
	switch {
	case isInstance(obj, py.ModuleType):
		if f, ok := attr(obj, "__file__"); ok && f != py.None {
			return f, nil
		}
		repr, _ := py.ReprAsString(obj)
		return nil, typeErr("%s is a built-in module", repr)
	case py.IsClassObject(obj):
		if m, ok := attr(obj, "__module__"); ok {
			if name, ok := m.(py.String); ok {
				if m, ok := self.(*py.Module); ok && m.Context != nil {
					if mod, err := m.Context.GetModule(string(name)); err == nil {
						if f, ok := mod.Globals.Get("__file__"); ok && f != py.None {
							return f, nil
						}
					}
				}
			}
		}
		repr, _ := py.ReprAsString(obj)
		return nil, typeErr("%s is a built-in class", repr)
	}
	switch o := obj.(type) {
	case *py.BoundMethod:
		return getfile(self, o.Method)
	case *py.Function:
		return py.String(o.Code.Filename), nil
	case *py.Code:
		return py.String(o.Filename), nil
	case *py.Frame:
		return py.String(o.Code.Filename), nil
	case *py.Traceback:
		return py.String(o.Frame.Code.Filename), nil
	}
	return nil, typeErr("module, class, method, function, traceback, frame, or code object was expected, got %s", obj.Type().Name)
}

// getsourcefile is inspect.getsourcefile: the .py file, or None when there is
// no source on disk.
func getsourcefile(self, obj py.Object) (py.Object, error) {
	f, err := getfile(self, obj)
	if err != nil {
		return nil, err
	}
	name := string(f.(py.String))
	if !strings.HasSuffix(name, ".py") {
		return py.None, nil
	}
	if _, err := py.ReadPath(name); err != nil {
		return py.None, nil
	}
	return f, nil
}

// sourceLines reads the source file of obj, keeping line endings.
func sourceLines(self, obj py.Object) ([]string, error) {
	f, err := getsourcefile(self, obj)
	if err != nil {
		return nil, err
	}
	if f == py.None {
		return nil, py.ExceptionNewf(py.OSError, "source code not available")
	}
	b, err := py.ReadPath(string(f.(py.String)))
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "could not get source code")
	}
	return splitKeepEnds(string(b)), nil
}

func splitKeepEnds(s string) []string {
	var lines []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines, s = append(lines, s), ""
			break
		}
		lines, s = append(lines, s[:i+1]), s[i+1:]
	}
	return lines
}

func stringList(lines []string) py.Object {
	items := make([]py.Object, len(lines))
	for i, l := range lines {
		items[i] = py.String(l)
	}
	return py.NewListFromItems(items)
}

// findStart returns the file's lines and the 0-based line the object starts on.
func findStart(self, obj py.Object) ([]string, int, error) {
	lines, err := sourceLines(self, obj)
	if err != nil {
		return nil, 0, err
	}
	if isInstance(obj, py.ModuleType) {
		return lines, 0, nil
	}
	if py.IsClassObject(obj) {
		n, ok := attr(obj, "__firstlineno__")
		if !ok {
			return nil, 0, py.ExceptionNewf(py.OSError, "source code not available")
		}
		i, err := py.IndexInt(n)
		if err != nil {
			return nil, 0, err
		}
		return lines, i - 1, nil
	}
	if bm, ok := obj.(*py.BoundMethod); ok {
		obj = bm.Method
	}
	if fn, ok := obj.(*py.Function); ok {
		obj = fn.Code
	}
	if tb, ok := obj.(*py.Traceback); ok {
		obj = tb.Frame
	}
	if fr, ok := obj.(*py.Frame); ok {
		obj = fr.Code
	}
	code, ok := obj.(*py.Code)
	if !ok {
		return nil, 0, typeErr("module, class, method, function, traceback, frame, or code object was expected, got %s", obj.Type().Name)
	}
	lnum := int(code.Firstlineno) - 1
	if lnum >= len(lines) {
		return nil, 0, py.ExceptionNewf(py.OSError, "lineno is out of bounds")
	}
	return lines, lnum, nil
}

func findsource(self, obj py.Object) (py.Object, error) {
	lines, lnum, err := findStart(self, obj)
	if err != nil {
		return nil, err
	}
	return py.Tuple{stringList(lines), py.Int(lnum)}, nil
}

func getsourcelines(self, obj py.Object) (py.Object, error) {
	if next, ok := unwrapAll(obj); ok {
		obj = next
	}
	if tb, ok := obj.(*py.Traceback); ok {
		obj = tb.Frame
	}
	lines, lnum, err := findStart(self, obj)
	if err != nil {
		return nil, err
	}
	whole := isInstance(obj, py.ModuleType)
	if fr, ok := obj.(*py.Frame); ok && fr.Code.Name == "<module>" {
		whole = true
	}
	if whole {
		return py.Tuple{stringList(lines), py.Int(0)}, nil
	}
	return py.Tuple{stringList(blockOf(lines[lnum:])), py.Int(lnum + 1)}, nil
}

func getsource(self, obj py.Object) (py.Object, error) {
	r, err := getsourcelines(self, obj)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	for _, l := range r.(py.Tuple)[0].(*py.List).Items {
		sb.WriteString(string(l.(py.String)))
	}
	return py.String(sb.String()), nil
}

func unwrapAll(obj py.Object) (py.Object, bool) {
	changed := false
	for i := 0; i < 1000; i++ {
		next, ok := attr(obj, "__wrapped__")
		if !ok {
			break
		}
		obj, changed = next, true
	}
	return obj, changed
}

// logicalLine is one statement of source: where it starts and ends (0-based,
// inclusive), and the indent of its first line.
type logicalLine struct {
	start, end int
	indent     int
	text       string // the first line, trimmed
}

// logicalLines splits source lines into logical lines, skipping blank and
// comment-only lines.  It tracks string literals (including triple-quoted),
// brackets and backslash continuations, which are what make a statement span
// physical lines.
func logicalLines(lines []string) []logicalLine {
	var out []logicalLine
	var quote string // the open triple quote, if inside a multi-line string
	depth := 0
	cont := false
	var cur *logicalLine
	for i, line := range lines {
		wasInside := quote != "" || depth > 0 || cont
		cont = false
		j := 0
		for j < len(line) {
			c := line[j]
			if quote != "" {
				if c == '\\' {
					j += 2
					continue
				}
				if strings.HasPrefix(line[j:], quote) {
					j += len(quote)
					quote = ""
					continue
				}
				j++
				continue
			}
			switch c {
			case '#':
				j = len(line)
			case '\'', '"':
				if strings.HasPrefix(line[j:], strings.Repeat(string(c), 3)) {
					quote = strings.Repeat(string(c), 3)
					j += 3
					continue
				}
				// single-line string: skip to its end
				j++
				for j < len(line) && line[j] != c && line[j] != '\n' {
					if line[j] == '\\' {
						j++
					}
					j++
				}
				j++
			case '(', '[', '{':
				depth++
				j++
			case ')', ']', '}':
				if depth > 0 {
					depth--
				}
				j++
			case '\\':
				if strings.TrimRight(line[j+1:], "\r\n") == "" {
					cont = true
				}
				j += 2
			default:
				j++
			}
		}
		if !wasInside {
			trimmed := strings.TrimLeft(line, " \t\f")
			if trimmed == "" || trimmed[0] == '#' || trimmed[0] == '\r' || trimmed[0] == '\n' {
				continue
			}
			out = append(out, logicalLine{start: i, indent: len(line) - len(trimmed), text: strings.TrimSpace(trimmed)})
			cur = &out[len(out)-1]
		}
		if cur != nil {
			cur.end = i
		}
	}
	return out
}

// blockOf is inspect.getblock: the lines of the def, class or lambda that
// starts at lines[0], decorators included.  Its end is the end of the last
// statement indented deeper than the header, or the header statement itself
// when the body is on the same line.
func blockOf(lines []string) []string {
	ll := logicalLines(lines)
	if len(ll) == 0 {
		return lines
	}
	i := 0
	for i < len(ll) && strings.HasPrefix(ll[i].text, "@") {
		i++
	}
	if i == len(ll) {
		i = len(ll) - 1
	}
	header := ll[i]
	end := header.end
	for _, next := range ll[i+1:] {
		if next.indent <= header.indent {
			break
		}
		end = next.end
	}
	if i > 0 && end < ll[i].end {
		end = ll[i].end
	}
	return lines[:end+1]
}
