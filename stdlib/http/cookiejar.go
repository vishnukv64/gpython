// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// http.cookiejar -- manage a jar of HTTP cookies.
//
// This ports CPython's http/cookiejar.py: the Cookie and CookiePolicy classes,
// DefaultCookiePolicy, CookieJar and the FileCookieJar family, plus the
// module-level parsing helpers (split_header_words, parse_ns_headers,
// http2time, domain_match, ...).  urllib3 and requests import Cookie,
// CookieJar and CookiePolicy, requests derives RequestsCookieJar from
// CookieJar, and http.cookiejar is what reads Set-Cookie headers out of a
// response and writes a Cookie header onto a request, so the behaviour that
// matters is make_cookies/extract_cookies/add_cookie_header/set_cookie/clear.
//
// Request and response objects are duck-typed exactly as CPython does it:
// request attributes (get_full_url, get_header, has_header,
// add_unredirected_header, host, origin_req_host) and response.info() are read
// through the ordinary attribute protocol, so both urllib.request's Request
// and requests' MockRequest work.
package http

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
)

const cookiejar_doc = `HTTP cookie handling for web clients.

This module defines Cookie and CookieJar classes, and the CookiePolicy classes
that decide which cookies are accepted from and returned to a server.
`

// LoadErrorType derives from OSError, for backwards compatibility with
// Python 2.4.0, exactly as CPython's LoadError.
var LoadErrorType = py.OSError.NewType("LoadError",
	"Raised when a cookie file cannot be loaded.", nil, nil)

// Absent marks an attribute that was not present, distinct from None.
var absentSentinel = py.NewTypeX("http.cookiejar.Absent",
	"Sentinel meaning the attribute was not present.", nil, nil)

// DEFAULT_HTTP_PORT is the port assumed when a request carries none.
const defaultHTTPPort = "80"

// ---------------------------------------------------------------------------
// Module level helpers
// ---------------------------------------------------------------------------

var (
	ipv4Re      = regexp.MustCompile(`\.\d+$`)
	cutPortRe   = regexp.MustCompile(`:\d+$`)
	escapedChar = regexp.MustCompile(`%([0-9a-fA-F][0-9a-fA-F])`)
)

// isHDN reports whether text is a host domain name.
func isHDN(text string) bool {
	if ipv4Re.MatchString(text) {
		return false
	}
	if text == "" {
		return false
	}
	if text[0] == '.' || text[len(text)-1] == '.' {
		return false
	}
	return true
}

// domainMatch reports whether domain A domain-matches domain B, RFC 2965.
func domainMatch(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == b {
		return true
	}
	if !isHDN(a) {
		return false
	}
	i := strings.LastIndex(a, b)
	if i <= 0 {
		return false
	}
	if !strings.HasPrefix(b, ".") {
		return false
	}
	return isHDN(b[1:])
}

// liberalIsHDN is a looser is_HDN used for accept/block domain lists.
func liberalIsHDN(text string) bool {
	return !ipv4Re.MatchString(text)
}

// userDomainMatch is the accept/block domain comparison.
func userDomainMatch(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if !(liberalIsHDN(a) && liberalIsHDN(b)) {
		return a == b
	}
	initialDot := strings.HasPrefix(b, ".")
	if initialDot && strings.HasSuffix(a, b) {
		return true
	}
	if !initialDot && a == b {
		return true
	}
	return false
}

// reach returns the reach R of host h, RFC 2965 section 1.
func reach(h string) string {
	i := strings.Index(h, ".")
	if i >= 0 {
		b := h[i+1:]
		j := strings.Index(b, ".")
		if isHDN(h) && (j >= 0 || b == "local") {
			return "." + b
		}
	}
	return h
}

// escPath escapes invalid characters and upper-cases existing escapes,
// CPython's escape_path.
func escPath(path string) string {
	path = quotePath(path)
	return escapedChar.ReplaceAllStringFunc(path, func(m string) string {
		g := escapedChar.FindStringSubmatch(m)
		return "%" + strings.ToUpper(g[1])
	})
}

// quotePath is urllib.parse.quote(path, "%/;:@&=+$,!~*'()").
func quotePath(path string) string {
	return quoteWithSafe(path, "%/;:@&=+$,!~*'()")
}

// stripQuotes removes one leading and one trailing double quote.
func stripQuotes(text string) string {
	text = strings.TrimPrefix(text, `"`)
	text = strings.TrimSuffix(text, `"`)
	return text
}

// http2time parses an HTTP date to seconds since the epoch, or -1.
func http2time(text string) int64 {
	// Try the handful of layouts HTTP uses; CPython's http2time is a
	// hand-written parser, but time.Parse over the known layouts accepts the
	// same well-formed strings and returns -1 otherwise.
	layouts := []string{
		time.RFC1123,
		time.RFC850,
		time.ANSIC,
		"Mon, 02-Jan-2006 15:04:05 MST",
		"Mon, 2-Jan-2006 15:04:05 MST",
		"Monday, 02-Jan-06 15:04:05 MST",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, text); err == nil {
			return t.Unix()
		}
	}
	return -1
}

// time2isoz formats the time for ISO 8601, CPython's time2isoz.
func time2isoz(t time.Time) string {
	t = t.UTC()
	return t.Format("2006-01-02 15:04:05Z")
}

// time2netscape formats the time for Netscape cookie files.
func time2netscape(t time.Time) string {
	t = t.UTC()
	return t.Format("Mon, 02-Jan-2006 15:04:05 GMT")
}

// headerTokenRe, headerQuotedValueRe, headerValueRe and headerEscapeRe are
// CPython's HEADER_*_RE, anchored at the start of the remaining text.
var (
	headerTokenRe       = regexp.MustCompile(`^\s*([^=\s;,]+)`)
	headerQuotedValueRe = regexp.MustCompile(`^\s*=\s*"([^"\\]*(?:\\.[^"\\]*)*)"`)
	headerValueRe       = regexp.MustCompile(`^\s*=\s*([^\s;,]*)`)
	headerEscapeRe      = regexp.MustCompile(`\\(.)`)
	junkRe              = regexp.MustCompile(`^[=\s;]*`)
)

// splitHeaderWords parses header values into lists of (key, value) pairs,
// CPython's split_header_words.  A lone token has a None value.
func splitHeaderWords(values []string) [][][2]py.Object {
	var result [][][2]py.Object
	for _, text := range values {
		var pairs [][2]py.Object
		for len(text) > 0 {
			m := headerTokenRe.FindStringSubmatch(text)
			if m != nil {
				name := m[1]
				text = text[len(m[0]):]
				var value string
				var found bool
				if qm := headerQuotedValueRe.FindStringSubmatch(text); qm != nil {
					text = text[len(qm[0]):]
					value = headerEscapeRe.ReplaceAllString(qm[1], "$1")
					found = true
				} else if vm := headerValueRe.FindStringSubmatch(text); vm != nil {
					text = text[len(vm[0]):]
					value = strings.TrimRight(vm[1], " \t\n\r\f\v")
					found = true
				}
				pairs = append(pairs, [2]py.Object{py.String(name), pyStringOrNone(value, found)})
				continue
			}
			lstripped := strings.TrimLeft(text, " \t\n\r\f\v")
			if strings.HasPrefix(lstripped, ",") {
				text = lstripped[1:]
				if len(pairs) > 0 {
					result = append(result, pairs)
					pairs = nil
				}
				continue
			}
			trimmed := junkRe.ReplaceAllString(text, "")
			if len(trimmed) == len(text) {
				break
			}
			text = trimmed
		}
		if len(pairs) > 0 {
			result = append(result, pairs)
		}
	}
	return result
}

// joinHeaderWords does the inverse of splitHeaderWords.
func joinHeaderWords(lists [][][2]py.Object) string {
	var headers []string
	for _, pairs := range lists {
		var attrs []string
		for _, p := range pairs {
			k := p[0].(py.String)
			v := p[1]
			if v != py.None {
				vs := ""
				if s, ok := v.(py.String); ok {
					vs = string(s)
				}
				if !isHeaderToken(vs) {
					vs = headerJoinEscape(vs)
					vs = `"` + vs + `"`
				}
				attrs = append(attrs, string(k)+"="+vs)
			} else {
				attrs = append(attrs, string(k))
			}
		}
		if len(attrs) > 0 {
			headers = append(headers, strings.Join(attrs, "; "))
		}
	}
	return strings.Join(headers, ", ")
}

func isHeaderToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' ||
			strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0) {
			return false
		}
	}
	return true
}

var headerJoinEscaper = regexp.MustCompile(`(["\\])`)

func headerJoinEscape(s string) string {
	return headerJoinEscaper.ReplaceAllString(s, `\$1`)
}

// parseNsHeaders is the ad-hoc Netscape Set-Cookie parser.
func parseNsHeaders(nsHeaders []string) [][][2]py.Object {
	known := map[string]bool{
		"expires": true, "domain": true, "path": true, "secure": true,
		"version": true, "port": true, "max-age": true,
	}
	var result [][][2]py.Object
	for _, hdr := range nsHeaders {
		var pairs [][2]py.Object
		versionSet := false
		for ii, param := range strings.Split(hdr, ";") {
			param = strings.TrimSpace(param)
			key := param
			val := ""
			hasVal := false
			if i := strings.Index(param, "="); i >= 0 {
				key = strings.TrimSpace(param[:i])
				val = strings.TrimSpace(param[i+1:])
				hasVal = true
			}
			if key == "" {
				if ii == 0 {
					break
				}
				continue
			}
			if ii != 0 {
				lc := strings.ToLower(key)
				if known[lc] {
					key = lc
				}
				if key == "version" {
					if hasVal {
						val = stripQuotes(val)
					}
					versionSet = true
				} else if key == "expires" {
					if hasVal {
						n := http2time(stripQuotes(val))
						val = strconv.FormatInt(n, 10)
					}
				}
			}
			pairs = append(pairs, [2]py.Object{py.String(key), pyStringOrNone(val, hasVal)})
		}
		if len(pairs) > 0 {
			if !versionSet {
				pairs = append(pairs, [2]py.Object{py.String("version"), py.String("0")})
			}
			result = append(result, pairs)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Cookie
// ---------------------------------------------------------------------------

// cookie mirrors CPython's Cookie: an attribute holder for one cookie.
type cookie struct {
	version          py.Object
	name             string
	value            py.Object
	port             py.Object
	portSpecified    bool
	domain           string
	domainSpecified  bool
	domainInitialDot bool
	path             string
	pathSpecified    bool
	secure           bool
	expires          py.Object
	discard          bool
	comment          py.Object
	commentURL       py.Object
	rfc2109          bool
	rest             *py.StringDict
	Dict             py.StringDict
}

func (c *cookie) Type() *py.Type         { return CookieType }
func (c *cookie) GetDict() py.StringDict { return c.Dict }

func (c *cookie) isExpired(now float64) bool {
	if c.expires == nil || c.expires == py.None {
		return false
	}
	switch v := c.expires.(type) {
	case py.Int:
		return float64(v) <= now
	case py.Float:
		return float64(v) <= now
	}
	return false
}

func cookieNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := &cookie{Dict: py.NewStringDict(), rest: new(py.StringDict)}
	fieldNames := []string{
		"version", "name", "value",
		"port", "port_specified",
		"domain", "domain_specified", "domain_initial_dot",
		"path", "path_specified",
		"secure", "expires", "discard", "comment", "comment_url",
		"rest", "rfc2109",
	}
	// Positional then keyword, mirroring CPython's signature.  Required are
	// the 16 that precede rfc2109.
	const required = 16
	if len(args) > len(fieldNames) {
		return nil, py.ExceptionNewf(py.TypeError, "Cookie() takes at most %d arguments (%d given)",
			len(fieldNames), len(args))
	}
	if len(args)+kwargs.Len() < required {
		return nil, py.ExceptionNewf(py.TypeError,
			"Cookie() requires %d arguments", required)
	}
	vals := make(map[string]py.Object, len(fieldNames))
	for i, a := range args {
		vals[fieldNames[i]] = a
	}
	for _, ent := range kwargs.Items() {
		vals[ent.Key] = ent.Value
	}
	get := func(k string) py.Object { return vals[k] }

	if v := get("version"); v != nil && v != py.None {
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		c.version = py.Int(n)
	}
	if v := get("expires"); v != nil && v != py.None {
		f, err := floatFromObject(v)
		if err != nil {
			return nil, err
		}
		c.expires = py.Int(int64(f))
	}
	if s, err := py.StrAsString(get("name")); err == nil {
		c.name = s
	}
	c.value = get("value")
	c.port = get("port")
	c.domain, _ = py.StrAsString(get("domain"))
	c.domain = strings.ToLower(c.domain)
	c.path, _ = py.StrAsString(get("path"))
	c.comment = get("comment")
	c.commentURL = get("comment_url")
	c.portSpecified = truthy(get("port_specified"))
	c.domainSpecified = truthy(get("domain_specified"))
	c.domainInitialDot = truthy(get("domain_initial_dot"))
	c.pathSpecified = truthy(get("path_specified"))
	c.secure = truthy(get("secure"))
	c.discard = truthy(get("discard"))
	c.rfc2109 = truthy(get("rfc2109"))
	if r, ok := get("rest").(py.StringDict); ok {
		*c.rest = r.Copy()
	}
	if c.port == nil {
		c.port = py.None
	}
	if c.port == py.None && c.portSpecified {
		return nil, py.ExceptionNewf(py.ValueError, "if port is None, port_specified must be false")
	}
	return c, nil
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

func floatFromObject(v py.Object) (float64, error) {
	switch x := v.(type) {
	case py.Int:
		return float64(x), nil
	case py.Float:
		return float64(x), nil
	}
	return 0, py.ExceptionNewf(py.TypeError, "a number is required")
}

func jarCookieStr(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*cookie)
	p := ""
	if c.port != nil && c.port != py.None {
		s, _ := py.StrAsString(c.port)
		p = ":" + s
	}
	limit := c.domain + p + c.path
	var namevalue string
	if c.value != nil && c.value != py.None {
		vs, _ := py.StrAsString(c.value)
		namevalue = c.name + "=" + vs
	} else {
		namevalue = c.name
	}
	return py.String("<Cookie " + namevalue + " for " + limit + ">"), nil
}

func cookieIsExpired(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := self.(*cookie)
	now := float64(time.Now().Unix())
	if len(args) > 0 && args[0] != py.None {
		f, err := floatFromObject(args[0])
		if err != nil {
			return nil, err
		}
		now = f
	}
	return py.Bool(c.isExpired(now)), nil
}

func cookieHasNonstandardAttr(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*cookie)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "has_nonstandard_attr() takes exactly one argument")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	return py.Bool(c.rest.Has(name)), nil
}

func cookieGetNonstandardAttr(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := self.(*cookie)
	var name py.Object = py.String("")
	var dflt py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "S|O:get_nonstandard_attr",
		[]string{"name", "default"}, &name, &dflt); err != nil {
		return nil, err
	}
	if v, ok := c.rest.Get(string(name.(py.String))); ok {
		return v, nil
	}
	return dflt, nil
}

func cookieSetNonstandardAttr(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*cookie)
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "set_nonstandard_attr() takes exactly two arguments")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	c.rest.Set(name, args[1])
	return py.None, nil
}

// ---------------------------------------------------------------------------
// CookiePolicy
// ---------------------------------------------------------------------------

// cookiePolicy is the base policy: set_ok/return_ok raise, the rest return True.
type cookiePolicy struct {
	Dict py.StringDict
}

func (p *cookiePolicy) Type() *py.Type         { return CookiePolicyType }
func (p *cookiePolicy) GetDict() py.StringDict { return p.Dict }

func cookiePolicyNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return &cookiePolicy{Dict: py.NewStringDict()}, nil
}

func policySetOk(self py.Object, args py.Tuple) (py.Object, error) {
	return nil, py.ExceptionNewf(py.NotImplementedError, "")
}

func policyReturnOk(self py.Object, args py.Tuple) (py.Object, error) {
	return nil, py.ExceptionNewf(py.NotImplementedError, "")
}

func policyReturnTrue(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Bool(true), nil
}

