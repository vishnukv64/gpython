// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// http.client -- HTTP protocol client.
//
// This module provides the classes and status constants that Python code
// imports from http.client.  urllib3, and therefore requests, imports the
// exception classes, HTTPMessage and HTTPResponse, and derives its own
// connection classes from HTTPConnection, so those have to exist and be
// subclassable.
//
// What works:
//
//   - every exception class, with its CPython hierarchy and constructor
//     behaviour (IncompleteRead carries .partial and .expected, BadStatusLine
//     carries .line, LineTooLong builds its message from _MAXLINE)
//   - every status constant and the responses dict
//   - HTTPMessage, a header container with get/get_all/getheaders/items/keys/
//     values, __getitem__, __contains__, __iter__ and __len__
//   - HTTPResponse, with status/reason/version/msg/headers and read/readinto/
//     getheader/getheaders, reading from an optional file-like object
//   - the connection classes HTTPConnection/HTTPSConnection: constructing one
//     stores host/port/timeout, and close/set_tunnel work, but the
//     request/response cycle over a real socket is NOT implemented
//
// What does NOT work:
//
//   - actually talking HTTP.  This interpreter has no working TLS and this
//     module never opens a socket or fakes a response: connect/request/
//     putrequest/putheader/send/endheaders/getresponse all raise
//     NotImplementedError naming the limit.
package http

import (
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const client_doc = `HTTP protocol client

This module provides the following classes:

    HTTPConnection
    HTTPSConnection
    HTTPResponse
    HTTPMessage

and the exceptions:

    HTTPException, NotConnected, CannotSendRequest, CannotSendHeader,
    ResponseNotReady, BadStatusLine, IncompleteRead, UnknownProtocol,
    LineTooLong, InvalidURL

plus the HTTP status constants (OK, NOT_FOUND, ...) and the responses dict.
`

// maxLine is the maximum length a line may be before LineTooLong is raised.
const maxLine = 65536

// ---------------------------------------------------------------------------
// Exceptions
// ---------------------------------------------------------------------------

// The exception hierarchy matches CPython exactly so that "except
// HTTPException" catches what it should and urllib3's own subclasses (which
// derive from these) inherit the right constructors.
var (
	HTTPExceptionType = py.ExceptionType.NewType("http.client.HTTPException",
		"Base class for all http exceptions.", nil, nil)
	NotConnectedType            = HTTPExceptionType.NewType("http.client.NotConnected", "Not connected", nil, nil)
	InvalidURLType              = HTTPExceptionType.NewType("http.client.InvalidURL", "InvalidURL", nil, nil)
	UnknownProtocolType         = HTTPExceptionType.NewType("http.client.UnknownProtocol", "UnknownProtocol", nil, nil)
	UnknownTransferEncodingType = HTTPExceptionType.NewType("http.client.UnknownTransferEncoding", "UnknownTransferEncoding", nil, nil)
	UnimplementedFileModeType   = HTTPExceptionType.NewType("http.client.UnimplementedFileMode", "UnimplementedFileMode", nil, nil)
	ImproperConnectionStateType = HTTPExceptionType.NewType("http.client.ImproperConnectionState", "ImproperConnectionState", nil, nil)
	CannotSendRequestType       = ImproperConnectionStateType.NewType("http.client.CannotSendRequest", "CannotSendRequest", nil, nil)
	CannotSendHeaderType        = ImproperConnectionStateType.NewType("http.client.CannotSendHeader", "CannotSendHeader", nil, nil)
	ResponseNotReadyType        = ImproperConnectionStateType.NewType("http.client.ResponseNotReady", "ResponseNotReady", nil, nil)
	BadStatusLineType           = HTTPExceptionType.NewType("http.client.BadStatusLine", "BadStatusLine", badStatusLineNew, nil)
	LineTooLongType             = HTTPExceptionType.NewType("http.client.LineTooLong", "LineTooLong", lineTooLongNew, nil)
	IncompleteReadType          = HTTPExceptionType.NewType("http.client.IncompleteRead", "IncompleteRead", incompleteReadNew, nil)
)

// The classes urllib3 subclasses or reads, created here so client.go can
// reference them before init() registers their methods.
var (
	HTTPMessageType    = py.NewTypeX("http.client.HTTPMessage", httpMessageDoc, httpMessageNew, nil)
	HTTPResponseType   = py.NewTypeX("http.client.HTTPResponse", httpResponseDoc, httpResponseNew, nil)
	HTTPConnectionType = py.NewTypeX("http.client.HTTPConnection", httpConnectionDoc, nil, nil)
)

// HTTPSConnection is an HTTPConnection: CPython swaps in a TLS socket here,
// which this interpreter cannot do, so it shares the base class and keeps the
// HTTPS default port.
var HTTPSConnectionType = HTTPConnectionType.NewType("http.client.HTTPSConnection", httpsConnectionDoc, nil, nil)

// HTTPConnection and HTTPMessage are subclassable, as in CPython: urllib3
// derives its own HTTPConnection and HTTPSConnection from this class.
func init() {
	HTTPConnectionType.Flags |= py.TPFLAGS_BASETYPE
	HTTPSConnectionType.Flags |= py.TPFLAGS_BASETYPE
	HTTPMessageType.Flags |= py.TPFLAGS_BASETYPE
	HTTPResponseType.Flags |= py.TPFLAGS_BASETYPE
}

// incompleteReadNew builds IncompleteRead(partial, expected=None).  CPython
// stores only partial in args and hangs .partial/.expected off the instance;
// urllib3 reads exactly those two attributes.
func incompleteReadNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, py.ExceptionNewf(py.TypeError,
			"IncompleteRead() takes 1 or 2 arguments (%d given)", len(args))
	}
	partial := args[0]
	var expected py.Object = py.None
	if len(args) == 2 {
		expected = args[1]
	}
	e, err := py.ExceptionNew(metatype, py.Tuple{partial}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	exc := e.(*py.Exception)
	exc.Dict.Set("partial", partial)
	exc.Dict.Set("expected", expected)
	return exc, nil
}

