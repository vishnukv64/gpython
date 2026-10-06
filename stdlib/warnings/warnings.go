// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package warnings provides the implementation of python's 'warnings'
// module.
//
// The filter list, the "once"/"default"/"module" registries and the
// standard output format are all implemented, so a library that emits a
// warning behaves as it does under CPython; the part that is not is the
// low-level C API (_warnings), which nothing at this level can use.
package warnings

import (
	"fmt"
	"regexp"

	"github.com/vishnukv64/gpython/py"
)

const warnings_doc = `Warning control.

Warning messages are typically issued in situations where it is useful to alert
the user of some condition in a program, where that condition (normally) doesn't
warrant raising an exception and terminating the program.  For example, one
might want to warn the user about a program that is using an outdated module.

Warning messages are normally written to sys.stderr, but their disposition
(including whether to ignore them) can be changed with the filterwarnings()
and simplefilter() functions.`

// A filter as it appears in warnings.filters: the five-tuple
// (action, message, category, module, lineno).  The message and module are
// regular expressions compiled from the strings given to the filter
// functions; an empty pattern matches anything.
type warnFilter struct {
	action   string
	message  string
	category *py.Type
	module   string
	lineno   int
	msgRe    *regexp.Regexp
	modRe    *regexp.Regexp
}

var (
	// gFilters is the authoritative filter list; the "filters" module global
	// is a list mirror of it, rebuilt whenever it changes.
	gFilters []*warnFilter
	// gFiltersList is the Python-visible list.
	gFiltersList = py.NewList()

	// The registries that give "once", "default" and "module" their
	// print-at-most-once behaviour.  The keys are strings rather than the
	// (text, category, lineno) tuples CPython uses so that no object has to
	// be hashable to record a warning.
	gOnceRegistry = map[string]bool{}
	// gModuleRegistry holds the "default"/"module" registry, keyed by
	// module name as well.
	gModuleRegistry = map[string]bool{}

	gDefaultAction = "default"
	gOnceregistry  = py.NewStringDict()
)

// init seeds the same default filters CPython starts with, so that
// "warnings.filters" has the documented shape before any call and
// DeprecationWarning is ignored by default.  Each is (action, message,
// category, module, lineno).
func init() {
	defaults := []struct {
		action   string
		message  string
		category *py.Type
		module   string
		lineno   int
	}{
		{"default", "", py.DeprecationWarning, "__main__", 0},
		{"ignore", "", py.DeprecationWarning, "", 0},
		{"ignore", "", py.PendingDeprecationWarning, "", 0},
		{"ignore", "", py.ImportWarning, "", 0},
		{"ignore", "", py.ResourceWarning, "", 0},
	}
	for _, d := range defaults {
		f, err := compileFilter(d.action, d.message, d.category, d.module, d.lineno)
		if err != nil {
			continue
		}
		gFilters = append(gFilters, f)
	}
	mutatingFilter()
}

// compileFilter compiles the two patterns, turning a bad one into the
// error CPython raises.
func compileFilter(action, message string, category *py.Type, module string, lineno int) (*warnFilter, error) {
	if !validAction(action) {
		return nil, py.ExceptionNewf(py.ValueError, "invalid action: %s", action)
	}
	f := &warnFilter{
		action:   action,
		message:  message,
		category: category,
		module:   module,
		lineno:   lineno,
	}
	if message != "" {
		re, err := regexp.Compile(message)
		if err != nil {
			return nil, py.ExceptionNewf(py.ValueError, "invalid message filter: %s", message)
		}
		f.msgRe = re
	}
	if module != "" {
		re, err := regexp.Compile(module)
		if err != nil {
			return nil, py.ExceptionNewf(py.ValueError, "invalid module filter: %s", module)
		}
		f.modRe = re
	}
	return f, nil
}

func validAction(action string) bool {
	switch action {
	case "error", "ignore", "always", "default", "module", "once":
		return true
	}
	return false
}

// mutatingFilter records that the filter list changed, which invalidates
// the registries - that is what makes a later simplefilter("always") show a
// warning a previous "default" had already printed.
func mutatingFilter() {
	gOnceRegistry = map[string]bool{}
	gModuleRegistry = map[string]bool{}
	gFiltersList.Items = gFiltersList.Items[:0]
	for _, f := range gFilters {
		msg := py.Object(py.None)
		if f.message != "" {
			msg = py.String(f.message)
		}
		mod := py.Object(py.None)
		if f.module != "" {
			mod = py.String(f.module)
		}
		gFiltersList.Append(py.Tuple{py.String(f.action), msg, f.category, mod, py.Int(f.lineno)})
	}
}

