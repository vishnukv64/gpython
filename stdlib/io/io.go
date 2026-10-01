// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package io provides the implementation of python's 'io' module: the
// in-memory stream types and the base classes of the I/O hierarchy.
//
// BytesIO and StringIO are complete: they hold their data in memory and
// support the read/write/seek/tell/getvalue protocol that code uses to
// build or read a stream without touching a file.
//
// The base classes - IOBase, RawIOBase, BufferedIOBase, TextIOBase - exist
// as real classes so that a class may derive from them and so that
// isinstance tests answer, but they add no behaviour of their own.  The
// classes that wrap a real file (TextIOWrapper, BufferedReader,
// BufferedWriter) do not wrap anything here: this interpreter's file
// objects come from open() and already behave as text or binary streams.
package io

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `The io module provides the Python interfaces to stream handling.

At the top of the I/O hierarchy is the abstract base class IOBase.  It
defines the basic interface to a stream.`

var UnsupportedOperationType = py.ExceptionType.NewType("io.UnsupportedOperation", "Raised when an unsupported operation is called on a stream.", nil, nil)

// The base classes.  They are ordinary classes that can be inherited from
// and tested against; the behaviour lives in the concrete types.
var (
	IOBaseType         = py.NewType("io.IOBase", "The abstract base class for all I/O classes.")
	RawIOBaseType      = py.NewType("io.RawIOBase", "Base class for raw binary I/O.")
	BufferedIOBaseType = py.NewType("io.BufferedIOBase", "Base class for buffered binary streams.")
	TextIOBaseType     = py.NewType("io.TextIOBase", "Base class for text streams.")
)

func init() {
	IOBaseType.Flags |= py.TPFLAGS_BASETYPE
	RawIOBaseType.Flags |= py.TPFLAGS_BASETYPE
	BufferedIOBaseType.Flags |= py.TPFLAGS_BASETYPE
	TextIOBaseType.Flags |= py.TPFLAGS_BASETYPE
	RawIOBaseType.Base = IOBaseType
	BufferedIOBaseType.Base = IOBaseType
	TextIOBaseType.Base = IOBaseType

	// close/closed/flush are common to every stream, so they live on the
	// base class and the concrete types override what they need to.
	IOBaseType.Dict["closed"] = &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if c, ok := self.(py.I__enter__); ok {
				_ = c
			}
			if s, ok := self.(interface{ isClosed() bool }); ok {
				return py.NewBool(s.isClosed()), nil
			}
			return py.False, nil
		},
	}
	IOBaseType.Dict["close"] = py.MustNewMethod("close", func(self py.Object, args py.Tuple) (py.Object, error) {
		if s, ok := self.(interface{ doClose() error }); ok {
			if err := s.doClose(); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}, 0, "Flush and close this stream.")
	IOBaseType.Dict["flush"] = py.MustNewMethod("flush", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "Flush the write buffers of the stream if applicable.")
	IOBaseType.Dict["seekable"] = py.MustNewMethod("seekable", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.False, nil
	}, 0, "Return whether the stream supports random access.")
	IOBaseType.Dict["readable"] = py.MustNewMethod("readable", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.False, nil
	}, 0, "Return whether the stream can be read.")
	IOBaseType.Dict["writable"] = py.MustNewMethod("writable", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.False, nil
	}, 0, "Return whether the stream can be written.")
	IOBaseType.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Return the stream itself.")
	IOBaseType.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
		if s, ok := self.(interface{ doClose() error }); ok {
			if err := s.doClose(); err != nil {
				return nil, err
			}
		}
		return py.False, nil
	}, 0, "Close the stream.")

	globals := py.StringDict{
		"IOBase":               IOBaseType,
		"RawIOBase":            RawIOBaseType,
		"BufferedIOBase":       BufferedIOBaseType,
		"TextIOBase":           TextIOBaseType,
		"BytesIO":              BytesIOType,
		"StringIO":             StringIOType,
		"UnsupportedOperation": UnsupportedOperationType,
		"SEEK_SET":             py.Int(0),
		"SEEK_CUR":             py.Int(1),
		"SEEK_END":             py.Int(2),
		"DEFAULT_BUFFER_SIZE":  py.Int(8192),
		"BufferedReader":       RawIOBaseType,
		"BufferedWriter":       RawIOBaseType,
		"BufferedRandom":       RawIOBaseType,
		"TextIOWrapper":        TextIOBaseType,
		"FileIO":               RawIOBaseType,
		"open":                 py.MustNewMethod("open", pyOpen, 0, "Open a file and return a stream."),
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "io",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// pyOpen delegates to the builtin open, which is the same function in
// CPython: io.open is open.
func pyOpen(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	openFn, err := py.GetModuleImplOrNil("builtins"), error(nil)
	_ = openFn
	if err != nil {
		return nil, err
	}
	builtins := py.GetModuleImplOrNil("builtins")
	if builtins == nil {
		return nil, py.ExceptionNewf(py.RuntimeError, "builtins is not available")
	}
	fn, err := py.GetAttrString(builtins, "open")
	if err != nil {
		return nil, err
	}
	return py.Call(fn, args, kwargs)
}

// ---------------------------------------------------------------------------
// BytesIO

const bytesio_doc = `BytesIO([initial_bytes]) -> a binary stream using an in-memory bytes buffer.

It inherits from BufferedIOBase.  The buffer is discarded when the close()
method is called.`

// BytesIO is an in-memory binary stream.
type BytesIO struct {
	data   []byte
	pos    int
	closed bool
}

var BytesIOType = py.NewTypeX("io.BytesIO", bytesio_doc, func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var initial py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:BytesIO", []string{"initial_bytes"}, &initial); err != nil {
		return nil, err
	}
	b := &BytesIO{}
	if initial != py.None {
		switch v := initial.(type) {
		case py.Bytes:
			b.data = append([]byte{}, v...)
		case py.String:
			b.data = []byte(v)
		default:
			return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not '%s'", initial.Type().Name)
		}
	}
	return b, nil
}, nil)

