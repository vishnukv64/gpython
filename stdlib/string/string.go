// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package string provides the implementation of the python's 'string' module.
package string

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "string",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("capwords", capwords, 0, capwords_doc),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "whitespace", Value: whitespace},
			py.DictEntry{Key: "ascii_lowercase", Value: ascii_lowercase},
			py.DictEntry{Key: "ascii_uppercase", Value: ascii_uppercase},
			py.DictEntry{Key: "ascii_letters", Value: ascii_letters},
			py.DictEntry{Key: "digits", Value: digits},
			py.DictEntry{Key: "hexdigits", Value: hexdigits},
			py.DictEntry{Key: "octdigits", Value: octdigits},
			py.DictEntry{Key: "punctuation", Value: punctuation},
			py.DictEntry{Key: "printable", Value: printable},
			py.DictEntry{Key: "Template", Value: TemplateType},
		),
	})
}

const module_doc = `A collection of string constants.

Public module variables:

whitespace -- a string containing all ASCII whitespace
ascii_lowercase -- a string containing all ASCII lowercase letters
ascii_uppercase -- a string containing all ASCII uppercase letters
ascii_letters -- a string containing all ASCII letters
digits -- a string containing all ASCII decimal digits
hexdigits -- a string containing all ASCII hexadecimal digits
octdigits -- a string containing all ASCII octal digits
punctuation -- a string containing all ASCII punctuation characters
printable -- a string containing all ASCII characters considered printable
`

var (
	whitespace      = py.String(" \t\n\r\x0b\x0c")
	ascii_lowercase = py.String("abcdefghijklmnopqrstuvwxyz")
	ascii_uppercase = py.String("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	ascii_letters   = ascii_lowercase + ascii_uppercase
	digits          = py.String("0123456789")
	hexdigits       = py.String("0123456789abcdefABCDEF")
	octdigits       = py.String("01234567")
	punctuation     = py.String("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~")
	printable       = py.String("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~ \t\n\r\x0b\x0c")
)

const capwords_doc = `capwords(s [,sep]) -> string

Split the argument into words using split, capitalize each
word using capitalize, and join the capitalized words using
join.  If the optional second argument sep is absent or None,
runs of whitespace characters are replaced by a single space
and leading and trailing whitespace are removed, otherwise
sep is used to split and join the words.`

func capwords(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pystr py.Object
		pysep py.Object = py.None
	)
	err := py.ParseTupleAndKeywords(args, kwargs, "s|z", []string{"s", "sep"}, &pystr, &pysep)
	if err != nil {
		return nil, err
	}

	pystr = py.String(strings.ToLower(string(pystr.(py.String))))
	pyvs, err := pystr.(py.String).Split(py.Tuple{pysep}, py.StringDict{})
	if err != nil {
		return nil, err
	}

	var (
		lst   = pyvs.(*py.List).Items
		vs    = make([]string, len(lst))
		sep   = ""
		title = func(s string) string {
			if s == "" {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		}
	)

	switch pysep {
	case py.None:
		for i := range vs {
			v := string(lst[i].(py.String))
			vs[i] = title(strings.Trim(v, string(whitespace)))
		}
		sep = " "
	default:
		sep = string(pysep.(py.String))
		for i := range vs {
			v := string(lst[i].(py.String))
			vs[i] = title(v)
		}
	}

	return py.String(strings.Join(vs, sep)), nil
}

// ---------------------------------------------------------------------------
// Template
//
// string.Template is $-substitution over a mapping.  CPython implements it by
// compiling a regular expression and running sub() with a conversion callable;
// the scanning here is done in Go for the same result, because the conversion
// depends on a mapping lookup whose failure has to raise the *same* exception
// object the caller would see - a KeyError naming the missing key - and going
// back and forth through a Python callable would lose that.
//
// The class still exposes pattern, idpattern, braceidpattern, delimiter and
// flags, and those are the values the scanner uses, so a subclass that
// overrides them changes the behaviour as documented.

// Template is the $-substituting class.
type Template struct {
	Dict py.StringDict
}

// TemplateType is string.Template.
var TemplateType = py.NewTypeX("string.Template", "A string class for supporting $-substitutions.", templateNew, nil)

func (t *Template) Type() *py.Type { return TemplateType }

func (t *Template) GetDict() py.StringDict { return t.Dict }

var _ py.IGetDict = (*Template)(nil)

// templatePattern is the default compiled expression, exactly CPython's: an
// escaped delimiter, a named placeholder, a braced placeholder, or the empty
// alternative that marks an invalid one.  The literal delimiter is matched by
// a character class rather than re.escape's backslash, because the re in this
// interpreter is RE2-backed and an escaped "\$" is an end anchor - the pattern
// then never matched anything.
const templatePattern = `[$](?:(?P<escaped>[$])|(?P<named>[_a-zA-Z][_a-zA-Z0-9]*)|{(?P<braced>[_a-zA-Z][_a-zA-Z0-9]*)}|(?P<invalid>))`

const (
	templateDelimiter = "$"
	templateIDPattern = `(?a:[_a-z][_a-z0-9]*)`
)

func templateNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "Template() takes exactly one argument")
	}
	// The template is stored as it was given; CPython only insists it be a
	// string when the pattern is applied, and the re module's error there
	// ("expected string or bytes-like object") is what a caller sees.
	t := &Template{Dict: py.NewStringDict()}
	t.Dict.Set("template", args[0])
	return t, nil
}