// incompleteReadStr renders like CPython:
//
//	IncompleteRead(3 bytes read, 5 more expected)
//	IncompleteRead(3 bytes read)
func incompleteReadStr(self py.Object, args py.Tuple) (py.Object, error) {
	exc, ok := self.(*py.Exception)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "descriptor '__str__' requires an IncompleteRead")
	}
	n := 0
	if b, err := py.BytesFromObject(exc.Dict.GetOrNil("partial")); err == nil {
		n = len(b)
	}
	expected := exc.Dict.GetOrNil("expected")
	if expected == nil || expected == py.None {
		return py.String("IncompleteRead(" + strconv.Itoa(n) + " bytes read)"), nil
	}
	m, err := py.IndexInt(expected)
	if err != nil {
		return nil, err
	}
	return py.String("IncompleteRead(" + strconv.Itoa(n) + " bytes read, " + strconv.Itoa(m) + " more expected)"), nil
}

// badStatusLineNew builds BadStatusLine(line).  CPython replaces a falsy line
// with its repr before storing it in args and .line.
func badStatusLineNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var line py.Object = py.None
	if len(args) >= 1 {
		line = args[0]
	}
	arg := line
	if arg == nil || arg == py.None || isFalsy(arg) {
		rep, err := py.ReprAsString(arg)
		if err != nil {
			return nil, err
		}
		arg = py.String(rep)
	}
	e, err := py.ExceptionNew(metatype, py.Tuple{arg}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	e.(*py.Exception).Dict.Set("line", arg)
	return e, nil
}

// lineTooLongNew builds LineTooLong(line_type) with CPython's message.
func lineTooLongNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	lineType := ""
	if len(args) >= 1 {
		if s, err := py.StrAsString(args[0]); err == nil {
			lineType = s
		}
	}
	return py.ExceptionNewf(metatype,
		"got more than %d bytes when reading %s", maxLine, lineType), nil
}

