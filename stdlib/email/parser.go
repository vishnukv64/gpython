// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package email

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const parser_doc = `email.parser - parse email messages from strings, bytes and files.

Parser and BytesParser read a whole message at once; FeedParser accepts data in
chunks.  Each returns an email.message.Message.  HeaderParser stops after the
headers and treats everything else as the body.`

// The parser is a hand-written RFC 5322 reader rather than a wrapper over Go's
// net/mail: net/mail drops the original header spelling, does not keep repeated
// headers separately with their order, and cannot report defects, all of which
// the email interface depends on.

// Parser parses a whole message from a string.
type Parser struct {
	policy  py.Object
	factory py.Object
	Dict    py.StringDict
}

func (p *Parser) Type() *py.Type { return ParserType }

func (p *Parser) GetDict() py.StringDict { return p.Dict }

// BytesParser parses a whole message from bytes.
type BytesParser struct {
	policy  py.Object
	factory py.Object
	Dict    py.StringDict
}

func (b *BytesParser) Type() *py.Type { return BytesParserType }

func (b *BytesParser) GetDict() py.StringDict { return b.Dict }

// HeaderParser parses only the headers.
type HeaderParser struct {
	policy  py.Object
	factory py.Object
	Dict    py.StringDict
}

func (h *HeaderParser) Type() *py.Type { return HeaderParserType }

func (h *HeaderParser) GetDict() py.StringDict { return h.Dict }

// BytesHeaderParser parses only the headers, from bytes.
type BytesHeaderParser struct {
	policy  py.Object
	factory py.Object
	Dict    py.StringDict
}

func (b *BytesHeaderParser) Type() *py.Type { return BytesHeaderParserType }

func (b *BytesHeaderParser) GetDict() py.StringDict { return b.Dict }

// FeedParser accepts the message incrementally.
type FeedParser struct {
	policy      py.Object
	factory     py.Object
	buf         strings.Builder
	closed      bool
	headersonly bool
	root        *Message
	Dict        py.StringDict
}

func (f *FeedParser) Type() *py.Type { return FeedParserType }

func (f *FeedParser) GetDict() py.StringDict { return f.Dict }

// --- parsing core --------------------------------------------------------

