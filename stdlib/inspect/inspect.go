// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package inspect provides the implementation of python's 'inspect' module.
//
// This covers the part of the module that code actually calls to look at
// functions and docstrings.  Signature introspection - inspect.signature and
// the parameter objects - is not implemented: it needs argument binding to
// be reflected back out of the function object, and the interpreter does not
// keep it in a form that can be reported.  Those names raise rather than
// returning something that looks right.
package inspect

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Get useful information from live Python objects.

This module provides some useful functions to help the user get information
about live objects such as modules, classes, methods, functions, tracebacks,
frame objects, and code objects.`

func init() {
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "currentframe", Value: py.MustNewMethod("currentframe", py.InternalMethodGetFrame, 0, currentframe_doc)},
		py.DictEntry{Key: "cleandoc", Value: py.MustNewMethod("cleandoc", inspectCleanDoc, 0, cleandoc_doc)},
		py.DictEntry{Key: "isfunction", Value: predicate(func(obj py.Object) bool { _, ok := obj.(*py.Function); return ok })},
		py.DictEntry{Key: "isgeneratorfunction", Value: py.MustNewMethod("isgeneratorfunction", inspectIsGeneratorFunction, 0, isgeneratorfunction_doc)},
		py.DictEntry{Key: "isgenerator", Value: predicate(func(obj py.Object) bool { _, ok := obj.(*py.Generator); return ok })},
		py.DictEntry{Key: "iscoroutine", Value: predicate(func(obj py.Object) bool { _, ok := obj.(*py.Coroutine); return ok })},
		py.DictEntry{Key: "isbuiltin", Value: predicate(func(obj py.Object) bool { _, ok := obj.(*py.Method); return ok })},
		py.DictEntry{Key: "ismethod", Value: predicate(func(obj py.Object) bool { _, ok := obj.(*py.Method); return ok })},
		py.DictEntry{Key: "ismodule", Value: predicate(func(obj py.Object) bool { _, ok := obj.(*py.Module); return ok })},
		// A python-level INSTANCE is also a *py.Type here (an empty Name is
		// what distinguishes it), so the bare type assertion reported every
		// instance as a class.  rich checks isclass before calling an object's
		// __rich_console__, so every Text was "not renderable".
		py.DictEntry{Key: "isclass", Value: predicate(py.IsClassObject)},
		py.DictEntry{Key: "isdatadescriptor", Value: predicate(func(obj py.Object) bool { _, ok := obj.(*py.Property); return ok })},
		py.DictEntry{Key: "getmro", Value: py.MustNewMethod("getmro", getmro, 0, getmro_doc)},
		py.DictEntry{Key: "getdoc", Value: py.MustNewMethod("getdoc", getdoc, 0, getdoc_doc)},
		py.DictEntry{Key: "signature", Value: py.MustNewMethod("signature", notImplemented, 0, signature_doc)},
	)

	// The frame constants are part of the module's interface.
	globals.Set("CO_GENERATOR", py.Int(py.CO_GENERATOR))
	globals.Set("CO_VARARGS", py.Int(py.CO_VARARGS))
	globals.Set("CO_VARKEYWORDS", py.Int(py.CO_VARKEYWORDS))
	globals.Set("CO_OPTIMIZED", py.Int(py.CO_OPTIMIZED))
	globals.Set("CO_NEWLOCALS", py.Int(py.CO_NEWLOCALS))
	globals.Set("CO_NOFREE", py.Int(py.CO_NOFREE))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "inspect",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// predicate builds an "is this that" function, which is what most of the
// module's is* helpers are.
func predicate(test func(py.Object) bool) *py.Method {
	return py.MustNewMethod("is", func(self py.Object, args py.Tuple) (py.Object, error) {
		var obj py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "is", 1, 1, &obj); err != nil {
			return nil, err
		}
		if test(obj) {
			return py.True, nil
		}
		return py.False, nil
	}, 0, "Is the object this kind of thing?")
}

const currentframe_doc = `currentframe() -> Frame object

Return the frame object for the caller's stack frame.  It is the same frame
sys._getframe() returns, so it is answered by the interpreter rather than by
this module.`

const cleandoc_doc = `Clean up indentation from docstrings.

Any leading whitespace is removed from the first line.  Any leading
whitespace that can be uniformly removed from the second and further lines
is removed.  Empty lines at the beginning and end are removed.`

func inspectCleanDoc(self py.Object, args py.Tuple) (py.Object, error) {
	var doc py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "cleandoc", 1, 1, &doc); err != nil {
		return nil, err
	}
	text, ok := doc.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "cleandoc() argument must be a string")
	}
	return py.String(cleanDoc(string(text))), nil
}

// cleanDoc is the indentation-cleaning rule from CPython's inspect.
func cleanDoc(doc string) string {
	lines := strings.Split(doc, "\n")

	// The first line has only its leading whitespace removed...
	lines[0] = strings.TrimLeft(lines[0], " \t")
	if len(lines) == 1 {
		return lines[0]
	}

	// ...and the rest lose the whitespace that is common to all of them,
	// ignoring lines that are entirely blank.
	margin := -1
	for _, line := range lines[1:] {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		indent := len(line) - len(trimmed)
		if margin < 0 || indent < margin {
			margin = indent
		}
	}
	if margin < 0 {
		margin = 0
	}
	for i, line := range lines[1:] {
		if len(line) >= margin {
			lines[i+1] = line[margin:]
		} else {
			lines[i+1] = strings.TrimLeft(line, " \t")
		}
	}

	// Blank lines at either end go, but not in the middle.
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

const isgeneratorfunction_doc = `isgeneratorfunction(object) -> bool

Return true if the object is a user-defined generator function.`

func inspectIsGeneratorFunction(self py.Object, args py.Tuple) (py.Object, error) {
	var obj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "isgeneratorfunction", 1, 1, &obj); err != nil {
		return nil, err
	}
	fn, ok := obj.(*py.Function)
	if !ok || fn.Code == nil {
		return py.False, nil
	}
	if fn.Code.Flags&py.CO_GENERATOR != 0 {
		return py.True, nil
	}
	return py.False, nil
}

const getdoc_doc = `getdoc(object) -> string

Get the documentation string for an object, cleaned up with cleandoc().`

const getmro_doc = `getmro(cls) -> tuple

Return a tuple of the method resolution order for cls, which is cls.__mro__.`

// getmro returns a class's method resolution order.
//
// It is cls.__mro__ and nothing more, which is why pkg_resources' _find_adapter
// - which looks an adapter up by walking a class's MRO - depends on it.  A
// non-class raises AttributeError, as CPython's does.
func getmro(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "getmro() takes exactly one argument")
	}
	return py.GetAttrString(args[0], "__mro__")
}

func getdoc(self py.Object, args py.Tuple) (py.Object, error) {
	var obj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "getdoc", 1, 1, &obj); err != nil {
		return nil, err
	}
	doc, err := py.GetAttrString(obj, "__doc__")
	if err != nil {
		return py.None, nil
	}
	if doc == py.None || doc == nil {
		return py.None, nil
	}
	text, ok := doc.(py.String)
	if !ok {
		return py.None, nil
	}
	cleaned := cleanDoc(string(text))
	if cleaned == "" {
		return py.None, nil
	}
	return py.String(cleaned), nil
}

const signature_doc = `signature(callable, *, follow_wrapped=True, globals=None, locals=None,
eval_str=False) -> Signature object`

func notImplemented(self py.Object, args py.Tuple) (py.Object, error) {
	return nil, py.ExceptionNewf(py.NotImplementedError, "inspect.signature is not implemented: this interpreter does not keep the argument binding needed to report it")
}
