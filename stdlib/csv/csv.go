// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package csv provides the implementation of python's 'csv' module.
//
// reader, writer, Dialect, the two predefined dialects, the four QUOTE_*
// constants and the Sniffer are implemented.  The reader and writer are stateful
// objects that pull from and push to a Python file object, so they are Go types
// exposing the iterable and __next__ protocol rather than wrappers around a
// buffered string.
//
// Where the module differs from CPython it is because CPython's csv is a C
// extension that supports dialect subclassing in Python; here a Dialect is a
// plain object whose attributes are read by reader() and writer().
package csv

import (
	"fmt"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `CSV parsing and writing.

This module provides classes that assist in the reading and writing
of Comma Separated Value (CSV) files, and implements the interface
described by PEP 305.  Although many CSV files are simple to parse,
the format is not formally defined by a stable specification and
is subtle enough that parsing lines of a CSV file with something
like line.split(",") is bound to fail.  The module supports three
basic APIs: reading, writing, and registration of dialects.

DIALECT REGISTRATION:

Readers and writers support a dialect argument, which is a convenient
handle on a group of settings.  When the dialect argument is a string,
it identifies one of the dialects previously registered with the module.
If it is a class or instance, the attributes of the argument are used as
the settings for the reader or writer:

    class excel:
        delimiter = ','
        quotechar = '"'
        escapechar = None
        doublequote = True
        skipinitialspace = False
        lineterminator = '\r\n'
        quoting = QUOTE_MINIMAL

SETTINGS:

    * delimiter    - the character used to separate fields in a record
    * quotechar    - the character used to quote fields containing special
                     characters
    * escapechar   - the character used to escape the delimiter if quoting is
                     set to QUOTE_NONE
    * doublequote  - whether quotechar inside a field is doubled
    * skipinitialspace - whether whitespace immediately after the delimiter is
                     ignored
    * lineterminator - the string ending each record written
    * quoting      - one of the QUOTE_* constants

QUOTE MINIMAL (0), QUOTE ALL (1), QUOTE NONNUMERIC (2), QUOTE NONE (3)`

// csvError is CPython's csv.Error, which derives from Exception.
var csvError = py.ExceptionType.NewType("csv.Error", "Error in the csv module", nil, nil)

// The QUOTE_* constants, with CPython's values.
const (
	quoteMinimal    = 0
	quoteAll        = 1
	quoteNonNumeric = 2
	quoteNone       = 3
)

// dialectSpec holds the attributes every dialect provides.
type dialectSpec struct {
	delimiter        string
	quotechar        string
	escapechar       string
	doublequote      bool
	skipinitialspace bool
	lineterminator   string
	quoting          int
	hasEscapechar    bool
	hasQuotechar     bool
	strict           bool
}

// defaultDialect is CPython's excel dialect.
func defaultDialect() dialectSpec {
	return dialectSpec{
		delimiter:      ",",
		quotechar:      `"`,
		doublequote:    true,
		lineterminator: "\r\n",
		quoting:        quoteMinimal,
		hasQuotechar:   true,
	}
}

// tabDialect is CPython's excel-tab dialect.
func tabDialect() dialectSpec {
	d := defaultDialect()
	d.delimiter = "\t"
	return d
}

// ---------------------------------------------------------------------------
// Dialect class

var dialectType = py.NewTypeX("csv.Dialect",
	`CSV dialect

The Dialect type records CSV parsing and generation options.`,
	dialectNew, nil)

// Dialect is the Python-visible dialect object.
type Dialect struct {
	spec dialectSpec
}

func (d *Dialect) Type() *py.Type { return dialectType }

func dialectNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d := &Dialect{spec: defaultDialect()}
	if err := applySettings(&d.spec, nil, kwargs); err != nil {
		return nil, err
	}
	return d, nil
}

