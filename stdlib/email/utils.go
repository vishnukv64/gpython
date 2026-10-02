// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package email

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const utils_doc = `email.utils - miscellaneous email utilities.

Address handling (parseaddr, formataddr, getaddresses), RFC 2822 date
formatting and parsing (formatdate, parsedate, parsedate_tz, mktime_tz) and
the RFC 2231 parameter helpers.`

// The address parser is a faithful port of CPython's email._parseaddr
// AddrlistClass.  It is a character-level state machine over the header field
// and has to match CPython's output for the awkward inputs (comments, quoted
// strings, route addresses, domain literals) byte for byte, which porting the
// original is the only reliable way to do.

const (
	emptyString = ""
	space       = " "
	commaSpace  = ", "
)

var (
	dayNames   = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	monthNames = []string{
		"jan", "feb", "mar", "apr", "may", "jun", "jul",
		"aug", "sep", "oct", "nov", "dec",
		"january", "february", "march", "april", "may", "june", "july",
		"august", "september", "october", "november", "december",
	}
	timezones = map[string]int{
		"UT": 0, "UTC": 0, "GMT": 0, "Z": 0,
		"AST": -400, "ADT": -300,
		"EST": -500, "EDT": -400,
		"CST": -600, "CDT": -500,
		"MST": -700, "MDT": -600,
		"PST": -800, "PDT": -700,
	}
)

// addrlist is CPython's AddrlistClass.
type addrlist struct {
	specials, LWS, CR, FWS, atomends, phraseends string
	field                                        string
	pos                                          int
	commentlist                                  []string
}

func newAddrlist(field string) *addrlist {
	a := &addrlist{
		specials: "()<>@,:;.\"[]",
		LWS:      " \t",
		CR:       "\r\n",
		field:    field,
	}
	a.FWS = a.LWS + a.CR
	a.atomends = a.specials + a.LWS + a.CR
	a.phraseends = strings.ReplaceAll(a.atomends, ".", "")
	return a
}

func (a *addrlist) gotonext() string {
	var wslist []string
	for a.pos < len(a.field) {
		c := a.field[a.pos]
		switch {
		case strings.IndexByte(a.LWS+"\n\r", c) >= 0:
			if c != '\n' && c != '\r' {
				wslist = append(wslist, string(c))
			}
			a.pos++
		case c == '(':
			a.commentlist = append(a.commentlist, a.getcomment())
		default:
			return strings.Join(wslist, emptyString)
		}
	}
	return strings.Join(wslist, emptyString)
}

func (a *addrlist) getaddrlist() [][2]string {
	result := [][2]string{}
	for a.pos < len(a.field) {
		ad := a.getaddress()
		if len(ad) > 0 {
			result = append(result, ad...)
		} else {
			result = append(result, [2]string{"", ""})
		}
	}
	return result
}

func (a *addrlist) getaddress() [][2]string {
	a.commentlist = nil
	a.gotonext()

	oldpos := a.pos
	oldcl := append([]string(nil), a.commentlist...)
	plist := a.getphraselist()

	a.gotonext()
	var returnlist [][2]string

	switch {
	case a.pos >= len(a.field):
		if len(plist) > 0 {
			returnlist = [][2]string{{strings.Join(a.commentlist, space), plist[0]}}
		}
	case strings.IndexByte(".@", a.field[a.pos]) >= 0:
		a.pos = oldpos
		a.commentlist = oldcl
		addrspec := a.getaddrspec()
		returnlist = [][2]string{{strings.Join(a.commentlist, space), addrspec}}
	case a.field[a.pos] == ':':
		fieldlen := len(a.field)
		a.pos++
		for a.pos < len(a.field) {
			a.gotonext()
			if a.pos < fieldlen && a.field[a.pos] == ';' {
				a.pos++
				break
			}
			returnlist = append(returnlist, a.getaddress()...)
		}
	case a.field[a.pos] == '<':
		routeaddr := a.getrouteaddr()
		if len(a.commentlist) > 0 {
			returnlist = [][2]string{{
				strings.Join(plist, space) + " (" + strings.Join(a.commentlist, space) + ")",
				routeaddr,
			}}
		} else {
			returnlist = [][2]string{{strings.Join(plist, space), routeaddr}}
		}
	default:
		if len(plist) > 0 {
			returnlist = [][2]string{{strings.Join(a.commentlist, space), plist[0]}}
		} else if strings.IndexByte(a.specials, a.field[a.pos]) >= 0 {
			a.pos++
		}
	}

	a.gotonext()
	if a.pos < len(a.field) && a.field[a.pos] == ',' {
		a.pos++
	}
	return returnlist
}