// categoryOf reports the warning class an object names: either the class
// itself or the class of a warning instance.
func categoryOf(message, category py.Object) (*py.Type, error) {
	if t, ok := category.(*py.Type); ok {
		return t, nil
	}
	return nil, py.ExceptionNewf(py.TypeError, "category must be a Warning subclass, not '%s'", category.Type().Name)
}

const warn_doc = `warn(message, category=UserWarning, stacklevel=1, source=None)

Issue a warning, or maybe ignore it or raise an exception.  The category
argument, if given, must be a warning category class; it defaults to
UserWarning.  Alternatively, message can be a Warning instance, in which case
category will be ignored and message.__class__ will be used.  In this case,
the message text will be str(message).  This function raises an exception if
the particular warning triggered by this call is set up to be an error.`

func warn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		message   py.Object
		category  py.Object = py.UserWarning
		stackObj  py.Object = py.Int(1)
		sourceObj py.Object
	)
	kwlist := []string{"message", "category", "stacklevel", "source"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO$O:warn", kwlist,
		&message, &category, &stackObj, &sourceObj); err != nil {
		return nil, err
	}
	stacklevel, err := py.IndexInt(stackObj)
	if err != nil {
		return nil, err
	}

	// "warnings.warn(SomeWarning(...))" takes its category from the
	// instance; the text is that instance rendered as a string.
	if exc, ok := message.(*py.Exception); ok {
		category = exc.Type()
	} else if t := message.Type(); t.IsSubtype(py.Warning) {
		category = t
	}

	text, err := py.StrAsString(message)
	if err != nil {
		return nil, err
	}

	// Walk out to the frame that asked for the warning.  A builtin has no
	// frame of its own, so CurrentFrame is the caller and stacklevel counts
	// from there.
	mod := moduleOf(self)
	frame := frameOf(self)
	for i := 1; i < stacklevel && frame != nil; i++ {
		frame = frame.Back
	}

	filename := "<string>"
	lineno := 0
	moduleName := "?"
	if frame != nil {
		filename = frame.Code.Filename
		lineno = int(frame.Code.Addr2Line(frame.Lasti))
		if name, ok := frame.Globals.GetOrNil("__name__").(py.String); ok {
			moduleName = string(name)
		}
	}
	return warnExplicit(mod, text, category, filename, lineno, moduleName, nil)
}

const warn_explicit_doc = `warn_explicit(message, category, filename, lineno, module=None,
              registry=None, module_globals=None, source=None)

This is a low-level interface to the functionality of warn().  It takes
the message, category, filename and lineno as explicit arguments.`

func warn_explicit(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		message        py.Object
		category       py.Object
		filenameObject py.Object
		linenoObject   py.Object
		moduleObject   py.Object = py.None
		registryObject py.Object = py.None
		globalsObject  py.Object
		sourceObject   py.Object
	)
	kwlist := []string{"message", "category", "filename", "lineno", "module", "registry", "module_globals", "source"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "OOOi|OOOO:warn_explicit", kwlist,
		&message, &category, &filenameObject, &linenoObject, &moduleObject,
		&registryObject, &globalsObject, &sourceObject); err != nil {
		return nil, err
	}
	filename, err := py.StrAsString(filenameObject)
	if err != nil {
		return nil, err
	}
	lineno, err := py.IndexInt(linenoObject)
	if err != nil {
		return nil, err
	}
	moduleName := "?"
	if s, ok := moduleObject.(py.String); ok {
		moduleName = string(s)
	}
	text, err := py.StrAsString(message)
	if err != nil {
		return nil, err
	}
	return warnExplicit(moduleOf(self), text, category, filename, lineno, moduleName, registryObject)
}

// moduleOf returns the module a method was called through, so that the
// module globals (showwarning, filters) that Python code may have replaced
// are the ones actually used.
func moduleOf(self py.Object) *py.Module {
	if m, ok := self.(*py.Module); ok {
		return m
	}
	return nil
}