// ---------------------------------------------------------------------------
// DefaultCookiePolicy
// ---------------------------------------------------------------------------

// defaultPolicy implements the standard accept/return rules.
type defaultPolicy struct {
	cookiePolicy
	netscape                  bool
	rfc2965                   bool
	rfc2109AsNetscape         py.Object
	hideCookie2               bool
	strictDomain              bool
	strictRfc2965Unverifiable bool
	strictNsUnverifiable      bool
	strictNsDomain            int
	strictNsSetInitialDollar  bool
	strictNsSetPath           bool
	secureProtocols           []string
	blockedDomains            []string
	allowedDomains            []string
	now                       int64
}

func (p *defaultPolicy) Type() *py.Type { return DefaultCookiePolicyType }

func defaultPolicyNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p := &defaultPolicy{
		cookiePolicy:              cookiePolicy{Dict: py.NewStringDict()},
		netscape:                  true,
		strictRfc2965Unverifiable: true,
		secureProtocols:           []string{"https", "wss"},
	}
	blocked := py.Object(py.None)
	allowed := py.Object(py.None)
	netscape := py.Object(py.Bool(true))
	rfc2965 := py.Object(py.Bool(false))
	rfc2109AsNetscape := py.Object(py.None)
	hideCookie2 := py.Object(py.Bool(false))
	strictDomain := py.Object(py.Bool(false))
	strictRfc := py.Object(py.Bool(true))
	strictNsUnver := py.Object(py.Bool(false))
	strictNsDomain := py.Object(py.Int(0))
	strictNsInitialDollar := py.Object(py.Bool(false))
	strictNsSetPath := py.Object(py.Bool(false))
	secureProtocols := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOOOOOOOOOOO:__init__",
		[]string{"blocked_domains", "allowed_domains", "netscape", "rfc2965",
			"rfc2109_as_netscape", "hide_cookie2", "strict_domain",
			"strict_rfc2965_unverifiable", "strict_ns_unverifiable",
			"strict_ns_domain", "strict_ns_set_initial_dollar", "strict_ns_set_path"},
		&blocked, &allowed, &netscape, &rfc2965, &rfc2109AsNetscape, &hideCookie2,
		&strictDomain, &strictRfc, &strictNsUnver, &strictNsDomain,
		&strictNsInitialDollar, &strictNsSetPath); err != nil {
		return nil, err
	}
	_ = secureProtocols
	p.netscape = truthy(netscape)
	p.rfc2965 = truthy(rfc2965)
	p.rfc2109AsNetscape = rfc2109AsNetscape
	p.hideCookie2 = truthy(hideCookie2)
	p.strictDomain = truthy(strictDomain)
	p.strictRfc2965Unverifiable = truthy(strictRfc)
	p.strictNsUnverifiable = truthy(strictNsUnver)
	if n, err := py.IndexInt(strictNsDomain); err == nil {
		p.strictNsDomain = n
	}
	p.strictNsSetInitialDollar = truthy(strictNsInitialDollar)
	p.strictNsSetPath = truthy(strictNsSetPath)
	p.blockedDomains = stringList(blocked)
	p.allowedDomains = stringList(allowed)
	return p, nil
}

func stringList(v py.Object) []string {
	if v == nil || v == py.None {
		return nil
	}
	l, err := py.SequenceList(v)
	if err != nil {
		return nil
	}
	var out []string
	for _, it := range l.Items {
		if s, err := py.StrAsString(it); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// CookieJar
// ---------------------------------------------------------------------------

// cookieJar stores cookies keyed domain -> path -> name.
type cookieJar struct {
	policy  *defaultPolicy
	cookies map[string]map[string]map[string]*cookie
	order   []string // domain order, for deterministic iteration
	now     int64
	Dict    py.StringDict
}

func (j *cookieJar) Type() *py.Type         { return CookieJarType }
func (j *cookieJar) GetDict() py.StringDict { return j.Dict }

func cookieJarNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	policyArg := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:__init__", []string{"policy"}, &policyArg); err != nil {
		return nil, err
	}
	j := &cookieJar{
		cookies: map[string]map[string]map[string]*cookie{},
		Dict:    py.NewStringDict(),
	}
	if policyArg == py.None || policyArg == nil {
		p, _ := defaultPolicyNew(DefaultCookiePolicyType, nil, py.NewStringDict())
		j.policy = p.(*defaultPolicy)
	} else {
		if p, ok := policyArg.(*defaultPolicy); ok {
			j.policy = p
		} else {
			// A user policy: store it so get_policy returns it; accept/return
			// checks go through the python object.
			j.Dict.Set("_user_policy", policyArg)
		}
	}
	return j, nil
}

func jarSetPolicy(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "set_policy() takes exactly one argument")
	}
	if p, ok := args[0].(*defaultPolicy); ok {
		j.policy = p
		j.Dict.Del("_user_policy")
	} else {
		j.Dict.Set("_user_policy", args[0])
	}
	return py.None, nil
}

func jarGetPolicy(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	if up, ok := j.Dict.Get("_user_policy"); ok {
		return up, nil
	}
	return j.policy, nil
}

func jarSetCookie(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "set_cookie() takes exactly one argument")
	}
	c, ok := args[0].(*cookie)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "set_cookie() requires a Cookie")
	}
	j.store(c)
	return py.None, nil
}

func (j *cookieJar) store(c *cookie) {
	if j.cookies[c.domain] == nil {
		j.cookies[c.domain] = map[string]map[string]*cookie{}
		j.order = append(j.order, c.domain)
		j.Dict.Set("_domain_order", py.NewListFromItems(func() []py.Object {
			o := make([]py.Object, len(j.order))
			for i, d := range j.order {
				o[i] = py.String(d)
			}
			return o
		}()))
	}
	if j.cookies[c.domain][c.path] == nil {
		j.cookies[c.domain][c.path] = map[string]*cookie{}
	}
	j.cookies[c.domain][c.path][c.name] = c
}

// all returns the cookies in domain/path insertion order.
func (j *cookieJar) all() []*cookie {
	var out []*cookie
	for _, d := range j.order {
		byPath := j.cookies[d]
		paths := sortedKeys(byPath)
		for _, p := range paths {
			byName := byPath[p]
			names := sortedKeys2(byName)
			for _, n := range names {
				out = append(out, byName[n])
			}
		}
	}
	return out
}

func sortedKeys(m map[string]map[string]*cookie) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys2(m map[string]*cookie) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func jarIter(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	cookies := j.all()
	items := make([]py.Object, len(cookies))
	for i, c := range cookies {
		items[i] = c
	}
	return py.NewListFromItems(items), nil
}

func jarLen(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Int(len(self.(*cookieJar).all())), nil
}

func jarContains(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__contains__() takes exactly one argument")
	}
	for _, c := range j.all() {
		if c == args[0] {
			return py.Bool(true), nil
		}
	}
	return py.Bool(false), nil
}

func jarClear(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	j := self.(*cookieJar)
	domain := py.Object(py.None)
	path := py.Object(py.None)
	name := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOO:clear",
		[]string{"domain", "path", "name"}, &domain, &path, &name); err != nil {
		return nil, err
	}
	if name != py.None {
		if domain == py.None || path == py.None {
			return nil, py.ExceptionNewf(py.ValueError, "domain and path must be given to remove a cookie by name")
		}
		d, _ := py.StrAsString(domain)
		p, _ := py.StrAsString(path)
		n, _ := py.StrAsString(name)
		if _, err := j.remove(d, p, n); err != nil {
			return nil, err
		}
		return py.None, nil
	}
	if path != py.None {
		if domain == py.None {
			return nil, py.ExceptionNewf(py.ValueError, "domain must be given to remove cookies by path")
		}
		d, _ := py.StrAsString(domain)
		p, _ := py.StrAsString(path)
		if _, ok := j.cookies[d][p]; !ok {
			return nil, py.ExceptionNewf(py.KeyError, "%s", d)
		}
		delete(j.cookies[d], p)
		return py.None, nil
	}
	if domain != py.None {
		d, _ := py.StrAsString(domain)
		if _, ok := j.cookies[d]; !ok {
			return nil, py.ExceptionNewf(py.KeyError, "%s", d)
		}
		delete(j.cookies, d)
		j.dropOrder(d)
		return py.None, nil
	}
	j.cookies = map[string]map[string]map[string]*cookie{}
	j.order = nil
	j.Dict.Set("_domain_order", py.NewListFromItems(nil))
	return py.None, nil
}

