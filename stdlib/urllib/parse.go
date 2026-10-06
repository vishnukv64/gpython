// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// urllib.parse -- parse URLs into components.
//
// This ports CPython's urllib/parse.py: the SplitResult/ParseResult named
// tuples are exposed as small tuple-like objects carrying scheme/netloc/path/
// params/query/fragment plus the URL-mutating helpers, and the functions
// urlsplit, urlparse, urlunparse, urlsplit, urljoin, urlencode, quote, unquote,
// quote_plus, unquote_plus, parse_qs and parse_qsl follow CPython's behaviour
// including the default safe sets and the handling of a missing scheme and an
// IPv6 host.
//
// The module is registered under urllib too, and urllib.parse is what
// requests/urllib3 import.
package urllibparse

import (
	"sort"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const parse_doc = `Parse (absolute and relative) URLs.

urlsplit(url) -- split a URL into scheme, netloc, path, query, fragment.
urlparse(url) -- split a URL into scheme, netloc, path, params, query, fragment.
urlunparse(parts) -- construct a URL from a tuple as returned by urlparse().
urljoin(base, url) -- construct an absolute URL by combining a base URL.
urlencode(query) -- convert a mapping or sequence of two-element tuples to a
                    percent-encoded string.
quote(s) / unquote(s) -- percent-encode or -decode a string.
parse_qs(qs) / parse_qsl(qs) -- parse a query string, returning a dict or list.
`

// hexdig and its inverse are the percent-encoding tables.
const hexdig = "0123456789ABCDEF"

// alwaysSafe are the characters quote() never escapes ("never_quote" plus the
// unreserved marks CPython keeps).
const alwaysSafe = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.-~"

// SplitResult type and ParseResult type.
var (
	splitResultType = py.NewTypeX("urllib.parse.SplitResult",
		"A 5-tuple (scheme, netloc, path, query, fragment).", splitResultNew, nil)
	parseResultType = py.NewTypeX("urllib.parse.ParseResult",
		"A 6-tuple (scheme, netloc, path, params, query, fragment).", parseResultNew, nil)
)

// splitResult carries the five fields of a split URL.
type splitResult struct {
	scheme, netloc, path, query, fragment string
	Dict                                  py.StringDict
}

func (r *splitResult) Type() *py.Type         { return splitResultType }
func (r *splitResult) GetDict() py.StringDict { return r.Dict }

// parseResult carries the six fields of a parsed URL.
type parseResult struct {
	scheme, netloc, path, params, query, fragment string
	Dict                                          py.StringDict
}

func (r *parseResult) Type() *py.Type         { return parseResultType }
func (r *parseResult) GetDict() py.StringDict { return r.Dict }

// splitResultNew and parseResultNew accept the positional fields.
func splitResultNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	r := &splitResult{Dict: py.NewStringDict()}
	if err := fillFields(args, kwargs,
		[]string{"scheme", "netloc", "path", "query", "fragment"},
		[]*string{&r.scheme, &r.netloc, &r.path, &r.query, &r.fragment}); err != nil {
		return nil, err
	}
	return r, nil
}

func parseResultNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	r := &parseResult{Dict: py.NewStringDict()}
	if err := fillFields(args, kwargs,
		[]string{"scheme", "netloc", "path", "params", "query", "fragment"},
		[]*string{&r.scheme, &r.netloc, &r.path, &r.params, &r.query, &r.fragment}); err != nil {
		return nil, err
	}
	return r, nil
}