// frameOf is the frame that invoked a method of this package.  It is
// reached through the calling module's context, which owns the frame stack.
func frameOf(self py.Object) *py.Frame {
	if mod := moduleOf(self); mod != nil && mod.Context != nil {
		return py.CurrentFrame()
	}
	return nil
}

// warnExplicit is the shared implementation: it consults the filters and
// either ignores, records an "once" entry, prints or raises.
func warnExplicit(mod *py.Module, text string, category py.Object, filename string, lineno int, moduleName string, registry py.Object) (py.Object, error) {
	cat, err := categoryOf(nil, category)
	if err != nil {
		return nil, err
	}

	action := gDefaultAction
	for _, f := range gFilters {
		if f.category != nil && !cat.IsSubtype(f.category) {
			continue
		}
		if f.msgRe != nil && !f.msgRe.MatchString(text) {
			continue
		}
		if f.lineno != 0 && f.lineno != lineno {
			continue
		}
		if f.modRe != nil && !f.modRe.MatchString(moduleName) {
			continue
		}
		action = f.action
		break
	}

	switch action {
	case "ignore":
		return py.None, nil
	case "error":
		return nil, py.ExceptionNewf(cat, "%s", text)
	case "once":
		key := fmt.Sprintf("%s\x00%s\x00%d", text, cat.Name, lineno)
		if gOnceRegistry[key] {
			return py.None, nil
		}
		gOnceRegistry[key] = true
	case "default", "module":
		// Both print at most once per location; "module" is named for the
		// registry it keeps, which is the importing module's.
		owner := moduleName
		if registry != nil && registry != py.None {
			owner = fmt.Sprintf("%v", filename)
		}
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%d", owner, text, cat.Name, lineno)
		if gModuleRegistry[key] {
			return py.None, nil
		}
		gModuleRegistry[key] = true
	}

	return showWarning(mod, text, cat, filename, lineno)
}

// showWarning delivers a warning: within a catch_warnings(record=True)
// block it appends a WarningMessage to the record list, otherwise it calls
// the module's showwarning, which a library may have replaced.
func showWarning(mod *py.Module, text string, cat *py.Type, filename string, lineno int) (py.Object, error) {
	if n := len(gRecorders); n > 0 {
		gRecorders[n-1].Append(&WarningMessage{
			Message:  py.String(text),
			Category: cat,
			Filename: filename,
			Lineno:   lineno,
		})
		return py.None, nil
	}
	fn := py.Object(nil)
	if mod != nil {
		fn = mod.Globals.GetOrNil("showwarning")
	}
	if fn == nil || fn == py.None {
		fn = py.MustNewMethod("showwarning", defaultShowWarning, 0, showwarning_doc)
	}
	_, err := py.Call(fn, py.Tuple{py.String(text), cat, py.String(filename), py.Int(lineno)}, py.StringDict{})
	if err != nil {
		return nil, err
	}
	return py.None, nil
}

const showwarning_doc = `showwarning(message, category, filename, lineno, file=None, line=None)

Write a warning to a file.  The default implementation calls
formatwarning(message, category, filename, lineno, line) and writes the
resulting string to file, which defaults to sys.stderr.`

func showwarning(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		message  py.Object
		category py.Object
		filename py.Object
		lineno   py.Object
		fileObj  py.Object = py.None
		lineObj  py.Object = py.None
	)
	kwlist := []string{"message", "category", "filename", "lineno", "file", "line"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "OOOi|OO:showwarning", kwlist,
		&message, &category, &filename, &lineno, &fileObj, &lineObj); err != nil {
		return nil, err
	}
	return writeWarning(moduleOf(self), message, category, filename, lineno, fileObj, lineObj)
}

func defaultShowWarning(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return showwarning(self, args, kwargs)
}