func (b *BytesIO) Type() *py.Type { return BytesIOType }

func (b *BytesIO) isClosed() bool { return b.closed }

func (b *BytesIO) doClose() error {
	b.closed = true
	b.data = nil
	b.pos = 0
	return nil
}

func (b *BytesIO) checkOpen() error {
	if b.closed {
		return py.ExceptionNewf(py.ValueError, "I/O operation on closed file.")
	}
	return nil
}

// readBytes reads up to n bytes, or all of them when n is negative.
func (b *BytesIO) readBytes(n int) []byte {
	if b.pos >= len(b.data) {
		return nil
	}
	if n < 0 || b.pos+n > len(b.data) {
		n = len(b.data) - b.pos
	}
	out := make([]byte, n)
	copy(out, b.data[b.pos:b.pos+n])
	b.pos += n
	return out
}

func init() {
	BytesIOType.Dict["read"] = py.MustNewMethod("read", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		size := py.Object(py.Int(-1))
		if err := py.UnpackTuple(args, nil, "read", 0, 1, &size); err != nil {
			return nil, err
		}
		n, err := py.IndexInt(size)
		if err != nil {
			return nil, err
		}
		return py.Bytes(b.readBytes(n)), nil
	}, 0, "Read up to size bytes.")

	BytesIOType.Dict["read1"] = BytesIOType.Dict["read"]

	BytesIOType.Dict["readline"] = py.MustNewMethod("readline", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		if b.pos >= len(b.data) {
			return py.Bytes(nil), nil
		}
		idx := indexByte(b.data[b.pos:], '\n')
		if idx < 0 {
			return py.Bytes(b.readBytes(-1)), nil
		}
		return py.Bytes(b.readBytes(idx + 1)), nil
	}, 0, "Read one line.")

	BytesIOType.Dict["readlines"] = py.MustNewMethod("readlines", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		items := []py.Object{}
		for {
			if b.pos >= len(b.data) {
				break
			}
			idx := indexByte(b.data[b.pos:], '\n')
			if idx < 0 {
				items = append(items, py.Bytes(b.readBytes(-1)))
				break
			}
			items = append(items, py.Bytes(b.readBytes(idx+1)))
		}
		return py.NewListFromItems(items), nil
	}, 0, "Read all lines into a list.")

	BytesIOType.Dict["readinto"] = py.MustNewMethod("readinto", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		var target py.Object
		if err := py.UnpackTuple(args, nil, "readinto", 1, 1, &target); err != nil {
			return nil, err
		}
		// bytearray does not exist in this interpreter, so there is no
		// writable in-place buffer to fill.  Say so rather than accepting
		// the argument and doing nothing.
		_ = target
		return nil, py.ExceptionNewf(UnsupportedOperationType, "readinto() needs a preallocated writable buffer, and bytearray is not available here")
	}, 0, "Read bytes into a preallocated buffer.")

	BytesIOType.Dict["write"] = py.MustNewMethod("write", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		var value py.Object
		if err := py.UnpackTuple(args, nil, "write", 1, 1, &value); err != nil {
			return nil, err
		}
		var chunk []byte
		switch v := value.(type) {
		case py.Bytes:
			chunk = []byte(v)
		case py.String:
			return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not 'str'")
		default:
			return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not '%s'", value.Type().Name)
		}
		// Writing past the end extends the buffer, as a real stream does.
		if b.pos+len(chunk) > len(b.data) {
			grown := make([]byte, b.pos+len(chunk))
			copy(grown, b.data)
			b.data = grown
		}
		copy(b.data[b.pos:], chunk)
		b.pos += len(chunk)
		return py.Int(len(chunk)), nil
	}, 0, "Write bytes to the buffer.")

	BytesIOType.Dict["getvalue"] = py.MustNewMethod("getvalue", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		out := make([]byte, len(b.data))
		copy(out, b.data)
		return py.Bytes(out), nil
	}, 0, "Return the bytes the buffer holds.")

	BytesIOType.Dict["seek"] = py.MustNewMethod("seek", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		var offset py.Object
		whence := py.Object(py.Int(0))
		if err := py.UnpackTuple(args, nil, "seek", 1, 2, &offset, &whence); err != nil {
			return nil, err
		}
		off, err := py.IndexInt(offset)
		if err != nil {
			return nil, err
		}
		wh, err := py.IndexInt(whence)
		if err != nil {
			return nil, err
		}
		var newPos int
		switch wh {
		case 0:
			newPos = off
		case 1:
			newPos = b.pos + off
		case 2:
			newPos = len(b.data) + off
		default:
			return nil, py.ExceptionNewf(py.ValueError, "invalid whence (%d, should be 0, 1 or 2)", wh)
		}
		if newPos < 0 {
			return nil, py.ExceptionNewf(py.ValueError, "negative seek position %d", newPos)
		}
		b.pos = newPos
		return py.Int(b.pos), nil
	}, 0, "Change the stream position.")

	BytesIOType.Dict["tell"] = py.MustNewMethod("tell", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		return py.Int(b.pos), nil
	}, 0, "Return the current stream position.")

	BytesIOType.Dict["truncate"] = py.MustNewMethod("truncate", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		size := py.Object(py.None)
		if err := py.UnpackTuple(args, nil, "truncate", 0, 1, &size); err != nil {
			return nil, err
		}
		n := b.pos
		if size != py.None {
			v, err := py.IndexInt(size)
			if err != nil {
				return nil, err
			}
			n = v
		}
		if n < len(b.data) {
			b.data = b.data[:n]
		}
		return py.Int(len(b.data)), nil
	}, 0, "Truncate the buffer.")

	BytesIOType.Dict["getbuffer"] = py.MustNewMethod("getbuffer", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		out := make([]byte, len(b.data))
		copy(out, b.data)
		return py.Bytes(out), nil
	}, 0, "Return a readable view of the buffer.")

	BytesIOType.Dict["readable"] = py.MustNewMethod("readable", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.True, nil
	}, 0, "A BytesIO is always readable.")
	BytesIOType.Dict["writable"] = py.MustNewMethod("writable", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.True, nil
	}, 0, "A BytesIO is always writable.")
	BytesIOType.Dict["seekable"] = py.MustNewMethod("seekable", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.True, nil
	}, 0, "A BytesIO is always seekable.")
	BytesIOType.Dict["isatty"] = py.MustNewMethod("isatty", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.False, nil
	}, 0, "A BytesIO is never a terminal.")
	BytesIOType.Dict["fileno"] = py.MustNewMethod("fileno", func(self py.Object, args py.Tuple) (py.Object, error) {
		return nil, py.ExceptionNewf(UnsupportedOperationType, "fileno")
	}, 0, "An in-memory stream has no file descriptor.")
	BytesIOType.Dict["__iter__"] = py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Iterate over the lines.")
	BytesIOType.Dict["__next__"] = py.MustNewMethod("__next__", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		if err := b.checkOpen(); err != nil {
			return nil, err
		}
		if b.pos >= len(b.data) {
			return nil, py.StopIteration
		}
		idx := indexByte(b.data[b.pos:], '\n')
		if idx < 0 {
			return py.Bytes(b.readBytes(-1)), nil
		}
		return py.Bytes(b.readBytes(idx + 1)), nil
	}, 0, "Return the next line.")
	BytesIOType.Dict["__repr__"] = py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		b := self.(*BytesIO)
		return py.String("<_io.BytesIO at " + reprAddr(b) + ">"), nil
	}, 0, "Return repr(self).")
}

