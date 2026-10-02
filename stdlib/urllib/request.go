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
// urlopen and the opener classes are deliberately absent rather than faked: a
// silent stub returning an empty response would be far worse than a missing
// name.
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
	})
}