func writeWarning(mod *py.Module, message, category, filename, lineno, fileObj, lineObj py.Object) (py.Object, error) {
	// The text is formatted with the module's formatwarning, which a
	// library may have replaced.
	formatFn := py.Object(nil)
	if mod != nil {
		formatFn = mod.Globals.GetOrNil("formatwarning")
	}
	if formatFn == nil || formatFn == py.None {
		formatFn = py.MustNewMethod("formatwarning", formatwarning, 0, formatwarning_doc)
	}
	s, err := py.Call(formatFn, py.Tuple{message, category, filename, lineno, lineObj}, py.StringDict{})
	if err != nil {
		return nil, err
	}
	text, err := py.StrAsString(s)
	if err != nil {
		return nil, err
	}

	if fileObj == nil || fileObj == py.None {
		fileObj = stderrOf(mod)
	}
	write, err := py.GetAttrString(fileObj, "write")
	if err != nil {
		return nil, err
	}
	if _, err := py.Call(write, py.Tuple{py.String(text)}, py.StringDict{}); err != nil {
		return nil, err
	}
	return py.None, nil
}

// stderrOf finds sys.stderr from the running context when it can, so that a
// test that swapped sys.stderr captures the warning.
func stderrOf(mod *py.Module) py.Object {
	if mod != nil && mod.Context != nil {
		if sys, err := mod.Context.GetModule("sys"); err == nil {
			if s, ok := sys.Globals.Get("stderr"); ok {
				return s
			}
		}
	}
	if sys := py.GetModuleImplOrNil("sys"); sys != nil {
		return sys.Globals.GetOrNil("stderr")
	}
	return py.None
}

const formatwarning_doc = `formatwarning(message, category, filename, lineno, line=None)

Format a warning the standard way, as "file:line: Category: message".`

func formatwarning(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		message  py.Object
		category py.Object
		filename py.Object
		lineno   py.Object
		lineObj  py.Object = py.None
	)
	kwlist := []string{"message", "category", "filename", "lineno", "line"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "OOOi|O:formatwarning", kwlist,
		&message, &category, &filename, &lineno, &lineObj); err != nil {
		return nil, err
	}
	cat, err := categoryOf(message, category)
	if err != nil {
		return nil, err
	}
	text, err := py.StrAsString(message)
	if err != nil {
		return nil, err
	}
	file, err := py.StrAsString(filename)
	if err != nil {
		return nil, err
	}
	n, err := py.IndexInt(lineno)
	if err != nil {
		return nil, err
	}
	return py.String(fmt.Sprintf("%s:%d: %s: %s\n", file, n, cat.Name, text)), nil
}

const filterwarnings_doc = `filterwarnings(action, message="", category=Warning, module="", lineno=0,
               append=False)

Insert an entry into the list of warnings filters (at the front).`

func filterwarnings(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		action    py.Object
		message   py.Object = py.String("")
		category  py.Object = py.Warning
		module    py.Object = py.String("")
		lineno    py.Object = py.Int(0)
		appendObj py.Object = py.False
	)
	kwlist := []string{"action", "message", "category", "module", "lineno", "append"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "s|sOsi$p:filterwarnings", kwlist,
		&action, &message, &category, &module, &lineno, &appendObj); err != nil {
		return nil, err
	}
	return addFilter(action, message, category, module, lineno, appendObj)
}

const simplefilter_doc = `simplefilter(action, category=UserWarning, lineno=0, append=False)

Insert a simple entry into the list of warnings filters (at the front).`

func simplefilter(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		action    py.Object
		category  py.Object = py.UserWarning
		lineno    py.Object = py.Int(0)
		appendObj py.Object = py.False
	)
	kwlist := []string{"action", "category", "lineno", "append"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "s|Oi$p:simplefilter", kwlist,
		&action, &category, &lineno, &appendObj); err != nil {
		return nil, err
	}
	return addFilter(action, py.String(""), category, py.String(""), lineno, appendObj)
}

func addFilter(action, message, category, module, lineno, appendObj py.Object) (py.Object, error) {
	actionStr, err := py.StrAsString(action)
	if err != nil {
		return nil, err
	}
	messageStr, err := py.StrAsString(message)
	if err != nil {
		return nil, err
	}
	moduleStr, err := py.StrAsString(module)
	if err != nil {
		return nil, err
	}
	linenoInt, err := py.IndexInt(lineno)
	if err != nil {
		return nil, err
	}
	cat, err := categoryOf(nil, category)
	if err != nil {
		return nil, err
	}
	f, err := compileFilter(actionStr, messageStr, cat, moduleStr, linenoInt)
	if err != nil {
		return nil, err
	}
	app, err := py.ObjectIsTrue(appendObj)
	if err != nil {
		return nil, err
	}
	if app {
		gFilters = append(gFilters, f)
	} else {
		gFilters = append([]*warnFilter{f}, gFilters...)
	}
	mutatingFilter()
	return py.None, nil
}

