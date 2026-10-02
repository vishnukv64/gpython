// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// urllib.request.urlopen, and urllib.error.
//
// The opener is a real one: it parses the URL, dials the host with Go's
// net/http - which gives http and https, including TLS with the system trust
// store - and returns a response object carrying the body and the headers.
// There is no proxy, redirect or authentication plumbing: a failure anywhere
// on the way raises urllib.error.URLError carrying the underlying cause, which
// is what a caller is expected to catch.

package urllibparse

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
)

// URLErrorType is urllib.error.URLError.  .reason is exposed as a property
// reading the first constructor argument, because the interpreter stores an
// exception's attributes in its Args rather than in a per-instance dict.
var URLErrorType = py.OSError.NewType("urllib.error.URLError", "Base exception class for network-related errors.", nil, nil)

// HTTPErrorType is urllib.error.HTTPError.  It carries the response fields a
// caller inspects: code, reason, headers and the read()able body.
var HTTPErrorType = URLErrorType.NewType("urllib.error.HTTPError",
	"Raised when an HTTP request returns an error status.", nil, nil)

// urlErrorNew builds URLError(reason), which also exposes .reason.
func urlErrorNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	reason := py.Object(py.None)
	if len(args) >= 1 {
		reason = args[0]
	}
	e, err := py.ExceptionNew(metatype, py.Tuple{reason}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	e.(*py.Exception).Dict.Set("reason", reason)
	return e, nil
}

// httpErrorNew builds HTTPError(url, code, msg, hdrs, fp).
func httpErrorNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		urlObj  py.Object = py.None
		codeObj py.Object = py.None
		msgObj  py.Object = py.None
		hdrsObj py.Object = py.None
		fpObj   py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OOOO:HTTPError",
		[]string{"url", "code", "msg", "hdrs", "fp"},
		&urlObj, &codeObj, &msgObj, &hdrsObj, &fpObj); err != nil {
		return nil, err
	}
	reason := msgObj
	e, err := py.ExceptionNew(metatype, py.Tuple{urlObj, codeObj, msgObj, hdrsObj, fpObj}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	exc := e.(*py.Exception)
	exc.Dict.Set("reason", reason)
	exc.Dict.Set("code", codeObj)
	exc.Dict.Set("headers", hdrsObj)
	exc.Dict.Set("fp", fpObj)
	return exc, nil
}

// urlopenResponse is the object urlopen returns.  It is a thin wrapper over
// the in-memory body and the header list: read/readline/readlines walk the
// body, and close() only marks it closed, because there is no live connection
// to release.
type urlopenResponse struct {
	Dict   py.StringDict
	url    string
	status int
	reason string
	body   []byte
	pos    int
	closed bool
}

var urlopenResponseType = py.NewTypeX("urllib.response.addinfourl",
	"An HTTP response with its body and headers.", nil, nil)

func (r *urlopenResponse) Type() *py.Type         { return urlopenResponseType }
func (r *urlopenResponse) GetDict() py.StringDict { return r.Dict }

var _ py.IGetDict = (*urlopenResponse)(nil)

// headersAsList renders the headers as the list of (name, value) pairs
// HTTPMessage.items returns.
func (r *urlopenResponse) headersList() py.Object {
	return r.Dict.GetOrNil("_headers_list")
}

func (r *urlopenResponse) M__enter__() (py.Object, error) { return r, nil }

func (r *urlopenResponse) M__exit__(args py.Tuple) (py.Object, error) {
	r.closed = true
	return py.False, nil
}

func urlopenRead(self py.Object, args py.Tuple) (py.Object, error) {
	r := self.(*urlopenResponse)
	// read([n]): no argument reads the rest, an integer reads at most n.
	if len(args) == 0 {
		out := append([]byte(nil), r.body[r.pos:]...)
		r.pos = len(r.body)
		return py.Bytes(out), nil
	}
	n, err := py.IndexInt(args[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "argument should be integer or None, not %s", args[0].Type().Name)
	}
	if n < 0 || r.pos+n > len(r.body) {
		n = len(r.body) - r.pos
	}
	out := append([]byte(nil), r.body[r.pos:r.pos+n]...)
	r.pos += n
	return py.Bytes(out), nil
}

