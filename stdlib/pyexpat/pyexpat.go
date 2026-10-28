// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pyexpat is a subset of CPython's pyexpat, enough for
// xml.parsers.expat, xmlrpc.client and xml.etree.ElementTree to parse
// well-formed documents.  It is built on encoding/xml, not on expat, so:
//
//   - Parse() buffers its input and runs the document when isfinal is true;
//     handlers fire then, not as each chunk arrives.
//   - Supported handlers: StartElementHandler, EndElementHandler,
//     CharacterDataHandler, CommentHandler, ProcessingInstructionHandler,
//     with buffer_text, ordered_attributes and namespace_separator.
//   - DOCTYPE, entity declarations, DefaultHandler, external entities and
//     the namespace-declaration handlers are not reported.
//   - Error text follows expat's wording for the common cases only.
package pyexpat

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/vishnukv64/gpython/py"
)

// ExpatError is pyexpat.ExpatError, also exported as pyexpat.error.
var ExpatError = py.ExceptionType.NewType("pyexpat.ExpatError", "Expat parse error.", nil, nil)

// ParserType is the class of a parser made by ParserCreate.
var ParserType = py.NewTypeX("pyexpat.xmlparser", "XML parser", nil, nil)

type parser struct {
	Dict py.StringDict // handlers and flags assigned from Python
	sep  string
	buf  []byte
	done bool
}

func (p *parser) Type() *py.Type         { return ParserType }
func (p *parser) GetDict() py.StringDict { return p.Dict }

func (p *parser) handler(name string) py.Object {
	if h := p.Dict.GetOrNil(name); h != nil && h != py.None {
		return h
	}
	return nil
}

func (p *parser) flag(name string) bool {
	v := p.Dict.GetOrNil(name)
	if v == nil {
		return false
	}
	ok, _ := py.ObjectIsTrue(v)
	return ok
}

func parserCreate(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var enc, sep, intern py.Object = py.None, py.None, py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOO:ParserCreate", []string{"encoding", "namespace_separator", "intern"}, &enc, &sep, &intern); err != nil {
		return nil, err
	}
	p := &parser{Dict: py.NewStringDict()}
	if sep != py.None {
		s, ok := sep.(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "ParserCreate() argument 'namespace_separator' must be str or None, not %s", sep.Type().Name)
		}
		if len(s) > 1 {
			return nil, py.ExceptionNewf(py.ValueError, "namespace_separator must be at most one character, omitted, or None")
		}
		p.sep = string(s)
	}
	for _, n := range []string{"ErrorLineNumber", "ErrorColumnNumber", "ErrorByteIndex", "ErrorCode"} {
		p.Dict.Set(n, py.Int(0))
	}
	p.Dict.Set("CurrentLineNumber", py.Int(1))
	p.Dict.Set("CurrentColumnNumber", py.Int(0))
	p.Dict.Set("buffer_text", py.False)
	p.Dict.Set("ordered_attributes", py.False)
	p.Dict.Set("specified_attributes", py.False)
	return p, nil
}

// lineCol is the 1-based line and 0-based column of byte offset off in buf.
func lineCol(buf []byte, off int64) (int, int) {
	if off > int64(len(buf)) {
		off = int64(len(buf))
	}
	head := buf[:off]
	line := bytes.Count(head, []byte("\n")) + 1
	col := len(head) - (bytes.LastIndexByte(head, '\n') + 1)
	return line, col
}

func (p *parser) fail(code int, msg string, off int64) error {
	line, col := lineCol(p.buf, off)
	p.Dict.Set("ErrorLineNumber", py.Int(line))
	p.Dict.Set("ErrorColumnNumber", py.Int(col))
	p.Dict.Set("ErrorByteIndex", py.Int(off))
	p.Dict.Set("ErrorCode", py.Int(code))
	e := py.ExceptionNewf(ExpatError, "%s: line %d, column %d", msg, line, col)
	e.Dict.Set("code", py.Int(code))
	e.Dict.Set("lineno", py.Int(line))
	e.Dict.Set("offset", py.Int(col))
	return e
}