// parseMessage reads an RFC 5322 message from text, recording defects on the
// message as CPython's compat32 policy does.
//
// Only the defects CPython's compat32 policy reports for a bodyless or
// malformed header block are produced: MissingHeaderBodySeparatorDefect and
// MultipartInvariantViolationDefect.  A missing start or close boundary needs
// the full MIME multipart scanner, which is not implemented - a message that
// claims to be multipart but carries no subparts still gets
// MultipartInvariantViolationDefect, which is the case urllib3 tests for.
func parseMessage(text string, headersonly bool) *Message {
	text = strings.ReplaceAll(text, "\r\n", "\n")

	msg := &Message{Dict: py.NewStringDict()}
	msg.Dict.Set("defects", py.NewListFromItems(nil))

	// Split headers from the body at the first empty line.
	headerEnd := len(text)
	bodyStart := len(text)
	for i := 0; i < len(text); i++ {
		if text[i] != '\n' {
			continue
		}
		// A line that is empty (or "\r") ends the headers.
		lineStart := i + 1
		if lineStart < len(text) && text[lineStart] == '\n' {
			headerEnd = i + 1
			bodyStart = lineStart + 1
			break
		}
		if lineStart >= len(text) {
			headerEnd = len(text)
			bodyStart = len(text)
			break
		}
	}
	if bodyStart > len(text) {
		bodyStart = len(text)
	}

	headerBlock := text[:headerEnd]
	foundSeparator := bodyStart > headerEnd ||
		(strings.HasSuffix(text[:bodyStart], "\n\n"))
	if !foundSeparator && !headersonly {
		// There was no blank line: the whole text was treated as headers.
		msg.addDefect(MissingHeaderBodySeparatorDefect)
	}

	// A header block's first line must be a real header.
	lines := splitHeaderLines(headerBlock)
	first := true
	var curName, curValue string
	haveCur := false
	flush := func() {
		if haveCur {
			msg.setRawHeader(curName, curValue)
			haveCur = false
		}
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if !haveCur {
				// A continuation as the first header line is a defect.
				msg.addDefect(FirstHeaderLineIsContinuationDefect)
				continue
			}
			curValue += " " + strings.TrimLeft(line, " \t")
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			flush()
			msg.addDefect(MissingHeaderBodySeparatorDefect)
			first = false
			continue
		}
		flush()
		curName = line[:colon]
		curValue = strings.TrimLeft(line[colon+1:], " \t")
		haveCur = true
		first = false
	}
	flush()
	_ = first

	if headersonly {
		return msg
	}

	body := ""
	if bodyStart <= len(text) {
		body = text[bodyStart:]
	}

	// A message that claims to be multipart is split on its boundary.  compat32
	// also reports why the parts were not found: no boundary parameter at all,
	// or a boundary that never appeared.
	main := msg.contentMainType()
	if main == "multipart" {
		if boundary, ok := msg.getParam("boundary"); ok {
			if parts, preamble, ok := splitMultipart(body, boundary); ok {
				msg.preamble = preamble
				subs := make([]py.Object, 0, len(parts))
				for _, p := range parts {
					subs = append(subs, parseMessage(p, false))
				}
				msg.payload = py.NewListFromItems(subs)
			} else {
				msg.addDefect(StartBoundaryNotFoundDefect)
				msg.addDefect(MultipartInvariantViolationDefect)
				msg.payload = py.String(body)
			}
		} else {
			msg.addDefect(NoBoundaryInMultipartDefect)
			msg.addDefect(MultipartInvariantViolationDefect)
			msg.payload = py.String(body)
		}
	} else {
		msg.payload = py.String(body)
	}
	return msg
}

// splitMultipart divides a multipart body on its boundary.  It returns the raw
// text of each part, the preamble before the first boundary, and whether a
// start boundary was found at all.
func splitMultipart(body, boundary string) ([]string, string, bool) {
	delim := "--" + boundary
	lines := strings.Split(body, "\n")
	var parts []string
	var cur []string
	started := false
	preamble := ""
	var pre []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed == delim || trimmed == delim+"--" {
			if !started {
				started = true
				preamble = strings.Join(pre, "\n")
				if preamble != "" {
					preamble += "\n"
				}
				cur = nil
			} else {
				parts = append(parts, strings.Join(cur, "\n"))
				cur = nil
			}
			if trimmed == delim+"--" {
				return parts, preamble, true
			}
			continue
		}
		if !started {
			pre = append(pre, line)
			continue
		}
		cur = append(cur, line)
	}
	if !started {
		return nil, "", false
	}
	parts = append(parts, strings.Join(cur, "\n"))
	return parts, preamble, true
}

// splitHeaderLines folds a header block into logical lines: each header starts
// a new entry, and a line beginning with whitespace is a continuation folded
// onto the previous entry's value by the caller.
func splitHeaderLines(block string) []string {
	if block == "" {
		return nil
	}
	return strings.Split(block, "\n")
}

// newDefect builds a defect instance of the named type.
func newDefect(t *py.Type) py.Object {
	obj, err := py.ExceptionNew(t, nil, py.NewStringDict())
	if err != nil {
		return py.None
	}
	return obj
}

// --- constructors --------------------------------------------------------

func newPolicyArg(kwargs py.StringDict) py.Object {
	if v, ok := kwargs.Get("policy"); ok {
		return v
	}
	return py.None
}

func parserNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	factory := py.Object(py.None)
	if len(args) > 0 {
		factory = args[0]
	}
	if v, ok := kwargs.Get("_factory"); ok {
		factory = v
	}
	return &Parser{policy: newPolicyArg(kwargs), factory: factory, Dict: py.NewStringDict()}, nil
}

func bytesParserNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	factory := py.Object(py.None)
	if len(args) > 0 {
		factory = args[0]
	}
	if v, ok := kwargs.Get("_factory"); ok {
		factory = v
	}
	return &BytesParser{policy: newPolicyArg(kwargs), factory: factory, Dict: py.NewStringDict()}, nil
}

func headerParserNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	factory := py.Object(py.None)
	if len(args) > 0 {
		factory = args[0]
	}
	if v, ok := kwargs.Get("_factory"); ok {
		factory = v
	}
	return &HeaderParser{policy: newPolicyArg(kwargs), factory: factory, Dict: py.NewStringDict()}, nil
}

func bytesHeaderParserNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	factory := py.Object(py.None)
	if len(args) > 0 {
		factory = args[0]
	}
	if v, ok := kwargs.Get("_factory"); ok {
		factory = v
	}
	return &BytesHeaderParser{policy: newPolicyArg(kwargs), factory: factory, Dict: py.NewStringDict()}, nil
}

func feedParserNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	factory := py.Object(py.None)
	if len(args) > 0 {
		factory = args[0]
	}
	if v, ok := kwargs.Get("_factory"); ok {
		factory = v
	}
	return &FeedParser{policy: newPolicyArg(kwargs), factory: factory, Dict: py.NewStringDict()}, nil
}

// --- methods -------------------------------------------------------------

func strOrBytesArg(o py.Object) (string, error) {
	switch v := o.(type) {
	case py.String:
		return string(v), nil
	case py.Bytes:
		return string(v), nil
	}
	return "", py.ExceptionNewf(py.TypeError, "expected str or bytes")
}

func parserParse(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return parseFileCommon(args, kwargs)
}

func parserParseStr(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		text        py.Object
		headersonly py.Object = py.False
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:parsestr",
		[]string{"text", "headersonly"}, &text, &headersonly); err != nil {
		return nil, err
	}
	s, err := strOrBytesArg(text)
	if err != nil {
		return nil, err
	}
	return parseMessage(s, isTrueObj(headersonly)), nil
}

func parseFileCommon(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		fp          py.Object
		headersonly py.Object = py.False
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:parse",
		[]string{"fp", "headersonly"}, &fp, &headersonly); err != nil {
		return nil, err
	}
	data, err := readAllOf(fp)
	if err != nil {
		return nil, err
	}
	return parseMessage(data, isTrueObj(headersonly)), nil
}

// readAllOf drains a file-like object with read().
func readAllOf(fp py.Object) (string, error) {
	read, err := py.GetAttrString(fp, "read")
	if err != nil {
		return "", err
	}
	res, err := py.Call(read, nil, py.NewStringDict())
	if err != nil {
		return "", err
	}
	return strOrBytesArg(res)
}

func feedParserFeed(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var data py.Object
	if err := py.UnpackTuple(args, kwargs, "feed", 1, 1, &data); err != nil {
		return nil, err
	}
	s, err := strOrBytesArg(data)
	if err != nil {
		return nil, err
	}
	f := self.(*FeedParser)
	f.buf.WriteString(s)
	return py.None, nil
}

func feedParserClose(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	f := self.(*FeedParser)
	if !f.closed {
		f.root = parseMessage(f.buf.String(), f.headersonly)
		f.closed = true
	}
	if f.root == nil {
		f.root = &Message{Dict: py.NewStringDict()}
	}
	return f.root, nil
}

func feedParserSetHeadersonly(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	self.(*FeedParser).headersonly = true
	return py.None, nil
}

func isTrueObj(o py.Object) bool {
	b, err := py.MakeBool(o)
	return err == nil && b == py.True
}

// --- module-level functions ----------------------------------------------

