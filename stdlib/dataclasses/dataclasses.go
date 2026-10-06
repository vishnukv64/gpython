// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dataclasses provides the implementation of python's 'dataclasses'
// module.
//
// The decorator reads the annotated fields of the class it is applied to and
// installs real __init__, __repr__ and __eq__ (and, for order=True, the four
// ordering methods) on the class.  The generated methods are ordinary
// callables, so they bind, can be called directly and can be overridden by
// the class itself.
//
// Field discovery needs the class's __annotations__.  This interpreter's
// compiler compiles the value of an annotated assignment but discards the
// annotation, so a class never carries __annotations__ - the one exception
// is a class body that sets it explicitly, which is honoured here.  Where
// the annotation dict cannot be read, the class's source is parsed to
// recover the annotated names and their order; the source of a class is
// located from the frame that called the decorator and the module's
// __file__.  A class defined by exec() of a string, or one whose source file
// is gone, therefore has no discoverable fields and is reported with an
// honest error rather than being silently mis-decorated.
package dataclasses

import (
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const dataclasses_doc = `Module for creating and manipulating dataclasses.

The dataclass decorator inspects the class for variables annotated at the
class level and uses those to add methods for initialization, representation
and comparison.`

// MISSING is the sentinel that marks a field with no default.
type missingType struct{}

var missing = missingType{}

var MissingType = py.NewType("dataclasses._MISSING_TYPE", "Sentinel to mark a missing value")

func (m missingType) Type() *py.Type { return MissingType }

func (m missingType) M__repr__() (py.Object, error) {
	return py.String("<dataclasses._MISSING_TYPE object>"), nil
}
func (m missingType) M__bool__() (py.Object, error) { return py.False, nil }

// KW_ONLY is the sentinel that marks the following fields keyword-only.
type kwOnlyType struct{}

var kwOnly = kwOnlyType{}

var KWOnlyType = py.NewType("dataclasses._KW_ONLY_TYPE", "Sentinel to mark fields as keyword only")

func (k kwOnlyType) Type() *py.Type { return KWOnlyType }

func (k kwOnlyType) M__repr__() (py.Object, error) {
	return py.String("<dataclasses._KW_ONLY_TYPE object>"), nil
}

// FrozenInstanceError is raised when a frozen instance is assigned to.
//
// The base type is readied first: NewType records the base but leaves the MRO
// computation to the interpreter's deferred pass, and a subclass made before
// its base is ready does not answer an isinstance/except check against that
// base.
var FrozenInstanceError = func() *py.Type {
	if err := py.AttributeError.Ready(); err != nil {
		panic(err)
	}
	t := py.AttributeError.NewType("dataclasses.FrozenInstanceError",
		"Raised when an implicitly defined __setattr__ or __delattr__ is called on a class instance.", nil, nil)
	if err := t.Ready(); err != nil {
		panic(err)
	}
	return t
}()

// fieldType records one field of a dataclass, mirroring dataclasses.Field.
type fieldType struct {
	name           string
	typ            py.Object
	defaultVal     py.Object
	defaultFactory py.Object
	init           bool
	repr           bool
	hash           py.Object
	compare        bool
	metadata       py.Object
	kwOnly         py.Object
}

var FieldType = py.NewType("dataclasses.Field", "Instance of Field describes a variable in a dataclass.")

func (f *fieldType) Type() *py.Type { return FieldType }

func (f *fieldType) M__repr__() (py.Object, error) {
	return py.String(fmt.Sprintf("Field(name=%s,type=%s,default=%s,default_factory=%s,init=%t,repr=%t,hash=%s,compare=%t,metadata=%s,kw_only=%s)",
		reprOrEmpty(py.String(f.name)), reprOrEmpty(f.typ),
		reprOrEmpty(f.defaultVal), reprOrEmpty(f.defaultFactory),
		f.init, f.repr, reprOrEmpty(f.hash), f.compare,
		reprOrEmpty(f.metadata), reprOrEmpty(f.kwOnly))), nil
}

const field_doc = `field(*, default=MISSING, default_factory=MISSING, init=True, repr=True,
      hash=None, compare=True, metadata=None, kw_only=MISSING)

Return an object to identify dataclass fields.

default is the default value of the field.  default_factory is a 0-argument
function called when a default value is needed for this field.  If both
default and default_factory are specified, a ValueError is raised.`

func field(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		defaultVal     py.Object = missing
		defaultFactory py.Object = missing
		init           py.Object = py.True
		reprObj        py.Object = py.True
		hashObj        py.Object = py.None
		compare        py.Object = py.True
		metadata       py.Object = py.None
		kwOnlyObj      py.Object = missing
	)
	kwlist := []string{"default", "default_factory", "init", "repr", "hash", "compare", "metadata", "kw_only"}
	if err := py.ParseTupleAndKeywords(nil, kwargs, "|$OOOOOOOO:field", kwlist,
		&defaultVal, &defaultFactory, &init, &reprObj, &hashObj, &compare, &metadata, &kwOnlyObj); err != nil {
		return nil, err
	}
	if defaultVal != missing && defaultFactory != missing {
		return nil, py.ExceptionNewf(py.ValueError, "cannot specify both default and default_factory")
	}
	f := &fieldType{
		defaultVal:     defaultVal,
		defaultFactory: defaultFactory,
		hash:           hashObj,
		metadata:       metadata,
		kwOnly:         kwOnlyObj,
	}
	var err error
	if f.init, err = py.ObjectIsTrue(init); err != nil {
		return nil, err
	}
	if f.repr, err = py.ObjectIsTrue(reprObj); err != nil {
		return nil, err
	}
	if f.compare, err = py.ObjectIsTrue(compare); err != nil {
		return nil, err
	}
	if f.metadata == nil {
		f.metadata = py.NewStringDict()
	}
	if f.kwOnly == nil {
		f.kwOnly = missing
	}
	return f, nil
}

// initField fills in the name and type of a field once they are known.
func (f *fieldType) setIdentity(name string, typ py.Object) {
	f.name = name
	f.typ = typ
}

// clone produces the per-class copy of a field so that two classes sharing
// one field() object do not share the resolved name.
func (f *fieldType) clone() *fieldType {
	c := *f
	c.normalize()
	return &c
}

// normalize fills in the sentinels a field built by hand leaves unset, so
// that a nil never has to be distinguished from MISSING.
func (f *fieldType) normalize() {
	if f.defaultVal == nil {
		f.defaultVal = missing
	}
	if f.defaultFactory == nil {
		f.defaultFactory = missing
	}
	if f.hash == nil {
		f.hash = py.None
	}
	if f.metadata == nil {
		f.metadata = py.NewStringDict()
	}
	if f.kwOnly == nil {
		f.kwOnly = missing
	}
}

const MISSING_doc = `Sentinel object to detect if a parameter is supplied or not.`