func charset(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return in, nil
	case "iso-8859-1", "latin-1", "latin1":
		raw, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		rs := make([]rune, len(raw))
		for i, b := range raw {
			rs[i] = rune(b)
		}
		return strings.NewReader(string(rs)), nil
	}
	return nil, io.ErrUnexpectedEOF
}

func call(h py.Object, args ...py.Object) error {
	_, err := py.Call(h, py.Tuple(args), py.StringDict{})
	return err
}

func (p *parser) name(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	if p.sep != "" {
		return n.Space + p.sep + n.Local
	}
	return n.Space + ":" + n.Local
}

func (p *parser) run() error {
	if !utf8.Valid(p.buf) && !bytes.HasPrefix(bytes.TrimSpace(p.buf), []byte("<?xml")) {
		return p.fail(31, "not well-formed (invalid token)", 0)
	}
	dec := xml.NewDecoder(bytes.NewReader(p.buf))
	dec.CharsetReader = charset
	var stack []string
	var text strings.Builder
	buffer := p.flag("buffer_text")
	flush := func() error {
		if text.Len() == 0 {
			return nil
		}
		s := text.String()
		text.Reset()
		if h := p.handler("CharacterDataHandler"); h != nil {
			return call(h, py.String(s))
		}
		return nil
	}
	sawRoot := false
	for {
		tok, err := p.next(dec)
		off := dec.InputOffset()
		if err == io.EOF {
			if !sawRoot || len(stack) > 0 {
				return p.fail(3, "no element found", int64(len(p.buf)))
			}
			return flush()
		}
		if err != nil {
			if strings.Contains(err.Error(), "unexpected EOF") {
				return p.fail(3, "no element found", int64(len(p.buf)))
			}
			if strings.Contains(err.Error(), "element <") {
				return p.fail(7, "mismatched tag", off)
			}
			return p.fail(2, "not well-formed (invalid token)", off)
		}
		line, col := lineCol(p.buf, off)
		p.Dict.Set("CurrentLineNumber", py.Int(line))
		p.Dict.Set("CurrentColumnNumber", py.Int(col))
		switch t := tok.(type) {
		case xml.CharData:
			if len(stack) == 0 {
				continue // whitespace around the root
			}
			text.Write(t)
			if !buffer {
				if err := flush(); err != nil {
					return err
				}
			}
			continue
		case xml.StartElement:
			if err := flush(); err != nil {
				return err
			}
			sawRoot = true
			nm := p.name(t.Name)
			stack = append(stack, nm)
			h := p.handler("StartElementHandler")
			if h == nil {
				continue
			}
			var attrs py.Object
			ordered := p.flag("ordered_attributes")
			list := py.NewListFromItems(nil)
			d := py.NewStringDict()
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
					if p.sep != "" {
						continue
					}
				}
				an := p.name(a.Name)
				if ordered {
					list.Items = append(list.Items, py.String(an), py.String(a.Value))
				} else {
					d.Set(an, py.String(a.Value))
				}
			}
			if ordered {
				attrs = list
			} else {
				attrs = d
			}
			if err := call(h, py.String(nm), attrs); err != nil {
				return err
			}
		case xml.EndElement:
			if err := flush(); err != nil {
				return err
			}
			if len(stack) == 0 {
				return p.fail(7, "mismatched tag", off)
			}
			nm := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if h := p.handler("EndElementHandler"); h != nil {
				if err := call(h, py.String(nm)); err != nil {
					return err
				}
			}
		case xml.Comment:
			if err := flush(); err != nil {
				return err
			}
			if h := p.handler("CommentHandler"); h != nil {
				if err := call(h, py.String(string(t))); err != nil {
					return err
				}
			}
		case xml.ProcInst:
			if t.Target == "xml" {
				continue
			}
			if err := flush(); err != nil {
				return err
			}
			if h := p.handler("ProcessingInstructionHandler"); h != nil {
				if err := call(h, py.String(t.Target), py.String(string(t.Inst))); err != nil {
					return err
				}
			}
		}
	}
}

// next reads a token: translated by namespace when a separator was asked for,
// raw (prefixes kept, as expat does without namespace processing) otherwise.
func (p *parser) next(dec *xml.Decoder) (xml.Token, error) {
	if p.sep != "" {
		return dec.Token()
	}
	return dec.RawToken()
}

