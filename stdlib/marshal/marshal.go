// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Implement unmarshal and marshal
package marshal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"

	"github.com/vishnukv64/gpython/py"
)

const (
	MARSHAL_VERSION     = 3
	TYPE_NULL           = '0'
	TYPE_NONE           = 'N'
	TYPE_FALSE          = 'F'
	TYPE_TRUE           = 'T'
	TYPE_STOPITER       = 'S'
	TYPE_ELLIPSIS       = '.'
	TYPE_INT            = 'i'
	TYPE_FLOAT          = 'f'
	TYPE_BINARY_FLOAT   = 'g'
	TYPE_COMPLEX        = 'x'
	TYPE_BINARY_COMPLEX = 'y'
	TYPE_LONG           = 'l'
	TYPE_STRING         = 's'
	TYPE_INTERNED       = 't'
	TYPE_REF            = 'r'
	TYPE_TUPLE          = '('
	TYPE_LIST           = '['
	TYPE_DICT           = '{'
	TYPE_CODE           = 'c'
	TYPE_UNICODE        = 'u'
	TYPE_UNKNOWN        = '?'
	TYPE_SET            = '<'
	TYPE_FROZENSET      = '>'
	FLAG_REF            = 0x80 // with a type, add obj to index
	SIZE32_MAX          = 0x7FFFFFFF

	TYPE_ASCII                = 'a'
	TYPE_ASCII_INTERNED       = 'A'
	TYPE_SMALL_TUPLE          = ')'
	TYPE_SHORT_ASCII          = 'z'
	TYPE_SHORT_ASCII_INTERNED = 'Z'

	// We assume that Python ints are stored internally in base some power of
	// 2**15; for the sake of portability we'll always read and write them in base
	// exactly 2**15.

	PyLong_MARSHAL_SHIFT = 15
	PyLong_MARSHAL_BASE  = (1 << PyLong_MARSHAL_SHIFT)
	PyLong_MARSHAL_MASK  = (PyLong_MARSHAL_BASE - 1)
)

// Represents currently being unmarshalled file
type rFile struct {
	r    io.Reader
	refs []py.Object
}

