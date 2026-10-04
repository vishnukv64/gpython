// Package plistlib implements property lists: the XML format pkg_resources
// reads to find the macOS version, and the same format a build tool writes.
//
// Only the XML format is implemented.  The BINARY format is read by nothing
// here, and a reader for it that answered plausibly would be worse than the
// honest refusal below - so FMT_BINARY is present as a constant (a program
// names it) but dumping or loading with it raises.
//
// The parsing uses Go's encoding/xml rather than adding an expat-shaped module
// to this interpreter: a property list is a small, regular document, and the
// whole grammar fits in one token loop.
package plistlib

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `plistlib is a module for reading and writing the plist format.

The plist format is basically a serializer of basic object types, and is
the standard way of storing configuration on macOS.  Two formats exist: the
XML format and the older binary format.  Only the XML format is implemented
here.
`

// PLISTHEADER is the XML prologue dumps() writes.
const PLISTHEADER = `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
	`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n"

// InvalidFileException is raised for a document that is not a property list.
// CPython defines it as a ValueError subclass, which is what a caller catching
// ValueError relies on.
var InvalidFileException = py.ValueError.NewType("plistlib.InvalidFileException",
	"Raised when a plist file is not valid.", nil, nil)

// UID is the "unique identifier" plist type.  It is a distinct type because a
// document may carry one and the caller compares it by type.
type uid struct{ data int64 }

var uidType = py.NewType("plistlib.UID", "The plist UID type.")

func (u *uid) Type() *py.Type { return uidType }

// PlistFormat holds the format constants.  CPython 3.9+ makes them members of
// a PlistFormat enum; the enum machinery is not needed for the value to be
// usable, so a plain class with two attributes is what a caller sees.
var plistFormatType = py.NewType("plistlib.PlistFormat", "The property list format.")

type plistFormat struct {
	name  string
	value int
}

func (f *plistFormat) Type() *py.Type { return plistFormatType }

var (
	fmtXML    = &plistFormat{name: "FMT_XML", value: 1}
	fmtBinary = &plistFormat{name: "FMT_BINARY", value: 2}
)

// ---------------------------------------------------------------------------
// Parsing

// parseXML reads one property list document into Python objects.
func parseXML(data []byte) (py.Object, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	// Property lists are UTF-8; a stray declaration must not break parsing.
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }

	// Find the root <plist> element.
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, py.ExceptionNewf(InvalidFileException, "no plist element found")
		}
		if err != nil {
			return nil, py.ExceptionNewf(InvalidFileException, "%s", err.Error())
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local != "plist" {
				return nil, py.ExceptionNewf(InvalidFileException,
					"unexpected element %q, expected plist", start.Name.Local)
			}
			return parseValue(dec, 0)
		}
	}
}

// parseValue reads the next element, which is one property list value.
func parseValue(dec *xml.Decoder, depth int) (py.Object, error) {
	if depth > 512 {
		return nil, py.ExceptionNewf(InvalidFileException, "plist is nested too deeply")
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return nil, py.ExceptionNewf(InvalidFileException, "unexpected end of document")
			}
			return nil, py.ExceptionNewf(InvalidFileException, "%s", err.Error())
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return parseElement(dec, t, depth)
		case xml.EndElement:
			return py.None, nil
		case xml.CharData:
			// Whitespace between elements carries no meaning.
			if strings.TrimSpace(string(t)) != "" {
				return nil, py.ExceptionNewf(InvalidFileException,
					"unexpected text %q in a plist", strings.TrimSpace(string(t)))
			}
		}
	}
}

