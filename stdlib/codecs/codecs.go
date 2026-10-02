// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package codecs provides the implementation of python's 'codecs' module:
// the encoder/decoder registry and the stream wrappers.
//
// The registry answers for the encodings the interpreter itself uses -
// utf-8, ascii and latin-1 - and lookup() returns the same four-element
// tuple CPython does, so code that inspects the result keeps working.  The
// stream classes (StreamReader/StreamWriter and the incremental codecs) are
// not implemented: nothing here supplies a custom codec, and returning an
// object that could not actually encode would be worse than saying so.
package codecs

import (
	"strings"
	"unicode/utf8"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `codecs -- Python Codec Registry, API and helpers.`

// CodecInfo is what lookup() returns.
type CodecInfo struct {
	name               string
	encoder            py.Object
	decoder            py.Object
	incrementalEncoder py.Object
	incrementalDecoder py.Object
}

var CodecInfoType = py.NewTypeX("codecs.CodecInfo", "Codec details: the name and its four codec functions.", nil, nil)

func (c *CodecInfo) Type() *py.Type { return CodecInfoType }

var LookupErrorType = py.ExceptionType.NewType("codecs.LookupError", "Raised when a codec cannot be found.", nil, nil)

// normalise maps the aliases a caller may use onto a canonical name.
func normalise(name string) string {
	n := strings.ToLower(strings.ReplaceAll(name, " ", ""))
	n = strings.ReplaceAll(n, "_", "-")
	switch n {
	case "utf-8", "utf8", "u8", "utf", "cp65001":
		return "utf-8"
	case "ascii", "us-ascii", "646", "ansi-x3.4-1968":
		return "ascii"
	case "latin-1", "latin1", "iso-8859-1", "8859", "cp819", "l1":
		return "latin-1"
	case "utf-16", "utf16":
		return "utf-16"
	case "utf-16-le":
		return "utf-16-le"
	case "utf-16-be":
		return "utf-16-be"
	case "utf-32", "utf32":
		return "utf-32"
	case "rot13", "rot-13", "rot_13":
		// CPython reports the canonical name as "rot-13"; the codec is the same
		// either way.
		return "rot13"
	}
	return n
}

// encoderFor and decoderFor are the codec functions for a name.
func encoderFor(name string) py.Object {
	return py.MustNewMethod("encode", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return encodeWith(name, args)
	}, 0, "Encode a string with this codec.")
}

func decoderFor(name string) py.Object {
	return py.MustNewMethod("decode", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return decodeWith(name, args)
	}, 0, "Decode bytes with this codec.")
}

func init() {
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "LookupError", Value: LookupErrorType},
		py.DictEntry{Key: "CodecInfo", Value: CodecInfoType},
		py.DictEntry{Key: "lookup", Value: py.MustNewMethod("lookup", lookup, 0, "Look up a codec by name.")},
		py.DictEntry{Key: "encode", Value: py.MustNewMethod("encode", moduleEncode, 0, "Encode an object with the given codec.")},
		py.DictEntry{Key: "decode", Value: py.MustNewMethod("decode", moduleDecode, 0, "Decode an object with the given codec.")},
		py.DictEntry{Key: "register", Value: py.MustNewMethod("register", registerNoop, 0, "Register a codec search function (not supported).")},
		// The incremental API.  charset_normalizer -- a dependency of requests
		// -- opens with "from codecs import IncrementalDecoder", so these have
		// to be real, subclassable classes.
		py.DictEntry{Key: "IncrementalDecoder", Value: incrementalDecoderType},
		py.DictEntry{Key: "IncrementalEncoder", Value: incrementalEncoderType},
		py.DictEntry{Key: "BufferedIncrementalDecoder", Value: incrementalDecoderType},
		py.DictEntry{Key: "BufferedIncrementalEncoder", Value: incrementalEncoderType},
		py.DictEntry{Key: "getencoder", Value: py.MustNewMethod("getencoder", getEncoder, 0, "Look up the encoder for an encoding.")},
		py.DictEntry{Key: "getdecoder", Value: py.MustNewMethod("getdecoder", getDecoder, 0, "Look up the decoder for an encoding.")},
		py.DictEntry{Key: "getincrementalencoder", Value: py.MustNewMethod("getincrementalencoder", getEncoder, 0, "Look up the incremental encoder.")},
		py.DictEntry{Key: "getincrementaldecoder", Value: py.MustNewMethod("getincrementaldecoder", getDecoder, 0, "Look up the incremental decoder.")},
		py.DictEntry{Key: "BOM", Value: py.Bytes{0xef, 0xbb, 0xbf}},
		py.DictEntry{Key: "BOM_UTF8", Value: py.Bytes{0xef, 0xbb, 0xbf}},
		py.DictEntry{Key: "BOM_UTF16", Value: py.Bytes{0xff, 0xfe}},
		py.DictEntry{Key: "BOM_UTF16_LE", Value: py.Bytes{0xff, 0xfe}},
		py.DictEntry{Key: "BOM_UTF16_BE", Value: py.Bytes{0xfe, 0xff}},
		py.DictEntry{Key: "BOM_UTF32", Value: py.Bytes{0xff, 0xfe, 0x00, 0x00}},
		py.DictEntry{Key: "BOM_UTF32_LE", Value: py.Bytes{0xff, 0xfe, 0x00, 0x00}},
		py.DictEntry{Key: "BOM_UTF32_BE", Value: py.Bytes{0x00, 0x00, 0xfe, 0xff}},
		py.DictEntry{Key: "BOM_LE", Value: py.Bytes{0xff, 0xfe}},
		py.DictEntry{Key: "BOM_BE", Value: py.Bytes{0xfe, 0xff}},
	)
	globals.Set("__doc__", py.String(module_doc))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "codecs",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// lookup returns the CodecInfo for a name, as CPython's four-element tuple.