func (a *addrlist) getrouteaddr() string {
	if a.field[a.pos] != '<' {
		return ""
	}
	expectroute := false
	a.pos++
	a.gotonext()
	adlist := ""
	for a.pos < len(a.field) {
		if expectroute {
			a.getdomain()
			expectroute = false
		} else if a.field[a.pos] == '>' {
			a.pos++
			break
		} else if a.field[a.pos] == '@' {
			a.pos++
			expectroute = true
		} else if a.field[a.pos] == ':' {
			a.pos++
		} else {
			adlist = a.getaddrspec()
			a.pos++
			break
		}
		a.gotonext()
	}
	return adlist
}

func (a *addrlist) getaddrspec() string {
	aslist := []string{}
	a.gotonext()
	for a.pos < len(a.field) {
		preserveWS := true
		if a.field[a.pos] == '.' {
			aslist = dropTrailingWS(aslist)
			aslist = append(aslist, ".")
			a.pos++
			preserveWS = false
		} else if a.field[a.pos] == '"' {
			aslist = append(aslist, "\""+quoteString(a.getquote())+"\"")
		} else if strings.IndexByte(a.atomends, a.field[a.pos]) >= 0 {
			aslist = dropTrailingWS(aslist)
			break
		} else {
			aslist = append(aslist, a.getatom(a.atomends))
		}
		ws := a.gotonext()
		if preserveWS && ws != "" {
			aslist = append(aslist, ws)
		}
	}

	if a.pos >= len(a.field) || a.field[a.pos] != '@' {
		return strings.Join(aslist, emptyString)
	}
	aslist = append(aslist, "@")
	a.pos++
	a.gotonext()
	domain := a.getdomain()
	if domain == "" {
		return emptyString
	}
	return strings.Join(aslist, emptyString) + domain
}

func dropTrailingWS(list []string) []string {
	if len(list) > 0 && strings.TrimSpace(list[len(list)-1]) == "" {
		return list[:len(list)-1]
	}
	return list
}

func (a *addrlist) getdomain() string {
	sdlist := []string{}
	for a.pos < len(a.field) {
		c := a.field[a.pos]
		switch {
		case strings.IndexByte(a.LWS, c) >= 0:
			a.pos++
		case c == '(':
			a.commentlist = append(a.commentlist, a.getcomment())
		case c == '[':
			sdlist = append(sdlist, a.getdomainliteral())
		case c == '.':
			a.pos++
			sdlist = append(sdlist, ".")
		case c == '@':
			return emptyString
		case strings.IndexByte(a.atomends, c) >= 0:
			return strings.Join(sdlist, emptyString)
		default:
			sdlist = append(sdlist, a.getatom(a.atomends))
		}
	}
	return strings.Join(sdlist, emptyString)
}

func (a *addrlist) getdelimited(beginchar byte, endchars string, allowcomments bool) string {
	if a.field[a.pos] != beginchar {
		return ""
	}
	slist := []string{""}
	quoted := false
	a.pos++
	for a.pos < len(a.field) {
		switch {
		case quoted:
			slist = append(slist, string(a.field[a.pos]))
			quoted = false
		case strings.IndexByte(endchars, a.field[a.pos]) >= 0:
			a.pos++
			return strings.Join(slist, emptyString)
		case allowcomments && a.field[a.pos] == '(':
			slist = append(slist, a.getcomment())
			continue
		case a.field[a.pos] == '\\':
			quoted = true
		default:
			slist = append(slist, string(a.field[a.pos]))
		}
		a.pos++
	}
	return strings.Join(slist, emptyString)
}

func (a *addrlist) getquote() string   { return a.getdelimited('"', "\"\r", false) }
func (a *addrlist) getcomment() string { return a.getdelimited('(', ")\r", true) }
func (a *addrlist) getdomainliteral() string {
	return "[" + a.getdelimited('[', "]\r", false) + "]"
}

func (a *addrlist) getatom(atomends string) string {
	atomlist := []string{""}
	for a.pos < len(a.field) {
		if strings.IndexByte(atomends, a.field[a.pos]) >= 0 {
			break
		}
		atomlist = append(atomlist, string(a.field[a.pos]))
		a.pos++
	}
	return strings.Join(atomlist, emptyString)
}

func (a *addrlist) getphraselist() []string {
	plist := []string{}
	for a.pos < len(a.field) {
		c := a.field[a.pos]
		switch {
		case strings.IndexByte(a.FWS, c) >= 0:
			a.pos++
		case c == '"':
			plist = append(plist, a.getquote())
		case c == '(':
			a.commentlist = append(a.commentlist, a.getcomment())
		case strings.IndexByte(a.phraseends, c) >= 0:
			return plist
		default:
			plist = append(plist, a.getatom(a.phraseends))
		}
	}
	return plist
}