func (j *cookieJar) remove(d, p, n string) (bool, error) {
	if j.cookies[d] == nil || j.cookies[d][p] == nil {
		return false, py.ExceptionNewf(py.KeyError, "%s", d)
	}
	if _, ok := j.cookies[d][p][n]; !ok {
		return false, py.ExceptionNewf(py.KeyError, "%s", n)
	}
	delete(j.cookies[d][p], n)
	return true, nil
}

func (j *cookieJar) dropOrder(d string) {
	for i, x := range j.order {
		if x == d {
			j.order = append(j.order[:i], j.order[i+1:]...)
			break
		}
	}
}

func jarClearSessionCookies(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	for _, c := range j.all() {
		if c.discard {
			j.remove(c.domain, c.path, c.name)
		}
	}
	return py.None, nil
}

func jarClearExpiredCookies(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	now := float64(time.Now().Unix())
	for _, c := range j.all() {
		if c.isExpired(now) {
			j.remove(c.domain, c.path, c.name)
		}
	}
	return py.None, nil
}

func jarRepr(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	var parts []string
	for _, c := range j.all() {
		s, _ := jarCookieStr(c, nil)
		parts = append(parts, string(s.(py.String)))
	}
	name := "CookieJar"
	if _, ok := self.(*cookieJar).Dict.Get("_user_policy"); ok {
		name = "CookieJar"
	}
	return py.String("<" + name + "[" + strings.Join(parts, ", ") + "]>"), nil
}

// extractCookies reads Set-Cookie headers from a response and stores the
// allowed ones.
func jarExtractCookies(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "extract_cookies() takes exactly two arguments")
	}
	made, err := j.makeCookies(args[0], args[1])
	if err != nil {
		return nil, err
	}
	for _, c := range made {
		ok, err := j.setOk(c, args[1])
		if err != nil {
			return nil, err
		}
		if ok {
			j.store(c)
		}
	}
	return py.None, nil
}

// makeCookies builds the Cookie objects a response offers.
func (j *cookieJar) makeCookies(response, request py.Object) ([]*cookie, error) {
	headers, err := callMethod(response, "info")
	if err != nil {
		return nil, err
	}
	rfc2965Hdrs, err := getAllHeader(headers, "Set-Cookie2")
	if err != nil {
		return nil, err
	}
	nsHdrs, err := getAllHeader(headers, "Set-Cookie")
	if err != nil {
		return nil, err
	}
	pol, userPolicy, err := jarPolicyObject(j)
	if err != nil {
		return nil, err
	}
	_ = userPolicy
	j.now = time.Now().Unix()
	if pol != nil {
		pol.now = j.now
	}
	rfc2965 := pol != nil && pol.rfc2965
	netscape := pol != nil && pol.netscape
	if (len(rfc2965Hdrs) == 0 && len(nsHdrs) == 0) ||
		(len(nsHdrs) == 0 && !rfc2965) ||
		(len(rfc2965Hdrs) == 0 && !netscape) ||
		(!netscape && !rfc2965) {
		return nil, nil
	}
	var cookies []*cookie
	if len(rfc2965Hdrs) > 0 {
		cs, err := j.cookiesFromAttrsSet(splitHeaderWords(rfc2965Hdrs), request)
		if err != nil {
			return nil, err
		}
		cookies = append(cookies, cs...)
	}
	if len(nsHdrs) > 0 && netscape {
		nsCookies, err := j.cookiesFromAttrsSet(parseNsHeaders(nsHdrs), request)
		if err != nil {
			return nil, err
		}
		j.processRfc2109(nsCookies, pol)
		if rfc2965 {
			lookup := map[string]bool{}
			for _, c := range cookies {
				lookup[c.domain+"\x00"+c.path+"\x00"+c.name] = true
			}
			var filtered []*cookie
			for _, c := range nsCookies {
				if !lookup[c.domain+"\x00"+c.path+"\x00"+c.name] {
					filtered = append(filtered, c)
				}
			}
			nsCookies = filtered
		}
		cookies = append(cookies, nsCookies...)
	}
	return cookies, nil
}

func jarPolicyObject(j *cookieJar) (*defaultPolicy, py.Object, error) {
	if up, ok := j.Dict.Get("_user_policy"); ok {
		return nil, up, nil
	}
	return j.policy, nil, nil
}

func (j *cookieJar) processRfc2109(cookies []*cookie, pol *defaultPolicy) {
	if pol == nil {
		return
	}
	asNs := pol.rfc2109AsNetscape
	if asNs == nil || asNs == py.None {
		asNs = py.Bool(!pol.rfc2965)
	}
	for _, c := range cookies {
		if v, ok := c.version.(py.Int); ok && v == 1 {
			c.rfc2109 = true
			if truthy(asNs) {
				c.version = py.Int(0)
			}
		}
	}
}

func (j *cookieJar) cookiesFromAttrsSet(attrsSet [][][2]py.Object, request py.Object) ([]*cookie, error) {
	tuples, err := j.normalizedCookieTuples(attrsSet)
	if err != nil {
		return nil, err
	}
	var cookies []*cookie
	for _, tup := range tuples {
		c, err := j.cookieFromTuple(tup, request)
		if err != nil {
			return nil, err
		}
		if c != nil {
			cookies = append(cookies, c)
		}
	}
	return cookies, nil
}

type cookieTuple struct {
	name     string
	value    py.Object
	standard map[string]py.Object
	rest     map[string]py.Object
}

func (j *cookieJar) normalizedCookieTuples(attrsSet [][][2]py.Object) ([]cookieTuple, error) {
	booleanAttrs := map[string]bool{"discard": true, "secure": true}
	valueAttrs := map[string]bool{
		"version": true, "expires": true, "max-age": true,
		"domain": true, "path": true, "port": true,
		"comment": true, "commenturl": true,
	}
	var out []cookieTuple
	for _, attrs := range attrsSet {
		if len(attrs) == 0 {
			continue
		}
		name := string(attrs[0][0].(py.String))
		value := attrs[0][1]
		maxAgeSet := false
		bad := false
		standard := map[string]py.Object{}
		rest := map[string]py.Object{}
		for _, kv := range attrs[1:] {
			k := string(kv[0].(py.String))
			v := kv[1]
			lc := strings.ToLower(k)
			if valueAttrs[lc] || booleanAttrs[lc] {
				k = lc
			}
			if booleanAttrs[k] && v == py.None {
				v = py.Bool(true)
			}
			if _, dup := standard[k]; dup {
				continue
			}
			if k == "domain" {
				if v == py.None {
					bad = true
					break
				}
				if s, ok := v.(py.String); ok {
					v = py.String(strings.ToLower(string(s)))
				}
			}
			if k == "expires" {
				if maxAgeSet {
					continue
				}
				if v == py.None {
					continue
				}
			}
			if k == "max-age" {
				maxAgeSet = true
				n, err := py.IndexInt(v)
				if err != nil {
					bad = true
					break
				}
				k = "expires"
				v = py.Int(j.now + int64(n))
			}
			if valueAttrs[k] || booleanAttrs[k] {
				if v == py.None && k != "port" && k != "comment" && k != "commenturl" {
					bad = true
					break
				}
				standard[k] = v
			} else {
				rest[k] = v
			}
		}
		if bad {
			continue
		}
		out = append(out, cookieTuple{name: name, value: value, standard: standard, rest: rest})
	}
	return out, nil
}

