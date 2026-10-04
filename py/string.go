// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// String objects
//
// Note that string objects in Python are arrays of unicode
// characters.  However we are using the native Go string which is
// UTF-8 encoded.  This makes very little difference most of the time,
// but care is needed when indexing, slicing or iterating through
// strings.

package py

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type String string

var StringType = ObjectType.NewType("str",
	`str(object='') -> str
str(bytes_or_buffer[, encoding[, errors]]) -> str

Create a new string object from the given object. If encoding or
errors is specified, then the object must expose a data buffer
that will be decoded using the given encoding and error handler.
Otherwise, returns the result of object.__str__() (if defined)
or repr(object).
encoding defaults to sys.getdefaultencoding().
errors defaults to 'strict'.`, StrNew, nil)

// Escape the py.String
func StringEscape(a String, ascii bool) string {
	s := string(a)
	var out bytes.Buffer
	quote := '\''
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	if !ascii {
		out.WriteRune(quote)
	}
	for _, c := range s {
		switch {
		case c < 0x20:
			switch c {
			case '\t':
				out.WriteString(`\t`)
			case '\n':
				out.WriteString(`\n`)
			case '\r':
				out.WriteString(`\r`)
			default:
				fmt.Fprintf(&out, `\x%02x`, c)
			}
		case !ascii && c < 0x7F:
			if c == '\\' || (quote == '\'' && c == '\'') || (quote == '"' && c == '"') {
				out.WriteRune('\\')
			}
			out.WriteRune(c)
		case c < 0x100:
			if ascii || strconv.IsPrint(c) {
				out.WriteRune(c)
			} else {
				fmt.Fprintf(&out, "\\x%02x", c)
			}
		case c < 0x10000:
			if !ascii && strconv.IsPrint(c) {
				out.WriteRune(c)
			} else {
				fmt.Fprintf(&out, "\\u%04x", c)
			}
		default:
			if !ascii && strconv.IsPrint(c) {
				out.WriteRune(c)
			} else {
				fmt.Fprintf(&out, "\\U%08x", c)
			}
		}
	}
	if !ascii {
		out.WriteRune(quote)
	}
	return out.String()
}