func fillFields(args py.Tuple, kwargs py.StringDict, names []string, dst []*string) error {
	get := func(i int) (string, bool) {
		if i < len(args) {
			s, err := py.StrAsString(args[i])
			return s, err == nil
		}
		if v, ok := kwargs.Get(names[i]); ok {
			s, err := py.StrAsString(v)
			return s, err == nil
		}
		return "", false
	}
	for i := range names {
		s, ok := get(i)
		if !ok {
			return py.ExceptionNewf(py.TypeError, "missing field: %s", names[i])
		}
		*dst[i] = s
	}
	return nil
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

// splitResult returns the five pieces of a URL.
//
// A scheme is only recognised as such when the character before "://" (or the
// single ":") is not a digit and the rest is a valid scheme, exactly as
// CPython does not mistake "localhost:8080" for a scheme.
func splitURL(url string, allowFragments bool) (scheme, netloc, path, query, fragment string) {
	// Strip the scheme.
	if i := strings.IndexByte(url, ':'); i > 0 {
		if schemeOk(url[:i]) {
			scheme = strings.ToLower(url[:i])
			url = url[i+1:]
		}
	}
	// Split off the fragment.
	if allowFragments {
		if i := strings.IndexByte(url, '#'); i >= 0 {
			fragment = url[i+1:]
			url = url[:i]
		}
	}
	// Split off the query.
	if i := strings.IndexByte(url, '?'); i >= 0 {
		query = url[i+1:]
		url = url[:i]
	}
	// Netloc: only when the URL starts with "//".
	if strings.HasPrefix(url, "//") {
		rest := url[2:]
		end := strings.IndexAny(rest, "/?#")
		if end < 0 {
			netloc = rest
			path = ""
		} else {
			netloc = rest[:end]
			path = rest[end:]
		}
		// A scheme with no path requires a "/", CPython returns "" and lets
		// the caller supply it; this mirrors urlunsplit's behaviour.
		if scheme != "" && path == "" && !strings.HasPrefix(url, "///") {
			path = ""
		}
	} else {
		path = url
	}
	return
}

// schemeOk reports whether s is a valid URL scheme (per RFC 3986).
func schemeOk(s string) bool {
	if s == "" {
		return false
	}
	if !isAlpha(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(isAlpha(c) || isDigit(c) || c == '+' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func parseURL(url string, allowFragments bool) (scheme, netloc, path, params, query, fragment string) {
	scheme, netloc, path, query, fragment = splitURL(url, allowFragments)
	// urlparse additionally splits params from the last path segment.
	if i := strings.LastIndexByte(path, ';'); i >= 0 {
		params = path[i+1:]
		path = path[:i]
	}
	return
}

func urllibSplit(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var url py.Object = py.String("")
	var scheme py.Object = py.String("")
	var allowFragments py.Object = py.Bool(true)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:urlsplit",
		[]string{"url", "scheme", "allow_fragments"}, &url, &scheme, &allowFragments); err != nil {
		return nil, err
	}
	u, _ := py.StrAsString(url)
	sc, nl, p, q, f := splitURL(u, truthy(allowFragments))
	if s, _ := py.StrAsString(scheme); s != "" {
		sc = s
	}
	r := &splitResult{scheme: sc, netloc: nl, path: p, query: q, fragment: f, Dict: py.NewStringDict()}
	return r, nil
}

func urllibParse(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var url py.Object = py.String("")
	var scheme py.Object = py.String("")
	var allowFragments py.Object = py.Bool(true)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:urlparse",
		[]string{"url", "scheme", "allow_fragments"}, &url, &scheme, &allowFragments); err != nil {
		return nil, err
	}
	u, _ := py.StrAsString(url)
	sc, nl, p, pr, q, f := parseURL(u, truthy(allowFragments))
	if s, _ := py.StrAsString(scheme); s != "" {
		sc = s
	}
	r := &parseResult{scheme: sc, netloc: nl, path: p, params: pr, query: q, fragment: f, Dict: py.NewStringDict()}
	return r, nil
}

// ---------------------------------------------------------------------------
// Unparsing
// ---------------------------------------------------------------------------

func urlUnsplit(scheme, netloc, path, query, fragment string) string {
	var b strings.Builder
	if scheme != "" {
		b.WriteString(scheme)
		b.WriteByte(':')
	}
	if netloc != "" || scheme == "file" && strings.HasPrefix(path, "/") {
		b.WriteString("//")
		b.WriteString(netloc)
	}
	if netloc != "" && path != "" && !strings.HasPrefix(path, "/") {
		b.WriteByte('/')
	}
	b.WriteString(path)
	if query != "" {
		b.WriteByte('?')
		b.WriteString(query)
	}
	if fragment != "" {
		b.WriteByte('#')
		b.WriteString(fragment)
	}
	return b.String()
}

// A fragment without a query needs the empty query marker.

func asStrField(v py.Object) (string, error) {
	if v == nil || v == py.None {
		return "", nil
	}
	return py.StrAsString(v)
}

func resultFields(obj py.Object, allowFrag bool) ([]string, error) {
	var fields []string
	switch r := obj.(type) {
	case *splitResult:
		fields = []string{r.scheme, r.netloc, r.path, r.query, r.fragment}
	case *parseResult:
		fields = []string{r.scheme, r.netloc, r.path, r.params, r.query, r.fragment}
	default:
		// Any 5- or 6-sequence.
		if seq, err := py.SequenceTuple(obj); err == nil {
			for _, it := range seq {
				s, err := asStrField(it)
				if err != nil {
					return nil, err
				}
				fields = append(fields, s)
			}
			return fields, nil
		}
		// A string with a geturl() method, or an object exposing the fields.
		if v, err := py.GetAttrString(obj, "geturl"); err == nil {
			_ = v
			s, _ := py.StrAsString(obj)
			fields = []string{"", "", s, "", ""}
			return fields, nil
		}
		return nil, py.ExceptionNewf(py.TypeError, "expected a 5- or 6-element sequence")
	}
	return fields, nil
}

func urllibUnsplit(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "urlunsplit() takes exactly one argument")
	}
	f, err := resultFields(args[0], true)
	if err != nil {
		return nil, err
	}
	if len(f) != 5 {
		return nil, py.ExceptionNewf(py.TypeError, "urlunsplit() argument must be a 5-element sequence")
	}
	return py.String(urlUnsplit(f[0], f[1], f[2], f[3], f[4])), nil
}