// quoteString is email._parseaddr.quote.
func quoteString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "\"", "\\\"")
}

// unquote is email.utils.unquote.
func unquote(s string) string {
	if len(s) > 1 {
		if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
			inner := s[1 : len(s)-1]
			inner = strings.ReplaceAll(inner, "\\\\", "\\")
			return strings.ReplaceAll(inner, "\\\"", "\"")
		}
		if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// --- validation, a port of email.utils' strict-parsing helpers ------------

func iterEscapedChars(addr string) []int {
	// Indices of characters, with an escaped pair collapsed to its first index.
	out := make([]int, 0, len(addr))
	escape := false
	for i := 0; i < len(addr); i++ {
		ch := addr[i]
		if escape {
			out = append(out, i)
			escape = false
		} else if ch == '\\' {
			escape = true
		} else {
			out = append(out, i)
		}
	}
	if escape {
		out = append(out, len(addr)-1)
	}
	return out
}

func stripQuotedRealnames(addr string) string {
	if !strings.Contains(addr, "\"") {
		return addr
	}
	start := 0
	openPos := -1
	var result []string
	for _, pos := range iterEscapedChars(addr) {
		if addr[pos] == '"' {
			if openPos < 0 {
				openPos = pos
			} else {
				if start != openPos {
					result = append(result, addr[start:openPos])
				}
				start = pos + 1
				openPos = -1
			}
		}
	}
	if start < len(addr) {
		result = append(result, addr[start:])
	}
	return strings.Join(result, emptyString)
}

func checkParenthesis(addr string) bool {
	addr = stripQuotedRealnames(addr)
	opens := 0
	for _, pos := range iterEscapedChars(addr) {
		switch addr[pos] {
		case '(':
			opens++
		case ')':
			opens--
			if opens < 0 {
				return false
			}
		}
	}
	return opens == 0
}

func preParseValidation(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if !checkParenthesis(v) {
			v = "('', '')"
		}
		out = append(out, v)
	}
	return out
}

func postParseValidation(vals [][2]string) [][2]string {
	out := make([][2]string, 0, len(vals))
	for _, v := range vals {
		if strings.Contains(v[1], "[") {
			v = [2]string{"", ""}
		}
		out = append(out, v)
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func atoi(s string) (int, error) {
	if s == "" {
		return 0, py.ExceptionNewf(py.ValueError, "invalid literal for int() with base 10: ''")
	}
	neg := false
	i := 0
	if s[0] == '+' || s[0] == '-' {
		neg = s[0] == '-'
		i = 1
	}
	n := 0
	if i >= len(s) {
		return 0, py.ExceptionNewf(py.ValueError, "invalid literal for int() with base 10: '%s'", s)
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, py.ExceptionNewf(py.ValueError, "invalid literal for int() with base 10: '%s'", s)
		}
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}

// --- Python-facing functions ---------------------------------------------

func pyStr(o py.Object) string {
	s, _ := py.StrAsString(o)
	return s
}

func truthy(o py.Object) bool {
	b, err := py.MakeBool(o)
	if err != nil {
		return false
	}
	return b == py.True
}

func parseaddrFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		addr   py.Object
		strict py.Object = py.True
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|$p:parseaddr",
		[]string{"addr", "strict"}, &addr, &strict); err != nil {
		return nil, err
	}
	if !truthy(strict) {
		addrs := newAddrlist(pyStr(addr)).getaddrlist()
		if len(addrs) == 0 {
			return addrTuple("", ""), nil
		}
		return addrTuple(addrs[0][0], addrs[0][1]), nil
	}
	if l, ok := addr.(*py.List); ok && len(l.Items) > 0 {
		addr = l.Items[0]
	} else if t, ok := addr.(py.Tuple); ok && len(t) > 0 {
		addr = t[0]
	}
	s, ok := addr.(py.String)
	if !ok {
		return addrTuple("", ""), nil
	}
	valid := preParseValidation([]string{string(s)})[0]
	addrs := postParseValidation(newAddrlist(valid).getaddrlist())
	if len(addrs) != 1 {
		return addrTuple("", ""), nil
	}
	return addrTuple(addrs[0][0], addrs[0][1]), nil
}

func getaddressesFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		fieldvalues py.Object
		strict      py.Object = py.True
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|$p:getaddresses",
		[]string{"fieldvalues", "strict"}, &fieldvalues, &strict); err != nil {
		return nil, err
	}
	items, err := py.SequenceList(fieldvalues)
	if err != nil {
		return nil, err
	}
	strs := make([]string, 0, len(items.Items))
	for _, it := range items.Items {
		strs = append(strs, pyStr(it))
	}

	if !truthy(strict) {
		addrs := newAddrlist(strings.Join(strs, commaSpace)).getaddrlist()
		return addrListToPython(addrs), nil
	}

	prepared := preParseValidation(strs)
	addrs := postParseValidation(newAddrlist(strings.Join(prepared, commaSpace)).getaddrlist())

	want := 0
	for _, v := range prepared {
		want += 1 + strings.Count(stripQuotedRealnames(v), ",")
	}
	if len(addrs) != want {
		return py.NewListFromItems([]py.Object{addrTuple("", "")}), nil
	}
	return addrListToPython(addrs), nil
}

func addrTuple(name, addr string) py.Object {
	return py.Tuple{py.String(name), py.String(addr)}
}

func addrListToPython(addrs [][2]string) py.Object {
	items := make([]py.Object, 0, len(addrs))
	for _, a := range addrs {
		items = append(items, addrTuple(a[0], a[1]))
	}
	return py.NewListFromItems(items)
}

func hasSpecial(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return strings.ContainsRune(`[]\()<>@,:;".`, r)
	}) >= 0
}

var escapeReplacer = strings.NewReplacer("\\", "\\\\", "\"", "\\\"")

func formataddrFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		pair    py.Object
		charset py.Object = py.String("utf-8")
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|z:formataddr",
		[]string{"pair", "charset"}, &pair, &charset); err != nil {
		return nil, err
	}
	p, err := py.SequenceTuple(pair)
	if err != nil {
		return nil, err
	}
	if len(p) != 2 {
		return nil, py.ExceptionNewf(py.IndexError, "formataddr() expects a 2-tuple")
	}
	name, address := pyStr(p[0]), pyStr(p[1])
	if !isASCII(address) {
		return nil, py.ExceptionNewf(py.UnicodeEncodeError, "'ascii' codec can't encode characters in address")
	}
	if name == "" {
		return py.String(address), nil
	}
	if !isASCII(name) {
		// CPython routes a non-ASCII realname through email.charset.Charset,
		// which performs RFC 2047 encoded-word construction.  That header
		// encoder is not implemented here, so this raises rather than emitting
		// a header that is silently wrong.  [architectural limit]
		return nil, py.ExceptionNewf(py.UnicodeEncodeError,
			"non-ASCII realname requires the email.charset header encoder, which is not implemented")
	}
	quotes := ""
	if hasSpecial(name) {
		quotes = "\""
	}
	name = escapeReplacer.Replace(name)
	return py.String(quotes + name + quotes + " <" + address + ">"), nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

func quoteFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if err := py.UnpackTuple(args, kwargs, "quote", 1, 1, &s); err != nil {
		return nil, err
	}
	return py.String(quoteString(pyStr(s))), nil
}

func unquoteFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if err := py.UnpackTuple(args, kwargs, "unquote", 1, 1, &s); err != nil {
		return nil, err
	}
	return py.String(unquote(pyStr(s))), nil
}

// --- dates ---------------------------------------------------------------