// standard golang strings.Fields doesn't have a 'first N' argument
func fieldsN(s string, n int) []string {
	out := []string{}
	cur := []rune{}
	for _, c := range s {
		//until we have covered the first N elements, multiple white-spaces are 'merged'
		if n < 0 || len(out) < n {
			if unicode.IsSpace(c) {
				if len(cur) > 0 {
					out = append(out, string(cur))
					cur = []rune{}
				}
			} else {
				cur = append(cur, c)
			}
			//until we see the next letter, after collecting the first N fields, continue to merge whitespaces
		} else if len(out) == n && len(cur) == 0 {
			if !unicode.IsSpace(c) {
				cur = append(cur, c)
			}
			//now that enough words have been collected, just copy into the last element
		} else {
			cur = append(cur, c)
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func init() {
	// str.encode mirrors bytes.decode, sharing encodeStringWith so both go
	// through one canonical-encoding table.
	StringType.Dict.Set("encode", MustNewMethod("encode", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		encoding := Object(String("utf-8"))
		errors := Object(String("strict"))
		if err := ParseTupleAndKeywords(args, kwargs, "|OO:encode",
			[]string{"encoding", "errors"}, &encoding, &errors); err != nil {
			return nil, err
		}
		enc, ok := encoding.(String)
		if !ok {
			return nil, ExceptionNewf(TypeError, "encode() argument 'encoding' must be str, not %s", encoding.Type().Name)
		}
		errs, ok := errors.(String)
		if !ok {
			return nil, ExceptionNewf(TypeError, "encode() argument 'errors' must be str, not %s", errors.Type().Name)
		}
		b, err := encodeStringWith(string(enc), string(self.(String)), string(errs))
		if err != nil {
			return nil, err
		}
		return Bytes(b), nil
	}, 0, `encode(encoding='utf-8', errors='strict') -> bytes

Encode the string using the codec registered for encoding.`))

	StringType.Dict.Set("endswith", MustNewMethod("endswith", func(self Object, args Tuple) (Object, error) {
		return strStartsEndsWith(self.(String), args, false)
	}, 0, "endswith(suffix[, start[, end]]) -> bool"))

	StringType.Dict.Set("count", MustNewMethod("count", func(self Object, args Tuple) (Object, error) {
		return self.(String).Count(args)
	}, 0, `count(sub[, start[, end]]) -> int
Return the number of non-overlapping occurrences of substring sub in
string S[start:end].  Optional arguments start and end are
interpreted as in slice notation.`))

	StringType.Dict.Set("find", MustNewMethod("find", func(self Object, args Tuple) (Object, error) {
		return self.(String).find(args)
	}, 0, `find(...)
S.find(sub[, start[, end]]) -> int

Return the lowest index in S where substring sub is found,
such that sub is contained within S[start:end].  Optional
arguments start and end are interpreted as in slice notation.

Return -1 on failure.`))

	StringType.Dict.Set("replace", MustNewMethod("replace", func(self Object, args Tuple) (Object, error) {
		return self.(String).Replace(args)
	}, 0, `replace(self, old, new, count=-1) -> return a copy with all occurrences of substring old replaced by new.

  count
    Maximum number of occurrences to replace.
    -1 (the default value) means replace all occurrences.

If the optional argument count is given, only the first count occurrences are
replaced.`))

	StringType.Dict.Set("rsplit", MustNewMethod("rsplit", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(String).RSplit(args, kwargs)
	}, 0, `rsplit(sep=None, maxsplit=-1) -> list of strings

Return a list of the words in the string, using sep as the delimiter, counting
from the right; at most maxsplit splits are made.`))

	StringType.Dict.Set("split", MustNewMethod("split", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(String).Split(args, kwargs)
	}, 0, "split(sub) -> split string with sub."))

	StringType.Dict.Set("startswith", MustNewMethod("startswith", func(self Object, args Tuple) (Object, error) {
		return strStartsEndsWith(self.(String), args, true)
	}, 0, "startswith(prefix[, start[, end]]) -> bool"))

	StringType.Dict.Set("strip", MustNewMethod("strip", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(String).Strip(args)
	}, 0, "strip(chars) -> replace chars from begining and end of string"))

	StringType.Dict.Set("rstrip", MustNewMethod("rstrip", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(String).RStrip(args)
	}, 0, "rstrip(chars) -> replace chars from end of string"))

	StringType.Dict.Set("lstrip", MustNewMethod("lstrip", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(String).LStrip(args)
	}, 0, "lstrip(chars) -> replace chars from begining of string"))

	StringType.Dict.Set("upper", MustNewMethod("upper", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(String).Upper()
	}, 0, "upper() -> a copy of the string converted to uppercase"))

	StringType.Dict.Set("lower", MustNewMethod("lower", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(String).Lower()
	}, 0, "lower() -> a copy of the string converted to lowercase"))

	StringType.Dict.Set("zfill", MustNewMethod("zfill", func(self Object, args Tuple) (Object, error) {
		return self.(String).ZFill(args)
	}, 0, "zfill(width) -> pad a numeric string with zeros on the left."))

	StringType.Dict.Set("center", MustNewMethod("center", func(self Object, args Tuple) (Object, error) {
		return self.(String).Justify(args, 0)
	}, 0, "center(width[, fillchar]) -> centred in a string of the given width."))

	StringType.Dict.Set("ljust", MustNewMethod("ljust", func(self Object, args Tuple) (Object, error) {
		return self.(String).Justify(args, -1)
	}, 0, "ljust(width[, fillchar]) -> left-justified in a string of the given width."))

	StringType.Dict.Set("expandtabs", MustNewMethod("expandtabs", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return self.(String).ExpandTabs(args, kwargs)
	}, 0, "expandtabs(tabsize=8) -> expand tabs, honouring column position."))

	StringType.Dict.Set("removeprefix", MustNewMethod("removeprefix", func(self Object, args Tuple) (Object, error) {
		return self.(String).RemoveAffix(args, true)
	}, 0, "removeprefix(prefix) -> the string without that prefix, if present."))

	StringType.Dict.Set("removesuffix", MustNewMethod("removesuffix", func(self Object, args Tuple) (Object, error) {
		return self.(String).RemoveAffix(args, false)
	}, 0, "removesuffix(suffix) -> the string without that suffix, if present."))

	StringType.Dict.Set("rfind", MustNewMethod("rfind", func(self Object, args Tuple) (Object, error) {
		return self.(String).FindMethod(args, "rfind", true, false)
	}, 0, "rfind(sub[, start[, end]]) -> the highest index, or -1."))

	StringType.Dict.Set("index", MustNewMethod("index", func(self Object, args Tuple) (Object, error) {
		return self.(String).FindMethod(args, "index", false, true)
	}, 0, "index(sub[, start[, end]]) -> the lowest index, or ValueError."))

	StringType.Dict.Set("rindex", MustNewMethod("rindex", func(self Object, args Tuple) (Object, error) {
		return self.(String).FindMethod(args, "rindex", true, true)
	}, 0, "rindex(sub[, start[, end]]) -> the highest index, or ValueError."))

	StringType.Dict.Set("partition", MustNewMethod("partition", func(self Object, args Tuple) (Object, error) {
		return self.(String).Partition(args, false)
	}, 0, "partition(sep) -> (before, sep, after), or (self, '', '')."))

	StringType.Dict.Set("rpartition", MustNewMethod("rpartition", func(self Object, args Tuple) (Object, error) {
		return self.(String).Partition(args, true)
	}, 0, "rpartition(sep) -> the same, splitting at the LAST occurrence."))

	StringType.Dict.Set("maketrans", MustNewMethod("maketrans", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return strMakeTrans(args)
	}, METH_CLASS, `maketrans(x, y=None, z=None) -> dict

Return a translation table usable by str.translate.

If there is only one argument, it must be a mapping of Unicode ordinals to
Unicode ordinals, strings or None.  If there are two arguments, they must be
strings of equal length.  With three arguments, the third must be a string of
characters to map to None.`))

	StringType.Dict.Set("translate", MustNewMethod("translate", func(self Object, args Tuple) (Object, error) {
		return self.(String).Translate(args)
	}, 0, "translate(table) -> the string with each character mapped through table."))
	StringType.Dict.Set("join", MustNewMethod("join", func(self Object, args Tuple) (Object, error) {
		return self.(String).Join(args)
	}, 0, "join(iterable) -> return a string which is the concatenation of the strings in iterable"))

	// The search-and-split family.  Each pairs with the one already present:
	// find/rfind, index/rindex, split/rsplit.
	strArgs := func(name string, args Tuple, n int) (string, error) {
		var sep Object
		if err := UnpackTuple(args, StringDict{}, name, 1, n, &sep); err != nil {
			return "", err
		}
		sepStr, err := StrAsString(sep)
		if err != nil {
			return "", err
		}
		if sepStr == "" {
			return "", ExceptionNewf(ValueError, "empty separator")
		}
		return sepStr, nil
	}

	StringType.Dict.Set("partition", MustNewMethod("partition", func(self Object, args Tuple) (Object, error) {
		s := string(self.(String))
		sep, err := strArgs("partition", args, 1)
		if err != nil {
			return nil, err
		}
		if i := strings.Index(s, sep); i >= 0 {
			return Tuple{String(s[:i]), String(sep), String(s[i+len(sep):])}, nil
		}
		return Tuple{String(s), String(""), String("")}, nil
	}, 0, "partition(sep) -> (before, sep, after); sep is empty when not found"))

	StringType.Dict.Set("rpartition", MustNewMethod("rpartition", func(self Object, args Tuple) (Object, error) {
		s := string(self.(String))
		sep, err := strArgs("rpartition", args, 1)
		if err != nil {
			return nil, err
		}
		if i := strings.LastIndex(s, sep); i >= 0 {
			return Tuple{String(s[:i]), String(sep), String(s[i+len(sep):])}, nil
		}
		// Searching from the right, the text is the LAST element.
		return Tuple{String(""), String(""), String(s)}, nil
	}, 0, "rpartition(sep) -> (before, sep, after) searching from the right"))

	StringType.Dict.Set("splitlines", MustNewMethod("splitlines", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		s := string(self.(String))
		var keepends Object = False
		// keepends is accepted BY NAME as well as positionally, which is what
		// CPython does: pip calls "text.splitlines(keepends=False)" while
		// reading a distribution's entry_points.txt, and UnpackTuple accepts no
		// keyword at all, so "pip show" died with "splitlines() does not take
		// keyword arguments".
		if err := ParseTupleAndKeywords(args, kwargs, "|O:splitlines", []string{"keepends"}, &keepends); err != nil {
			return nil, err
		}
		keep, _ := MakeBool(keepends)
		keepEnds := keep == True
		var out []Object
		start := 0
		for i := 0; i < len(s); i++ {
			var width int
			switch s[i] {
			case '\n', '\x0b', '\x0c', 0x1c, 0x1d, 0x1e, 0x85:
				width = 1
			case '\r':
				width = 1
				if i+1 < len(s) && s[i+1] == '\n' {
					width = 2
				}
			default:
				// U+2028 and U+2029 are three bytes in UTF-8.
				if strings.HasPrefix(s[i:], " ") || strings.HasPrefix(s[i:], " ") {
					width = 3
				} else {
					continue
				}
			}
			if keepEnds {
				out = append(out, String(s[start:i+width]))
			} else {
				out = append(out, String(s[start:i]))
			}
			i += width - 1
			start = i + 1
		}
		if start < len(s) {
			out = append(out, String(s[start:]))
		}
		return NewListFromItems(out), nil
	}, 0, "splitlines(keepends=False) -> a list of the lines, breaking at line boundaries"))

	StringType.Dict.Set("casefold", MustNewMethod("casefold", func(self Object, args Tuple) (Object, error) {
		// Full case folding is lower() plus the extra folds.  lowerString is the
		// one that EXPANDS where Go cannot - Go's strings.ToLower maps rune to
		// rune, so it silently drops the combining dot that the dotted capital
		// I folds to.  The sharp s needs the extra pass because it is a fold
		// rather than a lower-case mapping.
		s := lowerString(string(self.(String)))
		if strings.ContainsRune(s, 'ß') {
			s = strings.ReplaceAll(s, "ß", "ss")
		}
		return String(s), nil
	}, 0, "casefold() -> a casefolded copy, for caseless matching"))

	StringType.Dict.Set("title", MustNewMethod("title", func(self Object, args Tuple) (Object, error) {
		// Each run of ALPHABETIC characters starts uppercase and the rest are
		// lowercase, so "they're bill's".title() is "They'Re Bill'S" - the
		// apostrophe is not alphabetic, so the letter after it is capitalised.
		s := string(self.(String))
		var out strings.Builder
		prevAlpha := false
		for _, r := range s {
			switch {
			case unicode.IsLetter(r):
				if prevAlpha {
					out.WriteRune(unicode.ToLower(r))
				} else {
					out.WriteRune(unicode.ToTitle(r))
				}
				prevAlpha = true
			default:
				out.WriteRune(r)
				prevAlpha = false
			}
		}
		return String(out.String()), nil
	}, 0, "title() -> a titlecased copy: each word starts uppercase, the rest lowercase"))

	// format(*args, **kwargs) - the format-string mini-language.
	//
	// A replacement field is "{name!conv:spec}": the name selects an argument
	// by position, by keyword, or by an attribute/index chain; the conversion
	// is s/r/a; the spec is handed to the value's own formatting.  Field
	// numbering is AUTOMATIC until the first explicit index, and using an
	// automatic field after an explicit one is an error - as CPython has it.
	StringType.Dict.Set("format", MustNewMethod("format", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return formatPython(string(self.(String)), args, kwargs)
	}, 0, "format(*args, **kwargs) -> a formatted copy, with {} fields replaced by their arguments"))

	StringType.Dict.Set("format_map", MustNewMethod("format_map", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		var mapping Object
		if err := UnpackTuple(args, kwargs, "format_map", 1, 1, &mapping); err != nil {
			return nil, err
		}
		// Every field must be a NAME; a positional field is an error.
		return formatPythonMapping(string(self.(String)), mapping)
	}, 0, "format_map(mapping) -> like format(**mapping), taking one mapping"))

	StringType.Dict.Set("capitalize", MustNewMethod("capitalize", func(self Object, args Tuple) (Object, error) {
		// The first character to TITLE case and EVERY OTHER character to
		// lowercase, so "hELLO".capitalize() is "Hello".
		s := string(self.(String))
		if s == "" {
			return String(""), nil
		}
		r := []rune(s)
		return String(string(unicode.ToTitle(r[0])) + strings.ToLower(string(r[1:]))), nil
	}, 0, "capitalize() -> the first character titlecased and the rest lowercased"))

	StringType.Dict.Set("swapcase", MustNewMethod("swapcase", func(self Object, args Tuple) (Object, error) {
		s := string(self.(String))
		var out strings.Builder
		for _, r := range s {
			switch {
			case unicode.IsUpper(r):
				out.WriteRune(unicode.ToLower(r))
			case unicode.IsLower(r):
				out.WriteRune(unicode.ToUpper(r))
			default:
				out.WriteRune(r)
			}
		}
		return String(out.String()), nil
	}, 0, "swapcase() -> lowercase characters uppercased and vice versa"))

	// isidentifier() says whether the string is usable as an identifier, which
	// is what a library checks before taking a name from its caller - click
	// does exactly that when it parses a command's declarations.
	StringType.Dict.Set("isidentifier", MustNewMethod("isidentifier", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		s := string(self.(String))
		if s == "" {
			return False, nil
		}
		for i, r := range s {
			if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
				// A digit is allowed anywhere but the first character.
				if i == 0 && unicode.IsDigit(r) {
					return False, nil
				}
				continue
			}
			return False, nil
		}
		return True, nil
	}, 0, `isidentifier() -> bool

Return True if the string is a valid Python identifier, False otherwise.`))

	// The str.is*() family.  Each answers for the WHOLE string and needs at
	// least one character, so an empty string is False for every one of them.
	// These are what a library reaches for when it takes a name or a token
	// from its caller; click checks both when it parses declarations.
	isPred := func(name string, doc string, pred func(rune) bool) {
		StringType.Dict.Set(name, MustNewMethod(name, func(self Object, args Tuple, kwargs StringDict) (Object, error) {
			s := string(self.(String))
			if s == "" {
				return False, nil
			}
			for _, r := range s {
				if !pred(r) {
					return False, nil
				}
			}
			return True, nil
		}, 0, doc))
	}
	isPred("isalpha", "isalpha() -> bool\n\nReturn True if all characters are alphabetic and there is at least one.", unicode.IsLetter)
	isPred("isdigit", "isdigit() -> bool\n\nReturn True if all characters are digits and there is at least one.", func(r rune) bool { return r >= '0' && r <= '9' })
	isPred("isnumeric", "isnumeric() -> bool\n\nReturn True if all characters are numeric and there is at least one.", func(r rune) bool { return unicode.IsNumber(r) })
	isPred("isdecimal", "isdecimal() -> bool\n\nReturn True if all characters are decimal digits and there is at least one.", func(r rune) bool { return unicode.IsDigit(r) })
	isPred("isalnum", "isalnum() -> bool\n\nReturn True if all characters are alphanumeric and there is at least one.", func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsNumber(r) })
	isPred("isspace", "isspace() -> bool\n\nReturn True if all characters are whitespace and there is at least one.", unicode.IsSpace)
	isPred("isprintable", "isprintable() -> bool\n\nReturn True if all characters are printable and there is at least one.", func(r rune) bool { return unicode.IsPrint(r) || r == ' ' })
	isPred("isascii", "isascii() -> bool\n\nReturn True if all characters are ASCII.", func(r rune) bool { return r < 128 })

	// The case tests require at least one cased character, so "123" is neither
	// upper nor lower - the same rule CPython applies.
	casePred := func(name string, doc string, upper bool) {
		StringType.Dict.Set(name, MustNewMethod(name, func(self Object, args Tuple, kwargs StringDict) (Object, error) {
			s := string(self.(String))
			cased := false
			for _, r := range s {
				if unicode.IsUpper(r) {
					if !upper {
						return False, nil
					}
					cased = true
				} else if unicode.IsLower(r) {
					if upper {
						return False, nil
					}
					cased = true
				}
			}
			return Bool(cased), nil
		}, 0, doc))
	}
	casePred("isupper", "isupper() -> bool\n\nReturn True if all cased characters are uppercase and there is at least one.", true)
	casePred("islower", "islower() -> bool\n\nReturn True if all cased characters are lowercase and there is at least one.", false)

	// istitle() wants each run of letters to start uppercase and every other
	// letter lowercase, with at least one cased character overall.
	StringType.Dict.Set("istitle", MustNewMethod("istitle", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		s := string(self.(String))
		cased := false
		prevCased := false
		for _, r := range s {
			switch {
			case unicode.IsUpper(r):
				if prevCased {
					return False, nil
				}
				cased, prevCased = true, true
			case unicode.IsLower(r):
				if !prevCased {
					return False, nil
				}
				cased, prevCased = true, true
			default:
				prevCased = false
			}
		}
		return Bool(cased), nil
	}, 0, "istitle() -> bool\n\nReturn True if the string is titlecased and there is at least one cased character."))
}

