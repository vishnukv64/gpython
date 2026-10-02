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

var LogRecordType = py.NewType("logging.LogRecord", "A LogRecord instance represents an event being logged.")

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
type Handler struct {
	level     int
	formatter *Formatter
	// stream is where a stream-based handler writes.
	stream py.Object
	// emit is the output function; a subclass replaces it.
	emit func(h *Handler, record *LogRecord) error
}

var HandlerType = py.NewTypeX("logging.Handler",
	"Handler instances dispatch logging events to specific destinations.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		// The base Handler writes nowhere; a subclass such as StreamHandler
		// supplies the destination.  Constructing it directly is legal, as
		// in CPython.
		h := &Handler{level: NOTSET, emit: streamEmit}
		return h, nil
	}, nil)

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
}

var LoggerType = py.NewType("logging.Logger", "A Logger is a named logging channel.")

func (l *Logger) Type() *py.Type { return LoggerType }

func (l *Logger) M__repr__() (py.Object, error) {
	return py.String(fmt.Sprintf("<Logger %s (%s)>", l.name, effectiveLevelName(l.level))), nil
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

func newLogger(name string, level int) *Logger {
	return &Logger{name: name, level: level, propagate: true}
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
func (l *Logger) callHandlers(record *LogRecord) error {
	for logger := l; logger != nil; logger = logger.parent {
		for _, h := range logger.handlers {
			if record.LevelNo < h.level {
				continue
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
	h, ok := args[0].(*Handler)
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

func handlerSetLevel(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if err := checkArgs(args, kwargs, "setLevel", 1, 1); err != nil {
		return nil, err
	}
	n, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	self.(*Handler).level = n
	return py.None, nil
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
	self.(*Handler).formatter = f
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
		msg, err := record.getMessage()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s:%s:%s", effectiveLevelName(record.LevelNo), record.Name, msg), nil
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
			rendered, err := convertOne(value, conv, format[i:j+1])
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
	h := self.(*Handler)
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
	if _, err := py.Call(write, py.Tuple{py.String(text + "\n")}, nil); err != nil {
		return err
	}
	flush, err := py.GetAttrString(h.stream, "flush")
	if err != nil {
		return nil // A stream without flush is not an error.
	}
	if _, err := py.Call(flush, nil, nil); err != nil {
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
		open = impl.Globals["open"]
	}
	if open == nil {
		builtins := py.GetModuleImplOrNil("builtins")
		if builtins != nil {
			open = builtins.Globals["open"]
		}
	}
	if open == nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "cannot open file handler stream: no io.open")
	}
	stream, err := py.Call(open, py.Tuple{filename, mode}, nil)
	if err != nil {
		return nil, err
	}
	return newStreamHandler(stream), nil
}

// stderrStream finds sys.stderr through the running context, falling back to
// the module registered with the interpreter.
func stderrStream() py.Object {
	if impl := py.GetModuleImplOrNil("sys"); impl != nil {
		if s, ok := impl.Globals["stderr"]; ok {
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
			open = impl.Globals["open"]
		}
		if open == nil {
			return nil, py.ExceptionNewf(py.RuntimeError, "basicConfig(filename=...) needs io.open")
		}
		f, err := py.Call(open, py.Tuple{filename, filemode}, nil)
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
	LoggerType.Dict["debug"] = py.MustNewMethod("debug", loggerDebug, 0, "Log 'msg % args' with severity 'DEBUG'.")
	LoggerType.Dict["info"] = py.MustNewMethod("info", loggerInfo, 0, "Log 'msg % args' with severity 'INFO'.")
	LoggerType.Dict["warning"] = py.MustNewMethod("warning", loggerWarning, 0, "Log 'msg % args' with severity 'WARNING'.")
	LoggerType.Dict["warn"] = py.MustNewMethod("warn", loggerWarning, 0, "Log 'msg % args' with severity 'WARNING'.")
	LoggerType.Dict["error"] = py.MustNewMethod("error", loggerError, 0, "Log 'msg % args' with severity 'ERROR'.")
	LoggerType.Dict["critical"] = py.MustNewMethod("critical", loggerCritical, 0, "Log 'msg % args' with severity 'CRITICAL'.")
	LoggerType.Dict["fatal"] = py.MustNewMethod("fatal", loggerCritical, 0, "Log 'msg % args' with severity 'CRITICAL'.")
	LoggerType.Dict["exception"] = py.MustNewMethod("exception", loggerException, 0, loggerException_doc)
	LoggerType.Dict["log"] = py.MustNewMethod("log", loggerLogMethod, 0, loggerLog_doc)
	LoggerType.Dict["isEnabledFor"] = py.MustNewMethod("isEnabledFor", loggerIsEnabledFor, 0, loggerIsEnabledFor_doc)
	LoggerType.Dict["setLevel"] = py.MustNewMethod("setLevel", loggerSetLevel, 0, loggerSetLevel_doc)
	LoggerType.Dict["getEffectiveLevel"] = py.MustNewMethod("getEffectiveLevel", loggerGetEffectiveLevel, 0, loggerGetEffectiveLevel_doc)
	LoggerType.Dict["addHandler"] = py.MustNewMethod("addHandler", loggerAddHandler, 0, loggerAddHandler_doc)
	LoggerType.Dict["removeHandler"] = py.MustNewMethod("removeHandler", loggerRemoveHandler, 0, loggerRemoveHandler_doc)

	HandlerType.Dict["setLevel"] = py.MustNewMethod("setLevel", handlerSetLevel, 0, handlerSetLevel_doc)
	HandlerType.Dict["setFormatter"] = py.MustNewMethod("setFormatter", handlerSetFormatter, 0, handlerSetFormatter_doc)
	HandlerType.Dict["emit"] = py.MustNewMethod("emit", handlerEmit, 0, emit_doc)
	HandlerType.Dict["level"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.Int(self.(*Handler).level), nil },
		Doc:  "the handler's level",
	}
	// The stream an output handler writes to, as a read-only attribute.
	streamProp := &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return self.(*Handler).stream, nil },
		Doc:  "the stream this handler writes to",
	}
	HandlerType.Dict["stream"] = streamProp
	HandlerType.Dict["name"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.String(""), nil },
		Doc:  "the handler's name, if any",
	}

	LoggerType.Dict["level"] = &py.Property{
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
	}
	LoggerType.Dict["disabled"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.False, nil },
		Doc:  "whether this logger is disabled; never in this implementation",
	}
	LoggerType.Dict["propagate"] = &py.Property{
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
	}
	LoggerType.Dict["handlers"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			l := py.NewList()
			for _, h := range self.(*Logger).handlers {
				l.Append(h)
			}
			return l, nil
		},
		Doc: "the handlers of this logger",
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "logging",
			Doc:  logging_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("getLogger", getLogger, 0, getLogger_doc),
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
		Globals: py.StringDict{
			"DEBUG":           py.Int(DEBUG),
			"INFO":            py.Int(INFO),
			"WARNING":         py.Int(WARNING),
			"WARN":            py.Int(WARNING),
			"ERROR":           py.Int(ERROR),
			"CRITICAL":        py.Int(CRITICAL),
			"FATAL":           py.Int(CRITICAL),
			"NOTSET":          py.Int(NOTSET),
			"Logger":          LoggerType,
			"LogRecord":       LogRecordType,
			"Handler":         HandlerType,
			"StreamHandler":   StreamHandlerType,
			"FileHandler":     FileHandlerType,
			"NullHandler":     NullHandlerType,
			"Formatter":       FormatterType,
			"root":            gRoot,
			"BASIC_FORMAT":    py.String("%(levelname)s:%(name)s:%(message)s"),
			"raiseExceptions": py.True,
			"lastResort":      py.None,
		},
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
	if len(kwargs) != 0 {
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