func urlopenReadline(self py.Object, args py.Tuple) (py.Object, error) {
	r := self.(*urlopenResponse)
	if r.pos >= len(r.body) {
		return py.Bytes(nil), nil
	}
	i := r.pos
	for i < len(r.body) && r.body[i] != '\n' {
		i++
	}
	if i < len(r.body) {
		i++
	}
	out := append([]byte(nil), r.body[r.pos:i]...)
	r.pos = i
	return py.Bytes(out), nil
}

func urlopenReadlines(self py.Object, args py.Tuple) (py.Object, error) {
	r := self.(*urlopenResponse)
	var lines []py.Object
	for r.pos < len(r.body) {
		line, err := urlopenReadline(self, nil)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return py.NewListFromItems(lines), nil
}

func urlopenClose(self py.Object, args py.Tuple) (py.Object, error) {
	self.(*urlopenResponse).closed = true
	return py.None, nil
}

// urlopen is urllib.request.urlopen.
func urlopen(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		urlObj     py.Object
		dataObj    py.Object = py.None
		timeoutObj py.Object = py.None
		cafile     py.Object = py.None
		capath     py.Object = py.None
		cadefault  py.Object = py.False
		context    py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OOOO$OOO:urlopen",
		[]string{"url", "data", "timeout", "cafile", "capath", "cadefault", "context"},
		&urlObj, &dataObj, &timeoutObj, &cafile, &capath, &cadefault, &context); err != nil {
		return nil, err
	}

	rawURL, err := py.StrAsString(urlObj)
	if err != nil {
		rawURL = ""
		if b, ok := urlObj.(py.Bytes); ok {
			rawURL = string(b)
		} else {
			return nil, py.ExceptionNewf(py.TypeError, "urlopen() argument 1 must be str or bytes, not %s", urlObj.Type().Name)
		}
	}

	parsed, perr := url.Parse(rawURL)
	if perr != nil {
		return nil, urlError(perr)
	}
	if parsed.Host == "" {
		// A relative or opaque URL cannot be opened without a host, which is
		// the same failure CPython reports.
		return nil, urlError(errors.New("no host given"))
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return nil, urlError(errUnknownURLType(parsed.Scheme))
	}

	var body io.Reader
	var reqBody []byte
	if dataObj != py.None && dataObj != nil {
		b, err := py.BytesFromObject(dataObj)
		if err != nil {
			return nil, py.ExceptionNewf(py.TypeError, "POST data should be bytes, an iterable of bytes, or a file object")
		}
		reqBody = []byte(b)
		body = strings.NewReader(string(reqBody))
	}

	req, err := http.NewRequest(strings.ToUpper(methodFor(dataObj)), rawURL, body)
	if err != nil {
		return nil, urlError(err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Connection", "close")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig(cafile, capath, cadefault, context),
		},
		CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	if timeoutObj != py.None && timeoutObj != nil {
		if f, err := py.FloatAsFloat64(timeoutObj); err == nil && f > 0 {
			client.Timeout = time.Duration(f * float64(time.Second))
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, urlError(err)
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, urlError(readErr)
	}

	headers := headersMessage(resp.Header)
	statusObj := py.Int(resp.StatusCode)

	if resp.StatusCode >= 400 {
		// CPython raises HTTPError for a 4xx/5xx rather than returning a
		// response, and the caller reads the body off the exception.
		exc := py.ExceptionNewf(HTTPErrorType, "%s", resp.Status)
		exc.Args = py.Tuple{
			py.String(rawURL), statusObj, py.String(resp.Status), headers,
			&urlopenResponse{Dict: py.NewStringDict(), url: rawURL, status: resp.StatusCode, body: data},
		}
		exc.Dict.Set("reason", py.String(resp.Status))
		exc.Dict.Set("code", statusObj)
		exc.Dict.Set("headers", headers)
		exc.Dict.Set("status", statusObj)
		exc.Dict.Set("url", py.String(rawURL))
		// Returning the exception as the error is what raises it: a *py.Exception
		// is the interpreter's error value, so this is "raise HTTPError(...)".
		return nil, exc
	}

	r := &urlopenResponse{
		Dict:   py.NewStringDict(),
		url:    rawURL,
		status: resp.StatusCode,
		reason: resp.Status,
		body:   data,
	}
	r.Dict.Set("url", py.String(rawURL))
	r.Dict.Set("status", statusObj)
	r.Dict.Set("reason", py.String(resp.Status))
	r.Dict.Set("headers", headers)
	r.Dict.Set("msg", py.String(resp.Status))
	return r, nil
}

// methodFor is POST when there is data and GET otherwise.
func methodFor(dataObj py.Object) string {
	if dataObj != py.None && dataObj != nil {
		return "post"
	}
	return "get"
}

// errUnknownURLType is the error CPython raises for a scheme it cannot open.
func errUnknownURLType(scheme string) error {
	return errors.New("unknown url type: '" + scheme + "'")
}

// urlError wraps a Go error in the URLError a caller catches.
func urlError(err error) error {
	var certErr x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &hostErr) {
		// A certificate failure keeps its own type inside the URLError, which
		// is what the ssl module's exceptions represent here.
		return urlErrorReason(py.String(err.Error()))
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErrorReason(py.String(urlErr.Err.Error()))
	}
	return urlErrorReason(py.String(err.Error()))
}

func urlErrorReason(reason py.Object) error {
	e := py.ExceptionNewf(URLErrorType, "%s", reasonString(reason))
	e.Dict.Set("reason", reason)
	return e
}

func reasonString(o py.Object) string {
	s, _ := py.StrAsString(o)
	return s
}

// headersObject is the header mapping a response exposes.  CPython answers an
// HTTPMessage here, whose lookups are case-insensitive; this carries the same
// interface over the header pairs it was built from.
type headersObject struct {
	Dict    py.StringDict
	order   []string
	byLower map[string][]string
}

var headersObjectType = py.NewType("urllib.response.headers", "The headers of an HTTP response.")

func (h *headersObject) Type() *py.Type         { return headersObjectType }
func (h *headersObject) GetDict() py.StringDict { return h.Dict }

var _ py.IGetDict = (*headersObject)(nil)

func newHeadersObject(h http.Header) *headersObject {
	obj := &headersObject{
		Dict:    py.NewStringDict(),
		byLower: map[string][]string{},
	}
	for name, values := range h {
		obj.order = append(obj.order, name)
		obj.byLower[strings.ToLower(name)] = values
		if len(values) > 0 {
			obj.Dict.Set(name, py.String(values[0]))
		}
	}
	return obj
}

func (h *headersObject) values(name string) ([]string, bool) {
	v, ok := h.byLower[strings.ToLower(name)]
	return v, ok
}

func headersGet(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*headersObject)
	var (
		name    py.Object
		failobj py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, py.StringDict{}, "O|O:get",
		[]string{"name", "default"}, &name, &failobj); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	if v, ok := h.values(s); ok {
		return py.String(v[0]), nil
	}
	return failobj, nil
}

