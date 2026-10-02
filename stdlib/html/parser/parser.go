// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package parser provides python's 'html.parser' submodule: a tolerant HTML
// and XHTML parser.
//
// The parser scans its buffer for '<' and '&' and calls the handler methods
// for each construct it recognises.  A subclass overrides the handle_*
// methods; the handlers are looked up on the instance, not on this Go type, so
// an override in Python is the one that runs.
//
// CPython expresses its tag grammar as regular expressions with lookbehind.
// Go's regexp engine has no lookbehind, so the tag-name and attribute scanning
// is written out by hand here.  The grammar matched is the same one (a tag
// name followed by space- or slash-separated attributes, each with an optional
// quoted or bare value), and the results are checked against CPython's.
package parser

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/html"
)

const module_doc = `A parser for HTML and XHTML.`

func init() {
	parserType.Dict.Set("CDATA_CONTENT_ELEMENTS", py.Tuple{py.String("script"), py.String("style")})
	parserType.Dict.Set("RCDATA_CONTENT_ELEMENTS", py.Tuple{py.String("title"), py.String("textarea")})
	// convert_charrefs is fixed at construction but is read as an ordinary
	// attribute, as it is in CPython.
	parserType.Dict.Set("convert_charrefs", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.NewBool(self.(*HTMLParser).convertCharrefs), nil
	}})
	parserType.Dict.Set("scripting", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.NewBool(self.(*HTMLParser).scripting), nil
	}})

	setMethod("reset", parserReset, reset_doc)
	setMethod("feed", parserFeed, feed_doc)
	setMethod("close", parserClose, close_doc)
	setMethod("get_starttag_text", parserGetStartTagText, get_starttag_text_doc)
	setMethod("set_cdata_mode", parserSetCdataMode, "Internal -- set the current CDATA mode.")
	setMethod("clear_cdata_mode", parserClearCdataMode, "Internal -- restore normal parsing mode.")
	setMethod("_set_support_cdata", parserSetSupportCdata, "Enable or disable support of CDATA sections.")
	setMethod("parse_starttag", parserParseStarttag, "Internal -- parse a start tag, return the end of the tag.")
	setMethod("parse_endtag", parserParseEndtag, "Internal -- parse an end tag, return the end of the tag.")
	setMethod("parse_comment", parserParseComment, "Internal -- parse a comment.")
	setMethod("parse_pi", parserParsePI, "Internal -- parse a processing instruction.")
	setMethod("parse_html_declaration", parserParseDeclaration, "Internal -- parse an HTML declaration.")
	setMethod("parse_bogus_comment", parserParseBogusComment, "Internal -- parse a bogus comment.")
	setMethod("goahead", parserGoahead, "Internal -- scan the buffer, calling handlers as constructs are found.")

	// The overridable handler methods.  Each is a no-op that a subclass
	// replaces; they exist so that the base class has the attribute.
	setMethod("handle_starttag", parserNoop2, "Handle a start tag.")
	setMethod("handle_endtag", parserNoop1, "Handle an end tag.")
	setMethod("handle_startendtag", parserHandleStartendtag, "Handle an XHTML-style empty tag.")
	setMethod("handle_charref", parserNoop1, "Handle a numeric character reference.")
	setMethod("handle_entityref", parserNoop1, "Handle a named character reference.")
	setMethod("handle_data", parserNoop1, "Handle character data.")
	setMethod("handle_comment", parserNoop1, "Handle a comment.")
	setMethod("handle_decl", parserNoop1, "Handle a declaration.")
	setMethod("handle_pi", parserNoop1, "Handle a processing instruction.")
	setMethod("unknown_decl", parserNoop1, "Handle an unknown declaration.")

	globals := py.NewStringDict()
	globals.Set("HTMLParser", parserType)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "html.parser",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

func setMethod(name string, fn interface{}, doc string) {
	parserType.Dict.Set(name, py.MustNewMethod(name, fn, 0, doc))
}

const reset_doc = `Reset this instance.  Loses all unprocessed data.`
const feed_doc = `Feed data to the parser.

Call this as often as you want, with as little or as much text
as you want (may include '\n').`
const close_doc = `Handle any buffered data.`
const get_starttag_text_doc = `Return full source of start tag: '<...>'.`

const parser_doc = `Find tags and other markup and call handler functions.

Usage:
    p = HTMLParser()
    p.feed(data)
    ...
    p.close()

Start tags are handled by calling self.handle_starttag() or
self.handle_startendtag(); end tags by self.handle_endtag().  The
data between tags is passed from the parser to the derived class
by calling self.handle_data() with the data as argument (the data
may be split up in arbitrary chunks).  If convert_charrefs is
True the character references are converted automatically to the
corresponding Unicode character (and self.handle_data() is no
longer split in chunks), otherwise they are passed by calling
self.handle_entityref() or self.handle_charref() with the string
containing respectively the named or numeric reference as the
argument.`