// parsedateTz is email._parseaddr._parsedate_tz, returning the 10-element
// tuple or nil.  It returns py.Object elements so the timezone slot can be
// None exactly where CPython leaves it None.
func parsedateTz(data string) []py.Object {
	fields := strings.Fields(data)
	if len(fields) == 0 {
		return nil
	}
	// The FWS after the comma following the day-of-week is optional.
	if strings.HasSuffix(fields[0], ",") ||
		contains(dayNames, strings.ToLower(fields[0])) {
		fields = fields[1:]
	} else if i := strings.LastIndex(fields[0], ","); i >= 0 {
		fields[0] = fields[0][i+1:]
	}
	if len(fields) == 3 { // RFC 850 date, deprecated
		stuff := strings.Split(fields[0], "-")
		if len(stuff) == 3 {
			fields = append(append([]string{}, stuff...), fields[1:]...)
		}
	}
	if len(fields) == 4 {
		s := fields[3]
		i := strings.IndexAny(s, "+-")
		if i > 0 {
			fields = append([]string{}, fields[:3]...)
			fields = append(fields, s[:i], s[i:])
		} else {
			fields = append(fields, "")
		}
	}
	if len(fields) < 5 {
		return nil
	}
	fields = fields[:5]
	dd, mm, yy, tm, tz := fields[0], fields[1], fields[2], fields[3], fields[4]
	if dd == "" || mm == "" || yy == "" {
		return nil
	}
	mm = strings.ToLower(mm)
	if !contains(monthNames, mm) {
		dd, mm = mm, strings.ToLower(dd)
		if !contains(monthNames, mm) {
			return nil
		}
	}
	month := monthNumber(mm)
	if strings.HasSuffix(dd, ",") {
		dd = dd[:len(dd)-1]
	}
	if i := strings.Index(yy, ":"); i > 0 {
		yy, tm = tm, yy
	}
	if strings.HasSuffix(yy, ",") {
		yy = yy[:len(yy)-1]
		if yy == "" {
			return nil
		}
	}
	if yy[0] < '0' || yy[0] > '9' {
		yy, tz = tz, yy
	}
	if strings.HasSuffix(tm, ",") {
		tm = tm[:len(tm)-1]
	}
	thh, tmm, tss, ok := splitTime(tm)
	if !ok {
		return nil
	}
	yyN, err1 := atoi(yy)
	ddN, err2 := atoi(dd)
	thhN, err3 := atoi(thh)
	tmmN, err4 := atoi(tmm)
	tssN, err5 := atoi(tss)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
		return nil
	}
	if yyN < 100 {
		if yyN > 68 {
			yyN += 1900
		} else {
			yyN += 2000
		}
	}

	var tzoffset py.Object = py.None
	tzu := strings.ToUpper(tz)
	if v, ok := timezones[tzu]; ok {
		tzoffset = py.Int(v)
	} else if _, err := atoi(tz); err == nil {
		tzoffset = py.Int(0) // placeholder, replaced just below
		if tzoffset, err = tzInt(tz); err != nil {
			return nil
		}
	}
	if v, ok := tzoffset.(py.Int); ok && v != 0 {
		n := int64(v)
		sign := int64(1)
		if n < 0 {
			sign = -1
			n = -n
		}
		tzoffset = py.Int(sign * ((n/100)*3600 + (n%100)*60))
	}
	return []py.Object{
		py.Int(yyN), py.Int(month), py.Int(ddN),
		py.Int(thhN), py.Int(tmmN), py.Int(tssN),
		py.Int(0), py.Int(1), py.Int(-1), tzoffset,
	}
}

// tzInt parses a +HHMM/-HHMM offset, returning None's stand-in as an error for
// the CPython case where a bare "-0000" parses but must not be applied.
func tzInt(tz string) (py.Object, error) {
	n, err := atoi(tz)
	if err != nil {
		return py.None, nil
	}
	return py.Int(n), nil
}

func splitTime(tm string) (hh, mm, ss string, ok bool) {
	parts := strings.Split(tm, ":")
	switch len(parts) {
	case 1:
		if !strings.Contains(parts[0], ".") {
			return tm, "0", "0", true
		}
		dots := strings.SplitN(parts[0], ".", 3)
		switch len(dots) {
		case 2:
			return dots[0], dots[1], "0", true
		case 3:
			return dots[0], dots[1], dots[2], true
		}
	case 2:
		return parts[0], parts[1], "0", true
	case 3:
		return parts[0], parts[1], parts[2], true
	}
	return "", "", "", false
}

func monthNumber(mm string) int {
	if len(mm) > 3 {
		mm = mm[:3]
	}
	return indexOf(monthShort, mm) + 1
}

var monthShort = []string{"jan", "feb", "mar", "apr", "may", "jun",
	"jul", "aug", "sep", "oct", "nov", "dec"}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func parsedateTzFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var data py.Object
	if err := py.UnpackTuple(args, kwargs, "parsedate_tz", 1, 1, &data); err != nil {
		return nil, err
	}
	res := parsedateTz(pyStr(data))
	if res == nil {
		return py.None, nil
	}
	if res[9] == py.None {
		res[9] = py.Int(0)
	}
	return py.Tuple(res), nil
}

func parsedateFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var data py.Object
	if err := py.UnpackTuple(args, kwargs, "parsedate", 1, 1, &data); err != nil {
		return nil, err
	}
	res := parsedateTz(pyStr(data))
	if res == nil {
		return py.None, nil
	}
	return py.Tuple(res[:9]), nil
}

