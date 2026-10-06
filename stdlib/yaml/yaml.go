// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package yaml provides the implementation of python's 'yaml' module, the
// way PyYAML presents it.
//
// PyYAML is a pure Python package, but it cannot run here: it is large, it
// uses the walrus operator and a metaclass-based multi-constructor registry,
// and this interpreter has neither.  This is therefore a native port of the
// part that reads and writes ordinary configuration files.
//
// What is implemented: block mappings, block sequences, plain and quoted
// scalars, multi-line scalars (| and >), comments, documents separated by
// ---, flow collections ([a, b] and {a: b}), and the implicit type
// resolution of plain scalars (int, float, bool, null).
//
// What is NOT implemented, and raises rather than guessing: anchors and
// aliases (& and *), explicit tags (!!python/object), and multiple documents
// beyond the first.  Those are called out in the error message so the
// limitation is visible at the point of use rather than producing a wrong
// value.
package yaml

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `YAML is a data serialization format designed for human readability
and interaction with scripting languages.

This module provides a native implementation of the block-style subset;
anchors, aliases and explicit tags are not supported.`

var YAMLErrorType = py.ExceptionType.NewType("yaml.YAMLError", "Base class for YAML errors.", nil, nil)

var MarkedYAMLErrorType = py.ExceptionType.NewType("yaml.MarkedYAMLError", "A YAML error carrying the position where it was found.", nil, nil)

// scanner holds the lines of the document and the reading position.
type document struct {
	lines []string
	pos   int
}

func newDocument(text string) *document {
	// Normalise line endings; YAML treats CRLF and CR as line breaks.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return &document{lines: strings.Split(text, "\n")}
}

func init() {
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "safe_load", Value: py.MustNewMethod("safe_load", safeLoad, 0, "Parse the first YAML document in a stream and produce the corresponding Python object.")},
		py.DictEntry{Key: "safe_load_all", Value: py.MustNewMethod("safe_load_all", safeLoadAll, 0, "Parse all YAML documents in a stream.")},
		py.DictEntry{Key: "load", Value: py.MustNewMethod("load", load, 0, "Parse the first YAML document in a stream and produce the corresponding Python object.")},
		py.DictEntry{Key: "load_all", Value: py.MustNewMethod("load_all", safeLoadAll, 0, "Parse all YAML documents in a stream.")},
		py.DictEntry{Key: "safe_dump", Value: py.MustNewMethod("safe_dump", safeDump, 0, "Serialize a Python object into a YAML stream.")},
		py.DictEntry{Key: "dump", Value: py.MustNewMethod("dump", dump, 0, "Serialize a Python object into a YAML stream.")},
		py.DictEntry{Key: "safe_dump_all", Value: py.MustNewMethod("safe_dump_all", safeDumpAll, 0, "Serialize a sequence of Python objects into a YAML stream.")},
		py.DictEntry{Key: "dump_all", Value: py.MustNewMethod("dump_all", safeDumpAll, 0, "Serialize a sequence of Python objects into a YAML stream.")},
		py.DictEntry{Key: "add_constructor", Value: py.MustNewMethod("add_constructor", noop, 0, "Register a constructor (not supported).")},
		py.DictEntry{Key: "add_representer", Value: py.MustNewMethod("add_representer", noop, 0, "Register a representer (not supported).")},
		py.DictEntry{Key: "YAMLError", Value: YAMLErrorType},
		py.DictEntry{Key: "MarkedYAMLError", Value: MarkedYAMLErrorType},
		py.DictEntry{Key: "SafeLoader", Value: SafeLoaderType},
		py.DictEntry{Key: "Loader", Value: SafeLoaderType},
		py.DictEntry{Key: "FullLoader", Value: SafeLoaderType},
		py.DictEntry{Key: "UnsafeLoader", Value: SafeLoaderType},
		py.DictEntry{Key: "BaseLoader", Value: SafeLoaderType},
		py.DictEntry{Key: "SafeDumper", Value: SafeDumperType},
		py.DictEntry{Key: "Dumper", Value: SafeDumperType},
	)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "yaml",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

func noop(self py.Object, args py.Tuple) (py.Object, error) { return py.None, nil }

// SafeLoader and its aliases are accepted so that the "Loader=yaml.SafeLoader"
// form, which is how real code calls load(), is not a TypeError.  There is
// one loader here, and it is always safe: it never constructs arbitrary
// Python objects from tags.
var SafeLoaderType = py.NewType("yaml.SafeLoader", "The safe YAML loader.")