// ---------------------------------------------------------------------------
// StringIO

const stringio_doc = `StringIO([initial_value[, newline]]) -> a text stream using an in-memory
text buffer.

It inherits from TextIOBase.`

// StringIO is an in-memory text stream.
type StringIO struct {
	data   strings.Builder
	pos    int
	closed bool
}

var StringIOType = py.NewTypeX("io.StringIO", stringio_doc, func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var initial py.Object = py.String("")
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:StringIO", []string{"initial_value"}, &initial); err != nil {
		return nil, err
	}
	s := &StringIO{}
	if initial != py.None {
		text, err := py.StrAsString(initial)
		if err != nil {
			return nil, err
		}
		s.data.WriteString(text)
	}
	return s, nil
}, nil)

func (s *StringIO) Type() *py.Type { return StringIOType }

func (s *StringIO) text() string { return s.data.String() }

func (s *StringIO) isClosed() bool { return s.closed }

func (s *StringIO) doClose() error {
	s.closed = true
	s.data.Reset()
	s.pos = 0
	return nil
}

func (s *StringIO) checkOpen() error {
	if s.closed {
		return py.ExceptionNewf(py.ValueError, "I/O operation on closed file.")
	}
	return nil
}

// writeAt replaces the buffer with the value, so that a write past the end
// extends it as a stream does.
func (s *StringIO) writeAt(text string) {
	full := s.text()
	var b strings.Builder
	if s.pos <= len(full) {
		b.WriteString(full[:s.pos])
	} else {
		b.WriteString(full)
		b.WriteString(strings.Repeat("\x00", s.pos-len(full)))
	}
	b.WriteString(text)
	if s.pos+len(text) < len(full) {
		b.WriteString(full[s.pos+len(text):])
	}
	s.data.Reset()
	s.data.WriteString(b.String())
	s.pos += len(text)
}

