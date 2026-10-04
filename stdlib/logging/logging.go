// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package logging provides the implementation of python's 'logging' module.
//
// A working subset is implemented: loggers with a level and a handler list,
// the level constants, StreamHandler/FileHandler with a Formatter, and the
// module level convenience functions that forward to the root logger.  The
// default output - "LEVEL:name:message" on stderr - matches CPython.
//
// What is deliberately absent is the configuration layer (dictConfig,
// fileConfig, addLevelName, the LogRecord factory hook); a program that only
// emits and filters log records does not need it.
package logging

import (
	"fmt"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const logging_doc = `Logging facility for Python.

The key benefit of having the logging API provided by a standard library
module is that all Python modules can participate in logging, so your
application log can include your own messages integrated with messages from
third-party modules.`

// Level constants, as in CPython.
const (
	CRITICAL = 50
	ERROR    = 40
	WARNING  = 30
	INFO     = 20
	DEBUG    = 10
	NOTSET   = 0
)

var levelNames = map[int]string{
	CRITICAL: "CRITICAL",
	ERROR:    "ERROR",
	WARNING:  "WARNING",
	INFO:     "INFO",
	DEBUG:    "DEBUG",
	NOTSET:   "NOTSET",
}

// LogRecord carries one event through the handler chain.
type LogRecord struct {
	Name     string
	LevelNo  int
	Msg      py.Object
	Args     py.Object
	ExcInfo  py.Object
	Pathname string
	Filename string
	Module   string
	Lineno   int
	FuncName string
	Created  float64
}

var LogRecordType = py.NewTypeX("logging.LogRecord",
	"A LogRecord instance represents an event being logged.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		// CPython's signature is LogRecord(name, level, pathname, lineno, msg,
		// args, exc_info, func=None, sinfo=None).  A record can be built
		// directly - a Filter is handed one, and a test constructs one - so the
		// type has to be constructible rather than raising.
		rec := &LogRecord{Created: 0}
		var name, pathname, funcName string
		var level, lineno int
		var msg, a, exc py.Object
		get := func(i int, kw string) py.Object {
			if v, ok := kwargs.Get(kw); ok {
				return v
			}
			if i < len(args) {
				return args[i]
			}
			return nil
		}
		if v := get(0, "name"); v != nil {
			name, _ = py.StrAsString(v)
		}
		if v := get(1, "level"); v != nil {
			level = int(intArgOf(v))
		}
		if v := get(2, "pathname"); v != nil {
			pathname, _ = py.StrAsString(v)
		}
		if v := get(3, "lineno"); v != nil {
			lineno = int(intArgOf(v))
		}
		if v := get(4, "msg"); v != nil {
			msg = v
		}
		if v := get(5, "args"); v != nil {
			a = v
		}
		if v := get(6, "exc_info"); v != nil {
			exc = v
		}
		if v := get(7, "func"); v != nil {
			funcName, _ = py.StrAsString(v)
		}
		rec.Name, rec.LevelNo, rec.Pathname, rec.Lineno = name, level, pathname, lineno
		rec.Msg, rec.Args, rec.ExcInfo, rec.FuncName = msg, a, exc, funcName
		return rec, nil
	}, nil)

// intArgOf reads an integer from a Python value, returning 0 when it is not one.
func intArgOf(o py.Object) int64 {
	if v, ok := o.(py.Int); ok {
		n, _ := v.GoInt64()
		return n
	}
	return 0
}

func (r *LogRecord) Type() *py.Type { return LogRecordType }

func (r *LogRecord) M__repr__() (py.Object, error) {
	return py.String(fmt.Sprintf("<LogRecord: %s, %d, %s, %d, %q>", r.Name, r.LevelNo, r.Pathname, r.Lineno, r.Msg)), nil
}

// getMessage renders the record's message with its arguments, using the
// interpreter's own "%" so that any object implementing __str__ works.
func (r *LogRecord) getMessage() (string, error) {
	if r.Args == nil || r.Args == py.None {
		return py.StrAsString(r.Msg)
	}
	rendered, err := py.Mod(r.Msg, r.Args)
	if err != nil {
		return "", err
	}
	return py.StrAsString(rendered)
}

// Handler is the base class: anything that can emit a formatted record.
// filterObj is the Go value behind logging.Filter.
type filterObj struct {
	name string
}

var filterType = py.NewTypeX("logging.Filter",
	"Filter instances are used to perform arbitrary filtering of LogRecords.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		f := &filterObj{}
		// __init__ is not run for a NewTypeX-built type, so the name is applied
		// here - the same reason logging.handlers applies its arguments in its
		// constructor.
		if len(args) > 0 {
			f.name, _ = py.StrAsString(args[0])
		}
		if v, ok := kwargs.Get("name"); ok {
			f.name, _ = py.StrAsString(v)
		}
		return f, nil
	}, nil)

func (f *filterObj) Type() *py.Type { return filterType }

// filterMatches is the filtering rule itself, CPython's Filter.filter: a record
// from the named logger OR ANY OF ITS CHILDREN passes.
//
// The child rule is the whole point - "Filter('a')" passes "a.b" - and getting it
// backwards turns a filter into a silencer.
func filterMatches(name, recordName string) bool {
	if name == "" {
		return true
	}
	return recordName == name || strings.HasPrefix(recordName, name+".")
}

// filterOf returns the filter behind an object, or false when it has none.
//
// A filter written in Python is a subclass of this type, so it is not a
// *filterObj; its filter() method is what must run.  Callers use this only to
// reach the method, never to assume the Go type.
func filterNameOf(o py.Object) (string, bool) {
	switch f := o.(type) {
	case *filterObj:
		return f.name, true
	}
	return "", false
}

// runFilter calls an object's filter(record) and reports whether it passed.
//
// Anything callable is accepted, because a Python subclass of Filter keeps its
// override as a method rather than being a *filterObj.
func runFilter(o py.Object, record *LogRecord) (bool, error) {
	fn, err := py.GetAttrString(o, "filter")
	if err != nil {
		// No filter method: a filter object without one passes everything, as
		// CPython's duck-typing expects.
		return true, nil
	}
	res, err := py.Call(fn, py.Tuple{record}, py.NewStringDict())
	if err != nil {
		return false, err
	}
	return res == py.True, nil
}

type Handler struct {
	level     int
	formatter *Formatter
	// stream is where a stream-based handler writes.
	stream py.Object
	// emit is the output function; a subclass replaces it.
	emit func(h *Handler, record *LogRecord) error
	// owner is the Python object this Handler is embedded in, when it is a
	// handler from another module.  callHandlers uses it to reach the
	// subclass's own emit rather than the base one.
	owner py.Object
	// filters are run before a record is emitted; one returning false
	// suppresses it.
	filters []py.Object
}

var HandlerType = py.NewTypeX("logging.Handler",
	"Handler instances dispatch logging events to specific destinations.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		// The base Handler writes nowhere; a subclass such as StreamHandler
		// supplies the destination.  Constructing it directly is legal, as
		// in CPython.
		h := &Handler{level: NOTSET, emit: streamEmit}
		registerHandler(h)
		return h, nil
	}, nil)