var SafeDumperType = py.NewType("yaml.SafeDumper", "The safe YAML dumper.")

func init() {
	SafeLoaderType.Flags |= py.TPFLAGS_BASETYPE
	SafeDumperType.Flags |= py.TPFLAGS_BASETYPE
}

// readText accepts a string or a stream and returns its contents.
func readText(arg py.Object) (string, error) {
	if s, ok := arg.(py.String); ok {
		return string(s), nil
	}
	if b, ok := arg.(py.Bytes); ok {
		return string(b), nil
	}
	// A stream: read() it.
	read, err := py.GetAttrString(arg, "read")
	if err != nil {
		return "", py.ExceptionNewf(py.TypeError, "a string or a stream is required, not %s", arg.Type().Name)
	}
	res, err := py.Call(read, py.Tuple{}, py.StringDict{})
	if err != nil {
		return "", err
	}
	return readText(res)
}

func safeLoad(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	docs, err := loadDocs(args, kwargs)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return py.None, nil
	}
	return docs[0], nil
}

func load(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return safeLoad(self, args, kwargs)
}

func safeLoadAll(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	docs, err := loadDocs(args, kwargs)
	if err != nil {
		return nil, err
	}
	return py.NewListFromItems(docs), nil
}

// loadDocs parses every document in the argument.
func loadDocs(args py.Tuple, kwargs py.StringDict) ([]py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "load() missing required argument 'stream'")
	}
	text, err := readText(args[0])
	if err != nil {
		return nil, err
	}
	return parseDocuments(text)
}

// parseDocuments splits a stream into documents and parses each.
func parseDocuments(text string) ([]py.Object, error) {
	doc := newDocument(text)

	// Split on "---" lines, keeping the leading document.
	var chunks [][]string
	current := []string{}
	for _, line := range doc.lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "---" || strings.HasPrefix(trimmed, "--- ") {
			chunks = append(chunks, current)
			rest := strings.TrimPrefix(trimmed, "---")
			current = []string{}
			if strings.TrimSpace(rest) != "" {
				current = append(current, strings.TrimSpace(rest))
			}
			continue
		}
		if trimmed == "..." {
			chunks = append(chunks, current)
			current = []string{}
			continue
		}
		current = append(current, line)
	}
	chunks = append(chunks, current)

	out := []py.Object{}
	for _, chunk := range chunks {
		if onlyBlankOrComment(chunk) {
			continue
		}
		d := &document{lines: chunk}
		value, err := d.parseBlock(0)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return out, nil
	}
	return out, nil
}

func onlyBlankOrComment(lines []string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return false
		}
	}
	return true
}

// indentOf returns the number of leading spaces, and the content.
func indentOf(line string) (int, string) {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	return n, line[n:]
}

