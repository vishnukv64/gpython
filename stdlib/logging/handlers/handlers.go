// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package handlers implements logging.handlers: the handlers that do more than
// write to a stream.
//
// pip imports this module - "import logging.handlers" in
// pip/_internal/utils/logging.py - without using any name from it, so the
// module existing at all is what unblocks it.  What is implemented is the part
// that can be exact:
//
//   - RotatingFileHandler, rotating on size;
//   - TimedRotatingFileHandler, rotating on a time interval;
//   - MemoryHandler / BufferingHandler, buffering records until a condition;
//   - Handler, StreamHandler, FileHandler and NullHandler re-exported from the
//     parent module, so logging.handlers.StreamHandler IS logging.StreamHandler.
//
// What is NOT implemented raises at construction rather than accepting the call
// and dropping records: SocketHandler, DatagramHandler, SysLogHandler,
// SMTPHandler, NTEventLogHandler and HTTPHandler each need a socket, a mail
// transport or a platform log service.  A handler that silently swallows a log
// line is the worst outcome available, so these say what is missing.
//
// Each handler EMBEDS a *logging.Handler, so handle(), setLevel(),
// setFormatter() and close() are the parent module's, and the logger can drive
// it.  Because a Go embedding is not a subtype, the embedded pointer is
// registered as the handler's "owner" so the logger calls the SUBCLASS's emit.

package handlers

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/logging"
)

const module_doc = `Additional handlers for the logging package for Python.

This module provides RotatingFileHandler, TimedRotatingFileHandler and
MemoryHandler.  Handlers that need a socket, a mail transport or a platform log
service are not provided, and raise when constructed rather than discarding
records.
`

// rotatingFileHandler rotates a log file once it reaches maxBytes.
type rotatingFileHandler struct {
	*logging.Handler
	filename    string
	mode        string
	maxBytes    int64
	backupCount int
}

// timedRotatingFileHandler rolls over on a time interval.
type timedRotatingFileHandler struct {
	*logging.Handler
	filename    string
	when        string
	interval    int64
	backupCount int
	utc         bool
}

// memoryHandler buffers records and flushes them to a target.
type memoryHandler struct {
	*logging.Handler
	capacity     int
	buffer       []py.Object
	target       py.Object
	flushLevel   int
	flushOnClose bool
}

var rotatingFileHandlerType = py.NewTypeX("logging.handlers.RotatingFileHandler",
	"A handler that rotates a log file once it reaches a size.", newRotatingFileHandler, nil)
var timedRotatingFileHandlerType = py.NewTypeX("logging.handlers.TimedRotatingFileHandler",
	"A handler that rotates a log file on a time interval.", newTimedRotatingFileHandler, nil)
var memoryHandlerType = py.NewTypeX("logging.handlers.MemoryHandler",
	"A handler that buffers records and flushes them to a target.", newMemoryHandler, nil)

// AsHandler exposes the embedded handler, so a shared helper in the logging
// package can reach a derived handler's level and formatter without asserting
// the derived Go type - which it cannot know.
func (h *rotatingFileHandler) AsHandler() *logging.Handler      { return h.Handler }
func (h *timedRotatingFileHandler) AsHandler() *logging.Handler { return h.Handler }
func (h *memoryHandler) AsHandler() *logging.Handler            { return h.Handler }

func (h *rotatingFileHandler) Type() *py.Type      { return rotatingFileHandlerType }
func (h *timedRotatingFileHandler) Type() *py.Type { return timedRotatingFileHandlerType }
func (h *memoryHandler) Type() *py.Type            { return memoryHandlerType }

// LoggedHandler exposes the embedded Handler so a logger can hold and drive
// this object - see logging.HandlerOf.
func (h *rotatingFileHandler) LoggedHandler() *logging.Handler { return h.Handler }
func (h *timedRotatingFileHandler) LoggedHandler() *logging.Handler {
	return h.Handler
}
func (h *memoryHandler) LoggedHandler() *logging.Handler { return h.Handler }

