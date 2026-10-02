// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package base64 provides the implementation of python's 'base64' module.
//
// base64 itself is handled by Go's encoding/base64.  base32, base16, ascii85
// and base85 are ports of CPython's own implementations, because their
// behaviour is observable in ways a general-purpose Go codec does not
// reproduce: base32's partial-quantum padding and its map01/casefold options,
// base16's casefold, and ascii85's 'z'/'y' short forms, wrapcol and pad.
// Go has no base85 at all.
//
// Decoding failures raise binascii.Error, the class CPython's base64 module
// raises: it reuses binascii's exception rather than defining its own.
package base64

import (
	"encoding/base64"
	"strings"

	"github.com/vishnukv64/gpython/py"
	stdbinascii "github.com/vishnukv64/gpython/stdlib/binascii"
)

const module_doc = `Base16, Base32, Base64 (RFC 3548), Base85 and Ascii85 data encodings`

// ---------------------------------------------------------------------------
// Input conversion

// decodeData converts a Python str or bytes-like value to a byte slice, the
// conversion CPython's _bytes_from_decode_data performs.
//
// A str must be ASCII: CPython raises ValueError rather than encoding it, and
// that difference is observable, so it is reproduced here.
func decodeData(o py.Object) ([]byte, error) {
	switch v := o.(type) {
	case py.Bytes:
		return []byte(v), nil
	case py.String:
		out := make([]byte, len(v))
		for i := 0; i < len(v); i++ {
			if v[i] > 0x7f {
				return nil, py.ExceptionNewf(py.ValueError, "string argument should contain only ASCII characters")
			}
			out[i] = byte(v[i])
		}
		return out, nil
	}
	b, err := py.BytesFromObject(o)
	if err != nil {
		return nil, err
	}
	return []byte(b), nil
}

// encodeData converts a Python bytes-like value to a byte slice.  The encode
// direction accepts bytes only, so str is rejected as bytes() would reject it.
func encodeData(o py.Object) ([]byte, error) {
	b, err := py.BytesFromObject(o)
	if err != nil {
		return nil, err
	}
	return []byte(b), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// base64

// a2bBase64 is a port of CPython's binascii.a2b_base64.
//
// It is implemented from CPython's algorithm rather than by calling
// encoding/base64 because the lenient mode has observable behaviour - it
// discards invalid characters, ignores stray padding, and stops at the first
// complete pad sequence - that a strict decoder does not reproduce.
func a2bBase64(data []byte, strict bool) ([]byte, error) {
	// table is CPython's table_a2b_base64; 255 marks an invalid byte.
	var table [256]byte
	for i := range table {
		table[i] = 255
	}
	for i := 0; i < 26; i++ {
		table['A'+i] = byte(i)
		table['a'+i] = byte(26 + i)
	}
	for i := 0; i < 10; i++ {
		table['0'+i] = byte(52 + i)
	}
	table['+'] = 62
	table['/'] = 63

	if strict && len(data) > 0 && data[0] == '=' {
		return nil, py.ExceptionNewf(stdbinascii.Error, "Leading padding not allowed")
	}

	out := make([]byte, 0, ((len(data)+3)/4)*3)
	quadPos := 0
	var leftchar byte
	pads := 0
	paddingStarted := false

	for i := 0; i < len(data); i++ {
		thisCh := data[i]

		if thisCh == '=' {
			paddingStarted = true
			if quadPos >= 2 {
				pads++
				if quadPos+pads >= 4 {
					// A complete pad sequence: decoding stops here.  In strict
					// mode trailing data after it is an error.
					if strict && i+1 < len(data) {
						return nil, py.ExceptionNewf(stdbinascii.Error, "Excess data after padding")
					}
					return out, nil
				}
			}
			continue
		}

		v := table[thisCh]
		if v >= 64 {
			if strict {
				return nil, py.ExceptionNewf(stdbinascii.Error, "Only base64 data is allowed")
			}
			continue
		}
		if strict && paddingStarted {
			return nil, py.ExceptionNewf(stdbinascii.Error, "Discontinuous padding not allowed")
		}
		pads = 0

		switch quadPos {
		case 0:
			quadPos = 1
			leftchar = v
		case 1:
			quadPos = 2
			out = append(out, leftchar<<2|v>>4)
			leftchar = v & 0x0f
		case 2:
			quadPos = 3
			out = append(out, leftchar<<4|v>>2)
			leftchar = v & 0x03
		case 3:
			quadPos = 0
			out = append(out, leftchar<<6|v)
			leftchar = 0
		}
	}

	if quadPos != 0 {
		if quadPos == 1 {
			// Exactly one trailing data character is never a valid encoding;
			// CPython reports the input length rather than padding.
			return nil, py.ExceptionNewf(stdbinascii.Error,
				"Invalid base64-encoded string: number of data characters (%d) cannot be 1 more than a multiple of 4",
				len(out)/3*4+1)
		}
		return nil, py.ExceptionNewf(stdbinascii.Error, "Incorrect padding")
	}
	return out, nil
}

func b64encode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var altchars py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:b64encode", []string{"s", "altchars"}, &s, &altchars); err != nil {
		return nil, err
	}
	data, err := encodeData(s)
	if err != nil {
		return nil, err
	}
	enc := base64.StdEncoding.EncodeToString(data)
	if altchars != py.None {
		alt, err := decodeData(altchars)
		if err != nil {
			return nil, err
		}
		if len(alt) != 2 {
			return nil, py.ExceptionNewf(py.ValueError, "altchars must be a string of length 2")
		}
		replaced := make([]byte, len(enc))
		for i := 0; i < len(enc); i++ {
			switch enc[i] {
			case '+':
				replaced[i] = alt[0]
			case '/':
				replaced[i] = alt[1]
			default:
				replaced[i] = enc[i]
			}
		}
		enc = string(replaced)
	}
	return py.Bytes(enc), nil
}

