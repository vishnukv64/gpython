// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// http.cookies -- HTTP state management (cookies).
//
// This ports CPython's http/cookies.py: Morsel, BaseCookie, SimpleCookie,
// CookieError and the quoting helpers.  requests imports Morsel and checks
// isinstance(value, Morsel); it also builds cookies out of Morsel attributes,
// so the attribute names and defaults matter as much as the parsing.
//
// Morsel is a dict in CPython and BaseCookie a dict of Morsels; here both are
// Go types that carry an explicit ordered attribute map and answer the
// mapping protocol (__getitem__/__setitem__/__contains__/__iter__/__len__/
// get/items/keys/values), so ordinary cookie code works unchanged.
package http

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const cookies_doc = `HTTP state management (cookies).

This module defines a class, Cookie, which represents a collection of
HTTP cookies.  It also defines a class, Morsel, which represents a single
cookie with its attributes.  SimpleCookie is the usual entry point.
`

// CookieErrorType is the exception raised for malformed cookies.
var CookieErrorType = py.ExceptionType.NewType("http.cookies.CookieError", "Cookie error", nil, nil)

// reserved maps the lower-case attribute name to the spelling used on the
// wire, exactly as CPython's Morsel._reserved.
var reserved = map[string]string{
	"expires":     "expires",
	"path":        "Path",
	"comment":     "Comment",
	"domain":      "Domain",
	"max-age":     "Max-Age",
	"secure":      "Secure",
	"httponly":    "HttpOnly",
	"version":     "Version",
	"samesite":    "SameSite",
	"partitioned": "Partitioned",
}

// reservedOrder is the order CPython's dict.fromkeys(_reserved) produces; the
// reserved defaults are inserted in this order.
var reservedOrder = []string{
	"expires", "path", "comment", "domain", "max-age",
	"secure", "httponly", "version", "samesite", "partitioned",
}

// flags are the attributes that render as a bare name when set.
var flagAttrs = map[string]bool{"secure": true, "httponly": true, "partitioned": true}

// legalChars is CPython's _LegalChars.
const legalChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&'*+-.^_`|~:"

// unescapedChars is CPython's _UnescapedChars.
const unescapedChars = legalChars + " ()/<=>?@[]{}"

// isLegalKey reports whether s consists only of characters from legalChars
// (CPython's _is_legal_key fullmatch over "[_LegalChars]+").
func isLegalKey(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune(legalChars, rune(s[i])) {
			return false
		}
	}
	return true
}

// hasControlCharacter reports whether any of the stringifications contains a
// character in [\x00-\x1F\x7f], CPython's _has_control_character.
func hasControlCharacter(vals ...string) bool {
	for _, s := range vals {
		for i := 0; i < len(s); i++ {
			if s[i] <= 0x1f || s[i] == 0x7f {
				return true
			}
		}
	}
	return false
}