// Reads an object from the input
func (rfile *rFile) ReadObject() (obj py.Object, err error) {
	var code byte
	// defer func() { fmt.Printf("ReadObject(%c) returning %#v with error %v\n", code, obj, err) }()
	err = binary.Read(rfile.r, binary.LittleEndian, &code)
	if err != nil {
		return
	}

	AddRef := (code & FLAG_REF) != 0
	Type := code &^ FLAG_REF

	// Add a reference if required
	addRef := func(obj py.Object) py.Object {
		if AddRef {
			rfile.refs = append(rfile.refs, obj)
		}
		return obj
	}

	// Reserve a reference if required
	reserveRef := func() int {
		if !AddRef {
			return -1
		}
		rfile.refs = append(rfile.refs, nil)
		return len(rfile.refs) - 1
	}

	// Update a ref if required
	updateRef := func(i int, obj py.Object) py.Object {
		if i >= 0 {
			rfile.refs[i] = obj
		}
		return obj
	}

	switch Type {
	case TYPE_NULL:
		// A null object
		AddRef = false
		return nil, nil
	case TYPE_NONE:
		// The Python None object
		AddRef = false
		return py.None, nil
	case TYPE_FALSE:
		// The python False object
		AddRef = false
		return py.False, nil
	case TYPE_TRUE:
		// The python True object
		AddRef = false
		return py.True, nil
	case TYPE_STOPITER:
		// The python StopIteration Exception
		AddRef = false
		return py.StopIteration, nil
	case TYPE_ELLIPSIS:
		// The python elipsis object
		AddRef = false
		return py.Ellipsis, nil
	case TYPE_INT:
		// 4 bytes of signed integer
		var n int32
		err = binary.Read(rfile.r, binary.LittleEndian, &n)
		if err != nil {
			return
		}
		return addRef(py.Int(n)), nil
	case TYPE_FLOAT:
		// Floating point number as a string
		var length uint8
		err = binary.Read(rfile.r, binary.LittleEndian, &length)
		if err != nil {
			return
		}
		buf := make([]byte, int(length))
		_, err = io.ReadFull(rfile.r, buf)
		if err != nil {
			return
		}
		var f float64
		f, err = strconv.ParseFloat(string(buf), 64)
		if err != nil {
			return
		}
		return addRef(py.Float(f)), nil
	case TYPE_BINARY_FLOAT:
		var f float64
		err = binary.Read(rfile.r, binary.LittleEndian, &f)
		if err != nil {
			return
		}
		return addRef(py.Float(f)), nil
	case TYPE_COMPLEX:
		// Complex number as a string
		// FIXME this is using Go conversion not Python conversion which may differ
		var length uint8
		err = binary.Read(rfile.r, binary.LittleEndian, &length)
		if err != nil {
			return
		}
		buf := make([]byte, int(length))
		_, err = io.ReadFull(rfile.r, buf)
		if err != nil {
			return
		}
		var c complex128
		// FIXME c, err = strconv.ParseComplex(string(buf), 64)
		if err != nil {
			return
		}
		return addRef(py.Complex(c)), nil
	case TYPE_BINARY_COMPLEX:
		var c complex128
		err = binary.Read(rfile.r, binary.LittleEndian, &c)
		if err != nil {
			return
		}
		return addRef(py.Complex(c)), nil
	case TYPE_LONG:
		var size int32
		err = binary.Read(rfile.r, binary.LittleEndian, &size)
		if err != nil {
			return
		}
		// A NEGATIVE count means a negative number.  The sign was read and
		// then thrown away, so every large negative integer came back
		// positive.
		negative := size < 0
		if negative {
			size = -size
		}
		if size < 0 || size > SIZE32_MAX {
			return nil, errors.New("bad marshal data (long size out of range)")
		}
		// The digits are base 2**15, least significant first, 16 bits each.
		digits := make([]int16, size)
		err = binary.Read(rfile.r, binary.LittleEndian, &digits)
		if err != nil {
			return
		}
		// The digit that must be non-zero is the MOST significant one, which
		// is the LAST in this little-endian array.  Checking digits[0]
		// rejected perfectly good data: CPython encodes 2**40 as
		// 03000000 0000 0000 0400, whose first digit is legitimately 0.
		if len(digits) == 0 || digits[len(digits)-1] == 0 {
			return nil, py.ExceptionNewf(py.ValueError, "bad marshal data (digit out of range in long)")
		}
		// Convert into a big.Int.  Horner's rule needs the MOST significant
		// digit first, and the array is least significant first, so it is
		// walked backwards.  Going forwards made 2**40 come back as 1024.
		r := new(big.Int)
		t := new(big.Int)
		for i := len(digits) - 1; i >= 0; i-- {
			r.Lsh(r, 15)
			t.SetInt64(int64(digits[i]))
			r.Add(r, t)
		}
		if negative {
			r.Neg(r)
		}
		// FIXME try to fit into int64 if possible
		return addRef((*py.BigInt)(r)), nil
	case TYPE_STRING, TYPE_INTERNED, TYPE_UNICODE, TYPE_ASCII, TYPE_ASCII_INTERNED:
		var size int32
		err = binary.Read(rfile.r, binary.LittleEndian, &size)
		if err != nil {
			return
		}
		if size < 0 || size > SIZE32_MAX {
			return nil, errors.New("bad marshal data (string size out of range)")
		}
		buf := make([]byte, int(size))
		_, err = io.ReadFull(rfile.r, buf)
		if err != nil {
			return
		}
		// TYPE_STRING is BYTES and the rest are text; returning py.String for
		// every one of them made "marshal.loads(marshal.dumps(b'xy'))" a str.
		if Type == TYPE_STRING {
			return addRef(py.Bytes(buf)), nil
		}
		return addRef(py.String(buf)), nil
	case TYPE_SHORT_ASCII, TYPE_SHORT_ASCII_INTERNED:
		var size uint8
		err = binary.Read(rfile.r, binary.LittleEndian, &size)
		if err != nil {
			return
		}
		buf := make([]byte, int(size))
		_, err = io.ReadFull(rfile.r, buf)
		if err != nil {
			return
		}
		// FIXME do something different for interned?
		return addRef(py.String(buf)), nil
	case TYPE_TUPLE, TYPE_LIST, TYPE_SET, TYPE_FROZENSET:
		var size int32
		err = binary.Read(rfile.r, binary.LittleEndian, &size)
		if err != nil {
			return
		}
		if size < 0 || size > SIZE32_MAX {
			return nil, errors.New("bad marshal data (tuple size out of range)")
		}
		tuple := make([]py.Object, int(size))
		iref := reserveRef()
		for i := range tuple {
			tuple[i], err = rfile.ReadObject()
			if err != nil {
				return
			}
		}
		switch Type {
		case TYPE_TUPLE:
			return updateRef(iref, py.Tuple(tuple)), nil
		case TYPE_LIST:
			return updateRef(iref, py.NewListFromItems(tuple)), nil
		case TYPE_SET:
			return updateRef(iref, py.NewSetFromItems(tuple)), nil
		case TYPE_FROZENSET:
			return updateRef(iref, py.NewFrozenSetFromItems(tuple)), nil
		}
	case TYPE_SMALL_TUPLE:
		var size uint8
		err = binary.Read(rfile.r, binary.LittleEndian, &size)
		if err != nil {
			return
		}
		tuple := make([]py.Object, int(size))
		iref := reserveRef()
		for i := range tuple {
			tuple[i], err = rfile.ReadObject()
			if err != nil {
				return
			}
		}
		return updateRef(iref, py.Tuple(tuple)), nil
	case TYPE_DICT:
		// FIXME should be py.Dict
		dict := py.NewStringDict()
		iref := reserveRef()
		var key, value py.Object
		for {
			key, err = rfile.ReadObject()
			if err != nil {
				return
			}
			if key == nil {
				break
			}
			value, err = rfile.ReadObject()
			if err != nil {
				return
			}
			if value != nil {
				// FIXME should be objects as key
				dict[string(key.(py.String))] = value
			}
		}
		return updateRef(iref, dict), nil
	case TYPE_REF:
		// Reference to a previous read
		var n int32
		err = binary.Read(rfile.r, binary.LittleEndian, &n)
		if err != nil {
			return
		}
		if n < 0 || int(n) >= len(rfile.refs) {
			AddRef = false
			fmt.Printf("Returning None as %d/%d out of range\n", n, len(rfile.refs))
			return py.None, nil

			// return nil, fmt.Errorf("TYPE_REF (out of range) - %d vs %d: %#v", n, len(rfile.refs), rfile.refs)
		}
		AddRef = false
		return rfile.refs[n], nil
	case TYPE_CODE:
		var argcount int32
		var kwonlyargcount int32
		var nlocals int32
		var stacksize int32
		var flags int32
		var code py.Object
		var consts py.Object
		var names py.Object
		var varnames py.Object
		var freevars py.Object
		var cellvars py.Object
		var filename py.Object
		var name py.Object
		var firstlineno int32
		var lnotab py.Object
		iref := reserveRef()

		if err = binary.Read(rfile.r, binary.LittleEndian, &argcount); err != nil {
			return
		}
		if err = binary.Read(rfile.r, binary.LittleEndian, &kwonlyargcount); err != nil {
			return
		}
		if err = binary.Read(rfile.r, binary.LittleEndian, &nlocals); err != nil {
			return
		}
		if err = binary.Read(rfile.r, binary.LittleEndian, &stacksize); err != nil {
			return
		}
		if err = binary.Read(rfile.r, binary.LittleEndian, &flags); err != nil {
			return
		}
		if code, err = rfile.ReadObject(); err != nil {
			return
		}
		if consts, err = rfile.ReadObject(); err != nil {
			return
		}
		if names, err = rfile.ReadObject(); err != nil {
			return
		}
		if varnames, err = rfile.ReadObject(); err != nil {
			return
		}
		if freevars, err = rfile.ReadObject(); err != nil {
			return
		}
		if cellvars, err = rfile.ReadObject(); err != nil {
			return
		}
		if filename, err = rfile.ReadObject(); err != nil {
			return
		}
		if name, err = rfile.ReadObject(); err != nil {
			return
		}
		if err = binary.Read(rfile.r, binary.LittleEndian, &firstlineno); err != nil {
			return
		}
		if lnotab, err = rfile.ReadObject(); err != nil {
			return
		}

		// fmt.Printf("argcount = %v\n", argcount)
		// fmt.Printf("kwonlyargcount = %v\n", kwonlyargcount)
		// fmt.Printf("nlocals = %v\n", nlocals)
		// fmt.Printf("stacksize = %v\n", stacksize)
		// fmt.Printf("flags = %v\n", flags)
		// fmt.Printf("code = %x\n", code)
		// fmt.Printf("consts = %v\n", consts)
		// fmt.Printf("names = %v\n", names)
		// fmt.Printf("varnames = %v\n", varnames)
		// fmt.Printf("freevars = %v\n", freevars)
		// fmt.Printf("cellvars = %v\n", cellvars)
		// fmt.Printf("filename = %v\n", filename)
		// fmt.Printf("name = %v\n", name)
		// fmt.Printf("firstlineno = %v\n", firstlineno)
		// fmt.Printf("lnotab = %x\n", lnotab)

		v := py.NewCode(
			argcount, kwonlyargcount,
			nlocals, stacksize, flags,
			code, consts, names, varnames,
			freevars, cellvars, filename, name,
			firstlineno, lnotab)
		return updateRef(iref, v), nil
	default:
		return nil, fmt.Errorf("bad marshal data (unknown type code) 0x%02X '%c'", Type, Type)
	}

	return
}