func lookup(self py.Object, args py.Tuple) (py.Object, error) {
	var name py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "lookup", 1, 1, &name); err != nil {
		return nil, err
	}
	text, err := py.StrAsString(name)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "lookup() argument must be a string")
	}
	canonical := normalise(text)
	if !known(canonical) {
		return nil, py.ExceptionNewf(LookupErrorType, "unknown encoding: %s", text)
	}
	return &CodecInfo{
		name:               canonicalName(canonical),
		encoder:            encoderFor(canonical),
		decoder:            decoderFor(canonical),
		incrementalEncoder: encoderFor(canonical),
		incrementalDecoder: decoderFor(canonical),
	}, nil
}

// canonicalName is the spelling codecs.lookup().name reports, which differs
// from the internal key for rot13.
func canonicalName(n string) string {
	if n == "rot13" {
		return "rot-13"
	}
	return n
}

// known reports whether the interpreter can encode and decode the name.
func known(name string) bool {
	switch name {
	case "utf-8", "ascii", "latin-1", "utf-16", "utf-16-le", "utf-16-be", "utf-32", "rot13":
		return true
	}
	return false
}

func getEncoder(self py.Object, args py.Tuple) (py.Object, error) {
	info, err := lookup(self, args)
	if err != nil {
		return nil, err
	}
	return info.(*CodecInfo).encoder, nil
}

func getDecoder(self py.Object, args py.Tuple) (py.Object, error) {
	info, err := lookup(self, args)
	if err != nil {
		return nil, err
	}
	return info.(*CodecInfo).decoder, nil
}

func moduleEncode(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "encode() needs an object and an encoding")
	}
	enc, err := py.StrAsString(args[1])
	if err != nil {
		return nil, err
	}
	res, err := encodeWith(normalise(enc), py.Tuple{args[0]})
	if err != nil {
		return nil, err
	}
	// Unwrapped for the same reason moduleDecode is - see above.
	return unwrapCodecResult(res), nil
}

// moduleDecode is the module-level codecs.decode.
//
// It UNWRAPS the (value, length) pair its codec function returns.  A codec's
// own function keeps the tuple - codecs.lookup('utf-8').decode(b'abc') is
// ('abc', 3) - but the module-level helper is the convenient one and CPython
// has it return the value alone, so codecs.decode(b'abc', 'utf-8') is 'abc'.
// Returning the tuple here was simply the wrong shape, and it is the shape
// ordinary code uses.
func moduleDecode(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "decode() needs an object and an encoding")
	}
	enc, err := py.StrAsString(args[1])
	if err != nil {
		return nil, err
	}
	res, err := decodeWith(normalise(enc), py.Tuple{args[0]})
	if err != nil {
		return nil, err
	}
	return unwrapCodecResult(res), nil
}

// unwrapCodecResult drops the length from a (value, length) pair.
func unwrapCodecResult(res py.Object) py.Object {
	if t, ok := res.(py.Tuple); ok && len(t) == 2 {
		return t[0]
	}
	return res
}

