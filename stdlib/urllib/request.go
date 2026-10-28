// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// urllib.request -- extensible library for opening URLs.
//
// This interpreter cannot open a URL - there is no working HTTP transport and
// no TLS - so urllib.request here provides the proxy and header helpers that
// are pure computation, and nothing that would fetch.  requests imports
// getproxies, getproxies_environment, parse_http_list, proxy_bypass and
// proxy_bypass_environment from this module, and those are all implemented
// honestly: getproxies reads the standard proxy environment variables, and
// parse_http_list is CPython's header-list splitter.
//
// urlopen is real (Go, below).  The opener machinery is not: build_opener and
// urlretrieve raise NotImplementedError rather than return a stub that
// silently ignores its handlers.  The names exist, with the pure parts real
// (Request, pathname2url, url2pathname, HTTPPasswordMgr), because libraries
// import them at module load without ever opening a URL - pip's vendored
// distlib does, and could not otherwise be imported.
package urllibparse

import (
	"os"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const request_doc = `An extensible library for opening URLs using a variety of protocols.

This implementation provides the proxy and header helpers
(getproxies, getproxies_environment, proxy_bypass, proxy_bypass_environment)
and parse_http_list.  urlopen and the opener/handler classes are NOT provided:
this interpreter cannot open a URL, so there is no honest way to implement
them.`

// proxyEnvVars are the environment variables getproxies reads, and the scheme
// each one names.
var proxyEnvVars = []struct{ env, scheme string }{
	{"http_proxy", "http"},
	{"https_proxy", "https"},
	{"ftp_proxy", "ftp"},
	{"all_proxy", "all"},
}

// getProxiesEnvironment returns a dict of the proxy environment variables that
// are set, with the scheme as the key.
func getProxiesEnvironment() py.StringDict {
	d := py.NewStringDict()
	for _, e := range proxyEnvVars {
		if v := os.Getenv(e.env); v != "" {
			d.Set(e.scheme, py.String(v))
		}
	}
	// The upper-case variables are the Windows spellings; CPython reads them
	// in preference order, and a plain environment can have either.
	for _, e := range proxyEnvVars {
		if v := os.Getenv(strings.ToUpper(e.env)); v != "" {
			if _, ok := d.Get(e.scheme); !ok {
				d.Set(e.scheme, py.String(v))
			}
		}
	}
	if no := os.Getenv("no_proxy"); no != "" {
		d.Set("no", py.String(no))
	} else if no := os.Getenv("NO_PROXY"); no != "" {
		d.Set("no", py.String(no))
	}
	return d
}

func urllibGetProxies(self py.Object, args py.Tuple) (py.Object, error) {
	return getProxiesEnvironment(), nil
}

func urllibGetProxiesEnvironment(self py.Object, args py.Tuple) (py.Object, error) {
	return getProxiesEnvironment(), nil
}

// parseHTTPList splits a header value on commas, honouring quoting and
// ignoring empty fields, exactly as CPython's parse_http_list.
func parseHTTPList(s string) []string {
	var out []string
	i := 0
	for i < len(s) {
		// Skip leading whitespace and commas.
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			break
		}
		// Collect one field, which may be quoted.
		var b strings.Builder
		if s[i] == '"' {
			i++
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					b.WriteByte(s[i+1])
					i += 2
					continue
				}
				if s[i] == '"' {
					// An escaped quote inside the quoted string.
					if i+1 < len(s) && s[i+1] == '"' {
						b.WriteByte('"')
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteByte(s[i])
				i++
			}
			for i < len(s) && s[i] != ',' {
				i++
			}
		} else {
			start := i
			for i < len(s) && s[i] != ',' {
				i++
			}
			b.WriteString(strings.TrimRight(s[start:i], " \t"))
		}
		out = append(out, b.String())
	}
	return out
}

func urllibParseHTTPList(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "parse_http_list() takes exactly one argument")
	}
	s, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	items := parseHTTPList(s)
	out := make([]py.Object, len(items))
	for i, it := range items {
		out[i] = py.String(it)
	}
	return py.NewListFromItems(out), nil
}

// proxyByPassEnvironment reports whether host is in the no_proxy list.
func proxyByPassEnvironment(host string, noProxy string) bool {
	for _, entry := range strings.Split(noProxy, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == "*" {
			return true
		}
		// Strip a leading dot and a :port.
		e := strings.TrimPrefix(entry, ".")
		if i := strings.LastIndex(e, ":"); i >= 0 {
			e = e[:i]
		}
		if e == host || strings.HasSuffix(host, "."+e) {
			return true
		}
	}
	return false
}

func urllibProxyBypassEnvironment(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "proxy_bypass_environment() takes exactly one argument")
	}
	host, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	no := os.Getenv("no_proxy")
	if no == "" {
		no = os.Getenv("NO_PROXY")
	}
	return py.Bool(proxyByPassEnvironment(host, no)), nil
}

func urllibProxyBypass(self py.Object, args py.Tuple) (py.Object, error) {
	return urllibProxyBypassEnvironment(self, args)
}