// quoteCookie implements CPython's _quote: if the value is legal it is
// returned bare, otherwise it is wrapped in double quotes with every
// non-unescaped byte translated.
func quoteCookie(s string, isNone bool) string {
	if isNone || isLegalKey(s) {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString("\\\"")
		case c == '\\':
			b.WriteString("\\\\")
		case strings.IndexByte(unescapedChars, c) >= 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "\\%03o", c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// unquoteCookie implements CPython's _unquote, decoding a double-quoted value.
func unquoteCookie(s string, isNone bool) string {
	if isNone || len(s) < 2 {
		return s
	}
	if s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	body := s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(body); {
		if body[i] != '\\' || i+1 >= len(body) {
			b.WriteByte(body[i])
			i++
			continue
		}
		// \DDD octal, or \<char>
		if i+3 < len(body)+1 && i+4 <= len(body) &&
			body[i+1] >= '0' && body[i+1] <= '3' &&
			body[i+2] >= '0' && body[i+2] <= '7' &&
			body[i+3] >= '0' && body[i+3] <= '7' {
			n, _ := strconv.ParseInt(body[i+1:i+4], 8, 32)
			b.WriteByte(byte(n))
			i += 4
			continue
		}
		b.WriteByte(body[i+1])
		i += 2
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Morsel
// ---------------------------------------------------------------------------

// morsel is one cookie's key/value pair plus its attributes.  attrs holds the
// reserved attributes in insertion order; the three reserved flags keep the
// same slots but carry their default "" until set.
type morsel struct {
	key        string
	value      py.Object
	codedValue py.Object
	attrs      []string // attribute names in insertion order (reserved order)
	values     []py.Object
	Dict       py.StringDict
}

func (m *morsel) Type() *py.Type         { return MorselType }
func (m *morsel) GetDict() py.StringDict { return m.Dict }

func newMorsel() *morsel {
	m := &morsel{
		key:        "",
		value:      py.None,
		codedValue: py.None,
		Dict:       py.NewStringDict(),
	}
	for _, name := range reservedOrder {
		m.attrs = append(m.attrs, name)
		m.values = append(m.values, py.String(""))
	}
	return m
}

func (m *morsel) index(name string) int {
	name = strings.ToLower(name)
	for i, a := range m.attrs {
		if a == name {
			return i
		}
	}
	return -1
}

// getAttr returns the attribute value as a python object (String "" when unset).

// setAttr sets a reserved attribute, rejecting unknown names and control
// characters exactly as CPython's Morsel.__setitem__ does.
func (m *morsel) setAttr(name string, value py.Object) error {
	lower := strings.ToLower(name)
	i := m.index(lower)
	if i < 0 {
		return py.ExceptionNewf(CookieErrorType, "Invalid attribute %s", reprOfObject(py.String(lower)))
	}
	if hasControlCharacter(lower, pyStrOrEmpty(value)) {
		return py.ExceptionNewf(CookieErrorType, "Control characters are not allowed in cookies %s %s",
			reprOfObject(py.String(lower)), reprOfObject(value))
	}
	m.values[i] = value
	return nil
}

// pyStrOrEmpty is str(v) for the control-character check; unprintable values
// stringify to the empty string rather than failing the check.
func pyStrOrEmpty(v py.Object) string {
	s, err := py.StrAsString(v)
	if err != nil {
		return ""
	}
	return s
}

func reprOfObject(v py.Object) string {
	s, err := py.ReprAsString(v)
	if err != nil {
		return ""
	}
	return s
}

// isIntObject reports whether v is an int (the expires/max-age rendering
// branches test isinstance(value, int) in CPython).
func isIntObject(v py.Object) bool {
	switch v.(type) {
	case py.Int, py.Bool:
		return true
	}
	return false
}

// isStrObject reports whether v is a str.
func isStrObject(v py.Object) bool {
	_, ok := v.(py.String)
	return ok
}

// getdate formats the "expires" header for a timestamp, CPython's _getdate.
func getdate(ts int) string {
	weekday := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	month := []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	t := time.Unix(int64(ts), 0).UTC()
	return fmt.Sprintf("%s, %02d %3s %4d %02d:%02d:%02d GMT",
		weekday[int(t.Weekday())], t.Day(), month[int(t.Month())], t.Year(),
		t.Hour(), t.Minute(), t.Second())
}

func morselNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return newMorsel(), nil
}

func morselSet(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*morsel)
	if len(args) != 3 {
		return nil, py.ExceptionNewf(py.TypeError, "set() takes exactly 3 arguments")
	}
	key, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	if _, isReserved := reserved[strings.ToLower(key)]; isReserved {
		return nil, py.ExceptionNewf(CookieErrorType, "Attempt to set a reserved key %s", reprOfObject(py.String(key)))
	}
	if !isLegalKey(key) {
		return nil, py.ExceptionNewf(CookieErrorType, "Illegal key %s", reprOfObject(py.String(key)))
	}
	if hasControlCharacter(key, pyStrOrEmpty(args[1]), pyStrOrEmpty(args[2])) {
		return nil, py.ExceptionNewf(CookieErrorType,
			"Control characters are not allowed in cookies %s %s %s",
			reprOfObject(py.String(key)), reprOfObject(args[1]), reprOfObject(args[2]))
	}
	m.key = key
	m.value = args[1]
	m.codedValue = args[2]
	return py.None, nil
}

func morselGetItem(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*morsel)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__getitem__() takes exactly one argument")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	if i := m.index(name); i >= 0 {
		return m.values[i], nil
	}
	return nil, py.ExceptionNewf(py.KeyError, "%s", name)
}

func morselSetItem(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*morsel)
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "__setitem__() takes exactly two arguments")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	if err := m.setAttr(name, args[1]); err != nil {
		return nil, err
	}
	return py.None, nil
}