// Reads an object from the input
func ReadObject(r io.Reader) (obj py.Object, err error) {
	rfile := &rFile{r: r}
	return rfile.ReadObject()
}

// The header on a .pyc file
type PycHeader struct {
	Magic     uint32
	Timestamp int32
	Length    int32
}

// Reads a pyc file
func ReadPyc(r io.Reader) (obj py.Object, err error) {
	var header PycHeader
	if err = binary.Read(r, binary.LittleEndian, &header); err != nil {
		return
	}
	// FIXME do something with timestamp & length?
	if header.Magic>>16 != 0x0a0d {
		return nil, errors.New("bad magic in .pyc file")
	}
	// fmt.Printf("header = %v\n", header)
	return ReadObject(r)
}

const dump_doc = `dump(value, file[, version])

Write the value on the open file. The value must be a supported type.
The file must be an open file object such as sys.stdout or returned by
open() or os.popen(). It must be opened in binary mode ('wb' or 'w+b').

If the value has (or contains an object that has) an unsupported type, a
ValueError exception is raised — but garbage data will also be written
to the file. The object will not be properly read back by load()

The version argument indicates the data format that dump should use.`

func marshal_dump(self py.Object, args py.Tuple) (py.Object, error) {
	/*
	   // XXX Quick hack -- need to do this differently
	   PyObject *x;
	   PyObject *f;
	   int version = Py_MARSHAL_VERSION;
	   PyObject *s;
	   PyObject *res;
	   _Py_IDENTIFIER(write);

	   if (!PyArg_ParseTuple(args, "OO|i:dump", &x, &f, &version))
	       return NULL;
	   s = PyMarshal_WriteObjectToString(x, version);
	   if (s == NULL)
	       return NULL;
	   res = _PyObject_CallMethodId(f, &PyId_write, "O", s);
	   Py_DECREF(s);
	   return res;
	*/
	return nil, py.ExceptionNewf(py.SystemError, "dump not implemented")
}

