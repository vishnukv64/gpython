// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The incremental codec API: IncrementalDecoder / IncrementalEncoder.
//
// These exist because decoding a stream is not the same as decoding one buffer.
// A multi-byte character can be split across two reads, and the decoder has to
// HOLD BACK the partial character until the rest of it arrives rather than
// failing or emitting a replacement.  charset_normalizer -- a dependency of
// requests -- drives exactly that: it feeds bytes ONE AT A TIME
// ("charset_normalizer/cd.py:44-45") and expects the character to appear only
// once its last byte has been seen.

package codecs

import (
	"unicode/utf8"

	"github.com/vishnukv64/gpython/py"
)

const incrementalDecoderDoc = `IncrementalDecoder(errors='strict')

An incremental decoder: bytes in, text out, holding back a partial character
until the rest of it arrives.

The base class takes ONLY the error handling scheme; the encoding comes from
the subclass, which is why this is not constructible as "IncrementalDecoder
('utf-8')" - CPython rejects that too.  Base decode() raises
NotImplementedError, as CPython's does, rather than inventing a result.`

const incrementalEncoderDoc = `IncrementalEncoder(errors='strict')

An incremental encoder: text in, bytes out.`

// decoderState is the Go value behind an IncrementalDecoder instance.
//
// pending holds the bytes of a character whose remaining bytes have not arrived
// yet.  Keeping them is the whole point of the class: without it a split
// character would either raise or be mangled.
type decoderState struct {
	name    string
	errors  string
	pending []byte
}

type encoderState struct {
	name   string
	errors string
}

// newDecoder is the constructor the class itself uses, so that
// IncrementalDecoder("utf-8") - and a subclass that does not override
// __init__ - produces a working decoder.
func newDecoder(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	errors, err := errorsArg(args, kwargs)
	if err != nil {
		return nil, err
	}
	// name is filled in by whoever builds a concrete decoder; the base class on
	// its own has no encoding, exactly as in CPython.
	name := ""
	if metatype != nil {
		if v, ok := metatype.Dict.Get("_encoding"); ok {
			if s, err := py.StrAsString(v); err == nil {
				name = s
			}
		}
	}
	return &decoderState{name: name, errors: errors}, nil
}

// errorsArg reads the one argument the base classes take.
func errorsArg(args py.Tuple, kwargs py.StringDict) (string, error) {
	errors := "strict"
	if len(args) > 0 {
		s, err := py.StrAsString(args[0])
		if err != nil {
			return "", err
		}
		errors = s
	}
	if v, ok := kwargs.Get("errors"); ok {
		s, err := py.StrAsString(v)
		if err != nil {
			return "", err
		}
		errors = s
	}
	return errors, nil
}

func newEncoder(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	errors, err := errorsArg(args, kwargs)
	if err != nil {
		return nil, err
	}
	name := ""
	if metatype != nil {
		if v, ok := metatype.Dict.Get("_encoding"); ok {
			if s, err := py.StrAsString(v); err == nil {
				name = s
			}
		}
	}
	return &encoderState{name: name, errors: errors}, nil
}

var incrementalDecoderType = py.NewTypeX("codecs.IncrementalDecoder", incrementalDecoderDoc, newDecoder, nil)
var incrementalEncoderType = py.NewTypeX("codecs.IncrementalEncoder", incrementalEncoderDoc, newEncoder, nil)

func (d *decoderState) Type() *py.Type { return incrementalDecoderType }
func (e *encoderState) Type() *py.Type { return incrementalEncoderType }

// bytesOf accepts either bytes or bytearray, as CPython does.
//
// A bytearray is handled through its own __bytes__, so this does not need
// access to its (unexported) backing slice.
func bytesOf(o py.Object) ([]byte, bool) {
	switch b := o.(type) {
	case py.Bytes:
		return []byte(b), true
	case *py.ByteArray:
		v, err := b.M__bytes__()
		if err != nil {
			return nil, false
		}
		bs, ok := v.(py.Bytes)
		if !ok {
			return nil, false
		}
		return []byte(bs), true
	}
	return nil, false
}