// HTMLParser is one parser instance.  The Go fields hold the scanner state; the
// handler methods and any attributes a Python subclass sets live in the
// instance dictionary.  typ is the type the instance was built as, which is a
// Python subclass when one derives from HTMLParser, so that handle_* overrides
// are found.
type HTMLParser struct {
	typ  *py.Type
	dict py.StringDict

	rawdata string
	// pending buffers feed() input until enough has accumulated to parse.
	pending     []string
	pendingLen  int
	parseThresh int

	lasttag      string
	starttagText py.Object
	cdataElem    string
	// escapable is whether character references are converted inside the
	// current CDATA element.
	escapable   bool
	supportCDAT bool
	// convertCharrefs is fixed at construction.
	convertCharrefs bool
	scripting       bool
}

var parserType = py.NewTypeX("html.parser.HTMLParser", parser_doc, parserNew, nil)

func (p *HTMLParser) Type() *py.Type {
	if p.typ != nil {
		return p.typ
	}
	return parserType
}

func (p *HTMLParser) GetDict() py.StringDict { return p.dict }

func parserNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p := &HTMLParser{typ: metatype, dict: py.NewStringDict(), convertCharrefs: true}
	if v, ok := kwargs.Get("convert_charrefs"); ok {
		b, err := py.ObjectIsTrue(v)
		if err != nil {
			return nil, err
		}
		p.convertCharrefs = b
	}
	if v, ok := kwargs.Get("scripting"); ok {
		b, err := py.ObjectIsTrue(v)
		if err != nil {
			return nil, err
		}
		p.scripting = b
	}
	if len(args) > 0 {
		return nil, py.ExceptionNewf(py.TypeError, "HTMLParser() takes no positional arguments")
	}
	p.reset()
	return p, nil
}

func (p *HTMLParser) reset() {
	p.rawdata = ""
	p.lasttag = "???"
	p.cdataElem = ""
	p.supportCDAT = true
	p.escapable = true
	p.pending = nil
	p.pendingLen = 0
	p.parseThresh = 1
	p.starttagText = py.None
}

func parserReset(self py.Object, args py.Tuple) (py.Object, error) {
	self.(*HTMLParser).reset()
	return py.None, nil
}

func parserFeed(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	var dataObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "feed", 1, 1, &dataObj); err != nil {
		return nil, err
	}
	data, err := py.StrAsString(dataObj)
	if err != nil {
		return nil, err
	}
	// Accumulate in a list and only join and parse once enough has piled up.
	// Rescanning an unparsed buffer on every call would be quadratic in the
	// input size.
	p.pendingLen += len(data)
	if p.pendingLen < p.parseThresh {
		p.pending = append(p.pending, data)
		return py.None, nil
	}
	if len(p.pending) == 0 {
		p.rawdata += data
	} else {
		p.pending = append(p.pending, data)
		p.rawdata += strings.Join(p.pending, "")
		p.pending = nil
	}
	p.pendingLen = 0
	n := len(p.rawdata)
	if err := p.goahead(self, false); err != nil {
		return nil, err
	}
	if len(p.rawdata) < n {
		p.parseThresh = 1
	} else {
		p.parseThresh = len(p.rawdata)
	}
	return py.None, nil
}

func parserClose(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	if len(p.pending) > 0 {
		p.rawdata += strings.Join(p.pending, "")
		p.pending = nil
		p.pendingLen = 0
	}
	if err := p.goahead(self, true); err != nil {
		return nil, err
	}
	return py.None, nil
}

func parserGetStartTagText(self py.Object, args py.Tuple) (py.Object, error) {
	return self.(*HTMLParser).starttagText, nil
}

func parserSetCdataMode(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	var elemObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "set_cdata_mode", 1, 1, &elemObj); err != nil {
		return nil, err
	}
	elem, err := py.StrAsString(elemObj)
	if err != nil {
		return nil, err
	}
	p.cdataElem = strings.ToLower(elem)
	p.escapable = false
	return py.None, nil
}

func parserClearCdataMode(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	p.cdataElem = ""
	p.escapable = true
	return py.None, nil
}

func parserSetSupportCdata(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	p.supportCDAT = true
	if len(args) > 0 {
		b, err := py.ObjectIsTrue(args[0])
		if err != nil {
			return nil, err
		}
		p.supportCDAT = b
	}
	return py.None, nil
}

func parserGoahead(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	end := false
	if len(args) > 0 && args[0] != py.None {
		b, err := py.ObjectIsTrue(args[0])
		if err != nil {
			return nil, err
		}
		end = b
	}
	return py.None, p.goahead(self, end)
}

func parserNoop1(self py.Object, args py.Tuple) (py.Object, error) { return py.None, nil }
func parserNoop2(self py.Object, args py.Tuple) (py.Object, error) { return py.None, nil }

func parserHandleStartendtag(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "handle_startendtag() requires tag and attrs")
	}
	if _, err := callHandler(self, "handle_starttag", py.Tuple{args[0], args[1]}); err != nil {
		return nil, err
	}
	return callHandler(self, "handle_endtag", py.Tuple{args[0]})
}