// intArg reads a positional int argument, or its keyword twin.
func intArg(o py.Object, what string) int64 {
	v, ok := o.(py.Int)
	if !ok {
		return 0
	}
	n, _ := v.GoInt64()
	return n
}

// strArg reads a string argument, or returns the fallback.
func strArg(o py.Object, fallback string) string {
	s, err := py.StrAsString(o)
	if err != nil {
		return fallback
	}
	return s
}

// kwOr returns the keyword value when present, else the positional one.
func kwOr(args py.Tuple, idx int, kwargs py.StringDict, name string) (py.Object, bool) {
	if v, ok := kwargs.Get(name); ok {
		return v, true
	}
	if idx < len(args) {
		return args[idx], true
	}
	return nil, false
}

// fileSize returns the size of a file, or 0 when it does not exist.
func fileSize(name string) int64 {
	st, err := os.Stat(name)
	if err != nil {
		return 0
	}
	return st.Size()
}

// rotateBackups renames the log files up the chain, as CPython does: the
// oldest backup is discarded and each other moves up one, so the live file
// becomes .1.  A rename is used rather than a copy, so no record is lost.
func rotateBackups(filename string, backupCount int) error {
	if backupCount <= 0 {
		return nil
	}
	if err := os.Remove(fmt.Sprintf("%s.%d", filename, backupCount)); err != nil && !os.IsNotExist(err) {
		return err
	}
	for i := backupCount - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", filename, i)
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.Rename(from, fmt.Sprintf("%s.%d", filename, i+1)); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filename); err != nil {
		return nil
	}
	return os.Rename(filename, filename+".1")
}

// appendLine writes one line to a file, creating it if needed.
func appendLine(filename, line string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

// unsupported makes a handler that cannot work raise at construction.
func unsupported(name, needs string) py.Object {
	return py.NewTypeX("logging.handlers."+name, "Not implemented: "+needs+".",
		func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			return nil, py.ExceptionNewf(py.NotImplementedError,
				"logging.handlers.%s is not available: gpython has no %s, and a handler that dropped records silently would be worse than one that refuses",
				name, needs)
		}, nil)
}

func newRotatingFileHandler(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	h := &rotatingFileHandler{Handler: logging.NewHandler(), mode: "a"}
	logging.SetHandlerOwner(h.Handler, h)
	// The arguments are applied HERE rather than in __init__, because the type
	// is registered with a nil Init - a plain construction would otherwise
	// leave the handler with no filename, which showed up as "open : no such
	// file or directory" on the first write.
	if len(args) == 0 {
		if _, ok := kwargs.Get("filename"); !ok {
			return nil, py.ExceptionNewf(py.TypeError, "__init__() missing required argument: 'filename'")
		}
	}
	if v, ok := kwOr(args, 0, kwargs, "filename"); ok {
		h.filename = strArg(v, "")
	}
	if v, ok := kwOr(args, 1, kwargs, "mode"); ok {
		h.mode = strArg(v, "a")
	}
	if v, ok := kwOr(args, 2, kwargs, "maxBytes"); ok {
		h.maxBytes = intArg(v, "maxBytes")
	}
	if v, ok := kwOr(args, 3, kwargs, "backupCount"); ok {
		h.backupCount = int(intArg(v, "backupCount"))
		if h.backupCount < 0 {
			return nil, py.ExceptionNewf(py.ValueError, "backupCount must be greater than zero")
		}
	}
	return h, nil
}