// annotationFields recovers the fields of a class.
//
// The compiler of this interpreter drops the annotation attached to an
// annotated assignment, so a class body's __annotations__ is almost never
// populated.  When it is - a class that assigns it explicitly - that is the
// authoritative list.  Otherwise the class's source is parsed.
// annotationOrder returns the annotated names in DECLARATION order, read from
// the class's source.
//
// It returns nil when the source cannot be read - a class built by exec, or one
// whose file has gone - and the caller then falls back to the map's own order.
// Returning nil rather than an error is deliberate: the field TYPES come from
// __annotations__ and are always available, so a dataclass should still be
// buildable when only the order has to be guessed at.
func annotationOrder(cls *py.Type, decoFrame *py.Frame) []string {
	src, err := classSource(cls, decoFrame)
	if err != nil {
		return nil
	}
	parsed, err := parseClassFields(src, cls.Name)
	if err != nil || len(parsed) == 0 {
		return nil
	}
	names := make([]string, 0, len(parsed))
	for _, pf := range parsed {
		names = append(names, pf.name)
	}
	return names
}

func annotationFields(cls *py.Type, decoFrame *py.Frame) ([]*fieldType, error) {
	ann := cls.Dict.GetOrNil("__annotations__")
	var names []string
	var types []py.Object
	if ann != nil {
		d, ok := ann.(py.IGetDict)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "__annotations__ must be a dict")
		}
		// __annotations__ is a Go map, whose iteration order is not the
		// declaration order, so the source is consulted for the ORDER and the
		// annotation object supplies the types.
		//
		// This used to SORT the names, which is not the same thing at all: a
		// dataclass's field order decides its __init__ signature, and sorting
		// reordered it.  "zeta, alpha, mid" became "alpha, mid, zeta", and
		// rich's ConsoleOptions ended up with its defaulted field before a
		// required one - so the class raised "non-default argument follows
		// default argument" and pip could not import rich at all.
		types = []py.Object{}
		order := annotationOrder(cls, decoFrame)
		if len(order) == 0 {
			// No source to read - "-c" and anything compiled from a string
			// have no file to scan.  The annotation DICT is the authority:
			// a StringDict keeps insertion order, which for a class body is
			// declaration order.  This used to SORT the names, which is not
			// the same thing at all and threw away the very ordering the
			// source scan exists to recover - so a class with a defaulted
			// field declined before a required one raised "non-default
			// argument follows default argument" under "-c" while working
			// from a file.
			order = d.GetDict().Keys()
		}
		seen := map[string]bool{}
		for _, k := range order {
			if _, ok := d.GetDict().Get(k); !ok {
				continue
			}
			names = append(names, k)
			types = append(types, d.GetDict().GetOrNil(k))
			seen[k] = true
		}
		// Anything the source scan missed is still a field, so it is appended
		// rather than dropped - losing a field silently is worse than an
		// arbitrary position for it.
		for _, k := range sortedKeys(d.GetDict()) {
			if seen[k] {
				continue
			}
			names = append(names, k)
			types = append(types, d.GetDict().GetOrNil(k))
		}
	}
	// kwOnlyFromSource records, per name, whether the source showed a
	// "_: KW_ONLY" line before it - the sentinel itself never survives into
	// the class dictionary, because the compiler drops annotated
	// assignments, so the source is the only place it can be seen.
	kwOnlyFromSource := map[string]bool{}
	if len(names) == 0 {
		src, err := classSource(cls, decoFrame)
		if err != nil {
			return nil, err
		}
		parsed, err := parseClassFields(src, cls.Name)
		if err != nil {
			return nil, err
		}
		for _, pf := range parsed {
			names = append(names, pf.name)
			types = append(types, py.None)
			if pf.kwOnly {
				kwOnlyFromSource[pf.name] = true
			}
		}
	}

	fields := make([]*fieldType, 0, len(names))
	kwOnlySeen := false
	for i, name := range names {
		if kwOnlyFromSource[name] {
			kwOnlySeen = true
		}
		if value, ok := cls.Dict.Get(name); ok {
			if _, isKwOnly := value.(kwOnlyType); isKwOnly {
				kwOnlySeen = true
				continue
			}
			if f, isField := value.(*fieldType); isField {
				nf := f.clone()
				nf.setIdentity(name, types[i])
				if kwOnlySeen && nf.kwOnly == missing {
					nf.kwOnly = py.True
				}
				fields = append(fields, nf)
				continue
			}
			// A plain class attribute is the default value.
			nf := &fieldType{
				name:       name,
				typ:        types[i],
				defaultVal: value,
				init:       true,
				repr:       true,
				hash:       py.None,
				compare:    true,
				metadata:   py.NewStringDict(),
				kwOnly:     missing,
			}
			if kwOnlySeen {
				nf.kwOnly = py.True
			}
			fields = append(fields, nf)
			continue
		}
		// No value in the class: a bare annotation, so the field is
		// required.
		nf := &fieldType{
			name:       name,
			typ:        types[i],
			defaultVal: missing,
			init:       true,
			repr:       true,
			hash:       py.None,
			compare:    true,
			metadata:   py.NewStringDict(),
			kwOnly:     missing,
		}
		if kwOnlySeen {
			nf.kwOnly = py.True
		}
		fields = append(fields, nf)
	}
	return fields, nil
}

func sortedKeys(d py.StringDict) []string {
	keys := make([]string, 0, d.Len())
	keys = append(keys, d.Keys()...)
	// There is no declaration order to recover from a map, so the names are
	// ordered so that the result is at least stable.
	sortStrings(keys)
	return keys
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// classSource locates the source text of a class.  It uses the frame that
// called the decorator, whose code object names the file and whose current
// line is the decorator line.
func classSource(cls *py.Type, decoFrame *py.Frame) (string, error) {
	if decoFrame == nil {
		return "", py.ExceptionNewf(py.ValueError, "dataclass %s: cannot locate its source to read its fields", cls.Name)
	}
	file := decoFrame.Code.Filename
	if file == "" || file == "<string>" {
		return "", py.ExceptionNewf(py.ValueError,
			"dataclass %s: its source is not a file (it was defined in %q), so its fields cannot be discovered; add an explicit __annotations__",
			cls.Name, file)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", py.ExceptionNewf(py.ValueError,
			"dataclass %s: cannot read %s to discover its fields: %s", cls.Name, file, err)
	}
	return string(data), nil
}

// parsedField is one annotated name found by scanning source.
type parsedField struct {
	name     string
	kwOnly   bool
	hasValue bool
}

