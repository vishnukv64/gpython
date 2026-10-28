// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package binascii provides the implementation of the python's 'binascii' module.
package binascii

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"io"
	"mime/quotedprintable"

	"github.com/vishnukv64/gpython/py"
)

var (
	Incomplete = py.ExceptionType.NewType("binascii.Incomplete", "", nil, nil)
	Error      = py.ValueError.NewType("binascii.Error", "", nil, nil)
)

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "binascii",
			Doc:  "Conversion between binary data and ASCII",
		},
		Methods: []*py.Method{
			py.MustNewMethod("a2b_base64", a2b_base64, 0, "Decode a line of base64 data."),
			py.MustNewMethod("b2a_base64", b2a_base64, 0, "Base64-code line of data."),
			py.MustNewMethod("a2b_hex", a2b_hex, 0, a2b_hex_doc),
			py.MustNewMethod("b2a_hex", b2a_hex, 0, b2a_hex_doc),
			py.MustNewMethod("crc32", crc32_, 0, "Compute CRC-32 incrementally."),
			py.MustNewMethod("unhexlify", a2b_hex, 0, unhexlify_doc),
			py.MustNewMethod("hexlify", b2a_hex, 0, hexlify_doc),
			py.MustNewMethod("a2b_qp", a2b_qp, 0, a2b_qp_doc),
			py.MustNewMethod("b2a_qp", b2a_qp, 0, b2a_qp_doc),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "Incomplete", Value: Incomplete},
			py.DictEntry{Key: "Error", Value: Error},
		),
	})
}

func b2a_base64(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pydata py.Object
		pynewl py.Object = py.True
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "y*|p:binascii.b2a_base64", []string{"data", "newline"}, &pydata, &pynewl)
	if err != nil {
		return nil, err
	}

	var (
		buf     = []byte(pydata.(py.Bytes))
		newline = bool(pynewl.(py.Bool))
	)

	out := base64.StdEncoding.EncodeToString(buf)
	if newline {
		out += "\n"
	}
	return py.Bytes(out), nil
}

func a2b_base64(self py.Object, args py.Tuple) (py.Object, error) {
	var pydata py.Object
	err := py.ParseTuple(args, "s:binascii.a2b_base64", &pydata)
	if err != nil {
		return nil, err
	}

	out, err := base64.StdEncoding.DecodeString(string(pydata.(py.String)))
	if err != nil {
		return nil, py.ExceptionNewf(Error, "could not decode base64 data: %+v", err)
	}

	return py.Bytes(out), nil
}

func crc32_(self py.Object, args py.Tuple) (py.Object, error) {
	var (
		pydata py.Object
		pycrc  py.Object = py.Int(0)
	)

	err := py.ParseTuple(args, "y*|i:binascii.crc32", &pydata, &pycrc)
	if err != nil {
		return nil, err
	}

	crc := crc32.Update(uint32(pycrc.(py.Int)), crc32.IEEETable, []byte(pydata.(py.Bytes)))
	return py.Int(crc), nil

}

const a2b_hex_doc = `Binary data of hexadecimal representation.

hexstr must contain an even number of hex digits (upper or lower case).
This function is also available as "unhexlify()".`

func a2b_hex(self py.Object, args py.Tuple) (py.Object, error) {
	var (
		hexErr hex.InvalidByteError
		pydata py.Object
		src    string
	)
	err := py.ParseTuple(args, "s*:binascii.a2b_hex", &pydata)
	if err != nil {
		return nil, err
	}

	switch v := pydata.(type) {
	case py.String:
		src = string(v)
	case py.Bytes:
		src = string(v)
	}

	o, err := hex.DecodeString(src)
	if err != nil {
		switch {
		case errors.Is(err, hex.ErrLength):
			return nil, py.ExceptionNewf(Error, "Odd-length string")
		case errors.As(err, &hexErr):
			return nil, py.ExceptionNewf(Error, "Non-hexadecimal digit found")
		default:
			return nil, py.ExceptionNewf(Error, "could not decode hex data: %+v", err)
		}
	}

	return py.Bytes(o), nil
}

const b2a_hex_doc = `Hexadecimal representation of binary data.

The return value is a bytes object.  This function is also
available as "hexlify()".`