func urllibUnparse(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "urlunparse() takes exactly one argument")
	}
	f, err := resultFields(args[0], true)
	if err != nil {
		return nil, err
	}
	switch len(f) {
	case 5:
		return py.String(urlUnsplit(f[0], f[1], f[2], f[3], f[4])), nil
	case 6:
		// Re-attach the params to the path.
		path := f[2]
		if f[3] != "" {
			path = path + ";" + f[3]
		}
		return py.String(urlUnsplit(f[0], f[1], path, f[4], f[5])), nil
	}
	return nil, py.ExceptionNewf(py.TypeError, "urlunparse() argument must be a 6-element sequence")
}

// ---------------------------------------------------------------------------
// urljoin
// ---------------------------------------------------------------------------

func urllibJoin(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var base, url py.Object
	var allowFragments py.Object = py.Bool(true)
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|O:urljoin",
		[]string{"base", "url", "allow_fragments"}, &base, &url, &allowFragments); err != nil {
		return nil, err
	}
	b, err := py.StrAsString(base)
	if err != nil {
		return nil, err
	}
	u, err := py.StrAsString(url)
	if err != nil {
		return nil, err
	}
	return py.String(urljoin(b, u, truthy(allowFragments))), nil
}

// urljoin combines a base URL with a (possibly relative) URL, RFC 3986.
func urljoin(base, url string, allowFragments bool) string {
	if base == "" {
		return url
	}
	bscheme, bnetloc, bpath, bquery, bfrag := splitURL(base, allowFragments)
	_ = bquery
	_ = bfrag
	uscheme, unetloc, upath, uquery, ufrag := splitURL(url, allowFragments)

	if uscheme != bscheme {
		if uscheme != "" {
			return urlUnsplit(uscheme, unetloc, upath, uquery, ufrag)
		}
		if unetloc != "" {
			return urlUnsplit(bscheme, unetloc, upath, uquery, ufrag)
		}
		if upath == "" {
			return urlUnsplit(bscheme, bnetloc, bpath, uquery, ufrag)
		}
		if strings.HasPrefix(upath, "/") {
			return urlUnsplit(bscheme, bnetloc, removeDotSegments(upath), uquery, ufrag)
		}
		return urlUnsplit(bscheme, bnetloc, removeDotSegments(mergePaths(bnetloc, bpath, upath)), uquery, ufrag)
	}
	// Same scheme.
	return urlUnsplit(bscheme, bnetloc, removeDotSegments(mergePaths(bnetloc, bpath, upath)), uquery, ufrag)
}

// mergePaths implements RFC 3986 section 5.3.
func mergePaths(baseNetloc, basePath, refPath string) string {
	if baseNetloc != "" && basePath == "" {
		return "/" + refPath
	}
	i := strings.LastIndexByte(basePath, '/')
	if i < 0 {
		return refPath
	}
	return basePath[:i+1] + refPath
}