const load_doc = `load(file)

Read one value from the open file and return it. If no valid value is
read (e.g. because the data has a different Python version’s
incompatible marshal format), raise EOFError, ValueError or TypeError.
The file must be an open file object opened in binary mode ('rb' or
'r+b').

Note: If an object containing an unsupported type was marshalled with
dump(), load() will substitute None for the unmarshallable type.`

func marshal_load(self, f py.Object) (py.Object, error) {
	/*
	   PyObject *data, *result;
	   _Py_IDENTIFIER(read);
	   RFILE rf;

	    // Make a call to the read method, but read zero bytes.
	    // This is to ensure that the object passed in at least
	    // has a read method which returns bytes.
	   data = _PyObject_CallMethodId(f, &PyId_read, "i", 0);
	   if (data == NULL)
	       return NULL;
	   if (!PyBytes_Check(data)) {
	       PyErr_Format(PyExc_TypeError,
	                    "f.read() returned not bytes but %.100s",
	                    data->ob_type->tp_name);
	       result = NULL;
	   }
	   else {
	       rf.depth = 0;
	       rf.fp = NULL;
	       rf.readable = f;
	       rf.current_filename = NULL;
	       result = read_object(&rf);
	   }
	   Py_DECREF(data);
	   return result;
	*/
	return nil, py.ExceptionNewf(py.SystemError, "load not implemented")
}

const dumps_doc = `dumps(value[, version])

Return the string that would be written to a file by dump(value, file).
The value must be a supported type. Raise a ValueError exception if
value has (or contains an object that has) an unsupported type.

The version argument indicates the data format that dumps should use.`