func init() {
	globals := py.NewStringDict()
	globals.Set("getproxies", py.MustNewMethod("getproxies", urllibGetProxies, 0,
		"Return a dictionary of scheme -> proxy server URL mappings."))
	globals.Set("getproxies_environment", py.MustNewMethod("getproxies_environment", urllibGetProxiesEnvironment, 0,
		"Return a dictionary of scheme -> proxy server URL mappings from the environment."))
	globals.Set("parse_http_list", py.MustNewMethod("parse_http_list", urllibParseHTTPList, 0,
		"Parse a list of comma-separated header fields, honouring quoting."))
	globals.Set("proxy_bypass", py.MustNewMethod("proxy_bypass", urllibProxyBypass, 0,
		"Return True if the proxy should be bypassed for the given host."))
	globals.Set("proxy_bypass_environment", py.MustNewMethod("proxy_bypass_environment", urllibProxyBypassEnvironment, 0,
		"Return True if the proxy should be bypassed for the given host, per no_proxy."))
	globals.Set("urlopen", py.MustNewMethod("urlopen", urlopen, 0, urlopen_doc))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "urllib.request",
			Doc:  request_doc,
		},
		Globals: globals,
		CodeSrc: requestSrc,
	})
}

// requestSrc is the Python half of urllib.request.
const requestSrc = `
import os
from urllib.error import URLError
from urllib.parse import quote, unquote, urlsplit, urlunsplit, urldefrag

def pathname2url(pathname, *, add_scheme=False):
    if os.name == 'nt':
        pathname = pathname.replace('\\', '/')
    scheme = 'file:' if add_scheme else ''
    drive, root, tail = os.path.splitroot(pathname)
    if drive:
        if drive[:4] == '//?/':
            drive = drive[4:]
            if drive[:4].upper() == 'UNC/':
                drive = '//' + drive[4:]
        if drive[1:] == ':':
            drive = '///' + drive
        drive = quote(drive, safe='/:')
    elif root:
        root = '//' + root
    return scheme + drive + root + quote(tail)


# ponytail: only an empty authority or "localhost" counts as local; CPython
# also resolves the host name (resolve_host=True) - add if file URLs naming
# this machine's host are needed.
def url2pathname(url, *, require_scheme=False, resolve_host=False):
    if not require_scheme:
        url = 'file:' + url
    scheme, authority, url = urlsplit(url)[:3]
    if scheme != 'file':
        raise URLError("URL is missing a 'file:' scheme")
    if os.name == 'nt':
        if authority[1:2] == ':':
            url = authority + url
        elif authority not in ('', 'localhost'):
            url = '//' + authority + url
        elif url[:3] == '///':
            url = url[1:]
        else:
            if url[:1] == '/' and url[2:3] in (':', '|'):
                url = url[1:]
            if url[1:2] == '|':
                url = url[:1] + ':' + url[2:]
        url = url.replace('/', '\\')
    elif authority not in ('', 'localhost'):
        raise URLError("file:// scheme is supported only on localhost")
    return unquote(url)


class Request:
    def __init__(self, url, data=None, headers={}, origin_req_host=None,
                 unverifiable=False, method=None):
        self.full_url = url
        self.headers = {}
        self.unredirected_hdrs = {}
        self._data = data
        self.origin_req_host = origin_req_host
        self.unverifiable = unverifiable
        if method:
            self.method = method
        for key, value in headers.items():
            self.add_header(key, value)

    @property
    def full_url(self):
        if self.fragment:
            return "{}#{}".format(self._full_url, self.fragment)
        return self._full_url

    @full_url.setter
    def full_url(self, url):
        self._full_url, self.fragment = urldefrag(url)
        parts = urlsplit(self._full_url)
        self.type = parts.scheme or None
        self.host = parts.netloc
        self.selector = urlunsplit(("", "", parts.path, parts.query, ""))

    @property
    def data(self):
        return self._data

    @data.setter
    def data(self, data):
        self._data = data

    def get_method(self):
        default_method = "POST" if self.data is not None else "GET"
        return getattr(self, "method", default_method)

    def get_full_url(self):
        return self.full_url

    def add_header(self, key, val):
        self.headers[key.capitalize()] = val

    def add_unredirected_header(self, key, val):
        self.unredirected_hdrs[key.capitalize()] = val

    def has_header(self, header_name):
        return header_name in self.headers or header_name in self.unredirected_hdrs

    def get_header(self, header_name, default=None):
        return self.headers.get(header_name, self.unredirected_hdrs.get(header_name, default))

    def header_items(self):
        hdrs = {**self.unredirected_hdrs, **self.headers}
        return list(hdrs.items())


class BaseHandler:
    handler_order = 500

    def add_parent(self, parent):
        self.parent = parent

    def close(self):
        pass


class HTTPHandler(BaseHandler):
    pass


class HTTPSHandler(BaseHandler):
    def __init__(self, debuglevel=None, context=None, check_hostname=None):
        self._context = context


class HTTPRedirectHandler(BaseHandler):
    max_redirections = 10


class HTTPPasswordMgr:
    def __init__(self):
        self.passwd = {}

    def add_password(self, realm, uri, user, passwd):
        if isinstance(uri, str):
            uri = [uri]
        self.passwd.setdefault(realm, {})
        for u in uri:
            self.passwd[realm][u] = (user, passwd)

    def find_user_password(self, realm, authuri):
        domains = self.passwd.get(realm, {})
        for uris, authinfo in domains.items():
            if authuri == uris or authuri.startswith(uris):
                return authinfo
        return None, None


class HTTPBasicAuthHandler(BaseHandler):
    def __init__(self, password_mgr=None):
        if password_mgr is None:
            password_mgr = HTTPPasswordMgr()
        self.passwd = password_mgr


def build_opener(*handlers):
    raise NotImplementedError("urllib.request.build_opener: openers and handlers are not supported; use urlopen")


def urlretrieve(url, filename=None, reporthook=None, data=None):
    raise NotImplementedError("urllib.request.urlretrieve is not supported; use urlopen")
`