// applySettings overlays dialect attributes onto spec from a source object
// (a Dialect, or any object carrying the attributes) and then from keyword
// arguments.
func applySettings(spec *dialectSpec, source py.Object, kwargs py.StringDict) error {
	read := func(name string) (py.Object, bool) {
		if source != nil {
			if v, err := py.GetAttrString(source, name); err == nil {
				return v, true
			}
		}
		return nil, false
	}
	// delimiter
	if v, ok := read("delimiter"); ok && v != py.None {
		s, err := py.StrAsString(v)
		if err != nil {
			return py.ExceptionNewf(py.TypeError, "\"delimiter\" must be a 1-character string")
		}
		spec.delimiter = s
	}
	if v, ok := kwargs.Get("delimiter"); ok {
		s, err := py.StrAsString(v)
		if err != nil {
			return py.ExceptionNewf(py.TypeError, "\"delimiter\" must be a 1-character string")
		}
		spec.delimiter = s
	}
	// quotechar
	if v, ok := read("quotechar"); ok {
		if v == py.None {
			spec.quotechar = ""
			spec.hasQuotechar = false
		} else {
			s, err := py.StrAsString(v)
			if err != nil {
				return py.ExceptionNewf(py.TypeError, "\"quotechar\" must be a 1-character string")
			}
			spec.quotechar = s
			spec.hasQuotechar = true
		}
	}
	if v, ok := kwargs.Get("quotechar"); ok {
		if v == py.None {
			spec.quotechar = ""
			spec.hasQuotechar = false
		} else {
			s, err := py.StrAsString(v)
			if err != nil {
				return py.ExceptionNewf(py.TypeError, "\"quotechar\" must be a 1-character string")
			}
			spec.quotechar = s
			spec.hasQuotechar = true
		}
	}
	// escapechar
	if v, ok := read("escapechar"); ok {
		if v == py.None {
			spec.escapechar = ""
			spec.hasEscapechar = false
		} else {
			s, err := py.StrAsString(v)
			if err != nil {
				return py.ExceptionNewf(py.TypeError, "\"escapechar\" must be a 1-character string")
			}
			spec.escapechar = s
			spec.hasEscapechar = true
		}
	}
	if v, ok := kwargs.Get("escapechar"); ok {
		if v == py.None {
			spec.escapechar = ""
			spec.hasEscapechar = false
		} else {
			s, err := py.StrAsString(v)
			if err != nil {
				return py.ExceptionNewf(py.TypeError, "\"escapechar\" must be a 1-character string")
			}
			spec.escapechar = s
			spec.hasEscapechar = true
		}
	}
	// booleans
	boolAttr := func(name string, dst *bool) error {
		if v, ok := read(name); ok {
			b, err := py.ObjectIsTrue(v)
			if err != nil {
				return err
			}
			*dst = b
		}
		if v, ok := kwargs.Get(name); ok {
			b, err := py.ObjectIsTrue(v)
			if err != nil {
				return err
			}
			*dst = b
		}
		return nil
	}
	if err := boolAttr("doublequote", &spec.doublequote); err != nil {
		return err
	}
	if err := boolAttr("skipinitialspace", &spec.skipinitialspace); err != nil {
		return err
	}
	if err := boolAttr("strict", &spec.strict); err != nil {
		return err
	}
	// lineterminator
	if v, ok := read("lineterminator"); ok && v != py.None {
		s, err := py.StrAsString(v)
		if err != nil {
			return py.ExceptionNewf(py.TypeError, "\"lineterminator\" must be a string")
		}
		spec.lineterminator = s
	}
	if v, ok := kwargs.Get("lineterminator"); ok {
		s, err := py.StrAsString(v)
		if err != nil {
			return py.ExceptionNewf(py.TypeError, "\"lineterminator\" must be a string")
		}
		spec.lineterminator = s
	}
	// quoting
	if v, ok := read("quoting"); ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return py.ExceptionNewf(py.TypeError, "\"quoting\" must be an integer")
		}
		spec.quoting = n
	}
	if v, ok := kwargs.Get("quoting"); ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return py.ExceptionNewf(py.TypeError, "\"quoting\" must be an integer")
		}
		spec.quoting = n
	}
	if spec.quoting < quoteMinimal || spec.quoting > quoteNone {
		return py.ExceptionNewf(py.TypeError, "bad \"quoting\" value")
	}
	if !validDelim(spec.delimiter) {
		return py.ExceptionNewf(py.TypeError, "\"delimiter\" must be a 1-character string")
	}
	return nil
}

// validDelim rejects an empty or multi-byte delimiter.
func validDelim(s string) bool { return len(s) == 1 }

// ---------------------------------------------------------------------------
// Registry

// registered holds the named dialects, in the order CPython lists them.
var registered = []struct {
	name string
	spec dialectSpec
}{
	{"excel", defaultDialect()},
	{"excel-tab", tabDialect()},
	{"unix", func() dialectSpec { d := defaultDialect(); d.lineterminator = "\n"; return d }()},
}