// Type of this object
func (s String) Type() *Type {
	return StringType
}

// StrNew
func StrNew(metatype *Type, args Tuple, kwargs StringDict) (Object, error) {
	var (
		sObj     Object = String("")
		encoding Object
		errors   Object
	)
	// FIXME ignoring encoding and errors
	err := ParseTupleAndKeywords(args, kwargs, "|OOO:str", []string{"bytes_or_buffer", "encoding", "errors"}, &sObj, &encoding, &errors)
	if err != nil {
		return nil, err
	}
	// FIXME ignoring encoding
	// FIXME ignoring buffer protocol
	return Str(sObj)
}

// Intern s possibly returning a reference to an already interned string
func (s String) Intern() String {
	// fmt.Printf("FIXME interning %q\n", s)
	return s
}

func (a String) M__str__() (Object, error) {
	return a, nil
}

func (a String) M__repr__() (Object, error) {
	out := StringEscape(a, false)
	return String(out), nil
}

func (s String) M__bool__() (Object, error) {
	return NewBool(len(s) > 0), nil
}

// len returns length of the string in unicode characters
func (s String) len() int {
	return utf8.RuneCountInString(string(s))
}

func (s String) M__len__() (Object, error) {
	return Int(s.len()), nil
}

func (a String) M__add__(other Object) (Object, error) {
	if b, ok := other.(String); ok {
		return a + b, nil
	}
	return NotImplemented, nil
}

func (a String) M__radd__(other Object) (Object, error) {
	if b, ok := other.(String); ok {
		return b + a, nil
	}
	return NotImplemented, nil
}

func (a String) M__iadd__(other Object) (Object, error) {
	return a.M__add__(other)
}

func (a String) M__mul__(other Object) (Object, error) {
	if b, ok := convertToInt(other); ok {
		if b < 0 {
			b = 0
		}
		var out bytes.Buffer
		for i := 0; i < int(b); i++ {
			out.WriteString(string(a))
		}
		return String(out.String()), nil
	}
	return NotImplemented, nil
}

func (a String) M__rmul__(other Object) (Object, error) {
	return a.M__mul__(other)
}

func (a String) M__imul__(other Object) (Object, error) {
	return a.M__mul__(other)
}

// Convert an Object to an String
//
// Returns ok as to whether the conversion worked or not
func convertToString(other Object) (String, bool) {
	switch b := other.(type) {
	case String:
		return b, true
	}
	return "", false
}

// Rich comparison

func (a String) M__lt__(other Object) (Object, error) {
	if b, ok := convertToString(other); ok {
		return NewBool(a < b), nil
	}
	return NotImplemented, nil
}

func (a String) M__le__(other Object) (Object, error) {
	if b, ok := convertToString(other); ok {
		return NewBool(a <= b), nil
	}
	return NotImplemented, nil
}

func (a String) M__eq__(other Object) (Object, error) {
	if b, ok := convertToString(other); ok {
		return NewBool(a == b), nil
	}
	return NotImplemented, nil
}

func (a String) M__ne__(other Object) (Object, error) {
	if b, ok := convertToString(other); ok {
		return NewBool(a != b), nil
	}
	return NotImplemented, nil
}

func (a String) M__gt__(other Object) (Object, error) {
	if b, ok := convertToString(other); ok {
		return NewBool(a > b), nil
	}
	return NotImplemented, nil
}

func (a String) M__ge__(other Object) (Object, error) {
	if b, ok := convertToString(other); ok {
		return NewBool(a >= b), nil
	}
	return NotImplemented, nil
}

// % operator

