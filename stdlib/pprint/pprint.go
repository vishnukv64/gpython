// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pprint provides the implementation of python's 'pprint' module.
//
// The algorithm is a port of CPython's Lib/pprint.py: the same two-phase
// representation, in which _repr() builds a one-line rendering and _format()
// decides whether that rendering fits and, if not, dispatches on the object's
// type to break it across lines.  That structure is what makes the output
// match CPython's, including the detail that a single line is preferred
// whenever it fits.
package pprint

import (
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Support to pretty-print lists, tuples, & dictionaries recursively.

Very simple, but useful, especially in debugging data structures.

Classes
-------

PrettyPrinter()
    Handle pretty-printing operations onto a stream using a configured
    set of formatting parameters.

Functions
---------

pformat()
    Format a Python object into a pretty-printed representation.

pprint()
    Pretty-print a Python object to a stream [default is sys.stdout].

pp()
    Alias for pprint().  (with sort_dicts=False)`

// printer is the Go state behind one formatting run.
type printer struct {
	indent         int
	width          int
	depth          int // 0 means None
	hasDepth       bool
	compact        bool
	sortDicts      bool
	underscoreNums bool
	readable       bool
	recursive      bool
}

// newPrinter reads the PrettyPrinter keyword arguments.
func newPrinter(kwargs py.StringDict) (*printer, error) {
	p := &printer{
		indent:    1,
		width:     80,
		sortDicts: true,
		readable:  true,
	}
	if v, ok := kwargs["indent"]; ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		p.indent = n
	}
	if v, ok := kwargs["width"]; ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		p.width = n
	}
	if v, ok := kwargs["depth"]; ok && v != py.None {
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		p.depth = n
		p.hasDepth = true
	}
	if v, ok := kwargs["compact"]; ok {
		b, err := py.ObjectIsTrue(v)
		if err != nil {
			return nil, err
		}
		p.compact = b
	}
	if v, ok := kwargs["sort_dicts"]; ok {
		b, err := py.ObjectIsTrue(v)
		if err != nil {
			return nil, err
		}
		p.sortDicts = b
	}
	if v, ok := kwargs["underscore_numbers"]; ok {
		b, err := py.ObjectIsTrue(v)
		if err != nil {
			return nil, err
		}
		p.underscoreNums = b
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// The one-line representation, CPython's _safe_repr.

// safeRepr renders object on one line.  context holds the ids of the containers
// currently being rendered, so a cycle is reported rather than looping.
//
// maxlevels is the configured depth limit (0 when depth was not set) and level is
// the current nesting depth; they are separate arguments because CPython's
// _format calls _repr at the object's own depth while the limit stays fixed.
func (p *printer) safeRepr(object py.Object, context map[uintptr]bool, maxlevels int, level int) (string, bool, bool, error) {
	// The builtin scalars are formatted by their own __repr__.
	switch v := object.(type) {
	case py.String, py.Bytes, py.Float, py.Bool, py.NoneType, *py.BigInt:
		rep, err := py.ReprAsString(v)
		if err != nil {
			return "", false, false, err
		}
		return rep, true, false, nil
	case py.Int:
		if p.underscoreNums {
			return addUnderscores(int64(v)), true, false, nil
		}
		rep, err := py.ReprAsString(v)
		if err != nil {
			return "", false, false, err
		}
		return rep, true, false, nil
	}

	switch obj := object.(type) {
	case *py.List:
		if len(obj.Items) == 0 {
			return "[]", true, false, nil
		}
		objid := objID(obj)
		if maxlevels > 0 && level >= maxlevels {
			return "[...]", false, context[objid], nil
		}
		if context[objid] {
			return recursion(obj), false, true, nil
		}
		context[objid] = true
		readable, recursive := true, false
		components := make([]string, 0, len(obj.Items))
		for _, o := range obj.Items {
			orepr, oreadable, orecur, err := p.safeRepr(o, context, maxlevels, level+1)
			if err != nil {
				return "", false, false, err
			}
			components = append(components, orepr)
			if !oreadable {
				readable = false
			}
			if orecur {
				recursive = true
			}
		}
		delete(context, objid)
		return "[" + strings.Join(components, ", ") + "]", readable, recursive, nil
	case py.Tuple:
		if len(obj) == 0 {
			return "()", true, false, nil
		}
		format := "(%s)"
		if len(obj) == 1 {
			format = "(%s,)"
		}
		objid := objID(obj)
		if maxlevels > 0 && level >= maxlevels {
			return strings.Replace(format, "%s", "...", 1), false, context[objid], nil
		}
		if context[objid] {
			return recursion(obj), false, true, nil
		}
		context[objid] = true
		readable, recursive := true, false
		components := make([]string, 0, len(obj))
		for _, o := range obj {
			orepr, oreadable, orecur, err := p.safeRepr(o, context, maxlevels, level+1)
			if err != nil {
				return "", false, false, err
			}
			components = append(components, orepr)
			if !oreadable {
				readable = false
			}
			if orecur {
				recursive = true
			}
		}
		delete(context, objid)
		return strings.Replace(format, "%s", strings.Join(components, ", "), 1), readable, recursive, nil
	case py.StringDict:
		if len(obj) == 0 {
			return "{}", true, false, nil
		}
		objid := objID(obj)
		if maxlevels > 0 && level >= maxlevels {
			return "{...}", false, context[objid], nil
		}
		if context[objid] {
			return recursion(obj), false, true, nil
		}
		context[objid] = true
		readable, recursive := true, false
		pairs := p.dictEntries(obj)
		components := make([]string, 0, len(pairs))
		for _, kv := range pairs {
			krepr, kreadable, krecur, err := p.safeRepr(kv.key, context, maxlevels, level+1)
			if err != nil {
				return "", false, false, err
			}
			vrepr, vreadable, vrecur, err := p.safeRepr(kv.val, context, maxlevels, level+1)
			if err != nil {
				return "", false, false, err
			}
			components = append(components, krepr+": "+vrepr)
			if !kreadable || !vreadable {
				readable = false
			}
			if krecur || vrecur {
				recursive = true
			}
		}
		delete(context, objid)
		return "{" + strings.Join(components, ", ") + "}", readable, recursive, nil
	}

	rep, err := py.ReprAsString(object)
	if err != nil {
		return "", false, false, err
	}
	return rep, rep != "" && !strings.HasPrefix(rep, "<"), false, nil
}

// recursive is CPython's _recursion marker for a container already being
// rendered.
func recursion(object py.Object) string {
	// CPython includes the identity: "<Recursion on list with id=4426631808>".
	return "<Recursion on " + object.Type().Name + " with id=" + strconv.FormatUint(uint64(objID(object)), 10) + ">"
}

// objID is a stable identity for cycle detection.
//
// The interpreter's objects are Go values of many kinds and the values of the
// scalar types (py.Int, py.String, ...) are not pointers, so reflect.Pointer is
// not applicable to them.  Only the three container kinds can participate in a
// cycle, and those all arrive as pointers, so identity is taken from the pointer
// for those and from the value itself for everything else (which can never be a
// cycle and so only needs to differ from the containers' ids, which are real
// addresses and never equal a small integer).
func objID(object py.Object) uintptr {
	switch v := object.(type) {
	case *py.List:
		return reflect.ValueOf(v).Pointer()
	case py.Tuple:
		if len(v) == 0 {
			return 1
		}
		return reflect.ValueOf(v).Pointer()
	case py.StringDict:
		if len(v) == 0 {
			return 2
		}
		return reflect.ValueOf(v).Pointer()
	}
	return 0
}

// copyContext duplicates a context map, as CPython's context.copy() does
// before a speculative _repr call whose cycle marks must not leak.
func copyContext(m map[uintptr]bool) map[uintptr]bool {
	out := make(map[uintptr]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// underscores formats an unsigned integer in decimal; it is used by
// addUnderscores and by the recursion marker.
func itoa(n uintptr) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// addUnderscores formats an integer with _ digit separators, as CPython's
// f"{object:_d}" does.
func addUnderscores(n int64) string {
	neg := n < 0
	var u uint64
	if neg {
		u = uint64(-n)
	} else {
		u = uint64(n)
	}
	digits := itoa(uintptr(u))
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	lead := len(digits) % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(digits[:lead])
	for i := lead; i < len(digits); i += 3 {
		b.WriteByte('_')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// kv is one decoded dict entry.
type kv struct {
	key py.Object
	val py.Object
}

// dictEntries decodes a StringDict into ordered key/value pairs, sorted when
// sort_dicts is set.
//
// The interpreter stores dicts as a map with encoded string keys, so a dict
// built by a script is already in map order, which is not insertion order; the
// sort below is therefore also what makes the default output deterministic.
func (p *printer) dictEntries(d py.StringDict) []kv {
	pairs := make([]kv, 0, len(d))
	for k, v := range d {
		key, err := py.DictKeyDecode(k)
		if err != nil {
			continue
		}
		pairs = append(pairs, kv{key, v})
	}
	if p.sortDicts {
		sort.SliceStable(pairs, func(i, j int) bool {
			return safeKeyLess(pairs[i].key, pairs[j].key)
		})
	}
	return pairs
}

// safeKeyLess orders two dict keys the way CPython's _safe_key does: by the
// natural comparison, falling back to (type name, identity) for unorderable
// types.  CPython compares id() there; the pointer is the same idea.
func safeKeyLess(a, b py.Object) bool {
	res, err := py.Lt(a, b)
	if err == nil {
		if t, ok := res.(py.Bool); ok {
			return bool(t)
		}
	}
	ta := a.Type().Name
	tb := b.Type().Name
	if ta != tb {
		return ta < tb
	}
	return objID(a) < objID(b)
}

// ---------------------------------------------------------------------------
// The line-breaking representation, CPython's _format.

// formatObject writes object to b, breaking it across lines when its one-line
// rendering does not fit.
func (p *printer) formatObject(object py.Object, b *strings.Builder, indent, allowance int, context map[uintptr]bool, level int) error {
	objid := objID(object)
	if context[objid] {
		b.WriteString(recursion(object))
		p.recursive = true
		p.readable = false
		return nil
	}
	// The context is threaded through, not replaced with a fresh map: the
	// one-line rendering has to know which containers are already on the
	// stack or it recurses forever on a self-referential one.
	rep, readable, recursive, err := p.safeRepr(object, context, p.depthOrZero(level), level)
	if err != nil {
		return err
	}
	// Track readability the way CPython's _repr does, over the whole object.
	if !readable {
		p.readable = false
	}
	if recursive {
		p.recursive = true
	}
	maxWidth := p.width - indent - allowance
	if len(rep) <= maxWidth {
		b.WriteString(rep)
		return nil
	}
	// The one-line form does not fit: dispatch on the type.
	//
	// The object is marked in context for the duration of the dispatch, which
	// is what CPython's _format does.  Checking context[objid] without ever
	// SETTING it meant a self-referential container that does not fit on one
	// line recursed into itself forever: "pformat(r, width=20)" on "r = [];
	// r.append(r)" never returned, with no output and no exception.
	context[objid] = true
	defer delete(context, objid)

	switch obj := object.(type) {
	case *py.List:
		b.WriteByte('[')
		if err := p.formatItems(obj.Items, b, indent, allowance+1, context, level); err != nil {
			return err
		}
		b.WriteByte(']')
		return nil
	case py.Tuple:
		endchar := ")"
		if len(obj) == 1 {
			endchar = ",)"
		}
		b.WriteByte('(')
		if err := p.formatItems([]py.Object(obj), b, indent, allowance+len(endchar), context, level); err != nil {
			return err
		}
		b.WriteString(endchar)
		return nil
	case py.StringDict:
		b.WriteByte('{')
		if p.indent > 1 {
			b.WriteString(strings.Repeat(" ", p.indent-1))
		}
		if len(obj) > 0 {
			if err := p.formatDictItems(p.dictEntries(obj), b, indent, allowance+1, context, level); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	}
	b.WriteString(rep)
	return nil
}

// depthOrZero is CPython's maxlevels argument: the configured depth, or 0 when
// no depth was set.
func (p *printer) depthOrZero(level int) int {
	if !p.hasDepth {
		return 0
	}
	return p.depth
}

// reprString is CPython's _repr: a one-line rendering plus the readability and
// recursion flags folded into the printer.
func (p *printer) reprString(object py.Object, context map[uintptr]bool, level int) (string, error) {
	rep, readable, recursive, err := p.safeRepr(object, context, p.depthOrZero(level), level)
	if err != nil {
		return "", err
	}
	if !readable {
		p.readable = false
	}
	if recursive {
		p.recursive = true
	}
	return rep, nil
}

// formatItems is CPython's _format_items.
func (p *printer) formatItems(items []py.Object, b *strings.Builder, indent, allowance int, context map[uintptr]bool, level int) error {
	indent += p.indent
	if p.indent > 1 {
		b.WriteString(strings.Repeat(" ", p.indent-1))
	}
	delimnl := ",\n" + strings.Repeat(" ", indent)
	delim := ""
	width := p.width - indent + 1
	maxWidth := width
	if len(items) == 0 {
		return nil
	}
	for i := 0; i < len(items); i++ {
		last := i == len(items)-1
		ent := items[i]
		if last {
			maxWidth -= allowance
			width -= allowance
		}
		if p.compact {
			rep, err := p.reprString(ent, copyContext(context), level)
			if err != nil {
				return err
			}
			w := len(rep) + 2
			if width < w {
				width = maxWidth
				if delim != "" {
					delim = delimnl
				}
			}
			if width >= w {
				width -= w
				b.WriteString(delim)
				delim = ", "
				b.WriteString(rep)
				continue
			}
		}
		b.WriteString(delim)
		delim = delimnl
		a := 1
		if last {
			a = allowance
		}
		if err := p.formatObject(ent, b, indent, a, context, level); err != nil {
			return err
		}
	}
	return nil
}

// formatDictItems is CPython's _format_dict_items.
func (p *printer) formatDictItems(items []kv, b *strings.Builder, indent, allowance int, context map[uintptr]bool, level int) error {
	indent += p.indent
	delimnl := ",\n" + strings.Repeat(" ", indent)
	for i, kvp := range items {
		last := i == len(items)-1
		rep, err := p.reprString(kvp.key, context, level)
		if err != nil {
			return err
		}
		b.WriteString(rep)
		b.WriteString(": ")
		a := 1
		if last {
			a = allowance
		}
		if err := p.formatObject(kvp.val, b, indent+len(rep)+2, a, context, level); err != nil {
			return err
		}
		if !last {
			b.WriteString(delimnl)
		}
	}
	return nil
}

// pformat formats object the way PrettyPrinter.pformat does.
func (p *printer) pformat(object py.Object) (string, error) {
	var b strings.Builder
	if err := p.formatObject(object, &b, 0, 0, map[uintptr]bool{}, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

// ---------------------------------------------------------------------------
// Module-level functions

// stdoutObject returns sys.stdout through the module the calling function is
// bound to, which is how builtin.print reaches it too.
func stdoutObject(self py.Object) (py.Object, error) {
	mod, ok := self.(*py.Module)
	if !ok {
		// A PrettyPrinter method's self is the printer, not the module, so fall
		// back to its Method's module through the type's own dict.
		return nil, py.ExceptionNewf(py.RuntimeError, "pprint has no module context to reach sys.stdout through")
	}
	sysModule, err := mod.Context.GetModule("sys")
	if err != nil {
		return nil, err
	}
	stdout, ok := sysModule.Globals["stdout"]
	if !ok {
		return nil, py.ExceptionNewf(py.RuntimeError, "sys.stdout is not available")
	}
	return stdout, nil
}

// writeTo writes s to stream.
func writeTo(stream py.Object, s string) error {
	write, err := py.GetAttrString(stream, "write")
	if err != nil {
		return err
	}
	_, err = py.Call(write, py.Tuple{py.String(s)}, nil)
	return err
}

func pformatFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var object py.Object
	if err := py.UnpackTuple(args, nil, "pformat", 1, 1, &object); err != nil {
		return nil, err
	}
	p, err := newPrinter(kwargs)
	if err != nil {
		return nil, err
	}
	s, err := p.pformat(object)
	if err != nil {
		return nil, err
	}
	return py.String(s), nil
}

func pprintFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// stream may be given positionally or by keyword, and every other keyword is
	// a pretty-printer option, so the two are separated by hand.
	if len(args) < 1 || len(args) > 2 {
		return nil, py.ExceptionNewf(py.TypeError, "pprint() takes 1 or 2 positional arguments (%d given)", len(args))
	}
	object := args[0]
	var stream py.Object = py.None
	if len(args) == 2 {
		stream = args[1]
	}
	opts := py.StringDict{}
	for k, v := range kwargs {
		if k == "stream" {
			if len(args) == 2 {
				return nil, py.ExceptionNewf(py.TypeError, "pprint() got multiple values for argument 'stream'")
			}
			stream = v
			continue
		}
		opts[k] = v
	}
	p, err := newPrinter(opts)
	if err != nil {
		return nil, err
	}
	s, err := p.pformat(object)
	if err != nil {
		return nil, err
	}
	if stream == py.None {
		if stream, err = stdoutObject(self); err != nil {
			return nil, err
		}
	}
	if err := writeTo(stream, s+"\n"); err != nil {
		return nil, err
	}
	return py.None, nil
}

// ppFn is pp(), which is pprint() with sort_dicts defaulting to False.
func ppFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if _, ok := kwargs["sort_dicts"]; !ok {
		kw := py.StringDict{}
		for k, v := range kwargs {
			kw[k] = v
		}
		kw["sort_dicts"] = py.Bool(false)
		kwargs = kw
	}
	return pprintFn(self, args, kwargs)
}

// isReadableFn and isRecursiveFn expose CPython's isreadable/isrecursive.
func isReadableFn(self py.Object, args py.Tuple) (py.Object, error) {
	var object py.Object
	if err := py.UnpackTuple(args, nil, "isreadable", 1, 1, &object); err != nil {
		return nil, err
	}
	p, err := newPrinter(py.StringDict{})
	if err != nil {
		return nil, err
	}
	if _, err := p.pformat(object); err != nil {
		return nil, err
	}
	return py.NewBool(p.readable && !p.recursive), nil
}

func isRecursiveFn(self py.Object, args py.Tuple) (py.Object, error) {
	var object py.Object
	if err := py.UnpackTuple(args, nil, "isrecursive", 1, 1, &object); err != nil {
		return nil, err
	}
	p, err := newPrinter(py.StringDict{})
	if err != nil {
		return nil, err
	}
	if _, err := p.pformat(object); err != nil {
		return nil, err
	}
	return py.NewBool(p.recursive), nil
}

// ---------------------------------------------------------------------------
// PrettyPrinter class

var prettyPrinterType = py.NewTypeX("pprint.PrettyPrinter",
	`PrettyPrinter(indent=1, width=80, depth=None, stream=None, *, compact=False, sort_dicts=True, underscore_numbers=False)

Handle pretty-printing operations onto a stream using a configured set of
formatting parameters.`,
	prettyPrinterNew, nil)

// PrettyPrinter is the Python-visible printer object.
type PrettyPrinter struct {
	p      *printer
	stream py.Object
}

func (pp *PrettyPrinter) Type() *py.Type { return prettyPrinterType }

func prettyPrinterNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// indent, width, depth and stream are positional-or-keyword; the rest are
	// keyword-only.
	opts := py.StringDict{}
	for k, v := range kwargs {
		opts[k] = v
	}
	var stream py.Object = py.None
	if v, ok := opts["stream"]; ok {
		stream = v
		delete(opts, "stream")
	}
	if len(args) > 4 {
		return nil, py.ExceptionNewf(py.TypeError, "PrettyPrinter() takes at most 4 positional arguments (%d given)", len(args))
	}
	names := []string{"indent", "width", "depth"}
	for i, a := range args {
		if i == 3 {
			stream = a
			continue
		}
		opts[names[i]] = a
	}
	p, err := newPrinter(opts)
	if err != nil {
		return nil, err
	}
	return &PrettyPrinter{p: p, stream: stream}, nil
}

// formatAndWrite renders object and writes it.
//
// A PrettyPrinter created without a stream writes to sys.stdout.  Its methods
// are bound with the printer as self, which does not carry a module context, so
// the module is taken from the Method the call arrived through.
func (pp *PrettyPrinter) formatAndWrite(self py.Object, object py.Object, newline bool) error {
	s, err := pp.p.pformat(object)
	if err != nil {
		return err
	}
	if pp.stream != py.None {
		s2 := s
		if newline {
			s2 += "\n"
		}
		return writeTo(pp.stream, s2)
	}
	mod, err := moduleFromMethod(self)
	if err != nil {
		return err
	}
	stdout, err := stdoutObject(mod)
	if err != nil {
		return err
	}
	if newline {
		s += "\n"
	}
	return writeTo(stdout, s)
}

// moduleFromMethod recovers the module that owns the method being called.
//
// The interpreter binds a class method's self to the instance, so the module is
// reached through the type's Dict, where NewModule stored the live Method with
// its Module pointer.
func moduleFromMethod(self py.Object) (py.Object, error) {
	d := self.Type().Dict
	for _, name := range []string{"pprint", "pformat"} {
		if m, ok := d[name].(*py.Method); ok && m.Module != nil {
			return m.Module, nil
		}
	}
	return nil, py.ExceptionNewf(py.RuntimeError, "pprint cannot find the module that owns this PrettyPrinter")
}

func init() {
	prettyPrinterType.Dict["pprint"] = py.MustNewMethod("pprint", func(self py.Object, args py.Tuple) (py.Object, error) {
		pp := self.(*PrettyPrinter)
		var object py.Object
		if err := py.UnpackTuple(args, nil, "pprint", 1, 1, &object); err != nil {
			return nil, err
		}
		if err := pp.formatAndWrite(self, object, true); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Pretty-print a Python object to the configured stream.")

	prettyPrinterType.Dict["pformat"] = py.MustNewMethod("pformat", func(self py.Object, args py.Tuple) (py.Object, error) {
		pp := self.(*PrettyPrinter)
		var object py.Object
		if err := py.UnpackTuple(args, nil, "pformat", 1, 1, &object); err != nil {
			return nil, err
		}
		s, err := pp.p.pformat(object)
		if err != nil {
			return nil, err
		}
		return py.String(s), nil
	}, 0, "Format a Python object into a pretty-printed representation.")

	prettyPrinterType.Dict["isreadable"] = py.MustNewMethod("isreadable", func(self py.Object, args py.Tuple) (py.Object, error) {
		pp := self.(*PrettyPrinter)
		var object py.Object
		if err := py.UnpackTuple(args, nil, "isreadable", 1, 1, &object); err != nil {
			return nil, err
		}
		if _, err := pp.p.pformat(object); err != nil {
			return nil, err
		}
		return py.NewBool(pp.p.readable && !pp.p.recursive), nil
	}, 0, "Return True if the object is readable by eval().")

	prettyPrinterType.Dict["isrecursive"] = py.MustNewMethod("isrecursive", func(self py.Object, args py.Tuple) (py.Object, error) {
		pp := self.(*PrettyPrinter)
		var object py.Object
		if err := py.UnpackTuple(args, nil, "isrecursive", 1, 1, &object); err != nil {
			return nil, err
		}
		if _, err := pp.p.pformat(object); err != nil {
			return nil, err
		}
		return py.NewBool(pp.p.recursive), nil
	}, 0, "Return True if the object requires a recursive representation.")

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "pprint",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("pformat", pformatFn, 0, "Format a Python object into a pretty-printed representation."),
			py.MustNewMethod("pprint", pprintFn, 0, "Pretty-print a Python object to a stream [default is sys.stdout]."),
			py.MustNewMethod("pp", ppFn, 0, "Pretty-print a Python object (with sort_dicts=False)."),
			py.MustNewMethod("isreadable", isReadableFn, 0, "Determine if saferepr(object) is readable by eval()."),
			py.MustNewMethod("isrecursive", isRecursiveFn, 0, "Determine if object requires a recursive representation."),
		},
		Globals: py.StringDict{
			"PrettyPrinter": prettyPrinterType,
		},
	})
}