// isSkippable reports whether a line is blank or a comment.
func isSkippable(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

// parseBlock reads a block at the given indentation.
func (d *document) parseBlock(indent int) (py.Object, error) {
	d.skipSkippable()
	if d.pos >= len(d.lines) {
		return py.None, nil
	}
	curIndent, content := indentOf(d.lines[d.pos])

	// An explicit document start or end does not belong to the block.
	if content == "---" || content == "..." {
		return py.None, nil
	}
	if curIndent < indent {
		return py.None, nil
	}

	// A sequence is introduced by "- " at this indentation level.
	if content == "-" || strings.HasPrefix(content, "- ") {
		return d.parseSequence(curIndent)
	}
	// A document that is a single scalar - "hello", or "42" - is a value in
	// its own right, not a mapping key awaiting a colon.
	if _, _, err := splitKey(content); err != nil {
		d.pos++
		return d.parseInlineValue(content, curIndent)
	}
	return d.parseMapping(curIndent)
}

func (d *document) skipSkippable() {
	for d.pos < len(d.lines) && isSkippable(d.lines[d.pos]) {
		d.pos++
	}
}

func (d *document) parseMapping(indent int) (py.Object, error) {
	result := py.NewStringDict()
	for d.pos < len(d.lines) {
		if isSkippable(d.lines[d.pos]) {
			d.pos++
			continue
		}
		curIndent, content := indentOf(d.lines[d.pos])
		if curIndent < indent || content == "---" || content == "..." {
			break
		}
		if curIndent > indent {
			// Belongs to a nested value that the caller should have read.
			break
		}
		if content == "-" || strings.HasPrefix(content, "- ") {
			break
		}

		key, rest, err := splitKey(content)
		if err != nil {
			return nil, d.errorf("could not find expected ':'")
		}
		d.pos++

		var value py.Object
		if strings.TrimSpace(rest) == "" {
			// The value is the block below, or nothing.
			value, err = d.parseValueBlock(indent)
			if err != nil {
				return nil, err
			}
		} else {
			value, err = d.parseInlineValue(strings.TrimSpace(rest), indent)
			if err != nil {
				return nil, err
			}
		}
		keyObj, err := resolveScalar(key)
		if err != nil {
			return nil, err
		}
		// Dict keys go through setitem so the encoding matches a lookup.
		if _, err := result.M__setitem__(keyObj, value); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// parseValueBlock reads the value that follows a bare "key:" - either the
// nested block, or nothing (None).
func (d *document) parseValueBlock(parentIndent int) (py.Object, error) {
	// Look ahead: a more-indented line, or a sequence at the same indent
	// (which YAML allows for a sequence value), is the value.
	save := d.pos
	d.skipSkippable()
	if d.pos >= len(d.lines) {
		d.pos = save
		return py.None, nil
	}
	curIndent, content := indentOf(d.lines[d.pos])
	if curIndent > parentIndent {
		return d.parseBlock(curIndent)
	}
	if curIndent == parentIndent && (content == "-" || strings.HasPrefix(content, "- ")) {
		// A sequence may be written at the same indentation as its key.
		return d.parseSequence(curIndent)
	}
	d.pos = save
	return py.None, nil
}

func (d *document) parseSequence(indent int) (py.Object, error) {
	items := []py.Object{}
	for d.pos < len(d.lines) {
		if isSkippable(d.lines[d.pos]) {
			d.pos++
			continue
		}
		curIndent, content := indentOf(d.lines[d.pos])
		if curIndent != indent {
			break
		}
		if content != "-" && !strings.HasPrefix(content, "- ") {
			break
		}
		rest := strings.TrimSpace(strings.TrimPrefix(content, "-"))
		d.pos++

		if rest == "" {
			value, err := d.parseBlock(indent + 1)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
			continue
		}
		// "- key: value" starts a mapping whose first key is on this line.
		if key, kvRest, err := splitKey(rest); err == nil {
			mapping, err := d.parseMappingFromInlineKey(indent+2, key, kvRest)
			if err != nil {
				return nil, err
			}
			items = append(items, mapping)
			continue
		}
		value, err := d.parseInlineValue(rest, indent)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return py.NewListFromItems(items), nil
}

// parseMappingFromInlineKey handles "- key: value", where the mapping starts
// on the sequence item's own line.
func (d *document) parseMappingFromInlineKey(indent int, key, rest string) (py.Object, error) {
	result := py.NewStringDict()
	var err error
	var value py.Object
	if strings.TrimSpace(rest) == "" {
		value, err = d.parseValueBlock(indent - 2)
	} else {
		value, err = d.parseInlineValue(strings.TrimSpace(rest), indent)
	}
	if err != nil {
		return nil, err
	}
	keyObj, err := resolveScalar(key)
	if err != nil {
		return nil, err
	}
	if _, err := result.M__setitem__(keyObj, value); err != nil {
		return nil, err
	}

	// Any further keys of the same mapping are indented under the "-".
	for d.pos < len(d.lines) {
		if isSkippable(d.lines[d.pos]) {
			d.pos++
			continue
		}
		curIndent, content := indentOf(d.lines[d.pos])
		if curIndent < indent {
			break
		}
		if content == "-" || strings.HasPrefix(content, "- ") {
			break
		}
		k, r, err := splitKey(content)
		if err != nil {
			break
		}
		d.pos++
		var v py.Object
		if strings.TrimSpace(r) == "" {
			v, err = d.parseValueBlock(curIndent)
		} else {
			v, err = d.parseInlineValue(strings.TrimSpace(r), curIndent)
		}
		if err != nil {
			return nil, err
		}
		kObj, err := resolveScalar(k)
		if err != nil {
			return nil, err
		}
		if _, err := result.M__setitem__(kObj, v); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// splitKey splits "key: value" at the first colon that is a key separator:
// one that is followed by a space or the end of the line, and that is not
// inside quotes.
func splitKey(s string) (key, value string, err error) {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case ':':
			if i+1 >= len(s) || s[i+1] == ' ' {
				return strings.TrimSpace(s[:i]), s[i+1:], nil
			}
		}
	}
	return "", "", fmt.Errorf("no key separator")
}

// parseInlineValue reads a value written on the same line as its key.
func (d *document) parseInlineValue(text string, indent int) (py.Object, error) {
	text = stripComment(text)
	if text == "" {
		return py.None, nil
	}

	// Block scalars: | keeps newlines, > folds them.
	if text == "|" || text == ">" || strings.HasPrefix(text, "|") || strings.HasPrefix(text, ">") {
		return d.parseBlockScalar(text, indent)
	}

	// A nested collection may start on the same line.
	if strings.HasPrefix(text, "[") || strings.HasPrefix(text, "{") {
		return parseFlow(text)
	}

	// An anchor or alias is not supported: say so rather than dropping it.
	if strings.HasPrefix(text, "&") || strings.HasPrefix(text, "*") {
		return nil, py.ExceptionNewf(YAMLErrorType, "anchors and aliases are not supported by this implementation: %q", text)
	}
	if strings.HasPrefix(text, "!") {
		return nil, py.ExceptionNewf(YAMLErrorType, "explicit tags are not supported by this implementation: %q", text)
	}

	return resolveScalar(text)
}

// parseBlockScalar reads the lines that follow "key: |" or "key: >".
func (d *document) parseBlockScalar(header string, indent int) (py.Object, error) {
	style := header[0]
	// A chomping indicator (+/-) and an explicit indentation digit may
	// follow; the digit is honoured when given.
	explicit := -1
	for _, r := range header[1:] {
		switch {
		case r == '-' || r == '+':
			// Keep for the chomping below.
		case r >= '1' && r <= '9':
			explicit = int(r - '0')
		}
	}

	var contentLines []string
	blockIndent := -1
	if explicit > 0 {
		blockIndent = indent + explicit
	}
	for d.pos < len(d.lines) {
		line := d.lines[d.pos]
		if strings.TrimSpace(line) == "" {
			contentLines = append(contentLines, "")
			d.pos++
			continue
		}
		curIndent, _ := indentOf(line)
		if blockIndent < 0 {
			if curIndent <= indent {
				break
			}
			blockIndent = curIndent
		}
		if curIndent < blockIndent {
			break
		}
		contentLines = append(contentLines, line[blockIndent:])
		d.pos++
	}
	// Trailing blank lines belong to the separation, not the scalar.
	for len(contentLines) > 0 && contentLines[len(contentLines)-1] == "" {
		contentLines = contentLines[:len(contentLines)-1]
	}

	var text string
	if style == '|' {
		text = strings.Join(contentLines, "\n")
	} else {
		// Folded: single newlines become spaces, blank lines become
		// newlines, and a more-indented line keeps its own newline.
		var b strings.Builder
		for i, line := range contentLines {
			if line == "" {
				b.WriteString("\n")
				continue
			}
			if i > 0 && contentLines[i-1] != "" {
				b.WriteString(" ")
			}
			b.WriteString(line)
		}
		text = b.String()
	}
	// A literal block ends with a newline unless it was chomped.
	if !strings.Contains(header, "-") {
		text += "\n"
	}
	return py.String(text), nil
}

// stripComment removes a trailing comment that is not inside quotes.
func stripComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '#':
			if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
				return strings.TrimRight(s[:i], " \t")
			}
		}
	}
	return strings.TrimRight(s, " \t")
}