/*
4.7.2. printf-style String Formatting

Note The formatting operations described here exhibit a variety of
quirks that lead to a number of common errors (such as failing to
display tuples and dictionaries correctly). Using the newer
str.format() interface helps avoid these errors, and also provides a
generally more powerful, flexible and extensible approach to
formatting text.

String objects have one unique built-in operation: the % operator
(modulo). This is also known as the string formatting or interpolation
operator. Given format % values (where format is a string), %
conversion specifications in format are replaced with zero or more
elements of values. The effect is similar to using the sprintf() in
the C language.

If format requires a single argument, values may be a single non-tuple
object. [5] Otherwise, values must be a tuple with exactly the number
of items specified by the format string, or a single mapping object
(for example, a dictionary).

A conversion specifier contains two or more characters and has the
following components, which must occur in this order:

The '%' character, which marks the start of the specifier.

Mapping key (optional), consisting of a parenthesised sequence of
characters (for example, (somename)).

Conversion flags (optional), which affect the result of some
conversion types.

Minimum field width (optional). If specified as an '*' (asterisk), the
actual width is read from the next element of the tuple in values, and
the object to convert comes after the minimum field width and optional
precision.

Precision (optional), given as a '.' (dot) followed by the
precision. If specified as '*' (an asterisk), the actual precision is
read from the next element of the tuple in values, and the value to
convert comes after the precision.

Length modifier (optional).

Conversion type.

When the right argument is a dictionary (or other mapping type), then
the formats in the string must include a parenthesised mapping key
into that dictionary inserted immediately after the '%' character. The
mapping key selects the value to be formatted from the mapping. For
example:

>>>
>>> print('%(language)s has %(number)03d quote types.' %
...       {'language': "Python", "number": 2})
Python has 002 quote types.

In this case no * specifiers may occur in a format (since they require
a sequential parameter list).

The conversion flag characters are:

Flag	Meaning
'#'	The value conversion will use the “alternate form” (where defined below).
'0'	The conversion will be zero padded for numeric values.
'-'	The converted value is left adjusted (overrides the '0' conversion if both are given).
' '	(a space) A blank should be left before a positive number (or empty string) produced by a signed conversion.
'+'	A sign character ('+' or '-') will precede the conversion (overrides a “space” flag).

A length modifier (h, l, or L) may be present, but is ignored as it is
not necessary for Python – so e.g. %ld is identical to %d.

The conversion types are:

Conversion	Meaning	Notes
'd'	Signed integer decimal.
'i'	Signed integer decimal.
'o'	Signed octal value.	(1)
'u'	Obsolete type – it is identical to 'd'.	(7)
'x'	Signed hexadecimal (lowercase).	(2)
'X'	Signed hexadecimal (uppercase).	(2)
'e'	Floating point exponential format (lowercase).	(3)
'E'	Floating point exponential format (uppercase).	(3)
'f'	Floating point decimal format.	(3)
'F'	Floating point decimal format.	(3)
'g'	Floating point format. Uses lowercase exponential format if exponent is less than -4 or not less than precision, decimal format otherwise.	(4)
'G'	Floating point format. Uses uppercase exponential format if exponent is less than -4 or not less than precision, decimal format otherwise.	(4)
'c'	Single character (accepts integer or single character string).
'r'	String (converts any Python object using repr()).	(5)
's'	String (converts any Python object using str()).	(5)
'a'	String (converts any Python object using ascii()).	(5)
'%'	No argument is converted, results in a '%' character in the result.
Notes:

The alternate form causes a leading zero ('0') to be inserted between
left-hand padding and the formatting of the number if the leading
character of the result is not already a zero.

The alternate form causes a leading '0x' or '0X' (depending on whether
the 'x' or 'X' format was used) to be inserted between left-hand
padding and the formatting of the number if the leading character of
the result is not already a zero.

The alternate form causes the result to always contain a decimal
point, even if no digits follow it.

The precision determines the number of digits after the decimal point
and defaults to 6.

The alternate form causes the result to always contain a decimal
point, and trailing zeroes are not removed as they would otherwise be.

The precision determines the number of significant digits before and
after the decimal point and defaults to 6.

If precision is N, the output is truncated to N characters.

See PEP 237.  Since Python strings have an explicit length, %s
conversions do not assume that '\0' is the end of the string.

Changed in version 3.1: %f conversions for numbers whose absolute
value is over 1e50 are no longer replaced by %g conversions.
*/
func (a String) M__mod__(other Object) (Object, error) {
	// A tuple IS the argument list, always: "%s" % (1, 2) leaves an argument
	// unused and raises, which is why a one-element tuple has to be written
	// "((1, 2),)" to format the tuple itself.
	values := Tuple{other}
	if t, ok := other.(Tuple); ok {
		values = t
	}

	var out strings.Builder
	format := string(a)
	valueIdx := 0
	usedMapping := false
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			out.WriteByte(format[i])
			continue
		}
		i++
		if i >= len(format) {
			return nil, ExceptionNewf(ValueError, "incomplete format")
		}
		if format[i] == '%' {
			out.WriteByte('%')
			continue
		}

		// Collect the flags, width and precision, then the conversion.
		// A mapping key, "%(name)s", draws its value from a dict argument
		// instead of taking the next positional one.
		var mappingKey string
		var hasMappingKey bool
		if format[i] == '(' {
			// Scan to the matching ')' with no nesting; a key cannot contain
			// an unescaped ')'.
			j := i + 1
			for j < len(format) && format[j] != ')' {
				j++
			}
			if j >= len(format) {
				return nil, ExceptionNewf(ValueError, "incomplete format key")
			}
			mappingKey = format[i+1 : j]
			hasMappingKey = true
			i = j + 1
		}
		flagsStart := i
		for i < len(format) && strings.ContainsRune("-+ #0", rune(format[i])) {
			i++
		}
		flags := format[flagsStart:i]

		// A '*' means the width - or the precision - is taken from the next
		// argument instead of the format string: "%*s" % (4, "hi").  It is
		// what pip's option formatter writes, and without it "pip --help"
		// died with 'unsupported format character \'*\''.
		//
		// The argument is consumed here, in the order it appears, and its value
		// is written back as literal digits so that everything downstream -
		// specWidth, specPrecision, the padding - is unchanged.  A NEGATIVE
		// width means left-justify with the absolute value, so a '-' joins the
		// flags; a negative PRECISION means no precision at all, which is
		// CPython's rule and is not symmetric with width.
		width := ""
		if i < len(format) && format[i] == '*' {
			i++
			w, err := starArg(values, &valueIdx, hasMappingKey)
			if err != nil {
				return nil, err
			}
			if w < 0 {
				w = -w
				flags += "-"
			}
			width = strconv.Itoa(w)
		} else {
			widthStart := i
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
			width = format[widthStart:i]
		}
		precision := ""
		precisionAfterDot := false
		if i < len(format) && format[i] == '.' {
			precisionAfterDot = true
			dotStart := i
			i++
			if i < len(format) && format[i] == '*' {
				i++
				p, err := starArg(values, &valueIdx, hasMappingKey)
				if err != nil {
					return nil, err
				}
				if p < 0 {
					// A negative PRECISION is precision ZERO, not "no
					// precision": "%.*f" % (-2, 3.14159) is "3",
					// whereas leaving the precision off gives 3.141590.
					precision = ".0"
				} else {
					precision = "." + strconv.Itoa(p)
				}
			} else {
				for i < len(format) && format[i] >= '0' && format[i] <= '9' {
					i++
				}
				// The dot is kept even with no digits after it: "%.f" is
				// precision ZERO, which is not the same as no precision.
				precision = format[dotStart:i]
			}
		}
		if i >= len(format) {
			return nil, ExceptionNewf(ValueError, "incomplete format")
		}
		verb := format[i]
		// Rebuild the specifier from its parts, so a star-derived width or
		// precision reads exactly as if it had been written literally.
		spec := "%" + flags + width + precision + string(verb)

		var value Object
		if hasMappingKey {
			// A mapping argument: the value is looked up by key.  Missing keys
			// are a KeyError, as in CPython.
			m, ok := other.(StringDict)
			if !ok {
				return nil, ExceptionNewf(TypeError, "format requires a mapping")
			}
			v, ok := m.Get(mappingKey)
			if !ok {
				return nil, ExceptionNewf(KeyError, "'%s'", mappingKey)
			}
			value = v
			usedMapping = true
		} else {
			if valueIdx >= len(values) {
				return nil, ExceptionNewf(TypeError, "not enough arguments for format string")
			}
			value = values[valueIdx]
			valueIdx++
		}

		// The text of a value comes from Python's own str/repr, not from Go's
		// fmt: "%s" % [1, 2] used to print "&{[1 2]}".
		var text string
		var err error
		switch verb {
		case 's':
			if text, err = StrAsString(value); err != nil {
				return nil, err
			}
		case 'r':
			if text, err = ReprAsString(value); err != nil {
				return nil, err
			}
		case 'a':
			var r string
			if r, err = ReprAsString(value); err != nil {
				return nil, err
			}
			text = StringEscape(String(r), true)
		case 'd', 'i', 'u', 'x', 'X', 'o':
			n, err := MakeGoInt(value)
			if err != nil {
				return nil, err
			}
			if verb == 'u' {
				text = strconv.FormatInt(int64(n), 10)
			} else {
				text = strconv.FormatInt(int64(n), intBaseOf(verb))
			}
			if verb == 'X' {
				text = strings.ToUpper(text)
			}
		case 'c':
			if s, ok := value.(String); ok {
				text = string(s)
			} else {
				n, err := MakeGoInt(value)
				if err != nil {
					return nil, err
				}
				text = string(rune(n))
			}
		case 'e', 'E', 'f', 'F', 'g', 'G':
			f, err := FloatAsFloat64(value)
			if err != nil {
				return nil, err
			}
			prec := 6
			if precisionAfterDot {
				prec = specPrecision(spec)
			}
			text = strconv.FormatFloat(f, verb, prec, 64)
		default:
			return nil, ExceptionNewf(ValueError, "unsupported format character '%c'", verb)
		}

		// Precision TRUNCATES a string: "%.2s" % "abcdef" is "ab" and
		// "%.0s" is "".  It was applied only to the float verbs, so the
		// precision in an ordinary "%.2s" was silently ignored and the whole
		// string came through - measured against CPython, where %.0s gives
		// "" and %.2s gives "ab".
		if precisionAfterDot {
			switch verb {
			case 's', 'r', 'a':
				if prec := specPrecision(spec); prec < len(text) {
					text = text[:prec]
				}
			}
		}

		// Width, the '-' flag and the '0' flag apply to the finished text for
		// every conversion, so they are handled once here rather than per
		// verb.  '0' pads with zeros AFTER any sign, which is what makes
		// "%05.1f" % 3.14159 give "003.1" and not "- 03.1" for a negative.
		//
		// The flags must be read from the FLAG POSITION, not from anywhere in
		// the spec: "strings.ContainsRune(spec, '0')" also matches the 0 in a
		// width like %10.3f and in a precision like %.30f, so ordinary padding
		// came out as "000000.060" instead of "     0.060" and "%10s" rendered
		// "00000000ab" instead of "        ab".
		// The SIGN and ALTERNATE flags apply to the finished text too, and must
		// be added BEFORE the width is measured, or the sign eats into the
		// padding: "%+5d" % 42 is "  +42", not "   +42" or "+  42".
		//
		// They were read from the spec and then never used for the integer and
		// float verbs, so "%+d" % 42 gave "42" and "%#x" % 255 gave "ff".
		flags = specFlags(spec)
		switch {
		case strings.ContainsRune(flags, '+') && !strings.HasPrefix(text, "-") && !strings.HasPrefix(text, "+"):
			text = "+" + text
		case strings.ContainsRune(flags, ' ') && !strings.HasPrefix(text, "-") && !strings.HasPrefix(text, "+"):
			text = " " + text
		}
		if strings.ContainsRune(flags, '#') && !strings.HasPrefix(text, "0x") && !strings.HasPrefix(text, "0o") &&
			(verb == 'x' || verb == 'X' || verb == 'o') {
			prefix := "0"
			if verb == 'x' {
				prefix = "0x"
			} else if verb == 'X' {
				prefix = "0X"
			}
			text = prefix + text
		}
		if width := specWidth(spec); width > len([]rune(text)) {
			padLen := width - len([]rune(text))
			leftAlign := strings.ContainsRune(flags, '-')
			zeroPad := strings.ContainsRune(flags, '0') && !leftAlign
			switch {
			case leftAlign:
				text += strings.Repeat(" ", padLen)
			case zeroPad:
				sign := ""
				if len(text) > 0 && (text[0] == '-' || text[0] == '+') {
					sign, text = text[:1], text[1:]
				}
				text = sign + strings.Repeat("0", padLen) + text
			default:
				text = strings.Repeat(" ", padLen) + text
			}
		}
		out.WriteString(text)
	}

	if !usedMapping && valueIdx < len(values) {
		return nil, ExceptionNewf(TypeError, "not all arguments converted during string formatting")
	}
	return String(out.String()), nil
}