func init() {
	StringIOType.Dict["write"] = py.MustNewMethod("write", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*StringIO)
		if err := s.checkOpen(); err != nil {
			return nil, err
		}
		var value py.Object
		if err := py.UnpackTuple(args, nil, "write", 1, 1, &value); err != nil {
			return nil, err
		}
		text, err := py.StrAsString(value)
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "string argument expected, got '%s'", value.Type().Name)
		}
		s.writeAt(text)
		return py.Int(len(text)), nil
	}, 0, "Write a string to the buffer.")

	StringIOType.Dict["read"] = py.MustNewMethod("read", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*StringIO)
		if err := s.checkOpen(); err != nil {
			return nil, err
		}
		size := py.Object(py.Int(-1))
		if err := py.UnpackTuple(args, nil, "read", 0, 1, &size); err != nil {
			return nil, err
		}
		n, err := py.IndexInt(size)
		if err != nil {
			return nil, err
		}
		full := s.text()
		if s.pos >= len(full) {
			return py.String(""), nil
		}
		if n < 0 || s.pos+n > len(full) {
			n = len(full) - s.pos
		}
		out := full[s.pos : s.pos+n]
		s.pos += n
		return py.String(out), nil
	}, 0, "Read up to size characters.")

	StringIOType.Dict["readline"] = py.MustNewMethod("readline", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*StringIO)
		if err := s.checkOpen(); err != nil {
			return nil, err
		}
		full := s.text()
		if s.pos >= len(full) {
			return py.String(""), nil
		}
		idx := strings.IndexByte(full[s.pos:], '\n')
		if idx < 0 {
			out := full[s.pos:]
			s.pos = len(full)
			return py.String(out), nil
		}
		out := full[s.pos : s.pos+idx+1]
		s.pos += idx + 1
		return py.String(out), nil
	}, 0, "Read one line.")

	StringIOType.Dict["readlines"] = py.MustNewMethod("readlines", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*StringIO)
		if err := s.checkOpen(); err != nil {
			return nil, err
		}
		items := []py.Object{}
		for {
			line, err := StringIOType.Dict["readline"].(*py.Method).Call(self, py.Tuple{})
			if err != nil {
				return nil, err
			}
			if line == py.String("") {
				break
			}
			items = append(items, line)
		}
		return py.NewListFromItems(items), nil
	}, 0, "Read all lines into a list.")

	StringIOType.Dict["getvalue"] = py.MustNewMethod("getvalue", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*StringIO)
		if err := s.checkOpen(); err != nil {
			return nil, err
		}
		return py.String(s.text()), nil
	}, 0, "Return the string the buffer holds.")

	StringIOType.Dict["seek"] = py.MustNewMethod("seek", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*StringIO)
		if err := s.checkOpen(); err != nil {
			return nil, err
		}
		var offset py.Object
		whence := py.Object(py.Int(0))
		if err := py.UnpackTuple(args, nil, "seek", 1, 2, &offset, &whence); err != nil {
			return nil, err
		}
		off, err := py.IndexInt(offset)
		if err != nil {
			return nil, err
		}
		wh, err := py.IndexInt(whence)
		if err != nil {
			return nil, err
		}
		switch wh {
		case 0:
			s.pos = off
		case 1:
			s.pos += off
		case 2:
			s.pos = len(s.text()) + off
		default:
			return nil, py.ExceptionNewf(py.ValueError, "invalid whence (%d, should be 0, 1 or 2)", wh)
		}
		if s.pos < 0 {
			return nil, py.ExceptionNewf(py.ValueError, "negative seek position %d", s.pos)
		}
		return py.Int(s.pos), nil
	}, 0, "Change the stream position.")

	StringIOType.Dict["tell"] = py.MustNewMethod("tell", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*StringIO)
		if err := s.checkOpen(); err != nil {
			return nil, err
		}
		return py.Int(s.pos), nil
	}, 0, "Return the current stream position.")

	StringIOType.Dict["truncate"] = py.MustNewMethod("truncate", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*StringIO)
		if err := s.checkOpen(); err != nil {
			return nil, err
		}
		size := py.Object(py.None)
		if err := py.UnpackTuple(args, nil, "truncate", 0, 1, &size); err != nil {
			return nil, err
		}
		n := s.pos
		if size != py.None {
			v, err := py.IndexInt(size)
			if err != nil {
				return nil, err
			}
			n = v
		}
		full := s.text()
		if n < len(full) {
			s.data.Reset()
			s.data.WriteString(full[:n])
		}
		return py.Int(n), nil
	}, 0, "Truncate the buffer.")

	StringIOType.Dict["readable"] = py.MustNewMethod("readable", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.True, nil
	}, 0, "A StringIO is always readable.")
	StringIOType.Dict["writable"] = py.MustNewMethod("writable", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.True, nil
	}, 0, "A StringIO is always writable.")
	StringIOType.Dict["seekable"] = py.MustNewMethod("seekable", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.True, nil
	}, 0, "A StringIO is always seekable.")
	StringIOType.Dict["isatty"] = py.MustNewMethod("isatty", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.False, nil
	}, 0, "A StringIO is never a terminal.")
	StringIOType.Dict["__iter__"] = py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "Iterate over the lines.")
	StringIOType.Dict["__next__"] = py.MustNewMethod("__next__", func(self py.Object, args py.Tuple) (py.Object, error) {
		line, err := StringIOType.Dict["readline"].(*py.Method).Call(self, py.Tuple{})
		if err != nil {
			return nil, err
		}
		if line == py.String("") {
			return nil, py.StopIteration
		}
		return line, nil
	}, 0, "Return the next line.")
	StringIOType.Dict["__repr__"] = py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String("<_io.StringIO at " + reprAddr(self) + ">"), nil
	}, 0, "Return repr(self).")
}