// classHeader finds the "class <name>(...):" header of the named class and
// returns the body text and the indent of the body.
func classHeader(src, clsName string) (body string, indent string, ok bool) {
	// Annotations inside the body start at the body's indentation.
	headerRe := regexp.MustCompile(`(?m)^[ \t]*class[ \t]+` + regexp.QuoteMeta(clsName) + `\b[^\n]*:`)
	loc := headerRe.FindStringIndex(src)
	if loc == nil {
		return "", "", false
	}
	// The body runs until a line at the header's own indentation that is not
	// blank.
	headerIndent := leadingIndent(src[loc[0]:])
	rest := src[loc[1]:]
	lines := strings.Split(rest, "\n")
	var bodyLines []string
	bodyIndent := ""
	for i, line := range lines {
		if i == 0 {
			// Text after the colon on the same line is part of the header.
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			bodyLines = append(bodyLines, line)
			continue
		}
		ind := leadingIndent(line)
		if len(ind) <= len(headerIndent) {
			break
		}
		if bodyIndent == "" {
			bodyIndent = ind
		}
		if len(ind) < len(bodyIndent) {
			break
		}
		bodyLines = append(bodyLines, line)
	}
	return strings.Join(bodyLines, "\n"), bodyIndent, true
}

func leadingIndent(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[:i]
}

// annotationRe matches the annotated assignments in a class body, capturing
// the name and everything after the colon so that the initialiser can be
// seen.  It deliberately does not try to be a parser: the shape it has to
// recognise is "name: type" and "name: type = value", and a line it cannot
// match is skipped rather than mis-read.
var annotationRe = regexp.MustCompile(`^(\w+)\s*:\s*(.+)$`)

// parseClassFields scans the class body, returning the annotated names in
// declaration order.  A "_: KW_ONLY" annotation turns on keyword-only mode
// for the rest of the body, which is what "_: KW_ONLY" means.
func parseClassFields(src, clsName string) ([]parsedField, error) {
	body, indent, ok := classHeader(src, clsName)
	if !ok {
		return nil, py.ExceptionNewf(py.ValueError, "dataclass %s: cannot find its class statement in its source", clsName)
	}
	var fields []parsedField
	kwOnly := false
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Only statements at the body's OWN indentation are fields.  A nested
		// def or class is indented deeper, and its own locals, parameters and
		// return annotation are NOT fields of this class.
		//
		// This check was written but its body was empty - the indent was bound
		// to "_" - so the scan ran straight through method definitions and
		// collected them as fields.  For rich's ConsoleOptions that appended
		// "ConsoleOptions" (from a method's return annotation), "options", and
		// the method's own parameters, which pushed the required fields
		// "is_terminal" and "encoding" AFTER defaulted ones and made the whole
		// class raise "non-default argument follows default argument".
		if leadingIndent(line) != indent {
			continue
		}
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "@") || strings.HasPrefix(stripped, "def ") ||
			strings.HasPrefix(stripped, "class ") || strings.HasPrefix(stripped, "#") {
			continue
		}
		m := annotationRe.FindStringSubmatch(stripped)
		if m == nil {
			continue
		}
		name, rest := m[1], m[2]
		if strings.Contains(name, "=") {
			continue
		}
		// A ":" inside the type (a slice or a dict) is part of the type.
		hasValue := false
		if idx := topLevelAssign(rest); idx >= 0 {
			hasValue = true
			_ = idx
		}
		if name == "_" && strings.Contains(rest, "KW_ONLY") {
			kwOnly = true
			continue
		}
		fields = append(fields, parsedField{name: name, kwOnly: kwOnly, hasValue: hasValue})
	}
	return fields, nil
}

// topLevelAssign returns the index of the "=" that separates the annotation
// from the default value, or -1.  A "=" inside brackets is part of the type
// (a dict literal or an Annotated[...] call), not the assignment.
func topLevelAssign(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '=':
			if depth == 0 {
				if i+1 < len(s) && s[i+1] == '=' {
					i++
					continue
				}
				if i > 0 && (s[i-1] == '!' || s[i-1] == '<' || s[i-1] == '>' || s[i-1] == '=') {
					continue
				}
				return i
			}
		}
	}
	return -1
}

const dataclass_doc = `dataclass(cls=None, /, *, init=True, repr=True, eq=True, order=False,
              unsafe_hash=False, frozen=False, match_args=True, kw_only=False,
              slots=False, weakref_slot=False)

Returns the same class as was passed in, with dunder methods added based on
the fields defined in the class.

If init is true (the default), a __init__() method will be added.
If repr is true (the default), a __repr__() method will be added.
If eq is true (the default), an __eq__() method will be added.
If order is true, __lt__(), __le__(), __gt__(), and __ge__() methods will be
added.
If frozen is true, assigning to fields will generate an exception.
If unsafe_hash is true, a __hash__() method will be added.`

func dataclass(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		cls        py.Object
		initObj    py.Object = py.True
		reprObj    py.Object = py.True
		eqObj      py.Object = py.True
		orderObj   py.Object = py.False
		unsafeHash py.Object = py.False
		frozenObj  py.Object = py.False
		matchArgs  py.Object = py.True
		kwOnlyObj  py.Object = py.False
		slotsObj   py.Object = py.False
		weakrefObj py.Object = py.False
	)
	kwlist := []string{"cls", "init", "repr", "eq", "order", "unsafe_hash", "frozen", "match_args", "kw_only", "slots", "weakref_slot"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O$OOOOOOOOOO:dataclass", kwlist,
		&cls, &initObj, &reprObj, &eqObj, &orderObj, &unsafeHash, &frozenObj,
		&matchArgs, &kwOnlyObj, &slotsObj, &weakrefObj); err != nil {
		return nil, err
	}

	// A keyword-only "cls" also means the bare form was used.
	if cls == nil || cls == py.None {
		opts := &decoOpts{init: initObj, repr: reprObj, eq: eqObj, order: orderObj,
			unsafeHash: unsafeHash, frozen: frozenObj, kwOnly: kwOnlyObj}
		decorator := py.MustNewMethod("dataclass_decorator", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			if err := py.UnpackTuple(args, kwargs, "dataclass", 1, 1, &cls); err != nil {
				return nil, err
			}
			return applyDataclass(args[0], opts, callerFrame(self))
		}, 0, dataclass_doc)
		// The decorator is called by the class statement, so it has to know
		// the module it was created through: that is what locates the class
		// source.
		if m, ok := self.(*py.Module); ok {
			decorator.Module = m
		}
		return decorator, nil
	}
	opts := &decoOpts{init: initObj, repr: reprObj, eq: eqObj, order: orderObj,
		unsafeHash: unsafeHash, frozen: frozenObj, kwOnly: kwOnlyObj}
	return applyDataclass(cls, opts, callerFrame(self))
}

type decoOpts struct {
	init, repr, eq, order, unsafeHash, frozen, kwOnly py.Object
}

// callerFrame is the frame of the code that called a dataclasses method.
// It is needed to attribute the decorated class to its own decorator line.
func callerFrame(self py.Object) *py.Frame {
	m, ok := self.(*py.Module)
	if !ok || m.Context == nil {
		return nil
	}
	// A Go method runs no bytecode of its own, so the top of the frame
	// stack is already the Python code that called it - the class statement
	// being decorated.
	return py.CurrentFrame()
}