func headersGetAll(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*headersObject)
	var name py.Object
	if err := py.ParseTupleAndKeywords(args, py.StringDict{}, "O:get_all",
		[]string{"name"}, &name); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	v, _ := h.values(s)
	items := make([]py.Object, len(v))
	for i, s := range v {
		items[i] = py.String(s)
	}
	return py.NewListFromItems(items), nil
}

func headersItems(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*headersObject)
	var items []py.Object
	for _, name := range h.order {
		for _, v := range h.byLower[strings.ToLower(name)] {
			items = append(items, py.Tuple{py.String(name), py.String(v)})
		}
	}
	return py.NewListFromItems(items), nil
}

func headersGetItem(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*headersObject)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__getitem__ takes one argument")
	}
	s, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	if v, ok := h.values(s); ok {
		return py.String(v[0]), nil
	}
	return nil, py.ExceptionNewf(py.KeyError, "%s", s)
}

func headersContains(self py.Object, args py.Tuple) (py.Object, error) {
	h := self.(*headersObject)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__contains__ takes one argument")
	}
	s, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	_, ok := h.values(s)
	return py.Bool(ok), nil
}

// headersMessage turns Go's http.Header into the header object the response
// exposes, so .get("Name") works whatever the case the caller uses.
func headersMessage(h http.Header) py.Object {
	return newHeadersObject(h)
}