// classAttr reads a class attribute, falling back to a default when a subclass
// removed it.
func classAttr(self py.Object, name string, def py.Object) py.Object {
	if v, err := py.GetAttrString(self.Type(), name); err == nil && v != nil {
		return v
	}
	return def
}

// token is one placeholder occurrence, with the fields the substitution needs.
type token struct {
	kind  int // placeholder, escaped, invalid
	ident string
	start int // byte offset of the '$' that begins the token
	end   int // byte offset just past the token
	at    int // byte offset of the invalid part, for the error position
}

const (
	tokPlaceholder = iota
	tokEscaped
	tokInvalid
)

// scan walks the template the way the compiled pattern would, returning each
// match in order.  delimiter is the active delimiter string (usually "$").
func scan(text, delimiter string) []token {
	var toks []token
	identStart := func(b byte) bool {
		return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
	}
	identCont := func(b byte) bool {
		return identStart(b) || (b >= '0' && b <= '9')
	}
	for i := 0; i < len(text); {
		if !strings.HasPrefix(text[i:], delimiter) {
			i++
			continue
		}
		// An escaped delimiter is two delimiters in a row.
		if strings.HasPrefix(text[i+len(delimiter):], delimiter) {
			toks = append(toks, token{kind: tokEscaped, start: i, end: i + 2*len(delimiter), at: i + len(delimiter)})
			i += 2 * len(delimiter)
			continue
		}
		j := i + len(delimiter)
		if j < len(text) && text[j] == '{' {
			k := j + 1
			for k < len(text) && identCont(text[k]) {
				k++
			}
			if k > j+1 && k < len(text) && text[k] == '}' {
				toks = append(toks, token{kind: tokPlaceholder, ident: text[j+1 : k], start: i, end: k + 1, at: k + 1})
				i = k + 1
				continue
			}
			// "${" with no identifier, or no closing brace: the '$' alone is
			// the invalid match, exactly as the empty alternative matches.
			toks = append(toks, token{kind: tokInvalid, start: i, end: j, at: j})
			i = j
			continue
		}
		if j < len(text) && identStart(text[j]) {
			k := j
			for k < len(text) && identCont(text[k]) {
				k++
			}
			toks = append(toks, token{kind: tokPlaceholder, ident: text[j:k], start: i, end: k, at: k})
			i = k
			continue
		}
		// A bare '$' with nothing that can follow it.
		toks = append(toks, token{kind: tokInvalid, start: i, end: j, at: j})
		i = j
	}
	return toks
}

// invalidError is CPython's _invalid(): it reports the line and column of the
// offending '$' inside the template.  The column is the length of the last
// line of template[:at], which is what splitlines-based arithmetic works out
// to; \r\n counts as one break.
func invalidError(template string, at int) error {
	prefix := template[:at]
	line, col := 1, 1
	if prefix != "" {
		line, col = 1, 0
		for i := 0; i < len(prefix); i++ {
			switch prefix[i] {
			case '\r':
				if i+1 < len(prefix) && prefix[i+1] == '\n' {
					i++
				}
				line++
				col = 0
			case '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85:
				line++
				col = 0
			case '\n':
				line++
				col = 0
			default:
				col++
			}
		}
	}
	return py.ExceptionNewf(py.ValueError, "Invalid placeholder in string: line %d, col %d", line, col)
}

// lookupMapping fetches one key, distinguishing "the mapping said no" from
// "there is no mapping at all" so the caller raises the right exception.
func lookupMapping(mapping py.Object, key string) (py.Object, error) {
	v, err := py.GetItem(mapping, py.String(key))
	if err != nil {
		return nil, err
	}
	return v, nil
}