// frameContext is the context a generated method runs in, taken from the
// module the decoration happened through.
func applyDataclass(clsObj py.Object, opts *decoOpts, decoFrame *py.Frame) (py.Object, error) {
	cls, ok := clsObj.(*py.Type)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "dataclass() should be called on a class")
	}

	fields, err := annotationFields(cls, decoFrame)
	if err != nil {
		return nil, err
	}
	for _, f := range fields {
		f.normalize()
	}

	initOn, err := py.ObjectIsTrue(opts.init)
	if err != nil {
		return nil, err
	}
	reprOn, err := py.ObjectIsTrue(opts.repr)
	if err != nil {
		return nil, err
	}
	eqOn, err := py.ObjectIsTrue(opts.eq)
	if err != nil {
		return nil, err
	}
	orderOn, err := py.ObjectIsTrue(opts.order)
	if err != nil {
		return nil, err
	}
	hashOn, err := py.ObjectIsTrue(opts.unsafeHash)
	if err != nil {
		return nil, err
	}
	frozenOn, err := py.ObjectIsTrue(opts.frozen)
	if err != nil {
		return nil, err
	}
	// CPython generates __hash__ when unsafe_hash is set, and ALSO when the
	// class is frozen and compares by value - a frozen dataclass is immutable,
	// so it is safely hashable.  Only unsafe_hash was consulted here, so EVERY
	// frozen dataclass came out unhashable and could not key a dict; h11's
	// events are "@dataclass(init=False, frozen=True)" and its whole state
	// machine is keyed by them.
	if !hashOn {
		hashOn = eqOn && frozenOn
	}

	// "_: KW_ONLY" anywhere in the body, or kw_only=True on the decorator,
	// makes every field keyword-only.
	kwAll, err := py.ObjectIsTrue(opts.kwOnly)
	if err != nil {
		return nil, err
	}
	if kwAll {
		for _, f := range fields {
			f.kwOnly = py.True
		}
	}

	// A field with a default may not be followed by one without, which is
	// the same ordering rule CPython enforces.
	seenDefault := false
	for _, f := range fields {
		hasDefault := f.defaultVal != missing || f.defaultFactory != missing
		if hasDefault {
			seenDefault = true
		} else if seenDefault {
			return nil, py.ExceptionNewf(py.TypeError,
				"non-default argument %q follows default argument", f.name)
		}
	}

	if initOn {
		hasInit := cls.Dict.GetOrNil("__init__") != nil
		if !hasInit {
			cls.Dict.Set("__init__", py.MustNewMethod("__init__", makeInit(cls, fields, frozenOn), 0,
				"Generated __init__ for a dataclass."))
		}
	}
	if reprOn && cls.Dict.GetOrNil("__repr__") == nil {
		cls.Dict.Set("__repr__", py.MustNewMethod("__repr__", makeRepr(cls, fields), 0,
			"Generated __repr__ for a dataclass."))
	}
	if eqOn && cls.Dict.GetOrNil("__eq__") == nil {
		cls.Dict.Set("__eq__", py.MustNewMethod("__eq__", makeEq(cls, fields), 0,
			"Generated __eq__ for a dataclass."))
	}
	if orderOn {
		cls.Dict.Set("__lt__", py.MustNewMethod("__lt__", makeOrder(cls, fields, "<", false), 0, "Generated __lt__."))
		cls.Dict.Set("__le__", py.MustNewMethod("__le__", makeOrder(cls, fields, "<", true), 0, "Generated __le__."))
		cls.Dict.Set("__gt__", py.MustNewMethod("__gt__", makeOrder(cls, fields, ">", false), 0, "Generated __gt__."))
		cls.Dict.Set("__ge__", py.MustNewMethod("__ge__", makeOrder(cls, fields, ">", true), 0, "Generated __ge__."))
	}
	if hashOn {
		cls.Dict.Set("__hash__", py.MustNewMethod("__hash__", makeHash(cls, fields), 0, "Generated __hash__."))
	} else if eqOn && cls.Dict.GetOrNil("__hash__") == nil {
		// Defining __eq__ without __hash__ makes the class unhashable, as
		// CPython does.
		cls.Dict.Set("__hash__", py.None)
	}
	if frozenOn {
		cls.Dict.Set("__setattr__", py.MustNewMethod("__setattr__", frozenSetattr, 0,
			"Raise FrozenInstanceError on assignment to a frozen dataclass."))
		cls.Dict.Set("__delattr__", py.MustNewMethod("__delattr__", frozenDelattr, 0,
			"Raise FrozenInstanceError on deletion from a frozen dataclass."))
	}

	// The field table is kept on the class so that fields() and friends can
	// read it back.  The order lives in a separate list because this
	// interpreter's dict is a Go map, which does not preserve insertion
	// order.
	cls.Dict.Set("__dataclass_fields__", fieldsDict(fields))
	cls.Dict.Set("__dataclass_field_order__", fieldOrder(fields))
	cls.Dict.Set("__dataclass_params__", py.NewStringDict())
	return cls, nil
}

// fieldsDict exposes the fields as a dict keyed by name, which is the shape
// __dataclass_fields__ has.
func fieldsDict(fields []*fieldType) py.StringDict {
	d := py.NewStringDict()
	for _, f := range fields {
		d.Set(f.name, f)
	}
	return d
}

// initFields returns the fields that take part in __init__.
func initFields(fields []*fieldType) []*fieldType {
	var out []*fieldType
	for _, f := range fields {
		if f.init {
			out = append(out, f)
		}
	}
	return out
}

// compareFields returns the fields that take part in __eq__ and ordering.
func compareFields(fields []*fieldType) []*fieldType {
	var out []*fieldType
	for _, f := range fields {
		if f.compare {
			out = append(out, f)
		}
	}
	return out
}

// reprFields returns the fields that take part in __repr__.
func reprFields(fields []*fieldType) []*fieldType {
	var out []*fieldType
	for _, f := range fields {
		if f.repr {
			out = append(out, f)
		}
	}
	return out
}