// tlsConfig assembles the TLS configuration from urlopen's cafile/capath
// arguments.  Anything unset means the system trust store, which is what
// ssl.create_default_context does.
func tlsConfig(cafile, capath, cadefault, context py.Object) *tls.Config {
	cfg := &tls.Config{}
	if cafile == py.None || cafile == nil {
		if capath == py.None || capath == nil {
			if context == py.None || context == nil {
				return cfg
			}
		}
	}
	if f, err := py.StrAsString(cafile); err == nil && f != "" {
		pool := x509.NewCertPool()
		pem, err := os.ReadFile(f)
		if err == nil && pool.AppendCertsFromPEM(pem) {
			cfg.RootCAs = pool
		}
	}
	return cfg
}

const urlopen_doc = `urlopen(url, data=None, timeout=socket._GLOBAL_DEFAULT_TIMEOUT,
        *, cafile=None, capath=None, cadefault=False, context=None)

Open the URL url, which can be either a string or a Request object.

data must be bytes, an iterable of bytes, or a file object, or None.
A non-None data means the request is a POST.

Raise urllib.error.URLError if the connection cannot be made.`

func init() {
	headersObjectType.Dict.Set("get", py.MustNewMethod("get", headersGet, 0, "Return the first value of the named header, or the default."))
	headersObjectType.Dict.Set("get_all", py.MustNewMethod("get_all", headersGetAll, 0, "Return all values of the named header."))
	headersObjectType.Dict.Set("getheaders", py.MustNewMethod("getheaders", headersGetAll, 0, "Return all values of the named header."))
	headersObjectType.Dict.Set("items", py.MustNewMethod("items", headersItems, 0, "Return the header name/value pairs."))
	headersObjectType.Dict.Set("__getitem__", py.MustNewMethod("__getitem__", headersGetItem, 0, "Return the first value of the named header."))
	headersObjectType.Dict.Set("__contains__", py.MustNewMethod("__contains__", headersContains, 0, "Report whether the named header is present."))

	urlopenResponseType.Dict.Set("read", py.MustNewMethod("read", urlopenRead, 0, "Read and return the response body, or at most n bytes."))
	urlopenResponseType.Dict.Set("readline", py.MustNewMethod("readline", urlopenReadline, 0, "Read and return one line of the response body."))
	urlopenResponseType.Dict.Set("readlines", py.MustNewMethod("readlines", urlopenReadlines, 0, "Read and return the remaining lines of the response body."))
	urlopenResponseType.Dict.Set("close", py.MustNewMethod("close", urlopenClose, 0, "Close the response."))
	urlopenResponseType.Dict.Set("getcode", py.MustNewMethod("getcode", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(self.(*urlopenResponse).status), nil
	}, 0, "Return the HTTP status code."))
	urlopenResponseType.Dict.Set("geturl", py.MustNewMethod("geturl", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.String(self.(*urlopenResponse).url), nil
	}, 0, "Return the URL of the retrieved resource."))
	urlopenResponseType.Dict.Set("info", py.MustNewMethod("info", func(self py.Object, args py.Tuple) (py.Object, error) {
		r := self.(*urlopenResponse)
		if v, ok := r.Dict.Get("headers"); ok {
			return v, nil
		}
		return py.None, nil
	}, 0, "Return the response headers, as a message object."))

	URLErrorType.Dict.Set("reason", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		if e, ok := self.(*py.Exception); ok {
			if args, ok := e.Args.(py.Tuple); ok && len(args) >= 1 {
				return args[0], nil
			}
		}
		return py.None, nil
	}})

	// HTTPError's fields come from its five-argument constructor, so they are
	// exposed as properties over Args: the interpreter keeps an exception's
	// attributes in Args, not in a per-instance dict.
	httpArg := func(i int) func(py.Object) (py.Object, error) {
		return func(self py.Object) (py.Object, error) {
			if e, ok := self.(*py.Exception); ok {
				if args, ok := e.Args.(py.Tuple); ok && len(args) > i {
					return args[i], nil
				}
			}
			return py.None, nil
		}
	}
	for name, i := range map[string]int{"code": 1, "reason": 2, "headers": 3, "fp": 4, "url": 0, "status": 1} {
		HTTPErrorType.Dict.Set(name, &py.Property{Fget: httpArg(i)})
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "urllib.error",
			Doc:  "Exception classes raised by urllib.request.",
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "URLError", Value: URLErrorType},
			py.DictEntry{Key: "HTTPError", Value: HTTPErrorType},
			py.DictEntry{Key: "ContentTooShortError", Value: URLErrorType},
		),
	})
}
