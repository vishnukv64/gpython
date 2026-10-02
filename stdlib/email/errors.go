// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package email provides the implementation of python's 'email' package.
//
// It covers the parts the rest of the world actually imports: the exception
// hierarchy in email.errors (which urllib3 imports by name), the address and
// date utilities in email.utils, and a RFC 5322 parser in email.parser that
// builds a message object exposing the email.message.Message interface.
package email

import (
	"github.com/vishnukv64/gpython/py"
)

const errors_doc = `email.errors - exception and defect classes for the email package.

MessageError is the base class for the errors the package raises; the
MessageDefect hierarchy describes problems a parser was able to work around,
and each defect records the offending line (or None) in its .line attribute.`

// The exception classes are built as Go types subclassing the matching builtin
// exception.  NewType records the parent in .Bases (so issubclass and except
// work through the MRO) but leaves .Base nil, and Type.Ready would then default
// it to object - which is a bug for this purpose: isinstance walks .Base, so a
// defect would not be an instance of ValueError.  Setting .Base explicitly here
// keeps that walk on the hierarchy we declare.
func newExc(parent *py.Type, name, doc string, new py.NewFunc, init py.InitFunc) *py.Type {
	t := parent.NewType(name, doc, new, init)
	t.Base = parent
	return t
}

var (
	MessageError = newExc(py.ExceptionType, "email.errors.MessageError",
		"Base class for errors in the email package.", nil, nil)

	MessageParseError = newExc(MessageError, "email.errors.MessageParseError",
		"Base class for message parsing errors.", nil, nil)

	HeaderParseError = newExc(MessageParseError, "email.errors.HeaderParseError",
		"Error while parsing headers.", nil, nil)

	BoundaryError = newExc(MessageParseError, "email.errors.BoundaryError",
		"Couldn't find terminating boundary.", nil, nil)

	MultipartConversionError = newExc(MessageError, "email.errors.MultipartConversionError",
		"Conversion to a multipart is prohibited.", nil, nil)

	CharsetError = newExc(MessageError, "email.errors.CharsetError",
		"An illegal charset was given.", nil, nil)

	HeaderWriteError = newExc(MessageError, "email.errors.HeaderWriteError",
		"Error while writing headers.", nil, nil)
)

// defectNew builds a MessageDefect.  CPython's MessageDefect.__init__ takes a
// single optional line and stores it as .line, raising with the line as the
// argument when one was given; the plain Exception constructor already records
// the arguments, so only .line has to be added.  .line is a property on the
// type rather than an instance-dict entry because *py.Exception does not
// expose its Dict through IGetDict, so a Dict write would be invisible to
// attribute lookup.
func defectNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	obj, err := py.ExceptionNew(metatype, args, kwargs)
	if err != nil {
		return nil, err
	}
	e, ok := obj.(*py.Exception)
	if !ok {
		return obj, nil
	}
	var line py.Object = py.None
	if v, ok := kwargs.Get("line"); ok {
		line = v
	} else if len(args) > 0 {
		line = args[0]
	}
	e.Dict.Set("line", line)
	return obj, nil
}

// excLineProperty exposes .line for a defect: it reads and writes the
// exception's Dict, defaulting to None exactly as CPython's MessageDefect does.
func excLineProperty() *py.Property {
	return &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if e, ok := self.(*py.Exception); ok {
				if v, ok := e.Dict.Get("line"); ok {
					return v, nil
				}
			}
			return py.None, nil
		},
		Fset: func(self, value py.Object) error {
			if e, ok := self.(*py.Exception); ok {
				e.Dict.Set("line", value)
			}
			return nil
		},
	}
}

var (
	MessageDefect = newExc(py.ValueError, "email.errors.MessageDefect",
		"Base class for a message defect.", defectNew, nil)

	NoBoundaryInMultipartDefect = newExc(MessageDefect,
		"email.errors.NoBoundaryInMultipartDefect",
		"A message claimed to be a multipart but had no boundary parameter.", nil, nil)

	StartBoundaryNotFoundDefect = newExc(MessageDefect,
		"email.errors.StartBoundaryNotFoundDefect",
		"The claimed start boundary was never found.", nil, nil)

	CloseBoundaryNotFoundDefect = newExc(MessageDefect,
		"email.errors.CloseBoundaryNotFoundDefect",
		"A start boundary was found, but not the corresponding close boundary.", nil, nil)

	FirstHeaderLineIsContinuationDefect = newExc(MessageDefect,
		"email.errors.FirstHeaderLineIsContinuationDefect",
		"A message had a continuation line as its first header line.", nil, nil)

	MisplacedEnvelopeHeaderDefect = newExc(MessageDefect,
		"email.errors.MisplacedEnvelopeHeaderDefect",
		"A 'Unix-from' header was found in the middle of a header block.", nil, nil)

	MissingHeaderBodySeparatorDefect = newExc(MessageDefect,
		"email.errors.MissingHeaderBodySeparatorDefect",
		"Found line with no leading whitespace and no colon before blank line.", nil, nil)

	MultipartInvariantViolationDefect = newExc(MessageDefect,
		"email.errors.MultipartInvariantViolationDefect",
		"A message claimed to be a multipart but no subparts were found.", nil, nil)

	InvalidMultipartContentTransferEncodingDefect = newExc(MessageDefect,
		"email.errors.InvalidMultipartContentTransferEncodingDefect",
		"An invalid content transfer encoding was set on the multipart itself.", nil, nil)

	UndecodableBytesDefect = newExc(MessageDefect,
		"email.errors.UndecodableBytesDefect",
		"Header contained bytes that could not be decoded.", nil, nil)

	InvalidBase64PaddingDefect = newExc(MessageDefect,
		"email.errors.InvalidBase64PaddingDefect",
		"base64 encoded sequence had an incorrect length.", nil, nil)

	InvalidBase64CharactersDefect = newExc(MessageDefect,
		"email.errors.InvalidBase64CharactersDefect",
		"base64 encoded sequence had characters not in base64 alphabet.", nil, nil)

	InvalidBase64LengthDefect = newExc(MessageDefect,
		"email.errors.InvalidBase64LengthDefect",
		"base64 encoded sequence had invalid length (1 mod 4).", nil, nil)
)