// makeInit builds the __init__ that binds the fields in order.  Parameters
// with neither a default nor a default factory are required; one with a
// factory is looked up at call time so that a mutable default is fresh per
// instance.
func makeInit(cls *py.Type, fields []*fieldType, frozen bool) dataclassMethod {
	positional := initFields(fields)
	return func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		inst, args, err := instanceOf(cls, self, args, cls.Name+".__init__")
		if err != nil {
			return nil, err
		}
		self = inst
		seen := map[string]bool{}
		n := len(args)

		var positionalMax, minPositional int
		for _, f := range positional {
			isKwOnly := false
			if b, ok := f.kwOnly.(py.Bool); ok && bool(b) {
				isKwOnly = true
			}
			if isKwOnly {
				continue
			}
			if f.defaultVal == missing && f.defaultFactory == missing {
				positionalMax++
				minPositional++
			} else {
				positionalMax++
			}
		}
		if n > positionalMax {
			return nil, py.ExceptionNewf(py.TypeError, "%s.__init__() takes %d positional argument%s but %d %s given",
				cls.Name, positionalMax, plural(positionalMax), n, wasWere(n))
		}
		// A required field may be supplied BY KEYWORD instead of by position -
		// rich constructs ConsoleThreadLocals(theme_stack=...) - so counting
		// the positional arguments alone reported "missing 1 required
		// positional argument" for a call that supplied it.
		//
		// The default ordering rule means the required fields are exactly the
		// first minPositional of them, so each is satisfied by its position or
		// by its name; collect the ones that are satisfied by neither.
		var missingNames []string
		idx := 0
		for _, f := range positional {
			if b, ok := f.kwOnly.(py.Bool); ok && bool(b) {
				continue
			}
			if f.defaultVal != missing || f.defaultFactory != missing {
				break
			}
			if idx >= n {
				if _, ok := kwargs.Get(f.name); !ok {
					missingNames = append(missingNames, f.name)
				}
			}
			idx++
		}
		if len(missingNames) > 0 {
			quoted := make([]string, len(missingNames))
			for i, nm := range missingNames {
				quoted[i] = "'" + nm + "'"
			}
			return nil, py.ExceptionNewf(py.TypeError, "%s.__init__() missing %d required positional argument%s: %s",
				cls.Name, len(missingNames), plural(len(missingNames)), joinAnd(quoted))
		}

		i := 0
		for _, f := range positional {
			isKwOnly := false
			if b, ok := f.kwOnly.(py.Bool); ok && bool(b) {
				isKwOnly = true
			}
			var value py.Object
			if isKwOnly {
				value = missing
			} else if i < n {
				value = args[i]
				i++
			} else {
				value = missing
			}
			if v, ok := kwargs.Get(f.name); ok {
				if value != missing {
					return nil, py.ExceptionNewf(py.TypeError,
						"%s.__init__() got multiple values for argument '%s'", cls.Name, f.name)
				}
				value = v
				seen[f.name] = true
			}
			if value == missing {
				if f.defaultFactory != missing {
					v, err := py.Call(f.defaultFactory, nil, py.StringDict{})
					if err != nil {
						return nil, err
					}
					value = v
				} else if f.defaultVal != missing {
					value = f.defaultVal
				} else {
					return nil, py.ExceptionNewf(py.TypeError, "%s.__init__() missing 1 required keyword-only argument: '%s'",
						cls.Name, f.name)
				}
			}
			if err := setFieldValue(self, f.name, value); err != nil {
				return nil, err
			}
			seen[f.name] = true
		}

		// An unknown keyword is an error, not a silent attribute.
		known := map[string]bool{}
		for _, f := range positional {
			known[f.name] = true
		}
		for _, k := range kwargs.Keys() {
			if !known[k] {
				return nil, py.ExceptionNewf(py.TypeError, "%s.__init__() got an unexpected keyword argument '%s'", cls.Name, k)
			}
		}

		// __post_init__ runs last, and is how a dataclass computes derived
		// attributes.
		if cls.Dict.GetOrNil("__post_init__") != nil {
			if _, err := py.Call(bound(cls, self, "__post_init__"), nil, py.StringDict{}); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}
}

// joinAnd joins with ", " and a final " and ", which is how CPython lists the
// names of missing arguments: "'a' and 'b'".
func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func wasWere(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}

// makeRepr builds "Cls(field=value, ...)".
func makeRepr(cls *py.Type, fields []*fieldType) dataclassMethod {
	shown := reprFields(fields)
	return func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		inst, _, err := instanceOf(cls, self, args, cls.Name+".__repr__")
		if err != nil {
			return nil, err
		}
		self = inst
		var b strings.Builder
		b.WriteString(cls.Name)
		b.WriteString("(")
		for i, f := range shown {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(f.name)
			b.WriteString("=")
			v, err := py.GetAttrString(self, f.name)
			if err != nil {
				return nil, err
			}
			text, err := py.ReprAsString(v)
			if err != nil {
				return nil, err
			}
			b.WriteString(text)
		}
		b.WriteString(")")
		return py.String(b.String()), nil
	}
}

// makeEq compares the class-identity and then each compare field with the
// interpreter's own "==".
func makeEq(cls *py.Type, fields []*fieldType) dataclassMethod {
	cmp := compareFields(fields)
	return func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		self, args, err := instanceOf(cls, self, args, cls.Name+".__eq__")
		if err != nil {
			return nil, err
		}
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "__eq__() takes exactly 1 argument (%d given)", len(args))
		}
		if err != nil {
			return nil, err
		}
		other := args[0]
		// A generated __eq__ must not rely on the interpreter's "==" for the
		// identity shortcut: "==" on two instances does not reach __eq__ in
		// this interpreter, so identity is compared explicitly.
		if other == self {
			return py.True, nil
		}
		// Only instances of the same class compare equal, as dataclasses
		// specifies.
		if other.Type() != self.Type() {
			return py.NotImplemented, nil
		}
		for _, f := range cmp {
			a, err := py.GetAttrString(self, f.name)
			if err != nil {
				return nil, err
			}
			b, err := py.GetAttrString(other, f.name)
			if err != nil {
				return nil, err
			}
			eq, err := py.Eq(a, b)
			if err != nil {
				return nil, err
			}
			same, err := py.ObjectIsTrue(eq)
			if err != nil {
				return nil, err
			}
			if !same {
				return py.False, nil
			}
		}
		return py.True, nil
	}
}

// makeOrder builds one ordering method.  It finds the first pair of compare
// fields that differ and applies the field comparison; when every field is
// equal, "le"/"ge" are true and "lt"/"gt" are false.
func makeOrder(cls *py.Type, fields []*fieldType, op string, orEqual bool) dataclassMethod {
	cmp := compareFields(fields)
	name := op
	return func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		self, args, err := instanceOf(cls, self, args, cls.Name+"."+name)
		if err != nil {
			return nil, err
		}
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "%s() takes exactly 1 argument (%d given)", name, len(args))
		}
		if err != nil {
			return nil, err
		}
		other := args[0]
		if other.Type() != self.Type() {
			return py.NotImplemented, nil
		}
		for _, f := range cmp {
			a, err := py.GetAttrString(self, f.name)
			if err != nil {
				return nil, err
			}
			b, err := py.GetAttrString(other, f.name)
			if err != nil {
				return nil, err
			}
			eq, err := py.Eq(a, b)
			if err != nil {
				return nil, err
			}
			same, err := py.ObjectIsTrue(eq)
			if err != nil {
				return nil, err
			}
			if same {
				continue
			}
			var res py.Object
			if op == "<" {
				res, err = py.Lt(a, b)
			} else {
				res, err = py.Gt(a, b)
			}
			if err != nil {
				return nil, err
			}
			return res, nil
		}
		return py.MakeBool(py.Bool(orEqual))
	}
}