// lookupDialect resolves a dialect argument: a Dialect instance, any object
// with the attributes, or a registered name.
func lookupDialect(arg py.Object) (dialectSpec, error) {
	if arg == nil || arg == py.None {
		return defaultDialect(), nil
	}
	if s, ok := arg.(py.String); ok {
		for _, r := range registered {
			if r.name == string(s) {
				spec := r.spec
				return spec, nil
			}
		}
		// CPython also accepts the underscore spelling of a name.
		name := strings.ReplaceAll(string(s), "_", "-")
		for _, r := range registered {
			if r.name == name {
				spec := r.spec
				return spec, nil
			}
		}
		return dialectSpec{}, py.ExceptionNewf(csvError, "unknown dialect")
	}
	if d, ok := arg.(*Dialect); ok {
		return d.spec, nil
	}
	spec := defaultDialect()
	if err := applySettings(&spec, arg, py.StringDict{}); err != nil {
		return dialectSpec{}, err
	}
	return spec, nil
}

// ---------------------------------------------------------------------------
// Writer

// Writer is a csv.writer.
type Writer struct {
	spec dialectSpec
	file py.Object
}

func (w *Writer) Type() *py.Type { return writerType }

// writeRow renders one record, applying the dialect's quoting rules.
//
// The rule is CPython's join_append: a field is quoted when it is empty, or
// contains the delimiter, quotechar, or any character of the line terminator;
// under QUOTE_ALL always, and under QUOTE_NONNUMERIC whenever the field is not
// a number.
func (w *Writer) writeRow(fields []py.Object) (string, error) {
	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteString(w.spec.delimiter)
		}
		if i == 0 && w.spec.quoting == quoteNonNumeric {
			// QUOTE_NONNUMERIC writes the first field's numeric value back as a
			// number, not a string.
			switch f.(type) {
			case py.Int, py.Float, *py.BigInt:
				s, err := py.StrAsString(f)
				if err != nil {
					return "", err
				}
				b.WriteString(s)
				continue
			}
		}
		s, isStr, err := fieldString(f)
		if err != nil {
			return "", err
		}
		quoted, err := w.needsQuotes(s, isStr)
		if err != nil {
			return "", err
		}
		if quoted {
			b.WriteString(w.spec.quotechar)
			if w.spec.doublequote {
				b.WriteString(strings.ReplaceAll(s, w.spec.quotechar, w.spec.quotechar+w.spec.quotechar))
			} else {
				var q strings.Builder
				for _, r := range s {
					if string(r) == w.spec.quotechar && w.spec.hasEscapechar {
						q.WriteString(w.spec.escapechar)
					}
					q.WriteRune(r)
				}
				b.WriteString(q.String())
			}
			b.WriteString(w.spec.quotechar)
		} else {
			if w.spec.quoting == quoteNone && w.spec.hasEscapechar {
				// With QUOTE_NONE the delimiter, quotechar, escapechar and any
				// line terminator character must be escaped.
				b.WriteString(escapeField(s, w.spec))
			} else {
				b.WriteString(s)
			}
		}
	}
	b.WriteString(w.spec.lineterminator)
	return b.String(), nil
}