// decodeOne decodes as much of b as forms complete characters, returning the
// text and the bytes that were not consumed.
func decodeOne(name, errors string, b []byte) (string, []byte, error) {
	if len(b) == 0 {
		return "", nil, nil
	}
	switch name {
	case "ascii":
		for _, c := range b {
			if c > 0x7f {
				if errors == "ignore" {
					continue
				}
				return "", nil, py.ExceptionNewf(py.UnicodeDecodeError,
					"'ascii' codec can't decode byte 0x%02x in position: ordinal not in range(128)", c)
			}
		}
		return string(b), nil, nil
	case "latin-1":
		// Every byte is a character, so nothing is ever held back.
		runes := make([]rune, len(b))
		for i, c := range b {
			runes[i] = rune(c)
		}
		return string(runes), nil, nil
	case "utf-8":
		fallthrough
	default:
		// Walk the bytes and stop at the first incomplete sequence, which is
		// what the caller must hand back next time.
		out := make([]byte, 0, len(b))
		i := 0
		for i < len(b) {
			r, size := utf8.DecodeRune(b[i:])
			if r == utf8.RuneError && size == 1 {
				// Either genuinely invalid, or simply incomplete.  If the
				// remaining bytes could still become a valid character, hold
				// them back.
				if !utf8.FullRune(b[i:]) && len(b)-i < utf8.UTFMax {
					return string(out), b[i:], nil
				}
				if errors == "ignore" {
					i++
					continue
				}
				if errors == "replace" {
					out = append(out, 0xef, 0xbf, 0xbd)
					i++
					continue
				}
				return "", nil, py.ExceptionNewf(py.UnicodeDecodeError,
					"'utf-8' codec can't decode byte 0x%02x in position %d: invalid start byte", b[i], i)
			}
			out = append(out, b[i:i+size]...)
			i += size
		}
		return string(out), nil, nil
	}
}

func (d *decoderState) decode(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "decode() takes at least 1 argument (0 given)")
	}
	b, ok := bytesOf(args[0])
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not '%s'", args[0].Type().Name)
	}
	final := false
	if len(args) > 1 {
		if f, ok := args[1].(py.Bool); ok {
			final = bool(f)
		}
	}
	// The held-back bytes go FIRST: they came before this chunk.
	all := append(append([]byte{}, d.pending...), b...)
	d.pending = nil
	text, rest, err := decodeOne(d.name, d.errors, all)
	if err != nil {
		return nil, err
	}
	if final && len(rest) > 0 {
		// The stream ended mid-character.
		if d.errors == "ignore" {
			return py.String(text), nil
		}
		if d.errors == "replace" {
			return py.String(text + "\ufffd"), nil
		}
		return nil, py.ExceptionNewf(py.UnicodeDecodeError,
			"'utf-8' codec can't decode bytes in position: unexpected end of data")
	}
	d.pending = rest
	return py.String(text), nil
}

func (e *encoderState) encode(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "encode() takes at least 1 argument (0 given)")
	}
	s, err := py.StrAsString(args[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "a str is required, not '%s'", args[0].Type().Name)
	}
	switch e.name {
	case "ascii":
		out := make([]byte, 0, len(s))
		for _, r := range s {
			if r > 0x7f {
				if e.errors == "ignore" {
					continue
				}
				return nil, py.ExceptionNewf(py.UnicodeEncodeError,
					"'ascii' codec can't encode character %q: ordinal not in range(128)", string(r))
			}
			out = append(out, byte(r))
		}
		return py.Bytes(out), nil
	case "latin-1":
		out := make([]byte, 0, len(s))
		for _, r := range s {
			if r > 0xff {
				if e.errors == "ignore" {
					continue
				}
				return nil, py.ExceptionNewf(py.UnicodeEncodeError,
					"'latin-1' codec can't encode character %q: ordinal not in range(256)", string(r))
			}
			out = append(out, byte(r))
		}
		return py.Bytes(out), nil
	default:
		return py.Bytes([]byte(s)), nil
	}
}