// removeDotSegments implements RFC 3986 section 5.2.4.
func removeDotSegments(path string) string {
	var out []string
	input := path
	for input != "" {
		switch {
		case strings.HasPrefix(input, "../"):
			input = input[3:]
		case strings.HasPrefix(input, "./"):
			input = input[2:]
		case strings.HasPrefix(input, "/./"):
			input = input[2:]
		case input == "/.":
			input = "/"
		case strings.HasPrefix(input, "/../"):
			input = input[3:]
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		case input == "/..":
			input = "/"
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		case input == "." || input == "..":
			input = ""
		default:
			i := 0
			if input[0] == '/' {
				i = 1
			}
			j := strings.IndexByte(input[i:], '/')
			if j < 0 {
				out = append(out, input)
				input = ""
			} else {
				out = append(out, input[:i+j])
				input = input[i+j:]
			}
		}
	}
	return strings.Join(out, "")
}

// ---------------------------------------------------------------------------
// quote / unquote
// ---------------------------------------------------------------------------

// quoteBytes percent-encodes s, never escaping alwaysSafe or the bytes in safe.
func quoteBytes(s, safe string, encoding string) (string, error) {
	_ = encoding // only UTF-8 is supported
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case strings.IndexByte(alwaysSafe, c) >= 0:
			b.WriteByte(c)
		case strings.IndexByte(safe, c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexdig[c>>4])
			b.WriteByte(hexdig[c&0x0f])
		}
	}
	return b.String(), nil
}

// unquoteBytes decodes percent escapes, plus '+' when plus is set.
func unquoteBytes(s string, plus bool) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		if c == '%' && i+2 < len(s) {
			hi := unhex(s[i+1])
			lo := unhex(s[i+2])
			if hi >= 0 && lo >= 0 {
				b.WriteByte(byte(hi<<4 | lo))
				i += 3
				continue
			}
		}
		if c == '+' && plus {
			b.WriteByte(' ')
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func urllibQuote(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var safe py.Object = py.String("/")
	var encoding py.Object = py.String("utf-8")
	var errors py.Object = py.String("strict")
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OOO:quote",
		[]string{"string", "safe", "encoding", "errors"}, &s, &safe, &encoding, &errors); err != nil {
		return nil, err
	}
	str, err := py.StrAsString(s)
	if err != nil {
		// A bytes argument is quoted byte-by-byte.
		if b, berr := py.BytesFromObject(s); berr == nil {
			str = string(b)
		} else {
			return nil, err
		}
	}
	sf, _ := py.StrAsString(safe)
	enc, _ := py.StrAsString(encoding)
	out, err := quoteBytes(str, sf, enc)
	if err != nil {
		return nil, err
	}
	return py.String(out), nil
}

func urllibUnquote(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var encoding py.Object = py.String("utf-8")
	var errors py.Object = py.String("replace")
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:unquote",
		[]string{"string", "encoding", "errors"}, &s, &encoding, &errors); err != nil {
		return nil, err
	}
	str, err := py.StrAsString(s)
	if err != nil {
		return nil, err
	}
	return py.String(unquoteBytes(str, false)), nil
}

func urllibQuotePlus(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var safe py.Object = py.String("")
	var encoding py.Object = py.String("utf-8")
	var errors py.Object = py.String("strict")
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OOO:quote_plus",
		[]string{"string", "safe", "encoding", "errors"}, &s, &safe, &encoding, &errors); err != nil {
		return nil, err
	}
	str, err := py.StrAsString(s)
	if err != nil {
		return nil, err
	}
	sf, _ := py.StrAsString(safe)
	// CPython quote_plus quotes with safe + ' ', then turns each space into a
	// '+'.  '+' itself is not safe, so it becomes %2B.
	out, err := quoteBytes(str, sf+" ", "utf-8")
	if err != nil {
		return nil, err
	}
	return py.String(strings.ReplaceAll(out, " ", "+")), nil
}

func urllibUnquotePlus(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	var encoding py.Object = py.String("utf-8")
	var errors py.Object = py.String("replace")
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:unquote_plus",
		[]string{"string", "encoding", "errors"}, &s, &encoding, &errors); err != nil {
		return nil, err
	}
	str, err := py.StrAsString(s)
	if err != nil {
		return nil, err
	}
	return py.String(unquoteBytes(str, true)), nil
}