// escapeField escapes the characters that would otherwise be special when
// QUOTE_NONE is in force.
func escapeField(s string, spec dialectSpec) string {
	var b strings.Builder
	for _, r := range s {
		c := string(r)
		if c == spec.delimiter || (spec.hasQuotechar && c == spec.quotechar) || (spec.hasEscapechar && c == spec.escapechar) ||
			strings.Contains(spec.lineterminator, c) || c == "\n" || c == "\r" {
			if spec.hasEscapechar {
				b.WriteString(spec.escapechar)
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

// needsQuotes decides whether a field is written quoted.
func (w *Writer) needsQuotes(s string, isStr bool) (bool, error) {
	switch w.spec.quoting {
	case quoteAll:
		return true, nil
	case quoteNonNumeric:
		return isStr, nil
	case quoteNone:
		return false, nil
	}
	// QUOTE_MINIMAL: a field is quoted only when it contains a character that
	// would otherwise be special.  An empty field is written as nothing, so a
	// row of two empty fields is just the delimiter.
	if w.spec.hasQuotechar && strings.Contains(s, w.spec.quotechar) {
		return true, nil
	}
	if strings.Contains(s, w.spec.delimiter) {
		return true, nil
	}
	for _, r := range s {
		if strings.ContainsRune(w.spec.lineterminator, r) || r == '\n' || r == '\r' {
			return true, nil
		}
	}
	return false, nil
}

// fieldString renders one field, reporting whether it was a string (which
// QUOTE_NONNUMERIC needs to know).
func fieldString(f py.Object) (string, bool, error) {
	if f == py.None {
		return "", false, nil
	}
	isStr := false
	switch f.(type) {
	case py.String:
		isStr = true
	case py.Bytes:
		isStr = true
	}
	s, err := py.StrAsString(f)
	if err != nil {
		return "", false, err
	}
	return s, isStr, nil
}

func (w *Writer) writeRowObj(row py.Object) error {
	items, err := py.SequenceList(row)
	if err != nil {
		return err
	}
	text, err := w.writeRow(items.Items)
	if err != nil {
		return err
	}
	file := w.file
	write, err := py.GetAttrString(file, "write")
	if err != nil {
		return err
	}
	_, err = py.Call(write, py.Tuple{py.String(text)}, py.StringDict{})
	return err
}

// ---------------------------------------------------------------------------
// Reader

// Reader is a csv.reader.  It is its own iterator, as CPython's is.
type Reader struct {
	spec dialectSpec
	file py.Object
	// pending holds a partially consumed line when a quoted field spans a
	// newline.

	// iter is the fallback for an argument that is not a file: CPython's
	// csv.reader takes any iterable of strings, so "list(csv.reader(['a,b']))"
	// works.  It is nil when the argument has a readline, which is the usual
	// file case.
	iter py.Object
}

func (r *Reader) Type() *py.Type { return readerType }

// M__iter__ returns the reader itself.
func (r *Reader) M__iter__() (py.Object, error) { return r, nil }

// M__next__ yields the next record as a list, and signals the end of
// iteration with StopIteration - which is the exception a for loop looks
// for here, and in CPython.  Using EOFError made the reader unusable in a
// for loop: "list(csv.reader([...]))" propagated EOFError instead of ending.
func (r *Reader) M__next__() (py.Object, error) {
	rec, err := r.readRecord()
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, py.StopIteration
	}
	return py.NewListFromItems(rec), nil
}

// readOneLine reads one line from the file, reporting false at EOF.
//
// A file answers through readline().  Anything else - a list, a generator -
// is iterated, because csv.reader accepts any iterable of strings and not
// only a file object.
func (r *Reader) readOneLine() (string, bool, error) {
	var line py.Object
	var err error
	if r.iter != nil {
		next, ok := r.iter.(py.I__next__)
		if !ok {
			return "", false, py.ExceptionNewf(py.TypeError, "argument 1 must be an iterator")
		}
		line, err = next.M__next__()
		if err == py.StopIteration {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
	} else {
		readline, err := py.GetAttrString(r.file, "readline")
		if err != nil {
			return "", false, err
		}
		line, err = py.Call(readline, py.Tuple{}, py.StringDict{})
		if err != nil {
			return "", false, err
		}
		if line == py.None {
			return "", false, nil
		}
	}
	s, err := py.StrAsString(line)
	if err != nil {
		return "", false, err
	}
	if s == "" {
		return "", false, nil
	}
	return s, true, nil
}

// readRecord parses one CSV record.  It returns nil at end of input.
func (r *Reader) readRecord() ([]py.Object, error) {
	line, ok, err := r.readOneLine()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	spec := r.spec
	var fields []py.Object
	var cur strings.Builder
	fieldQuoted := false
	// inQuotes tracks an open quoted field that may span lines.
	inQuotes := false

	// The record ends when a line terminator is met.  A line without one (the last
	// line of a file with no trailing newline) still yields its final field, which
	// process records here.
	i := 0
	ended := false
	process := func() error {
		for i < len(line) {
			c := line[i]
			if inQuotes {
				if spec.hasEscapechar && string(c) == spec.escapechar {
					if i+1 < len(line) {
						cur.WriteByte(line[i+1])
						i += 2
						continue
					}
					i++
					continue
				}
				if spec.hasQuotechar && string(c) == spec.quotechar {
					if spec.doublequote && i+1 < len(line) && string(line[i+1]) == spec.quotechar {
						cur.WriteByte(c)
						i += 2
						continue
					}
					inQuotes = false
					i++
					continue
				}
				cur.WriteByte(c)
				i++
				continue
			}
			if spec.hasQuotechar && string(c) == spec.quotechar {
				if cur.Len() == 0 && !fieldQuoted {
					inQuotes = true
					fieldQuoted = true
					i++
					continue
				}
				if spec.strict {
					return py.ExceptionNewf(csvError, "%d: unexpected quotechar", i)
				}
				cur.WriteByte(c)
				i++
				continue
			}
			if string(c) == spec.delimiter {
				fields = append(fields, r.mkField(cur.String(), fieldQuoted))
				cur.Reset()
				fieldQuoted = false
				i++
				if spec.skipinitialspace {
					for i < len(line) && line[i] == ' ' {
						i++
					}
				}
				continue
			}
			if c == '\r' || c == '\n' {
				// End of record; consume a following \n after \r.
				if c == '\r' && i+1 < len(line) && line[i+1] == '\n' {
					i++
				}
				i++
				fields = append(fields, r.mkField(cur.String(), fieldQuoted))
				ended = true
				return nil
			}
			if spec.hasEscapechar && string(c) == spec.escapechar && !spec.hasQuotechar {
				if i+1 < len(line) {
					cur.WriteByte(line[i+1])
					i += 2
					continue
				}
				i++
				continue
			}
			cur.WriteByte(c)
			i++
		}
		return nil
	}

	for {
		if err := process(); err != nil {
			return nil, err
		}
		if inQuotes {
			// The quoted field continues on the next line.  The line just consumed
			// already ended in a newline character (readline keeps it), and CPython's
			// parser sees that same character as the embedded newline, so nothing is
			// inserted here - appending one would double it.
			next, ok, err := r.readOneLine()
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, py.ExceptionNewf(csvError, "unexpected end of data")
			}
			line = next
			i = 0
			continue
		}
		break
	}
	if !ended {
		// The line ran out without a terminator; its last field is the remainder.
		fields = append(fields, r.mkField(cur.String(), fieldQuoted))
	}
	return fields, nil
}

// mkField applies the QUOTE_NONNUMERIC conversion.
func (r *Reader) mkField(s string, quoted bool) py.Object {
	if r.spec.quoting == quoteNonNumeric && !quoted {
		if f, err := parseNumber(s); err == nil {
			return f
		}
	}
	return py.String(s)
}

// parseNumber converts a field to int or float for QUOTE_NONNUMERIC.
func parseNumber(s string) (py.Object, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil, fmt.Errorf("empty")
	}
	if i, err := py.IntFromString(t, 10); err == nil {
		return i, nil
	}
	return py.FloatFromString(t)
}

// ---------------------------------------------------------------------------
// Sniffer

// snifferType is the Python-visible Sniffer class.
var snifferType = py.NewTypeX("csv.Sniffer",
	`Sniffer() -> object

"Sniffs" the format of a CSV file (i.e. delimiter, quotechar)
and returns a Dialect object.`,
	snifferNew, nil)

type sniffer struct{}

func (s *sniffer) Type() *py.Type { return snifferType }

func snifferNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 0 || kwargs.Len() != 0 {
		return nil, py.ExceptionNewf(py.TypeError, "Sniffer() takes no arguments")
	}
	return &sniffer{}, nil
}

// candidates are the delimiters CPython's Sniffer prefers, in order.
var candidates = []string{"\t", ";", ":", ",", "|", " "}

// sniffSample picks the delimiter that occurs most consistently across lines.
func sniffSample(sample string) (dialectSpec, error) {
	lines := splitNonEmpty(sample, 3)
	best := ""
	bestCount := 0
	for _, c := range candidates {
		// Consistency: the number of occurrences must be the same on at least
		// two lines, and greater than zero.
		counts := map[int]int{}
		for _, l := range lines {
			counts[strings.Count(l, c)]++
		}
		for n, freq := range counts {
			if n > 0 && freq > 1 && n > bestCount {
				best = c
				bestCount = n
			}
		}
	}
	if best == "" {
		return dialectSpec{}, py.ExceptionNewf(csvError, "Could not determine delimiter")
	}
	d := defaultDialect()
	d.delimiter = best
	return d, nil
}

// splitNonEmpty splits s into at most n non-empty lines.
func splitNonEmpty(s string, n int) []string {
	var out []string
	for _, l := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' }) {
		if l != "" {
			out = append(out, l)
			if len(out) == n {
				break
			}
		}
	}
	return out
}

