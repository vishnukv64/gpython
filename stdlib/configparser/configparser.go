// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package configparser implements the configparser module: reading and writing
// INI-style configuration files.
//
// The format is the one CPython documents - "[section]" headers, "key = value"
// or "key: value" entries, "#" and ";" comments, and continuation lines indented
// deeper than the key they belong to.  Values are stored as strings and
// converted on request by getint/getfloat/getboolean.
//
// Interpolation is NOT performed.  "%(name)s" in a value is left exactly as
// written, which is what RawConfigParser does; BasicInterpolation and
// ExtendedInterpolation are absent rather than silently applying a partial
// version of the rules.  Code that needs them gets an AttributeError naming the
// missing class, which is an honest failure, where a half-implemented
// interpolation would quietly return the wrong string.
//
// This exists for pip: rich's theme.py calls ConfigParser().read_file() and
// .items("styles"), and pip renders its own output through rich.

package configparser

import (
	"os"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// DefaultSection is the name of the section whose options every other section
// also sees, spelled once so the comparison cannot drift from the registration.
const DefaultSection = "DEFAULT"

const module_doc = `Configuration file parser.

A set of default options for an interpreter-wide parser is kept in the DEFAULTSECT
section.  Values can contain "%" interpolations, but this implementation stores
them literally - see the package documentation.
`

// Errors, as CPython defines them.
var (
	ErrorType                           = py.ExceptionType.NewType("configparser.Error", "Base class for configparser exceptions.", nil, nil)
	NoSectionErrorType                  = ErrorType.NewType("configparser.NoSectionError", "Raised when a section is not found.", nil, nil)
	DuplicateSectionErrorType           = ErrorType.NewType("configparser.DuplicateSectionError", "Raised when a section is repeated.", nil, nil)
	DuplicateOptionErrorType            = ErrorType.NewType("configparser.DuplicateOptionError", "Raised when an option is repeated.", nil, nil)
	NoOptionErrorType                   = ErrorType.NewType("configparser.NoOptionError", "Raised when an option is not found.", nil, nil)
	InterpolationErrorType              = ErrorType.NewType("configparser.InterpolationError", "Base class for interpolation errors.", nil, nil)
	InterpolationMissingOptionErrorType = InterpolationErrorType.NewType("configparser.InterpolationMissingOptionError", "Raised when an interpolation reference is missing.", nil, nil)
	InterpolationSyntaxErrorType        = InterpolationErrorType.NewType("configparser.InterpolationSyntaxError", "Raised when interpolation syntax is wrong.", nil, nil)
	InterpolationDepthErrorType         = InterpolationErrorType.NewType("configparser.InterpolationDepthError", "Raised when interpolation nesting is too deep.", nil, nil)
	ParsingErrorType                    = ErrorType.NewType("configparser.ParsingError", "Raised when a file cannot be parsed.", nil, nil)
	MissingSectionHeaderErrorType       = ParsingErrorType.NewType("configparser.MissingSectionHeaderError", "Raised when a line appears before any section.", nil, nil)
)

// parser is the Go value behind a ConfigParser instance.
type parser struct {
	// sections maps a section name to its options, in insertion order.
	sections py.StringDict
	// defaults holds the [DEFAULT] options, which every section also sees.
	defaults py.StringDict
	// optionxform lowercases an option name, as CPython does by default.
	optionxform py.Object
	// strict rejects a repeated section or option.
	strict bool
}

var parserType = py.NewTypeX("configparser.ConfigParser", "Configuration file parser.", newParser, nil)

// newParser is the constructor; the type is built with NewTypeX so that
// ConfigParser() - and a subclass that does not override __init__ - produces a
// working parser.

func (p *parser) Type() *py.Type { return parserType }

// newParser is the constructor for both ConfigParser and RawConfigParser.
func newParser(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p := &parser{
		sections: py.NewStringDict(),
		defaults: py.NewStringDict(),
		strict:   true,
	}
	// defaults=... seeds the DEFAULT section.
	if v, ok := kwargs.Get("defaults"); ok {
		if d, ok := v.(py.StringDict); ok {
			p.defaults = d
		}
	}
	if v, ok := kwargs.Get("strict"); ok {
		if b, ok := v.(py.Bool); ok {
			p.strict = bool(b)
		}
	}
	if v, ok := kwargs.Get("allow_no_value"); ok {
		_ = v
	}
	return p, nil
}

// xform applies optionxform to an option name.
func (p *parser) xform(name string) string {
	if p.optionxform == nil {
		return strings.ToLower(name)
	}
	res, err := py.Call(p.optionxform, py.Tuple{py.String(name)}, py.NewStringDict())
	if err != nil {
		return strings.ToLower(name)
	}
	s, err := py.StrAsString(res)
	if err != nil {
		return strings.ToLower(name)
	}
	return s
}

// sectionOf returns a section's dict, or nil.
func (p *parser) sectionOf(name string) (py.StringDict, bool) {
	if name == DefaultSection {
		return p.defaults, true
	}
	v, ok := p.sections.Get(name)
	if !ok {
		return py.StringDict{}, false
	}
	d, ok := v.(py.StringDict)
	if !ok {
		return py.StringDict{}, false
	}
	return d, true
}

// get implements get() for the sections and for the proxy.
func (p *parser) get(section, option string) (py.Object, bool) {
	d, ok := p.sectionOf(section)
	if !ok {
		return nil, false
	}
	if v, ok := d.Get(p.xform(option)); ok {
		return v, true
	}
	// A section also sees the DEFAULT options, which is why an option may be
	// missing from its own section and still be found.
	if v, ok := p.defaults.Get(p.xform(option)); ok {
		return v, true
	}
	return nil, false
}

// parse reads INI text into the parser.
//
// The continuation rule is the one that catches people out: a line indented
// deeper than the option it follows CONTINUES that option's value, rather than
// starting a new one - which is how pip's and rich's own theme files are
// written.
func (p *parser) parse(text, source string) error {
	lines := strings.Split(text, "\n")
	section := ""
	indent := 0
	currentKey := ""
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' || trimmed[0] == ';' {
			continue
		}
		lead := len(line) - len(strings.TrimLeft(line, " \t"))
		if line[lead] == '[' {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				return py.ExceptionNewf(ParsingErrorType, "Source contains parsing errors: %s", source)
			}
			section = strings.TrimSpace(line[lead+1 : end])
			indent = 0
			currentKey = ""
			// [DEFAULT] is not a section in the listing - it is the shared
			// options every other section inherits - so it is never added to
			// the section table.  CPython's sections() omits it, and listing
			// it made "sections()" report one section too many.
			if section == DefaultSection {
				continue
			}
			if _, exists := p.sections.Get(section); exists && p.strict {
				return py.ExceptionNewf(DuplicateSectionErrorType,
					"While reading from %s : section %q already exists", source, section)
			}
			if _, exists := p.sections.Get(section); !exists {
				p.sections.Set(section, py.NewStringDict())
			}
			continue
		}
		if section == "" {
			return py.ExceptionNewf(MissingSectionHeaderErrorType,
				"File contains no section headers.\nfile: %s, line: %d\n%q", source, i+1, raw)
		}
		if currentKey != "" && lead > indent {
			// A continuation of the previous value.
			d, _ := p.sectionOf(section)
			prev, _ := d.Get(currentKey)
			prevStr, _ := py.StrAsString(prev)
			// A blank continuation line contributes a newline; CPython keeps
			// it, and dropping it changes a multi-line value.
			if trimmed == "" {
				d.Set(currentKey, py.String(prevStr+"\n"))
			} else {
				d.Set(currentKey, py.String(prevStr+"\n"+trimmed))
			}
			continue
		}
		// key = value   or   key : value
		eq := strings.IndexByte(trimmed, '=')
		colon := strings.IndexByte(trimmed, ':')
		sep := eq
		if sep < 0 || (colon >= 0 && colon < sep) {
			sep = colon
		}
		if sep < 0 {
			return py.ExceptionNewf(ParsingErrorType,
				"Source contains parsing errors: %s\n\t[line %2d]: %q", source, i+1, raw)
		}
		key := p.xform(strings.TrimSpace(trimmed[:sep]))
		value := strings.TrimSpace(trimmed[sep+1:])
		d, _ := p.sectionOf(section)
		if _, exists := d.Get(key); exists && p.strict {
			return py.ExceptionNewf(DuplicateOptionErrorType,
				"While reading from %s : option %q in section %q already exists", source, key, section)
		}
		d.Set(key, py.String(value))
		currentKey = key
		indent = lead
	}
	return nil
}