func parse(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p := self.(*parser)
	var data py.Object
	var isfinal py.Object = py.False
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:Parse", []string{"data", "isfinal"}, &data, &isfinal); err != nil {
		return nil, err
	}
	switch d := data.(type) {
	case py.String:
		p.buf = append(p.buf, d...)
	case py.Bytes:
		p.buf = append(p.buf, d...)
	default:
		return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not '%s'", data.Type().Name)
	}
	final, _ := py.ObjectIsTrue(isfinal)
	if !final {
		return py.Int(1), nil
	}
	if p.done {
		return nil, p.fail(36, "parsing finished", 0)
	}
	p.done = true
	if err := p.run(); err != nil {
		return nil, err
	}
	return py.Int(1), nil
}

func init() {
	ParserType.Dict.Set("Parse", py.MustNewMethod("Parse", parse, 0, "Parse(data[, isfinal]) -> 1. Input is buffered until isfinal is true."))
	for _, n := range []string{"SetBase", "SetReparseDeferralEnabled", "SetParamEntityParsing"} {
		ParserType.Dict.Set(n, py.MustNewMethod(n, func(self py.Object, args py.Tuple) (py.Object, error) { return py.None, nil }, 0, ""))
	}
	ParserType.Dict.Set("GetBase", py.MustNewMethod("GetBase", func(self py.Object, args py.Tuple) (py.Object, error) { return py.None, nil }, 0, ""))
	ParserType.Dict.Set("GetReparseDeferralEnabled", py.MustNewMethod("GetReparseDeferralEnabled", func(self py.Object, args py.Tuple) (py.Object, error) { return py.False, nil }, 0, ""))

	g := py.NewStringDict()
	g.Set("__path__", py.NewListFromItems(nil))
	g.Set("ExpatError", ExpatError)
	g.Set("error", ExpatError)
	g.Set("XMLParserType", ParserType)
	g.Set("version_info", py.Tuple{py.Int(2), py.Int(6), py.Int(0)})
	g.Set("EXPAT_VERSION", py.String("expat_2.6.0"))
	g.Set("native_encoding", py.String("UTF-8"))
	g.Set("features", py.NewListFromItems(nil))
	g.Set("XML_PARAM_ENTITY_PARSING_NEVER", py.Int(0))
	g.Set("XML_PARAM_ENTITY_PARSING_UNLESS_STANDALONE", py.Int(1))
	g.Set("XML_PARAM_ENTITY_PARSING_ALWAYS", py.Int(2))
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "pyexpat", Doc: "Python wrapper for Expat parser (subset)."},
		Methods: []*py.Method{
			py.MustNewMethod("ParserCreate", parserCreate, 0, "ParserCreate([encoding[, namespace_separator]]) -> parser"),
		},
		Globals: g,
		CodeSrc: "import pyexpat.errors as errors\nimport pyexpat.model as model\n",
	})

	msgs := map[int]string{
		2: "not well-formed (invalid token)", 3: "no element found", 7: "mismatched tag",
		31: "not well-formed (invalid token)", 36: "parsing finished",
	}
	names := map[int]string{2: "XML_ERROR_INVALID_TOKEN", 3: "XML_ERROR_NO_ELEMENTS", 7: "XML_ERROR_TAG_MISMATCH",
		36: "XML_ERROR_FINISHED"}
	eg := py.NewStringDict()
	codes, messages := py.NewStringDict(), py.NewStringDict()
	for code, msg := range msgs {
		if n, ok := names[code]; ok {
			eg.Set(n, py.String(msg))
			codes.Set(msg, py.Int(code))
			messages.Set(string(rune('0'+code/10))+string(rune('0'+code%10)), py.String(msg))
		}
	}
	eg.Set("codes", codes)
	eg.Set("messages", messages)
	py.RegisterModule(&py.ModuleImpl{Info: py.ModuleInfo{Name: "pyexpat.errors", Doc: "Constants used to describe error conditions."}, Globals: eg})
	py.RegisterModule(&py.ModuleImpl{Info: py.ModuleInfo{Name: "pyexpat.model", Doc: "Constants used to interpret content model information."}, Globals: py.NewStringDict()})
}