// starArg consumes the next positional argument as the width or precision of a
// "*" in a conversion specifier, and returns it.
//
// A "*" cannot be combined with a mapping key in CPython ("%*s" % {"a": 1} is a
// TypeError), and the value must be an integer - a float is refused rather than
// truncated.
func starArg(values Tuple, valueIdx *int, hasMappingKey bool) (int, error) {
	if hasMappingKey {
		return 0, ExceptionNewf(ValueError, "* wants int, not a mapping")
	}
	if *valueIdx >= len(values) {
		return 0, ExceptionNewf(TypeError, "not enough arguments for format string")
	}
	v := values[*valueIdx]
	*valueIdx++
	n, err := MakeGoInt(v)
	if err != nil {
		return 0, ExceptionNewf(TypeError, "* wants int, not %s", v.Type().Name)
	}
	return n, nil
}

// specWidth reads the width out of a conversion specifier such as "%-8.3f".
// specFlags returns only the FLAG characters of a conversion specifier -
// the run of "-+ #0" immediately after the '%' and before any width.  Reading
// flags from the whole spec confuses the '0' flag with a digit of the width or
// the precision.
func specFlags(spec string) string {
	// spec is like "%-+10.3f": skip the leading '%', take flags, stop at the
	// first character that is not one.
	start := 1
	for start < len(spec) && strings.ContainsRune("-+ #0", rune(spec[start])) {
		start++
	}
	return spec[1:start]
}

func specWidth(spec string) int {
	digits := ""
	for i := 1; i < len(spec)-1; i++ {
		c := spec[i]
		if c >= '0' && c <= '9' {
			digits += string(c)
		} else if c == '.' {
			break
		} else {
			digits = ""
		}
	}
	if digits == "" {
		return 0
	}
	n, _ := strconv.Atoi(digits)
	return n
}

// specPrecision reads the precision out of a conversion specifier.
func specPrecision(spec string) int {
	dot := strings.IndexByte(spec, '.')
	if dot < 0 {
		return -1
	}
	digits := ""
	for i := dot + 1; i < len(spec)-1; i++ {
		if spec[i] >= '0' && spec[i] <= '9' {
			digits += string(spec[i])
		} else {
			break
		}
	}
	if digits == "" {
		return 0
	}
	n, _ := strconv.Atoi(digits)
	return n
}

// intBaseOf maps a conversion verb to the base strconv wants.
func intBaseOf(verb byte) int {
	switch verb {
	case 'x', 'X':
		return 16
	case 'o':
		return 8
	}
	return 10
}

func (a String) M__rmod__(other Object) (Object, error) {
	switch b := other.(type) {
	case String:
		return b.M__mod__(a)
	}
	return NotImplemented, nil
}

func (a String) M__imod__(other Object) (Object, error) {
	return a.M__mod__(other)
}

// Returns position in string of n-th character
//
// returns end of string if not found
func (s String) pos(n int) int {
	characterNumber := 0
	for i := range s {
		if characterNumber == n {
			return i
		}
		characterNumber++
	}
	return len(s)
}

// slice returns the slice of this string using character positions
//
// length should be the length of the string in unicode characters
func (s String) slice(start, stop, length int) String {
	if start >= stop {
		return String("")
	}
	if length == len(s) {
		return s[start:stop] // ascii only
	}
	if start <= 0 && stop >= length {
		return s
	}
	startI := s.pos(start)
	stopI := s[startI:].pos(stop-start) + startI
	return s[startI:stopI]
}

func (s String) M__getitem__(key Object) (Object, error) {
	length := s.len()
	asciiOnly := length == len(s)
	if slice, ok := key.(*Slice); ok {
		start, stop, step, slicelength, err := slice.GetIndices(length)
		if err != nil {
			return nil, err
		}
		if step == 1 {
			// Return a subslice since strings are immutable
			return s.slice(start, stop, length), nil
		}
		if asciiOnly {
			newString := make([]byte, slicelength)
			for i, j := start, 0; j < slicelength; i, j = i+step, j+1 {
				newString[j] = s[i]
			}
			return String(newString), nil
		}
		// Unpack the string into a []rune to do this for speed
		runeString := []rune(string(s))
		newString := make([]rune, slicelength)
		for i, j := start, 0; j < slicelength; i, j = i+step, j+1 {
			newString[j] = runeString[i]
		}
		return String(newString), nil
	}
	i, err := IndexIntCheckNamed(key, length, "string")
	if err != nil {
		return nil, err
	}
	if asciiOnly {
		return s[i : i+1], nil
	}
	s = s[s.pos(i):]
	_, runeSize := utf8.DecodeRuneInString(string(s))
	return s[:runeSize], nil
}

func (s String) M__contains__(item Object) (Object, error) {
	needle, ok := item.(String)
	if !ok {
		return nil, ExceptionNewf(TypeError, "'in <string>' requires string as left operand, not %s", item.Type().Name)
	}
	return NewBool(strings.Contains(string(s), string(needle))), nil
}

// adjustIndices clamps a (start, end) pair to a string of the given length the
// way CPython's ADJUST_INDICES does: a NEGATIVE index counts back from the end,
// and either may land outside the string.
//
// Both count and find used to clamp only the upper bound, so a negative start
// went straight into a Go slice and panicked - not raised, PANICKED, taking the
// whole host process down with "slice bounds out of range".
// "abc".count("b", -1) was enough to do it.
func adjustIndices(beg, end, size int) (int, int) {
	if beg < 0 {
		beg += size
		if beg < 0 {
			beg = 0
		}
	}
	if end < 0 {
		end += size
		if end < 0 {
			end = 0
		}
	}
	if end > size {
		end = size
	}
	if beg > end {
		// CPython treats a start past the end as an empty range, not an error.
		beg = end
	}
	return beg, end
}

// strStartsEndsWith implements startswith and endswith, which differ only in
// which end they test.
//
// Both arguments used to be handled badly: end was IGNORED entirely, so
// "abc".endswith("b", 0, 2) answered about the whole string and returned False
// where CPython says True, and a negative start went into a Go slice
// unadjusted and PANICKED - "abc".startswith("b", -2) killed the host process.
// The (start, end) pair now goes through the same adjustment count and find
// use.
func strStartsEndsWith(self String, args Tuple, atStart bool) (Object, error) {
	name := "endswith"
	if atStart {
		name = "startswith"
	}
	if len(args) == 0 {
		return nil, ExceptionNewf(TypeError, "%s() takes at least 1 argument (0 given)", name)
	}
	var prefixes []string
	switch v := args[0].(type) {
	case String:
		prefixes = append(prefixes, string(v))
	case Tuple:
		for _, t := range v {
			s, ok := t.(String)
			if !ok {
				return nil, ExceptionNewf(TypeError, "%s first arg must be str or a tuple of str, not %s", name, t.Type().Name)
			}
			prefixes = append(prefixes, string(s))
		}
	default:
		return nil, ExceptionNewf(TypeError, "%s first arg must be str or a tuple of str, not %s", name, args[0].Type().Name)
	}

	beg, end := 0, self.len()
	if len(args) > 1 {
		b, ok := args[1].(Int)
		if !ok {
			return nil, ExceptionNewf(TypeError, "%s indices must be integers, not %s", name, args[1].Type().Name)
		}
		beg = int(b)
	}
	if len(args) > 2 {
		e, ok := args[2].(Int)
		if !ok {
			return nil, ExceptionNewf(TypeError, "%s indices must be integers, not %s", name, args[2].Type().Name)
		}
		end = int(e)
	}
	// CPython checks this BEFORE clamping: a start that is not a valid index into
	// the string fails against a prefix of any length, even the empty one.
	// "abc".startswith("", 3) is True but "abc".startswith("", 4) is False,
	// which no amount of clamping a window reproduces.  The check has to come
	// BEFORE adjustIndices, which would otherwise pull beg back to the end.
	if beg > self.len() {
		return False, nil
	}
	beg, end = adjustIndices(beg, end, self.len())
	// The comparison happens on the SLICE, which is what makes the end argument
	// mean anything.
	window := string(self.slice(beg, end, self.len()))
	for _, p := range prefixes {
		if atStart {
			if strings.HasPrefix(window, p) {
				return True, nil
			}
		} else if strings.HasSuffix(window, p) {
			return True, nil
		}
	}
	return False, nil
}

func (s String) Count(args Tuple) (Object, error) {
	var (
		pysub Object
		pybeg Object = Int(0)
		pyend Object = Int(s.len())
		pyfmt        = "s|ii:count"
	)
	err := ParseTuple(args, pyfmt, &pysub, &pybeg, &pyend)
	if err != nil {
		return nil, err
	}

	var (
		beg  = int(pybeg.(Int))
		end  = int(pyend.(Int))
		size = s.len()
	)
	beg, end = adjustIndices(beg, end, size)

	var (
		str = string(s.slice(beg, end, s.len()))
		sub = string(pysub.(String))
	)
	return Int(strings.Count(str, sub)), nil
}

func (s String) find(args Tuple) (Object, error) {
	var (
		pysub Object
		pybeg Object = Int(0)
		pyend Object = Int(s.len())
		pyfmt        = "s|ii:find"
	)
	err := ParseTuple(args, pyfmt, &pysub, &pybeg, &pyend)
	if err != nil {
		return nil, err
	}

	var (
		beg  = int(pybeg.(Int))
		end  = int(pyend.(Int))
		size = s.len()
	)
	beg, end = adjustIndices(beg, end, size)

	var (
		off = s.slice(0, beg, s.len()).len()
		str = string(s.slice(beg, end, s.len()))
		sub = string(pysub.(String))
		idx = strings.Index(str, sub)
	)
	if idx < 0 {
		return Int(idx), nil
	}
	return Int(off + String(str[:idx]).len()), nil
}