func marshal_dumps(self py.Object, args py.Tuple) (py.Object, error) {
	// dumps(value[, version]) -> the value as marshal bytes.
	//
	// This is a real encoder for the format the reader above accepts.  It
	// writes the subset that covers the values marshal is actually used for -
	// None, bool, the numbers, strings, bytes, and the containers - and
	// refuses anything else with the message CPython uses rather than writing
	// a stream that this module's own loads() could not read back.
	var x py.Object
	var versionObj py.Object
	if err := py.UnpackTuple(args, nil, "dumps", 1, 2, &x, &versionObj); err != nil {
		return nil, err
	}
	_ = versionObj // Only version 3 is written; the argument is accepted.
	e := &marshalWriter{}
	if err := e.write(x); err != nil {
		return nil, err
	}
	return py.Bytes(e.buf), nil
}

// marshalWriter encodes values in the format the reader accepts.
//
// The format supports shared references - a container written twice is
// written once and referred to after that - and CPython uses them.  This
// writer does NOT: identifying an object would need a map keyed by identity,
// and the container types here are a Go slice and a Go map, neither of which
// can be a map key.  Every value is therefore written out in full.
//
// The consequence is that a SELF-REFERENTIAL structure cannot be written and
// would recurse until the depth guard stops it; CPython raises ValueError for
// one at depth 1, this at maxDepth.  For the flat data marshal is used for,
// the output is identical.
type marshalWriter struct {
	buf   []byte
	depth int
}

// maxDepth bounds the recursion.  CPython caps marshal nesting at 2000; the
// limit is what turns a cyclic structure into an error instead of a stack
// overflow.
const maxDepth = 2000

func (e *marshalWriter) writeInt32(n int32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(n))
	e.buf = append(e.buf, b[:]...)
}

// writeBytes writes a length-prefixed byte string, which is how the format
// stores strings, bytes and the text of a float.
func (e *marshalWriter) writeBytes(data []byte) {
	e.writeInt32(int32(len(data)))
	e.buf = append(e.buf, data...)
}

func (e *marshalWriter) write(o py.Object) error {
	e.depth++
	defer func() { e.depth-- }()
	if e.depth > maxDepth {
		return py.ExceptionNewf(py.ValueError, "exceeded maximum marshal recursion depth")
	}

	switch v := o.(type) {
	case nil:
		e.buf = append(e.buf, TYPE_NULL)
		return nil
	case py.NoneType:
		e.buf = append(e.buf, TYPE_NONE)
		return nil
	case py.Bool:
		if v {
			e.buf = append(e.buf, TYPE_TRUE)
		} else {
			e.buf = append(e.buf, TYPE_FALSE)
		}
		return nil
	case py.EllipsisType:
		e.buf = append(e.buf, TYPE_ELLIPSIS)
		return nil
	case py.Int:
		n := int64(v)
		if n >= math.MinInt32 && n <= math.MaxInt32 {
			e.buf = append(e.buf, TYPE_INT)
			e.writeInt32(int32(n))
			return nil
		}
		// Too large for the 4-byte form.  The long form is a count of
		// base-2**15 digits followed by that many 16-bit digits, least
		// significant first, and the count is NEGATED for a negative value.
		// Writing raw bytes here produced a stream the reader rejected with
		// "EOF read where object expected".
		e.buf = append(e.buf, TYPE_LONG)
		neg := n < 0
		m := uint64(n)
		if neg {
			m = uint64(-n)
		}
		var digits []int16
		for m > 0 {
			digits = append(digits, int16(m&PyLong_MARSHAL_MASK))
			m >>= PyLong_MARSHAL_SHIFT
		}
		if len(digits) == 0 {
			digits = append(digits, 0)
		}
		size := int32(len(digits))
		if neg {
			size = -size
		}
		e.writeInt32(size)
		for _, d := range digits {
			var b [2]byte
			binary.LittleEndian.PutUint16(b[:], uint16(d))
			e.buf = append(e.buf, b[:]...)
		}
		return nil
	case py.Float:
		// The binary float form: an 8-byte float64.  'g' is the binary
		// form, which is what version 3 writes.
		e.buf = append(e.buf, TYPE_BINARY_FLOAT)
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(float64(v)))
		e.buf = append(e.buf, b[:]...)
		return nil
	case py.String:
		e.buf = append(e.buf, TYPE_UNICODE)
		e.writeBytes([]byte(v))
		return nil
	case py.Bytes:
		e.buf = append(e.buf, TYPE_STRING)
		e.writeBytes([]byte(v))
		return nil
	case py.Tuple:
		e.buf = append(e.buf, TYPE_TUPLE)
		e.writeInt32(int32(len(v)))
		for _, item := range v {
			if err := e.write(item); err != nil {
				return err
			}
		}
		return nil
	case *py.List:
		e.buf = append(e.buf, TYPE_LIST)
		e.writeInt32(int32(len(v.Items)))
		for _, item := range v.Items {
			if err := e.write(item); err != nil {
				return err
			}
		}
		return nil
	case py.StringDict:
		e.buf = append(e.buf, TYPE_DICT)
		// A dict is NOT length-prefixed: it is a run of key/value pairs
		// terminated by a NULL.  Writing a count instead left the reader
		// reading pairs past the end, which reported "unknown type code 0x02".
		keys, err := v.M__iter__()
		if err != nil {
			return err
		}
		it, ok := keys.(py.I__next__)
		if !ok {
			return py.ExceptionNewf(py.ValueError, "unmarshallable object")
		}
		for {
			k, err := it.M__next__()
			if err != nil {
				if py.IsException(py.StopIteration, err) {
					break
				}
				return err
			}
			if err := e.write(k); err != nil {
				return err
			}
			val, err := v.M__getitem__(k)
			if err != nil {
				return err
			}
			if err := e.write(val); err != nil {
				return err
			}
		}
		e.buf = append(e.buf, TYPE_NULL)
		return nil
	}
	return py.ExceptionNewf(py.ValueError, "unmarshallable object")
}