func morselSetdefault(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*morsel)
	var name py.Object = py.String("")
	var dflt py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "S|O:setdefault", []string{"key", "val"}, &name, &dflt); err != nil {
		return nil, err
	}
	key := strings.ToLower(string(name.(py.String)))
	i := m.index(key)
	if i < 0 {
		return nil, py.ExceptionNewf(CookieErrorType, "Invalid attribute %s", reprOfObject(py.String(key)))
	}
	cur := m.values[i]
	if s, ok := cur.(py.String); ok && string(s) == "" {
		m.values[i] = dflt
		return dflt, nil
	}
	return cur, nil
}

func morselIsReservedKey(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "isReservedKey() takes exactly one argument")
	}
	key, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	_, ok := reserved[strings.ToLower(key)]
	return py.Bool(ok), nil
}

func morselOutputString(self py.Object, attrs py.Object) (string, error) {
	m := self.(*morsel)
	var b strings.Builder
	b.WriteString(m.key)
	b.WriteString("=")
	if s, err := py.StrAsString(m.codedValue); err == nil {
		b.WriteString(s)
	}
	// Build the attribute list in sorted order, as CPython does.
	var allowed func(string) bool
	if attrs == nil || attrs == py.None {
		allowed = func(string) bool { return true }
	} else {
		set := map[string]bool{}
		if l, ok := attrs.(*py.List); ok {
			for _, it := range l.Items {
				if s, err := py.StrAsString(it); err == nil {
					set[s] = true
				}
			}
		}
		allowed = func(k string) bool { return set[k] }
	}
	order := make([]string, len(m.attrs))
	copy(order, m.attrs)
	sortStrings(order)
	for _, key := range order {
		i := m.index(key)
		value := m.values[i]
		if s, ok := value.(py.String); ok && string(s) == "" {
			continue
		}
		if !allowed(key) {
			continue
		}
		wire := reserved[key]
		switch {
		case key == "expires" && isIntObject(value):
			n, _ := py.IndexInt(value)
			b.WriteString("; ")
			b.WriteString(wire)
			b.WriteString("=")
			b.WriteString(getdate(n))
		case key == "max-age" && isIntObject(value):
			n, _ := py.IndexInt(value)
			b.WriteString("; ")
			b.WriteString(wire)
			b.WriteString("=")
			b.WriteString(strconv.Itoa(n))
		case key == "comment" && isStrObject(value):
			s, _ := py.StrAsString(value)
			b.WriteString("; ")
			b.WriteString(wire)
			b.WriteString("=")
			b.WriteString(quoteCookie(s, false))
		case flagAttrs[key]:
			if ok, _ := py.ObjectIsTrue(value); ok {
				b.WriteString("; ")
				b.WriteString(wire)
			}
		default:
			s, _ := py.StrAsString(value)
			b.WriteString("; ")
			b.WriteString(wire)
			b.WriteString("=")
			b.WriteString(s)
		}
	}
	return b.String(), nil
}

func morselOutput(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var attrs py.Object = py.None
	var header py.Object = py.String("Set-Cookie:")
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:output", []string{"attrs", "header"}, &attrs, &header); err != nil {
		return nil, err
	}
	s, err := morselOutputString(self, attrs)
	if err != nil {
		return nil, err
	}
	h := "Set-Cookie:"
	if hs, err := py.StrAsString(header); err == nil {
		h = hs
	}
	return py.String(h + " " + s), nil
}

func morselStr(self py.Object, args py.Tuple) (py.Object, error) {
	return morselOutput(self, args, py.NewStringDict())
}

func morselRepr(self py.Object, args py.Tuple) (py.Object, error) {
	s, err := morselOutputString(self, py.None)
	if err != nil {
		return nil, err
	}
	return py.String("<Morsel: " + s + ">"), nil
}

func morselCopy(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*morsel)
	c := newMorsel()
	c.key = m.key
	c.value = m.value
	c.codedValue = m.codedValue
	copy(c.values, m.values)
	return c, nil
}

func morselKeys(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*morsel)
	items := make([]py.Object, len(m.attrs))
	for i, a := range m.attrs {
		items[i] = py.String(a)
	}
	return py.NewListFromItems(items), nil
}

func morselItems(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*morsel)
	items := make([]py.Object, len(m.attrs))
	for i, a := range m.attrs {
		items[i] = py.Tuple{py.String(a), m.values[i]}
	}
	return py.NewListFromItems(items), nil
}