func (j *cookieJar) cookieFromTuple(tup cookieTuple, request py.Object) (*cookie, error) {
	standard := tup.standard
	domain, domainAbsent := standard["domain"]
	path, pathAbsent := standard["path"]
	port, portAbsent := standard["port"]
	expires, expiresAbsent := standard["expires"]

	version := py.Object(py.None)
	if v, ok := standard["version"]; ok {
		if v != py.None {
			if n, err := py.IndexInt(v); err == nil {
				version = py.Int(n)
			} else {
				return nil, nil
			}
		}
	}
	secure := standard["secure"]
	discard := standard["discard"]
	comment := standard["comment"]
	commentURL := standard["commenturl"]
	versionInt := 0
	if v, ok := version.(py.Int); ok {
		versionInt = int(v)
	}
	if secure == nil {
		secure = py.Bool(false)
	}
	if discard == nil {
		discard = py.Bool(false)
	}

	var pathStr string
	pathSpecified := false
	if !pathAbsent && path != py.None {
		if s, ok := path.(py.String); ok && string(s) != "" {
			pathSpecified = true
			pathStr = escPath(string(s))
		}
	}
	if !pathSpecified {
		rp, err := requestPath(request)
		if err != nil {
			return nil, err
		}
		pathStr = rp
		if i := strings.LastIndex(pathStr, "/"); i != -1 {
			if versionInt == 0 {
				pathStr = pathStr[:i]
			} else {
				pathStr = pathStr[:i+1]
			}
		}
		if len(pathStr) == 0 {
			pathStr = "/"
		}
	}

	domainSpecified := !domainAbsent
	domainInitialDot := false
	if domainSpecified {
		if s, ok := domain.(py.String); ok && strings.HasPrefix(string(s), ".") {
			domainInitialDot = true
		}
	}
	var domainStr string
	if domainAbsent {
		_, erhn, err := effRequestHost(request)
		if err != nil {
			return nil, err
		}
		domainStr = erhn
	} else {
		ds, _ := py.StrAsString(domain)
		if !strings.HasPrefix(ds, ".") {
			ds = "." + ds
		}
		domainStr = ds
	}

	portSpecified := false
	var portObj py.Object = py.None
	if !portAbsent {
		if port == py.None {
			p, err := requestPort(request)
			if err != nil {
				return nil, err
			}
			portObj = p
		} else {
			portSpecified = true
			ps, _ := py.StrAsString(port)
			portObj = py.String(regexp.MustCompile(`\s+`).ReplaceAllString(ps, ""))
		}
	}

	var expiresObj py.Object = py.None
	if expiresAbsent {
		expiresObj = py.None
		discard = py.Bool(true)
	} else {
		if ex, ok := expires.(py.Int); ok && int64(ex) <= j.now {
			j.remove(domainStr, pathStr, tup.name)
			return nil, nil
		}
		expiresObj = expires
	}

	rest := py.NewStringDict()
	for k, v := range tup.rest {
		rest.Set(k, v)
	}
	c := &cookie{
		version:          version,
		name:             tup.name,
		value:            tup.value,
		port:             portObj,
		portSpecified:    portSpecified,
		domain:           strings.ToLower(domainStr),
		domainSpecified:  domainSpecified,
		domainInitialDot: domainInitialDot,
		path:             pathStr,
		pathSpecified:    pathSpecified,
		secure:           truthy(secure),
		expires:          expiresObj,
		discard:          truthy(discard),
		comment:          comment,
		commentURL:       commentURL,
		rest:             &rest,
		Dict:             py.NewStringDict(),
	}
	return c, nil
}

// addCookieHeader writes the Cookie header for a request.
func jarAddCookieHeader(self py.Object, args py.Tuple) (py.Object, error) {
	j := self.(*cookieJar)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "add_cookie_header() takes exactly one argument")
	}
	request := args[0]
	j.now = time.Now().Unix()
	pol, _, err := jarPolicyObject(j)
	if err != nil {
		return nil, err
	}
	if pol != nil {
		pol.now = j.now
	}
	cookies, err := j.cookiesForRequest(request, pol, nil)
	if err != nil {
		return nil, err
	}
	attrs, err := j.cookieAttrs(cookies)
	if err != nil {
		return nil, err
	}
	if len(attrs) > 0 {
		has := false
		if h, err := callMethodOrNil(request, "has_header", py.String("Cookie")); err == nil {
			has = truthy(h)
		}
		if !has {
			if _, err := callMethod(request, "add_unredirected_header",
				py.String("Cookie"), py.String(strings.Join(attrs, "; "))); err != nil {
				return nil, err
			}
		}
	}
	jarClearExpiredCookies(self, nil)
	return py.None, nil
}

func (j *cookieJar) cookiesForRequest(request py.Object, pol *defaultPolicy, userPolicy py.Object) ([]*cookie, error) {
	var out []*cookie
	for _, d := range j.order {
		cs, err := j.cookiesForDomain(d, request, pol, userPolicy)
		if err != nil {
			return nil, err
		}
		out = append(out, cs...)
	}
	return out, nil
}