func parserParseStarttag(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	var iObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "parse_starttag", 1, 1, &iObj); err != nil {
		return nil, err
	}
	i, err := toInt(iObj)
	if err != nil {
		return nil, err
	}
	end, err := p.parseStarttag(self, i)
	if err != nil {
		return nil, err
	}
	return py.Int(end), nil
}

func parserParseEndtag(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	var iObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "parse_endtag", 1, 1, &iObj); err != nil {
		return nil, err
	}
	i, err := toInt(iObj)
	if err != nil {
		return nil, err
	}
	end, err := p.parseEndtag(self, i)
	if err != nil {
		return nil, err
	}
	return py.Int(end), nil
}

func parserParseComment(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	var iObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "parse_comment", 1, 2, &iObj); err != nil {
		return nil, err
	}
	i, err := toInt(iObj)
	if err != nil {
		return nil, err
	}
	end, err := p.parseComment(self, i)
	if err != nil {
		return nil, err
	}
	return py.Int(end), nil
}

func parserParsePI(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	var iObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "parse_pi", 1, 1, &iObj); err != nil {
		return nil, err
	}
	i, err := toInt(iObj)
	if err != nil {
		return nil, err
	}
	end, err := p.parsePI(self, i)
	if err != nil {
		return nil, err
	}
	return py.Int(end), nil
}

func parserParseDeclaration(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	var iObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "parse_html_declaration", 1, 1, &iObj); err != nil {
		return nil, err
	}
	i, err := toInt(iObj)
	if err != nil {
		return nil, err
	}
	end, err := p.parseDeclaration(self, i)
	if err != nil {
		return nil, err
	}
	return py.Int(end), nil
}

func parserParseBogusComment(self py.Object, args py.Tuple) (py.Object, error) {
	p := self.(*HTMLParser)
	var iObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "parse_bogus_comment", 1, 2, &iObj); err != nil {
		return nil, err
	}
	i, err := toInt(iObj)
	if err != nil {
		return nil, err
	}
	end, err := p.parseBogusComment(self, i, true)
	if err != nil {
		return nil, err
	}
	return py.Int(end), nil
}

// goahead is the main scan loop, a port of HTMLParser.goahead.
func (p *HTMLParser) goahead(self py.Object, end bool) error {
	rawdata := p.rawdata
	i := 0
	n := len(rawdata)
	for i < n {
		var j int
		if p.convertCharrefs && p.cdataElem == "" {
			k := strings.IndexByte(rawdata[i:], '<')
			if k < 0 {
				// If the next '<' cannot be found, either we are at the end
				// or more text is coming.  In the latter case the text cannot
				// be handed to handle_data yet, because a character reference
				// could be cut in half.  Look for an '&' near the end and see
				// whether it is followed by a terminator.
				scanFrom := i
				if n-34 > scanFrom {
					scanFrom = n - 34
				}
				amppos := strings.LastIndexByte(rawdata[scanFrom:], '&')
				if amppos >= 0 {
					amppos += scanFrom
					if !hasCharrefTerminator(rawdata[amppos:]) {
						break // wait until all the text has arrived
					}
				}
				j = n
			} else {
				j = i + k
			}
		} else {
			k := findInteresting(rawdata, i, p.cdataElem, p.convertCharrefs, p.escapable)
			if k >= 0 {
				j = k
			} else {
				if p.cdataElem != "" {
					break
				}
				j = n
			}
		}
		if i < j {
			text := rawdata[i:j]
			if p.convertCharrefs && p.escapable {
				text = html.Unescape(text)
			}
			if err := p.callHandler(self, "handle_data", py.Tuple{py.String(text)}); err != nil {
				return err
			}
		}
		i = j
		if i == n {
			break
		}
	startToken:
		switch {
		case strings.HasPrefix(rawdata[i:], "<"):
			var k int
			var err error
			switch {
			case starttagopen(rawdata, i):
				k, err = p.parseStarttag(self, i)
				if err != nil {
					return err
				}
			case strings.HasPrefix(rawdata[i:], "</"):
				k, err = p.parseEndtag(self, i)
				if err != nil {
					return err
				}
			case strings.HasPrefix(rawdata[i:], "<!--"):
				k, err = p.parseComment(self, i)
				if err != nil {
					return err
				}
			case strings.HasPrefix(rawdata[i:], "<?"):
				k, err = p.parsePI(self, i)
				if err != nil {
					return err
				}
			case strings.HasPrefix(rawdata[i:], "<!"):
				k, err = p.parseDeclaration(self, i)
				if err != nil {
					return err
				}
			case i+1 < n || end:
				if err := p.callHandler(self, "handle_data", py.Tuple{py.String("<")}); err != nil {
					return err
				}
				k = i + 1
			default:
				k = -1
			}
			if k < 0 {
				if !end {
					break
				}
				if err := p.handleIncomplete(self, rawdata, i, n); err != nil {
					return err
				}
				k = n
			}
			i = k
		case strings.HasPrefix(rawdata[i:], "&#"):
			if name, k, ok := matchCharref(rawdata, i); ok {
				if err := p.callHandler(self, "handle_charref", py.Tuple{py.String(name)}); err != nil {
					return err
				}
				i = k
				continue
			}
			if incompleteCharref(rawdata, i) {
				if end {
					if err := p.callHandler(self, "handle_charref", py.Tuple{py.String(rawdata[i+2:])}); err != nil {
						return err
					}
					i = n
					break
				}
				// incomplete: stop and wait for more data.
				break startToken
			}
			if i+3 < n {
				// Not the end of the buffer, and can't be confused with some
				// other construct.
				if err := p.callHandler(self, "handle_data", py.Tuple{py.String("&#")}); err != nil {
					return err
				}
				i = i + 2
			} else {
				break startToken
			}
		case strings.HasPrefix(rawdata[i:], "&"):
			if name, k, ok := matchEntityref(rawdata, i); ok {
				if err := p.callHandler(self, "handle_entityref", py.Tuple{py.String(name)}); err != nil {
					return err
				}
				i = k
				continue
			}
			if incompleteEntityref(rawdata, i) {
				if end {
					if err := p.callHandler(self, "handle_entityref", py.Tuple{py.String(rawdata[i+1:])}); err != nil {
						return err
					}
					i = n
					break
				}
				break startToken
			}
			if i+1 < n {
				if err := p.callHandler(self, "handle_data", py.Tuple{py.String("&")}); err != nil {
					return err
				}
				i = i + 1
			} else {
				break startToken
			}
		default:
			// interesting.search() cannot have found anything else.
			return py.ExceptionNewf(py.AssertionError, "unexpected character in goahead")
		}
	}
	if end && i < n {
		text := rawdata[i:n]
		if p.convertCharrefs && p.escapable {
			text = html.Unescape(text)
		}
		if err := p.callHandler(self, "handle_data", py.Tuple{py.String(text)}); err != nil {
			return err
		}
		i = n
	}
	p.rawdata = rawdata[i:]
	return nil
}