// ---------------------------------------------------------------------------
// parse_qs / parse_qsl / urlencode
// ---------------------------------------------------------------------------

func urllibParseQsl(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var qs py.Object = py.String("")
	var keepBlank py.Object = py.Bool(false)
	var strictParsing py.Object = py.Bool(false)
	var encoding py.Object = py.String("utf-8")
	var errors py.Object = py.String("replace")
	var maxNumFields py.Object = py.None
	var separator py.Object = py.String("&")
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOOOOOO:parse_qsl",
		[]string{"qs", "keep_blank_values", "strict_parsing", "encoding", "errors",
			"max_num_fields", "separator"},
		&qs, &keepBlank, &strictParsing, &encoding, &errors, &maxNumFields, &separator); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(qs)
	if err != nil {
		return nil, err
	}
	sep, _ := py.StrAsString(separator)
	if sep == "" {
		sep = "&"
	}
	pairs, err := parseQsl(s, truthy(keepBlank), truthy(strictParsing), sep)
	if err != nil {
		return nil, err
	}
	out := make([]py.Object, len(pairs))
	for i, p := range pairs {
		out[i] = py.Tuple{py.String(p[0]), py.String(p[1])}
	}
	return py.NewListFromItems(out), nil
}

func parseQsl(qs string, keepBlank, strict bool, sep string) ([][2]string, error) {
	var pairs [][2]string
	// max_num_fields defaults to unlimited, matching CPython.
	for _, field := range splitUnquoted(qs, sep) {
		if field == "" {
			if strict {
				return nil, py.ExceptionNewf(py.ValueError, "bad query field: %s", reprStr(""))
			}
			continue
		}
		if i := strings.IndexByte(field, '='); i >= 0 {
			k := unquoteBytes(field[:i], true)
			v := unquoteBytes(field[i+1:], true)
			if v != "" || keepBlank {
				pairs = append(pairs, [2]string{k, v})
			}
		} else {
			k := unquoteBytes(field, true)
			if keepBlank {
				pairs = append(pairs, [2]string{k, ""})
			} else if strict {
				return nil, py.ExceptionNewf(py.ValueError, "bad query field: %s", reprStr(field))
			}
		}
	}
	return pairs, nil
}

// splitUnquoted splits s on sep, ignoring separators inside percent escapes.
func splitUnquoted(s, sep string) []string {
	if sep == "" {
		return []string{s}
	}
	var out []string
	start := 0
	for i := 0; i+len(sep) <= len(s); {
		if s[i] == '%' {
			// Skip the escape so "&" inside it is not a separator (there is
			// none inside a two-hex escape, but this mirrors the intent).
			i += 3
			continue
		}
		if s[i:i+len(sep)] == sep {
			out = append(out, s[start:i])
			i += len(sep)
			start = i
			continue
		}
		i++
	}
	out = append(out, s[start:])
	return out
}

func reprStr(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}

func urllibParseQs(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	res, err := urllibParseQsl(self, args, kwargs)
	if err != nil {
		return nil, err
	}
	l := res.(*py.List)
	// Build a dict mapping each key to the list of its values.
	d := py.NewStringDict()
	for _, it := range l.Items {
		pair := it.(py.Tuple)
		k := string(pair[0].(py.String))
		v := pair[1]
		if cur, ok := d.Get(k); ok {
			cur.(*py.List).Items = append(cur.(*py.List).Items, v)
		} else {
			d.Set(k, py.NewListFromItems([]py.Object{v}))
		}
	}
	return d, nil
}