func b2a_hex(self py.Object, args py.Tuple) (py.Object, error) {
	var pydata py.Object
	err := py.ParseTuple(args, "y*:binascii.b2a_hex", &pydata)
	if err != nil {
		return nil, err
	}

	o := hex.EncodeToString([]byte(pydata.(py.Bytes)))
	return py.Bytes(o), nil
}

const unhexlify_doc = `Binary data of hexadecimal representation.

hexstr must contain an even number of hex digits (upper or lower case).`

const hexlify_doc = `Hexadecimal representation of binary data.

The return value is a bytes object.  This function is also
available as "b2a_hex()".`

const a2b_qp_doc = `Decode a string of qp-encoded data.`

func a2b_qp(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pydata py.Object
		pyhdr  py.Object = py.Bool(false)
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "y*|p:binascii.a2b_qp", []string{"data", "header"}, &pydata, &pyhdr)
	if err != nil {
		return nil, err
	}

	// TODO(sbinet)
	if pyhdr.(py.Bool) {
		return nil, py.NotImplementedError
	}

	o := new(bytes.Buffer)
	r := quotedprintable.NewReader(bytes.NewReader([]byte(pydata.(py.Bytes))))
	_, err = io.Copy(o, r)
	if err != nil {
		// FIXME(sbinet): decorate the error somehow?
		return nil, err
	}
	return py.Bytes(o.Bytes()), nil
}

const b2a_qp_doc = `Encode a string using quoted-printable encoding.

On encoding, when istext is set, newlines are not encoded, and white
space at end of lines is.  When istext is not set, \r and \n (CR/LF)
are both encoded.  When quotetabs is set, space and tabs are encoded.`

func b2a_qp(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pydata  py.Object
		pyqtabs py.Object = py.Bool(false)
		pyistxt py.Object = py.Bool(true)
		pyhdr   py.Object = py.Bool(false)
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "y*|ppp:binascii.b2a_qp", []string{"data", "quotetabs", "istext", "header"}, &pydata, &pyqtabs, &pyistxt, &pyhdr)
	if err != nil {
		return nil, err
	}
	return py.Bytes(encodeQP([]byte(pydata.(py.Bytes)), bool(pyqtabs.(py.Bool)), bool(pyistxt.(py.Bool)), bool(pyhdr.(py.Bool)))), nil
}

// encodeQP is a port of CPython's binascii.b2a_qp: lines break at 76 columns
// and use CRLF only if the first line of the input does.
func encodeQP(data []byte, quotetabs, istext, header bool) []byte {
	const maxLine = 76
	const hex = "0123456789ABCDEF"
	crlf := false
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		crlf = true
	}
	var out []byte
	softBreak := func() {
		out = append(out, '=')
		if crlf {
			out = append(out, '\r')
		}
		out = append(out, '\n')
	}
	n := len(data)
	linelen := 0
	for in := 0; in < n; {
		c := data[in]
		quote := c > 126 || c == '=' || (header && c == '_') ||
			(c == '.' && linelen == 0 && (in+1 == n || data[in+1] == '\n' || data[in+1] == '\r' || data[in+1] == 0)) ||
			(!istext && (c == '\r' || c == '\n')) ||
			((c == '\t' || c == ' ') && in+1 == n) ||
			(c <= 32 && c != '\r' && c != '\n' && (quotetabs || (c != '\t' && c != ' ')))
		switch {
		case quote:
			if linelen+3 >= maxLine {
				softBreak()
				linelen = 0
			}
			out = append(out, '=', hex[c>>4], hex[c&15])
			in++
			linelen += 3
		case istext && (c == '\n' || (in+1 < n && c == '\r' && data[in+1] == '\n')):
			linelen = 0
			// Protect whitespace at the end of a line.
			if l := len(out); l > 0 && (out[l-1] == ' ' || out[l-1] == '\t') {
				ch := out[l-1]
				out[l-1] = '='
				out = append(out, hex[ch>>4], hex[ch&15])
			}
			if crlf {
				out = append(out, '\r')
			}
			out = append(out, '\n')
			if c == '\r' {
				in += 2
			} else {
				in++
			}
		default:
			if in+1 != n && data[in+1] != '\n' && linelen+1 >= maxLine {
				softBreak()
				linelen = 0
			}
			linelen++
			if header && c == ' ' {
				out = append(out, '_')
			} else {
				out = append(out, c)
			}
			in++
		}
	}
	return out
}