func (s String) Split(args Tuple, kwargs StringDict) (Object, error) {
	var (
		pyval Object = None
		pymax Object = Int(-2)
		pyfmt        = "|Oi:split"
		kwlst        = []string{"sep", "maxsplit"}
	)
	err := ParseTupleAndKeywords(args, kwargs, pyfmt, kwlst, &pyval, &pymax)
	if err != nil {
		return nil, err
	}

	var (
		max = pymax.(Int)
		vs  []string
	)
	switch v := pyval.(type) {
	case String:
		vs = strings.SplitN(string(s), string(v), int(max)+1)
	case NoneType:
		vs = fieldsN(string(s), int(max))
	default:
		return nil, ExceptionNewf(TypeError, "Can't convert '%s' object to str implicitly", pyval.Type())
	}
	o := List{}
	for _, j := range vs {
		o.Items = append(o.Items, String(j))
	}
	return &o, nil
}

// RSplit is split() counting from the RIGHT: at most maxsplit splits are made,
// and the unsplit remainder is the FIRST element of the result.
//
// For maxsplit of -1 (the default) the two are the same, which is why the plain
// case is easy to get right and the limited case is not: "a,b,c".rsplit(",", 1)
// is ["a,b", "c"] where split(",", 1) is ["a", "b,c"].
func (s String) RSplit(args Tuple, kwargs StringDict) (Object, error) {
	var (
		pyval Object = None
		pymax Object = Int(-2)
	)
	if err := ParseTupleAndKeywords(args, kwargs, "|Oi:rsplit", []string{"sep", "maxsplit"}, &pyval, &pymax); err != nil {
		return nil, err
	}
	max := int(pymax.(Int))
	var vs []string
	switch v := pyval.(type) {
	case String:
		sep := string(v)
		if sep == "" {
			return nil, ExceptionNewf(ValueError, "empty separator")
		}
		if max < 0 {
			vs = strings.Split(string(s), sep)
			break
		}
		// Split from the right, at most max times.
		rest := string(s)
		var out []string
		for len(out) < max {
			i := strings.LastIndex(rest, sep)
			if i < 0 {
				break
			}
			out = append([]string{rest[i+len(sep):]}, out...)
			rest = rest[:i]
		}
		vs = append([]string{rest}, out...)
	case NoneType:
		// Whitespace mode with a limit: the LAST max fields are split off, and
		// the remainder is the ORIGINAL text before them, trimmed at its edges.
		// It is not re-normalised in the middle -
		// "a  b  c d".rsplit(None, 2) is ["a  b", "c", "d"], with the double
		// space intact - which is what makes this different from splitting the
		// field list.
		if max < 0 {
			vs = fieldsN(string(s), -1)
			break
		}
		text := string(s)
		var tail []string
		end := len(text)
		for len(tail) < max {
			// Find the end of a field: skip trailing whitespace, then read
			// back to the run's start.
			for end > 0 && unicode.IsSpace(rune(text[end-1])) {
				end--
			}
			if end == 0 {
				break
			}
			start := end
			for start > 0 && !unicode.IsSpace(rune(text[start-1])) {
				start--
			}
			tail = append([]string{text[start:end]}, tail...)
			end = start
		}
		head := strings.TrimSpace(text[:end])
		if head == "" {
			vs = tail
			break
		}
		vs = append([]string{head}, tail...)
	default:
		return nil, ExceptionNewf(TypeError, "Can't convert '%s' object to str implicitly", pyval.Type())
	}
	o := List{}
	for _, j := range vs {
		o.Items = append(o.Items, String(j))
	}
	return &o, nil
}

func (s String) Replace(args Tuple) (Object, error) {
	var (
		pyold Object = None
		pynew Object = None
		pycnt Object = Int(-1)
	)
	err := ParseTuple(args, "ss|i:replace", &pyold, &pynew, &pycnt)
	if err != nil {
		return nil, err
	}

	var (
		old = string(pyold.(String))
		new = string(pynew.(String))
		cnt = int(pycnt.(Int))
	)

	return String(strings.Replace(string(s), old, new, cnt)), nil
}

func stripFunc(args Tuple) (func(rune) bool, error) {
	var (
		pyval Object = None
	)
	err := ParseTuple(args, "|s", &pyval)
	if err != nil {
		return nil, err
	}
	f := unicode.IsSpace
	switch v := pyval.(type) {
	case String:
		chars := []rune(string(v))
		f = func(s rune) bool {
			for _, i := range chars {
				if s == i {
					return true
				}
			}
			return false
		}
	}
	return f, nil
}

func (s String) Strip(args Tuple) (Object, error) {
	f, err := stripFunc(args)
	if err != nil {
		return nil, err
	}
	return String(strings.TrimFunc(string(s), f)), nil
}

func (s String) LStrip(args Tuple) (Object, error) {
	f, err := stripFunc(args)
	if err != nil {
		return nil, err
	}
	return String(strings.TrimLeftFunc(string(s), f)), nil
}

func (s String) RStrip(args Tuple) (Object, error) {
	f, err := stripFunc(args)
	if err != nil {
		return nil, err
	}
	return String(strings.TrimRightFunc(string(s), f)), nil
}

func (s String) Upper() (Object, error) {
	return String(upperString(string(s))), nil
}

func (s String) Lower() (Object, error) {
	return String(lowerString(string(s))), nil
}

func (s String) Join(args Tuple) (Object, error) {
	if len(args) != 1 {
		return nil, ExceptionNewf(TypeError, "join() takes exactly one argument (%d given)", len(args))
	}
	var parts []string
	iterable, err := Iter(args[0])
	if err != nil {
		return nil, err
	}
	item, err := Next(iterable)
	for err == nil {
		str, ok := item.(String)
		if !ok {
			return nil, ExceptionNewf(TypeError, "sequence item %d: expected str instance, %s found", len(parts), item.Type().Name)
		}
		parts = append(parts, string(str))
		item, err = Next(iterable)
	}
	if err != StopIteration {
		return nil, err
	}
	return String(strings.Join(parts, string(s))), nil
}

// Check stringerface is satisfied
var (
	_ richComparison     = String("")
	_ sequenceArithmetic = String("")
	_ I__mod__           = String("")
	_ I__rmod__          = String("")
	_ I__imod__          = String("")
	_ I__len__           = String("")
	_ I__bool__          = String("")
	_ I__getitem__       = String("")
	_ I__contains__      = String("")
)

// formatPython implements str.format(*args, **kwargs).
//
// The parser walks the format string, copying literal text and replacing each
// {field} with the formatted argument.  "{{" and "}}" are the escapes.
func formatPython(format string, args Tuple, kwargs StringDict) (Object, error) {
	autoNum := -1
	manual := false
	var out strings.Builder
	runes := []rune(format)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '{':
			if i+1 < len(runes) && runes[i+1] == '{' {
				out.WriteRune('{')
				i++
				continue
			}
			// Find the matching '}', skipping a nested spec's own braces.
			depth, j := 1, i+1
			for ; j < len(runes) && depth > 0; j++ {
				switch runes[j] {
				case '{':
					depth++
				case '}':
					depth--
				}
			}
			if depth != 0 {
				return nil, ExceptionNewf(ValueError, "Single '{' encountered in format string")
			}
			body := string(runes[i+1 : j-1])
			text, err := formatField(body, args, kwargs, &autoNum, &manual)
			if err != nil {
				return nil, err
			}
			out.WriteString(text)
			i = j - 1
		case '}':
			if i+1 < len(runes) && runes[i+1] == '}' {
				out.WriteRune('}')
				i++
				continue
			}
			return nil, ExceptionNewf(ValueError, "Single '}' encountered in format string")
		default:
			out.WriteRune(runes[i])
		}
	}
	return String(out.String()), nil
}

// formatPythonMapping implements str.format_map(mapping): every field must be
// a name looked up in the one mapping.
func formatPythonMapping(format string, mapping Object) (Object, error) {
	// A mapping is used by name, so it is turned into the keyword set that
	// the ordinary formatter expects.
	if d, ok := mapping.(StringDict); ok {
		return formatPython(format, nil, d)
	}
	// Anything else is asked for its items.
	it, err := Iter(mapping)
	if err != nil {
		return nil, err
	}
	items, ok := it.(I__next__)
	if !ok {
		return nil, ExceptionNewf(TypeError, "format_map() argument must be a mapping")
	}
	kw := NewStringDict()
	for {
		item, err := items.M__next__()
		if err != nil {
			if IsException(StopIteration, err) {
				break
			}
			return nil, err
		}
		pair, ok := item.(Tuple)
		if !ok || len(pair) != 2 {
			return nil, ExceptionNewf(ValueError, "format_map() mapping items must be pairs")
		}
		k, err := StrAsString(pair[0])
		if err != nil {
			return nil, err
		}
		kw.Set(k, pair[1])
	}
	return formatPython(format, nil, kw)
}