// resolveScalar turns a scalar's text into the Python value the YAML rules
// imply for a plain (unquoted) scalar.
func resolveScalar(text string) (py.Object, error) {
	text = strings.TrimSpace(text)

	// A quoted scalar is always a string, with the escapes processed.
	if len(text) >= 2 {
		if text[0] == '"' && text[len(text)-1] == '"' {
			inner := text[1 : len(text)-1]
			inner = strings.ReplaceAll(inner, `\n`, "\n")
			inner = strings.ReplaceAll(inner, `\t`, "\t")
			inner = strings.ReplaceAll(inner, `\\`, "\\")
			inner = strings.ReplaceAll(inner, `\"`, `"`)
			return py.String(inner), nil
		}
		if text[0] == '\'' && text[len(text)-1] == '\'' {
			// In single quotes, '' is an escaped single quote.
			inner := text[1 : len(text)-1]
			inner = strings.ReplaceAll(inner, "''", "'")
			return py.String(inner), nil
		}
	}

	// The null spellings.
	switch text {
	case "", "~", "null", "Null", "NULL":
		return py.None, nil
	case "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
		return py.True, nil
	case "false", "False", "FALSE", "no", "No", "NO", "off", "Off", "OFF":
		return py.False, nil
	}

	// Numbers, in the bases YAML allows.
	if n, ok := parseYAMLNumber(text); ok {
		return n, nil
	}

	return py.String(text), nil
}