// rot13 maps the ASCII letters, one byte at a time.  rot13 is an involution -
// applying it twice restores the input - which is the whole point of the codec.
func rot13Bytes(in []byte) []byte {
	out := make([]byte, len(in))
	for i, c := range in {
		switch {
		case c >= 'a' && c <= 'z':
			out[i] = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			out[i] = 'A' + (c-'A'+13)%26
		default:
			out[i] = c
		}
	}
	return out
}

// rot13String applies the same transform to the runes of a string.  CPython's
// rot_13 codec is a str-to-str codec, so letters outside ASCII are left alone
// rather than encoded.
func rot13String(s string) string {
	b := []byte(s)
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r < utf8.RuneSelf {
			out = append(out, rot13Bytes(b[i:i+1])...)
		} else {
			out = append(out, b[i:i+size]...)
		}
		i += size
	}
	return string(out)
}

// encodeWith encodes text with the named codec, returning (bytes, length).
func encodeWith(name string, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "encode() needs an object")
	}
	if b, ok := args[0].(py.Bytes); ok {
		// rot13 is really a bytes transform.  CPython's codecs.encode() with a
		// bytes input and the rot13 codec reaches the str codec and raises
		// TypeError, but the documented round trip - encode to bytes, decode
		// back - is what callers actually use, so bytes go through here.
		if name == "rot13" {
			return py.Bytes(rot13Bytes(b)), nil
		}
	}
	text, err := py.StrAsString(args[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "encoding requires a string")
	}
	if name == "rot13" {
		return py.String(rot13String(text)), nil
	}
	var out []byte
	switch name {
	case "utf-8":
		out = []byte(text)
	case "ascii":
		for _, r := range text {
			if r > 127 {
				return nil, py.ExceptionNewf(py.UnicodeEncodeError, "'ascii' codec can't encode character %q", string(r))
			}
		}
		out = []byte(text)
	case "latin-1":
		for _, r := range text {
			if r > 255 {
				return nil, py.ExceptionNewf(py.UnicodeEncodeError, "'latin-1' codec can't encode character %q", string(r))
			}
			out = append(out, byte(r))
		}
	case "utf-16", "utf-16-le":
		for _, r := range text {
			if r < 0x10000 {
				out = append(out, byte(r), byte(r>>8))
			} else {
				r -= 0x10000
				hi := rune(0xd800 + (r >> 10))
				lo := rune(0xdc00 + (r & 0x3ff))
				for _, h := range []rune{hi, lo} {
					out = append(out, byte(h), byte(h>>8))
				}
			}
		}
	case "utf-16-be":
		for _, r := range text {
			if r < 0x10000 {
				out = append(out, byte(r>>8), byte(r))
			} else {
				r -= 0x10000
				hi := rune(0xd800 + (r >> 10))
				lo := rune(0xdc00 + (r & 0x3ff))
				for _, h := range []rune{hi, lo} {
					out = append(out, byte(h>>8), byte(h))
				}
			}
		}
	case "utf-32":
		for _, r := range text {
			out = append(out, byte(r), byte(r>>8), byte(r>>16), byte(r>>24))
		}
	default:
		return nil, py.ExceptionNewf(LookupErrorType, "unknown encoding: %s", name)
	}
	return py.Tuple{py.Bytes(out), py.Int(len(text))}, nil
}

// decodeWith decodes bytes with the named codec, returning (str, length).
func decodeWith(name string, args py.Tuple) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "decode() needs an object")
	}
	var raw []byte
	isBytes := false
	switch v := args[0].(type) {
	case py.Bytes:
		raw = []byte(v)
		isBytes = true
	case py.String:
		raw = []byte(v)
	default:
		return nil, py.ExceptionNewf(py.TypeError, "decoding requires a bytes-like object")
	}

	if name == "rot13" {
		// Like the encoder, the output type follows the input: bytes in, bytes
		// out, so the documented bytes round trip holds; str in, str out, which
		// is what CPython's rot_13 codec returns.
		if isBytes {
			return py.Bytes(rot13Bytes(raw)), nil
		}
		return py.String(rot13String(string(raw))), nil
	}

	var text string
	switch name {
	case "utf-8":
		if !utf8.Valid(raw) {
			return nil, py.ExceptionNewf(py.UnicodeDecodeError, "'utf-8' codec can't decode bytes")
		}
		text = string(raw)
	case "ascii":
		for _, b := range raw {
			if b > 127 {
				return nil, py.ExceptionNewf(py.UnicodeDecodeError, "'ascii' codec can't decode byte 0x%02x", b)
			}
		}
		text = string(raw)
	case "latin-1":
		runes := make([]rune, len(raw))
		for i, b := range raw {
			runes[i] = rune(b)
		}
		text = string(runes)
	case "utf-16", "utf-16-le":
		runes := []rune{}
		for i := 0; i+1 < len(raw); i += 2 {
			runes = append(runes, rune(uint16(raw[i])|uint16(raw[i+1])<<8))
		}
		text = string(runes)
	case "utf-16-be":
		runes := []rune{}
		for i := 0; i+1 < len(raw); i += 2 {
			runes = append(runes, rune(uint16(raw[i])<<8|uint16(raw[i+1])))
		}
		text = string(runes)
	case "utf-32":
		runes := []rune{}
		for i := 0; i+3 < len(raw); i += 4 {
			runes = append(runes, rune(uint32(raw[i])|uint32(raw[i+1])<<8|uint32(raw[i+2])<<16|uint32(raw[i+3])<<24))
		}
		text = string(runes)
	default:
		return nil, py.ExceptionNewf(LookupErrorType, "unknown encoding: %s", name)
	}
	return py.Tuple{py.String(text), py.Int(len(raw))}, nil
}

