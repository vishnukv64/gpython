// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// f-string (PEP 498) literal carrier.
//
// This object is a *compile-time* carrier only.  The lexer builds one
// for every f"..." / rf"..." literal and hands it to the parser as the
// value of a STRING token.  The compiler recognises it inside an
// *ast.Str and lowers it to ordinary bytecode; it must never survive
// into the running program.  If it somehow does, __repr__ is safe.

package py

// FString holds the raw text of an f-string literal exactly as written
// between the quotes (escape sequences and braces are NOT processed)
// together with whether the literal carried a raw (r) prefix.
type FString struct {
	Text string // raw contents, e.g. `a{x!r}b`
	Raw  bool   // true for rf"" / fr"" raw f-strings
}

// FStringType is the type of FString objects.
var FStringType = NewType("_FString", "internal f-string literal carrier")

// Type returns the type of the object.
func (f *FString) Type() *Type { return FStringType }

// M__repr__ is a safe representation; FString should never be visible
// to user code, but if it escapes it must not crash.
func (f *FString) M__repr__() (Object, error) {
	if f == nil {
		return String("<fstring: nil>"), nil
	}
	return String("<fstring " + f.Text + ">"), nil
}

// M__str__ mirrors __repr__.
func (f *FString) M__str__() (Object, error) { return f.M__repr__() }

// NewFString makes a new f-string carrier from the raw literal text.
func NewFString(text string, raw bool) *FString {
	return &FString{Text: text, Raw: raw}
}

var _ Object = (*FString)(nil)
var _ I__repr__ = (*FString)(nil)
var _ I__str__ = (*FString)(nil)