// formatField resolves and renders one {field}.
func formatField(body string, args Tuple, kwargs StringDict, autoNum *int, manual *bool) (string, error) {
	name, conv, spec := splitField(body)

	// The name may be a chain: "0.attr" or "0[key]" or "name.attr".
	var value Object
	if name == "" {
		if *manual {
			return "", ExceptionNewf(ValueError, "cannot switch from manual field specification to automatic field numbering")
		}
		*autoNum++
		if *autoNum >= len(args) {
			return "", ExceptionNewf(IndexError, "Replacement index %d out of range for positional args tuple", *autoNum)
		}
		value = args[*autoNum]
	} else if isDigits(name) {
		*manual = true
		n := 0
		for _, r := range name {
			n = n*10 + int(r-'0')
		}
		if n >= len(args) {
			return "", ExceptionNewf(IndexError, "Replacement index %d out of range for positional args tuple", n)
		}
		value = args[n]
	} else {
		*manual = true
		v, ok := kwargs.Get(name)
		if !ok {
			return "", ExceptionNewf(KeyError, "'%s'", name)
		}
		value = v
	}

	// Apply the conversion, then the spec.
	switch conv {
	case 'r':
		rep, err := Repr(value)
		if err != nil {
			return "", err
		}
		value = rep
	case 's':
		if rep, err := Str(value); err == nil {
			value = rep
		}
	case 'a':
		rep, err := ReprAsString(value)
		if err != nil {
			return "", err
		}
		value = String(StringEscape(String(rep), true))
	}

	// The value formats itself when it can, which is what honours a user
	// type's __format__; otherwise the shared mini-language does it.
	if f, ok := value.(I__format__); ok {
		res, err := f.M__format__(String(spec))
		if err != nil {
			return "", err
		}
		return StrAsString(res)
	}
	res, err := Format(value, spec)
	if err != nil {
		return "", err
	}
	return StrAsString(res)
}

// splitField splits "{name!conv:spec}" into its three parts.  The conversion
// and the spec are only recognised OUTSIDE any nested brackets, so
// "{0[1:2]}" keeps its slice.
func splitField(body string) (name string, conv rune, spec string) {
	depth := 0
	for i, r := range body {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case '!':
			if depth == 0 {
				name = body[:i]
				rest := body[i+1:]
				// The conversion is one character; anything after it before a
				// ':' is a syntax error, but tolerating it is kinder than
				// failing, and CPython requires the single character.
				for j, c := range rest {
					if c == ':' {
						spec = rest[j+1:]
						break
					}
					conv = c
				}
				return resolveChain(name), conv, spec
			}
		case ':':
			if depth == 0 {
				return resolveChain(body[:i]), 0, body[i+1:]
			}
		}
	}
	return resolveChain(body), 0, ""
}

// resolveChain handles "0.attr" and "0[key]" by keeping them for the caller;
// without attribute and index support in this interpreter the chain is
// returned whole, so a plain name or index still resolves.
func resolveChain(s string) string { return s }

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ZFill pads a numeric string with zeros on the left: "42".zfill(5) is "00042".
// A leading sign stays at the front, which is the whole point of the method and
// what makes it different from a plain left-pad.
func (s String) ZFill(args Tuple) (Object, error) {
	var pywidth Object
	if err := ParseTuple(args, "i:zfill", &pywidth); err != nil {
		return nil, err
	}
	width := int(pywidth.(Int))
	if width <= s.len() {
		return s, nil
	}
	pad := width - s.len()
	text := string(s)
	if len(text) > 0 && (text[0] == '-' || text[0] == '+') {
		return String(text[:1] + strings.Repeat("0", pad) + text[1:]), nil
	}
	return String(strings.Repeat("0", pad) + text), nil
}

// Justify returns the string padded to a width: mode -1 is left-justified,
// 0 is centred, and mode 1 would be right-justified.  The fill character must
// be exactly one character, which is CPython's rule and its message.
//
// Centring puts the ODD extra character on the RIGHT, so "ab".center(5) is
// "  ab " - measured, not assumed.
func (s String) Justify(args Tuple, mode int) (Object, error) {
	var (
		pywidth Object
		pyfill  Object = String(" ")
		name    string
	)
	switch mode {
	case 0:
		name = "center"
	case -1:
		name = "ljust"
	default:
		name = "rjust"
	}
	if err := ParseTuple(args, "i|O:"+name, &pywidth, &pyfill); err != nil {
		return nil, err
	}
	width := int(pywidth.(Int))
	fillStr, err := StrAsString(pyfill)
	if err != nil {
		return nil, ExceptionNewf(TypeError, "The fill character must be a unicode character, not %s", pyfill.Type().Name)
	}
	if fillStr == "" {
		fillStr = " "
	}
	fill := []rune(fillStr)
	if len(fill) != 1 {
		return nil, ExceptionNewf(TypeError, "The fill character must be exactly one character long")
	}
	if width <= s.len() {
		return s, nil
	}
	pad := width - s.len()
	switch mode {
	case -1:
		return String(string(s) + strings.Repeat(fillStr, pad)), nil
	case 1:
		return String(strings.Repeat(fillStr, pad) + string(s)), nil
	default:
		// The odd extra character goes on the RIGHT: "ab".center(5) is
		// "  ab " and "ab".center(4) is " ab " - measured against CPython.
		right := pad / 2
		left := pad - right
		return String(strings.Repeat(fillStr, left) + string(s) + strings.Repeat(fillStr, right)), nil
	}
}

// ExpandTabs expands tabs against the column position, and a newline or
// carriage return RESETS that column: "ab\tc".expandtabs(4) is "ab  c", not
// "ab  c" from a fixed replacement.
func (s String) ExpandTabs(args Tuple, kwargs StringDict) (Object, error) {
	var (
		pytabsize Object = Int(8)
	)
	if err := ParseTupleAndKeywords(args, kwargs, "|i:expandtabs", []string{"tabsize"}, &pytabsize); err != nil {
		return nil, err
	}
	tabSize := int(pytabsize.(Int))
	var out strings.Builder
	column := 0
	for _, r := range string(s) {
		switch r {
		case '\t':
			if tabSize <= 0 {
				// A non-positive tab size removes the tab entirely.
				continue
			}
			spaces := tabSize - (column % tabSize)
			out.WriteString(strings.Repeat(" ", spaces))
			column += spaces
		case '\n', '\r':
			out.WriteRune(r)
			column = 0
		default:
			out.WriteRune(r)
			column++
		}
	}
	return String(out.String()), nil
}

// RemoveAffix removes a prefix (or a suffix) when present, and returns the
// string unchanged when it is not.
func (s String) RemoveAffix(args Tuple, prefix bool) (Object, error) {
	var pyaffix Object
	name := "removesuffix"
	if prefix {
		name = "removeprefix"
	}
	if err := ParseTuple(args, "O:"+name, &pyaffix); err != nil {
		return nil, err
	}
	affix, err := StrAsString(pyaffix)
	if err != nil {
		return nil, err
	}
	text := string(s)
	if prefix {
		if strings.HasPrefix(text, affix) {
			return String(text[len(affix):]), nil
		}
	} else if affix != "" && strings.HasSuffix(text, affix) {
		return String(text[:len(text)-len(affix)]), nil
	}
	return s, nil
}

// FindMethod implements find, index, rfind and rindex.  A reverse search
// takes the HIGHEST index; raiseMissing turns the -1 into a ValueError, which
// is the only difference between find and index.
func (s String) FindMethod(args Tuple, name string, reverse, raiseMissing bool) (Object, error) {
	var (
		pysub Object
		pybeg Object = Int(0)
		pyend Object = Int(s.len())
	)
	if err := ParseTuple(args, "s|ii:"+name, &pysub, &pybeg, &pyend); err != nil {
		return nil, err
	}
	sub := string(pysub.(String))
	beg, end := adjustIndices(int(pybeg.(Int)), int(pyend.(Int)), s.len())
	haystack := string(s.slice(beg, end, s.len()))

	idx := -1
	if sub == "" {
		// An empty needle matches at the START for a forward search and at the
		// END for a reverse one: "abc".find("") is 0 and "abc".rfind("") is 3.
		if reverse {
			idx = len(haystack)
		} else {
			idx = 0
		}
	} else if reverse {
		idx = strings.LastIndex(haystack, sub)
	} else {
		idx = strings.Index(haystack, sub)
	}
	if idx < 0 {
		if raiseMissing {
			return nil, ExceptionNewf(ValueError, "substring not found")
		}
		return Int(-1), nil
	}
	return Int(beg + idx), nil
}

// Partition splits at the first (or the last) occurrence of sep, returning the
// three pieces.  A separator that is not found gives the whole string in the
// FIRST position for partition and in the LAST for rpartition, and an empty
// separator is a ValueError.
//
// Unlike split, the separator is KEPT, which is the point of the method.
func (s String) Partition(args Tuple, reverse bool) (Object, error) {
	name := "partition"
	if reverse {
		name = "rpartition"
	}
	var pysep Object
	if err := ParseTuple(args, "O:"+name, &pysep); err != nil {
		return nil, err
	}
	sep, err := StrAsString(pysep)
	if err != nil {
		return nil, err
	}
	if sep == "" {
		return nil, ExceptionNewf(ValueError, "empty separator")
	}
	text := string(s)
	idx := strings.Index(text, sep)
	if reverse {
		idx = strings.LastIndex(text, sep)
	}
	if idx < 0 {
		if reverse {
			return Tuple{String(""), String(""), s}, nil
		}
		return Tuple{s, String(""), String("")}, nil
	}
	return Tuple{String(text[:idx]), String(sep), String(text[idx+len(sep):])}, nil
}

// CaseFold is lower() plus the full case folds, of which the sharp s is the