const resetwarnings_doc = `resetwarnings()

Reset the warnings filter.  This discards the effect of all previous calls to
filterwarnings(), including that of the -W command line options and calls to
simplefilter().`

func resetwarnings(self py.Object, args py.Tuple) (py.Object, error) {
	if err := checkArgs(args, py.StringDict{}, "resetwarnings", 0, 0); err != nil {
		return nil, err
	}
	gFilters = nil
	mutatingFilter()
	return py.None, nil
}

// WarningMessage is the record handed back by catch_warnings(record=True).
type WarningMessage struct {
	Message  py.Object
	Category *py.Type
	Filename string
	Lineno   int
}

var WarningMessageType = py.NewType("warnings.WarningMessage", "A record of a warning.")

func (w *WarningMessage) Type() *py.Type { return WarningMessageType }

// catchWarnings is the context manager returned by catch_warnings().
//
// It saves the filter list and the two output hooks on entry and puts them
// back on exit, so a block can change them without leaking the change.
type catchWarnings struct {
	record    bool
	list      *py.List
	saved     []*warnFilter
	savedList []py.Object
	show      py.Object
	format    py.Object
	mod       *py.Module
}

var catchWarningsType = py.NewType("warnings.catch_warnings", "A context manager that copies and restores the warnings filter.")

func (c *catchWarnings) Type() *py.Type { return catchWarningsType }

const catch_warnings_doc = `catch_warnings(*, record=False, module=None, action=None, category=Warning,
             lineno=0, append=False)

A context manager that copies and, upon exit, restores the warnings filter
and the showwarning() function.  If the record argument is True (the default
is False) the context manager returns a list where the warnings are placed
in the order they were issued.`

func catch_warnings(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		record    py.Object = py.False
		module    py.Object = py.None
		action    py.Object = py.None
		category  py.Object = py.Warning
		lineno    py.Object = py.Int(0)
		appendObj py.Object = py.False
	)
	kwlist := []string{"record", "module", "action", "category", "lineno", "append"}
	if err := py.ParseTupleAndKeywords(nil, kwargs, "|$pOOsip:catch_warnings", kwlist,
		&record, &module, &action, &category, &lineno, &appendObj); err != nil {
		return nil, err
	}
	rec, err := py.ObjectIsTrue(record)
	if err != nil {
		return nil, err
	}
	c := &catchWarnings{record: rec, mod: moduleOf(self)}
	if action != py.None && action != nil {
		if err := c.save(); err != nil {
			return nil, err
		}
		if _, err := addFilter(action, py.String(""), category, py.String(""), lineno, appendObj); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// save copies the state that a catch_warnings block may change.
func (c *catchWarnings) save() error {
	c.saved = append([]*warnFilter(nil), gFilters...)
	c.savedList = append([]py.Object(nil), gFiltersList.Items...)
	if c.mod != nil {
		c.show = c.mod.Globals.GetOrNil("showwarning")
		c.format = c.mod.Globals.GetOrNil("formatwarning")
	}
	return nil
}

const enter_doc = `Enter the context manager, returning the record list if recording.`

func catchWarningsEnter(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*catchWarnings)
	if c.saved == nil {
		if err := c.save(); err != nil {
			return nil, err
		}
	}
	if c.record {
		c.list = py.NewList()
		gRecorders = append(gRecorders, c.list)
		return c.list, nil
	}
	return py.None, nil
}

const exit_doc = `Restore the warnings filter and output hooks.`

func catchWarningsExit(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*catchWarnings)
	if c.record && len(gRecorders) > 0 {
		gRecorders = gRecorders[:len(gRecorders)-1]
	}
	gFilters = append([]*warnFilter(nil), c.saved...)
	gFiltersList.Items = append([]py.Object(nil), c.savedList...)
	if c.mod != nil {
		if c.show != nil {
			c.mod.Globals.Set("showwarning", c.show)
		}
		if c.format != nil {
			c.mod.Globals.Set("formatwarning", c.format)
		}
	}
	return py.False, nil
}

// gRecorders holds the active catch_warnings(record=True) lists, innermost
// last, so that the warn family - which is not a method of the context
// manager - can append to the right one.
var gRecorders []*py.List