// items returns a section's options as a list of (name, value) pairs.
func (p *parser) items(section string) (py.Object, error) {
	if _, ok := p.sections.Get(section); !ok {
		return nil, py.ExceptionNewf(NoSectionErrorType, "No section: %q", section)
	}
	out := py.NewList()
	// The DEFAULT options come first and are overridden by the section's own,
	// which is the order CPython reports.
	seen := map[string]bool{}
	for _, ent := range p.defaults.Items() {
		out.Append(py.Tuple{py.String(ent.Key), ent.Value})
		seen[ent.Key] = true
	}
	d, _ := p.sectionOf(section)
	for _, ent := range d.Items() {
		if seen[ent.Key] {
			continue
		}
		out.Append(py.Tuple{py.String(ent.Key), ent.Value})
	}
	return out, nil
}

// proxy is the object dict["section"] returns: it behaves like the section's
// mapping, falling back to the parser's defaults.
type proxy struct {
	p       *parser
	section string
}

var proxyType = py.NewType("configparser.SectionProxy", "A view onto one section.")

func (s *proxy) Type() *py.Type { return proxyType }

func (s *proxy) keys() []string {
	out := []string{}
	seen := map[string]bool{}
	for _, ent := range s.p.defaults.Items() {
		out = append(out, ent.Key)
		seen[ent.Key] = true
	}
	d, _ := s.p.sectionOf(s.section)
	for _, ent := range d.Items() {
		if !seen[ent.Key] {
			out = append(out, ent.Key)
		}
	}
	return out
}