// hasHeader implements the Sniffer's heuristic: the first row's fields differ
// in type from the second row's, or the columns have differing lengths.
func hasHeader(sample string, spec dialectSpec) bool {
	lines := splitNonEmpty(sample, 2)
	if len(lines) < 2 {
		return false
	}
	first := splitFields(lines[0], spec)
	second := splitFields(lines[1], spec)
	if len(first) != len(second) {
		return false
	}
	for i := range first {
		if typeName(first[i]) != typeName(second[i]) {
			return true
		}
	}
	return false
}

// splitFields is a minimal field splitter for the header heuristic.
func splitFields(line string, spec dialectSpec) []string {
	p := &Reader{spec: spec, file: py.None}
	fields, err := p.parseLineString(line)
	if err != nil {
		return nil
	}
	return fields
}

// parseLineString parses one line without a file, for the Sniffer.
func (r *Reader) parseLineString(line string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQuotes := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inQuotes {
			if r.spec.hasQuotechar && string(c) == r.spec.quotechar {
				if i+1 < len(line) && string(line[i+1]) == r.spec.quotechar {
					cur.WriteByte(c)
					i++
					continue
				}
				inQuotes = false
				continue
			}
			cur.WriteByte(c)
			continue
		}
		if r.spec.hasQuotechar && string(c) == r.spec.quotechar && cur.Len() == 0 {
			inQuotes = true
			continue
		}
		if string(c) == r.spec.delimiter {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	out = append(out, cur.String())
	return out, nil
}

// typeName classifies a field the way CPython's Sniffer does: as a number or a
// string.
func typeName(s string) string {
	if _, err := parseNumber(s); err == nil {
		return "number"
	}
	return "string"
}

// ---------------------------------------------------------------------------
// Module-level functions

func readerFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var csvfile py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "reader", 1, 1, &csvfile); err != nil {
		return nil, err
	}
	var dialect py.Object = py.None
	if v, ok := kwargs.Get("dialect"); ok {
		dialect = v
	}
	spec, err := lookupDialect(dialect)
	if err != nil {
		return nil, err
	}
	opts := py.NewStringDict()
	kwargs.Range(func(k string, v py.Object) bool {
		if k != "dialect" {
			opts.Set(k, v)
		}
		return false
	})
	if err := applySettings(&spec, nil, opts); err != nil {
		return nil, err
	}
	// A file answers through readline(); anything else is iterated.  Deciding
	// once here rather than per line keeps the common file case on its fast
	// path.
	if hasReadline(csvfile) {
		return &Reader{spec: spec, file: csvfile}, nil
	}
	it, err := py.Iter(csvfile)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "argument 1 must be an iterator")
	}
	return &Reader{spec: spec, file: csvfile, iter: it}, nil
}