// gHandlerList holds every handler created, so that shutdown() can flush and
// close them.  CPython keeps weak references; a direct list is close enough
// here and cannot lose an entry to collection mid-run.
//
// The entries are the handler objects, so that a Python-level close() on a
// subclass is what shutdown calls.
var gHandlerList []py.Object

// registerHandler records a handler for shutdown().  Called from every
// constructor, since a handler that shutdown cannot see is a handler whose
// buffered output is lost.
func registerHandler(h py.Object) {
	gHandlerList = append(gHandlerList, h)
}

func (h *Handler) Type() *py.Type { return HandlerType }

// A stream handler and a file handler are the base handler with a different
// destination, so they are types of their own that share the Go struct.
var StreamHandlerType = py.NewTypeX("logging.StreamHandler",
	"Handler instances dispatch logging events to specific destinations.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return streamHandler(metatype, args, kwargs)
	}, nil)

var FileHandlerType = py.NewTypeX("logging.FileHandler",
	"Handler instances dispatch logging events to a file.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return fileHandler(metatype, args, kwargs)
	}, nil)

// nullHandler discards every record; it is what a library attaches to its
// logger when it has no opinion about output, and requests depends on it.
var NullHandlerType = py.NewTypeX("logging.NullHandler",
	"This handler does nothing with the events it receives.  It is intended to be used by library developers.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		h := &Handler{level: NOTSET, emit: func(h *Handler, record *LogRecord) error { return nil }}
		return h, nil
	}, nil)

// Formatter turns a record into text.
type Formatter struct {
	fmt        string
	dateFmt    string
	defaultFmt bool
}

var FormatterType = py.NewTypeX("logging.Formatter",
	"Formatter instances are used to convert a LogRecord to text.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return formatter(metatype, args, kwargs)
	}, nil)

func (f *Formatter) Type() *py.Type { return FormatterType }

// Logger is a named channel with a level and a list of handlers.
type Logger struct {
	name     string
	level    int
	handlers []*Handler
	parent   *Logger
	// propagate mirrors the attribute of the same name.
	propagate bool
	// filters are run before dispatch; one returning false drops the record.
	filters []py.Object
	// cls is the class this logger was created as, which is a subclass when
	// logging.setLoggerClass installed one.  Every logger used to report
	// LoggerType, so a subclass instance lost its identity: a method the
	// subclass defined was unreachable and pip's VerboseLogger (installed with
	// setLoggerClass) was a plain Logger, which is why "logger.verbose()"
	// raised AttributeError.
	cls *py.Type
}

var LoggerType = py.NewType("logging.Logger", "A Logger is a named logging channel.")

func (l *Logger) Type() *py.Type {
	if l.cls != nil {
		return l.cls
	}
	return LoggerType
}

// Logger attributes.  A logger's name is how code refers to it - a library
// reads logger.name to label its own output - and it was reachable only from
// __repr__ before.
const shutdown_doc = `Perform any cleanup actions in the logging system (e.g. flushing
buffers).

Should be called at application exit.`

// loggingShutdown flushes and closes every handler, newest first.
//
// It NEVER raises: it runs while a program is exiting, where an error would
// replace whatever the program was doing with a traceback about the logging
// system.  CPython swallows those errors for the same reason, and pip calls
// this on its way out.
func loggingShutdown(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	handlers := make([]py.Object, len(gHandlerList))
	copy(handlers, gHandlerList)
	for i := len(handlers) - 1; i >= 0; i-- {
		h := handlers[i]
		if h == nil {
			continue
		}
		// flushOnClose defaults to true; MemoryHandler sets it false so that
		// its buffered records are not written on the way out.
		if v, err := py.GetAttrString(h, "flushOnClose"); err == nil && v == py.False {
			continue
		}
		if flush, err := py.GetAttrString(h, "flush"); err == nil {
			_, _ = py.Call(flush, py.Tuple{}, py.NewStringDict())
		}
		if close, err := py.GetAttrString(h, "close"); err == nil {
			_, _ = py.Call(close, py.Tuple{}, py.NewStringDict())
		}
	}
	return py.None, nil
}

func init() {
	LoggerType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.String(self.(*Logger).name), nil
		},
	})
}

func (l *Logger) M__repr__() (py.Object, error) {
	return py.String(fmt.Sprintf("<Logger %s (%s)>", l.name, effectiveLevelName(l.level))), nil
}

func addLevelName(self py.Object, args py.Tuple) (py.Object, error) {
	var level, name py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "addLevelName", 2, 2, &level, &name); err != nil {
		return nil, err
	}
	n, err := py.MakeGoInt(level)
	if err != nil {
		return nil, err
	}
	s, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	// A user level is recorded in the same map getLevelName reads, which is
	// what pip does to name its own levels.
	levelNames[n] = s
	return py.None, nil
}

func effectiveLevelName(level int) string {
	if name, ok := levelNames[level]; ok {
		return name
	}
	return fmt.Sprintf("Level %d", level)
}

// gRoot is the root logger; gLoggers holds every logger by name so that
// getLogger is a pure constructor and repeated calls return the same object.
var (
	gRoot    *Logger
	gLoggers = map[string]*Logger{}
)

// gLoggerClass is the class getLogger instantiates, settable through
// setLoggerClass.
//
// pip._internal.utils._log calls logging.setLoggerClass with its own Logger
// subclass, and without the function the import died at
// "'module' has no attribute 'setLoggerClass'".
var gLoggerClass *py.Type

// newLogger builds a logger whose class is the one setLoggerClass installed.
//
// A logger stays the Go value it always was - it is not wrapped - and only its
// Type() reports the subclass, so the subclass's methods are found by the
// ordinary MRO walk and a native accessor like ".name" still asserts *Logger
// successfully.
func newLogger(name string, level int) *Logger {
	l := &Logger{name: name, level: level, propagate: true, cls: LoggerType}
	if gLoggerClass != nil && gLoggerClass != LoggerType {
		l.cls = gLoggerClass
		// A Python __init__ on the subclass runs with this logger as self,
		// which is what a user initialiser expects - and what lets
		// "super().__init__(name)" reach the native one.
		if initFn := gLoggerClass.LookupPython("__init__"); initFn != nil {
			_, _ = py.Call(initFn, py.Tuple{l, py.String(name)}, py.NewStringDict())
		}
	}
	return l
}

func init() {
	gRoot = newLogger("root", WARNING)
	gLoggers["root"] = gRoot
}

// splitName breaks a dotted logger name into its parts.
func splitName(name string) []string {
	return strings.Split(name, ".")
}

// resolveLogger finds (and creates) the logger named name, chaining it to
// the closest existing ancestor so that propagation and effective levels
// work.
func resolveLogger(name string) *Logger {
	if l, ok := gLoggers[name]; ok {
		return l
	}
	parts := splitName(name)
	var parent *Logger = gRoot
	for i := 1; i < len(parts); i++ {
		ancestor := strings.Join(parts[:i], ".")
		if l, ok := gLoggers[ancestor]; ok {
			parent = l
		} else {
			l := newLogger(ancestor, NOTSET)
			l.parent = parent
			gLoggers[ancestor] = l
			parent = l
		}
	}
	l := newLogger(name, NOTSET)
	l.parent = parent
	gLoggers[name] = l
	return l
}