// mktimeTz turns the 10-tuple from parsedate_tz into a POSIX timestamp.  The
// offset is applied with Go's time arithmetic rather than the C library, so the
// result does not depend on the host timezone database.
func mktimeTzFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var data py.Object
	if err := py.UnpackTuple(args, kwargs, "mktime_tz", 1, 1, &data); err != nil {
		return nil, err
	}
	t, err := py.SequenceTuple(data)
	if err != nil {
		return nil, err
	}
	if len(t) < 10 {
		return nil, py.ExceptionNewf(py.IndexError, "tuple index out of range")
	}
	nums := make([]int, 9)
	for i := 0; i < 9; i++ {
		n, err := py.MakeGoInt(t[i])
		if err != nil {
			return nil, err
		}
		nums[i] = n
	}
	year := nums[0]
	if year < 1900 {
		year += 1900
	}
	utc := time.Date(year, time.Month(nums[1]), nums[2], nums[3], nums[4], nums[5], 0, time.UTC)
	if t[9] == py.None {
		// time.mktime() returns a float; calendar.timegm() below returns int.
		loc := time.Date(nums[0], time.Month(nums[1]), nums[2], nums[3], nums[4], nums[5], 0, time.Local)
		return py.Float(float64(loc.Unix())), nil
	}
	off, err := py.MakeGoInt(t[9])
	if err != nil {
		return nil, err
	}
	return py.Int(utc.Unix() - int64(off)), nil
}

func formatdateFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		timeval py.Object = py.None
		loclt   py.Object = py.False
		usegmt  py.Object = py.False
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|Opp:formatdate",
		[]string{"timeval", "localtime", "usegmt"}, &timeval, &loclt, &usegmt); err != nil {
		return nil, err
	}
	var t time.Time
	if timeval == py.None {
		t = time.Now()
	} else {
		n, err := py.MakeGoInt64(timeval)
		if err != nil {
			f, ferr := py.FloatAsFloat64(timeval)
			if ferr != nil {
				return nil, err
			}
			t = time.Unix(int64(f), 0)
		} else {
			t = time.Unix(n, 0)
		}
	}
	return py.String(formatTimestamp(t, truthy(loclt), truthy(usegmt))), nil
}

func formatTimestamp(t time.Time, localtime, usegmt bool) string {
	zone := "-0000"
	if localtime {
		zone = t.Format("-0700")
	} else {
		t = t.UTC()
		if usegmt {
			zone = "GMT"
		}
	}
	dayNamesFull := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	monthNamesFull := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	return fmt.Sprintf("%s, %02d %s %04d %02d:%02d:%02d %s",
		dayNamesFull[int(t.Weekday()+6)%7], t.Day(), monthNamesFull[int(t.Month())-1],
		t.Year(), t.Hour(), t.Minute(), t.Second(), zone)
}

func formatDatetimeFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		dt     py.Object
		usegmt py.Object = py.False
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|p:format_datetime",
		[]string{"dt", "usegmt"}, &dt, &usegmt); err != nil {
		return nil, err
	}
	getInt := func(name string) (int, error) {
		v, err := py.GetAttrString(dt, name)
		if err != nil {
			return 0, err
		}
		return py.MakeGoInt(v)
	}
	year, err := getInt("year")
	if err != nil {
		return nil, err
	}
	month, err := getInt("month")
	if err != nil {
		return nil, err
	}
	day, err := getInt("day")
	if err != nil {
		return nil, err
	}
	hour, err := getInt("hour")
	if err != nil {
		return nil, err
	}
	minute, err := getInt("minute")
	if err != nil {
		return nil, err
	}
	second, err := getInt("second")
	if err != nil {
		return nil, err
	}
	zone := formatZone(dt, truthy(usegmt))
	weekday := int(time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC).Weekday())
	dayNamesFull := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	monthNamesFull := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	return py.String(fmt.Sprintf("%s, %02d %s %04d %02d:%02d:%02d %s",
		dayNamesFull[int(weekday+6)%7], day, monthNamesFull[month-1],
		year, hour, minute, second, zone)), nil
}

// formatZone renders the tzinfo offset the way format_datetime does.  A naive
// datetime (tzinfo None) is "-0000"; usegmt forces "GMT".  The datetime module
// here has no utcoffset(), so an aware datetime reports its offset from the
// tzinfo's own utcoffset attribute when one exists.
func formatZone(dt py.Object, usegmt bool) string {
	if usegmt {
		return "GMT"
	}
	tzinfo, err := py.GetAttrString(dt, "tzinfo")
	if err != nil || tzinfo == nil || tzinfo == py.None {
		return "-0000"
	}
	off, err := py.GetAttrString(tzinfo, "utcoffset")
	if err != nil || off == nil {
		return "-0000"
	}
	v, err := py.Call(off, py.Tuple{dt}, py.NewStringDict())
	if err != nil || v == nil {
		return "-0000"
	}
	secs, err := py.GetAttrString(v, "total_seconds")
	if err != nil || secs == nil {
		return "-0000"
	}
	total, err := py.Call(secs, nil, py.NewStringDict())
	if err != nil {
		return "-0000"
	}
	n, err := py.MakeGoInt(total)
	if err != nil {
		return "-0000"
	}
	sign := "+"
	if n < 0 {
		sign = "-"
		n = -n
	}
	return fmt.Sprintf("%s%02d%02d", sign, n/3600, (n%3600)/60)
}

func makeMsgidFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		idstring py.Object = py.None
		domain   py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|zz:make_msgid",
		[]string{"idstring", "domain"}, &idstring, &domain); err != nil {
		return nil, err
	}
	timeval := time.Now().UnixNano() / int64(10*time.Millisecond)
	pid := os.Getpid()
	randint := rand.Uint64() >> 1
	suffix := ""
	if idstring != py.None {
		suffix = "." + pyStr(idstring)
	}
	dom := ""
	if h, err := os.Hostname(); err == nil {
		dom = h
	}
	if domain != py.None {
		dom = pyStr(domain)
	}
	return py.String(fmt.Sprintf("<%d.%d.%d%s@%s>", timeval, pid, randint, suffix, dom)), nil
}

// encodeRfc2231 is email.utils.encode_rfc2231.  urllib3 calls it with
// charset="utf-8" for non-ASCII form field values.
func encodeRfc2231Func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		s       py.Object
		charset py.Object = py.None
		lang    py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|zz:encode_rfc2231",
		[]string{"s", "charset", "language"}, &s, &charset, &lang); err != nil {
		return nil, err
	}
	cs := "ascii"
	if charset != py.None && pyStr(charset) != "" {
		cs = pyStr(charset)
	}
	if charset == py.None && lang == py.None {
		return py.String(quotePercent(pyStr(s), false)), nil
	}
	l := ""
	if lang != py.None {
		l = pyStr(lang)
	}
	return py.String(cs + "'" + l + "'" + quotePercent(pyStr(s), false)), nil
}

// quotePercent is urllib.parse.quote with safe=” for the characters that
// matter here: everything outside the RFC 3986 unreserved set is escaped, with
// the non-ASCII bytes percent-encoded UTF-8 first.
func quotePercent(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) || (c == '/' && !encodeSlash) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' || strings.IndexByte("-_.~", c) >= 0
}

func decodeRfc2231Func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var s py.Object
	if err := py.UnpackTuple(args, kwargs, "decode_rfc2231", 1, 1, &s); err != nil {
		return nil, err
	}
	text := pyStr(s)
	parts := strings.SplitN(text, "'", 3)
	if len(parts) <= 2 {
		return py.NewListFromItems([]py.Object{py.None, py.None, py.String(text)}), nil
	}
	// CPython 3.14 returns a list here (it used to return a tuple).
	return py.NewListFromItems([]py.Object{
		py.String(parts[0]), py.String(parts[1]), py.String(parts[2]),
	}), nil
}

func collapseRfc2231ValueFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		value    py.Object
		errs     py.Object = py.String("replace")
		fallback py.Object = py.String("us-ascii")
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|zz:collapse_rfc2231_value",
		[]string{"value", "errors", "fallback_charset"}, &value, &errs, &fallback); err != nil {
		return nil, err
	}
	var parts []py.Object
	switch v := value.(type) {
	case py.Tuple:
		parts = v
	case *py.List:
		parts = v.Items
	default:
		return py.String(unquote(pyStr(value))), nil
	}
	if len(parts) != 3 {
		return py.String(unquote(pyStr(value))), nil
	}
	charset := pyStr(parts[0])
	if charset == "" {
		charset = pyStr(fallback)
	}
	// CPython reinterprets the third element through the raw-unicode-escape
	// codec and then decodes the resulting bytes with the charset.  raw-
	// unicode-escape writes each character below U+0100 as its byte value and
	// everything above as a literal "\uXXXX" escape.
	raw := rawUnicodeEscape(pyStr(parts[2]))
	decoded, err := decodeBytesCharset(raw, charset, pyStr(errs))
	if err != nil {
		// An unknown codec falls back to the unquoted text, as CPython does.
		return py.String(unquote(pyStr(parts[2]))), nil
	}
	return py.String(decoded), nil
}

// rawUnicodeEscape is str.encode('raw-unicode-escape') followed by the implicit
// bytes() conversion CPython relies on in collapse_rfc2231_value.
func rawUnicodeEscape(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 0x100:
			out = append(out, byte(r))
		default:
			out = append(out, []byte(fmt.Sprintf("\\u%04x", r))...)
		}
	}
	return out
}