// mappingArgs handles the (mapping=None, /, **kwds) signature both substitution
// methods share, folding keywords into the mapping the way CPython does.
func mappingArgs(name string, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) > 1 {
		return nil, py.ExceptionNewf(py.TypeError, "Template.%s() takes from 1 to 2 positional arguments but %d were given", name, len(args)+1)
	}
	if len(args) == 1 {
		if kwargs.Len() == 0 {
			return args[0], nil
		}
		// ChainMap(kwds, mapping), so a keyword wins over the mapping.
		chainMap := py.GetModuleImplOrNil("collections")
		if chainMap == nil {
			return nil, py.ExceptionNewf(py.TypeError, "collections.ChainMap is not available")
		}
		cm, err := py.GetAttrString(chainMap, "ChainMap")
		if err != nil {
			return nil, err
		}
		return py.Call(cm, py.Tuple{kwargs, args[0]}, py.StringDict{})
	}
	return kwargs, nil
}

// templateString coerces the object being operated on to a str, raising
// CPython's message for anything else.
func templateString(v py.Object) (string, error) {
	switch s := v.(type) {
	case py.String:
		return string(s), nil
	case py.Bytes:
		return string(s), nil
	}
	return "", py.ExceptionNewf(py.TypeError, "expected string or bytes-like object, got '%s'", v.Type().Name)
}

// render builds the substituted string.  safe selects safe_substitute's
// behaviour for a missing key and for an invalid placeholder.
func (t *Template) render(args py.Tuple, kwargs py.StringDict, name string, safe bool) (py.Object, error) {
	templateObj, ok := t.Dict.Get("template")
	if !ok {
		return nil, py.ExceptionNewf(py.AttributeError, "template")
	}
	text, err := templateString(templateObj)
	if err != nil {
		return nil, err
	}
	// Touch the pattern first: CPython's methods run the compiled expression,
	// and compiling it caches flags onto the class, which is observable.
	if _, err := py.GetAttrString(t, "pattern"); err != nil {
		return nil, err
	}
	mapping, err := mappingArgs(name, args, kwargs)
	if err != nil {
		return nil, err
	}
	delimiter := templateDelimiter
	if v := classAttr(t, "delimiter", py.String(templateDelimiter)); v != nil {
		if s, err := py.StrAsString(v); err == nil {
			delimiter = s
		}
	}

	var b strings.Builder
	last := 0
	for _, tok := range scan(text, delimiter) {
		b.WriteString(text[last:tok.start])
		switch tok.kind {
		case tokEscaped:
			b.WriteString(delimiter)
		case tokInvalid:
			if !safe {
				return nil, invalidError(text, tok.at)
			}
			b.WriteString(text[tok.start:tok.end])
		case tokPlaceholder:
			v, err := lookupMapping(mapping, tok.ident)
			if err != nil {
				if safe && isKeyError(err) {
					b.WriteString(text[tok.start:tok.end])
					break
				}
				return nil, err
			}
			s, err := py.StrAsString(v)
			if err != nil {
				return nil, err
			}
			b.WriteString(s)
		}
		last = tok.end
	}
	b.WriteString(text[last:])
	return py.String(b.String()), nil
}

// isKeyError reports whether an error is a KeyError, which safe_substitute
// treats as "leave the placeholder alone".
func isKeyError(err error) bool {
	e, ok := err.(*py.Exception)
	return ok && e.Base.IsSubtype(py.KeyError)
}

const templateSubstitute_doc = `substitute(mapping=None, /, **kws) -> str

Like %s, but raise KeyError for a missing key.`

func templateSubstitute(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return self.(*Template).render(args, kwargs, "substitute", false)
}

const templateSafeSubstitute_doc = `safe_substitute(mapping=None, /, **kws) -> str

Like %s, but leave placeholders that are not valid Python identifiers or
for which a key is missing alone rather than raising.`

func templateSafeSubstitute(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return self.(*Template).render(args, kwargs, "safe_substitute", true)
}

func templateIsValid(self py.Object, args py.Tuple) (py.Object, error) {
	t := self.(*Template)
	templateObj, ok := t.Dict.Get("template")
	if !ok {
		return nil, py.ExceptionNewf(py.AttributeError, "template")
	}
	text, err := templateString(templateObj)
	if err != nil {
		return nil, err
	}
	if _, err := py.GetAttrString(t, "pattern"); err != nil {
		return nil, err
	}
	delimiter := templateDelimiter
	if v := classAttr(t, "delimiter", py.String(templateDelimiter)); v != nil {
		if s, err := py.StrAsString(v); err == nil {
			delimiter = s
		}
	}
	for _, tok := range scan(text, delimiter) {
		if tok.kind == tokInvalid {
			return py.False, nil
		}
	}
	return py.True, nil
}