// handleIncomplete reports the constructs that are only closed at the very end
// of the document, mirroring the k < 0 branch of goahead.
func (p *HTMLParser) handleIncomplete(self py.Object, rawdata string, i, n int) error {
	switch {
	case starttagopen(rawdata, i):
		return nil
	case strings.HasPrefix(rawdata[i:], "</"):
		switch {
		case i+2 == n:
			return p.callHandler(self, "handle_data", py.Tuple{py.String("</")})
		case endtagopen(rawdata, i):
			return nil
		default:
			return p.callHandler(self, "handle_comment", py.Tuple{py.String(rawdata[i+2:])})
		}
	case strings.HasPrefix(rawdata[i:], "<!--"):
		j := n
		for _, suffix := range []string{"--!", "--", "-"} {
			if strings.HasSuffix(rawdata[i+4:j], suffix) {
				j -= len(suffix)
				break
			}
		}
		return p.callHandler(self, "handle_comment", py.Tuple{py.String(rawdata[i+4 : j])})
	case strings.HasPrefix(rawdata[i:], "<![CDATA[") && p.supportCDAT:
		return p.callHandler(self, "unknown_decl", py.Tuple{py.String(rawdata[i+3:])})
	case i+9 <= n && strings.EqualFold(rawdata[i:i+9], "<!doctype"):
		return p.callHandler(self, "handle_decl", py.Tuple{py.String(rawdata[i+2:])})
	case strings.HasPrefix(rawdata[i:], "<!"):
		return p.callHandler(self, "handle_comment", py.Tuple{py.String(rawdata[i+2:])})
	case strings.HasPrefix(rawdata[i:], "<?"):
		return p.callHandler(self, "handle_pi", py.Tuple{py.String(rawdata[i+2:])})
	}
	return nil
}

// callHandler looks up a handler on the instance and calls it, so that a
// Python subclass's override runs.
func (p *HTMLParser) callHandler(self py.Object, name string, args py.Tuple) error {
	_, err := callHandler(self, name, args)
	return err
}

// callHandler resolves name on self (as a bound method) and calls it.
func callHandler(self py.Object, name string, args py.Tuple) (py.Object, error) {
	fn, err := py.GetAttrString(self, name)
	if err != nil {
		return nil, err
	}
	return py.Call(fn, args, py.StringDict{})
}

// ---------------------------------------------------------------------------
// tag scanning