// hasReadline reports whether the object is file-like, which is how CPython
// tells a file from a plain iterable.
func hasReadline(o py.Object) bool {
	_, err := py.GetAttrString(o, "readline")
	return err == nil
}

func writerFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var csvfile py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "writer", 1, 1, &csvfile); err != nil {
		return nil, err
	}
	var dialect py.Object = py.None
	if v, ok := kwargs.Get("dialect"); ok {
		dialect = v
	}
	spec, err := lookupDialect(dialect)
	if err != nil {
		return nil, err
	}
	opts := py.NewStringDict()
	kwargs.Range(func(k string, v py.Object) bool {
		if k != "dialect" {
			opts.Set(k, v)
		}
		return false
	})
	if err := applySettings(&spec, nil, opts); err != nil {
		return nil, err
	}
	return &Writer{spec: spec, file: csvfile}, nil
}

func registerDialectFn(self py.Object, args py.Tuple) (py.Object, error) {
	var dialect py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "register_dialect", 1, 1, &dialect); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(dialect)
	if err != nil {
		return nil, err
	}
	spec := defaultDialect()
	for _, r := range registered {
		if r.name == name {
			return nil, py.ExceptionNewf(csvError, "dialect %q is already registered", name)
		}
	}
	registered = append(registered, struct {
		name string
		spec dialectSpec
	}{name, spec})
	return py.None, nil
}

func getDialectFn(self py.Object, args py.Tuple) (py.Object, error) {
	var dialect py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "get_dialect", 1, 1, &dialect); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(dialect)
	if err != nil {
		return nil, err
	}
	for _, r := range registered {
		if r.name == name {
			return dialectObject(r.spec), nil
		}
	}
	return nil, py.ExceptionNewf(csvError, "unknown dialect")
}

// dialectObject wraps a spec as a Dialect.
func dialectObject(spec dialectSpec) py.Object {
	return &Dialect{spec: spec}
}

func listDialectsFn(self py.Object, args py.Tuple) (py.Object, error) {
	if err := py.UnpackTuple(args, py.StringDict{}, "list_dialects", 0, 0); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(registered))
	for _, r := range registered {
		names = append(names, r.name)
	}
	return py.NewListFromStrings(names), nil
}