// ---------------------------------------------------------------------------
// helpers

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// reprAddr renders the address of an object the way the repr of a stream
// does.  The address is not exposed as a number here, so a placeholder is
// used: nothing depends on the value, only on the shape of the repr.
func reprAddr(obj py.Object) string {
	_ = obj
	return "0x0"
}

// closed, close and flush are registered on each concrete type: a method
// found through the base class's Dict is not reliably consulted for the
// instance here, so the concrete type carries its own.
func init() {
	for _, t := range []*py.Type{BytesIOType, StringIOType} {
		t.Dict["closed"] = &py.Property{
			Fget: func(self py.Object) (py.Object, error) {
				if c, ok := self.(interface{ isClosed() bool }); ok {
					return py.NewBool(c.isClosed()), nil
				}
				return py.False, nil
			},
		}
		t.Dict["close"] = py.MustNewMethod("close", func(self py.Object, args py.Tuple) (py.Object, error) {
			if c, ok := self.(interface{ doClose() error }); ok {
				if err := c.doClose(); err != nil {
					return nil, err
				}
			}
			return py.None, nil
		}, 0, "Close the stream.")
		t.Dict["__enter__"] = py.MustNewMethod("__enter__", func(self py.Object, args py.Tuple) (py.Object, error) {
			return self, nil
		}, 0, "Return the stream itself.")
		t.Dict["__exit__"] = py.MustNewMethod("__exit__", func(self py.Object, args py.Tuple) (py.Object, error) {
			if c, ok := self.(interface{ doClose() error }); ok {
				if err := c.doClose(); err != nil {
					return nil, err
				}
			}
			return py.False, nil
		}, 0, "Close the stream.")
	}
}