// parseStarttag is HTMLParser.parse_starttag.
func (p *HTMLParser) parseStarttag(self py.Object, i int) (int, error) {
	p.starttagText = py.None
	endpos := p.checkForWholeStartTag(i)
	if endpos < 0 {
		return endpos, nil
	}
	rawdata := p.rawdata
	p.starttagText = py.String(rawdata[i:endpos])

	k, tag := tagfindTolerant(rawdata, i+1)
	if k < 0 {
		return -1, py.ExceptionNewf(py.AssertionError, "unexpected call to parse_starttag()")
	}
	p.lasttag = tag
	var attrs []attr
	for k < endpos {
		next, a, ok := attrfindTolerant(rawdata, k)
		if !ok {
			break
		}
		attrs = append(attrs, a)
		k = next
	}
	end := strings.TrimSpace(rawdata[k:endpos])
	if end != ">" && end != "/>" {
		if err := p.callHandler(self, "handle_data", py.Tuple{py.String(rawdata[i:endpos])}); err != nil {
			return -1, err
		}
		return endpos, nil
	}
	// attrs is a list, exactly as CPython's parse_starttag builds it.
	attrPairs := py.NewList()
	for _, a := range attrs {
		var val py.Object = py.None
		if a.hasValue {
			val = py.String(a.value)
		}
		attrPairs.Append(py.Tuple{py.String(a.name), val})
	}
	if strings.HasSuffix(end, "/>") {
		// XHTML-style empty tag: <span attr="value" />
		if err := p.callHandler(self, "handle_startendtag", py.Tuple{py.String(tag), attrPairs}); err != nil {
			return -1, err
		}
	} else {
		if err := p.callHandler(self, "handle_starttag", py.Tuple{py.String(tag), attrPairs}); err != nil {
			return -1, err
		}
		if tag == "script" || tag == "style" || (p.scripting && tag == "noscript") || tag == "plaintext" {
			p.setCdataMode(tag, false)
		} else if tag == "title" || tag == "textarea" {
			p.setCdataMode(tag, true)
		}
	}
	return endpos, nil
}

// checkForWholeStartTag is HTMLParser.check_for_whole_start_tag.
func (p *HTMLParser) checkForWholeStartTag(i int) int {
	j, ok := locateTagEnd(p.rawdata, i+1)
	if !ok {
		return -1
	}
	if j < 1 || p.rawdata[j-1] != '>' {
		return -1
	}
	return j
}

// parseEndtag is HTMLParser.parse_endtag.
func (p *HTMLParser) parseEndtag(self py.Object, i int) (int, error) {
	rawdata := p.rawdata
	if strings.IndexByte(rawdata[i+2:], '>') < 0 {
		return -1, nil
	}
	if !endtagopen(rawdata, i) {
		if i+2 < len(rawdata) && rawdata[i+2] == '>' {
			// "</>" is ignored.
			return i + 3, nil
		}
		return p.parseBogusComment(self, i, true)
	}
	j, ok := locateTagEnd(rawdata, i+2)
	if !ok || j < 1 || rawdata[j-1] != '>' {
		return -1, nil
	}
	_, tag := tagfindTolerant(rawdata, i+2)
	if tag == "" {
		return -1, nil
	}
	if err := p.callHandler(self, "handle_endtag", py.Tuple{py.String(tag)}); err != nil {
		return -1, err
	}
	p.clearCdataMode()
	return j, nil
}

// setCdataMode and clearCdataMode mirror the parser's CDATA bookkeeping; the
// "interesting" regex of CPython is recomputed from these fields by
// findInteresting.
func (p *HTMLParser) setCdataMode(elem string, escapable bool) {
	p.cdataElem = strings.ToLower(elem)
	p.escapable = escapable
}

func (p *HTMLParser) clearCdataMode() {
	p.cdataElem = ""
	p.escapable = true
}

// ---------------------------------------------------------------------------
// low-level scanners, one per CPython regex

// starttagopen is '<[a-zA-Z]'.
func starttagopen(s string, i int) bool {
	return i+1 < len(s) && isASCIILetter(s[i+1])
}

// endtagopen is '</[a-zA-Z]'.
func endtagopen(s string, i int) bool {
	return i+2 < len(s) && isASCIILetter(s[i+2])
}

// isNameStop is the [\t\n\r\f />] class used by both tagfind and locatetagend.
func isNameStop(c byte) bool {
	switch c {
	case '\t', '\n', '\r', '\f', ' ', '/', '>':
		return true
	}
	return false
}

// tagfindTolerant matches tagfind_tolerant at k:
//
//	([a-zA-Z][^\t\n\r\f />]*)(?:[\t\n\r\f ]|/(?!>))*
//
// It returns the offset past the whole match and the lowercased tag name.
func tagfindTolerant(s string, k int) (int, string) {
	if k >= len(s) || !isASCIILetter(s[k]) {
		return -1, ""
	}
	j := k + 1
	for j < len(s) && !isNameStop(s[j]) {
		j++
	}
	tag := strings.ToLower(s[k:j])
	// (?:[\t\n\r\f ]|/(?!>))*
	for j < len(s) {
		c := s[j]
		if c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == ' ' {
			j++
			continue
		}
		if c == '/' && (j+1 >= len(s) || s[j+1] != '>') {
			j++
			continue
		}
		break
	}
	return j, tag
}