func newTimedRotatingFileHandler(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	h := &timedRotatingFileHandler{Handler: logging.NewHandler(), when: "h", interval: 1}
	logging.SetHandlerOwner(h.Handler, h)
	if len(args) == 0 {
		if _, ok := kwargs.Get("filename"); !ok {
			return nil, py.ExceptionNewf(py.TypeError, "__init__() missing required argument: 'filename'")
		}
	}
	if v, ok := kwOr(args, 0, kwargs, "filename"); ok {
		h.filename = strArg(v, "")
	}
	if v, ok := kwOr(args, 1, kwargs, "when"); ok {
		// CPython matches the interval spec case-INSENSITIVELY.
		h.when = strings.ToLower(strArg(v, "h"))
	}
	if v, ok := kwOr(args, 2, kwargs, "interval"); ok {
		h.interval = intArg(v, "interval")
	}
	if v, ok := kwOr(args, 3, kwargs, "backupCount"); ok {
		h.backupCount = int(intArg(v, "backupCount"))
	}
	if v, ok := kwOr(args, 6, kwargs, "utc"); ok {
		h.utc = v == py.True
	}
	switch h.when {
	case "s", "m", "h", "d", "midnight", "w0", "w1", "w2", "w3", "w4", "w5", "w6":
	default:
		return nil, py.ExceptionNewf(py.ValueError, "Invalid rollover interval specified: %s", h.when)
	}
	return h, nil
}

func newMemoryHandler(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	h := &memoryHandler{Handler: logging.NewHandler(), flushLevel: logging.ERROR, flushOnClose: true}
	logging.SetHandlerOwner(h.Handler, h)
	if v, ok := kwOr(args, 0, kwargs, "target"); ok {
		h.target = v
	}
	if v, ok := kwOr(args, 1, kwargs, "capacity"); ok {
		h.capacity = int(intArg(v, "capacity"))
	}
	if v, ok := kwOr(args, 2, kwargs, "flushLevel"); ok {
		h.flushLevel = int(intArg(v, "flushLevel"))
	}
	if v, ok := kwargs.Get("flushOnClose"); ok {
		h.flushOnClose = v == py.True
	}
	return h, nil
}

// unitSeconds is the length of one interval unit for this handler's "when".
func (h *timedRotatingFileHandler) unitSeconds() int64 {
	switch h.when {
	case "s":
		return 1
	case "m":
		return 60
	case "h":
		return 3600
	case "d", "midnight":
		return 86400
	}
	// A "wN" interval is a week.
	return 7 * 86400
}

// nextRollover is CPython's computeRollover: the moment the next rollover is
// due after now.
func (h *timedRotatingFileHandler) nextRollover(now float64) float64 {
	loc := time.Local
	if h.utc {
		loc = time.UTC
	}
	t := time.Unix(int64(now), 0).In(loc)
	switch h.when {
	case "midnight":
		return float64(time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc).Unix())
	case "s", "m", "h", "d":
		step := h.unitSeconds() * h.interval
		if step <= 0 {
			return now
		}
		// The next multiple of the step since the epoch.
		return float64(int64(now) - int64(now)%step + step)
	}
	// A weekday: the next occurrence of day N at midnight.
	target := int(h.when[1] - '0')
	for i := 1; i <= 7; i++ {
		d := t.AddDate(0, 0, i)
		if int(d.Weekday()) == target {
			return float64(time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc).Unix())
		}
	}
	return now
}

// flush sends every buffered record to the target, KEEPING the ones that fail.
//
// Dropping a record that could not be delivered is how a log line disappears
// with nothing to show for it, so a failure leaves it in the buffer.
func flush(h *memoryHandler) (py.Object, error) {
	if h.target == nil || h.target == py.None {
		h.buffer = h.buffer[:0]
		return py.None, nil
	}
	handle, err := py.GetAttrString(h.target, "handle")
	if err != nil {
		return nil, err
	}
	pending := h.buffer[:0]
	for _, rec := range h.buffer {
		if _, cerr := py.Call(handle, py.Tuple{rec}, py.NewStringDict()); cerr != nil {
			pending = append(pending, rec)
		}
	}
	h.buffer = pending
	return py.None, nil
}