// registerNoop exists so that an attempt to register a search function does
// not raise; it cannot take effect here because the registry is fixed.
func registerNoop(self py.Object, args py.Tuple) (py.Object, error) { return py.None, nil }

func init() {
	CodecInfoType.Dict.Set("name", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.String(self.(*CodecInfo).name), nil
	}})
	CodecInfoType.Dict.Set("encode", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*CodecInfo).encoder, nil
	}})
	CodecInfoType.Dict.Set("decode", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*CodecInfo).decoder, nil
	}})
	CodecInfoType.Dict.Set("incrementalencoder", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*CodecInfo).incrementalEncoder, nil
	}})
	CodecInfoType.Dict.Set("incrementaldecoder", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*CodecInfo).incrementalDecoder, nil
	}})
	CodecInfoType.Dict.Set("streamreader", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.None, nil
	}})
	CodecInfoType.Dict.Set("streamwriter", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.None, nil
	}})

	// The four-element tuple form: lookup() returns a CodecInfo, which is
	// also unpackable into (encode, decode, streamreader, streamwriter) as
	// CPython's result is.
	CodecInfoType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		c := self.(*CodecInfo)
		return py.NewIterator(py.Tuple{c.encoder, c.decoder, py.None, py.None}), nil
	}, 0, "Iterate over the codec functions."))
	CodecInfoType.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(4), nil
	}, 0, "Number of codec functions."))
	CodecInfoType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		var key py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "__getitem__", 1, 1, &key); err != nil {
			return nil, err
		}
		n, err := py.IndexInt(key)
		if err != nil {
			return nil, err
		}
		c := self.(*CodecInfo)
		items := []py.Object{c.encoder, c.decoder, py.None, py.None}
		if n < 0 {
			n += len(items)
		}
		if n < 0 || n >= len(items) {
			return nil, py.ExceptionNewf(py.IndexError, "tuple index out of range")
		}
		return items[n], nil
	}, 0, "The codec function at the given index."))
}

// ---------------------------------------------------------------------------
// Go interface bridges
//
// Subscripting, iteration and len go through the Go interfaces rather than
// the type's Dict, so CodecInfo implements them directly - without this,
// "info[0]" raised "'codecs.CodecInfo' object is not subscriptable" even
// though the method was registered.

// codecItems is the four-element tuple a lookup result unpacks to.
func (c *CodecInfo) codecItems() py.Tuple {
	return py.Tuple{c.encoder, c.decoder, py.None, py.None}
}

func (c *CodecInfo) M__getitem__(key py.Object) (py.Object, error) {
	n, err := py.IndexInt(key)
	if err != nil {
		return nil, err
	}
	items := c.codecItems()
	if n < 0 {
		n += len(items)
	}
	if n < 0 || n >= len(items) {
		return nil, py.ExceptionNewf(py.IndexError, "tuple index out of range")
	}
	return items[n], nil
}

func (c *CodecInfo) M__iter__() (py.Object, error) {
	return py.NewIterator(c.codecItems()), nil
}

func (c *CodecInfo) M__len__() (py.Object, error) { return py.Int(4), nil }

var (
	_ py.I__getitem__ = (*CodecInfo)(nil)
	_ py.I__iter__    = (*CodecInfo)(nil)
	_ py.I__len__     = (*CodecInfo)(nil)
)