// attr is one parsed attribute.
type attr struct {
	name     string
	value    string
	hasValue bool
}

// attrfindTolerant matches attrfind_tolerant at k.  It returns the offset past
// the match, the parsed attribute, and whether it matched at all.
func attrfindTolerant(s string, k int) (int, attr, bool) {
	n := len(s)
	// The attribute-name group has a lookbehind (?<=['"\t\n\r\f /]) on the
	// character before k.
	if k == 0 || !isAttrLookbehind(s[k-1]) {
		return k, attr{}, false
	}
	if k >= n || isNameStop(s[k]) {
		return k, attr{}, false
	}
	j := k + 1
	for j < n && !isAttrNameStop(s[j]) {
		j++
	}
	name := strings.ToLower(s[k:j])
	a := attr{name: name}
	// ([\t\n\r\f ]*=[\t\n\r\f ]*(...))?
	p := skipAttrWS(s, j, n)
	if p < n && s[p] == '=' {
		p++
		p = skipAttrWS(s, p, n)
		valStart := p
		var val string
		if p < n && (s[p] == '\'' || s[p] == '"') {
			q := s[p]
			qe := strings.IndexByte(s[p+1:], q)
			if qe < 0 {
				// '[^']*' can match to the end without a closing quote.
				val = s[p+1:]
				p = n
			} else {
				val = s[p+1 : p+1+qe]
				p = p + 1 + qe + 1
			}
			a.hasValue = true
		} else {
			// bare value: (?!['"])[^>\t\n\r\f ]*
			ve := p
			for ve < n && !isBareValueStop(s[ve]) {
				ve++
			}
			val = s[valStart:ve]
			p = ve
			a.hasValue = true
		}
		if a.hasValue && val != "" {
			val = unescapeAttrvalue(val)
		}
		a.value = val
	}
	k = p
	// (?:[\t\n\r\f ]|/(?!>))*
	for k < n {
		c := s[k]
		if c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == ' ' {
			k++
			continue
		}
		if c == '/' && (k+1 >= n || s[k+1] != '>') {
			k++
			continue
		}
		break
	}
	return k, a, true
}

// locateTagEnd matches locatetagend at k, returning the offset past the match
// and whether it matched.  The caller checks that the last character is '>'.
func locateTagEnd(s string, k int) (int, bool) {
	n := len(s)
	if k >= n || !isASCIILetter(s[k]) {
		return 0, false
	}
	j := k + 1
	for j < n && !isNameStop(s[j]) {
		j++
	}
	// [\t\n\r\f /]*
	j = skipSpaceSlash(s, j, n)
	for {
		// (?:(?<=['"\t\n\r\f /])[^\t\n\r\f />][^\t\n\r\f /=>]*
		//   (?:...)? [\t\n\r\f /]* )*
		if j == 0 || j >= n || !isAttrLookbehind(s[j-1]) || isNameStop(s[j]) {
			break
		}
		aj := j + 1
		for aj < n && !isAttrNameStop(s[aj]) {
			aj++
		}
		p := skipAttrWS(s, aj, n)
		if p < n && s[p] == '=' {
			p++
			p = skipAttrWS(s, p, n)
			p = skipLocateValue(s, p, n)
		}
		j = skipSpaceSlash(s, p, n)
	}
	// >?
	if j < n && s[j] == '>' {
		j++
	}
	return j, true
}

// skipLocateValue skips the value alternative of locatetagend.
func skipLocateValue(s string, p, n int) int {
	if p < n && (s[p] == '\'' || s[p] == '"') {
		q := s[p]
		if qe := strings.IndexByte(s[p+1:], q); qe >= 0 {
			return p + 1 + qe + 1
		}
		return n
	}
	for p < n && !isBareValueStop(s[p]) {
		p++
	}
	return p
}

// skipAttrWS is the [\t\n\r\f ]* leading a value indicator.
func skipAttrWS(s string, k, n int) int {
	for k < n {
		switch s[k] {
		case '\t', '\n', '\r', '\f', ' ':
			k++
		default:
			return k
		}
	}
	return k
}

// skipSpaceSlash is the [\t\n\r\f /]* separators of locatetagend.
func skipSpaceSlash(s string, k, n int) int {
	for k < n {
		switch s[k] {
		case '\t', '\n', '\r', '\f', ' ', '/':
			k++
		default:
			return k
		}
	}
	return k
}

// parseComment is HTMLParser.parse_comment.
func (p *HTMLParser) parseComment(self py.Object, i int) (int, error) {
	rawdata := p.rawdata
	// An empty comment is abruptly closed by the first ">" or "->", taking
	// priority over a later "-->" or "--!>" close.
	end, inComment, ok := matchCommentAbruptClose(rawdata, i+4)
	if !ok {
		end, inComment, ok = matchCommentClose(rawdata, i+4)
		if !ok {
			return -1, nil
		}
	}
	if err := p.callHandler(self, "handle_comment", py.Tuple{py.String(rawdata[i+4 : inComment])}); err != nil {
		return -1, err
	}
	return end, nil
}