// effectiveLevel walks up the chain for the first level that is set.
func (l *Logger) effectiveLevel() int {
	for logger := l; logger != nil; logger = logger.parent {
		if logger.level != NOTSET {
			return logger.level
		}
	}
	return WARNING
}

func (l *Logger) isEnabledFor(level int) bool {
	return level >= l.effectiveLevel()
}

const getLogger_doc = `getLogger(name=None)

Return a logger with the specified name, creating it if necessary.
If no name is specified, return the root logger.`

func getLogger(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	name := "root"
	if len(args) > 0 {
		if args[0] != py.None {
			s, err := py.StrAsString(args[0])
			if err != nil {
				return nil, err
			}
			name = s
		}
	}
	if err := checkArgs(args, kwargs, "getLogger", 0, 1); err != nil {
		return nil, err
	}
	if name == "root" {
		return gRoot, nil
	}
	return resolveLogger(name), nil
}

// callHandlers runs the record through this logger and, unless propagation
// stops, its ancestors.
// handlerDrops runs a handler's filters, reporting whether the record is
// suppressed.  A filter that returns a falsey value drops the record, which is
// the rule for a Python subclass too.
func handlerDrops(h *Handler, record *LogRecord) (bool, error) {
	for _, f := range h.filters {
		passes, err := runFilter(f, record)
		if err != nil {
			return false, err
		}
		if !passes {
			return true, nil
		}
	}
	return false, nil
}

func (l *Logger) callHandlers(record *LogRecord) error {
	for logger := l; logger != nil; logger = logger.parent {
		for _, h := range logger.handlers {
			if record.LevelNo < h.level {
				continue
			}
			// The handler's OWN emit is called, looked up on the object it was
			// reached through.  Calling the embedded Handler's emit function
			// directly ran the base implementation for every subclass, so a
			// handler from another module - RotatingFileHandler, MemoryHandler
			// - wrote nothing at all and the log line vanished silently.
			if owner := h.owner; owner != nil {
				emit, err := py.GetAttrString(owner, "emit")
				if err == nil {
					if _, cerr := py.Call(emit, py.Tuple{record}, py.NewStringDict()); cerr != nil {
						return cerr
					}
					continue
				}
			}
			if err := h.emit(h, record); err != nil {
				return err
			}
		}
		if !logger.propagate {
			break
		}
	}
	return nil
}

// handle makes a record and dispatches it, returning whether it was passed
// to a handler at all.
func (l *Logger) handle(level int, msg py.Object, args py.Object) (bool, error) {
	if !l.isEnabledFor(level) {
		return false, nil
	}
	record := &LogRecord{
		Name:    l.name,
		LevelNo: level,
		Msg:     msg,
		Args:    args,
	}
	// This logger's filters, and every ancestor's, run BEFORE dispatch - a
	// record dropped here never reaches a handler.  pip's ExcludeLoggerFilter
	// depends on it: it suppresses a whole logger's output, and without this the
	// filter was silently ignored and every record was written anyway.
	for logger := l; logger != nil; logger = logger.parent {
		for _, f := range logger.filters {
			passes, ferr := runFilter(f, record)
			if ferr != nil {
				return false, ferr
			}
			if !passes {
				return false, nil
			}
		}
	}
	if err := l.callHandlers(record); err != nil {
		return false, err
	}
	return true, nil
}

// loggerLog is the body shared by all the level-named methods: the first
// argument is the message, any others are "%"-style arguments for it.
func loggerLog(l *Logger, args py.Tuple, kwargs py.StringDict, level int, name string) (py.Object, error) {
	if err := checkArgs(args, kwargs, "name", 1, -1); err != nil {
		return nil, err
	}
	if !l.isEnabledFor(level) {
		return py.None, nil
	}
	msg := args[0]
	var rest py.Object = py.None
	if len(args) > 1 {
		if len(args) == 2 {
			rest = args[1]
		} else {
			rest = py.Tuple(args[1:])
		}
	}
	if _, err := l.handle(level, msg, rest); err != nil {
		return nil, err
	}
	return py.None, nil
}

func loggerDebug(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return loggerLog(self.(*Logger), args, kwargs, DEBUG, "debug")
}

func loggerInfo(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return loggerLog(self.(*Logger), args, kwargs, INFO, "info")
}

func loggerWarning(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return loggerLog(self.(*Logger), args, kwargs, WARNING, "warning")
}

func loggerError(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return loggerLog(self.(*Logger), args, kwargs, ERROR, "error")
}

func loggerCritical(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return loggerLog(self.(*Logger), args, kwargs, CRITICAL, "critical")
}

const loggerException_doc = `exception(msg, *args, exc_info=True)

Convenience method for logging an ERROR with exception information.`

func loggerException(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// The exception is recorded and reported; the traceback is left to the
	// interpreter's own reporting, which is what the record carries.
	if err := checkArgs(args, kwargs, "exception", 0, -1); err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return py.None, nil
	}
	return loggerLog(self.(*Logger), args, kwargs, ERROR, "exception")
}

const loggerLog_doc = `log(level, msg, *args, **kwargs)

Log 'msg % args' with the integer severity 'level'.`

func loggerLogMethod(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "log", 1, -1); err != nil {
		return nil, err
	}
	levelObj := args[0]
	var level int
	switch v := levelObj.(type) {
	case py.Int:
		n, err := v.GoInt()
		if err != nil {
			return nil, err
		}
		level = n
	default:
		return nil, py.ExceptionNewf(py.TypeError, "level must be an integer")
	}
	l := self.(*Logger)
	if err := checkArgs(args, kwargs, "log", 2, -1); err != nil {
		return nil, err
	}
	if !l.isEnabledFor(level) {
		return py.None, nil
	}
	msg, err := py.Str(args[1])
	if err != nil {
		return nil, err
	}
	if len(args) == 2 && msg.Type().IsSubtype(py.StringType) {
		if _, err := l.handle(level, msg, py.None); err != nil {
			return nil, err
		}
		return py.None, nil
	}
	rest := args[1:]
	if len(rest) == 1 {
		rest = py.Tuple{rest[0]}
	}
	if _, err := l.handle(level, rest[0], rest[1]); err != nil {
		return nil, err
	}
	return py.None, nil
}

const loggerIsEnabledFor_doc = `isEnabledFor(level)

Is this logger enabled for level 'level'?`

func loggerIsEnabledFor(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "isEnabledFor", 1, 1); err != nil {
		return nil, err
	}
	n, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	return py.MakeBool(py.Bool(self.(*Logger).isEnabledFor(n)))
}

const loggerSetLevel_doc = `setLevel(level)

Set the logging level of this logger.`

func loggerSetLevel(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "setLevel", 1, 1); err != nil {
		return nil, err
	}
	n, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	self.(*Logger).level = n
	return py.None, nil
}

const loggerGetEffectiveLevel_doc = `getEffectiveLevel()

Get the effective level for this logger.`