// isFalsy reports the truth value of obj, treating an error as true.
func isFalsy(obj py.Object) bool {
	b, err := py.ObjectIsTrue(obj)
	if err != nil {
		return false
	}
	return !b
}

// stringArg pulls a required string out of a positional or keyword argument.
func stringArg(args py.Tuple, kwargs py.StringDict, format string, kwlist []string) (string, error) {
	var s string
	out := make([]*py.Object, len(kwlist))
	for i := range out {
		out[i] = new(py.Object)
	}
	*out[0] = py.String("")
	if err := py.ParseTupleAndKeywords(args, kwargs, format, kwlist, out...); err != nil {
		return "", err
	}
	if v, ok := (*out[0]).(py.String); ok {
		s = string(v)
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// HTTPMessage
// ---------------------------------------------------------------------------

// httpMessage is the header container returned as HTTPResponse.msg.  CPython's
// HTTPMessage is an email.message.Message, but the interface the ecosystem
// uses is get/get_all/getheaders/items/keys/values/__getitem__/__contains__/
// __iter__/__len__, which is what this provides.  Header names are
// case-insensitive; a repeated header keeps every value and get_all returns
// them all.
type httpMessage struct {
	names  []string // lower-cased names, in insertion order (may repeat)
	values []string
	Dict   py.StringDict
}

func (m *httpMessage) Type() *py.Type         { return HTTPMessageType }
func (m *httpMessage) GetDict() py.StringDict { return m.Dict }

func (m *httpMessage) get(name string) (string, bool) {
	lower := strings.ToLower(name)
	for i, n := range m.names {
		if n == lower {
			return m.values[i], true
		}
	}
	return "", false
}

func (m *httpMessage) getAll(name string) []string {
	lower := strings.ToLower(name)
	var out []string
	for i, n := range m.names {
		if n == lower {
			out = append(out, m.values[i])
		}
	}
	return out
}

func httpMessageNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return &httpMessage{Dict: py.NewStringDict()}, nil
}

func msgGet(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*httpMessage)
	var name py.Object = py.String("")
	var failobj py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "S|O:get", []string{"name", "failobj"}, &name, &failobj); err != nil {
		return nil, err
	}
	if v, ok := m.get(string(name.(py.String))); ok {
		return py.String(v), nil
	}
	return failobj, nil
}

func msgGetAll(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*httpMessage)
	var name py.Object = py.String("")
	var failobj py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "S|O:get_all", []string{"name", "failobj"}, &name, &failobj); err != nil {
		return nil, err
	}
	vals := m.getAll(string(name.(py.String)))
	if len(vals) == 0 {
		return failobj, nil
	}
	items := make([]py.Object, len(vals))
	for i, v := range vals {
		items[i] = py.String(v)
	}
	return py.NewListFromItems(items), nil
}

func msgGetHeaders(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	name, err := stringArg(args, kwargs, "S:getheaders", []string{"name"})
	if err != nil {
		return nil, err
	}
	return msgGetAll(self, py.Tuple{py.String(name)}, py.NewStringDict())
}

func msgItems(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*httpMessage)
	items := make([]py.Object, len(m.names))
	for i, n := range m.names {
		items[i] = py.Tuple{py.String(n), py.String(m.values[i])}
	}
	return py.NewListFromItems(items), nil
}

func msgKeys(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*httpMessage)
	items := make([]py.Object, len(m.names))
	for i, n := range m.names {
		items[i] = py.String(n)
	}
	return py.NewListFromItems(items), nil
}

func msgValues(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*httpMessage)
	items := make([]py.Object, len(m.values))
	for i, v := range m.values {
		items[i] = py.String(v)
	}
	return py.NewListFromItems(items), nil
}

func msgGetItem(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*httpMessage)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__getitem__() takes exactly one argument")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	if v, ok := m.get(name); ok {
		return py.String(v), nil
	}
	return nil, py.ExceptionNewf(py.KeyError, "%s", name)
}