func (s *proxy) M__getitem__(key py.Object) (py.Object, error) {
	name, err := py.StrAsString(key)
	if err != nil {
		return nil, err
	}
	v, ok := s.p.get(s.section, name)
	if !ok {
		return nil, py.ExceptionNewf(py.KeyError, "%s", name)
	}
	return v, nil
}

func (s *proxy) M__setitem__(key, value py.Object) (py.Object, error) {
	name, err := py.StrAsString(key)
	if err != nil {
		return nil, err
	}
	val, err := py.StrAsString(value)
	if err != nil {
		return nil, err
	}
	d, ok := s.p.sectionOf(s.section)
	if !ok {
		return nil, py.ExceptionNewf(NoSectionErrorType, "No section: %q", s.section)
	}
	d.Set(s.p.xform(name), py.String(val))
	return py.None, nil
}

func (s *proxy) M__contains__(key py.Object) (py.Object, error) {
	name, err := py.StrAsString(key)
	if err != nil {
		return py.False, nil
	}
	_, ok := s.p.get(s.section, name)
	return py.NewBool(ok), nil
}

func (s *proxy) M__iter__() (py.Object, error) {
	out := py.NewList()
	for _, k := range s.keys() {
		out.Append(py.String(k))
	}
	return py.Iter(out)
}

func (s *proxy) M__len__() (py.Object, error) { return py.Int(len(s.keys())), nil }

func (s *proxy) M__repr__() (py.Object, error) {
	return py.String("<Section: " + s.section + ">"), nil
}

// readFile reads a whole file, for read().
func readFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

// readAll drains a file-like object to a string.
//
// configparser is handed an open file by its callers - rich passes the object
// it opened - so this uses read() when the object offers it and falls back to
// iterating lines for one that does not.
func readAll(f py.Object) (string, error) {
	if _, ok, err := py.TypeCall0(f, "read"); err != nil {
		return "", err
	} else if ok {
		res, _, err := py.TypeCall0(f, "read")
		if err != nil {
			return "", err
		}
		if s, err := py.StrAsString(res); err == nil {
			return s, nil
		}
	}
	var sb strings.Builder
	for {
		line, _, err := py.TypeCall0(f, "readline")
		if err != nil {
			return "", err
		}
		s, _ := py.StrAsString(line)
		if s == "" {
			break
		}
		sb.WriteString(s)
	}
	return sb.String(), nil
}