// parseYAMLNumber recognises the numeric forms YAML resolves, which are wider
// than JSON's: underscores, a leading +, octal 0o, hex 0x, and .inf/.nan.
func parseYAMLNumber(text string) (py.Object, bool) {
	clean := strings.ReplaceAll(text, "_", "")
	lower := strings.ToLower(clean)

	switch lower {
	case ".inf", "+.inf":
		return py.Float(inf()), true
	case "-.inf":
		return py.Float(-inf()), true
	case ".nan":
		return py.Float(nan()), true
	}

	// An integer, possibly in a non-decimal base.
	body := clean
	negative := false
	if strings.HasPrefix(body, "+") {
		body = body[1:]
	} else if strings.HasPrefix(body, "-") {
		negative = true
		body = body[1:]
	}
	base := 10
	digits := body
	switch {
	case strings.HasPrefix(lower, "0x"):
		base, digits = 16, body[2:]
	case strings.HasPrefix(lower, "0o"):
		base, digits = 8, body[2:]
	case strings.HasPrefix(lower, "0b"):
		base, digits = 2, body[2:]
	}
	if digits != "" && isAllDigitsInBase(digits, base) {
		if n, err := strconv.ParseInt(digits, base, 64); err == nil {
			if negative {
				n = -n
			}
			return py.Int(n), true
		}
		// Too large for an int: a Python int of that magnitude.
		signed := digits
		if negative {
			signed = "-" + digits
		}
		if big, err := py.IntFromString(signed, base); err == nil {
			return big, true
		}
	}

	// A float.  A leading sign and a decimal point or exponent are required
	// to make it a float rather than a string.
	if strings.ContainsAny(clean, ".eE") {
		if f, err := strconv.ParseFloat(clean, 64); err == nil {
			return py.Float(f), true
		}
	}
	return nil, false
}

func isAllDigitsInBase(s string, base int) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return false
		}
		if d >= base {
			return false
		}
	}
	return true
}

func inf() float64 { return math.Inf(1) }

func nan() float64 { return math.NaN() }

// parseFlow reads a flow collection: [a, b] or {a: b}.
func parseFlow(text string) (py.Object, error) {
	p := &flowParser{src: text}
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos < len(p.src) {
		return nil, py.ExceptionNewf(YAMLErrorType, "unexpected trailing text in flow collection: %q", p.src[p.pos:])
	}
	return value, nil
}

type flowParser struct {
	src string
	pos int
}

func (p *flowParser) skipSpace() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *flowParser) parseValue() (py.Object, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return py.None, nil
	}
	switch p.src[p.pos] {
	case '[':
		return p.parseList()
	case '{':
		return p.parseMap()
	}
	// A plain or quoted scalar up to the next delimiter.
	start := p.pos
	var quote byte
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			p.pos++
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			p.pos++
			continue
		}
		if c == ',' || c == ']' || c == '}' {
			break
		}
		// A ':' that ends the scalar separates a key from its value in a
		// flow mapping, so the scalar stops there.
		if c == ':' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] == ' ') {
			break
		}
		p.pos++
	}
	return resolveScalar(strings.TrimSpace(p.src[start:p.pos]))
}

func (p *flowParser) parseList() (py.Object, error) {
	p.pos++ // [
	items := []py.Object{}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil, py.ExceptionNewf(YAMLErrorType, "unterminated flow sequence")
		}
		if p.src[p.pos] == ']' {
			p.pos++
			return py.NewListFromItems(items), nil
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		items = append(items, value)
		p.skipSpace()
		if p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
		}
	}
}

func (p *flowParser) parseMap() (py.Object, error) {
	p.pos++ // {
	result := py.NewStringDict()
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil, py.ExceptionNewf(YAMLErrorType, "unterminated flow mapping")
		}
		if p.src[p.pos] == '}' {
			p.pos++
			return result, nil
		}
		key, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.pos < len(p.src) && p.src[p.pos] == ':' {
			p.pos++
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		if _, err := result.M__setitem__(key, value); err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
		}
	}
}