func msgContains(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*httpMessage)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__contains__() takes exactly one argument")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return py.Bool(false), nil
	}
	_, ok := m.get(name)
	return py.Bool(ok), nil
}

func msgLen(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Int(len(self.(*httpMessage).names)), nil
}

// msgIter yields the header names.
func msgIter(self py.Object, args py.Tuple) (py.Object, error) {
	return msgKeys(self, args)
}

// msgAddHeader is HTTPMessage.add_header(name, value, **params).
func msgAddHeader(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*httpMessage)
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "add_header() takes at least two arguments")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	value, err := py.StrAsString(args[1])
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(value)
	if !kwargs.IsNil() {
		kwargs.Range(func(k string, v py.Object) bool {
			s, err := py.StrAsString(v)
			if err == nil {
				b.WriteString("; ")
				b.WriteString(strings.ReplaceAll(k, "_", "-"))
				b.WriteString("=\"")
				b.WriteString(s)
				b.WriteString("\"")
			}
			return true
		})
	}
	m.names = append(m.names, strings.ToLower(name))
	m.values = append(m.values, b.String())
	return py.None, nil
}

func msgRepr(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*httpMessage)
	var b strings.Builder
	b.WriteString("<http.client.HTTPMessage object\n")
	for i, n := range m.names {
		b.WriteString("  ")
		b.WriteString(n)
		b.WriteString(": ")
		b.WriteString(m.values[i])
		b.WriteString("\n")
	}
	b.WriteString(">")
	return py.String(b.String()), nil
}

// ---------------------------------------------------------------------------
// HTTPResponse
// ---------------------------------------------------------------------------

// httpResponse is the response object: status, reason, version, the header
// message and an optional file-like object to read the body from.  urllib3
// reads these attributes off whatever getresponse() returns, so the surface
// matters more than a real parser.
type httpResponse struct {
	status  int
	reason  string
	version int
	msg     *httpMessage
	fp      py.Object // file-like or None
	closed  bool
	Dict    py.StringDict
}

func (r *httpResponse) Type() *py.Type         { return HTTPResponseType }
func (r *httpResponse) GetDict() py.StringDict { return r.Dict }

func httpResponseNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) > 0 {
		return nil, py.ExceptionNewf(py.TypeError, "HTTPResponse() takes no arguments")
	}
	return &httpResponse{
		version: 11,
		msg:     &httpMessage{Dict: py.NewStringDict()},
		fp:      py.None,
		Dict:    py.NewStringDict(),
	}, nil
}

func respStatus(self py.Object) py.Object  { return py.Int(self.(*httpResponse).status) }
func respReason(self py.Object) py.Object  { return py.String(self.(*httpResponse).reason) }
func respVersion(self py.Object) py.Object { return py.Int(self.(*httpResponse).version) }
func respMsg(self py.Object) py.Object     { return self.(*httpResponse).msg }
func respHeaders(self py.Object) py.Object { return self.(*httpResponse).msg }
func respClosed(self py.Object) py.Object  { return py.Bool(self.(*httpResponse).closed) }

func respGetHeader(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	r := self.(*httpResponse)
	var name py.Object = py.String("")
	var dflt py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "S|O:getheader", []string{"name", "default"}, &name, &dflt); err != nil {
		return nil, err
	}
	return msgGet(r.msg, py.Tuple{name, dflt}, py.NewStringDict())
}

func respGetHeaders(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	name, err := stringArg(args, kwargs, "S:getheaders", []string{"name"})
	if err != nil {
		return nil, err
	}
	return msgGetAll(self.(*httpResponse).msg, py.Tuple{py.String(name)}, py.NewStringDict())
}