// translateAltChars replaces altchars[0] with '+' and altchars[1] with '/',
// which is what CPython's b64decode does with bytes.translate before decoding.
func translateAltChars(s, altchars []byte) []byte {
	out := make([]byte, len(s))
	for i, c := range s {
		switch c {
		case altchars[0]:
			out[i] = '+'
		case altchars[1]:
			out[i] = '/'
		default:
			out[i] = c
		}
	}
	return out
}

func b64decode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var altchars py.Object = py.None
	var validate py.Object = py.Bool(false)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:b64decode", []string{"s", "altchars", "validate"}, &s, &altchars, &validate); err != nil {
		return nil, err
	}
	strict, err := py.ObjectIsTrue(validate)
	if err != nil {
		return nil, err
	}
	data, err := decodeData(s)
	if err != nil {
		return nil, err
	}
	if altchars != py.None {
		alt, err := decodeData(altchars)
		if err != nil {
			return nil, err
		}
		if len(alt) != 2 {
			return nil, py.ExceptionNewf(py.ValueError, "altchars must be a string of length 2")
		}
		data = translateAltChars(data, alt)
	}
	out, err := a2bBase64(data, strict)
	if err != nil {
		return nil, err
	}
	return py.Bytes(out), nil
}

func urlsafeB64encode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:urlsafe_b64encode", []string{"s"}, &s); err != nil {
		return nil, err
	}
	data, err := encodeData(s)
	if err != nil {
		return nil, err
	}
	return py.Bytes(base64.URLEncoding.EncodeToString(data)), nil
}

func urlsafeB64decode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:urlsafe_b64decode", []string{"s"}, &s); err != nil {
		return nil, err
	}
	data, err := decodeData(s)
	if err != nil {
		return nil, err
	}
	// CPython translates '-' and '_' to '+' and '/' before a plain b64decode,
	// so a '+' or '/' in the input is still accepted.
	translated := make([]byte, len(data))
	for i, c := range data {
		switch c {
		case '-':
			translated[i] = '+'
		case '_':
			translated[i] = '/'
		default:
			translated[i] = c
		}
	}
	out, err := a2bBase64(translated, false)
	if err != nil {
		return nil, err
	}
	return py.Bytes(out), nil
}

// ---------------------------------------------------------------------------
// base32

const b32Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
const b32HexAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUV"