func (d *document) errorf(format string, a ...interface{}) error {
	return py.ExceptionNewf(YAMLErrorType, "line %d: "+format, append([]interface{}{d.pos + 1}, a...)...)
}

// ---------------------------------------------------------------------------
// Emitting

func safeDump(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "dump() missing required argument 'data'")
	}
	text, err := emitValue(args[0], 0)
	if err != nil {
		return nil, err
	}

	// When a stream is given as the second argument, write to it.
	for _, a := range args[1:] {
		if _, err := py.GetAttrString(a, "write"); err == nil {
			write, _ := py.GetAttrString(a, "write")
			if _, err := py.Call(write, py.Tuple{py.String(text)}, py.StringDict{}); err != nil {
				return nil, err
			}
			return py.None, nil
		}
	}
	return py.String(text), nil
}

func dump(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return safeDump(self, args, kwargs)
}

func safeDumpAll(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "dump_all() missing required argument 'documents'")
	}
	items, err := py.SequenceList(args[0])
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for i, item := range items.Items {
		if i > 0 {
			b.WriteString("---\n")
		}
		text, err := emitValue(item, 0)
		if err != nil {
			return nil, err
		}
		b.WriteString(text)
	}
	for _, a := range args[1:] {
		if write, err := py.GetAttrString(a, "write"); err == nil {
			if _, err := py.Call(write, py.Tuple{py.String(b.String())}, py.StringDict{}); err != nil {
				return nil, err
			}
			return py.None, nil
		}
	}
	return py.String(b.String()), nil
}

// emitValue renders a value as block-style YAML.
func emitValue(obj py.Object, indent int) (string, error) {
	var b strings.Builder
	if err := emitTo(&b, obj, indent); err != nil {
		return "", err
	}
	return b.String(), nil
}

func pad(indent int) string { return strings.Repeat(" ", indent) }

// emitTo writes obj at the given indentation.
func emitTo(b *strings.Builder, obj py.Object, indent int) error {
	switch v := obj.(type) {
	case py.NoneType:
		b.WriteString("null\n")
		return nil
	case py.Bool:
		if v {
			b.WriteString("true\n")
		} else {
			b.WriteString("false\n")
		}
		return nil
	case py.Int:
		b.WriteString(strconv.FormatInt(int64(v), 10))
		b.WriteString("\n")
		return nil
	case py.Float:
		b.WriteString(formatYAMLFloat(float64(v)))
		b.WriteString("\n")
		return nil
	case py.String:
		b.WriteString(quoteIfNeeded(string(v)))
		b.WriteString("\n")
		return nil
	}

	if d, ok := obj.(py.IGetDict); ok {
		return emitMappingEntries(b, d.GetDict(), indent, false)
	}

	if items, ok := sequenceOf(obj); ok {
		if len(items) == 0 {
			b.WriteString("[]\n")
			return nil
		}
		for _, item := range items {
			b.WriteString(pad(indent))
			b.WriteString("- ")
			// A mapping item starts on the "-" line, which is how PyYAML
			// writes a list of mappings and what makes it readable.
			if d, ok := item.(py.IGetDict); ok && d.GetDict().Len() > 0 {
				if err := emitMappingEntries(b, d.GetDict(), indent+2, true); err != nil {
					return err
				}
				continue
			}
			if isBlockValue(item) {
				b.WriteString("\n")
				if err := emitTo(b, item, indent+2); err != nil {
					return err
				}
				continue
			}
			if err := emitInline(b, item); err != nil {
				return err
			}
			b.WriteString("\n")
		}
		return nil
	}

	// Anything else is written with str(), which is what PyYAML does for an
	// object it has no representer for.
	text, err := py.StrAsString(obj)
	if err != nil {
		return py.ExceptionNewf(YAMLErrorType, "cannot represent an object: %s", obj.Type().Name)
	}
	b.WriteString(quoteIfNeeded(text))
	b.WriteString("\n")
	return nil
}