func respRead(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	r := self.(*httpResponse)
	if r.fp == nil || r.fp == py.None {
		return py.Bytes(nil), nil
	}
	amt := -1
	if len(args) > 0 && args[0] != py.None {
		n, err := py.IndexInt(args[0])
		if err != nil {
			return nil, err
		}
		amt = n
	}
	data, err := readFrom(r.fp, amt)
	if err != nil {
		return nil, err
	}
	return py.Bytes(data), nil
}

func respReadInto(self py.Object, args py.Tuple) (py.Object, error) {
	r := self.(*httpResponse)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "readinto() takes exactly one argument")
	}
	b, ok := args[0].(*py.ByteArray)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "readinto() argument must be a writable bytes-like object")
	}
	if r.fp == nil || r.fp == py.None {
		return py.Int(0), nil
	}
	n, err := b.M__len__()
	if err != nil {
		return nil, err
	}
	amt, err := py.IndexInt(n)
	if err != nil {
		return nil, err
	}
	data, err := readFrom(r.fp, amt)
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		if _, err := b.M__setitem__(&py.Slice{Start: py.Int(0), Stop: py.Int(len(data))}, py.Bytes(data)); err != nil {
			return nil, err
		}
	}
	return py.Int(len(data)), nil
}

func respClose(self py.Object, args py.Tuple) (py.Object, error) {
	r := self.(*httpResponse)
	if r.fp != nil && r.fp != py.None {
		fn, err := py.GetAttrString(r.fp, "close")
		if err == nil {
			if _, err := py.Call(fn, py.Tuple{}, py.NewStringDict()); err != nil {
				return nil, err
			}
		}
	}
	r.closed = true
	return py.None, nil
}

func respIsClosed(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Bool(self.(*httpResponse).closed), nil
}

// readFrom reads up to amt bytes from a python file-like object (amt < 0 means
// read everything).
func readFrom(fp py.Object, amt int) ([]byte, error) {
	readFn, err := py.GetAttrString(fp, "read")
	if err != nil {
		return nil, err
	}
	var res py.Object
	if amt < 0 {
		res, err = py.Call(readFn, py.Tuple{}, py.NewStringDict())
	} else {
		res, err = py.Call(readFn, py.Tuple{py.Int(amt)}, py.NewStringDict())
	}
	if err != nil {
		return nil, err
	}
	b, err := py.BytesFromObject(res)
	if err != nil {
		return nil, err
	}
	return []byte(b), nil
}

// ---------------------------------------------------------------------------
// Connections
// ---------------------------------------------------------------------------

// httpConnection is the base connection class.  Constructing one records the
// parameters; no socket is opened and no bytes are sent.  urllib3 derives
// HTTPConnection from this and calls super().__init__(...) / super().close(),
// so __init__ and close exist and are callable, while the methods that would
// have to talk to a server raise NotImplementedError naming the limit.
type httpConnection struct {
	host    string
	port    int
	timeout py.Object
	Dict    py.StringDict
}

func (c *httpConnection) Type() *py.Type         { return HTTPConnectionType }
func (c *httpConnection) GetDict() py.StringDict { return c.Dict }

const (
	httpMessageDoc = `HTTPMessage is the header container for a response.

It answers get/get_all/getheaders/items/keys/values, and supports
"msg[name]", "name in msg", len(msg) and iteration over the header names.
Header names are matched case-insensitively and a repeated header keeps
every value.`
	httpResponseDoc = `An HTTPResponse object.

It carries status, reason, version, msg (the headers) and read/readinto for
the body.  This interpreter has no HTTP transport, so the only way to obtain
one is to build it directly.`
	httpConnectionDoc = `An HTTPConnection object.

Constructing one records the host, port and timeout, but this interpreter has
no HTTP transport, so connect/request/putrequest/putheader/send/endheaders/
getresponse all raise NotImplementedError.`
	httpsConnectionDoc = `An HTTPSConnection object.

Same as HTTPConnection but with the HTTPS default port.  Actual TLS is not
implemented in this interpreter, so no method here performs a handshake.`
)