// b32encodeImpl is CPython's _b32encode: the input is read as 5-byte big-endian
// quanta, each producing 8 characters, with the final partial quantum zero
// padded and its unused characters replaced by '='.
func b32encodeImpl(alphabet string, data []byte) []byte {
	leftover := len(data) % 5
	buf := data
	if leftover != 0 {
		buf = append(append([]byte{}, data...), make([]byte, 5-leftover)...)
	}
	out := make([]byte, 0, (len(buf)/5)*8)
	for i := 0; i < len(buf); i += 5 {
		var c uint64
		for j := 0; j < 5; j++ {
			c = c<<8 | uint64(buf[i+j])
		}
		out = append(out,
			alphabet[(c>>35)&0x1f],
			alphabet[(c>>30)&0x1f],
			alphabet[(c>>25)&0x1f],
			alphabet[(c>>20)&0x1f],
			alphabet[(c>>15)&0x1f],
			alphabet[(c>>10)&0x1f],
			alphabet[(c>>5)&0x1f],
			alphabet[c&0x1f],
		)
	}
	// The same tail replacement CPython applies to the last quantum.  copy()
	// would copy only the length of its source, so the pad run is built with
	// strings.Repeat.
	switch leftover {
	case 1:
		copy(out[len(out)-6:], strings.Repeat("=", 6))
	case 2:
		copy(out[len(out)-4:], strings.Repeat("=", 4))
	case 3:
		copy(out[len(out)-3:], strings.Repeat("=", 3))
	case 4:
		copy(out[len(out)-1:], "=")
	}
	return out
}

// b32decodeImpl is CPython's _b32decode.
//
// The final partial quantum is decoded in the same loop as the full ones, then
// its surplus bytes are overwritten: CPython accumulates the digits of the
// whole (partial) quantum, shifts left by 5 bits per pad character and keeps
// only the first (43-5*padchars)/8 bytes of the big-endian result.
func b32decodeImpl(alphabet string, data []byte, casefold bool, map01 []byte) ([]byte, error) {
	if len(data)%8 != 0 {
		return nil, py.ExceptionNewf(stdbinascii.Error, "Incorrect padding")
	}
	if map01 != nil {
		if len(map01) != 1 {
			return nil, py.ExceptionNewf(py.ValueError, "map01 must be a single character")
		}
		// bytes.maketrans(b'01', b'O' + map01): '0' -> 'O', '1' -> map01.
		mapped := make([]byte, len(data))
		for i, c := range data {
			switch c {
			case '0':
				mapped[i] = 'O'
			case '1':
				mapped[i] = map01[0]
			default:
				mapped[i] = c
			}
		}
		data = mapped
	}
	if casefold {
		upper := make([]byte, len(data))
		for i, c := range data {
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			upper[i] = c
		}
		data = upper
	}
	l := len(data)
	trimmed := byteTrimRight(data, '=')
	data = trimmed
	padchars := l - len(data)

	var rev [256]int
	for i := range rev {
		rev[i] = -1
	}
	for i := 0; i < len(alphabet); i++ {
		rev[alphabet[i]] = i
	}

	// decode all quanta, including a trailing partial one
	out := make([]byte, 0, (len(data)/8)*5)
	var acc uint64
	for i := 0; i < len(data); i += 8 {
		end := minInt(i+8, len(data))
		quantum := data[i:end]
		acc = 0
		for _, c := range quantum {
			d := rev[c]
			if d < 0 {
				return nil, py.ExceptionNewf(stdbinascii.Error, "Non-base32 digit found")
			}
			acc = acc<<5 | uint64(d)
		}
		// CPython appends acc.to_bytes(5) for every quantum; for the partial
		// one the surplus bytes are replaced below.
		out = append(out, byte(acc>>32), byte(acc>>24), byte(acc>>16), byte(acc>>8), byte(acc))
	}

	if l%8 != 0 || !(padchars == 0 || padchars == 1 || padchars == 3 || padchars == 4 || padchars == 6) {
		return nil, py.ExceptionNewf(stdbinascii.Error, "Incorrect padding")
	}
	if padchars != 0 && len(out) > 0 {
		// Shift the last quantum's accumulated value out to a whole number of
		// bits and keep the leading bytes that are real data.  CPython assigns
		// to decoded[-5:], which RESIZES the bytearray; Go's copy would leave the
		// length unchanged, so the tail is truncated and the real bytes appended.
		acc <<= 5 * uint(padchars)
		last := []byte{byte(acc >> 32), byte(acc >> 24), byte(acc >> 16), byte(acc >> 8), byte(acc)}
		leftover := (43 - 5*padchars) / 8
		out = append(out[:len(out)-5], last[:leftover]...)
	}
	return out, nil
}