// matchCommentAbruptClose is commentabruptclose (-?>) at k: a '>' optionally
// preceded by a '-'.  It runs first, so an empty comment "<!-->" closes there.
func matchCommentAbruptClose(s string, k int) (end, inComment int, ok bool) {
	if k < len(s) && s[k] == '>' {
		return k + 1, k, true
	}
	if k+1 < len(s) && s[k] == '-' && s[k+1] == '>' {
		return k + 2, k, true
	}
	return 0, 0, false
}

// matchCommentClose is commentclose (--!?>) searched from k.
func matchCommentClose(s string, k int) (end, inComment int, ok bool) {
	for i := k; i+1 < len(s); i++ {
		if s[i] != '-' || s[i+1] != '-' {
			continue
		}
		j := i + 2
		if j < len(s) && s[j] == '!' {
			j++
		}
		if j < len(s) && s[j] == '>' {
			return j + 1, i, true
		}
	}
	return 0, 0, false
}

// parseBogusComment is HTMLParser.parse_bogus_comment.
func (p *HTMLParser) parseBogusComment(self py.Object, i int, report bool) (int, error) {
	rawdata := p.rawdata
	pos := strings.IndexByte(rawdata[i+2:], '>')
	if pos < 0 {
		return -1, nil
	}
	pos += i + 2
	if report {
		if err := p.callHandler(self, "handle_comment", py.Tuple{py.String(rawdata[i+2 : pos])}); err != nil {
			return -1, err
		}
	}
	return pos + 1, nil
}

// parsePI is HTMLParser.parse_pi.
func (p *HTMLParser) parsePI(self py.Object, i int) (int, error) {
	rawdata := p.rawdata
	j := strings.IndexByte(rawdata[i+2:], '>')
	if j < 0 {
		return -1, nil
	}
	j += i + 2
	if err := p.callHandler(self, "handle_pi", py.Tuple{py.String(rawdata[i+2 : j])}); err != nil {
		return -1, err
	}
	return j + 1, nil
}

// parseDeclaration is HTMLParser.parse_html_declaration.
func (p *HTMLParser) parseDeclaration(self py.Object, i int) (int, error) {
	rawdata := p.rawdata
	n := len(rawdata)
	switch {
	case strings.HasPrefix(rawdata[i:], "<!--"):
		return p.parseComment(self, i)
	case strings.HasPrefix(rawdata[i:], "<![CDATA[") && p.supportCDAT:
		idx := strings.Index(rawdata[i+9:], "]]>")
		if idx < 0 {
			return -1, nil
		}
		j := i + 9 + idx
		if err := p.callHandler(self, "unknown_decl", py.Tuple{py.String(rawdata[i+3 : j])}); err != nil {
			return -1, err
		}
		return j + 3, nil
	case i+9 <= n && strings.EqualFold(rawdata[i:i+9], "<!doctype"):
		gt := strings.IndexByte(rawdata[i+9:], '>')
		if gt < 0 {
			return -1, nil
		}
		gt += i + 9
		if err := p.callHandler(self, "handle_decl", py.Tuple{py.String(rawdata[i+2 : gt])}); err != nil {
			return -1, err
		}
		return gt + 1, nil
	}
	return p.parseBogusComment(self, i, true)
}

// ---------------------------------------------------------------------------
// character reference matching

// matchCharref matches charref (&#(?:[0-9]+|[xX][0-9a-fA-F]+)[^0-9a-fA-F]) at
// i.  It returns the name (the digits, without the "&#" or the terminator) and
// the offset past the reference, leaving a terminator that is not ';' for the
// scan to re-read.
func matchCharref(s string, i int) (name string, end int, ok bool) {
	j := i + 2
	hex := false
	if j < len(s) && (s[j] == 'x' || s[j] == 'X') {
		hex = true
		j++
	}
	ds := j
	if hex {
		for j < len(s) && isHexDigitForRef(s[j], true) {
			j++
		}
	} else {
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
	}
	if j == ds {
		return "", 0, false
	}
	// The required trailing [^0-9a-fA-F].
	if j >= len(s) {
		return "", 0, false
	}
	term := s[j]
	end = j + 1
	name = s[ds:j]
	if term != ';' {
		// The terminator was only a lookahead; do not consume it.
		end = j
	}
	return name, end, true
}

// incompleteCharref is incomplete_charref (&#(?:[0-9]|[xX][0-9a-fA-F])).
func incompleteCharref(s string, i int) bool {
	j := i + 2
	if j >= len(s) {
		return false
	}
	if s[j] >= '0' && s[j] <= '9' {
		return true
	}
	if (s[j] == 'x' || s[j] == 'X') && j+1 < len(s) && isHexDigitForRef(s[j+1], true) {
		return true
	}
	return false
}