func urllibUrlencode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var query py.Object
	var doseq py.Object = py.Bool(false)
	var safe py.Object = py.String("")
	var encoding py.Object = py.String("utf-8")
	var errors py.Object = py.String("strict")
	var quoteVia py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OOOOO:urlencode",
		[]string{"query", "doseq", "safe", "encoding", "errors", "quote_via"},
		&query, &doseq, &safe, &encoding, &errors, &quoteVia); err != nil {
		return nil, err
	}
	sf, _ := py.StrAsString(safe)
	_ = quoteVia
	var pairs [][2]string
	// A mapping, or a sequence of two-element sequences.
	if d, ok := query.(py.StringDict); ok {
		for _, ent := range d.Items() {
			pairs = append(pairs, [2]string{ent.Key, objToStr(ent.Value)})
		}
	} else if itemsFn, err := py.GetAttrString(query, "items"); err == nil && truthyDoseq(doseq, query) {
		_ = itemsFn
	}
	if len(pairs) == 0 {
		seq, err := py.SequenceList(query)
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError,
				"not a valid non-string sequence or mapping object")
		}
		for _, item := range seq.Items {
			pair, err := py.SequenceTuple(item)
			if err != nil || len(pair) != 2 {
				return nil, py.ExceptionNewf(py.TypeError,
					"not a valid non-string sequence or mapping object")
			}
			if truthy(doseq) {
				if l, ok := pair[1].(*py.List); ok {
					for _, v := range l.Items {
						pairs = append(pairs, [2]string{objToStr(pair[0]), objToStr(v)})
					}
					continue
				}
			}
			pairs = append(pairs, [2]string{objToStr(pair[0]), objToStr(pair[1])})
		}
	}
	var parts []string
	for _, p := range pairs {
		k, err := quoteBytes(p[0], sf+" ", "utf-8")
		if err != nil {
			return nil, err
		}
		v, err := quoteBytes(p[1], sf+" ", "utf-8")
		if err != nil {
			return nil, err
		}
		parts = append(parts, strings.ReplaceAll(k, " ", "+")+"="+strings.ReplaceAll(v, " ", "+"))
	}
	return py.String(strings.Join(parts, "&")), nil
}

func truthyDoseq(d py.Object, query py.Object) bool { return truthy(d) }

func objToStr(v py.Object) string {
	switch x := v.(type) {
	case py.String:
		return string(x)
	case py.Int:
		return strconv.FormatInt(int64(x), 10)
	case py.Float:
		return strconv.FormatFloat(float64(x), 'g', -1, 64)
	case py.Bool:
		if bool(x) {
			return "True"
		}
		return "False"
	case py.Bytes:
		return string(x)
	}
	s, err := py.StrAsString(v)
	if err != nil {
		return ""
	}
	return s
}

func truthy(v py.Object) bool {
	if v == nil || v == py.None {
		return false
	}
	b, err := py.ObjectIsTrue(v)
	if err != nil {
		return false
	}
	return b
}

var _ = sort.Strings

// ---------------------------------------------------------------------------
// Sequence protocol
// ---------------------------------------------------------------------------

// splitResult and parseResult behave like their CPython named tuples: the
// fields are readable by position and by name, len() is fixed, and geturl()
// reconstructs the URL.
func splitFields(r *splitResult) []string {
	return []string{r.scheme, r.netloc, r.path, r.query, r.fragment}
}

func parseFields(r *parseResult) []string {
	return []string{r.scheme, r.netloc, r.path, r.params, r.query, r.fragment}
}

func resultGetItem(fields []string, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__getitem__() takes exactly one argument")
	}
	i, err := py.IndexInt(args[0])
	if err != nil {
		return nil, err
	}
	if i < 0 {
		i += len(fields)
	}
	if i < 0 || i >= len(fields) {
		return nil, py.ExceptionNewf(py.IndexError, "tuple index out of range")
	}
	return py.String(fields[i]), nil
}

// resultList builds the field list.

func urllibResultGetURL(fields []string, hasParams bool) (py.Object, error) {
	if hasParams {
		path := fields[2]
		if fields[3] != "" {
			path = path + ";" + fields[3]
		}
		return py.String(urlUnsplit(fields[0], fields[1], path, fields[4], fields[5])), nil
	}
	return py.String(urlUnsplit(fields[0], fields[1], fields[2], fields[3], fields[4])), nil
}

// str and repr match CPython's named-tuple rendering.
func resultRepr(name string, names []string, fields []string) string {
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = names[i] + "=" + reprStr(f)
	}
	return name + "(" + strings.Join(parts, ", ") + ")"
}

// ---------------------------------------------------------------------------
// Type registration
// ---------------------------------------------------------------------------