// connInit implements HTTPConnection.__init__(host, port=None, timeout=...,
// source_address=None, blocksize=8192).  Only host/port/timeout are kept; the
// rest are accepted and ignored, so a subclass calling super().__init__ with
// urllib3's full argument list does not fail.
func connInit(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c, ok := self.(*httpConnection)
	if !ok {
		return py.None, nil
	}
	var host py.Object = py.String("")
	port := py.Object(py.None)
	timeout := py.Object(py.None)
	sourceAddress := py.Object(py.None)
	blocksize := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "S|OOOO:__init__",
		[]string{"host", "port", "timeout", "source_address", "blocksize"},
		&host, &port, &timeout, &sourceAddress, &blocksize); err != nil {
		return nil, err
	}
	c.host = string(host.(py.String))
	if port != py.None {
		p, err := py.IndexInt(port)
		if err != nil {
			return nil, err
		}
		c.port = p
	} else {
		c.port = defaultPortFor(c.Type())
	}
	if c.Dict.IsNil() {
		c.Dict = py.NewStringDict()
	}
	c.Dict.Set("timeout", timeout)
	return py.None, nil
}

// defaultPortFor returns the port CPython uses when none is given: 443 for
// HTTPSConnection, 80 otherwise.
func defaultPortFor(t *py.Type) int {
	if t == HTTPSConnectionType {
		return 443
	}
	return 80
}

func connClose(self py.Object, args py.Tuple) (py.Object, error) {
	return py.None, nil
}

func connNotImplemented(name string) func(py.Object, py.Tuple, py.StringDict) (py.Object, error) {
	return func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"http.client.HTTPConnection.%s: this interpreter has no working HTTP transport "+
				"(no TLS, no socket client), so the request/response cycle is not implemented", name)
	}
}