func init() {
	parserType.Dict.Set("read", py.MustNewMethod("read", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "read() needs filenames")
		}
		var names []py.Object
		if t, ok := args[0].(py.Tuple); ok {
			names = t
		} else {
			names = []py.Object{args[0]}
		}
		read := py.NewList()
		for _, n := range names {
			s, err := py.StrAsString(n)
			if err != nil {
				return nil, err
			}
			data, rerr := readFile(s)
			if rerr != nil {
				// A missing file is not an error here: read() reports which
				// files it managed to read, and the caller checks that.
				continue
			}
			if err := p.parse(string(data), s); err != nil {
				return nil, err
			}
			read.Append(py.String(s))
		}
		return read, nil
	}, 0, "Read and parse a filename or a list of filenames."))

	parserType.Dict.Set("read_file", py.MustNewMethod("read_file", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "read_file() needs a file object")
		}
		source := "<???>"
		if v, ok := kwargs.Get("source"); ok {
			if s, err := py.StrAsString(v); err == nil {
				source = s
			}
		}
		text, err := readAll(args[0])
		if err != nil {
			return nil, err
		}
		if err := p.parse(text, source); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Like read(), but the file name is not checked first."))

	parserType.Dict.Set("read_string", py.MustNewMethod("read_string", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p := self.(*parser)
		text := ""
		if len(args) > 0 {
			s, err := py.StrAsString(args[0])
			if err != nil {
				return nil, err
			}
			text = s
		}
		source := "<string>"
		if v, ok := kwargs.Get("source"); ok {
			if s, err := py.StrAsString(v); err == nil {
				source = s
			}
		}
		if err := p.parse(text, source); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Read configuration from a string."))

	parserType.Dict.Set("read_dict", py.MustNewMethod("read_dict", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "read_dict() needs a dictionary")
		}
		outer, ok := args[0].(py.StringDict)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "read_dict() needs a dictionary, not %s", args[0].Type().Name)
		}
		for _, sec := range outer.Items() {
			inner, ok := sec.Value.(py.StringDict)
			if !ok {
				continue
			}
			if sec.Key != "DEFAULT" {
				if _, exists := p.sections.Get(sec.Key); !exists {
					p.sections.Set(sec.Key, py.NewStringDict())
				}
			}
			d, _ := p.sectionOf(sec.Key)
			for _, ent := range inner.Items() {
				s, _ := py.StrAsString(ent.Value)
				d.Set(p.xform(ent.Key), py.String(s))
			}
		}
		return py.None, nil
	}, 0, "Read configuration from a dictionary."))

	parserType.Dict.Set("sections", py.MustNewMethod("sections", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		out := py.NewList()
		for _, ent := range p.sections.Items() {
			out.Append(py.String(ent.Key))
		}
		return out, nil
	}, 0, "Return a list of section names."))

	parserType.Dict.Set("has_section", py.MustNewMethod("has_section", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "has_section() needs a section name")
		}
		s, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		_, ok := p.sections.Get(s)
		return py.NewBool(ok), nil
	}, 0, "Indicate whether the named section is present in the configuration."))

	parserType.Dict.Set("has_option", py.MustNewMethod("has_option", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 2 {
			return nil, py.ExceptionNewf(py.TypeError, "has_option() needs a section and an option")
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		opt, err := py.StrAsString(args[1])
		if err != nil {
			return nil, err
		}
		_, ok := p.get(sec, opt)
		return py.NewBool(ok), nil
	}, 0, "Return True if the given section exists and contains the given option."))

	parserType.Dict.Set("options", py.MustNewMethod("options", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "options() needs a section name")
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		items, err := p.items(sec)
		if err != nil {
			return nil, err
		}
		out := py.NewList()
		for _, it := range items.(*py.List).Items {
			out.Append(it.(py.Tuple)[0])
		}
		return out, nil
	}, 0, "Return a list of option names for the given section name."))

	parserType.Dict.Set("items", py.MustNewMethod("items", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "items() needs a section name")
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		return p.items(sec)
	}, 0, "Return a list of (name, value) tuples for each option in a section."))

	parserType.Dict.Set("get", py.MustNewMethod("get", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 2 {
			return nil, py.ExceptionNewf(py.TypeError, "get() needs a section and an option")
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		opt, err := py.StrAsString(args[1])
		if err != nil {
			return nil, err
		}
		if v, ok := p.get(sec, opt); ok {
			return v, nil
		}
		if fallback, ok := kwargs.Get("fallback"); ok {
			return fallback, nil
		}
		if _, ok := p.sections.Get(sec); !ok {
			return nil, py.ExceptionNewf(NoSectionErrorType, "No section: %q", sec)
		}
		return nil, py.ExceptionNewf(NoOptionErrorType, "No option %q in section %q", p.xform(opt), sec)
	}, 0, "Get an option value for a given section."))

	parserType.Dict.Set("set", py.MustNewMethod("set", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 3 {
			return nil, py.ExceptionNewf(py.TypeError, "set() needs a section, an option and a value")
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		opt, err := py.StrAsString(args[1])
		if err != nil {
			return nil, err
		}
		val, err := py.StrAsString(args[2])
		if err != nil {
			return nil, err
		}
		d, ok := p.sectionOf(sec)
		if !ok {
			return nil, py.ExceptionNewf(NoSectionErrorType, "No section: %q", sec)
		}
		d.Set(p.xform(opt), py.String(val))
		return py.None, nil
	}, 0, "Set an option."))

	parserType.Dict.Set("remove_option", py.MustNewMethod("remove_option", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 2 {
			return nil, py.ExceptionNewf(py.TypeError, "remove_option() needs a section and an option")
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		opt, err := py.StrAsString(args[1])
		if err != nil {
			return nil, err
		}
		d, ok := p.sectionOf(sec)
		if !ok {
			return nil, py.ExceptionNewf(NoSectionErrorType, "No section: %q", sec)
		}
		had := d.Has(p.xform(opt))
		d.Del(p.xform(opt))
		return py.NewBool(had), nil
	}, 0, "Remove an option."))

	parserType.Dict.Set("add_section", py.MustNewMethod("add_section", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "add_section() needs a section name")
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		if sec == "DEFAULT" {
			return nil, py.ExceptionNewf(py.ValueError, "Invalid section name: %q", sec)
		}
		if _, exists := p.sections.Get(sec); exists {
			return nil, py.ExceptionNewf(DuplicateSectionErrorType, "Section %q already exists", sec)
		}
		p.sections.Set(sec, py.NewStringDict())
		return py.None, nil
	}, 0, "Create a new section in the configuration."))

	parserType.Dict.Set("remove_section", py.MustNewMethod("remove_section", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "remove_section() needs a section name")
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		if _, exists := p.sections.Get(sec); !exists {
			return py.False, nil
		}
		p.sections.Del(sec)
		return py.True, nil
	}, 0, "Remove a file section."))

	// The typed getters, which is what most callers actually use.
	for _, g := range []struct {
		name string
		conv func(string) (py.Object, error)
	}{
		{"getint", func(v string) (py.Object, error) {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return nil, py.ExceptionNewf(py.ValueError,
					"invalid literal for int() with base 10: %q", v)
			}
			return py.Int(n), nil
		}},
		{"getfloat", func(v string) (py.Object, error) {
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return nil, py.ExceptionNewf(py.ValueError,
					"could not convert string to float: %q", v)
			}
			return py.Float(f), nil
		}},
		{"getboolean", func(v string) (py.Object, error) {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "1", "yes", "true", "on":
				return py.True, nil
			case "0", "no", "false", "off":
				return py.False, nil
			}
			return nil, py.ExceptionNewf(py.ValueError,
				"Not a boolean: %s", v)
		}},
	} {
		g := g
		parserType.Dict.Set(g.name, py.MustNewMethod(g.name, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			p := self.(*parser)
			if len(args) < 2 {
				return nil, py.ExceptionNewf(py.TypeError, "%s() needs a section and an option", g.name)
			}
			sec, err := py.StrAsString(args[0])
			if err != nil {
				return nil, err
			}
			opt, err := py.StrAsString(args[1])
			if err != nil {
				return nil, err
			}
			raw, ok := p.get(sec, opt)
			if !ok {
				if fallback, ok := kwargs.Get("fallback"); ok {
					return fallback, nil
				}
				if _, ok := p.sections.Get(sec); !ok {
					return nil, py.ExceptionNewf(NoSectionErrorType, "No section: %q", sec)
				}
				return nil, py.ExceptionNewf(NoOptionErrorType, "No option %q in section %q", p.xform(opt), sec)
			}
			str, err := py.StrAsString(raw)
			if err != nil {
				return nil, err
			}
			return g.conv(str)
		}, 0, "Convert an option of this section to "+g.name[3:]+"."))
	}

	parserType.Dict.Set("defaults", py.MustNewMethod("defaults", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*parser).defaults, nil
	}, 0, "Return a dictionary of the DEFAULT section."))

	parserType.Dict.Set("write", py.MustNewMethod("write", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "write() needs a file object")
		}
		var sb strings.Builder
		if p.defaults.Len() > 0 {
			sb.WriteString("[DEFAULT]\n")
			for _, ent := range p.defaults.Items() {
				s := ""
				if v, err := py.StrAsString(ent.Value); err == nil {
					s = v
				}
				sb.WriteString(ent.Key + " = " + s + "\n")
			}
			sb.WriteString("\n")
		}
		for _, sec := range p.sections.Items() {
			sb.WriteString("[" + sec.Key + "]\n")
			if d, ok := sec.Value.(*py.StringDict); ok {
				for _, ent := range d.Items() {
					s := ""
					if v, err := py.StrAsString(ent.Value); err == nil {
						s = v
					}
					sb.WriteString(ent.Key + " = " + s + "\n")
				}
			}
			sb.WriteString("\n")
		}
		if _, err := py.Call(args[0], py.Tuple{py.String(sb.String())}, py.NewStringDict()); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Write a representation of the configuration to the specified file object."))

	parserType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "__getitem__() needs a section name")
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		if _, ok := p.sections.Get(sec); !ok {
			return nil, py.ExceptionNewf(py.KeyError, "%s", sec)
		}
		return &proxy{p: p, section: sec}, nil
	}, 0, "Return the section as a mapping."))

	parserType.Dict.Set("__contains__", py.MustNewMethod("__contains__", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*parser)
		if len(args) < 1 {
			return py.False, nil
		}
		sec, err := py.StrAsString(args[0])
		if err != nil {
			return py.False, nil
		}
		_, ok := p.sections.Get(sec)
		return py.NewBool(ok), nil
	}, 0, "Return True if the section exists."))

	parserType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String("<configparser.ConfigParser object>"), nil
	}, 0, "Return repr(self)."))

	// A section proxy's own methods.
	proxyType.Dict.Set("get", py.MustNewMethod("get", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		s := self.(*proxy)
		if len(args) < 1 {
			return nil, py.ExceptionNewf(py.TypeError, "get() needs an option")
		}
		opt, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		if v, ok := s.p.get(s.section, opt); ok {
			return v, nil
		}
		if fallback, ok := kwargs.Get("fallback"); ok {
			return fallback, nil
		}
		if len(args) > 1 {
			return args[1], nil
		}
		return nil, py.ExceptionNewf(NoOptionErrorType, "No option %q in section %q", s.p.xform(opt), s.section)
	}, 0, "Get an option from this section."))

	proxyType.Dict.Set("keys", py.MustNewMethod("keys", func(self py.Object, args py.Tuple) (py.Object, error) {
		s := self.(*proxy)
		out := py.NewList()
		for _, k := range s.keys() {
			out.Append(py.String(k))
		}
		return out, nil
	}, 0, "Return the option names, including inherited defaults."))

	proxyType.Dict.Set("items", py.MustNewMethod("items", func(self py.Object, args py.Tuple) (py.Object, error) {
		return self.(*proxy).p.items(self.(*proxy).section)
	}, 0, "Return the (name, value) pairs of this section."))

	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "Error", Value: ErrorType},
		py.DictEntry{Key: "NoSectionError", Value: NoSectionErrorType},
		py.DictEntry{Key: "DuplicateSectionError", Value: DuplicateSectionErrorType},
		py.DictEntry{Key: "DuplicateOptionError", Value: DuplicateOptionErrorType},
		py.DictEntry{Key: "NoOptionError", Value: NoOptionErrorType},
		py.DictEntry{Key: "InterpolationError", Value: InterpolationErrorType},
		py.DictEntry{Key: "InterpolationMissingOptionError", Value: InterpolationMissingOptionErrorType},
		py.DictEntry{Key: "InterpolationSyntaxError", Value: InterpolationSyntaxErrorType},
		py.DictEntry{Key: "InterpolationDepthError", Value: InterpolationDepthErrorType},
		py.DictEntry{Key: "ParsingError", Value: ParsingErrorType},
		py.DictEntry{Key: "MissingSectionHeaderError", Value: MissingSectionHeaderErrorType},
		py.DictEntry{Key: "ConfigParser", Value: parserType},
		py.DictEntry{Key: "RawConfigParser", Value: parserType},
		py.DictEntry{Key: "DEFAULTSECT", Value: py.String(DefaultSection)},
		py.DictEntry{Key: "MAX_INTERPOLATION_DEPTH", Value: py.Int(10)},
		py.DictEntry{Key: "__doc__", Value: py.String(module_doc)},
	)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "configparser",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