func init() {
	registerResultFields(splitResultType, []string{"scheme", "netloc", "path", "query", "fragment"},
		func(self py.Object) []string { return splitFields(self.(*splitResult)) },
		func(self py.Object) (py.Object, error) {
			return urllibResultGetURL(splitFields(self.(*splitResult)), false)
		}, "SplitResult")
	registerResultFields(parseResultType, []string{"scheme", "netloc", "path", "params", "query", "fragment"},
		func(self py.Object) []string { return parseFields(self.(*parseResult)) },
		func(self py.Object) (py.Object, error) {
			return urllibResultGetURL(parseFields(self.(*parseResult)), true)
		}, "ParseResult")
	registerDefragResult()

	parseGlobals := py.NewStringDict()
	for _, m := range []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"urlsplit", urllibSplit, "urlsplit(url) -> SplitResult(scheme, netloc, path, query, fragment)"},
		{"urlparse", urllibParse, "urlparse(url) -> ParseResult(scheme, netloc, path, params, query, fragment)"},
		{"urlunparse", urllibUnparse, "urlunparse(parts) -> a URL string"},
		{"urlunsplit", urllibUnsplit, "urlunsplit(parts) -> a URL string"},
		{"urljoin", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return urllibJoin(self, args, kw)
		}, "urljoin(base, url) -> an absolute URL"},
		{"quote", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return urllibQuote(self, args, kw)
		}, "quote(string, safe='/') -> a percent-encoded string"},
		{"unquote", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return urllibUnquote(self, args, kw)
		}, "unquote(string) -> a percent-decoded string"},
		{"quote_plus", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return urllibQuotePlus(self, args, kw)
		}, "quote_plus(string, safe='') -> a percent-encoded string with '+' for space"},
		{"unquote_plus", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return urllibUnquotePlus(self, args, kw)
		}, "unquote_plus(string) -> a percent-decoded string with '+' as space"},
		{"urlencode", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return urllibUrlencode(self, args, kw)
		}, "urlencode(query) -> a '&'-separated percent-encoded string"},
		{"parse_qs", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return urllibParseQs(self, args, kw)
		}, "parse_qs(qs) -> a dict of key to value list"},
		{"parse_qsl", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return urllibParseQsl(self, args, kw)
		}, "parse_qsl(qs) -> a list of (key, value) pairs"},
		{"quote_from_bytes", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return urllibQuote(self, args, kw)
		}, "quote_from_bytes(bytes) -> a percent-encoded string"},
		{"urldefrag", urllibDefragNamed, "urldefrag(url) -> DefragResult(url_without_fragment, fragment)"},
	} {
		parseGlobals.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}
	parseGlobals.Set("MAX_CACHE_SIZE", py.Int(0))
	parseGlobals.Set("SplitResult", splitResultType)
	parseGlobals.Set("ParseResult", parseResultType)
	parseGlobals.Set("DefragResult", defragResultType)
	parseGlobals.Set("_ALWAYS_SAFE", py.String(alwaysSafe))
	parseGlobals.Set("_WHATWG_C0_CONTROL_OR_SPACE", py.String(""))
	// The scheme tables, as CPython 3.14 has them.  They are public and
	// MUTABLE: pip's VersionControl.__init__ does
	// "urllib.parse.uses_netloc.extend(self.schemes)" at import time, so their
	// absence stopped every "pip install" before it reached the network.
	for name, schemes := range map[string]string{
		"uses_relative":    ",ftp,http,gopher,nntp,imap,wais,file,https,shttp,mms,prospero,rtsp,rtsps,rtspu,sftp,svn,svn+ssh,ws,wss",
		"uses_netloc":      ",ftp,http,gopher,nntp,telnet,imap,wais,file,mms,https,shttp,snews,prospero,rtsp,rtsps,rtspu,rsync,svn,svn+ssh,sftp,nfs,git,git+ssh,ws,wss,itms-services",
		"uses_params":      ",ftp,hdl,prospero,http,imap,https,shttp,rtsp,rtsps,rtspu,sip,sips,mms,sftp,tel",
		"uses_query":       ",http,wais,imap,https,shttp,mms,gopher,rtsp,rtsps,rtspu,sip,sips",
		"uses_fragment":    ",ftp,hdl,http,gopher,news,nntp,wais,https,shttp,snews,file,prospero",
		"non_hierarchical": "gopher,hdl,mailto,news,telnet,wais,imap,snews,sip,sips",
	} {
		parts := strings.Split(schemes, ",")
		items := make([]py.Object, len(parts))
		for i, s := range parts {
			items[i] = py.String(s)
		}
		parseGlobals.Set(name, py.NewListFromItems(items))
	}
	parseGlobals.Set("scheme_chars", py.String("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+-."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "urllib.parse",
			Doc:  parse_doc,
		},
		Globals: parseGlobals,
	})

	// The parent urllib package, so "import urllib" works.
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "urllib",
			Doc:  "urllib package: URL handling modules.",
		},
		Globals: py.NewStringDict(),
	})
}