// makeHash hashes the tuple of compare fields, which is what dataclasses
// does for an unsafe_hash.
func makeHash(cls *py.Type, fields []*fieldType) dataclassMethod {
	cmp := compareFields(fields)
	return func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		inst, _, err := instanceOf(cls, self, args, cls.Name+".__hash__")
		if err != nil {
			return nil, err
		}
		self = inst
		items := make(py.Tuple, 0, len(cmp))
		for _, f := range cmp {
			v, err := py.GetAttrString(self, f.name)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		return hashTuple(items)
	}
}

func frozenSetattr(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "__setattr__() takes exactly 2 arguments (%d given)", len(args))
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	return nil, py.ExceptionNewf(FrozenInstanceError, "cannot assign to field %q", name)
}

func frozenDelattr(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__delattr__() takes exactly 1 argument (%d given)", len(args))
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	return nil, py.ExceptionNewf(FrozenInstanceError, "cannot delete field %q", name)
}

const fields_doc = `fields(class_or_instance)

Return a tuple describing the fields of this dataclass.

Accepts a dataclass or an instance of one.  Tuple elements are Field objects.`

func fields(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	obj, err := oneArg(args, kwargs, "fields")
	if err != nil {
		return nil, err
	}
	fs, err := classFields(obj)
	if err != nil {
		return nil, err
	}
	t := make(py.Tuple, 0, len(fs))
	for _, f := range fs {
		t = append(t, f)
	}
	return t, nil
}

// setFieldValue stores a field on a fresh instance.
//
// This is what object.__setattr__ does in CPython: the generated __init__
// must be able to fill in the fields of a frozen dataclass, and CPython
// achieves that by writing through object.__setattr__ rather than the
// class's own __setattr__.  The interpreter has no object.__setattr__, so
// the instance dictionary is written directly instead.  An instance here is
// itself a *py.Type whose Dict holds its attributes.
func setFieldValue(self py.Object, name string, value py.Object) error {
	if inst, ok := self.(*py.Type); ok {
		if inst.Dict.IsNil() {
			inst.Dict = py.NewStringDict()
		}
		inst.Dict.Set(name, value)
		return nil
	}
	_, err := py.SetAttrString(self, name, value)
	return err
}

// fieldOrder records the declaration order of the fields, which the dict
// backing __dataclass_fields__ cannot.
func fieldOrder(fields []*fieldType) *py.List {
	l := py.NewList()
	for _, f := range fields {
		l.Append(py.String(f.name))
	}
	return l
}

// classFields reads __dataclass_fields__ off a class or an instance.
//
// An instance of a Python class in this interpreter is itself a *py.Type -
// the class allocator hands one back and the instance's attributes live in
// its own Dict - so "obj.(*py.Type)" does not tell a class from an instance.
// The field table has to be looked for on the object itself first (the class
// case) and then on its type (the instance case).
func classFields(obj py.Object) ([]*fieldType, error) {
	d := dataclassFields(obj)
	if d == nil {
		return nil, py.ExceptionNewf(py.TypeError, "must be called with a dataclass type or instance")
	}
	dict, ok := d.(py.StringDict)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "must be called with a dataclass type or instance")
	}
	if order := dataclassFieldOrder(obj); order != nil {
		fs := make([]*fieldType, 0, len(order.Items))
		for _, nameObj := range order.Items {
			name, err := py.StrAsString(nameObj)
			if err != nil {
				return nil, err
			}
			f, ok := dict.GetOrNil(name).(*fieldType)
			if !ok {
				continue
			}
			fs = append(fs, f)
		}
		return fs, nil
	}
	var fs []*fieldType
	for _, v := range dict.Values() {
		f, ok := v.(*fieldType)
		if !ok {
			continue
		}
		fs = append(fs, f)
	}
	return fs, nil
}

// dataclassFieldOrder finds the recorded declaration order of a class's
// fields, or nil when there is none.
func dataclassFieldOrder(obj py.Object) *py.List {
	if t, ok := obj.(*py.Type); ok {
		if l, ok := t.Dict.GetOrNil("__dataclass_field_order__").(*py.List); ok {
			return l
		}
	}
	if l, ok := obj.Type().Dict.GetOrNil("__dataclass_field_order__").(*py.List); ok {
		return l
	}
	return nil
}

const is_dataclass_doc = `is_dataclass(class_or_instance)

Return True if obj is a dataclass or an instance of a dataclass.`

func is_dataclass(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	obj, err := oneArg(args, kwargs, "is_dataclass")
	if err != nil {
		return nil, err
	}
	return py.Bool(dataclassFields(obj) != nil), nil
}

const asdict_doc = `asdict(obj, *, dict_factory=dict)

Return the fields of a dataclass instance as a new dictionary mapping field
names to field values.  Lists, tuples and dicts are recursed into.`

func asdict(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		obj     py.Object
		factory py.Object = py.None
	)
	kwlist := []string{"obj", "dict_factory"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|$O:asdict", kwlist, &obj, &factory); err != nil {
		return nil, err
	}
	return asdictImpl(obj, factory, 0)
}

func asdictImpl(obj, factory py.Object, depth int) (py.Object, error) {
	if depth > 64 {
		return nil, py.ExceptionNewf(py.ValueError, "recursion limit exceeded in asdict")
	}
	fs, err := classFields(obj)
	if err != nil {
		return nil, err
	}
	out := py.NewStringDict()
	for _, f := range fs {
		v, err := py.GetAttrString(obj, f.name)
		if err != nil {
			return nil, err
		}
		v, err = asdictValue(v, factory, depth)
		if err != nil {
			return nil, err
		}
		out.Set(f.name, v)
	}
	if factory != nil && factory != py.None {
		return py.Call(factory, py.Tuple{out}, py.StringDict{})
	}
	return out, nil
}

// asdictValue recurses into the containers dataclasses specifies.
func asdictValue(v, factory py.Object, depth int) (py.Object, error) {
	switch tv := v.(type) {
	case *py.List:
		l := tv
		out := py.NewList()
		for _, item := range l.Items {
			conv, err := asdictValue(item, factory, depth+1)
			if err != nil {
				return nil, err
			}
			out.Append(conv)
		}
		return out, nil
	case py.Tuple:
		t := tv
		out := make(py.Tuple, 0, len(t))
		for _, item := range t {
			conv, err := asdictValue(item, factory, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, conv)
		}
		return out, nil
	}
	if d, ok := v.(py.StringDict); ok {
		out := py.NewStringDict()
		for _, __e := range d.Items() {
			k := __e.Key
			item := __e.Value

			conv, err := asdictValue(item, factory, depth+1)
			if err != nil {
				return nil, err
			}
			out.Set(k, conv)
		}
		return out, nil
	}
	if _, ok := v.Type().Dict.Get("__dataclass_fields__"); ok {
		return asdictImpl(v, factory, depth+1)
	}
	return v, nil
}

const astuple_doc = `astuple(obj, *, tuple_factory=tuple)

Return the fields of a dataclass instance as a new tuple of field values.
Lists, tuples and dicts are recursed into.`