// matchEntityref matches entityref (&([a-zA-Z][-.a-zA-Z0-9]*)[^a-zA-Z0-9]) at i.
func matchEntityref(s string, i int) (name string, end int, ok bool) {
	j := i + 1
	if j >= len(s) || !isASCIILetter(s[j]) {
		return "", 0, false
	}
	j++
	for j < len(s) {
		c := s[j]
		if isASCIILetter(c) || c >= '0' && c <= '9' || c == '-' || c == '.' {
			j++
			continue
		}
		break
	}
	// The required trailing [^a-zA-Z0-9].
	if j >= len(s) {
		return "", 0, false
	}
	term := s[j]
	end = j + 1
	name = s[i+1 : j]
	if term != ';' {
		end = j
	}
	return name, end, true
}

// incompleteEntityref is incomplete (&[a-zA-Z#]).
func incompleteEntityref(s string, i int) bool {
	return i+1 < len(s) && (isASCIILetter(s[i+1]) || s[i+1] == '#')
}

// findInteresting locates the next '<' or '&' from i, honoring the CDATA mode's
// own pattern (CPython's self.interesting).
func findInteresting(s string, i int, cdataElem string, convertCharrefs, escapable bool) int {
	if cdataElem == "" {
		for k := i; k < len(s); k++ {
			if s[k] == '<' || s[k] == '&' {
				return k
			}
		}
		return -1
	}
	if cdataElem == "plaintext" {
		return -1
	}
	lower := strings.ToLower(s)
	needle := "</" + cdataElem
	ampInteresting := escapable && !convertCharrefs
	for k := i; k < len(s); k++ {
		if ampInteresting && s[k] == '&' {
			return k
		}
		if s[k] != '<' {
			continue
		}
		if strings.HasPrefix(lower[k:], needle) {
			after := k + len(needle)
			if after >= len(s) || isNameStop(s[after]) {
				return k
			}
		}
	}
	return -1
}

// hasCharrefTerminator is the ([\t\n\r\f ;]) lookahead that decides whether the
// tail of the buffer can already be handed to handle_data.
func hasCharrefTerminator(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\t', '\n', '\r', '\f', ' ', ';':
			return true
		}
	}
	return false
}

// unescapeAttrvalue is parser._unescape_attrvalue; it applies attr_charref
// (&(#[0-9]+|#[xX][0-9a-fA-F]+|[a-zA-Z][a-zA-Z0-9]*)[;=]?) to an attribute
// value.
func unescapeAttrvalue(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i + 1
		if j < len(s) && s[j] == '#' {
			j++
			hex := false
			if j < len(s) && (s[j] == 'x' || s[j] == 'X') {
				hex = true
				j++
			}
			ds := j
			for j < len(s) && isHexDigitForRef(s[j], hex) {
				j++
			}
			if j == ds {
				b.WriteByte('&')
				i++
				continue
			}
			end := j
			if j < len(s) && (s[j] == ';' || s[j] == '=') {
				end++
			}
			num := html.ParseRefInt(s[ds:j], hex)
			if num < 0 {
				b.WriteString(s[i:end])
				i = end
				continue
			}
			b.WriteString(html.CharrefReplacement(num))
			i = end
			continue
		}
		if j < len(s) && isASCIILetter(s[j]) {
			ds := j
			j++
			for j < len(s) && (isASCIILetter(s[j]) || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			name := s[ds:j]
			end := j
			if j < len(s) && (s[j] == ';' || s[j] == '=') {
				end++
			}
			if repl, ok := entityReplacement(name); ok {
				b.WriteString(repl)
				i = end
				continue
			}
		}
		b.WriteByte('&')
		i++
	}
	return b.String()
}

// entityReplacement looks up a name in html.entities.html5, with or without a
// semicolon, as attr_charref allows an optional trailing ';' or '='.
func entityReplacement(name string) (string, bool) {
	table := html.HTML5Table()
	if v, ok := table[name+";"]; ok {
		return v, true
	}
	if v, ok := table[name]; ok {
		return v, true
	}
	return "", false
}

func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isAttrLookbehind(c byte) bool {
	switch c {
	case '\'', '"', '\t', '\n', '\r', '\f', ' ', '/':
		return true
	}
	return false
}

func isAttrNameStop(c byte) bool {
	switch c {
	case '\t', '\n', '\r', '\f', ' ', '/', '=', '>':
		return true
	}
	return false
}

func isBareValueStop(c byte) bool {
	switch c {
	case '\t', '\n', '\r', '\f', ' ', '>':
		return true
	}
	return false
}

func isHexDigitForRef(c byte, hex bool) bool {
	if c >= '0' && c <= '9' {
		return true
	}
	if hex && (c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
		return true
	}
	return false
}

func toInt(o py.Object) (int, error) {
	switch v := o.(type) {
	case py.Int:
		return int(v), nil
	case py.Bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case *py.BigInt:
		return v.GoInt()
	}
	return 0, py.ExceptionNewf(py.TypeError, "an integer is required")
}