// parseElement reads the element tok and its children.
func parseElement(dec *xml.Decoder, tok xml.StartElement, depth int) (py.Object, error) {
	switch tok.Name.Local {
	case "true":
		if err := skipElement(dec, tok); err != nil {
			return nil, err
		}
		return py.True, nil
	case "false":
		if err := skipElement(dec, tok); err != nil {
			return nil, err
		}
		return py.False, nil
	case "string", "key":
		text, err := elementText(dec, tok)
		if err != nil {
			return nil, err
		}
		return py.String(text), nil
	case "integer":
		text, err := elementText(dec, tok)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return nil, py.ExceptionNewf(InvalidFileException,
				"%q is not an integer", strings.TrimSpace(text))
		}
		return py.Int(n), nil
	case "real":
		text, err := elementText(dec, tok)
		if err != nil {
			return nil, err
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			if strings.TrimSpace(text) == "nan" {
				f = math.NaN()
			} else if strings.TrimSpace(text) == "+inf" {
				f = math.Inf(1)
			} else if strings.TrimSpace(text) == "-inf" {
				f = math.Inf(-1)
			} else {
				return nil, py.ExceptionNewf(InvalidFileException,
					"%q is not a float", strings.TrimSpace(text))
			}
		}
		return py.Float(f), nil
	case "data":
		text, err := elementText(dec, tok)
		if err != nil {
			return nil, err
		}
		// Whitespace inside a <data> element is insignificant, as in CPython.
		clean := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, text)
		raw, err := base64.StdEncoding.DecodeString(clean)
		if err != nil {
			return nil, py.ExceptionNewf(InvalidFileException, "invalid base64 data")
		}
		return py.Bytes(raw), nil
	case "date":
		// CPython returns a datetime.datetime.  This interpreter cannot build
		// one from here without an import cycle, and returning the ISO string
		// instead would be a silent type change - a caller comparing against a
		// datetime would get a wrong answer rather than an error.
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"plistlib: <date> values are not supported; reading one as a string would change its type")

	case "array":
		items := []py.Object{}
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, py.ExceptionNewf(InvalidFileException, "%s", err.Error())
			}
			switch t := tok.(type) {
			case xml.StartElement:
				v, err := parseElement(dec, t, depth+1)
				if err != nil {
					return nil, err
				}
				items = append(items, v)
			case xml.EndElement:
				return py.NewListFromItems(items), nil
			}
		}
	case "dict":
		d := py.NewStringDict()
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, py.ExceptionNewf(InvalidFileException, "%s", err.Error())
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local != "key" {
					return nil, py.ExceptionNewf(InvalidFileException,
						"missing <key> in a dict, found <%s>", t.Name.Local)
				}
				keyText, err := elementText(dec, t)
				if err != nil {
					return nil, err
				}
				val, err := parseValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				if val == py.None {
					// parseValue gives None when the dict ended instead of
					// supplying a value.  CPython raises ValueError here, and
					// storing None would be a silent wrong answer: a caller
					// reading the key would get None rather than an error.
					return nil, py.ExceptionNewf(InvalidFileException,
						"missing value for key '%s'", keyText)
				}
				d.Set(keyText, val)
			case xml.EndElement:
				return d, nil
			}
		}
	}
	return nil, py.ExceptionNewf(InvalidFileException, "unknown element <%s>", tok.Name.Local)
}

// elementText returns the character data directly inside tok.
func elementText(dec *xml.Decoder, tok xml.StartElement) (string, error) {
	var b strings.Builder
	for {
		t, err := dec.Token()
		if err != nil {
			return "", py.ExceptionNewf(InvalidFileException, "%s", err.Error())
		}
		switch v := t.(type) {
		case xml.CharData:
			b.Write(v)
		case xml.EndElement:
			return b.String(), nil
		case xml.StartElement:
			return "", py.ExceptionNewf(InvalidFileException,
				"<%s> must not contain <%s>", tok.Name.Local, v.Name.Local)
		}
	}
}

// skipElement consumes the rest of an empty element such as <true/>.
func skipElement(dec *xml.Decoder, tok xml.StartElement) error {
	for {
		t, err := dec.Token()
		if err != nil {
			return py.ExceptionNewf(InvalidFileException, "%s", err.Error())
		}
		if _, ok := t.(xml.EndElement); ok {
			return nil
		}
	}
}

// ---------------------------------------------------------------------------
// Serialising

// serializeXML writes a value as a property list document.
func serializeXML(v py.Object, sortKeys bool, indent int) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(PLISTHEADER)
	b.WriteString(`<plist version="1.0">` + "\n")
	if err := writeValue(&b, v, 0, sortKeys, indent); err != nil {
		return nil, err
	}
	b.WriteString("</plist>\n")
	return b.Bytes(), nil
}

func writeValue(b *bytes.Buffer, v py.Object, depth int, sortKeys bool, indent int) error {
	if depth > 512 {
		return py.ExceptionNewf(InvalidFileException, "value is nested too deeply")
	}
	pad := strings.Repeat("\t", depth)
	switch t := v.(type) {
	case py.Bool:
		if bool(t) {
			b.WriteString(pad + "<true/>\n")
		} else {
			b.WriteString(pad + "<false/>\n")
		}
	case py.Int:
		b.WriteString(fmt.Sprintf("%s<integer>%d</integer>\n", pad, int64(t)))
	case py.Float:
		b.WriteString(fmt.Sprintf("%s<real>%s</real>\n", pad, formatReal(float64(t))))
	case py.String:
		b.WriteString(fmt.Sprintf("%s<string>%s</string>\n", pad, xmlEscape(string(t))))
	case py.Bytes:
		b.WriteString(fmt.Sprintf("%s<data>%s</data>\n", pad,
			base64.StdEncoding.EncodeToString([]byte(t))))
	case py.Tuple:
		b.WriteString(pad + "<array>\n")
		for _, item := range t {
			if err := writeValue(b, item, depth+1, sortKeys, indent); err != nil {
				return err
			}
		}
		b.WriteString(pad + "</array>\n")
	case *py.List:
		b.WriteString(pad + "<array>\n")
		for _, item := range t.Items {
			if err := writeValue(b, item, depth+1, sortKeys, indent); err != nil {
				return err
			}
		}
		b.WriteString(pad + "</array>\n")
	case *uid:
		b.WriteString(fmt.Sprintf("%s<integer>%d</integer>\n", pad, t.data))
	case py.IGetDict:
		b.WriteString(pad + "<dict>\n")
		keys := t.GetDict().Keys()
		if sortKeys {
			sortStrings(keys)
		}
		for _, k := range keys {
			val, _ := t.GetDict().Get(k)
			b.WriteString(fmt.Sprintf("%s\t<key>%s</key>\n", pad, xmlEscape(k)))
			if err := writeValue(b, val, depth+1, sortKeys, indent); err != nil {
				return err
			}
		}
		b.WriteString(pad + "</dict>\n")
	default:
		return py.ExceptionNewf(py.TypeError,
			"unsupported type: %s", v.Type().Name)
	}
	return nil
}