// registerResultFields installs the field properties, the sequence protocol
// and geturl() on a result type.
func registerResultFields(t *py.Type, names []string,
	fields func(py.Object) []string,
	geturl func(py.Object) (py.Object, error),
	typeName string) {
	for i, name := range names {
		idx := i
		nm := name
		t.Dict.Set(nm, &py.Property{Fget: func(self py.Object) (py.Object, error) {
			return py.String(fields(self)[idx]), nil
		}})
	}
	t.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return resultGetItem(fields(self), args)
	}, 0, "x[i]"))
	t.Dict.Set("__len__", py.MustNewMethod("__len__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(len(fields(self))), nil
	}, 0, "len(x)"))
	// __iter__ must return an iterator, not a list: the interpreter's next()
	// would otherwise have nothing to advance.  NewIterator walks __getitem__
	// and stops at IndexError, which this type raises past the end.
	t.Dict.Set("__iter__", py.MustNewMethod("__iter__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewIterator(self), nil
	}, 0, "iter(x)"))
	t.Dict.Set("__str__", py.MustNewMethod("__str__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String(resultRepr(typeName, names, fields(self))), nil
	}, 0, "str(x)"))
	t.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String(resultRepr(typeName, names, fields(self))), nil
	}, 0, "repr(x)"))
	t.Dict.Set("geturl", py.MustNewMethod("geturl", func(self py.Object, args py.Tuple) (py.Object, error) {
		return geturl(self)
	}, 0, "Return the re-combined URL."))
}

var _ = sort.Strings

// ---------------------------------------------------------------------------
// DefragResult
// ---------------------------------------------------------------------------

// defragResult is what urldefrag returns: the URL without its fragment and the
// fragment itself, exposed by name as well as by position.
type defragResult struct {
	url      string
	fragment string
	Dict     py.StringDict
}

var defragResultType = py.NewTypeX("urllib.parse.DefragResult",
	"A 2-tuple (url, fragment) returned by urldefrag().",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		r := &defragResult{Dict: py.NewStringDict()}
		var url, frag py.Object
		if err := py.ParseTupleAndKeywords(args, kwargs, "OO:DefragResult",
			[]string{"url", "fragment"}, &url, &frag); err != nil {
			return nil, err
		}
		r.url, _ = py.StrAsString(url)
		r.fragment, _ = py.StrAsString(frag)
		return r, nil
	}, nil)

func (r *defragResult) Type() *py.Type         { return defragResultType }
func (r *defragResult) GetDict() py.StringDict { return r.Dict }

func defragFields(r *defragResult) []string { return []string{r.url, r.fragment} }

// registerDefragResult installs the field accessors and protocol on the
// DefragResult type; the module init calls it.
func registerDefragResult() {
	registerResultFields(defragResultType, []string{"url", "fragment"},
		func(self py.Object) []string { return defragFields(self.(*defragResult)) },
		func(self py.Object) (py.Object, error) {
			r := self.(*defragResult)
			return py.String(urlUnsplit("", "", r.url, "", r.fragment)), nil
		}, "DefragResult")
}

func urllibDefragNamed(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var url py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:urldefrag",
		[]string{"url"}, &url); err != nil {
		return nil, err
	}
	u, err := py.StrAsString(url)
	if err != nil {
		return nil, err
	}
	i := strings.IndexByte(u, '#')
	if i < 0 {
		return &defragResult{url: u, fragment: "", Dict: py.NewStringDict()}, nil
	}
	return &defragResult{url: u[:i], fragment: u[i+1:], Dict: py.NewStringDict()}, nil
}