// byteTrimRight removes trailing occurrences of c.
func byteTrimRight(b []byte, c byte) []byte {
	end := len(b)
	for end > 0 && b[end-1] == c {
		end--
	}
	return b[:end]
}

func b32encode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:b32encode", []string{"s"}, &s); err != nil {
		return nil, err
	}
	data, err := encodeData(s)
	if err != nil {
		return nil, err
	}
	return py.Bytes(b32encodeImpl(b32Alphabet, data)), nil
}

func b32decode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var casefold py.Object = py.Bool(false)
	var map01 py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:b32decode", []string{"s", "casefold", "map01"}, &s, &casefold, &map01); err != nil {
		return nil, err
	}
	cf, err := py.ObjectIsTrue(casefold)
	if err != nil {
		return nil, err
	}
	data, err := decodeData(s)
	if err != nil {
		return nil, err
	}
	var m []byte
	if map01 != py.None {
		if m, err = decodeData(map01); err != nil {
			return nil, err
		}
	}
	out, err := b32decodeImpl(b32Alphabet, data, cf, m)
	if err != nil {
		return nil, err
	}
	return py.Bytes(out), nil
}

func b32hexencode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:b32hexencode", []string{"s"}, &s); err != nil {
		return nil, err
	}
	data, err := encodeData(s)
	if err != nil {
		return nil, err
	}
	return py.Bytes(b32encodeImpl(b32HexAlphabet, data)), nil
}

func b32hexdecode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var casefold py.Object = py.Bool(false)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:b32hexdecode", []string{"s", "casefold"}, &s, &casefold); err != nil {
		return nil, err
	}
	cf, err := py.ObjectIsTrue(casefold)
	if err != nil {
		return nil, err
	}
	data, err := decodeData(s)
	if err != nil {
		return nil, err
	}
	// base32hex has no 01 mapping argument in CPython.
	out, err := b32decodeImpl(b32HexAlphabet, data, cf, nil)
	if err != nil {
		return nil, err
	}
	return py.Bytes(out), nil
}

// ---------------------------------------------------------------------------
// base16

const hexDigits = "0123456789ABCDEF"

func b16encode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:b16encode", []string{"s"}, &s); err != nil {
		return nil, err
	}
	data, err := encodeData(s)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(data)*2)
	for _, b := range data {
		out = append(out, hexDigits[b>>4], hexDigits[b&0x0f])
	}
	return py.Bytes(out), nil
}

func b16decode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var casefold py.Object = py.Bool(false)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:b16decode", []string{"s", "casefold"}, &s, &casefold); err != nil {
		return nil, err
	}
	cf, err := py.ObjectIsTrue(casefold)
	if err != nil {
		return nil, err
	}
	data, err := decodeData(s)
	if err != nil {
		return nil, err
	}
	if cf {
		upper := make([]byte, len(data))
		for i, c := range data {
			if c >= 'a' && c <= 'f' {
				c -= 'a' - 'A'
			}
			upper[i] = c
		}
		data = upper
	}
	// CPython deletes legal digits and complains if anything remains, so a
	// lowercase input gives 'Non-base16 digit found' unless casefold was asked
	// for.  The odd-length check comes afterwards, from unhexlify.
	for _, c := range data {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return nil, py.ExceptionNewf(stdbinascii.Error, "Non-base16 digit found")
		}
	}
	if len(data)%2 != 0 {
		return nil, py.ExceptionNewf(stdbinascii.Error, "Odd-length string")
	}
	out := make([]byte, len(data)/2)
	for i := range out {
		out[i] = hexVal(data[2*i])<<4 | hexVal(data[2*i+1])
	}
	return py.Bytes(out), nil
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// ---------------------------------------------------------------------------
// ascii85 / base85