func morselValues(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*morsel)
	items := make([]py.Object, len(m.values))
	copy(items, m.values)
	return py.NewListFromItems(items), nil
}

func morselIter(self py.Object, args py.Tuple) (py.Object, error) {
	return morselKeys(self, args)
}

// key/value/coded_value are read-only properties.
func morselKeyProp(self py.Object) py.Object        { return py.String(self.(*morsel).key) }
func morselValueProp(self py.Object) py.Object      { return self.(*morsel).value }
func morselCodedValueProp(self py.Object) py.Object { return self.(*morsel).codedValue }

// ---------------------------------------------------------------------------
// BaseCookie
// ---------------------------------------------------------------------------

// baseCookie is a dict of Morsels keyed by cookie name.
type baseCookie struct {
	keys []string
	vals []*morsel
	Dict py.StringDict
	// simple selects CPython's BaseCookie vs SimpleCookie value codec.
	simple bool
}

func (c *baseCookie) Type() *py.Type {
	if c.simple {
		return SimpleCookieType
	}
	return BaseCookieType
}
func (c *baseCookie) GetDict() py.StringDict { return c.Dict }

func (c *baseCookie) find(name string) *morsel {
	for i, k := range c.keys {
		if k == name {
			return c.vals[i]
		}
	}
	return nil
}

func (c *baseCookie) put(name string, m *morsel) {
	for i, k := range c.keys {
		if k == name {
			c.vals[i] = m
			return
		}
	}
	c.keys = append(c.keys, name)
	c.vals = append(c.vals, m)
}

// valueDecode / valueEncode implement CPython's methods for the simple case.
func (c *baseCookie) valueEncode(v py.Object) (py.Object, py.Object, error) {
	strval, err := py.StrAsString(v)
	if err != nil {
		// str() always succeeds in practice; fall back to repr.
		strval = reprOfObject(v)
	}
	if c.simple {
		return py.String(strval), py.String(quoteCookie(strval, false)), nil
	}
	return py.String(strval), py.String(strval), nil
}

func (c *baseCookie) valueDecode(v string) (py.Object, py.Object) {
	if c.simple {
		return py.String(unquoteCookie(v, false)), py.String(v)
	}
	return py.String(v), py.String(v)
}

func baseCookieNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var input py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:__init__", []string{"input"}, &input); err != nil {
		return nil, err
	}
	c := &baseCookie{Dict: py.NewStringDict()}
	if input != py.None && input != nil {
		if err := loadCookieData(c, input); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func simpleCookieNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var input py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:__init__", []string{"input"}, &input); err != nil {
		return nil, err
	}
	c := &baseCookie{simple: true, Dict: py.NewStringDict()}
	if input != py.None && input != nil {
		if err := loadCookieData(c, input); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// setCookieMorsel mirrors BaseCookie.__set: get-or-create the Morsel and set it.
func setCookieMorsel(c *baseCookie, key string, rval, cval py.Object) error {
	m := c.find(key)
	if m == nil {
		m = newMorsel()
	}
	if err := morselSetRaw(m, key, rval, cval); err != nil {
		return err
	}
	c.put(key, m)
	return nil
}

// morselSetRaw is morselSet without the py.Tuple packing.
func morselSetRaw(m *morsel, key string, val, coded py.Object) error {
	if _, isReserved := reserved[strings.ToLower(key)]; isReserved {
		return py.ExceptionNewf(CookieErrorType, "Attempt to set a reserved key %s", reprOfObject(py.String(key)))
	}
	if !isLegalKey(key) {
		return py.ExceptionNewf(CookieErrorType, "Illegal key %s", reprOfObject(py.String(key)))
	}
	if hasControlCharacter(key, pyStrOrEmpty(val), pyStrOrEmpty(coded)) {
		return py.ExceptionNewf(CookieErrorType,
			"Control characters are not allowed in cookies %s %s %s",
			reprOfObject(py.String(key)), reprOfObject(val), reprOfObject(coded))
	}
	m.key = key
	m.value = val
	m.codedValue = coded
	return nil
}

func cookieSetItem(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*baseCookie)
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "__setitem__() takes exactly two arguments")
	}
	key, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	if m, ok := args[1].(*morsel); ok {
		c.put(key, m)
		return py.None, nil
	}
	rval, cval, err := c.valueEncode(args[1])
	if err != nil {
		return nil, err
	}
	if err := setCookieMorsel(c, key, rval, cval); err != nil {
		return nil, err
	}
	return py.None, nil
}