func astuple(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		obj     py.Object
		factory py.Object = py.None
	)
	kwlist := []string{"obj", "tuple_factory"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|$O:astuple", kwlist, &obj, &factory); err != nil {
		return nil, err
	}
	out, err := astupleImpl(obj, 0)
	if err != nil {
		return nil, err
	}
	if factory != nil && factory != py.None {
		return py.Call(factory, py.Tuple{out}, py.StringDict{})
	}
	return out, nil
}

func astupleImpl(obj py.Object, depth int) (py.Object, error) {
	if depth > 64 {
		return nil, py.ExceptionNewf(py.ValueError, "recursion limit exceeded in astuple")
	}
	fs, err := classFields(obj)
	if err != nil {
		return nil, err
	}
	out := make(py.Tuple, 0, len(fs))
	for _, f := range fs {
		v, err := py.GetAttrString(obj, f.name)
		if err != nil {
			return nil, err
		}
		v, err = astupleValue(v, depth)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func astupleValue(v py.Object, depth int) (py.Object, error) {
	switch t := v.(type) {
	case *py.List:
		out := py.NewList()
		for _, item := range t.Items {
			conv, err := astupleValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			out.Append(conv)
		}
		return out, nil
	case py.Tuple:
		out := make(py.Tuple, 0, len(t))
		for _, item := range t {
			conv, err := astupleValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, conv)
		}
		return out, nil
	}
	if d, ok := v.(py.StringDict); ok {
		out := py.NewStringDict()
		for _, __e := range d.Items() {
			k := __e.Key
			item := __e.Value

			conv, err := astupleValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			out.Set(k, conv)
		}
		return out, nil
	}
	if _, ok := v.Type().Dict.Get("__dataclass_fields__"); ok {
		return astupleImpl(v, depth+1)
	}
	return v, nil
}

const replace_doc = `replace(obj, /, **changes)

Return a new object replacing specified fields with new values.`

func replace(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "replace() missing 1 required positional argument: 'obj'")
	}
	obj := args[0]
	fs, err := classFields(obj)
	if err != nil {
		return nil, err
	}
	cls := obj.Type()
	var positional []py.Object
	for _, f := range fs {
		if !f.init {
			continue
		}
		if v, ok := kwargs.Get(f.name); ok {
			positional = append(positional, v)
			continue
		}
		v, err := py.GetAttrString(obj, f.name)
		if err != nil {
			return nil, err
		}
		positional = append(positional, v)
	}
	for _, k := range kwargs.Keys() {
		known := false
		for _, f := range fs {
			if f.name == k {
				known = true
				break
			}
		}
		if !known {
			return nil, py.ExceptionNewf(py.TypeError, "replace() got an unexpected keyword argument %q", k)
		}
	}
	return py.Call(cls, py.Tuple(positional), py.StringDict{})
}

const make_dataclass_doc = `make_dataclass(cls_name, fields, *, bases=(), namespace=None,
                init=True, repr=True, eq=True, order=False, unsafe_hash=False,
                frozen=False, match_args=True, kw_only=False, slots=False,
                weakref_slot=False, module=None)

Create a new dataclass with name cls_name, fields as defined in fields,
base classes as given in bases, and initialized with a namespace as given
in namespace.`

func make_dataclass(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		nameObj   py.Object
		fieldsOb  py.Object
		bases     py.Object = py.None
		ns        py.Object = py.None
		initObj   py.Object = py.True
		reprObj   py.Object = py.True
		eqObj     py.Object = py.True
		orderObj  py.Object = py.False
		hashObj   py.Object = py.False
		frozen    py.Object = py.False
		kwOnly    py.Object = py.False
		matchArgs py.Object
		slotsObj  py.Object
		weakref   py.Object
		moduleObj py.Object
	)
	kwlist := []string{"cls_name", "fields", "bases", "namespace", "init", "repr", "eq",
		"order", "unsafe_hash", "frozen", "match_args", "kw_only", "slots", "weakref_slot", "module"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|$OOOOOOOOOOOOO:make_dataclass", kwlist,
		&nameObj, &fieldsOb, &bases, &ns, &initObj, &reprObj, &eqObj, &orderObj,
		&hashObj, &frozen, &matchArgs, &kwOnly, &slotsObj, &weakref, &moduleObj); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(nameObj)
	if err != nil {
		return nil, err
	}

	// The class namespace gets each field's default.
	namespace := py.NewStringDict()
	if ns != nil && ns != py.None {
		if d, ok := ns.(py.IGetDict); ok {
			for _, __e := range d.GetDict().Items() {
				k := __e.Key
				v := __e.Value
				namespace.Set(k, v)
			}
		}
	}
	ann := py.NewStringDict()
	iter, err := py.Iter(fieldsOb)
	if err != nil {
		return nil, err
	}
	for {
		item, err := nextItem(iter)
		if err != nil {
			if py.IsException(py.StopIteration, err) {
				break
			}
			return nil, err
		}
		// A field entry is a name on its own - "make_dataclass('X', ['a'])" -
		// or a (name, type) / even (name, type, Field) sequence.  Requiring a
		// sequence rejected the plain-string form that CPython accepts and
		// which is the common way to call this.
		//
		// The annotations are built in the same pass: walking the argument a
		// second time found a generator already exhausted.  A bare name is
		// annotated typing.Any, as CPython has it.
		switch v := item.(type) {
		case py.String:
			ann.Set(string(v), anyType())
		case py.Tuple:
			if len(v) < 2 || len(v) > 3 {
				return nil, py.ExceptionNewf(py.TypeError, "Invalid field: %s", reprOf(item))
			}
			fname, err := py.StrAsString(v[0])
			if err != nil {
				return nil, err
			}
			ann.Set(fname, v[1])
			if len(v) == 3 {
				namespace.Set(fname, v[2])
			}
		default:
			// CPython reaches this through len(item), so that is its message.
			return nil, py.ExceptionNewf(py.TypeError, "object of type '%s' has no len()", item.Type().Name)
		}
	}

	var baseList []py.Object
	if bases != nil && bases != py.None {
		bi, err := py.Iter(bases)
		if err != nil {
			return nil, err
		}
		for {
			b, err := nextItem(bi)
			if err != nil {
				if py.IsException(py.StopIteration, err) {
					break
				}
				return nil, err
			}
			baseList = append(baseList, b)
		}
	}

	cls, err := createType(name, baseList, namespace)
	if err != nil {
		return nil, err
	}
	opts := &decoOpts{init: initObj, repr: reprObj, eq: eqObj, order: orderObj,
		unsafeHash: hashObj, frozen: frozen, kwOnly: kwOnly}
	// A generated class has no source to scan, so the annotations built above
	// are installed directly, which annotationFields reads first.
	cls.Dict.Set("__annotations__", ann)
	return applyDataclass(cls, opts, nil)
}