// b85Alphabet is CPython's base85 alphabet (the one Mercurial uses).
const b85Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%&()*+-;<=>?@^_`{|}~"

// ascii85Alphabet is '!' through 'u'.
var ascii85Alphabet = func() string {
	b := make([]byte, 0, 85)
	for i := 33; i < 118; i++ {
		b = append(b, byte(i))
	}
	return string(b)
}()

// encode85 is a port of CPython's _85encode.  encoding/ascii85 cannot express
// this because of the foldspaces 'y' form and the pad argument.
func encode85(data []byte, alphabet string, pad, foldnuls, foldspaces bool) []byte {
	padding := (4 - len(data)%4) % 4
	buf := data
	if padding != 0 {
		buf = append(append([]byte{}, data...), make([]byte, padding)...)
	}
	chunks := make([][]byte, 0, len(buf)/4)
	for i := 0; i < len(buf); i += 4 {
		word := uint32(buf[i])<<24 | uint32(buf[i+1])<<16 | uint32(buf[i+2])<<8 | uint32(buf[i+3])
		var chunk []byte
		switch {
		case foldnuls && word == 0:
			chunk = []byte{'z'}
		case foldspaces && word == 0x20202020:
			chunk = []byte{'y'}
		default:
			// The five digits of word in base 85, most significant first,
			// exactly as CPython's chars2[word//614125] + chars2[word//85%7225]
			// + chars[word%85] expands (chars2 is itself pairs of digits).
			chunk = []byte{
				alphabet[word/52200625%85],
				alphabet[word/614125%85],
				alphabet[word/7225%85],
				alphabet[word/85%85],
				alphabet[word%85],
			}
		}
		chunks = append(chunks, chunk)
	}
	if padding != 0 && !pad {
		last := chunks[len(chunks)-1]
		if len(last) == 1 && last[0] == 'z' {
			// A zero-padded tail was folded to 'z' but the padding has to be
			// visible, so it expands back to five zeros before the tail is cut.
			chunks[len(chunks)-1] = []byte(strings.Repeat(string(alphabet[0]), 5))
		}
		last = chunks[len(chunks)-1]
		chunks[len(chunks)-1] = last[:len(last)-padding]
	}
	var out []byte
	for _, c := range chunks {
		out = append(out, c...)
	}
	return out
}

// a85encodeImpl is CPython's a85encode: _85encode with the always-on 'z' fold,
// optional 'y' fold, optional wrapcol, and optional adobe framing.
func a85encodeImpl(data []byte, foldspaces, pad, adobe bool, wrapcol int) []byte {
	result := string(encode85(data, ascii85Alphabet, pad, true, foldspaces))
	if adobe {
		result = "<~" + result
	}
	if wrapcol > 0 {
		wc := wrapcol
		if adobe {
			// CPython widens the wrap so the framing markers stay on the line.
			if wc < 2 {
				wc = 2
			}
			// Split, then add an empty trailing chunk when the last chunk plus
			// the two-byte end marker would overflow.
			chunks := splitEvery(result, wc)
			if len(chunks[len(chunks)-1])+2 > wc {
				chunks = append(chunks, "")
			}
			result = strings.Join(chunks, "\n")
		} else {
			if wc < 1 {
				wc = 1
			}
			result = strings.Join(splitEvery(result, wc), "\n")
		}
	}
	if adobe {
		result += "~>"
	}
	return []byte(result)
}

// splitEvery breaks s into chunks of at most n bytes.
func splitEvery(s string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	var chunks []string
	for i := 0; i < len(s); i += n {
		chunks = append(chunks, s[i:minInt(i+n, len(s))])
	}
	if len(chunks) == 0 {
		chunks = []string{""}
	}
	return chunks
}

func a85encode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var foldspaces py.Object = py.Bool(false)
	var wrapcol py.Object = py.Int(0)
	var pad py.Object = py.Bool(false)
	var adobe py.Object = py.Bool(false)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|$OOOO:a85encode",
		[]string{"b", "foldspaces", "wrapcol", "pad", "adobe"},
		&s, &foldspaces, &wrapcol, &pad, &adobe); err != nil {
		return nil, err
	}
	fs, err := py.ObjectIsTrue(foldspaces)
	if err != nil {
		return nil, err
	}
	pd, err := py.ObjectIsTrue(pad)
	if err != nil {
		return nil, err
	}
	ad, err := py.ObjectIsTrue(adobe)
	if err != nil {
		return nil, err
	}
	wc, err := py.IndexInt(wrapcol)
	if err != nil {
		return nil, err
	}
	data, err := encodeData(s)
	if err != nil {
		return nil, err
	}
	return py.Bytes(a85encodeImpl(data, fs, pd, ad, wc)), nil
}

func b85encode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var pad py.Object = py.Bool(false)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|$O:b85encode", []string{"b", "pad"}, &s, &pad); err != nil {
		return nil, err
	}
	pd, err := py.ObjectIsTrue(pad)
	if err != nil {
		return nil, err
	}
	data, err := encodeData(s)
	if err != nil {
		return nil, err
	}
	// base85 never folds, so foldnuls and foldspaces are both false.
	return py.Bytes(encode85(data, b85Alphabet, pd, false, false)), nil
}

func a85decode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var foldspaces py.Object = py.Bool(false)
	var adobe py.Object = py.Bool(false)
	var ignorechars py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|$OOO:a85decode",
		[]string{"b", "foldspaces", "adobe", "ignorechars"},
		&s, &foldspaces, &adobe, &ignorechars); err != nil {
		return nil, err
	}
	fs, err := py.ObjectIsTrue(foldspaces)
	if err != nil {
		return nil, err
	}
	ad, err := py.ObjectIsTrue(adobe)
	if err != nil {
		return nil, err
	}
	ignore := " \t\n\r\v"
	if ignorechars != py.None {
		b, err := decodeData(ignorechars)
		if err != nil {
			return nil, err
		}
		ignore = string(b)
	}
	data, err := decodeData(s)
	if err != nil {
		return nil, err
	}
	if ad {
		if !strings.HasSuffix(string(data), "~>") {
			return nil, py.ExceptionNewf(py.ValueError, "Ascii85 encoded byte sequences must end with b'~>'")
		}
		if strings.HasPrefix(string(data), "<~") {
			data = data[2 : len(data)-2] // strip the start and end markers
		} else {
			data = data[:len(data)-2] // strip only the end marker
		}
	}
	return decodeA85(data, fs, ignore)
}

// decodeA85 is a port of CPython's a85decode loop, including the 'z' short form
// and the optional 'y' four-space form.
func decodeA85(b []byte, foldspaces bool, ignorechars string) (py.Object, error) {
	var decoded []byte
	var curr []byte
	// CPython appends four 'u' characters so a trailing partial group is
	// flushed into the decoded buffer; the surplus bytes are trimmed below.
	extended := append(append([]byte{}, b...), 'u', 'u', 'u', 'u')
	for _, x := range extended {
		switch {
		case x >= '!' && x <= 'u':
			curr = append(curr, x)
			if len(curr) == 5 {
				var acc uint32
				for _, c := range curr {
					acc = 85*acc + uint32(c-33)
				}
				curr = curr[:0]
				decoded = append(decoded, byte(acc>>24), byte(acc>>16), byte(acc>>8), byte(acc))
			}
		case x == 'z':
			if len(curr) != 0 {
				return nil, py.ExceptionNewf(py.ValueError, "z inside Ascii85 5-tuple")
			}
			decoded = append(decoded, 0, 0, 0, 0)
		case foldspaces && x == 'y':
			if len(curr) != 0 {
				return nil, py.ExceptionNewf(py.ValueError, "y inside Ascii85 5-tuple")
			}
			decoded = append(decoded, ' ', ' ', ' ', ' ')
		case strings.IndexByte(ignorechars, x) >= 0:
			continue
		default:
			return nil, py.ExceptionNewf(py.ValueError, "Non-Ascii85 digit found: %c", x)
		}
	}
	padding := 4 - len(curr)
	if padding != 0 && len(decoded) >= padding {
		decoded = decoded[:len(decoded)-padding]
	}
	return py.Bytes(decoded), nil
}

func b85decode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:b85decode", []string{"b"}, &s); err != nil {
		return nil, err
	}
	data, err := decodeData(s)
	if err != nil {
		return nil, err
	}
	return decodeB85(data)
}

// decodeB85 is a port of CPython's b85decode.  A partial trailing group is
// padded with '~' (the highest digit) and the surplus output bytes dropped.
func decodeB85(b []byte) (py.Object, error) {
	padding := (5 - len(b)%5) % 5
	buf := append(append([]byte{}, b...), []byte(strings.Repeat("~", padding))...)

	var out []byte
	for i := 0; i < len(buf); i += 5 {
		chunk := buf[i : i+5]
		var acc uint64
		for j, c := range chunk {
			d := strings.IndexByte(b85Alphabet, c)
			if d < 0 {
				return nil, py.ExceptionNewf(py.ValueError, "bad base85 character at position %d", i+j)
			}
			acc = acc*85 + uint64(d)
		}
		if acc > 0xffffffff {
			return nil, py.ExceptionNewf(py.ValueError, "base85 overflow in hunk starting at byte %d", i)
		}
		out = append(out, byte(acc>>24), byte(acc>>16), byte(acc>>8), byte(acc))
	}
	if padding != 0 {
		out = out[:len(out)-padding]
	}
	return py.Bytes(out), nil
}

// ---------------------------------------------------------------------------

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "base64",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("b64encode", b64encode, 0,
				"Encode the bytes-like object s using Base64 and return a bytes object.\n\nOptional altchars should be a byte string of length 2 which specifies an\nalternative alphabet for '+' and '/'."),
			py.MustNewMethod("b64decode", b64decode, 0,
				"Decode the Base64 encoded bytes-like object or ASCII string s.\n\nOptional altchars must be a bytes-like object or ASCII string of length 2\nwhich specifies the alternative alphabet used instead of the '+' and '/'\ncharacters."),
			py.MustNewMethod("standard_b64encode", b64encode, 0, "Encode bytes-like object s using the standard Base64 alphabet."),
			py.MustNewMethod("standard_b64decode", b64decode, 0, "Decode bytes encoded with the standard Base64 alphabet."),
			py.MustNewMethod("urlsafe_b64encode", urlsafeB64encode, 0,
				"Encode bytes using the URL- and filesystem-safe Base64 alphabet.\n\nThe alphabet uses '-' instead of '+' and '_' instead of '/'."),
			py.MustNewMethod("urlsafe_b64decode", urlsafeB64decode, 0,
				"Decode bytes using the URL- and filesystem-safe Base64 alphabet.\n\nThe alphabet uses '-' instead of '+' and '_' instead of '/'."),
			py.MustNewMethod("b32encode", b32encode, 0, "Encode the bytes-like objects using base32 and return a bytes object."),
			py.MustNewMethod("b32decode", b32decode, 0, "Decode the base32 encoded bytes-like object or ASCII string s."),
			py.MustNewMethod("b32hexencode", b32hexencode, 0, "Encode the bytes-like objects using base32hex and return a bytes object."),
			py.MustNewMethod("b32hexdecode", b32hexdecode, 0, "Decode the base32hex encoded bytes-like object or ASCII string s."),
			py.MustNewMethod("b16encode", b16encode, 0, "Encode the bytes-like object s using Base16 and return a bytes object."),
			py.MustNewMethod("b16decode", b16decode, 0,
				"Decode the Base16 encoded bytes-like object or ASCII string s.\n\nOptional casefold is a flag specifying whether a lowercase alphabet is\nacceptable as input.  For security purposes, the default is False."),
			py.MustNewMethod("a85encode", a85encode, 0, "Encode bytes-like object b using Ascii85 and return a bytes object."),
			py.MustNewMethod("a85decode", a85decode, 0, "Decode the Ascii85 encoded bytes-like object or ASCII string b."),
			py.MustNewMethod("b85encode", b85encode, 0, "Encode bytes-like object b in base85 format and return a bytes object."),
			py.MustNewMethod("b85decode", b85decode, 0, "Decode the base85-encoded bytes-like object or ASCII string b."),
		},
		Globals: py.NewStringDict(),
	})
}
