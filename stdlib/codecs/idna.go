// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// encodings.idna -- Internationalized Domain Names in Applications.
//
// ToASCII and ToUnicode implement the RFC 3490 conversions on top of the
// RFC 3492 punycode algorithm, so a non-ASCII label round-trips through its
// "xn--" ACE form.  requests imports this module for its side effect (so the
// codec is registered before a thread can race the import) and never calls
// into it; the implementations are provided so that the import is honest and
// the conversions work.
package codecs

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const idnaDoc = `Internationalized Domain Names in Applications (IDNA).

ToASCII(label) -- convert a label to its ASCII-compatible encoding.
ToUnicode(label) -- convert a label from ASCII-compatible encoding.
`

const (
	acePrefix   = "xn--"
	base        = 36
	tmin        = 1
	tmax        = 26
	skew        = 38
	damp        = 700
	initialBias = 72
	initialN    = 128
)

// punyEncode encodes a Unicode label to punycode (RFC 3492).
func punyEncode(input string) string {
	runes := []rune(input)
	var out strings.Builder
	basic := 0
	for _, r := range runes {
		if r < 0x80 {
			out.WriteRune(r)
			basic++
		}
	}
	h := basic
	if basic > 0 && basic < len(runes) {
		out.WriteByte('-')
	}
	n := int64(initialN)
	delta := int64(0)
	bias := int64(initialBias)
	for h < len(runes) {
		var m int64 = 1 << 30
		for _, r := range runes {
			if int64(r) >= n && int64(r) < m {
				m = int64(r)
			}
		}
		delta += (m - n) * int64(h+1)
		n = m
		for _, r := range runes {
			c := int64(r)
			if c < n {
				delta++
			}
			if c == n {
				q := delta
				for k := int64(base); ; k += base {
					t := k - bias
					if t < tmin {
						t = tmin
					} else if t > tmax {
						t = tmax
					}
					if q < t {
						break
					}
					out.WriteByte(digitToChar(t + (q-t)%(base-t)))
					q = (q - t) / (base - t)
				}
				out.WriteByte(digitToChar(q))
				bias = adapt(delta, int64(h+1), h == basic)
				delta = 0
				h++
			}
		}
		delta++
		n++
	}
	return out.String()
}

// punyDecode decodes a punycode string (RFC 3492).
func punyDecode(input string) (string, bool) {
	var out []rune
	b := strings.LastIndexByte(input, '-')
	if b < 0 {
		b = 0
	}
	for i := 0; i < b; i++ {
		out = append(out, rune(input[i]))
	}
	i := b
	if i > 0 {
		i++ // skip the '-'
	}
	n := int64(initialN)
	delta := int64(0)
	bias := int64(initialBias)
	for i < len(input) {
		oldi := delta
		w := int64(1)
		for k := int64(base); ; k += base {
			if i >= len(input) {
				return "", false
			}
			d := charToDigit(input[i])
			if d < 0 {
				return "", false
			}
			i++
			delta += int64(d) * w
			t := k - bias
			if t < tmin {
				t = tmin
			} else if t > tmax {
				t = tmax
			}
			if int64(d) < t {
				break
			}
			w *= base - t
		}
		bias = adapt(delta-oldi, int64(len(out)+1), oldi == 0)
		n += delta / int64(len(out)+1)
		delta %= int64(len(out) + 1)
		if n > 0x10FFFF {
			return "", false
		}
		out = append(out[:delta], append([]rune{rune(n)}, out[delta:]...)...)
		delta++
	}
	return string(out), true
}

func digitToChar(d int64) byte {
	if d < 26 {
		return byte('a' + d)
	}
	return byte('0' + d - 26)
}

func charToDigit(c byte) int {
	switch {
	case c >= 'a' && c <= 'z':
		return int(c - 'a')
	case c >= 'A' && c <= 'Z':
		return int(c - 'A')
	case c >= '0' && c <= '9':
		return int(c-'0') + 26
	}
	return -1
}