func loggerGetEffectiveLevel(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Int(self.(*Logger).effectiveLevel()), nil
}

const loggerAddHandler_doc = `addHandler(hdlr)

Add the specified handler to this logger.`

func loggerAddHandler(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "addHandler", 1, 1); err != nil {
		return nil, err
	}
	h, ok := HandlerOf(args[0])
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "addHandler() argument must be a Handler")
	}
	l := self.(*Logger)
	for _, existing := range l.handlers {
		if existing == h {
			return py.None, nil
		}
	}
	l.handlers = append(l.handlers, h)
	return py.None, nil
}

const loggerRemoveHandler_doc = `removeHandler(hdlr)

Remove the specified handler from this logger.`

func loggerRemoveHandler(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "removeHandler", 1, 1); err != nil {
		return nil, err
	}
	h, ok := args[0].(*Handler)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "removeHandler() argument must be a Handler")
	}
	l := self.(*Logger)
	for i, existing := range l.handlers {
		if existing == h {
			l.handlers = append(l.handlers[:i], l.handlers[i+1:]...)
			break
		}
	}
	return py.None, nil
}

const handlerSetLevel_doc = `setLevel(level)

Set the logging level of this handler.`

// asHandler returns the Handler behind an object, which is NOT the object
// itself for a handler defined in another package or in Python.
//
// RotatingFileHandler and its siblings embed a *Handler and derive from
// logging.Handler, but their Go type is their own - so the bare type assertion
// panicked the host process with
// "interface conversion: py.Object is *handlers.rotatingFileHandler, not
// *logging.Handler" on an ordinary handler.setLevel(level) call.
func asHandler(self py.Object) (*Handler, bool) {
	switch h := self.(type) {
	case *Handler:
		return h, true
	case interface{ AsHandler() *Handler }:
		return h.AsHandler(), true
	}
	if payload, ok := py.PayloadOf(self); ok {
		return asHandler(payload)
	}
	return nil, false
}

// asHandlerOrNil returns the Handler behind an object, or nil when it is not
// one.  Returning the POINTER matters: callers assign through it.
func asHandlerOrNil(self py.Object) *Handler {
	h, _ := asHandler(self)
	return h
}

// handlerOf is asHandlerOrNil for the paths that cannot sensibly be reached
// without a handler, such as a handler's own property.
func handlerOf(self py.Object) *Handler {
	h, ok := asHandler(self)
	if !ok {
		panic(py.ExceptionNewf(py.TypeError, "not a Handler: %s", self.Type().Name))
	}
	return h
}

func handlerSetLevel(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "setLevel", 1, 1); err != nil {
		return nil, err
	}
	n, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	h, ok := asHandler(self)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "not a Handler: %s", self.Type().Name)
	}
	h.level = n
	return py.None, nil
}

const handlerHandle_doc = `handle(record)

Call the handlers for the record, honouring this handler's level.`

func handlerHandle(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "handle", 1, 1); err != nil {
		return nil, err
	}
	h, ok := HandlerOf(self)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "not a Handler")
	}
	rec, ok := args[0].(*LogRecord)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "a LogRecord is required")
	}
	if rec.LevelNo < h.level {
		return py.None, nil
	}
	// The object's OWN emit, so a subclass's emit runs - see callHandlers.
	emit, err := py.GetAttrString(self, "emit")
	if err != nil {
		return nil, err
	}
	return py.Call(emit, py.Tuple{rec}, py.NewStringDict())
}

const handlerSetFormatter_doc = `setFormatter(fmt)

Set the formatter for this handler.`

func handlerSetFormatter(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "setFormatter", 1, 1); err != nil {
		return nil, err
	}
	f, ok := args[0].(*Formatter)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "setFormatter() argument must be a Formatter")
	}
	asHandlerOrNil(self).formatter = f
	return py.None, nil
}

// defaultFormat returns the formatter to use when a handler has none.
func defaultFormat() *Formatter {
	return &Formatter{defaultFmt: true}
}

// format renders the record.  The default formatter is the CPython one:
// "LEVEL:name:message".
func (f *Formatter) format(record *LogRecord) (string, error) {
	if f == nil || f.defaultFmt {
		// A handler with NO formatter writes the bare message - "hello", not
		// "WARNING:t:hello".  That module-level shape belongs to basicConfig,
		// and applying it here changed the output of every handler that did
		// not set one, including the rotating ones.
		return record.getMessage()
	}
	return expandFormat(f.fmt, record)
}

// expandFormat renders a "%(field)s" style format string.  A literal "%%" is
// kept as a single "%".
func expandFormat(format string, record *LogRecord) (string, error) {
	var b strings.Builder
	for i := 0; i < len(format); {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			b.WriteByte('%')
			i += 2
			continue
		}
		if i+1 < len(format) && format[i+1] == '(' {
			end := strings.IndexByte(format[i+2:], ')')
			if end < 0 {
				return "", py.ExceptionNewf(py.ValueError, "incomplete format key")
			}
			field := format[i+2 : i+2+end]
			// The conversion spec runs to the end of the format character.
			j := i + 2 + end + 1
			for j < len(format) && strings.ContainsRune("-+ #0123456789.", rune(format[j])) {
				j++
			}
			if j >= len(format) {
				return "", py.ExceptionNewf(py.ValueError, "incomplete format")
			}
			conv := format[j]
			value, err := recordField(record, field)
			if err != nil {
				return "", err
			}
			// Only the CONVERSION goes to py.Mod - "%" plus flags, width and
			// the conversion character.  Passing the whole "%(field)s" asked
			// Python's % to format a plain value with a mapping spec, which
			// raised "format requires a mapping" instead of rendering the
			// field.
			convSpec := "%" + format[i+2+end+1:j+1]
			rendered, err := convertOne(value, conv, convSpec)
			if err != nil {
				return "", err
			}
			b.WriteString(rendered)
			i = j + 1
			continue
		}
		// A positional conversion cannot be resolved against a record, so it
		// is copied through untouched rather than mis-rendered.
		b.WriteByte(c)
		i++
	}
	return b.String(), nil
}

// recordField resolves a "%(name)s" field against the record.
func recordField(record *LogRecord, field string) (py.Object, error) {
	switch field {
	case "name":
		return py.String(record.Name), nil
	case "levelno":
		return py.Int(record.LevelNo), nil
	case "levelname":
		return py.String(effectiveLevelName(record.LevelNo)), nil
	case "message":
		return py.String(mustMessage(record)), nil
	case "msg":
		return record.Msg, nil
	case "pathname":
		return py.String(record.Pathname), nil
	case "filename":
		return py.String(record.Filename), nil
	case "module":
		return py.String(record.Module), nil
	case "lineno":
		return py.Int(record.Lineno), nil
	case "funcName":
		return py.String(record.FuncName), nil
	case "created":
		return py.Float(record.Created), nil
	}
	return nil, py.ExceptionNewf(py.KeyError, "%s", field)
}

// mustMessage renders the message; the error is reported by the caller of
// expandFormat, so here it degrades to the raw text.
func mustMessage(record *LogRecord) string {
	msg, err := record.getMessage()
	if err != nil {
		return ""
	}
	return msg
}