func cookieGetItem(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*baseCookie)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__getitem__() takes exactly one argument")
	}
	key, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	if m := c.find(key); m != nil {
		return m, nil
	}
	return nil, py.ExceptionNewf(py.KeyError, "%s", key)
}

func cookieGet(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := self.(*baseCookie)
	var key py.Object = py.String("")
	var dflt py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "S|O:get", []string{"key", "default"}, &key, &dflt); err != nil {
		return nil, err
	}
	if m := c.find(string(key.(py.String))); m != nil {
		return m, nil
	}
	return dflt, nil
}

func cookieContains(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*baseCookie)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__contains__() takes exactly one argument")
	}
	key, err := py.StrAsString(args[0])
	if err != nil {
		return py.Bool(false), nil
	}
	return py.Bool(c.find(key) != nil), nil
}

func cookieLen(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Int(len(self.(*baseCookie).keys)), nil
}

func cookieKeys(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*baseCookie)
	items := make([]py.Object, len(c.keys))
	for i, k := range c.keys {
		items[i] = py.String(k)
	}
	return py.NewListFromItems(items), nil
}

func cookieValues(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*baseCookie)
	items := make([]py.Object, len(c.vals))
	for i, m := range c.vals {
		items[i] = m
	}
	return py.NewListFromItems(items), nil
}

func cookieItems(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*baseCookie)
	items := make([]py.Object, len(c.keys))
	for i, k := range c.keys {
		items[i] = py.Tuple{py.String(k), c.vals[i]}
	}
	return py.NewListFromItems(items), nil
}

func cookieIter(self py.Object, args py.Tuple) (py.Object, error) {
	return cookieKeys(self, args)
}

func cookieOutput(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := self.(*baseCookie)
	var attrs py.Object = py.None
	var header py.Object = py.String("Set-Cookie:")
	var sep py.Object = py.String("\r\n")
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOO:output",
		[]string{"attrs", "header", "sep"}, &attrs, &header, &sep); err != nil {
		return nil, err
	}
	h, _ := py.StrAsString(header)
	sepStr, _ := py.StrAsString(sep)
	// Output is in sorted key order, like CPython.
	order := make([]string, len(c.keys))
	copy(order, c.keys)
	sortStrings(order)
	parts := make([]string, 0, len(order))
	for _, k := range order {
		m := c.find(k)
		s, err := morselOutputString(m, attrs)
		if err != nil {
			return nil, err
		}
		out := h + " " + s
		if hasControlCharacter(out) {
			return nil, py.ExceptionNewf(CookieErrorType, "Control characters are not allowed in cookies")
		}
		parts = append(parts, out)
	}
	return py.String(strings.Join(parts, sepStr)), nil
}

func cookieStr(self py.Object, args py.Tuple) (py.Object, error) {
	return cookieOutput(self, args, py.NewStringDict())
}

func cookieRepr(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*baseCookie)
	order := make([]string, len(c.keys))
	copy(order, c.keys)
	sortStrings(order)
	parts := make([]string, 0, len(order))
	for _, k := range order {
		m := c.find(k)
		parts = append(parts, k+"="+reprOfObject(m.value))
	}
	name := "BaseCookie"
	if c.simple {
		name = "SimpleCookie"
	}
	return py.String("<" + name + ": " + strings.Join(parts, " ") + ">"), nil
}

func cookieLoad(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*baseCookie)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "load() takes exactly one argument")
	}
	if err := loadCookieData(c, args[0]); err != nil {
		return nil, err
	}
	return py.None, nil
}