func init() {
	// These handlers must DERIVE from logging.Handler, so that the methods
	// registered there - setLevel, setFormatter, addFilter, filter, handle,
	// close, flush and the level property - are inherited.
	//
	// They were separate types with no base at all, and pip's setup_logging
	// calls handler.setLevel: "'RotatingFileHandler' object has no attribute
	// 'setLevel'".
	for _, t := range []*py.Type{
		rotatingFileHandlerType, timedRotatingFileHandlerType, memoryHandlerType,
	} {
		if t != nil && t.Base == nil && logging.HandlerType != nil {
			t.Base = logging.HandlerType
			t.Bases = py.Tuple{logging.HandlerType}
			if err := t.Ready(); err != nil {
				panic(err)
			}
		}
	}

	globals := py.NewStringDict()
	globals.Set("__doc__", py.String(module_doc))

	globals.Set("DEFAULT_TCP_LOGGING_PORT", py.Int(9020))
	globals.Set("DEFAULT_UDP_LOGGING_PORT", py.Int(9021))
	globals.Set("DEFAULT_HTTP_LOGGING_PORT", py.Int(9022))
	globals.Set("DEFAULT_SOAP_LOGGING_PORT", py.Int(9023))
	globals.Set("SYSLOG_UDP_PORT", py.Int(514))
	globals.Set("SYSLOG_TCP_PORT", py.Int(514))
	globals.Set("MIDNIGHT", py.String("midnight"))
	globals.Set("_MIDNIGHT", py.String("midnight"))

	// The parent module's classes are NOT re-exported.  CPython's
	// logging.handlers has none of Handler, StreamHandler, FileHandler or
	// NullHandler - measured, not assumed - and exporting them would make
	// "hasattr(logging.handlers, 'StreamHandler')" answer differently from
	// CPython, which is exactly the kind of difference that makes a port
	// program behave unexpectedly.

	globals.Set("RotatingFileHandler", rotatingFileHandlerType)
	globals.Set("TimedRotatingFileHandler", timedRotatingFileHandlerType)
	globals.Set("MemoryHandler", memoryHandlerType)
	globals.Set("BufferingHandler", memoryHandlerType)

	rotatingFileHandlerType.Dict.Set("emit", py.MustNewMethod("emit", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := self.(*rotatingFileHandler)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a RotatingFileHandler")
		}
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "emit() takes a record")
		}
		line, err := logging.FormatRecord(h.Handler, args[0])
		if err != nil {
			return nil, err
		}
		if h.maxBytes > 0 && fileSize(h.filename)+int64(len(line)) >= h.maxBytes {
			if err := rotateBackups(h.filename, h.backupCount); err != nil {
				return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
			}
		}
		if err := appendLine(h.filename, line); err != nil {
			return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
		}
		return py.None, nil
	}, 0, "Emit a record, rotating the file first if it is due."))

	rotatingFileHandlerType.Dict.Set("doRollover", py.MustNewMethod("doRollover", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := self.(*rotatingFileHandler)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a RotatingFileHandler")
		}
		if err := rotateBackups(h.filename, h.backupCount); err != nil {
			return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
		}
		return py.None, nil
	}, 0, "Rotate the log files."))

	rotatingFileHandlerType.Dict.Set("shouldRollover", py.MustNewMethod("shouldRollover", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := self.(*rotatingFileHandler)
		if !ok || h.maxBytes <= 0 {
			return py.False, nil
		}
		return py.NewBool(fileSize(h.filename) >= h.maxBytes), nil
	}, 0, "Check whether the file should be rotated before the record is emitted."))

	// The rotating handlers expose their configuration, as CPython's do.
	rotatingFileHandlerType.Dict.Set("baseFilename", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if h, ok := self.(*rotatingFileHandler); ok {
				return py.String(h.filename), nil
			}
			return py.None, nil
		},
		Doc: "The name of the file this handler writes to.",
	})
	timedRotatingFileHandlerType.Dict.Set("baseFilename", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if h, ok := self.(*timedRotatingFileHandler); ok {
				return py.String(h.filename), nil
			}
			return py.None, nil
		},
		Doc: "The name of the file this handler writes to.",
	})

	timedRotatingFileHandlerType.Dict.Set("computeRollover", py.MustNewMethod("computeRollover", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := self.(*timedRotatingFileHandler)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a TimedRotatingFileHandler")
		}
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "computeRollover() needs a timestamp")
		}
		now, err := py.FloatAsFloat64(args[0])
		if err != nil {
			return nil, err
		}
		return py.Float(h.nextRollover(now)), nil
	}, 0, "Work out the rollover time based on the specified time."))

	timedRotatingFileHandlerType.Dict.Set("emit", py.MustNewMethod("emit", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := self.(*timedRotatingFileHandler)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a TimedRotatingFileHandler")
		}
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "emit() takes a record")
		}
		line, err := logging.FormatRecord(h.Handler, args[0])
		if err != nil {
			return nil, err
		}
		// The rollover is checked at EMIT time, which is what makes the handler
		// work for a process that stays up for longer than one interval.
		window := time.Duration(h.interval*h.unitSeconds()) * time.Second
		if st, serr := os.Stat(h.filename); serr == nil && time.Since(st.ModTime()) >= window {
			if err := rotateBackups(h.filename, h.backupCount); err != nil {
				return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
			}
		}
		if err := appendLine(h.filename, line); err != nil {
			return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
		}
		return py.None, nil
	}, 0, "Emit a record, rolling the file over when the interval has elapsed."))

	memoryHandlerType.Dict.Set("emit", py.MustNewMethod("emit", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := self.(*memoryHandler)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a MemoryHandler")
		}
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "emit() takes a record")
		}
		h.buffer = append(h.buffer, args[0])
		if h.capacity > 0 && len(h.buffer) >= h.capacity {
			return flush(h)
		}
		return py.None, nil
	}, 0, "Append the record, flushing when the buffer is full."))

	memoryHandlerType.Dict.Set("flush", py.MustNewMethod("flush", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := self.(*memoryHandler)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a MemoryHandler")
		}
		return flush(h)
	}, 0, "Send the buffered records to the target, then clear the buffer."))

	memoryHandlerType.Dict.Set("shouldFlush", py.MustNewMethod("shouldFlush", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := self.(*memoryHandler)
		if !ok || len(args) < 1 {
			return py.False, nil
		}
		if h.capacity > 0 && len(h.buffer) >= h.capacity {
			return py.True, nil
		}
		levelVal, err := py.GetAttrString(args[0], "levelno")
		if err != nil {
			return py.False, nil
		}
		if int(intArg(levelVal, "levelno")) >= h.flushLevel {
			return py.True, nil
		}
		return py.False, nil
	}, 0, "Check whether the buffer should be flushed."))

	memoryHandlerType.Dict.Set("close", py.MustNewMethod("close", func(self py.Object, args py.Tuple) (py.Object, error) {
		h, ok := self.(*memoryHandler)
		if !ok {
			return py.None, nil
		}
		if h.flushOnClose {
			return flush(h)
		}
		return py.None, nil
	}, 0, "Flush if configured to, then close."))

	memoryHandlerType.Dict.Set("buffer", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			h, ok := self.(*memoryHandler)
			if !ok {
				return py.NewList(), nil
			}
			out := py.NewList()
			for _, r := range h.buffer {
				out.Append(r)
			}
			return out, nil
		},
		Fset: func(self py.Object, value py.Object) error {
			h, ok := self.(*memoryHandler)
			if !ok {
				return py.ExceptionNewf(py.TypeError, "not a MemoryHandler")
			}
			h.buffer = nil
			if l, ok := value.(*py.List); ok {
				h.buffer = append(h.buffer, l.Items...)
			}
			return nil
		},
		Doc: "The records held, not yet flushed.",
	})

	globals.Set("SocketHandler", unsupported("SocketHandler", "socket-based log transport"))
	globals.Set("DatagramHandler", unsupported("DatagramHandler", "UDP log transport"))
	globals.Set("SysLogHandler", unsupported("SysLogHandler", "a syslog service"))
	globals.Set("SMTPHandler", unsupported("SMTPHandler", "a mail transport"))
	globals.Set("NTEventLogHandler", unsupported("NTEventLogHandler", "the Windows event log"))
	globals.Set("HTTPHandler", unsupported("HTTPHandler", "an HTTP client for log delivery"))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "logging.handlers",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