// formatReal renders a float the way CPython's repr does, so that a round trip
// through the XML text gives the same value back.
func formatReal(f float64) string {
	if math.IsInf(f, 1) {
		return "+inf"
	}
	if math.IsInf(f, -1) {
		return "-inf"
	}
	if math.IsNaN(f) {
		return "nan"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// xmlEscape escapes the five characters XML reserves.  A plist value is text,
// so & and < would otherwise end the element early.
func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ---------------------------------------------------------------------------
// The module

func loads(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "loads() missing 1 required positional argument: 'value'")
	}
	// The value may be bytes or str; CPython accepts either.
	var data []byte
	switch v := args[0].(type) {
	case py.Bytes:
		data = []byte(v)
	case py.String:
		data = []byte(v)
	default:
		return nil, py.ExceptionNewf(py.TypeError,
			"loads() argument must be bytes or str, not %s", args[0].Type().Name)
	}
	if fmtArg, ok := kwargs.Get("fmt"); ok {
		if f, isFmt := fmtArg.(*plistFormat); isFmt && f.value == fmtBinary.value {
			return nil, py.ExceptionNewf(py.NotImplementedError,
				"plistlib: the BINARY format is not supported, only FMT_XML")
		}
	}
	return parseXML(data)
}

func load(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "load() missing 1 required positional argument: 'fp'")
	}
	obj := args[0]
	// The file object's read() returns the bytes, whatever kind of file it is.
	read, err := py.GetAttrString(obj, "read")
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "load() argument must be a file object")
	}
	data, err := py.Call(read, py.Tuple{}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	return loads(self, py.Tuple{data}, kwargs)
}

func dumps(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "dumps() missing 1 required positional argument: 'value'")
	}
	// sort_keys DEFAULTS TO TRUE in CPython, so the keys of a dumped dict come
	// out sorted unless the caller says otherwise.
	sortKeys := true
	if v, ok := kwargs.Get("sort_keys"); ok {
		b, err := py.ObjectIsTrue(v)
		if err != nil {
			return nil, err
		}
		sortKeys = b
	}
	if fmtArg, ok := kwargs.Get("fmt"); ok {
		if f, isFmt := fmtArg.(*plistFormat); isFmt && f.value == fmtBinary.value {
			return nil, py.ExceptionNewf(py.NotImplementedError,
				"plistlib: the BINARY format is not supported, only FMT_XML")
		}
	}
	out, err := serializeXML(args[0], sortKeys, -1)
	if err != nil {
		return nil, err
	}
	return py.Bytes(out), nil
}

func dump(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "dump() missing required arguments")
	}
	data, err := dumps(self, py.Tuple{args[0]}, kwargs)
	if err != nil {
		return nil, err
	}
	write, err := py.GetAttrString(args[1], "write")
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "dump() needs a writable file object")
	}
	if _, err := py.Call(write, py.Tuple{data}, py.NewStringDict()); err != nil {
		return nil, err
	}
	return py.None, nil
}

func init() {
	// FMT_XML and FMT_BINARY are attributes of the PlistFormat class in
	// CPython 3.9+, and of the module before that.  Both are set, so a program
	// reading either finds them.
	plistFormatType.Dict.Set("FMT_XML", fmtXML)
	plistFormatType.Dict.Set("FMT_BINARY", fmtBinary)

	methods := []*py.Method{
		py.MustNewMethod("loads", loads, 0, "loads(value, *, fmt=FMT_XML, dict_type=dict) -> the parsed property list."),
		py.MustNewMethod("load", load, 0, "load(fp, *, fmt=None, dict_type=dict) -> the parsed property list."),
		py.MustNewMethod("dumps", dumps, 0, "dumps(value, *, fmt=FMT_XML, sort_keys=True, skipkeys=False) -> the property list as bytes."),
		py.MustNewMethod("dump", dump, 0, "dump(value, fp, *, fmt=FMT_XML, sort_keys=True, skipkeys=False) -> write a property list."),
	}

	globals := py.NewStringDict()
	globals.Set("__doc__", py.String(module_doc))
	globals.Set("PLISTHEADER", py.Bytes(PLISTHEADER))
	globals.Set("InvalidFileException", InvalidFileException)
	globals.Set("PlistFormat", plistFormatType)
	globals.Set("FMT_XML", fmtXML)
	globals.Set("FMT_BINARY", fmtBinary)
	globals.Set("UID", uidType)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "plistlib",
			Doc:  module_doc,
		},
		Methods: methods,
		Globals: globals,
	})
}