func fieldSizeLimitFn(self py.Object, args py.Tuple) (py.Object, error) {
	if err := py.UnpackTuple(args, py.StringDict{}, "field_size_limit", 0, 0); err != nil {
		return nil, err
	}
	return py.Int(131072), nil
}

// ---------------------------------------------------------------------------

func init() {

	// Dialect attributes are exposed so a script can read and write them.
	dialectAttrs := []struct {
		name  string
		isStr bool
	}{
		{"delimiter", true},
		{"quotechar", true},
		{"escapechar", true},
		{"doublequote", false},
		{"skipinitialspace", false},
		{"lineterminator", true},
		{"quoting", false},
	}
	for _, attr := range dialectAttrs {
		name := attr.name
		isStr := attr.isStr
		dialectType.Dict.Set(name, &py.Property{
			Fget: func(self py.Object) (py.Object, error) {
				d := self.(*Dialect)
				switch name {
				case "delimiter":
					return py.String(d.spec.delimiter), nil
				case "quotechar":
					if !d.spec.hasQuotechar {
						return py.None, nil
					}
					return py.String(d.spec.quotechar), nil
				case "escapechar":
					if !d.spec.hasEscapechar {
						return py.None, nil
					}
					return py.String(d.spec.escapechar), nil
				case "lineterminator":
					return py.String(d.spec.lineterminator), nil
				case "doublequote":
					return py.NewBool(d.spec.doublequote), nil
				case "skipinitialspace":
					return py.NewBool(d.spec.skipinitialspace), nil
				case "quoting":
					return py.Int(d.spec.quoting), nil
				}
				return py.None, nil
			},
			Fset: func(self, value py.Object) error {
				d := self.(*Dialect)
				if isStr {
					if value == py.None {
						switch name {
						case "quotechar":
							d.spec.quotechar = ""
							d.spec.hasQuotechar = false
						case "escapechar":
							d.spec.escapechar = ""
							d.spec.hasEscapechar = false
						}
						return nil
					}
					s, err := py.StrAsString(value)
					if err != nil {
						return err
					}
					switch name {
					case "delimiter":
						d.spec.delimiter = s
					case "quotechar":
						d.spec.quotechar = s
						d.spec.hasQuotechar = true
					case "escapechar":
						d.spec.escapechar = s
						d.spec.hasEscapechar = true
					case "lineterminator":
						d.spec.lineterminator = s
					}
					return nil
				}
				if name == "quoting" {
					n, err := py.IndexInt(value)
					if err != nil {
						return err
					}
					d.spec.quoting = n
					return nil
				}
				b, err := py.ObjectIsTrue(value)
				if err != nil {
					return err
				}
				switch name {
				case "doublequote":
					d.spec.doublequote = b
				case "skipinitialspace":
					d.spec.skipinitialspace = b
				}
				return nil
			},
		})
	}

	// writer: writerow / writerows
	writerType = py.NewTypeX("csv.writer", "CSV writer", nil, nil)
	writerType.Dict.Set("writerow", py.MustNewMethod("writerow", func(self py.Object, args py.Tuple) (py.Object, error) {
		w := self.(*Writer)
		var row py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "writerow", 1, 1, &row); err != nil {
			return nil, err
		}
		if err := w.writeRowObj(row); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "writerow(iterable)\n\nWrite a single row to the writer's file."))
	writerType.Dict.Set("writerows", py.MustNewMethod("writerows", func(self py.Object, args py.Tuple) (py.Object, error) {
		w := self.(*Writer)
		var rows py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "writerows", 1, 1, &rows); err != nil {
			return nil, err
		}
		it, err := py.Iter(rows)
		if err != nil {
			return nil, err
		}
		for {
			row, err := it.(py.I__next__).M__next__()
			if err != nil {
				// StopIteration arrives as an error value, which is how this
				// interpreter's iterators signal exhaustion.
				if isStopIteration(err) {
					break
				}
				return nil, err
			}
			if err := w.writeRowObj(row); err != nil {
				return nil, err
			}
		}
		return py.None, nil
	}, 0, "writerows(iterable of iterables)\n\nWrite a sequence of rows to the writer's file."))
	writerType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "The writer is its own iterator."))

	// reader: __iter__ / __next__ are defined on the type below.
	readerType = py.NewTypeX("csv.reader", "CSV reader", nil, nil)

	snifferType.Dict.Set("sniff", py.MustNewMethod("sniff", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var sample py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "sniff", 1, 1, &sample); err != nil {
			return nil, err
		}
		s, err := py.StrAsString(sample)
		if err != nil {
			return nil, err
		}
		spec, err := sniffSample(s)
		if err != nil {
			return nil, err
		}
		return dialectObject(spec), nil
	}, 0, "sniff(sample, [delimiters]) -> Dialect\n\nAnalyse the sample and return a Dialect that describes its format."))
	snifferType.Dict.Set("has_header", py.MustNewMethod("has_header", func(self py.Object, args py.Tuple) (py.Object, error) {
		var sample py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "has_header", 1, 1, &sample); err != nil {
			return nil, err
		}
		s, err := py.StrAsString(sample)
		if err != nil {
			return nil, err
		}
		spec, err := sniffSample(s)
		if err != nil {
			return nil, err
		}
		return py.NewBool(hasHeader(s, spec)), nil
	}, 0, "has_header(sample) -> bool\n\nAnalyse the sample text to determine if the first row is a header."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "csv",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("reader", readerFn, 0, "reader(csvfile, dialect='excel', **fmtparams)\n\nCreate a CSV reader."),
			py.MustNewMethod("writer", writerFn, 0, "writer(csvfile, dialect='excel', **fmtparams)\n\nCreate a CSV writer."),
			py.MustNewMethod("register_dialect", registerDialectFn, 0, "Create a mapping from a string name to a dialect class."),
			py.MustNewMethod("unregister_dialect", func(self py.Object, args py.Tuple) (py.Object, error) {
				var dialect py.Object
				if err := py.UnpackTuple(args, py.StringDict{}, "unregister_dialect", 1, 1, &dialect); err != nil {
					return nil, err
				}
				name, err := py.StrAsString(dialect)
				if err != nil {
					return nil, err
				}
				for i, r := range registered {
					if r.name == name {
						registered = append(registered[:i], registered[i+1:]...)
						return py.None, nil
					}
				}
				return nil, py.ExceptionNewf(csvError, "unknown dialect")
			}, 0, "Delete the name/dialect mapping associated with a string name."),
			py.MustNewMethod("get_dialect", getDialectFn, 0, "Return the dialect instance associated with name."),
			py.MustNewMethod("list_dialects", listDialectsFn, 0, "Return a list of all know dialect names.\nnames = csv.list_dialects()"),
			py.MustNewMethod("field_size_limit", fieldSizeLimitFn, 0, "Sets an upper limit on parsed fields.\n    csv.field_size_limit([limit])"),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "Error", Value: csvError},
			py.DictEntry{Key: "Dialect", Value: dialectType},
			py.DictEntry{Key: "Sniffer", Value: snifferType},
			py.DictEntry{Key: "excel", Value: dialectObject(defaultDialect())},
			py.DictEntry{Key: "excel_tab", Value: dialectObject(tabDialect())},
			py.DictEntry{Key: "QUOTE_MINIMAL", Value: py.Int(quoteMinimal)},
			py.DictEntry{Key: "QUOTE_ALL", Value: py.Int(quoteAll)},
			py.DictEntry{Key: "QUOTE_NONNUMERIC", Value: py.Int(quoteNonNumeric)},
			py.DictEntry{Key: "QUOTE_NONE", Value: py.Int(quoteNone)},
			py.DictEntry{Key: "__version__", Value: py.String("1.0")},
		),
	})
}

// isStopIteration reports whether err is the iterator-exhaustion signal.
//
// The interpreter's own iterators return the StopIteration *type* itself (see
// Iterator.M__next__ in the py package) while a Python-level generator raises it
// as an instance, so both shapes are recognised.
func isStopIteration(err error) bool {
	if t, ok := err.(*py.Type); ok {
		return t == py.StopIteration
	}
	if exc, ok := err.(*py.Exception); ok {
		if exc.Base == py.StopIteration {
			return true
		}
		return py.ExceptionGivenMatches(exc, py.StopIteration)
	}
	return false
}

// writerType and readerType are the csv.writer and csv.reader types.  They are
// created in init, where their methods are attached, and referenced from the
// Type methods of Writer and Reader so attribute lookup finds them.
var (
	writerType *py.Type
	readerType *py.Type
)

func init() {
	readerType.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self, nil
	}, 0, "The reader is its own iterator."))
	readerType.Dict.Set("__next__", py.MustNewMethod("__next__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*Reader).M__next__()
	}, 0, "Return the next row of the CSV file."))
	readerType.Dict.Set("line_num", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(0), nil
	}})
}