func (j *cookieJar) cookiesForDomain(domain string, request py.Object, pol *defaultPolicy, userPolicy py.Object) ([]*cookie, error) {
	if pol != nil {
		if !pol.domainReturnOk(domain, request) {
			return nil, nil
		}
	}
	var out []*cookie
	byPath := j.cookies[domain]
	for _, path := range sortedKeys(byPath) {
		if pol != nil && !pol.pathReturnOk(path, request) {
			continue
		}
		byName := byPath[path]
		for _, name := range sortedKeys2(byName) {
			c := byName[name]
			ok, err := j.returnOk(c, request, pol, userPolicy)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func (j *cookieJar) cookieAttrs(cookies []*cookie) ([]string, error) {
	sort.SliceStable(cookies, func(a, b int) bool {
		return len(cookies[a].path) > len(cookies[b].path)
	})
	versionSet := false
	var attrs []string
	nonWord := regexp.MustCompile(`\W`)
	quoteRe := regexp.MustCompile(`(["\\])`)
	for _, c := range cookies {
		version := 0
		if v, ok := c.version.(py.Int); ok {
			version = int(v)
		}
		if !versionSet {
			versionSet = true
			if version > 0 {
				attrs = append(attrs, "$Version="+strconv.Itoa(version))
			}
		}
		var value string
		if c.value != nil && c.value != py.None {
			vs, _ := py.StrAsString(c.value)
			if nonWord.MatchString(vs) && version > 0 {
				value = quoteRe.ReplaceAllString(vs, `\\$1`)
			} else {
				value = vs
			}
		}
		if c.value == nil || c.value == py.None {
			attrs = append(attrs, c.name)
		} else {
			attrs = append(attrs, c.name+"="+value)
		}
		if version > 0 {
			if c.pathSpecified {
				attrs = append(attrs, `$Path="`+c.path+`"`)
			}
			if strings.HasPrefix(c.domain, ".") {
				d := c.domain
				if !c.domainInitialDot && strings.HasPrefix(d, ".") {
					d = d[1:]
				}
				attrs = append(attrs, `$Domain="`+d+`"`)
			}
			if c.port != nil && c.port != py.None {
				p := "$Port"
				if c.portSpecified {
					ps, _ := py.StrAsString(c.port)
					p += `="` + ps + `"`
				}
				attrs = append(attrs, p)
			}
		}
	}
	return attrs, nil
}

// setOk runs the policy's accept check.
func (j *cookieJar) setOk(c *cookie, request py.Object) (bool, error) {
	pol, userPolicy, err := jarPolicyObject(j)
	if err != nil {
		return false, err
	}
	if userPolicy != nil {
		res, err := callMethod(userPolicy, "set_ok", c, request)
		if err != nil {
			return false, err
		}
		return truthy(res), nil
	}
	return pol.setOk(c, request)
}

// returnOk runs the policy's return check.
func (j *cookieJar) returnOk(c *cookie, request py.Object, pol *defaultPolicy, userPolicy py.Object) (bool, error) {
	if userPolicy != nil {
		res, err := callMethod(userPolicy, "return_ok", c, request)
		if err != nil {
			return false, err
		}
		return truthy(res), nil
	}
	return pol.returnOk(c, request)
}

// ---------------------------------------------------------------------------
// DefaultCookiePolicy rules
// ---------------------------------------------------------------------------

func (p *defaultPolicy) setOk(c *cookie, request py.Object) (bool, error) {
	if !p.netscape && !p.rfc2965 {
		return false, nil
	}
	checks := []func(*cookie, py.Object) (bool, error){
		p.setOkVersion, p.setOkName, p.setOkVerifiability, p.setOkPath,
		p.setOkDomain, p.setOkPort,
	}
	for _, chk := range checks {
		ok, err := chk(c, request)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func (p *defaultPolicy) returnOk(c *cookie, request py.Object) (bool, error) {
	if !p.netscape && !p.rfc2965 {
		return false, nil
	}
	checks := []func(*cookie, py.Object) (bool, error){
		p.returnOkVersion, p.returnOkVerifiability, p.returnOkSecure,
		p.returnOkExpires, p.returnOkPort, p.returnOkDomain,
	}
	for _, chk := range checks {
		ok, err := chk(c, request)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func cookieVersion(c *cookie) int {
	if v, ok := c.version.(py.Int); ok {
		return int(v)
	}
	return 0
}

func (p *defaultPolicy) setOkVersion(c *cookie, request py.Object) (bool, error) {
	if cookieVersion(c) == 0 && !p.netscape {
		return false, nil
	}
	if cookieVersion(c) == 1 && !p.rfc2965 {
		return false, nil
	}
	return true, nil
}

func (p *defaultPolicy) setOkName(c *cookie, request py.Object) (bool, error) {
	if cookieVersion(c) == 1 && strings.HasPrefix(c.name, "$") {
		if !p.strictNsSetInitialDollar {
			return false, nil
		}
	}
	return true, nil
}

func (p *defaultPolicy) setOkVerifiability(c *cookie, request py.Object) (bool, error) {
	if !p.strictRfc2965Unverifiable {
		return true, nil
	}
	if cookieVersion(c) == 1 {
		unv, err := requestAttr(request, "unverifiable")
		if err != nil {
			return false, err
		}
		if truthy(unv) {
			return false, nil
		}
	}
	return true, nil
}

func (p *defaultPolicy) setOkPath(c *cookie, request py.Object) (bool, error) {
	if p.strictNsSetPath && cookieVersion(c) == 0 {
		rp, err := requestPath(request)
		if err != nil {
			return false, err
		}
		if c.path != rp {
			return false, nil
		}
	}
	return true, nil
}

func (p *defaultPolicy) setOkDomain(c *cookie, request py.Object) (bool, error) {
	if p.isBlocked(c.domain) {
		return false, nil
	}
	if p.isNotAllowed(c.domain) {
		return false, nil
	}
	if p.strictDomain {
		return true, nil
	}
	if cookieVersion(c) == 0 {
		if !c.domainSpecified {
			return true, nil
		}
	}
	erhn := ""
	if _, e, err := effRequestHost(request); err == nil {
		erhn = e
	}
	if erhn == "" {
		return false, nil
	}
	if !domainMatch(erhn, c.domain) {
		if p.strictNsDomain&1 != 0 && cookieVersion(c) == 0 {
			return false, nil
		}
		return false, nil
	}
	if p.strictNsDomain&2 != 0 && cookieVersion(c) == 0 && !c.domainSpecified {
		return false, nil
	}
	return true, nil
}

func (p *defaultPolicy) setOkPort(c *cookie, request py.Object) (bool, error) {
	if cookieVersion(c) != 1 {
		return true, nil
	}
	if c.port == nil || c.port == py.None {
		return true, nil
	}
	reqPort, err := requestPort(request)
	if err != nil {
		return false, err
	}
	ps, _ := py.StrAsString(c.port)
	rps, _ := py.StrAsString(reqPort)
	for _, part := range strings.Split(ps, ",") {
		if strings.TrimSpace(part) == rps {
			return true, nil
		}
	}
	return false, nil
}

func (p *defaultPolicy) returnOkVersion(c *cookie, request py.Object) (bool, error) {
	if cookieVersion(c) == 0 && !p.netscape {
		return false, nil
	}
	if cookieVersion(c) == 1 && !p.rfc2965 {
		return false, nil
	}
	return true, nil
}

func (p *defaultPolicy) returnOkVerifiability(c *cookie, request py.Object) (bool, error) {
	if !p.strictNsUnverifiable {
		return true, nil
	}
	if cookieVersion(c) != 0 {
		return true, nil
	}
	if !c.secure {
		return true, nil
	}
	unv, err := requestAttr(request, "unverifiable")
	if err != nil {
		return false, err
	}
	if truthy(unv) {
		return false, nil
	}
	return true, nil
}

func (p *defaultPolicy) returnOkSecure(c *cookie, request py.Object) (bool, error) {
	if !c.secure {
		return true, nil
	}
	proto, err := requestAttr(request, "type")
	if err != nil {
		return false, err
	}
	ps, _ := py.StrAsString(proto)
	for _, sp := range p.secureProtocols {
		if ps == sp {
			return true, nil
		}
	}
	return false, nil
}

func (p *defaultPolicy) returnOkExpires(c *cookie, request py.Object) (bool, error) {
	if c.expires == nil || c.expires == py.None {
		return true, nil
	}
	return !c.isExpired(float64(p.now)), nil
}

func (p *defaultPolicy) returnOkPort(c *cookie, request py.Object) (bool, error) {
	if cookieVersion(c) != 1 {
		return true, nil
	}
	if c.port != nil && c.port != py.None {
		return true, nil
	}
	return false, nil
}

func (p *defaultPolicy) returnOkDomain(c *cookie, request py.Object) (bool, error) {
	if cookieVersion(c) != 1 {
		_, erhn, err := effRequestHost(request)
		if err != nil {
			return false, err
		}
		return domainMatch(erhn, c.domain), nil
	}
	return true, nil
}

func (p *defaultPolicy) domainReturnOk(domain string, request py.Object) bool {
	if p.isBlocked(domain) || p.isNotAllowed(domain) {
		return false
	}
	if cookieVersion(&cookie{}) == 1 {
		return true
	}
	_, erhn, err := effRequestHost(request)
	if err != nil {
		return false
	}
	if !domainMatch(erhn, domain) {
		reqHost, _ := requestHost(request)
		if !(p.strictRfc2965Unverifiable && domainMatch(reqHost, domain)) {
			return false
		}
	}
	return true
}

func (p *defaultPolicy) pathReturnOk(path string, request py.Object) bool {
	rp, err := requestPath(request)
	if err != nil {
		return false
	}
	if rp == path {
		return true
	}
	if strings.HasPrefix(rp, path) && strings.HasSuffix(path, "/") {
		return true
	}
	if !strings.HasSuffix(path, "/") && strings.HasPrefix(rp, path+"/") {
		return true
	}
	return false
}

func (p *defaultPolicy) isBlocked(domain string) bool {
	for _, d := range p.blockedDomains {
		if userDomainMatch(domain, d) {
			return true
		}
	}
	return false
}

func (p *defaultPolicy) isNotAllowed(domain string) bool {
	if len(p.allowedDomains) == 0 {
		return false
	}
	for _, d := range p.allowedDomains {
		if userDomainMatch(domain, d) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Request plumbing
// ---------------------------------------------------------------------------

func requestAttr(request py.Object, name string) (py.Object, error) {
	v, err := py.GetAttrString(request, name)
	if err != nil {
		return py.None, nil
	}
	return v, nil
}

func callMethod(obj py.Object, name string, args ...py.Object) (py.Object, error) {
	fn, err := py.GetAttrString(obj, name)
	if err != nil {
		return nil, err
	}
	return py.Call(fn, py.Tuple(args), py.NewStringDict())
}

func callMethodOrNil(obj py.Object, name string, args ...py.Object) (py.Object, error) {
	fn, err := py.GetAttrString(obj, name)
	if err != nil {
		return py.None, nil
	}
	return py.Call(fn, py.Tuple(args), py.NewStringDict())
}

func getAllHeader(headers py.Object, name string) ([]string, error) {
	fn, err := py.GetAttrString(headers, "get_all")
	if err != nil {
		return nil, nil
	}
	res, err := py.Call(fn, py.Tuple{py.String(name)}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	if l, ok := res.(*py.List); ok {
		var out []string
		for _, it := range l.Items {
			if s, err := py.StrAsString(it); err == nil {
				out = append(out, s)
			}
		}
		return out, nil
	}
	return nil, nil
}

func requestHost(request py.Object) (string, error) {
	url, err := callMethod(request, "get_full_url")
	if err != nil {
		return "", err
	}
	us, _ := py.StrAsString(url)
	host := ""
	if h, ok := parseURLHost(us); ok {
		host = h
	}
	if host == "" {
		hv, err := callMethodOrNil(request, "get_header", py.String("Host"), py.String(""))
		if err == nil {
			host, _ = py.StrAsString(hv)
		}
	}
	host = cutPortRe.ReplaceAllString(host, "")
	return strings.ToLower(host), nil
}

func effRequestHost(request py.Object) (string, string, error) {
	reqHost, err := requestHost(request)
	if err != nil {
		return "", "", err
	}
	erhn := reqHost
	if !strings.Contains(reqHost, ".") {
		erhn = reqHost + ".local"
	}
	return reqHost, erhn, nil
}

func requestPath(request py.Object) (string, error) {
	url, err := callMethod(request, "get_full_url")
	if err != nil {
		return "", err
	}
	us, _ := py.StrAsString(url)
	path := urlPath(us)
	path = escPath(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path, nil
}

func requestPort(request py.Object) (py.Object, error) {
	host, err := py.GetAttrString(request, "host")
	if err != nil {
		return py.String(defaultHTTPPort), nil
	}
	hs, _ := py.StrAsString(host)
	if i := strings.Index(hs, ":"); i >= 0 {
		port := hs[i+1:]
		if _, err := strconv.Atoi(port); err != nil {
			return py.None, nil
		}
		return py.String(port), nil
	}
	return py.String(defaultHTTPPort), nil
}

// ---------------------------------------------------------------------------
// Type registration variables
// ---------------------------------------------------------------------------

var (
	CookieType              = py.NewTypeX("http.cookiejar.Cookie", cookieDoc, cookieNew, nil)
	CookiePolicyType        = py.NewTypeX("http.cookiejar.CookiePolicy", cookiePolicyDoc, cookiePolicyNew, nil)
	DefaultCookiePolicyType = CookiePolicyType.NewType("http.cookiejar.DefaultCookiePolicy", defaultPolicyDoc, defaultPolicyNew, nil)
	CookieJarType           = py.NewTypeX("http.cookiejar.CookieJar", cookieJarDoc, cookieJarNew, nil)
	FileCookieJarType       = CookieJarType.NewType("http.cookiejar.FileCookieJar", fileCookieJarDoc, nil, nil)
	LWPCookieJarType        = FileCookieJarType.NewType("http.cookiejar.LWPCookieJar", lwpCookieJarDoc, nil, nil)
	MozillaCookieJarType    = FileCookieJarType.NewType("http.cookiejar.MozillaCookieJar", mozillaCookieJarDoc, nil, nil)
)

const (
	cookieDoc = `HTTP Cookie class.

Information about a single HTTP cookie: its name, value, domain, path, the
flags secure/discard and the standard and non-standard attributes.`
	cookiePolicyDoc = `CookiePolicy provides a base class for cookie policy enforcement.

The set_ok and return_ok methods raise NotImplementedError; a real policy
subclass must provide them.`
	defaultPolicyDoc = `DefaultCookiePolicy implements the standard accept and return rules.

It accepts cookies from a Set-Cookie header per the Netscape rules and from a
Set-Cookie2 header per RFC 2965, and decides which cookies to return with a
request.`
	cookieJarDoc = `Collection of HTTP cookies.

Cookies are stored keyed by domain, then path, then name.  extract_cookies
reads Set-Cookie/Set-Cookie2 headers from a response; add_cookie_header writes
the Cookie header for a request.`
	fileCookieJarDoc = `A CookieJar that can be saved to and loaded from a file.

This base class does not implement the file format; LWPCookieJar and
MozillaCookieJar do.`
	lwpCookieJarDoc     = `A FileCookieJar using the libwww-perl Set-Cookie3 file format.`
	mozillaCookieJarDoc = `A FileCookieJar using the Netscape cookies.txt file format.`
)

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func init() {
	// Cookie, CookieJar and the policy are subclassable, as in CPython:
	// requests derives RequestsCookieJar from CookieJar.
	CookieType.Flags |= py.TPFLAGS_BASETYPE
	CookieJarType.Flags |= py.TPFLAGS_BASETYPE
	CookiePolicyType.Flags |= py.TPFLAGS_BASETYPE
	DefaultCookiePolicyType.Flags |= py.TPFLAGS_BASETYPE
	FileCookieJarType.Flags |= py.TPFLAGS_BASETYPE
	LWPCookieJarType.Flags |= py.TPFLAGS_BASETYPE
	MozillaCookieJarType.Flags |= py.TPFLAGS_BASETYPE

	// Cookie: the read-only attributes are exposed as properties; the rest are
	// plain python attributes with defaults set per instance.
	CookieType.Dict.Set("is_expired", py.MustNewMethod("is_expired",
		func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return cookieIsExpired(self, args, kw)
		}, 0, "True if the cookie has expired."))
	CookieType.Dict.Set("has_nonstandard_attr", py.MustNewMethod("has_nonstandard_attr",
		cookieHasNonstandardAttr, 0, "True if the cookie has the named non-standard attribute."))
	CookieType.Dict.Set("get_nonstandard_attr", py.MustNewMethod("get_nonstandard_attr",
		func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return cookieGetNonstandardAttr(self, args, kw)
		}, 0, "Return the named non-standard attribute, or default."))
	CookieType.Dict.Set("set_nonstandard_attr", py.MustNewMethod("set_nonstandard_attr",
		cookieSetNonstandardAttr, 0, "Set a non-standard attribute."))
	CookieType.Dict.Set("__str__", py.MustNewMethod("__str__", jarCookieStr, 0, "str(cookie)"))
	CookieType.Dict.Set("__repr__", py.MustNewMethod("__repr__", jarCookieStr, 0, "repr(cookie)"))

	CookiePolicyType.Dict.Set("set_ok", py.MustNewMethod("set_ok", policySetOk, 0,
		"Return true if the cookie should be set on the request."))
	CookiePolicyType.Dict.Set("return_ok", py.MustNewMethod("return_ok", policyReturnOk, 0,
		"Return true if the cookie should be returned with the request."))
	for _, name := range []string{"domain_return_ok", "path_return_ok", "return_ok_secure",
		"return_ok_expires", "return_ok_port", "return_ok_domain",
		"set_ok_version", "set_ok_name", "set_ok_verifiability",
		"set_ok_path", "set_ok_port", "set_ok_domain"} {
		CookiePolicyType.Dict.Set(name, py.MustNewMethod(name, policyReturnTrue, 0, name))
	}

	// DefaultCookiePolicy.
	for _, m := range []struct {
		name string
		fn   interface{}
	}{
		{"set_ok", func(self py.Object, args py.Tuple) (py.Object, error) {
			p, ok := self.(*defaultPolicy)
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "set_ok() requires a DefaultCookiePolicy")
			}
			if len(args) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "set_ok() takes exactly two arguments")
			}
			c, ok := args[0].(*cookie)
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "set_ok() requires a Cookie")
			}
			ok2, err := p.setOk(c, args[1])
			if err != nil {
				return nil, err
			}
			return py.Bool(ok2), nil
		}},
		{"return_ok", func(self py.Object, args py.Tuple) (py.Object, error) {
			p, ok := self.(*defaultPolicy)
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "return_ok() requires a DefaultCookiePolicy")
			}
			if len(args) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "return_ok() takes exactly two arguments")
			}
			c, ok := args[0].(*cookie)
			if !ok {
				return nil, py.ExceptionNewf(py.TypeError, "return_ok() requires a Cookie")
			}
			ok2, err := p.returnOk(c, args[1])
			if err != nil {
				return nil, err
			}
			return py.Bool(ok2), nil
		}},
	} {
		DefaultCookiePolicyType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.name))
	}
	for _, name := range []string{"domain_return_ok", "path_return_ok"} {
		DefaultCookiePolicyType.Dict.Set(name, py.MustNewMethod(name, policyReturnTrue, 0, name))
	}
	DefaultCookiePolicyType.Dict.Set("_now", py.Int(0))
	DefaultCookiePolicyType.Dict.Set("_blocked_domains", py.NewListFromItems(nil))
	DefaultCookiePolicyType.Dict.Set("_allowed_domains", py.NewListFromItems(nil))

	// CookieJar.
	for _, m := range []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"set_cookie", jarSetCookie, "Set a Cookie."},
		{"extract_cookies", jarExtractCookies, "Extract cookies from a response for a request."},
		{"add_cookie_header", jarAddCookieHeader, "Add a Cookie header to the request."},
		{"clear", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return jarClear(self, args, kw)
		}, "Clear cookies."},
		{"clear_session_cookies", jarClearSessionCookies, "Discard all session cookies."},
		{"clear_expired_cookies", jarClearExpiredCookies, "Discard all expired cookies."},
		{"set_policy", jarSetPolicy, "Set the cookie policy."},
		{"get_policy", jarGetPolicy, "Return the cookie policy."},
		{"__iter__", jarIter, "Iterate over the cookies."},
		{"__len__", jarLen, "Number of cookies."},
		{"__contains__", jarContains, "True if the cookie is in the jar."},
		{"__repr__", jarRepr, "repr of the jar."},
	} {
		CookieJarType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}

	globals := py.NewStringDict()
	for name, t := range map[string]*py.Type{
		"Cookie":              CookieType,
		"CookiePolicy":        CookiePolicyType,
		"DefaultCookiePolicy": DefaultCookiePolicyType,
		"CookieJar":           CookieJarType,
		"FileCookieJar":       FileCookieJarType,
		"LWPCookieJar":        LWPCookieJarType,
		"MozillaCookieJar":    MozillaCookieJarType,
		"LoadError":           LoadErrorType,
	} {
		globals.Set(name, t)
	}
	globals.Set("DEFAULT_HTTP_PORT", py.String(defaultHTTPPort))
	globals.Set("Absent", absentSentinel)
	globals.Set("split_header_words", py.MustNewMethod("split_header_words",
		splitHeaderWordsPy, 0, "Parse header values into lists of (key, value) pairs."))
	globals.Set("join_header_words", py.MustNewMethod("join_header_words",
		joinHeaderWordsPy, 0, "Join (key, value) pairs back into a header value."))
	globals.Set("parse_ns_headers", py.MustNewMethod("parse_ns_headers",
		parseNsHeadersPy, 0, "Ad-hoc parser for Netscape protocol cookie-attributes."))
	globals.Set("http2time", py.MustNewMethod("http2time", func(self py.Object, args py.Tuple) (py.Object, error) {
		s, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		return py.Int(http2time(s)), nil
	}, 0, "Parse an HTTP date; -1 if it cannot be parsed."))
	globals.Set("time2isoz", py.MustNewMethod("time2isoz", time2isozPy, 0, "Format a time as ISO 8601."))
	globals.Set("time2netscape", py.MustNewMethod("time2netscape", time2netscapePy, 0, "Format a time for Netscape cookie files."))
	globals.Set("is_HDN", py.MustNewMethod("is_HDN", func(self py.Object, args py.Tuple) (py.Object, error) {
		s, _ := py.StrAsString(args[0])
		return py.Bool(isHDN(s)), nil
	}, 0, "True if the text is a host domain name."))
	globals.Set("domain_match", py.MustNewMethod("domain_match", func(self py.Object, args py.Tuple) (py.Object, error) {
		a, _ := py.StrAsString(args[0])
		b, _ := py.StrAsString(args[1])
		return py.Bool(domainMatch(a, b)), nil
	}, 0, "True if string A domain-matches string B."))
	globals.Set("escape_path", py.MustNewMethod("escape_path", func(self py.Object, args py.Tuple) (py.Object, error) {
		s, _ := py.StrAsString(args[0])
		return py.String(escPath(s)), nil
	}, 0, "Escape a path."))
	globals.Set("reach", py.MustNewMethod("reach", func(self py.Object, args py.Tuple) (py.Object, error) {
		s, _ := py.StrAsString(args[0])
		return py.String(reach(s)), nil
	}, 0, "Return the reach of a host."))
	globals.Set("strip_quotes", py.MustNewMethod("strip_quotes", func(self py.Object, args py.Tuple) (py.Object, error) {
		s, _ := py.StrAsString(args[0])
		return py.String(stripQuotes(s)), nil
	}, 0, "Remove one leading and one trailing double quote."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "http.cookiejar",
			Doc:  cookiejar_doc,
		},
		Globals: globals,
	})
}