func connSetTunnel(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c, ok := self.(*httpConnection)
	if !ok {
		return py.None, nil
	}
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "set_tunnel() requires host")
	}
	host, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	c.Dict.Set("_tunnel_host", py.String(host))
	if len(args) > 1 {
		c.Dict.Set("_tunnel_port", args[1])
	}
	return py.None, nil
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func init() {
	IncompleteReadType.Dict.Set("__str__", py.MustNewMethod("__str__", incompleteReadStr, 0,
		"IncompleteRead.__str__"))
	// partial and expected live in the exception's instance dict; a property
	// exposes them, because a plain *py.Exception does not surface its Dict as
	// attributes (the same reason email.errors registers .line as a property).
	IncompleteReadType.Dict.Set("partial", excDictProperty("partial"))
	IncompleteReadType.Dict.Set("expected", excDictProperty("expected"))
	BadStatusLineType.Dict.Set("line", excDictProperty("line"))

	msgMethods := []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"get", msgGet, "Return the value of the named header field."},
		{"get_all", msgGetAll, "Return a list of all the values of the named header field."},
		{"getheaders", msgGetHeaders, "Return a list of all the values of the named header field."},
		{"getheader", msgGet, "Return the value of the named header field."},
		{"items", msgItems, "Return a list of (name, value) header pairs."},
		{"keys", msgKeys, "Return a list of header names."},
		{"values", msgValues, "Return a list of header values."},
		{"add_header", msgAddHeader, "add_header(name, value, **params)"},
		{"__getitem__", msgGetItem, "Return the value of the named header field."},
		{"__contains__", msgContains, "True if the header is present."},
		{"__len__", msgLen, "Number of header fields."},
		{"__iter__", msgIter, "Iterate over the header names."},
		{"__repr__", msgRepr, "repr of the message headers."},
		{"__str__", msgRepr, "str of the message headers."},
	}
	for _, m := range msgMethods {
		HTTPMessageType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}

	// HTTPResponse attributes are read by urllib3 as plain attributes, so they
	// are exposed as read-only properties.
	respProps := []struct {
		name string
		get  func(py.Object) py.Object
	}{
		{"status", respStatus},
		{"reason", respReason},
		{"version", respVersion},
		{"msg", respMsg},
		{"headers", respHeaders},
		{"closed", respClosed},
	}
	for _, p := range respProps {
		get := p.get
		HTTPResponseType.Dict.Set(p.name, &py.Property{
			Fget: func(self py.Object) (py.Object, error) { return get(self), nil },
		})
	}
	respMethods := []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"getheader", respGetHeader, "Return the value of the named header field."},
		{"getheaders", respGetHeaders, "Return a list of all the values of the named header field."},
		{"read", respRead, "Read and return the response body."},
		{"readinto", respReadInto, "Read bytes into a pre-allocated bytearray."},
		{"close", respClose, "Close the response."},
		{"isclosed", respIsClosed, "Is the response closed?"},
	}
	for _, m := range respMethods {
		HTTPResponseType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}

	HTTPConnectionType.Dict.Set("__init__", py.MustNewMethod("__init__", connInit, 0,
		"HTTPConnection(host, port=None, timeout=..., source_address=None, blocksize=8192)"))
	HTTPConnectionType.Dict.Set("close", py.MustNewMethod("close", connClose, 0, "Close the connection."))
	HTTPConnectionType.Dict.Set("set_tunnel", py.MustNewMethod("set_tunnel", connSetTunnel, 0,
		"set_tunnel(host, port=None, headers=None)"))
	connMethods := []struct {
		name string
		doc  string
	}{
		{"connect", "Connect to the host.  Not implemented: no HTTP transport."},
		{"request", "Send a request.  Not implemented: no HTTP transport."},
		{"send", "Send data.  Not implemented: no HTTP transport."},
		{"putrequest", "Send a request line.  Not implemented: no HTTP transport."},
		{"putheader", "Send a header line.  Not implemented: no HTTP transport."},
		{"endheaders", "End the header block.  Not implemented: no HTTP transport."},
		{"getresponse", "Get the response.  Not implemented: no HTTP transport."},
	}
	for _, m := range connMethods {
		HTTPConnectionType.Dict.Set(m.name, py.MustNewMethod(m.name, connNotImplemented(m.name), 0, m.doc))
	}

	globals := py.NewStringDict()
	for name, t := range map[string]*py.Type{
		"HTTPException":           HTTPExceptionType,
		"NotConnected":            NotConnectedType,
		"InvalidURL":              InvalidURLType,
		"UnknownProtocol":         UnknownProtocolType,
		"UnknownTransferEncoding": UnknownTransferEncodingType,
		"UnimplementedFileMode":   UnimplementedFileModeType,
		"ImproperConnectionState": ImproperConnectionStateType,
		"CannotSendRequest":       CannotSendRequestType,
		"CannotSendHeader":        CannotSendHeaderType,
		"ResponseNotReady":        ResponseNotReadyType,
		"BadStatusLine":           BadStatusLineType,
		"LineTooLong":             LineTooLongType,
		"IncompleteRead":          IncompleteReadType,
		"HTTPMessage":             HTTPMessageType,
		"HTTPResponse":            HTTPResponseType,
		"HTTPConnection":          HTTPConnectionType,
		"HTTPSConnection":         HTTPSConnectionType,
	} {
		globals.Set(name, t)
	}
	globals.Set("_MAXLINE", py.Int(maxLine))
	globals.Set("responses", responsesDict())
	for name, code := range statusCodes {
		globals.Set(name, py.Int(code))
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "http.client",
			Doc:  client_doc,
		},
		Globals: globals,
	})
}

// excDictProperty exposes a value stored in an exception's instance Dict as a
// read-only attribute, defaulting to None.  A plain *py.Exception does not
// make its Dict visible as attributes, so this is how IncompleteRead.partial,
// IncompleteRead.expected and BadStatusLine.line are reachable from Python.
func excDictProperty(name string) *py.Property {
	return &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			if e, ok := self.(*py.Exception); ok {
				if v, ok := e.Dict.Get(name); ok {
					return v, nil
				}
			}
			return py.None, nil
		},
	}
}