func messageFromString(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		text        py.Object
		headersonly py.Object = py.False
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:message_from_string",
		[]string{"s", "headersonly"}, &text, &headersonly); err != nil {
		return nil, err
	}
	s, err := strOrBytesArg(text)
	if err != nil {
		return nil, err
	}
	return parseMessage(s, isTrueObj(headersonly)), nil
}

func messageFromBytes(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		text        py.Object
		headersonly py.Object = py.False
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:message_from_bytes",
		[]string{"s", "headersonly"}, &text, &headersonly); err != nil {
		return nil, err
	}
	s, err := strOrBytesArg(text)
	if err != nil {
		return nil, err
	}
	return parseMessage(s, isTrueObj(headersonly)), nil
}

func messageFromFile(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return parseFileCommon(args, kwargs)
}

func messageFromBinaryFile(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return parseFileCommon(args, kwargs)
}

var (
	ParserType            *py.Type
	BytesParserType       *py.Type
	HeaderParserType      *py.Type
	BytesHeaderParserType *py.Type
	FeedParserType        *py.Type
)

func init() {
	ParserType = py.NewTypeX("email.parser.Parser", "Parse a whole message from a string.", parserNew, nil)
	BytesParserType = py.NewTypeX("email.parser.BytesParser", "Parse a whole message from bytes.", bytesParserNew, nil)
	HeaderParserType = py.NewTypeX("email.parser.HeaderParser", "Parse only the headers of a message.", headerParserNew, nil)
	BytesHeaderParserType = py.NewTypeX("email.parser.BytesHeaderParser", "Parse only the headers from bytes.", bytesHeaderParserNew, nil)
	FeedParserType = py.NewTypeX("email.parser.FeedParser", "A feed-style parser of email.", feedParserNew, nil)

	for _, spec := range []struct {
		t        *py.Type
		fromFile bool
	}{
		{ParserType, true},
		{BytesParserType, true},
		{HeaderParserType, true},
		{BytesHeaderParserType, true},
	} {
		if spec.fromFile {
			spec.t.Dict.Set("parse", py.MustNewMethod("parse", parserParse, 0,
				"Parse the data from a file-like object and return a Message."))
			spec.t.Dict.Set("parsestr", py.MustNewMethod("parsestr", parserParseStr, 0,
				"Parse the given string and return a Message."))
			spec.t.Dict.Set("parsebytes", py.MustNewMethod("parsebytes", parserParseStr, 0,
				"Parse the given bytes and return a Message."))
		}
	}

	FeedParserType.Dict.Set("feed", py.MustNewMethod("feed", feedParserFeed, 0,
		"Push more data into the parser."))
	FeedParserType.Dict.Set("close", py.MustNewMethod("close", feedParserClose, 0,
		"Parse all remaining data and return the root message object."))
	FeedParserType.Dict.Set("_set_headersonly", py.MustNewMethod("_set_headersonly", feedParserSetHeadersonly, 0,
		"Parse only the headers."))

	globals := py.NewStringDict()
	globals.Set("Parser", ParserType)
	globals.Set("BytesParser", BytesParserType)
	globals.Set("HeaderParser", HeaderParserType)
	globals.Set("BytesHeaderParser", BytesHeaderParserType)
	globals.Set("FeedParser", FeedParserType)
	globals.Set("message_from_string", py.MustNewMethod("message_from_string", messageFromString, 0,
		"Parse a message from a string."))
	globals.Set("message_from_bytes", py.MustNewMethod("message_from_bytes", messageFromBytes, 0,
		"Parse a message from bytes."))
	globals.Set("message_from_file", py.MustNewMethod("message_from_file", messageFromFile, 0,
		"Parse a message from an open text file."))
	globals.Set("message_from_binary_file", py.MustNewMethod("message_from_binary_file", messageFromBinaryFile, 0,
		"Parse a message from an open binary file."))
	globals.Set("BytesFeedParser", FeedParserType)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "email.parser",
			Doc:  parser_doc,
		},
		Globals: globals,
	})
}