var (
	HeaderDefect = newExc(MessageDefect, "email.errors.HeaderDefect",
		"Base class for a header defect.", nil, nil)

	InvalidHeaderDefect = newExc(HeaderDefect, "email.errors.InvalidHeaderDefect",
		"Header is not valid, message gives details.", nil, nil)

	HeaderMissingRequiredValue = newExc(HeaderDefect, "email.errors.HeaderMissingRequiredValue",
		"A header that must have a value had none.", nil, nil)

	HeaderMayNotBeEmpty = newExc(HeaderDefect, "email.errors.HeaderMayNotBeEmpty",
		"A header that must not be empty was empty.", nil, nil)

	ObsoleteHeaderDefect = newExc(HeaderDefect, "email.errors.ObsoleteHeaderDefect",
		"Header uses syntax declared obsolete by RFC 5322.", nil, nil)

	NonASCIILocalPartDefect = newExc(HeaderDefect, "email.errors.NonASCIILocalPartDefect",
		"local_part contains non-ASCII characters.", nil, nil)

	InvalidDateDefect = newExc(HeaderDefect, "email.errors.InvalidDateDefect",
		"Header has unparsable or invalid date.", nil, nil)

	FirstHeaderLineIsContinuationDefectAlias = FirstHeaderLineIsContinuationDefect
)

// nonPrintableDefectNew records .non_printables and renders CPython's message.
func nonPrintableDefectNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	obj, err := py.ExceptionNew(metatype, args, kwargs)
	if err != nil {
		return nil, err
	}
	e, ok := obj.(*py.Exception)
	if !ok {
		return obj, nil
	}
	if len(args) > 0 {
		e.Dict.Set("non_printables", args[0])
		e.Dict.Set("line", args[0])
	}
	return obj, nil
}

// nonPrintableDefectStr is CPython's __str__ for NonPrintableDefect.
func nonPrintableDefectStr(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	np, err := py.GetAttrString(self, "non_printables")
	if err != nil {
		return nil, err
	}
	s, err := py.StrAsString(np)
	if err != nil {
		return nil, err
	}
	return py.String("the following ASCII non-printables found in header: " + s), nil
}

var NonPrintableDefect = newExc(HeaderDefect, "email.errors.NonPrintableDefect",
	"ASCII characters outside the ascii-printable range found.", nonPrintableDefectNew, nil)

func init() {
	MessageDefect.Dict.Set("line", excLineProperty())
	NonPrintableDefect.Dict.Set("__str__", py.MustNewMethod("__str__", nonPrintableDefectStr, 0,
		"the following ASCII non-printables found in header: <non_printables>"))

	// XXX: backward compatibility alias, exactly as CPython defines it.
	globals := py.NewStringDict()
	globals.Set("MessageError", MessageError)
	globals.Set("MessageParseError", MessageParseError)
	globals.Set("HeaderParseError", HeaderParseError)
	globals.Set("BoundaryError", BoundaryError)
	globals.Set("MultipartConversionError", MultipartConversionError)
	globals.Set("CharsetError", CharsetError)
	globals.Set("HeaderWriteError", HeaderWriteError)
	globals.Set("MessageDefect", MessageDefect)
	globals.Set("NoBoundaryInMultipartDefect", NoBoundaryInMultipartDefect)
	globals.Set("StartBoundaryNotFoundDefect", StartBoundaryNotFoundDefect)
	globals.Set("CloseBoundaryNotFoundDefect", CloseBoundaryNotFoundDefect)
	globals.Set("FirstHeaderLineIsContinuationDefect", FirstHeaderLineIsContinuationDefect)
	globals.Set("MisplacedEnvelopeHeaderDefect", MisplacedEnvelopeHeaderDefect)
	globals.Set("MissingHeaderBodySeparatorDefect", MissingHeaderBodySeparatorDefect)
	globals.Set("MalformedHeaderDefect", MissingHeaderBodySeparatorDefect)
	globals.Set("MultipartInvariantViolationDefect", MultipartInvariantViolationDefect)
	globals.Set("InvalidMultipartContentTransferEncodingDefect", InvalidMultipartContentTransferEncodingDefect)
	globals.Set("UndecodableBytesDefect", UndecodableBytesDefect)
	globals.Set("InvalidBase64PaddingDefect", InvalidBase64PaddingDefect)
	globals.Set("InvalidBase64CharactersDefect", InvalidBase64CharactersDefect)
	globals.Set("InvalidBase64LengthDefect", InvalidBase64LengthDefect)
	globals.Set("HeaderDefect", HeaderDefect)
	globals.Set("InvalidHeaderDefect", InvalidHeaderDefect)
	globals.Set("HeaderMissingRequiredValue", HeaderMissingRequiredValue)
	globals.Set("HeaderMayNotBeEmpty", HeaderMayNotBeEmpty)
	globals.Set("ObsoleteHeaderDefect", ObsoleteHeaderDefect)
	globals.Set("NonASCIILocalPartDefect", NonASCIILocalPartDefect)
	globals.Set("InvalidDateDefect", InvalidDateDefect)
	globals.Set("NonPrintableDefect", NonPrintableDefect)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "email.errors",
			Doc:  errors_doc,
		},
		Globals: globals,
	})
}