// adapt is the RFC 3492 bias adaptation function.
func adapt(delta, numPoints int64, firstTime bool) int64 {
	if firstTime {
		delta /= damp
	} else {
		delta /= 2
	}
	delta += delta / numPoints
	k := int64(0)
	for delta > ((base-tmin)*tmax)/2 {
		delta /= base - tmin
		k += base
	}
	return k + (base-tmin+1)*delta/(delta+skew)
}

// toASCII converts one label to its ACE form.  An all-ASCII label shorter than
// 64 bytes is returned unchanged; otherwise (and for a non-ASCII label) the
// "xn--" punycode form is produced, matching CPython's encodings.idna.
func toASCII(label string) (string, error) {
	if isASCII(label) {
		if len(label) == 0 {
			return "", py.ExceptionNewf(py.UnicodeError, "label empty")
		}
		if len(label) < 64 {
			return label, nil
		}
		return "", py.ExceptionNewf(py.UnicodeError, "label too long")
	}
	encoded := acePrefix + punyEncode(label)
	if len(encoded) > 63 {
		return "", py.ExceptionNewf(py.UnicodeError, "label too long")
	}
	return encoded, nil
}

// toUnicode converts one ACE label back to Unicode.
func toUnicode(label string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(label), acePrefix) {
		return label, nil
	}
	decoded, ok := punyDecode(label[len(acePrefix):])
	if !ok {
		return "", py.ExceptionNewf(py.UnicodeError, "Invalid character in IDN label")
	}
	return decoded, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func idnaToASCII(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "ToASCII() takes exactly one argument")
	}
	s, err := py.StrAsString(args[0])
	if err != nil {
		// A bytes argument is decoded first, as CPython does.
		b, berr := py.BytesFromObject(args[0])
		if berr != nil {
			return nil, err
		}
		s = string(b)
	}
	// CPython's ToASCII operates on a single label and does NOT split on
	// dots; the caller is expected to pass one label.
	out, err := toASCII(s)
	if err != nil {
		return nil, err
	}
	return py.Bytes([]byte(out)), nil
}

func idnaToUnicode(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "ToUnicode() takes exactly one argument")
	}
	var s string
	if b, err := py.BytesFromObject(args[0]); err == nil {
		s = string(b)
	} else {
		var serr error
		s, serr = py.StrAsString(args[0])
		if serr != nil {
			return nil, serr
		}
	}
	out, err := toUnicode(s)
	if err != nil {
		return nil, err
	}
	return py.String(out), nil
}

func init() {
	globals := py.NewStringDict()
	globals.Set("ToASCII", py.MustNewMethod("ToASCII", idnaToASCII, 0,
		"ToASCII(label) -- convert a label to its ASCII-compatible encoding."))
	globals.Set("ToUnicode", py.MustNewMethod("ToUnicode", idnaToUnicode, 0,
		"ToUnicode(label) -- convert a label from ASCII-compatible encoding."))
	globals.Set("ace_prefix", py.Bytes([]byte(acePrefix)))
	globals.Set("nameprep", py.MustNewMethod("nameprep", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "nameprep() takes exactly one argument")
		}
		return args[0], nil
	}, 0, "nameprep(label) -- return the label unchanged (no stringprep tables here)."))
	globals.Set("Codec", py.MustNewMethod("Codec", func(self py.Object, args py.Tuple) (py.Object, error) {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"encodings.idna.Codec: the incremental idna codec class is not implemented; "+
				"ToASCII and ToUnicode are the supported entry points")
	}, 0, "The idna codec class (not implemented)."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "encodings.idna",
			Doc:  idnaDoc,
		},
		Globals: globals,
	})

	// The encodings package itself, so "import encodings.idna" resolves the
	// parent before the submodule.
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "encodings",
			Doc:  "Standard codecs for the Python text model.",
		},
		Globals: py.NewStringDict(),
	})
}