// convertOne applies a single "%"-conversion to one value by handing the
// work to the interpreter, which implements the numeric and string
// conversions itself.
func convertOne(value py.Object, conv byte, spec string) (string, error) {
	// A "s" conversion means str(value), not the interpreter's "%s" on a
	// non-string, which would fall back to Go formatting.
	if conv == 's' {
		text, err := py.StrAsString(value)
		if err != nil {
			return "", err
		}
		value = py.String(text)
	} else if conv == 'r' {
		text, err := py.ReprAsString(value)
		if err != nil {
			return "", err
		}
		value = py.String(text)
		conv = 's'
		spec = spec[:len(spec)-1] + "s"
	}
	rendered, err := py.Mod(py.String(spec), value)
	if err != nil {
		return "", err
	}
	text, err := py.StrAsString(rendered)
	if err != nil {
		return "", err
	}
	return text, nil
}

const emit_doc = `emit(record)

Do whatever it takes to actually log the specified logging record.`

func handlerEmit(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "emit", 1, 1); err != nil {
		return nil, err
	}
	record, ok := args[0].(*LogRecord)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "emit() argument must be a LogRecord")
	}
	h := asHandlerOrNil(self)
	if err := h.emit(h, record); err != nil {
		return nil, err
	}
	return py.None, nil
}

// streamEmit writes a formatted record to the handler's stream.
func streamEmit(h *Handler, record *LogRecord) error {
	if h.stream == nil || h.stream == py.None {
		return nil
	}
	text, err := h.formatterFor().format(record)
	if err != nil {
		return err
	}
	write, err := py.GetAttrString(h.stream, "write")
	if err != nil {
		return err
	}
	if _, err := py.Call(write, py.Tuple{py.String(text + "\n")}, py.StringDict{}); err != nil {
		return err
	}
	flush, err := py.GetAttrString(h.stream, "flush")
	if err != nil {
		return nil // A stream without flush is not an error.
	}
	if _, err := py.Call(flush, nil, py.StringDict{}); err != nil {
		return err
	}
	return nil
}

// formatterFor picks the handler's formatter or the default one.
func (h *Handler) formatterFor() *Formatter {
	if h.formatter != nil {
		return h.formatter
	}
	return defaultFormat()
}

// HandlerOf returns the *Handler behind any handler object.
//
// A logger stores *Handler, and logging.handlers' handlers EMBED one rather
// than being one - a Go embedding is not a subtype.  They expose their embedded
// pointer through this interface so a logger can hold and drive them, which is
// what makes "logger.addHandler(RotatingFileHandler(...))" work.
type HandlerProvider interface {
	LoggedHandler() *Handler
}

// HandlerOf unwraps a handler object: a *Handler is returned as-is, and
// anything implementing HandlerProvider yields the Handler it embeds.
// SetHandlerOwner records the object a Handler is embedded in, so the logger
// can call that object's own emit.  logging.handlers calls it at construction.
func SetHandlerOwner(h *Handler, owner py.Object) {
	h.owner = owner
}

func HandlerOf(o py.Object) (*Handler, bool) {
	if h, ok := o.(*Handler); ok {
		return h, true
	}
	if p, ok := o.(HandlerProvider); ok {
		return p.LoggedHandler(), true
	}
	return nil, false
}

// NewHandler builds a base Handler, which writes nowhere until a subclass
// supplies a destination.
//
// It is exported for logging.handlers, whose handlers EMBED a Handler so that
// handle(), setLevel(), setFormatter() and close() behave exactly as the parent
// module's do.  Constructing the struct directly from another package is not
// possible - its fields are unexported - so this is the way in.
func NewHandler() *Handler {
	return &Handler{level: NOTSET, emit: streamEmit}
}

// FormatRecord formats one record the way this handler would, including the
// trailing newline.
//
// It is exported for logging.handlers, so a rotating handler writes exactly
// what StreamHandler or FileHandler would - the same formatter, the same
// format string.  Duplicating the formatting there would let the two drift.
func FormatRecord(h *Handler, record py.Object) (string, error) {
	rec, ok := record.(*LogRecord)
	if !ok {
		return "", py.ExceptionNewf(py.TypeError, "a LogRecord is required, not %s", record.Type().Name)
	}
	text, err := h.formatterFor().format(rec)
	if err != nil {
		return "", err
	}
	return text + "\n", nil
}

// StreamHandler writes to a stream; FileHandler to a file it opens.
func newStreamHandler(stream py.Object) *Handler {
	h := &Handler{level: NOTSET, emit: streamEmit}
	h.stream = stream
	return h
}

const streamHandler_doc = `StreamHandler(stream=None)

Returns a new instance of the StreamHandler class.  If stream is specified,
the instance will use it for logging output; otherwise, sys.stderr will be
used.`

func streamHandler(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var stream py.Object = py.None
	kwlist := []string{"stream"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:StreamHandler", kwlist, &stream); err != nil {
		return nil, err
	}
	if stream == nil || stream == py.None {
		stream = stderrStream()
	}
	return newStreamHandler(stream), nil
}

const fileHandler_doc = `FileHandler(filename, mode='a', encoding=None, delay=False, errors=None)

Returns a new instance of the FileHandler class.  The specified file is
opened and used as the stream for logging.`

func fileHandler(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		filename py.Object
		mode     py.Object = py.String("a")
		encoding py.Object = py.None
	)
	kwlist := []string{"filename", "mode", "encoding"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:FileHandler", kwlist,
		&filename, &mode, &encoding); err != nil {
		return nil, err
	}
	open := py.Object(nil)
	if impl := py.GetModuleImplOrNil("io"); impl != nil {
		open = impl.Globals.GetOrNil("open")
	}
	if open == nil {
		builtins := py.GetModuleImplOrNil("builtins")
		if builtins != nil {
			open = builtins.Globals.GetOrNil("open")
		}
	}
	if open == nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "cannot open file handler stream: no io.open")
	}
	stream, err := py.Call(open, py.Tuple{filename, mode}, py.StringDict{})
	if err != nil {
		return nil, err
	}
	return newStreamHandler(stream), nil
}

// stderrStream finds sys.stderr through the running context, falling back to
// the module registered with the interpreter.
func stderrStream() py.Object {
	if impl := py.GetModuleImplOrNil("sys"); impl != nil {
		if s, ok := impl.Globals.Get("stderr"); ok {
			return s
		}
	}
	return py.None
}

const formatter_doc = `Formatter(fmt=None, datefmt=None, style='%', validate=True, *, defaults=None)

Initialize the formatter with specified format strings.`

func formatter(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		fmtObj  py.Object = py.None
		datefmt py.Object = py.None
		style   py.Object = py.String("%")
	)
	kwlist := []string{"fmt", "datefmt", "style"}
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOO:Formatter", kwlist,
		&fmtObj, &datefmt, &style); err != nil {
		return nil, err
	}
	f := &Formatter{}
	if fmtObj != nil && fmtObj != py.None {
		s, err := py.StrAsString(fmtObj)
		if err != nil {
			return nil, err
		}
		f.fmt = s
	} else {
		f.defaultFmt = true
	}
	if datefmt != nil && datefmt != py.None {
		s, err := py.StrAsString(datefmt)
		if err != nil {
			return nil, err
		}
		f.dateFmt = s
	}
	return f, nil
}