func splitHeaderWordsPy(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "split_header_words() takes exactly one argument")
	}
	l, err := py.SequenceList(args[0])
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(l.Items))
	for _, it := range l.Items {
		s, err := py.StrAsString(it)
		if err != nil {
			return nil, err
		}
		values = append(values, s)
	}
	words := splitHeaderWords(values)
	out := make([]py.Object, len(words))
	for i, pairs := range words {
		ps := make([]py.Object, len(pairs))
		for j, p := range pairs {
			ps[j] = py.Tuple{p[0], p[1]}
		}
		out[i] = py.NewListFromItems(ps)
	}
	return py.NewListFromItems(out), nil
}

func joinHeaderWordsPy(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "join_header_words() takes exactly one argument")
	}
	outer, err := py.SequenceList(args[0])
	if err != nil {
		return nil, err
	}
	lists := make([][][2]py.Object, 0, len(outer.Items))
	for _, it := range outer.Items {
		inner, err := py.SequenceList(it)
		if err != nil {
			return nil, err
		}
		var pairs [][2]py.Object
		for _, p := range inner.Items {
			pair, err := py.SequenceTuple(p)
			if err != nil || len(pair) != 2 {
				return nil, py.ExceptionNewf(py.TypeError, "join_header_words() pairs must be 2-tuples")
			}
			pairs = append(pairs, [2]py.Object{pair[0], pair[1]})
		}
		lists = append(lists, pairs)
	}
	return py.String(joinHeaderWords(lists)), nil
}