func templateGetIdentifiers(self py.Object, args py.Tuple) (py.Object, error) {
	t := self.(*Template)
	templateObj, ok := t.Dict.Get("template")
	if !ok {
		return nil, py.ExceptionNewf(py.AttributeError, "template")
	}
	text, err := templateString(templateObj)
	if err != nil {
		return nil, err
	}
	if _, err := py.GetAttrString(t, "pattern"); err != nil {
		return nil, err
	}
	delimiter := templateDelimiter
	if v := classAttr(t, "delimiter", py.String(templateDelimiter)); v != nil {
		if s, err := py.StrAsString(v); err == nil {
			delimiter = s
		}
	}
	seen := map[string]bool{}
	var ids []py.Object
	for _, tok := range scan(text, delimiter) {
		if tok.kind != tokPlaceholder || seen[tok.ident] {
			continue
		}
		seen[tok.ident] = true
		ids = append(ids, py.String(tok.ident))
	}
	return py.NewListFromItems(ids), nil
}

// templatePatternAttr compiles the class's pattern on first access and caches
// the result back on the class, exactly as CPython's _compile_pattern does -
// including writing IGNORECASE back into flags.
func templatePatternAttr(self py.Object) (py.Object, error) {
	cls := self.Type()
	// Lookup returns the raw class-dict value, so a Property left in place by
	// a previous miss is not re-invoked here.
	if cur := cls.Lookup("pattern"); cur != nil {
		switch v := cur.(type) {
		case *py.Property:
			// The descriptor itself: not yet compiled.
		default:
			if v == py.None {
				break
			}
			if s, err := py.StrAsString(v); err == nil {
				_ = s
				return v, nil
			}
			if _, err := py.GetAttrString(v, "finditer"); err == nil {
				return v, nil
			}
		}
	}

	re := py.GetModuleImplOrNil("re")
	if re == nil {
		return py.None, nil
	}
	compile, err := py.GetAttrString(re, "compile")
	if err != nil {
		return nil, err
	}
	ic, err := py.GetAttrString(re, "IGNORECASE")
	if err != nil {
		return nil, err
	}
	verbose, err := py.GetAttrString(re, "VERBOSE")
	if err != nil {
		return nil, err
	}
	source := templatePattern
	if v := classAttr(self, "pattern", py.None); v != nil {
		if s, err := py.StrAsString(v); err == nil {
			source = s
		}
	}
	f, _ := py.IndexInt(ic)
	fl, _ := py.IndexInt(verbose)
	// _compile_pattern defaults flags to IGNORECASE and keeps it, so the class
	// attribute stops being None once the pattern has been compiled.
	cls.Dict.Set("flags", py.Int(reIgnoreCase))
	pat, err := py.Call(compile, py.Tuple{py.String(source), py.Int(f | fl)}, py.StringDict{})
	if err != nil {
		return nil, err
	}
	// The compiled pattern replaces the descriptor, so later reads are cheap
	// and a subclass sees its own compiled object.
	cls.Dict.Set("pattern", pat)
	return pat, nil
}

func init() {
	TemplateType.Dict.Set("delimiter", py.String(templateDelimiter))
	TemplateType.Dict.Set("idpattern", py.String(templateIDPattern))
	TemplateType.Dict.Set("braceidpattern", py.None)
	TemplateType.Dict.Set("flags", py.None)

	TemplateType.Dict.Set("template", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			v, ok := self.(*Template).Dict.Get("template")
			if !ok {
				return py.None, nil
			}
			return v, nil
		},
		Fset: func(self py.Object, value py.Object) error {
			self.(*Template).Dict.Set("template", value)
			return nil
		},
	})
	// The pattern is computed on access and cached back, as CPython's
	// _compile_pattern does.
	TemplateType.Dict.Set("pattern", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return templatePatternAttr(self)
	}})

	TemplateType.Dict.Set("substitute", py.MustNewMethod("substitute", templateSubstitute, 0, replaceAll(templateSubstitute_doc, "substitute")))
	TemplateType.Dict.Set("safe_substitute", py.MustNewMethod("safe_substitute", templateSafeSubstitute, 0, replaceAll(templateSafeSubstitute_doc, "safe_substitute")))
	TemplateType.Dict.Set("is_valid", py.MustNewMethod("is_valid", templateIsValid, 0, "Return whether every placeholder in the template is valid."))
	TemplateType.Dict.Set("get_identifiers", py.MustNewMethod("get_identifiers", templateGetIdentifiers, 0, "Return the identifiers in the template, in order of first appearance."))
}

// reIgnoreCase is re.IGNORECASE, which Template.flags caches once the pattern
// has been compiled - CPython's _compile_pattern writes it back.
const reIgnoreCase = 2

// replaceAll is a tiny helper so the two one-line doc constants above can be
// templated without duplicating the text.
func replaceAll(s, name string) string {
	return strings.ReplaceAll(s, "%s", name)
}