const basicConfig_doc = `basicConfig(**kwargs)

Do basic configuration for the logging system.

This function does nothing if the root logger already has handlers
configured, unless the keyword argument *force* is set to True.`

func basicConfig(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		filename py.Object = py.None
		filemode py.Object = py.String("a")
		format   py.Object = py.None
		datefmt  py.Object = py.None
		style    py.Object = py.String("%")
		level    py.Object = py.None
		stream   py.Object = py.None
		force    py.Object = py.False
		encoding py.Object = py.None
	)
	kwlist := []string{"filename", "filemode", "format", "datefmt", "style", "level", "stream", "force", "encoding"}
	if err := py.ParseTupleAndKeywords(nil, kwargs, "|$OOOOOOObOO:basicConfig", kwlist,
		&filename, &filemode, &format, &datefmt, &style, &level, &stream, &force, &encoding); err != nil {
		return nil, err
	}
	// datefmt/style/encoding are accepted for compatibility; the default
	// formatter is used and the stream is opened in text mode.
	_, _, _ = datefmt, style, encoding
	forced, err := py.ObjectIsTrue(force)
	if err != nil {
		return nil, err
	}
	if len(gRoot.handlers) > 0 && !forced {
		return py.None, nil
	}
	if forced {
		gRoot.handlers = nil
	}

	var handler *Handler
	switch {
	case filename != nil && filename != py.None:
		open := py.Object(nil)
		if impl := py.GetModuleImplOrNil("io"); impl != nil {
			open = impl.Globals.GetOrNil("open")
		}
		if open == nil {
			return nil, py.ExceptionNewf(py.RuntimeError, "basicConfig(filename=...) needs io.open")
		}
		f, err := py.Call(open, py.Tuple{filename, filemode}, py.StringDict{})
		if err != nil {
			return nil, err
		}
		handler = newStreamHandler(f)
	case stream != nil && stream != py.None:
		handler = newStreamHandler(stream)
	default:
		handler = newStreamHandler(stderrStream())
	}

	if format != nil && format != py.None {
		s, err := py.StrAsString(format)
		if err != nil {
			return nil, err
		}
		handler.formatter = &Formatter{fmt: s}
	}
	if level != nil && level != py.None {
		n, err := py.IndexInt(level)
		if err != nil {
			return nil, err
		}
		gRoot.level = n
	}
	gRoot.handlers = []*Handler{handler}
	return py.None, nil
}

// moduleLevel builds the module level debug()/info()/... functions, which
// forward to the root logger.
func moduleLevel(name string, level int) func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return loggerLog(gRoot, args, kwargs, level, name)
	}
}

const getLevelName_doc = `getLevelName(level)

Return the textual representation of logging level 'level'.`

func getLevelName(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "getLevelName", 1, 1); err != nil {
		return nil, err
	}
	if s, ok := args[0].(py.String); ok {
		for n, name := range levelNames {
			if name == string(s) {
				return py.Int(n), nil
			}
		}
		return py.String("Level " + string(s)), nil
	}
	n, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	if name, ok := levelNames[n]; ok {
		return py.String(name), nil
	}
	return py.String(fmt.Sprintf("Level %d", n)), nil
}