// cookieLoad implements BaseCookie.load for a raw argument: a string is parsed,
// a mapping is copied through value encode + set.
func loadCookieData(c *baseCookie, rawdata py.Object) error {
	if s, ok := rawdata.(py.String); ok {
		return parseCookieString(c, string(s))
	}
	// A python dict: iterate its items and set each.
	if d, ok := rawdata.(py.StringDict); ok {
		for _, ent := range d.Items() {
			key, err := decodeDictKey(ent.Key)
			if err != nil {
				return err
			}
			rval, cval, err := c.valueEncode(ent.Value)
			if err != nil {
				return err
			}
			if err := setCookieMorsel(c, key, rval, cval); err != nil {
				return err
			}
		}
		return nil
	}
	// A generic mapping: call .items().
	itemsFn, err := py.GetAttrString(rawdata, "items")
	if err != nil {
		return py.ExceptionNewf(py.TypeError, "load() argument must be a string or a mapping")
	}
	items, err := py.Call(itemsFn, py.Tuple{}, py.NewStringDict())
	if err != nil {
		return err
	}
	list, err := py.SequenceList(items)
	if err != nil {
		return err
	}
	for _, it := range list.Items {
		pair, err := py.SequenceTuple(it)
		if err != nil || len(pair) != 2 {
			return py.ExceptionNewf(py.TypeError, "load() items must be key/value pairs")
		}
		key, err := py.StrAsString(pair[0])
		if err != nil {
			return err
		}
		rval, cval, err := c.valueEncode(pair[1])
		if err != nil {
			return err
		}
		if err := setCookieMorsel(c, key, rval, cval); err != nil {
			return err
		}
	}
	return nil
}