func init() {
	catchWarningsType.Dict.Set("__enter__", py.MustNewMethod("__enter__", catchWarningsEnter, 0, enter_doc))
	catchWarningsType.Dict.Set("__exit__", py.MustNewMethod("__exit__", catchWarningsExit, 0, exit_doc))

	WarningMessageType.Dict.Set("message", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*WarningMessage).Message, nil },
		Doc:  "the message text",
	})
	WarningMessageType.Dict.Set("category", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*WarningMessage).Category, nil },
		Doc:  "the warning category",
	})
	WarningMessageType.Dict.Set("filename", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(self.(*WarningMessage).Filename), nil },
		Doc:  "the file the warning came from",
	})
	WarningMessageType.Dict.Set("lineno", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.Int(self.(*WarningMessage).Lineno), nil },
		Doc:  "the line the warning came from",
	})

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "warnings",
			Doc:  warnings_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("warn", warn, 0, warn_doc),
			py.MustNewMethod("warn_explicit", warn_explicit, 0, warn_explicit_doc),
			py.MustNewMethod("showwarning", showwarning, 0, showwarning_doc),
			py.MustNewMethod("formatwarning", formatwarning, 0, formatwarning_doc),
			py.MustNewMethod("filterwarnings", filterwarnings, 0, filterwarnings_doc),
			py.MustNewMethod("simplefilter", simplefilter, 0, simplefilter_doc),
			py.MustNewMethod("resetwarnings", resetwarnings, 0, resetwarnings_doc),
			py.MustNewMethod("catch_warnings", catch_warnings, 0, catch_warnings_doc),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "filters", Value: gFiltersList},
			py.DictEntry{Key: "defaultaction", Value: py.String("default")},
			py.DictEntry{Key: "onceregistry", Value: gOnceregistry},
			py.DictEntry{Key: "WarningMessage", Value: WarningMessageType},
			// The filter machinery needs the same hook objects Python code
			// can replace, so the module and the package agree on them.
			py.DictEntry{Key: "showwarning", Value: py.MustNewMethod("showwarning", showwarning, 0, showwarning_doc)},
			py.DictEntry{Key: "formatwarning", Value: py.MustNewMethod("formatwarning", formatwarning, 0, formatwarning_doc)},
			py.DictEntry{Key: "Warning", Value: py.Warning},
			py.DictEntry{Key: "UserWarning", Value: py.UserWarning},
			py.DictEntry{Key: "DeprecationWarning", Value: py.DeprecationWarning},
			py.DictEntry{Key: "PendingDeprecationWarning", Value: py.PendingDeprecationWarning},
			py.DictEntry{Key: "SyntaxWarning", Value: py.SyntaxWarning},
			py.DictEntry{Key: "RuntimeWarning", Value: py.RuntimeWarning},
			py.DictEntry{Key: "FutureWarning", Value: py.FutureWarning},
			py.DictEntry{Key: "ImportWarning", Value: py.ImportWarning},
			py.DictEntry{Key: "BytesWarning", Value: py.BytesWarning},
			py.DictEntry{Key: "UnicodeWarning", Value: py.UnicodeWarning},
			py.DictEntry{Key: "ResourceWarning", Value: py.ResourceWarning},
		),
	})
}

// unused keeps the import list honest.
var _ = fmt.Sprintf

// checkArgs validates the positional and keyword argument counts of a
// method whose body reads its arguments directly, rather than unpacking
// them into named variables.  min may be negative, meaning "no lower
// bound".
func checkArgs(args py.Tuple, kwargs py.StringDict, name string, min, max int) error {
	if kwargs.Len() != 0 {
		return py.ExceptionNewf(py.TypeError, "%s() takes no keyword arguments", name)
	}
	n := len(args)
	if min >= 0 && n < min {
		return py.ExceptionNewf(py.TypeError, "%s() takes at least %d argument%s (%d given)", name, min, plural(min), n)
	}
	if max >= 0 && n > max {
		if min == max {
			return py.ExceptionNewf(py.TypeError, "%s() takes exactly %d argument%s (%d given)", name, max, plural(max), n)
		}
		return py.ExceptionNewf(py.TypeError, "%s() takes at most %d argument%s (%d given)", name, max, plural(max), n)
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