func init() {
	LoggerType.Dict.Set("addFilter", py.MustNewMethod("addFilter", func(self py.Object, args py.Tuple) (py.Object, error) {
		l, ok := self.(*Logger)
		if !ok || len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "addFilter() takes a filter")
		}
		for _, existing := range l.filters {
			if existing == args[0] {
				return py.None, nil
			}
		}
		l.filters = append(l.filters, args[0])
		return py.None, nil
	}, 0, "Add the specified filter to this logger."))

	LoggerType.Dict.Set("removeFilter", py.MustNewMethod("removeFilter", func(self py.Object, args py.Tuple) (py.Object, error) {
		l, ok := self.(*Logger)
		if !ok || len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "removeFilter() takes a filter")
		}
		for i, f := range l.filters {
			if f == args[0] {
				l.filters = append(l.filters[:i], l.filters[i+1:]...)
				break
			}
		}
		return py.None, nil
	}, 0, "Remove the specified filter from this logger."))

	// filter consults this logger's filters AND every ancestor's: a record is
	// dispatched up the chain, and each level may drop it.
	LoggerType.Dict.Set("filter", py.MustNewMethod("filter", func(self py.Object, args py.Tuple) (py.Object, error) {
		l, ok := self.(*Logger)
		if !ok || len(args) < 1 {
			return py.True, nil
		}
		rec, ok := args[0].(*LogRecord)
		if !ok {
			return py.True, nil
		}
		for logger := l; logger != nil; logger = logger.parent {
			for _, f := range logger.filters {
				passes, err := runFilter(f, rec)
				if err != nil {
					return nil, err
				}
				if !passes {
					return py.False, nil
				}
			}
		}
		return py.True, nil
	}, 0, "Run this logger's filters and its ancestors'; a record is dropped when any returns false."))

	LoggerType.Dict.Set("debug", py.MustNewMethod("debug", loggerDebug, 0, "Log 'msg % args' with severity 'DEBUG'."))
	LoggerType.Dict.Set("info", py.MustNewMethod("info", loggerInfo, 0, "Log 'msg % args' with severity 'INFO'."))
	LoggerType.Dict.Set("warning", py.MustNewMethod("warning", loggerWarning, 0, "Log 'msg % args' with severity 'WARNING'."))
	LoggerType.Dict.Set("warn", py.MustNewMethod("warn", loggerWarning, 0, "Log 'msg % args' with severity 'WARNING'."))
	LoggerType.Dict.Set("error", py.MustNewMethod("error", loggerError, 0, "Log 'msg % args' with severity 'ERROR'."))
	LoggerType.Dict.Set("critical", py.MustNewMethod("critical", loggerCritical, 0, "Log 'msg % args' with severity 'CRITICAL'."))
	LoggerType.Dict.Set("fatal", py.MustNewMethod("fatal", loggerCritical, 0, "Log 'msg % args' with severity 'CRITICAL'."))
	LoggerType.Dict.Set("exception", py.MustNewMethod("exception", loggerException, 0, loggerException_doc))
	LoggerType.Dict.Set("log", py.MustNewMethod("log", loggerLogMethod, 0, loggerLog_doc))
	LoggerType.Dict.Set("isEnabledFor", py.MustNewMethod("isEnabledFor", loggerIsEnabledFor, 0, loggerIsEnabledFor_doc))
	LoggerType.Dict.Set("setLevel", py.MustNewMethod("setLevel", loggerSetLevel, 0, loggerSetLevel_doc))
	LoggerType.Dict.Set("getEffectiveLevel", py.MustNewMethod("getEffectiveLevel", loggerGetEffectiveLevel, 0, loggerGetEffectiveLevel_doc))
	LoggerType.Dict.Set("addHandler", py.MustNewMethod("addHandler", loggerAddHandler, 0, loggerAddHandler_doc))
	LoggerType.Dict.Set("removeHandler", py.MustNewMethod("removeHandler", loggerRemoveHandler, 0, loggerRemoveHandler_doc))

	// logging.Filter.  pip subclasses it - "class ExcludeLoggerFilter(Filter)" -
	// and calls super().filter(record), so the base implementation has to be a
	// real method on a real type.
	filterType.Dict.Set("__init__", py.MustNewMethod("__init__", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		f, ok := self.(*filterObj)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a Filter")
		}
		if len(args) > 0 {
			f.name, _ = py.StrAsString(args[0])
		}
		if v, ok := kwargs.Get("name"); ok {
			f.name, _ = py.StrAsString(v)
		}
		return py.None, nil
	}, 0, "__init__(name='')"))

	filterType.Dict.Set("filter", py.MustNewMethod("filter", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "filter() takes a record")
		}
		rec, ok := args[0].(*LogRecord)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "a LogRecord is required")
		}
		name, _ := filterNameOf(self)
		return py.NewBool(filterMatches(name, rec.Name)), nil
	}, 0, "Filter a record: pass it when it came from the named logger or a child."))

	filterType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			name, _ := filterNameOf(self)
			return py.String(name), nil
		},
		Fset: func(self py.Object, value py.Object) error {
			if f, ok := self.(*filterObj); ok {
				f.name, _ = py.StrAsString(value)
			}
			return nil
		},
		Doc: "The name of the logger this filter admits.",
	})

	HandlerType.Dict.Set("addFilter", py.MustNewMethod("addFilter", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := HandlerOf(self)
		if !ok || len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "addFilter() takes a filter")
		}
		// The filter object is KEPT AS GIVEN, not unwrapped: a Python subclass's
		// filter() override is what has to run.
		for _, existing := range h.filters {
			if existing == args[0] {
				return py.None, nil
			}
		}
		h.filters = append(h.filters, args[0])
		return py.None, nil
	}, 0, "Add the specified filter to this handler."))

	HandlerType.Dict.Set("removeFilter", py.MustNewMethod("removeFilter", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := HandlerOf(self)
		if !ok || len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "removeFilter() takes a filter")
		}
		for i, f := range h.filters {
			if f == args[0] {
				h.filters = append(h.filters[:i], h.filters[i+1:]...)
				break
			}
		}
		return py.None, nil
	}, 0, "Remove the specified filter from this handler."))

	HandlerType.Dict.Set("filter", py.MustNewMethod("filter", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := HandlerOf(self)
		if !ok || len(args) < 1 {
			return py.True, nil
		}
		rec, ok := args[0].(*LogRecord)
		if !ok {
			return py.True, nil
		}
		for _, f := range h.filters {
			passes, err := runFilter(f, rec)
			if err != nil {
				return nil, err
			}
			if !passes {
				return py.False, nil
			}
		}
		return py.True, nil
	}, 0, "Run every filter; a record is dropped when any returns false."))

	// A handler's close() and flush() are what logging.shutdown calls, and what
	// a program calls to release a file early.  They were absent entirely, so
	// "logging.shutdown()" - which pip calls on the way out - raised
	// AttributeError on the module.
	HandlerType.Dict.Set("close", py.MustNewMethod("close", func(self py.Object, args py.Tuple) (py.Object, error) {
		if c, ok := self.(interface{ Close() error }); ok {
			if err := c.Close(); err != nil {
				return nil, err
			}
			return py.None, nil
		}
		// A handler with no Go close, or a Python subclass whose close() has
		// already run: nothing to release.
		return py.None, nil
	}, 0, "Tidy up any resources used by the handler."))

	HandlerType.Dict.Set("flush", py.MustNewMethod("flush", func(self py.Object, args py.Tuple) (py.Object, error) {
		if f, ok := self.(interface{ Flush() error }); ok {
			if err := f.Flush(); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}, 0, "Ensure all logging output has been flushed."))

	HandlerType.Dict.Set("setLevel", py.MustNewMethod("setLevel", handlerSetLevel, 0, handlerSetLevel_doc))
	HandlerType.Dict.Set("setFormatter", py.MustNewMethod("setFormatter", handlerSetFormatter, 0, handlerSetFormatter_doc))
	HandlerType.Dict.Set("emit", py.MustNewMethod("emit", handlerEmit, 0, emit_doc))
	// A LogRecord's attributes are read directly - "record.levelno",
	// "record.name", "record.msg" - by formatters, filters and handlers, and
	// none of them existed: only the Go struct had them.  pygments' logging
	// filter reads record.levelno, and "%(levelname)s" resolution needs the
	// rest.
	LogRecordType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if r, ok := self.(*LogRecord); ok {
				return py.String(r.Name), nil
			}
			return py.None, nil
		},
		Doc: "The name of the logger that created this record.",
	})
	LogRecordType.Dict.Set("levelno", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if r, ok := self.(*LogRecord); ok {
				return py.Int(r.LevelNo), nil
			}
			return py.None, nil
		},
		Doc: "The numeric level of this record.",
	})
	LogRecordType.Dict.Set("levelname", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if r, ok := self.(*LogRecord); ok {
				return py.String(effectiveLevelName(r.LevelNo)), nil
			}
			return py.None, nil
		},
		Doc: "The text name of this record's level.",
	})
	LogRecordType.Dict.Set("msg", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if r, ok := self.(*LogRecord); ok && r.Msg != nil {
				return r.Msg, nil
			}
			return py.None, nil
		},
		Doc: "The format string, before its arguments are applied.",
	})
	LogRecordType.Dict.Set("args", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if r, ok := self.(*LogRecord); ok && r.Args != nil {
				return r.Args, nil
			}
			return py.None, nil
		},
		Doc: "The arguments to apply to the format string.",
	})
	LogRecordType.Dict.Set("pathname", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if r, ok := self.(*LogRecord); ok {
				return py.String(r.Pathname), nil
			}
			return py.None, nil
		},
		Doc: "The full path of the source file.",
	})
	LogRecordType.Dict.Set("lineno", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if r, ok := self.(*LogRecord); ok {
				return py.Int(r.Lineno), nil
			}
			return py.None, nil
		},
		Doc: "The line number in the source file.",
	})
	LogRecordType.Dict.Set("funcName", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if r, ok := self.(*LogRecord); ok {
				return py.String(r.FuncName), nil
			}
			return py.None, nil
		},
		Doc: "The name of the function that logged this.",
	})
	LogRecordType.Dict.Set("getMessage", py.MustNewMethod("getMessage", func(self py.Object, args py.Tuple) (py.Object, error) {
		r, ok := self.(*LogRecord)
		if !ok {
			return py.None, nil
		}
		msg, err := r.getMessage()
		if err != nil {
			return nil, err
		}
		return py.String(msg), nil
	}, 0, "Return the message after applying the record's arguments."))

	// Formatter.format is public API - a config sets a format string and a
	// program calls format(record) - and it was reachable only from Go, so
	// "logging.Formatter(...).format(rec)" raised AttributeError.
	FormatterType.Dict.Set("format", py.MustNewMethod("format", func(self py.Object, args py.Tuple) (py.Object, error) {
		f, ok := self.(*Formatter)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a Formatter")
		}
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "format() takes a record")
		}
		rec, ok := args[0].(*LogRecord)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "a LogRecord is required")
		}
		text, err := f.format(rec)
		if err != nil {
			return nil, err
		}
		return py.String(text), nil
	}, 0, "Format a record, returning the formatted string."))

	HandlerType.Dict.Set("handle", py.MustNewMethod("handle", handlerHandle, 0, handlerHandle_doc))
	HandlerType.Dict.Set("level", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.Int(handlerOf(self).level), nil },
		Doc:  "the handler's level",
	})
	// The stream an output handler writes to, as a read-only attribute.
	streamProp := &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return handlerOf(self).stream, nil },
		Doc:  "the stream this handler writes to",
	}
	HandlerType.Dict.Set("stream", streamProp)
	HandlerType.Dict.Set("name", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(""), nil },
		Doc:  "the handler's name, if any",
	})

	LoggerType.Dict.Set("level", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.Int(self.(*Logger).level), nil },
		Fset: func(self, value py.Object) error {
			n, err := py.IndexInt(value)
			if err != nil {
				return err
			}
			self.(*Logger).level = n
			return nil
		},
		Doc: "the logger's level",
	})
	LoggerType.Dict.Set("disabled", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.False, nil },
		Doc:  "whether this logger is disabled; never in this implementation",
	})
	LoggerType.Dict.Set("propagate", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.Bool(self.(*Logger).propagate), nil },
		Fset: func(self, value py.Object) error {
			b, err := py.ObjectIsTrue(value)
			if err != nil {
				return err
			}
			self.(*Logger).propagate = b
			return nil
		},
		Doc: "whether records propagate to ancestor loggers",
	})
	LoggerType.Dict.Set("handlers", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			l := py.NewList()
			for _, h := range self.(*Logger).handlers {
				l.Append(h)
			}
			return l, nil
		},
		Doc: "the handlers of this logger",
	})

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "logging",
			Doc:  logging_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("getLogger", getLogger, 0, getLogger_doc),
			py.MustNewMethod("setLoggerClass", setLoggerClass, 0, `setLoggerClass(klass)

Set the class to be used when instantiating a logger.  The class should
define __init__ such that it requires only a name argument.`),
			py.MustNewMethod("getLoggerClass", getLoggerClass, 0, "Return the class to be used when instantiating a logger."),
			py.MustNewMethod("addLevelName", addLevelName, 0, "Associate levelName with level."),
			py.MustNewMethod("getLevelName", getLevelName, 0, "Return the textual or numeric representation of logging level 'level'."),
			py.MustNewMethod("basicConfig", basicConfig, 0, basicConfig_doc),
			py.MustNewMethod("debug", moduleLevel("debug", DEBUG), 0, "Log 'msg % args' with severity 'DEBUG'."),
			py.MustNewMethod("info", moduleLevel("info", INFO), 0, "Log 'msg % args' with severity 'INFO'."),
			py.MustNewMethod("warning", moduleLevel("warning", WARNING), 0, "Log 'msg % args' with severity 'WARNING'."),
			py.MustNewMethod("warn", moduleLevel("warn", WARNING), 0, "Log 'msg % args' with severity 'WARNING'."),
			py.MustNewMethod("error", moduleLevel("error", ERROR), 0, "Log 'msg % args' with severity 'ERROR'."),
			py.MustNewMethod("critical", moduleLevel("critical", CRITICAL), 0, "Log 'msg % args' with severity 'CRITICAL'."),
			py.MustNewMethod("fatal", moduleLevel("fatal", CRITICAL), 0, "Log 'msg % args' with severity 'CRITICAL'."),
			py.MustNewMethod("exception", moduleLevel("exception", ERROR), 0, loggerException_doc),
			py.MustNewMethod("log", moduleLevelLog, 0, loggerLog_doc),
			py.MustNewMethod("getLevelName", getLevelName, 0, getLevelName_doc),
			py.MustNewMethod("disable", disable, 0, disable_doc),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "DEBUG", Value: py.Int(DEBUG)},
			py.DictEntry{Key: "shutdown", Value: py.MustNewMethod("shutdown", loggingShutdown, 0, shutdown_doc)},
			py.DictEntry{Key: "INFO", Value: py.Int(INFO)},
			py.DictEntry{Key: "WARNING", Value: py.Int(WARNING)},
			py.DictEntry{Key: "WARN", Value: py.Int(WARNING)},
			py.DictEntry{Key: "ERROR", Value: py.Int(ERROR)},
			py.DictEntry{Key: "CRITICAL", Value: py.Int(CRITICAL)},
			py.DictEntry{Key: "FATAL", Value: py.Int(CRITICAL)},
			py.DictEntry{Key: "NOTSET", Value: py.Int(NOTSET)},
			py.DictEntry{Key: "Logger", Value: LoggerType},
			py.DictEntry{Key: "LogRecord", Value: LogRecordType},
			py.DictEntry{Key: "Filter", Value: filterType},
			py.DictEntry{Key: "Handler", Value: HandlerType},
			py.DictEntry{Key: "StreamHandler", Value: StreamHandlerType},
			py.DictEntry{Key: "FileHandler", Value: FileHandlerType},
			py.DictEntry{Key: "NullHandler", Value: NullHandlerType},
			py.DictEntry{Key: "Formatter", Value: FormatterType},
			py.DictEntry{Key: "root", Value: gRoot},
			py.DictEntry{Key: "BASIC_FORMAT", Value: py.String("%(levelname)s:%(name)s:%(message)s")},
			py.DictEntry{Key: "raiseExceptions", Value: py.True},
			py.DictEntry{Key: "lastResort", Value: py.None},
		),
	})
}