// createType builds a new class with the given bases and namespace by
// calling type(name, bases, namespace), which is exactly what a class
// statement compiles to.
func createType(name string, bases []py.Object, namespace py.StringDict) (*py.Type, error) {
	baseTuple := make(py.Tuple, 0, len(bases))
	baseTuple = append(baseTuple, bases...)
	res, err := py.Call(py.TypeType, py.Tuple{py.String(name), baseTuple, namespace}, py.StringDict{})
	if err != nil {
		return nil, err
	}
	cls, ok := res.(*py.Type)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "type() did not return a class")
	}
	return cls, nil
}

// nextItem advances a Python iterator, so that make_dataclass can walk the
// field sequence without depending on the iterator's concrete type.
func nextItem(iter py.Object) (py.Object, error) {
	i, ok := iter.(py.I__next__)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "object is not an iterator")
	}
	return i.M__next__()
}

// hashTuple hashes a tuple with the interpreter's own hash function, which
// is what makes a generated __hash__ agree with equality.
func hashTuple(t py.Tuple) (py.Object, error) {
	builtins := py.GetModuleImplOrNil("builtins")
	if builtins == nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "dataclasses: builtins is not loaded")
	}
	fn := builtins.Globals.GetOrNil("hash")
	if fn == nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "dataclasses: builtins.hash is not available")
	}
	return py.Call(fn, py.Tuple{t}, py.StringDict{})
}

// dataclassFields finds the __dataclass_fields__ table of a class or
// instance, or nil when the object is not a dataclass.
func dataclassFields(obj py.Object) py.Object {
	if t, ok := obj.(*py.Type); ok {
		if d, ok := t.Dict.Get("__dataclass_fields__"); ok {
			return d
		}
	}
	if d, ok := obj.Type().Dict.Get("__dataclass_fields__"); ok {
		return d
	}
	return nil
}

// oneArg validates a method that takes exactly one argument and returns it.
func oneArg(args py.Tuple, kwargs py.StringDict, name string) (py.Object, error) {
	if kwargs.Len() != 0 {
		return nil, py.ExceptionNewf(py.TypeError, "%s() takes no keyword arguments", name)
	}
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "%s() takes exactly 1 argument (%d given)", name, len(args))
	}
	return args[0], nil
}

// bound looks up a name on a class and binds it to an instance, which is
// what calling a method on the instance does.
func bound(cls *py.Type, self py.Object, name string) py.Object {
	m := cls.Dict.GetOrNil(name)
	if m == nil {
		return py.None
	}
	if b, ok := m.(py.I__get__); ok {
		res, err := b.M__get__(self, cls)
		if err != nil {
			return py.None
		}
		return res
	}
	return m
}

// instanceOf resolves the instance a generated method applies to.
//
// A bound method - the normal path, "p.__repr__()" or attribute lookup -
// passes the instance as self.  One caller does not: the interpreter's
// object.__init__ looks a class's __init__ up and calls it with the instance
// prepended to the arguments without binding it, so self arrives as the
// method's module.  That case is recognised here rather than worked around
// by leaving the generated __init__ unbound.
func instanceOf(cls *py.Type, self py.Object, args py.Tuple, name string) (py.Object, py.Tuple, error) {
	_, isModule := self.(*py.Module)
	// "Cls.__eq__(a, b)" reaches the class-dict method with the class as self
	// and both operands in args, which is the unbound method call.
	unbound := isModule || self == py.Object(cls)
	if unbound {
		if len(args) == 0 {
			return nil, nil, py.ExceptionNewf(py.TypeError, "%s() missing self argument", name)
		}
		return args[0], args[1:], nil
	}
	return self, args, nil
}

// dataclassMethod is the shape the generated methods have.
type dataclassMethod = func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error)

// reprOrEmpty is ReprAsString with the error dropped, for building a Field
// repr where a failure to render one part should not raise.
func reprOrEmpty(o py.Object) string {
	// A field's type is unset until the decorator reads its annotation, and
	// "None" is what CPython's own Field repr shows for that state.
	if o == nil {
		return "None"
	}
	s, err := py.ReprAsString(o)
	if err != nil {
		return "?"
	}
	return s
}

func init() {
	MissingType.Dict.Set("__bool__", py.MustNewMethod("__bool__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.False, nil
	}, 0, "MISSING is false."))

	FieldType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*fieldType).name), nil },
		Doc:  "name of the field",
	})
	FieldType.Dict.Set("type", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*fieldType).typ, nil },
		Doc:  "type of the field",
	})
	FieldType.Dict.Set("default", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*fieldType).defaultVal, nil },
		Doc:  "default value of the field",
	})
	FieldType.Dict.Set("default_factory", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*fieldType).defaultFactory, nil },
		Doc:  "default factory of the field",
	})
	FieldType.Dict.Set("init", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.Bool(self.(*fieldType).init), nil
		},
		Doc: "whether the field takes part in __init__",
	})
	FieldType.Dict.Set("repr", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.Bool(self.(*fieldType).repr), nil
		},
		Doc: "whether the field takes part in __repr__",
	})
	FieldType.Dict.Set("compare", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.Bool(self.(*fieldType).compare), nil
		},
		Doc: "whether the field takes part in comparisons",
	})
	FieldType.Dict.Set("hash", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*fieldType).hash, nil },
		Doc:  "whether the field takes part in __hash__",
	})
	FieldType.Dict.Set("metadata", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*fieldType).metadata, nil },
		Doc:  "the field's metadata mapping",
	})
	FieldType.Dict.Set("kw_only", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*fieldType).kwOnly, nil },
		Doc:  "whether the field is keyword-only",
	})

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "dataclasses",
			Doc:  dataclasses_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("dataclass", dataclass, 0, dataclass_doc),
			py.MustNewMethod("field", field, 0, field_doc),
			py.MustNewMethod("fields", fields, 0, fields_doc),
			py.MustNewMethod("is_dataclass", is_dataclass, 0, is_dataclass_doc),
			py.MustNewMethod("asdict", asdict, 0, asdict_doc),
			py.MustNewMethod("astuple", astuple, 0, astuple_doc),
			py.MustNewMethod("replace", replace, 0, replace_doc),
			py.MustNewMethod("make_dataclass", make_dataclass, 0, make_dataclass_doc),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "MISSING", Value: missing},
			py.DictEntry{Key: "KW_ONLY", Value: kwOnly},
			py.DictEntry{Key: "Field", Value: FieldType},
			py.DictEntry{Key: "FrozenInstanceError", Value: FrozenInstanceError},
			py.DictEntry{Key: "__doc__", Value: py.String(dataclasses_doc)},
		),
	})
}

var _ = fs.ErrNotExist

// anyType is typing.Any, the annotation make_dataclass gives a field named
// without a type, as CPython does.
func anyType() py.Object {
	if m := py.GetModuleImplOrNil("typing"); m != nil {
		if a, err := py.GetAttrString(m, "Any"); err == nil {
			return a
		}
	}
	return py.None
}

func reprOf(o py.Object) string {
	s, err := py.ReprAsString(o)
	if err != nil {
		return "?"
	}
	return s
}