// emitMappingEntries writes a dict's entries.  When afterDash is set, the
// first key continues a "- " that has already been written rather than
// starting a new line.
func emitMappingEntries(b *strings.Builder, m py.StringDict, indent int, afterDash bool) error {
	if m.Len() == 0 {
		b.WriteString("{}\n")
		return nil
	}
	// Keys go out in sorted order, which is what PyYAML does with sort_keys
	// defaulting to True, and makes the output stable.
	keys := make([]string, 0, m.Len())
	keys = append(keys, m.Keys()...)
	sort.Strings(keys)
	for i, encoded := range keys {
		key, err := py.DictKeyDecode(encoded)
		if err != nil {
			return err
		}
		keyText, err := scalarText(key)
		if err != nil {
			return err
		}
		if i > 0 || !afterDash {
			b.WriteString(pad(indent))
		}
		b.WriteString(keyText)
		b.WriteString(":")
		value := m.GetOrNil(encoded)
		if isBlockValue(value) {
			b.WriteString("\n")
			if err := emitTo(b, value, indent+2); err != nil {
				return err
			}
			continue
		}
		b.WriteString(" ")
		if err := emitInline(b, value); err != nil {
			return err
		}
		b.WriteString("\n")
	}
	return nil
}

// isBlockValue reports whether a value should be written on its own lines.
func isBlockValue(obj py.Object) bool {
	if d, ok := obj.(py.IGetDict); ok {
		return d.GetDict().Len() > 0
	}
	if items, ok := sequenceOf(obj); ok {
		return len(items) > 0
	}
	return false
}

// emitInline writes a scalar value on one line.
func emitInline(b *strings.Builder, obj py.Object) error {
	switch v := obj.(type) {
	case py.NoneType:
		b.WriteString("null")
	case py.Bool:
		if v {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case py.Int:
		b.WriteString(strconv.FormatInt(int64(v), 10))
	case py.Float:
		b.WriteString(formatYAMLFloat(float64(v)))
	case py.String:
		b.WriteString(quoteIfNeeded(string(v)))
	default:
		text, err := py.StrAsString(obj)
		if err != nil {
			return err
		}
		b.WriteString(quoteIfNeeded(text))
	}
	return nil
}

// scalarText renders a dict key.
func scalarText(obj py.Object) (string, error) {
	switch v := obj.(type) {
	case py.NoneType:
		return "null", nil
	case py.Bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case py.Int:
		return strconv.FormatInt(int64(v), 10), nil
	case py.Float:
		return formatYAMLFloat(float64(v)), nil
	case py.String:
		return quoteIfNeeded(string(v)), nil
	}
	text, err := py.StrAsString(obj)
	if err != nil {
		return "", err
	}
	return quoteIfNeeded(text), nil
}

func sequenceOf(obj py.Object) ([]py.Object, bool) {
	switch v := obj.(type) {
	case *py.List:
		return v.Items, true
	case py.Tuple:
		return []py.Object(v), true
	}
	return nil, false
}

// quoteIfNeeded quotes a string when writing it plain would change what it
// resolves to, or would not parse.
func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	// A multi-line string goes out as a literal block.
	if strings.Contains(s, "\n") {
		return "|-\n" + indentAll(s)
	}
	switch {
	case strings.HasPrefix(s, " ") || strings.HasSuffix(s, " "),
		strings.Contains(s, ": "), strings.Contains(s, " #"),
		strings.HasPrefix(s, "-"), strings.HasPrefix(s, "?"),
		strings.HasPrefix(s, ":"), strings.HasPrefix(s, "["),
		strings.HasPrefix(s, "]"), strings.HasPrefix(s, "{"),
		strings.HasPrefix(s, "}"), strings.HasPrefix(s, ","),
		strings.HasPrefix(s, "&"), strings.HasPrefix(s, "*"),
		strings.HasPrefix(s, "!"), strings.HasPrefix(s, "|"),
		strings.HasPrefix(s, ">"), strings.HasPrefix(s, "'"),
		strings.HasPrefix(s, `"`), strings.HasPrefix(s, "%"),
		strings.HasPrefix(s, "@"), strings.HasPrefix(s, "`"),
		strings.HasPrefix(s, "#"):
		return quote(s)
	}
	// Would it resolve to a non-string?
	if resolved, err := resolveScalar(s); err == nil {
		if _, isStr := resolved.(py.String); !isStr {
			return quote(s)
		}
	}
	return s
}

func quote(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	return `"` + escaped + `"`
}

func indentAll(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}

func formatYAMLFloat(f float64) string {
	switch {
	case f != f:
		return ".nan"
	case f > 1.7976931348623157e308:
		return ".inf"
	case f < -1.7976931348623157e308:
		return "-.inf"
	}
	text := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return text
}