func parseNsHeadersPy(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "parse_ns_headers() takes exactly one argument")
	}
	l, err := py.SequenceList(args[0])
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(l.Items))
	for _, it := range l.Items {
		s, err := py.StrAsString(it)
		if err != nil {
			return nil, err
		}
		values = append(values, s)
	}
	words := parseNsHeaders(values)
	out := make([]py.Object, len(words))
	for i, pairs := range words {
		ps := make([]py.Object, len(pairs))
		for j, p := range pairs {
			ps[j] = py.Tuple{p[0], p[1]}
		}
		out[i] = py.NewListFromItems(ps)
	}
	return py.NewListFromItems(out), nil
}

func time2isozPy(self py.Object, args py.Tuple) (py.Object, error) {
	t := time.Now()
	if len(args) > 0 && args[0] != py.None {
		f, err := floatFromObject(args[0])
		if err != nil {
			return nil, err
		}
		t = time.Unix(int64(f), 0)
	}
	return py.String(time2isoz(t)), nil
}

func time2netscapePy(self py.Object, args py.Tuple) (py.Object, error) {
	t := time.Now()
	if len(args) > 0 && args[0] != py.None {
		f, err := floatFromObject(args[0])
		if err != nil {
			return nil, err
		}
		t = time.Unix(int64(f), 0)
	}
	return py.String(time2netscape(t)), nil
}

// quoteWithSafe is urllib.parse.quote with a custom safe set.  It lives here
// because cookiejar needs it before urllib.parse is registered.
func quoteWithSafe(s, safe string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' ||
			c >= '0' && c <= '9' || strings.IndexByte("_.-~", c) >= 0:
			b.WriteByte(c)
		case strings.IndexByte(safe, c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
		}
	}
	return b.String()
}

// parseURLHost extracts the host from a URL, used by the request helpers.
func parseURLHost(rawurl string) (string, bool) {
	schemeEnd := strings.Index(rawurl, "://")
	if schemeEnd < 0 {
		return "", false
	}
	rest := rawurl[schemeEnd+3:]
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		rest = rest[i+1:]
	}
	return rest, true
}

// urlPath extracts the path (plus query) from a URL.
func urlPath(rawurl string) string {
	schemeEnd := strings.Index(rawurl, "://")
	rest := rawurl
	if schemeEnd >= 0 {
		rest = rawurl[schemeEnd+3:]
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		return rest[i:]
	}
	return ""
}