const disable_doc = `disable(level=CRITICAL)

Disable all logging calls of severity 'level' and below.`

func disable(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var level py.Object = py.Int(CRITICAL)
	if err := checkArgs(args, kwargs, "disable", 0, 1); err != nil {
		return nil, err
	}
	if len(args) > 0 {
		level = args[0]
	}
	n, err := py.IndexInt(level)
	if err != nil {
		return nil, err
	}
	gRoot.level = n + 1
	return py.None, nil
}

const moduleLog_doc = `log(level, msg, *args, **kwargs)

Log 'msg % args' with the integer severity 'level' on the root logger.`

func moduleLevelLog(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return loggerLogMethod(gRoot, args, kwargs)
}

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

const setLoggerClass_doc = `setLoggerClass(klass)

Set the class to be used when instantiating a logger.  The class should
define __init__ such that it requires only a name argument.`

func setLoggerClass(self py.Object, args py.Tuple) (py.Object, error) {
	var klass py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "setLoggerClass", 1, 1, &klass); err != nil {
		return nil, err
	}
	t, ok := klass.(*py.Type)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "setLoggerClass() argument must be a class")
	}
	gLoggerClass = t
	return py.None, nil
}

func getLoggerClass(self py.Object, args py.Tuple) (py.Object, error) {
	if gLoggerClass != nil {
		return gLoggerClass, nil
	}
	return LoggerType, nil
}