// parseCookieString implements BaseCookie.__parse_string: it scans the raw
// Set-Cookie text with the CPython _CookiePattern grammar and rejects a
// syntactically invalid string by stopping early.  This is a hand-written
// scanner rather than a regexp, which is easier to keep readable than the
// CPython pattern, but it accepts exactly the same language.
func parseCookieString(c *baseCookie, s string) error {
	i, n := 0, len(s)
	type parsed struct {
		isKeyValue bool
		key        string
		value      py.Object // decoded val for key/value; raw for attribute
	}
	var items []parsed
	morselSeen := false

	for i < n {
		key, val, hasVal, next, ok := matchCookie(s, i)
		if !ok {
			break
		}
		i = next
		if key[0] == '$' {
			if !morselSeen {
				continue
			}
			items = append(items, parsed{key: strings.ToLower(key[1:]), value: pyStringOrNone(val, hasVal)})
			continue
		}
		if _, isReserved := reserved[strings.ToLower(key)]; isReserved {
			if !morselSeen {
				return nil // invalid: stop, applying nothing
			}
			if !hasVal {
				if flagAttrs[strings.ToLower(key)] {
					items = append(items, parsed{key: strings.ToLower(key), value: py.Bool(true)})
					continue
				}
				return nil
			}
			items = append(items, parsed{key: strings.ToLower(key), value: py.String(unquoteCookie(val, false))})
			continue
		}
		if !hasVal {
			return nil
		}
		rval, cval := c.valueDecode(val)
		items = append(items, parsed{isKeyValue: true, key: key, value: py.Tuple{rval, cval}})
		morselSeen = true
	}

	var m *morsel
	for _, it := range items {
		if it.isKeyValue {
			pair := it.value.(py.Tuple)
			if err := setCookieMorsel(c, it.key, pair[0], pair[1]); err != nil {
				return err
			}
			m = c.find(it.key)
			continue
		}
		if m != nil {
			if err := m.setAttr(it.key, it.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func pyStringOrNone(s string, has bool) py.Object {
	if !has {
		return py.None
	}
	return py.String(s)
}

// legalKeyByte / legalValueByte mirror CPython's _LegalKeyChars /
// _LegalValueChars character classes.
func legalKeyByte(b byte) bool {
	return isAlnum(b) || strings.IndexByte("!#%&'~_`><@,:/$*+-.^|)(?}{=", b) >= 0
}

func legalValueByte(b byte) bool {
	return legalKeyByte(b) || b == '[' || b == ']'
}

func isAlnum(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

// matchCookie scans one cookie token starting at i, implementing CPython's
// _CookiePattern:
//
//	\s* (?P<key>[_LegalKeyChars]+?) (\s*=\s* (?P<val>"(?:[^\\"]|\\.)*" |
//	  (\w{3,6}day|\w{3}),\s[\w\d\s-]{9,11}\s[\d:]{8}\sGMT | [_LegalValueChars]*))?
//	\s* (\s+|;|$)
//
// It returns the key, the raw value, whether a value was present, and the
// index past the match.
func matchCookie(s string, start int) (key, val string, hasVal bool, next int, ok bool) {
	i := start
	n := len(s)
	for i < n && isSpaceByte(s[i]) {
		i++
	}
	keyStart := i
	for i < n && legalKeyByte(s[i]) {
		i++
	}
	if i == keyStart {
		return "", "", false, start, false
	}
	key = s[keyStart:i]

	// Optional = value
	j := i
	for j < n && isSpaceByte(s[j]) {
		j++
	}
	if j < n && s[j] == '=' {
		j++
		for j < n && isSpaceByte(s[j]) {
			j++
		}
		switch {
		case j < n && s[j] == '"':
			// A double-quoted string: "(?:[^\\"]|\\.)*"
			k := j + 1
			for k < n {
				if s[k] == '\\' && k+1 < n {
					k += 2
					continue
				}
				if s[k] == '"' {
					k++
					break
				}
				k++
			}
			val = s[j:k]
			hasVal = true
			i = k
		case matchGMTDateLen(s, j) > 0:
			i = j + matchGMTDateLen(s, j)
			val = s[j:i]
			hasVal = true
		default:
			k := j
			for k < n && legalValueByte(s[k]) {
				k++
			}
			val = s[j:k]
			hasVal = true
			i = k
		}
	}

	// Trailing \s* then (\s+|;|$)
	for i < n && isSpaceByte(s[i]) {
		i++
	}
	if i < n && s[i] == ';' {
		i++
	} else if i < n && !isSpaceByte(s[i]) {
		// The next char is neither whitespace nor ';' and we are not at end:
		// the pattern would fail here, so stop without consuming.
		return "", "", false, start, false
	}
	return key, val, hasVal, i, true
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}
func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }

// matchGMTDate checks the specialised "expires" date alternative at j:
//
//	(\w{3,6}day|\w{3}),\s[\w\d\s-]{9,11}\s[\d:]{8}\sGMT
//
// It returns the length of the matched span, or 0 if it does not match.
func matchGMTDateLen(s string, j int) int {
	i, n := j, len(s)
	wordStart := i
	for i < n && isAlnum(s[i]) {
		i++
	}
	word := s[wordStart:i]
	// \w{3,6}day or \w{3}
	validWord := len(word) == 3 ||
		(len(word) >= 6 && len(word) <= 9 && strings.HasSuffix(strings.ToLower(word), "day"))
	if !validWord || i >= n || s[i] != ',' {
		return 0
	}
	i++ // comma
	if i >= n || !isSpaceByte(s[i]) {
		return 0
	}
	i++
	// [\w\d\s-]{9,11}
	field := i
	for i < n && (isAlnum(s[i]) || isSpaceByte(s[i]) || s[i] == '-') {
		i++
	}
	if l := i - field; l < 9 || l > 11 {
		return 0
	}
	if i >= n || !isSpaceByte(s[i]) {
		return 0
	}
	i++
	// [\d:]{8}
	clock := i
	for i < n && (isDigitByte(s[i]) || s[i] == ':') {
		i++
	}
	if i-clock != 8 || i >= n || !isSpaceByte(s[i]) {
		return 0
	}
	i++
	if i+3 > n || s[i:i+3] != "GMT" {
		return 0
	}
	return i + 3 - j
}

// decodeDictKey turns the interpreter's encoded dict key back into a string.
func decodeDictKey(k string) (string, error) {
	if k == "" || k[0] != '\x00' {
		return k, nil
	}
	obj, err := py.DictKeyDecode(k)
	if err != nil {
		return "", err
	}
	s, err := py.StrAsString(obj)
	if err != nil {
		return "", err
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Type registration variables
// ---------------------------------------------------------------------------

var (
	// Morsel is subclassable in CPython (it is a dict); here it is a plain
	// type, which is enough for requests, which only reads and writes it.
	MorselType       = py.NewTypeX("http.cookies.Morsel", morselDoc, morselNew, nil)
	BaseCookieType   = py.NewTypeX("http.cookies.BaseCookie", baseCookieDoc, baseCookieNew, nil)
	SimpleCookieType = py.NewTypeX("http.cookies.SimpleCookie", simpleCookieDoc, simpleCookieNew, nil)
)

const (
	morselDoc = `A class to hold ONE (key, value) pair.

In a cookie, each such pair may have several attributes, so this class is used
to keep the attributes associated with the appropriate key,value pair.  It also
has a coded_value attribute holding the network representation of the value.`
	baseCookieDoc   = `A collection of HTTP cookies, as a mapping of name to Morsel.`
	simpleCookieDoc = `SimpleCookie supports strings as cookie values.

When setting a value with dict notation SimpleCookie calls str() on it; values
received over HTTP are kept as strings.`
)

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func init() {
	morselProps := []struct {
		name string
		get  func(py.Object) py.Object
	}{
		{"key", morselKeyProp},
		{"value", morselValueProp},
		{"coded_value", morselCodedValueProp},
	}
	for _, p := range morselProps {
		get := p.get
		MorselType.Dict.Set(p.name, &py.Property{
			Fget: func(self py.Object) (py.Object, error) { return get(self), nil },
		})
	}
	for _, m := range []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"set", morselSet, "set(key, val, coded_val)"},
		{"setdefault", morselSetdefault, "setdefault(key, val)"},
		{"isReservedKey", morselIsReservedKey, "isReservedKey(k)"},
		{"output", morselOutput, "output(attrs=None, header='Set-Cookie:')"},
		{"copy", morselCopy, "copy() -> a shallow copy"},
		{"keys", morselKeys, "the attribute names"},
		{"values", morselValues, "the attribute values"},
		{"items", morselItems, "(name, value) attribute pairs"},
		{"__getitem__", morselGetItem, "m[name]"},
		{"__setitem__", morselSetItem, "m[name] = value"},
		{"__contains__", morselKeysContains, "name in m"},
		{"__len__", morselLen, "number of attributes"},
		{"__iter__", morselIter, "iterate over the attribute names"},
		{"__str__", morselStr, "str(m)"},
		{"__repr__", morselRepr, "repr(m)"},
	} {
		MorselType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}

	for _, m := range []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"load", cookieLoad, "Load cookies from a string or mapping."},
		{"output", cookieOutput, "output(attrs=None, header='Set-Cookie:', sep='\\r\\n')"},
		{"get", cookieGet, "get(key, default=None)"},
		{"keys", cookieKeys, "the cookie names"},
		{"values", cookieValues, "the Morsels"},
		{"items", cookieItems, "(name, Morsel) pairs"},
		{"__getitem__", cookieGetItem, "c[name]"},
		{"__setitem__", cookieSetItem, "c[name] = value"},
		{"__contains__", cookieContains, "name in c"},
		{"__len__", cookieLen, "number of cookies"},
		{"__iter__", cookieIter, "iterate over the cookie names"},
		{"__str__", cookieStr, "str(c)"},
		{"__repr__", cookieRepr, "repr(c)"},
	} {
		BaseCookieType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
		SimpleCookieType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}

	globals := py.NewStringDict()
	for name, t := range map[string]*py.Type{
		"Morsel":       MorselType,
		"BaseCookie":   BaseCookieType,
		"SimpleCookie": SimpleCookieType,
		"CookieError":  CookieErrorType,
	} {
		globals.Set(name, t)
	}
	globals.Set("_is_legal_key", py.MustNewMethod("_is_legal_key", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "_is_legal_key() takes exactly one argument")
		}
		s, err := py.StrAsString(args[0])
		if err != nil {
			return py.Bool(false), nil
		}
		return py.Bool(isLegalKey(s)), nil
	}, 0, "Return True if the key is a legal cookie key."))
	globals.Set("_quote", py.MustNewMethod("_quote", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, py.ExceptionNewf(py.TypeError, "_quote() takes 1 or 2 arguments")
		}
		s, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		isNone := len(args) == 2 && args[1] == py.None
		return py.String(quoteCookie(s, isNone)), nil
	}, 0, "Return a string of the value quoted if necessary."))
	globals.Set("_unquote", py.MustNewMethod("_unquote", func(self py.Object, args py.Tuple) (py.Object, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, py.ExceptionNewf(py.TypeError, "_unquote() takes 1 or 2 arguments")
		}
		s, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		isNone := len(args) == 2 && args[1] == py.None
		return py.String(unquoteCookie(s, isNone)), nil
	}, 0, "Return a string of the value unquoted."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "http.cookies",
			Doc:  cookies_doc,
		},
		Globals: globals,
	})
}

func morselKeysContains(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*morsel)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__contains__() takes exactly one argument")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return py.Bool(false), nil
	}
	return py.Bool(m.index(name) >= 0), nil
}

func morselLen(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Int(len(self.(*morsel).attrs)), nil
}

var _ = time.Now