// ---------------------------------------------------------------------------
// Go interface bridges
//
// The VM reaches __iter__, __next__, __enter__ and __exit__ through the Go
// interfaces rather than through the type's Dict, so each stream implements
// them directly.  The assertions at the bottom make an omission a compile
// error rather than a runtime surprise.

func (b *BytesIO) M__iter__() (py.Object, error) { return b, nil }

func (b *BytesIO) M__next__() (py.Object, error) {
	if err := b.checkOpen(); err != nil {
		return nil, err
	}
	if b.pos >= len(b.data) {
		return nil, py.StopIteration
	}
	idx := indexByte(b.data[b.pos:], '\n')
	if idx < 0 {
		return py.Bytes(b.readBytes(-1)), nil
	}
	return py.Bytes(b.readBytes(idx + 1)), nil
}

func (b *BytesIO) M__enter__() (py.Object, error) { return b, nil }

func (b *BytesIO) M__exit__(excType, excValue, traceback py.Object) (py.Object, error) {
	return py.False, b.doClose()
}

func (s *StringIO) M__iter__() (py.Object, error) { return s, nil }

func (s *StringIO) M__next__() (py.Object, error) {
	line, err := StringIOType.Dict["readline"].(*py.Method).Call(s, py.Tuple{})
	if err != nil {
		return nil, err
	}
	if line == py.String("") {
		return nil, py.StopIteration
	}
	return line, nil
}

func (s *StringIO) M__enter__() (py.Object, error) { return s, nil }

func (s *StringIO) M__exit__(excType, excValue, traceback py.Object) (py.Object, error) {
	return py.False, s.doClose()
}

func (b *BytesIO) M__repr__() (py.Object, error) {
	return py.String("<_io.BytesIO at " + reprAddr(b) + ">"), nil
}

func (s *StringIO) M__repr__() (py.Object, error) {
	return py.String("<_io.StringIO at " + reprAddr(s) + ">"), nil
}

var (
	_ py.I__iter__  = (*BytesIO)(nil)
	_ py.I__next__  = (*BytesIO)(nil)
	_ py.I__enter__ = (*BytesIO)(nil)
	_ py.I__exit__  = (*BytesIO)(nil)
	_ py.I__repr__  = (*BytesIO)(nil)
	_ py.I__iter__  = (*StringIO)(nil)
	_ py.I__next__  = (*StringIO)(nil)
	_ py.I__enter__ = (*StringIO)(nil)
	_ py.I__exit__  = (*StringIO)(nil)
	_ py.I__repr__  = (*StringIO)(nil)
)