// decodeBytesCharset decodes raw bytes with one of the charsets this
// interpreter's codec layer knows.  LookupError for anything else, which the
// caller turns into CPython's fallback.
func decodeBytesCharset(b []byte, charset, errors string) (string, error) {
	switch strings.ToLower(strings.ReplaceAll(charset, "_", "-")) {
	case "utf-8", "utf8":
		return string(b), nil
	case "latin-1", "latin1", "iso-8859-1", "iso8859-1":
		var sb strings.Builder
		for _, c := range b {
			sb.WriteRune(rune(c))
		}
		return sb.String(), nil
	case "ascii", "us-ascii":
		for _, c := range b {
			if c > 127 {
				return "", py.ExceptionNewf(py.UnicodeDecodeError, "'ascii' codec can't decode byte 0x%02x", c)
			}
		}
		return string(b), nil
	}
	return "", py.ExceptionNewf(py.LookupError, "unknown encoding: %s", charset)
}

// localtime is email.utils.localtime.  The datetime module here has no
// astimezone(), so unlike CPython this cannot convert a naive datetime to the
// host zone; it returns the value it was given and, with no argument, builds
// the current naive local datetime.
// [architectural limit: no timezone-aware datetime conversion]
func localtimeFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var dt py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|z:localtime", []string{"dt"}, &dt); err != nil {
		return nil, err
	}
	if dt != py.None {
		return dt, nil
	}
	mod, err := py.NewModuleStore().GetModule("datetime")
	if err != nil {
		return nil, err
	}
	return py.Call(mod.Globals.GetOrNil("datetime"), nil, py.NewStringDict())
}

func init() {
	globals := py.NewStringDict()
	globals.Set("parseaddr", py.MustNewMethod("parseaddr", parseaddrFunc, 0,
		"Parse an address into a (realname, email_address) tuple."))
	globals.Set("formataddr", py.MustNewMethod("formataddr", formataddrFunc, 0,
		"The inverse of parseaddr: (realname, address) -> a From/To/Cc header value."))
	globals.Set("getaddresses", py.MustNewMethod("getaddresses", getaddressesFunc, 0,
		"Return a list of (realname, email_address) tuples parsed from a list of header values."))
	globals.Set("quote", py.MustNewMethod("quote", quoteFunc, 0,
		"Prepare a string to be used inside a quoted string."))
	globals.Set("unquote", py.MustNewMethod("unquote", unquoteFunc, 0,
		"Remove enclosing quotes (or angle brackets) from a string."))
	globals.Set("parsedate_tz", py.MustNewMethod("parsedate_tz", parsedateTzFunc, 0,
		"Convert a date string to a 10-element time tuple including the UTC offset."))
	globals.Set("parsedate", py.MustNewMethod("parsedate", parsedateFunc, 0,
		"Convert a date string to a 9-element time tuple."))
	globals.Set("mktime_tz", py.MustNewMethod("mktime_tz", mktimeTzFunc, 0,
		"Turn a 10-item tuple as returned by parsedate_tz() into a POSIX timestamp."))
	globals.Set("formatdate", py.MustNewMethod("formatdate", formatdateFunc, 0,
		"Return a date string as specified by RFC 2822."))
	globals.Set("format_datetime", py.MustNewMethod("format_datetime", formatDatetimeFunc, 0,
		"Turn a datetime into a date string as specified in RFC 2822."))
	globals.Set("make_msgid", py.MustNewMethod("make_msgid", makeMsgidFunc, 0,
		"Return a string suitable for an RFC 2822 Message-ID."))
	globals.Set("encode_rfc2231", py.MustNewMethod("encode_rfc2231", encodeRfc2231Func, 0,
		"Encode a string according to RFC 2231."))
	globals.Set("decode_rfc2231", py.MustNewMethod("decode_rfc2231", decodeRfc2231Func, 0,
		"Decode a string according to RFC 2231."))
	globals.Set("collapse_rfc2231_value", py.MustNewMethod("collapse_rfc2231_value", collapseRfc2231ValueFunc, 0,
		"Turn an RFC 2231 value tuple into a plain string."))
	globals.Set("localtime", py.MustNewMethod("localtime", localtimeFunc, 0,
		"Return local time as an aware datetime object."))
	globals.Set("COMMASPACE", py.String(", "))
	globals.Set("EMPTYSTRING", py.String(""))
	globals.Set("CRLF", py.String("\r\n"))
	globals.Set("TICK", py.String("'"))
	globals.Set("supports_strict_parsing", py.True)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "email.utils",
			Doc:  utils_doc,
		},
		Globals: globals,
	})
}