func init() {
	// The base class says so rather than guessing: CPython's own
	// IncrementalDecoder.decode raises NotImplementedError, and a codec that
	// did not override it would silently do the wrong thing if this returned
	// something plausible.
	// The BASE class does not decode.  CPython's own IncrementalDecoder.decode
	// raises NotImplementedError, because the base has no encoding: only the
	// per-encoding subclass (encodings.utf_8, encodings.latin_1, ...) knows what
	// conversion to perform.  Decoding as UTF-8 here would have been a
	// plausible wrong answer for every encoding, which is worse than a refusal.
	incrementalDecoderType.Dict.Set("decode", py.MustNewMethod("decode", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		d, ok := self.(*decoderState)
		if !ok || d.name == "" {
			return nil, py.ExceptionNewf(py.NotImplementedError, "")
		}
		return d.decode(args, kwargs)
	}, 0, "Decode bytes, holding back a partial character."))

	incrementalDecoderType.Dict.Set("encode", py.MustNewMethod("encode", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return nil, py.ExceptionNewf(py.NotImplementedError, "codecs.IncrementalDecoder.encode() should be overridden")
	}, 0, "Not supported by a decoder."))

	incrementalDecoderType.Dict.Set("reset", py.MustNewMethod("reset", func(self py.Object, args py.Tuple) (py.Object, error) {
		if d, ok := self.(*decoderState); ok {
			d.pending = nil
		}
		return py.None, nil
	}, 0, "Reset the decoder, discarding any held-back bytes."))

	incrementalDecoderType.Dict.Set("getstate", py.MustNewMethod("getstate", func(self py.Object, args py.Tuple) (py.Object, error) {
		d, ok := self.(*decoderState)
		if !ok {
			return py.Tuple{py.Bytes{}, py.Int(0)}, nil
		}
		return py.Tuple{py.Bytes(append([]byte{}, d.pending...)), py.Int(0)}, nil
	}, 0, "Return the current state, as (pending_bytes, offset)."))

	incrementalDecoderType.Dict.Set("setstate", py.MustNewMethod("setstate", func(self py.Object, args py.Tuple) (py.Object, error) {
		d, ok := self.(*decoderState)
		if !ok {
			return py.None, nil
		}
		if len(args) >= 1 {
			if st, ok := args[0].(py.Tuple); ok && len(st) >= 1 {
				if b, ok := bytesOf(st[0]); ok {
					d.pending = b
				}
			}
		}
		return py.None, nil
	}, 0, "Restore state from getstate()."))

	incrementalDecoderType.Dict.Set("errors", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if d, ok := self.(*decoderState); ok {
				return py.String(d.errors), nil
			}
			return py.String("strict"), nil
		},
		Fset: func(self py.Object, value py.Object) error {
			s, err := py.StrAsString(value)
			if err != nil {
				return err
			}
			if d, ok := self.(*decoderState); ok {
				d.errors = s
			}
			return nil
		},
		Doc: "The error handling scheme.",
	})

	incrementalEncoderType.Dict.Set("encode", py.MustNewMethod("encode", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		e, ok := self.(*encoderState)
		if !ok {
			return nil, py.ExceptionNewf(py.NotImplementedError, "codecs.IncrementalEncoder.encode() should be overridden")
		}
		return e.encode(args, kwargs)
	}, 0, "Encode text to bytes."))

	incrementalEncoderType.Dict.Set("decode", py.MustNewMethod("decode", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return nil, py.ExceptionNewf(py.NotImplementedError, "codecs.IncrementalEncoder.decode() should be overridden")
	}, 0, "Not supported by an encoder."))

	incrementalEncoderType.Dict.Set("reset", py.MustNewMethod("reset", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "Reset the encoder."))

	incrementalEncoderType.Dict.Set("getstate", py.MustNewMethod("getstate", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(0), nil
	}, 0, "Return the current state."))

	incrementalEncoderType.Dict.Set("setstate", py.MustNewMethod("setstate", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "Restore state from getstate()."))

	incrementalEncoderType.Dict.Set("errors", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if e, ok := self.(*encoderState); ok {
				return py.String(e.errors), nil
			}
			return py.String("strict"), nil
		},
		Fset: func(self py.Object, value py.Object) error {
			s, err := py.StrAsString(value)
			if err != nil {
				return err
			}
			if e, ok := self.(*encoderState); ok {
				e.errors = s
			}
			return nil
		},
		Doc: "The error handling scheme.",
	})
}

// NewIncrementalDecoder builds an incremental decoder for a canonical name.
// encodings/ registers one per module; codecs.lookup reaches them through
// CodecInfo.
func NewIncrementalDecoder(name, errors string) py.Object {
	return &decoderState{name: name, errors: errors}
}

// NewIncrementalEncoder builds an incremental encoder for a canonical name.
func NewIncrementalEncoder(name, errors string) py.Object {
	return &encoderState{name: name, errors: errors}
}

// LookupIncrementalDecoder is the callable an encodings module exposes as
// IncrementalDecoder: it takes (errors='strict') and returns a decoder.
func LookupIncrementalDecoder(name string) py.Object {
	return py.MustNewMethod("IncrementalDecoder", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		errors := "strict"
		if len(args) > 0 {
			s, err := py.StrAsString(args[0])
			if err != nil {
				return nil, err
			}
			errors = s
		}
		if v, ok := kwargs.Get("errors"); ok {
			s, err := py.StrAsString(v)
			if err != nil {
				return nil, err
			}
			errors = s
		}
		return NewIncrementalDecoder(name, errors), nil
	}, 0, "Incremental decoder for "+name+".")
}

// LookupIncrementalEncoder is the callable an encodings module exposes as
// IncrementalEncoder: it takes (errors='strict') and returns an encoder.
func LookupIncrementalEncoder(name string) py.Object {
	return py.MustNewMethod("IncrementalEncoder", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		errors, err := errorsArg(args, kwargs)
		if err != nil {
			return nil, err
		}
		return NewIncrementalEncoder(name, errors), nil
	}, 0, "Incremental encoder for "+name+".")
}