// strMakeTrans builds a translation table, in the three forms CPython allows:
// a single mapping, two equal-length strings, or two strings plus a set of
// characters to delete.
func strMakeTrans(args Tuple) (Object, error) {
	if len(args) == 0 || len(args) > 3 {
		return nil, ExceptionNewf(TypeError, "maketrans() takes from 1 to 3 positional arguments but %d were given", len(args))
	}
	if len(args) == 1 {
		// A mapping, or an object with __getitem__ that yields the entries.
		table := NewStringDict()
		if d, ok := args[0].(IGetDict); ok {
			for _, e := range d.GetDict().Items() {
				key, err := DictKeyDecode(e.Key)
				if err != nil {
					return nil, err
				}
				// A single-character STRING key is cast to its ordinal -
				// maketrans({"a": "x"}) is {97: "x"} - and a longer one is
				// CPython's ValueError.
				var k int
				if str, ok := key.(String); ok {
					rs := []rune(string(str))
					if len(rs) != 1 {
						return nil, ExceptionNewf(ValueError,
							"string keys in translate table must be of length 1")
					}
					k = int(rs[0])
				} else {
					k, err = IndexInt(key)
					if err != nil {
						return nil, err
					}
				}
				if err := setTransEntry(table, Int(k), e.Value); err != nil {
					return nil, err
				}
			}
			return table, nil
		}
		return nil, ExceptionNewf(TypeError, "maketrans() argument must be a dict mapping")
	}

	x, err := StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	y, err := StrAsString(args[1])
	if err != nil {
		return nil, err
	}
	xs, ys := []rune(x), []rune(y)
	if len(xs) != len(ys) {
		return nil, ExceptionNewf(ValueError, "the first two maketrans arguments must have equal length")
	}
	table := NewStringDict()
	for i, r := range xs {
		// The value is the other string's ORDINAL, not the character:
		// str.maketrans("ab", "xy") is {97: 120, 98: 121}.
		if err := setTransEntry(table, Int(r), Int(ys[i])); err != nil {
			return nil, err
		}
	}
	if len(args) == 3 {
		z, err := StrAsString(args[2])
		if err != nil {
			return nil, err
		}
		for _, r := range z {
			if err := setTransEntry(table, Int(r), None); err != nil {
				return nil, err
			}
		}
	}
	return table, nil
}

// setTransEntry files one table entry, validating the value the way CPython
// does: a string replacement must be exactly one character, and anything that
// is neither a string, an int nor None is refused.
func setTransEntry(table StringDict, key Int, value Object) error {
	// The MAPPING form keeps whatever it was given - a string stays a string,
	// an int stays an int - so only the validity is checked here.  The
	// two-string form builds its own ordinals, above.
	if v, ok := value.(String); ok {
		if utf8.RuneCountInString(string(v)) != 1 {
			return ExceptionNewf(ValueError, "string keys in translate table must be of length 1")
		}
	}
	var buf []byte
	if err := appendKey(&buf, Int(key)); err != nil {
		return err
	}
	table.Set(string(buf), value)
	return nil
}

// Translate maps every character through a table: a value of None DELETES the
// character, a string replaces it, and an int is an ordinal to substitute.
//
// The table may be a mapping or any object with __getitem__, which rich relies
// on - its control-code stripper is a dict of ordinals to None.
func (s String) Translate(args Tuple) (Object, error) {
	var pytable Object
	if err := ParseTuple(args, "O:translate", &pytable); err != nil {
		return nil, err
	}
	var out strings.Builder
	for _, r := range string(s) {
		repl, err := transLookup(pytable, r)
		if err != nil {
			return nil, err
		}
		if repl == None {
			// None DELETES the character, which is a deliberate entry.
			continue
		}
		if repl == nil {
			// No entry at all keeps the character as it is.  Treating this
			// like None deleted every character the table did not mention.
			out.WriteRune(r)
			continue
		}
		if n, ok := repl.(Int); ok {
			// An int entry is an ordinal to substitute.
			out.WriteRune(rune(n))
			continue
		}
		text, err := StrAsString(repl)
		if err != nil {
			return nil, err
		}
		out.WriteString(text)
	}
	return String(out.String()), nil
}

// lookupTransKey reads an entry using the dict's own key encoding.
func lookupTransKey(d StringDict, key Object) (Object, bool) {
	var buf []byte
	if err := appendKey(&buf, key); err != nil {
		return nil, false
	}
	return d.Get(string(buf))
}

// transLookup finds the replacement for a rune, or nil when the table has no
// entry for it.  A table key that is a string is accepted too, because
// translate accepts a dict keyed by either the ordinal or the character.
func transLookup(table Object, r rune) (Object, error) {
	if d, ok := table.(IGetDict); ok {
		if v, ok := lookupTransKey(d.GetDict(), Int(r)); ok {
			return v, nil
		}
		if v, ok := lookupTransKey(d.GetDict(), String(string(r))); ok {
			return v, nil
		}
		return nil, nil
	}
	// An object with __getitem__: look the ordinal up and treat a KeyError as
	// "no entry".
	v, err := GetItem(table, Int(r))
	if err != nil {
		if IsException(KeyError, err) || IsException(LookupError, err) {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

// The case mappings that are NOT one character to one character.  Go's
// strings.ToLower/ToUpper can only map rune to rune, so these expand: the
// sharp s upper-cases to "SS", and the dotted capital I lower-cases to "i"
// PLUS a combining dot above, which is two characters.
//
// The tables are generated from CPython's own str.lower()/str.upper() over the
// whole codepoint range - every codepoint whose result is not a single
// character - rather than hand-picked, so the set is complete rather than the
// few that came to mind.
var specialUpper = map[rune][]rune{
	0xDF:   {83, 83},
	0x149:  {700, 78},
	0x1F0:  {74, 780},
	0x390:  {921, 776, 769},
	0x3B0:  {933, 776, 769},
	0x587:  {1333, 1362},
	0x1E96: {72, 817},
	0x1E97: {84, 776},
	0x1E98: {87, 778},
	0x1E99: {89, 778},
	0x1E9A: {65, 702},
	0x1F50: {933, 787},
	0x1F52: {933, 787, 768},
	0x1F54: {933, 787, 769},
	0x1F56: {933, 787, 834},
	0x1F80: {7944, 921},
	0x1F81: {7945, 921},
	0x1F82: {7946, 921},
	0x1F83: {7947, 921},
	0x1F84: {7948, 921},
	0x1F85: {7949, 921},
	0x1F86: {7950, 921},
	0x1F87: {7951, 921},
	0x1F88: {7944, 921},
	0x1F89: {7945, 921},
	0x1F8A: {7946, 921},
	0x1F8B: {7947, 921},
	0x1F8C: {7948, 921},
	0x1F8D: {7949, 921},
	0x1F8E: {7950, 921},
	0x1F8F: {7951, 921},
	0x1F90: {7976, 921},
	0x1F91: {7977, 921},
	0x1F92: {7978, 921},
	0x1F93: {7979, 921},
	0x1F94: {7980, 921},
	0x1F95: {7981, 921},
	0x1F96: {7982, 921},
	0x1F97: {7983, 921},
	0x1F98: {7976, 921},
	0x1F99: {7977, 921},
	0x1F9A: {7978, 921},
	0x1F9B: {7979, 921},
	0x1F9C: {7980, 921},
	0x1F9D: {7981, 921},
	0x1F9E: {7982, 921},
	0x1F9F: {7983, 921},
	0x1FA0: {8040, 921},
	0x1FA1: {8041, 921},
	0x1FA2: {8042, 921},
	0x1FA3: {8043, 921},
	0x1FA4: {8044, 921},
	0x1FA5: {8045, 921},
	0x1FA6: {8046, 921},
	0x1FA7: {8047, 921},
	0x1FA8: {8040, 921},
	0x1FA9: {8041, 921},
	0x1FAA: {8042, 921},
	0x1FAB: {8043, 921},
	0x1FAC: {8044, 921},
	0x1FAD: {8045, 921},
	0x1FAE: {8046, 921},
	0x1FAF: {8047, 921},
	0x1FB2: {8122, 921},
	0x1FB3: {913, 921},
	0x1FB4: {902, 921},
	0x1FB6: {913, 834},
	0x1FB7: {913, 834, 921},
	0x1FBC: {913, 921},
	0x1FC2: {8138, 921},
	0x1FC3: {919, 921},
	0x1FC4: {905, 921},
	0x1FC6: {919, 834},
	0x1FC7: {919, 834, 921},
	0x1FCC: {919, 921},
	0x1FD2: {921, 776, 768},
	0x1FD3: {921, 776, 769},
	0x1FD6: {921, 834},
	0x1FD7: {921, 776, 834},
	0x1FE2: {933, 776, 768},
	0x1FE3: {933, 776, 769},
	0x1FE4: {929, 787},
	0x1FE6: {933, 834},
	0x1FE7: {933, 776, 834},
	0x1FF2: {8186, 921},
	0x1FF3: {937, 921},
	0x1FF4: {911, 921},
	0x1FF6: {937, 834},
	0x1FF7: {937, 834, 921},
	0x1FFC: {937, 921},
	0xFB00: {70, 70},
	0xFB01: {70, 73},
	0xFB02: {70, 76},
	0xFB03: {70, 70, 73},
	0xFB04: {70, 70, 76},
	0xFB05: {83, 84},
	0xFB06: {83, 84},
	0xFB13: {1348, 1350},
	0xFB14: {1348, 1333},
	0xFB15: {1348, 1339},
	0xFB16: {1358, 1350},
	0xFB17: {1348, 1341},
}

var specialLower = map[rune][]rune{
	0x130: {105, 775},
}

// lowerString maps a string through lower(), expanding where CPython does.
func lowerString(s string) string {
	var out strings.Builder
	for _, r := range s {
		if repl, ok := specialLower[r]; ok {
			out.WriteString(string(repl))
			continue
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}

// upperString maps a string through upper(), expanding where CPython does.
func upperString(s string) string {
	var out strings.Builder
	for _, r := range s {
		if repl, ok := specialUpper[r]; ok {
			out.WriteString(string(repl))
			continue
		}
		out.WriteRune(unicode.ToUpper(r))
	}
	return out.String()
}
