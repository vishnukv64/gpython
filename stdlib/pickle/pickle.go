// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pickle provides python's 'pickle' module: serialisation of a Python
// object graph.
//
// It implements protocol 0, the original ASCII protocol.  dumps/loads and
// dump/load work over None, True, False, int (including the arbitrary-precision
// int), float, complex, str, bytes, bytearray, list, tuple, dict, set and
// frozenset, nested arbitrarily, with the memo (PUT/GET) so a shared or
// self-referential container survives the round trip as one object.
//
// The byte stream is CPython's own, opcode for opcode - the forms were read off
// pickletools.dis of CPython's output - so a pickle written here loads in
// CPython and a protocol-0 pickle written by CPython loads here.
//
// NOT supported, and said plainly rather than stubbed: pickling an instance of
// an arbitrary class.  That needs the __reduce__/__reduce_ex__ protocol and a
// stable importable name for the class, neither of which this interpreter
// provides; such a value raises PicklingError.  Protocol 1 and 2 output is not
// emitted either (dumps accepts the argument and writes protocol 0, which
// every CPython reads).
package pickle

import (
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Create portable serialized representations of Python objects.

See module copyreg for a mechanism for registering custom picklers.
See https://docs.python.org/3/library/pickle.html for the format.

This implementation writes and reads protocol 0 over the base and container
types.`

// PicklingError is raised for a value the format cannot represent; it derives
// from Exception, as CPython's does.
var PicklingError = py.ExceptionType.NewType("pickle.PicklingError", "Error raised when an unpicklable object is encountered by Pickler.", nil, nil)

// UnpicklingError is raised for input that is not a valid pickle.
var UnpicklingError = py.ExceptionType.NewType("pickle.UnpicklingError", "Raised when there is an error while unpickling.", nil, nil)

// Protocol numbers, as CPython names them.
const (
	HIGHEST_PROTOCOL = 5
	DEFAULT_PROTOCOL = 4
)

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "pickle",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("dumps", dumps, 0, dumps_doc),
			py.MustNewMethod("loads", loads, 0, loads_doc),
			py.MustNewMethod("dump", dump, 0, dump_doc),
			py.MustNewMethod("load", load, 0, load_doc),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "PicklingError", Value: PicklingError},
			py.DictEntry{Key: "UnpicklingError", Value: UnpicklingError},
			py.DictEntry{Key: "PickleError", Value: PicklingError},
			py.DictEntry{Key: "HIGHEST_PROTOCOL", Value: py.Int(HIGHEST_PROTOCOL)},
			py.DictEntry{Key: "DEFAULT_PROTOCOL", Value: py.Int(DEFAULT_PROTOCOL)},
		),
	})
}

const dumps_doc = `dumps(obj, protocol=None, *, fix_imports=True) -> bytes

Return the pickled representation of the object as a bytes object.

protocol is accepted for compatibility.  Protocol 0 is what is produced: it is
understood by every CPython.`

const loads_doc = `loads(data, /, *, fix_imports=True, encoding='ASCII', errors='strict') -> object

Read a pickled object representation from the given bytes object and return the
reconstituted object hierarchy specified therein.`

const dump_doc = `dump(obj, file, protocol=None, *, fix_imports=True) -> None

Write the pickled representation of obj to the open file object.`

const load_doc = `load(file, /, *, fix_imports=True, encoding='ASCII', errors='strict') -> object

Read a pickled object representation from the open file object and return the
reconstituted object hierarchy specified therein.`

func dumps(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var obj py.Object
	protocol := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:dumps", []string{"obj", "protocol"}, &obj, &protocol); err != nil {
		return nil, err
	}
	if err := checkProtocol(protocol); err != nil {
		return nil, err
	}
	e := newEncoder()
	if err := e.encode(obj, 0); err != nil {
		return nil, err
	}
	e.buf.WriteByte('.')
	return py.Bytes(e.buf.String()), nil
}

func loads(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var data py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:loads", []string{"data"}, &data); err != nil {
		return nil, err
	}
	raw, err := asBytes(data)
	if err != nil {
		return nil, err
	}
	d := &decoder{s: string(raw)}
	v, err := d.decode()
	if err != nil {
		return nil, err
	}
	return v, nil
}

func dump(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var obj, file py.Object
	protocol := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|O:dump", []string{"obj", "file", "protocol"}, &obj, &file, &protocol); err != nil {
		return nil, err
	}
	data, err := dumps(self, py.Tuple{obj, protocol}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	write, err := py.GetAttrString(file, "write")
	if err != nil {
		return nil, err
	}
	if _, err := py.Call(write, py.Tuple{data}, py.NewStringDict()); err != nil {
		return nil, err
	}
	return py.None, nil
}

func load(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var file py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:load", []string{"file"}, &file); err != nil {
		return nil, err
	}
	read, err := py.GetAttrString(file, "read")
	if err != nil {
		return nil, err
	}
	data, err := py.Call(read, py.Tuple{}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	return loads(self, py.Tuple{data}, py.NewStringDict())
}

// checkProtocol accepts CPython's protocol arguments for compatibility.  The
// value does not change the output: protocol 0 is what this module writes.
func checkProtocol(protocol py.Object) error {
	if protocol == py.None {
		return nil
	}
	n, err := py.MakeGoInt(protocol)
	if err != nil {
		return nil // a non-integer protocol is a TypeError in CPython; writing still succeeds
	}
	if n < 0 || n > HIGHEST_PROTOCOL {
		return py.ExceptionNewf(py.ValueError, "pickle protocol must be <= %d", HIGHEST_PROTOCOL)
	}
	return nil
}

// asBytes accepts the buffer types CPython accepts where a bytes object is
// expected.
func asBytes(o py.Object) ([]byte, error) {
	switch v := o.(type) {
	case py.Bytes:
		return []byte(v), nil
	case py.String:
		return []byte(v), nil
	case *py.ByteArray:
		return bytesOf(v)
	}
	return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not '%s'", o.Type().Name)
}