const loads_doc = `loads(bytes)

Convert the bytes object to a value. If no valid value is found, raise
EOFError, ValueError or TypeError. Extra characters in the input are
ignored.`

func marshal_loads(self py.Object, args py.Tuple) (py.Object, error) {
	// loads(bytes) -> the value those bytes encode.
	//
	// The reader above implements the whole format; this only had to be wired
	// to it.  Trailing bytes are ignored, as CPython's docs say, and a
	// truncated stream is an EOFError.
	var src py.Object
	if err := py.UnpackTuple(args, nil, "loads", 1, 1, &src); err != nil {
		return nil, err
	}
	var data []byte
	switch v := src.(type) {
	case py.Bytes:
		data = []byte(v)
	case py.String:
		return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not 'str'")
	case *py.File:
		return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not 'file'")
	default:
		if b, ok := src.(interface{ Bytes() []byte }); ok {
			data = b.Bytes()
			break
		}
		return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not '%s'", src.Type().Name)
	}
	rf := &rFile{r: bytes.NewReader(data)}
	value, err := rf.ReadObject()
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, py.ExceptionNewf(py.EOFError, "EOF read where object expected")
		}
		return nil, err
	}
	return value, nil
}

const module_doc = `This module contains functions that can read and write Python values in
a binary format. The format is specific to Python, but independent of
machine architecture issues.

Not all Python object types are supported; in general, only objects
whose value is independent from a particular invocation of Python can be
written and read by this module. The following types are supported:
None, integers, floating point numbers, strings, bytes, bytearrays,
tuples, lists, sets, dictionaries, and code objects, where it
should be understood that tuples, lists and dictionaries are only
supported as long as the values contained therein are themselves
supported; and recursive lists and dictionaries should not be written
(they will cause infinite loops).

Variables:

version -- indicates the format that the module uses. Version 0 is the
    historical format, version 1 shares interned strings and version 2
    uses a binary format for floating point numbers.

Functions:

dump() -- write value to a file
load() -- read value from a file
dumps() -- write value to a string
loads() -- read value from a string`

// Initialise the module
func init() {
	methods := []*py.Method{
		py.MustNewMethod("dump", marshal_dump, 0, dump_doc),
		py.MustNewMethod("load", marshal_load, 0, load_doc),
		py.MustNewMethod("dumps", marshal_dumps, 0, dumps_doc),
		py.MustNewMethod("loads", marshal_loads, 0, loads_doc),
	}
	globals := py.StringDict{
		"version": py.Int(MARSHAL_VERSION),
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:  "marshal",
			Doc:   module_doc,
			Flags: py.ShareModule,
		},
		Globals: globals,
		Methods: methods,
	})
}
