// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package email

import (
	"fmt"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const message_doc = `email.message - the Message class representing an email message.

A Message holds an ordered set of headers and a payload, and answers the
accessors the rest of the world uses: __getitem__, get, get_all, keys, items,
values, is_multipart, walk, get_payload, get_content_type and defects.`

// MessageType is the class of a parsed (or constructed) email message.
var MessageType *py.Type

// Message is the parsed representation of an RFC 5322 message.  Headers keep
// insertion order, and a repeated header keeps every value, which is what
// get_all and the "Received" handling require.
//
// It is a Go type rather than a python class because the interpreter's support
// for python classes deriving from a builtin, and for instance attributes on
// exception/base-exception classes, is incomplete; a Go type gives the whole
// Message interface the ordinary way.
type Message struct {
	headers []string // header names, in order
	values  []string // header values, parallel to headers
	payload py.Object
	policy  py.Object
	// defaultTyp is the content type used when no Content-Type header is set.
	defaultTyp string
	// preamble is the text before the first boundary of a multipart, exposed
	// as Message.preamble.
	preamble string
	// Dict carries attributes assigned from python, so "msg.foo = 1" works and
	// reads back.  "defects" lives here too: in CPython it is a plain list
	// attribute, not a method.
	Dict py.StringDict
}

func (m *Message) Type() *py.Type { return MessageType }

// GetDict exposes the instance dict, which is what makes attribute assignment
// and lookup behave as they do for a python object.
func (m *Message) GetDict() py.StringDict { return m.Dict }

// addDefect records a parse defect on the message.  CPython stores defects in
// the message's "defects" attribute (a plain list), which is where a caller
// reads them; keeping them there rather than in a Go field means an assignment
// from python to msg.defects is visible to both.
func (m *Message) addDefect(t *py.Type) {
	list, ok := m.Dict.GetOrNil("defects").(*py.List)
	if !ok {
		list = py.NewListFromItems(nil)
		m.Dict.Set("defects", list)
	}
	list.Items = append(list.Items, newDefect(t))
}

func (m *Message) defectsList() *py.List {
	if l, ok := m.Dict.GetOrNil("defects").(*py.List); ok {
		return l
	}
	l := py.NewListFromItems(nil)
	m.Dict.Set("defects", l)
	return l
}

// messageNew is Message().
func messageNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) > 0 {
		return nil, py.ExceptionNewf(py.TypeError, "Message() takes no positional arguments")
	}
	m := &Message{Dict: py.NewStringDict()}
	// CPython's Message.__init__ sets self.defects = [].
	m.Dict.Set("defects", py.NewListFromItems(nil))
	return m, nil
}

// indexOfHeader returns the position of the first header matching name, or -1.
func (m *Message) indexOfHeader(name string) int {
	for i, h := range m.headers {
		if strings.EqualFold(h, name) {
			return i
		}
	}
	return -1
}

func (m *Message) getRaw(name string) (string, bool) {
	if i := m.indexOfHeader(name); i >= 0 {
		return m.values[i], true
	}
	return "", false
}

// setRawHeader appends a header without parsing or folding it, which is what
// CPython's Message.set_raw does and what __setitem__ does: setting a header
// that already exists ADDS another one, it does not replace the first.  Use
// replace_header to overwrite in place.
func (m *Message) setRawHeader(name, value string) {
	m.headers = append(m.headers, name)
	m.values = append(m.values, value)
}

func (m *Message) contentMainType() string {
	ct, _ := m.contentType()
	if i := strings.Index(ct, "/"); i >= 0 {
		return ct[:i]
	}
	return ct
}

func (m *Message) contentSubType() string {
	ct, _ := m.contentType()
	if i := strings.Index(ct, "/"); i >= 0 {
		return ct[i+1:]
	}
	return ""
}

// contentType is get_content_type: the Content-Type with parameters dropped and
// the default type substituted when the header is absent.
func (m *Message) contentType() (string, error) {
	if v, ok := m.getRaw("Content-Type"); ok {
		main, _ := parseContentTypeValue(v)
		return main, nil
	}
	if m.defaultTyp != "" {
		return m.defaultTyp, nil
	}
	return "text/plain", nil
}

func (m *Message) getParam(name string) (string, bool) {
	ct, ok := m.getRaw("Content-Type")
	if !ok {
		return "", false
	}
	_, params := parseContentTypeValue(ct)
	for _, p := range params {
		if strings.EqualFold(p[0], name) {
			return p[1], true
		}
	}
	return "", false
}

// parseContentTypeValue splits a Content-Type value into its main type and its
// parameters, honouring quoted parameter values.
func parseContentTypeValue(v string) (string, [][2]string) {
	parts := splitSemicolons(v)
	if len(parts) == 0 {
		return "", nil
	}
	main := strings.ToLower(strings.TrimSpace(parts[0]))
	var params [][2]string
	for _, p := range parts[1:] {
		eq := strings.Index(p, "=")
		if eq < 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(p[:eq]))
		val := unquote(strings.TrimSpace(p[eq+1:]))
		params = append(params, [2]string{name, val})
	}
	return main, params
}

// splitSemicolons splits on ";" outside a quoted string.
func splitSemicolons(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote = !inQuote
			cur.WriteByte(c)
		case c == ';' && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(out, cur.String())
}

func (m *Message) isMultipart() bool {
	_, ok := m.payload.(*py.List)
	return ok
}

// walk visits this message and every subpart depth-first.
func (m *Message) walk() []py.Object {
	out := []py.Object{m}
	if subs, ok := m.payload.(*py.List); ok {
		for _, item := range subs.Items {
			if sub, ok := item.(*Message); ok {
				out = append(out, sub.walk()...)
			}
		}
	}
	return out
}

// headerString renders headers and payload the way Message.as_string does.
func (m *Message) headerString() string {
	var b strings.Builder
	for i, h := range m.headers {
		b.WriteString(h)
		b.WriteString(": ")
		b.WriteString(m.values[i])
		b.WriteString("\n")
	}
	if subs, ok := m.payload.(*py.List); ok && len(subs.Items) > 0 {
		boundary, _ := m.getParam("boundary")
		b.WriteString("This is a multi-part message in MIME format.\n\n")
		for _, item := range subs.Items {
			sub, ok := item.(*Message)
			if !ok {
				continue
			}
			b.WriteString("--" + boundary + "\n")
			b.WriteString(sub.headerString())
		}
		b.WriteString("--" + boundary + "--\n")
	} else if m.payload != nil {
		switch p := m.payload.(type) {
		case py.String:
			b.WriteString(string(p))
		case py.Bytes:
			b.WriteString(string(p))
		}
	}
	return b.String()
}

// getPayloadValue returns the payload as a python object.
func (m *Message) getPayloadValue() py.Object {
	if m.payload == nil {
		return py.String("")
	}
	return m.payload
}

// --- methods exposed to python -------------------------------------------

func msgGetitem(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__getitem__() takes exactly one argument")
	}
	m := self.(*Message)
	s, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	if v, ok := m.getRaw(s); ok {
		return py.String(v), nil
	}
	return py.None, nil
}

func msgSetitem(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 2 {
		return nil, py.ExceptionNewf(py.TypeError, "__setitem__() takes exactly two arguments")
	}
	m := self.(*Message)
	n, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	v, err := py.StrAsString(args[1])
	if err != nil {
		return nil, err
	}
	m.setRawHeader(n, v)
	return py.None, nil
}

func msgDelitem(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__delitem__() takes exactly one argument")
	}
	m := self.(*Message)
	n, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	if i := m.indexOfHeader(n); i >= 0 {
		m.headers = append(m.headers[:i], m.headers[i+1:]...)
		m.values = append(m.values[:i], m.values[i+1:]...)
	}
	return py.None, nil
}

func msgContains(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "__contains__() takes exactly one argument")
	}
	m := self.(*Message)
	s, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	return py.Bool(m.indexOfHeader(s) >= 0), nil
}

func msgLen(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return py.Int(len(self.(*Message).headers)), nil
}

func msgIter(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*Message)
	items := make([]py.Object, 0, len(m.headers))
	for _, h := range m.headers {
		items = append(items, py.String(h))
	}
	return py.NewListFromItems(items).M__iter__()
}

func msgGet(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		name    py.Object
		failobj py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:get", []string{"name", "failobj"}, &name, &failobj); err != nil {
		return nil, err
	}
	m := self.(*Message)
	s, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	if v, ok := m.getRaw(s); ok {
		return py.String(v), nil
	}
	return failobj, nil
}

func msgGetAll(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		name    py.Object
		failobj py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:get_all", []string{"name", "failobj"}, &name, &failobj); err != nil {
		return nil, err
	}
	m := self.(*Message)
	s, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	items := []py.Object{}
	for i, h := range m.headers {
		if strings.EqualFold(h, s) {
			items = append(items, py.String(m.values[i]))
		}
	}
	if len(items) == 0 {
		return failobj, nil
	}
	return py.NewListFromItems(items), nil
}

func msgKeys(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*Message)
	items := make([]py.Object, 0, len(m.headers))
	for _, h := range m.headers {
		items = append(items, py.String(h))
	}
	return py.NewListFromItems(items), nil
}

func msgValues(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*Message)
	items := make([]py.Object, 0, len(m.values))
	for _, v := range m.values {
		items = append(items, py.String(v))
	}
	return py.NewListFromItems(items), nil
}

func msgItems(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*Message)
	items := make([]py.Object, 0, len(m.headers))
	for i, h := range m.headers {
		items = append(items, py.Tuple{py.String(h), py.String(m.values[i])})
	}
	return py.NewListFromItems(items), nil
}

func msgIsMultipart(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return py.Bool(self.(*Message).isMultipart()), nil
}

func msgWalk(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return py.NewListFromItems(self.(*Message).walk()).M__iter__()
}

func msgGetPayload(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var i py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:get_payload", []string{"i"}, &i); err != nil {
		return nil, err
	}
	m := self.(*Message)
	if i != py.None {
		idx, err := py.MakeGoInt(i)
		if err != nil {
			return nil, err
		}
		subs, ok := m.payload.(*py.List)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "payload is not a list")
		}
		if idx < 0 || idx >= len(subs.Items) {
			return nil, py.ExceptionNewf(py.IndexError, "list index out of range")
		}
		return subs.Items[idx], nil
	}
	return m.getPayloadValue(), nil
}

func msgSetPayload(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var payload py.Object
	if err := py.UnpackTuple(args, kwargs, "set_payload", 1, 1, &payload); err != nil {
		return nil, err
	}
	self.(*Message).payload = payload
	return py.None, nil
}

func msgGetContentType(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	ct, err := self.(*Message).contentType()
	if err != nil {
		return nil, err
	}
	return py.String(ct), nil
}

func msgGetContentMaintype(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return py.String(self.(*Message).contentMainType()), nil
}

func msgGetContentSubtype(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return py.String(self.(*Message).contentSubType()), nil
}

func msgGetDefaultType(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*Message)
	if m.defaultTyp != "" {
		return py.String(m.defaultTyp), nil
	}
	return py.String("text/plain"), nil
}

func msgSetDefaultType(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var v py.Object
	if err := py.UnpackTuple(args, kwargs, "set_default_type", 1, 1, &v); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(v)
	if err != nil {
		return nil, err
	}
	self.(*Message).defaultTyp = s
	return py.None, nil
}

func msgGetParams(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	m := self.(*Message)
	ct, ok := m.getRaw("Content-Type")
	items := []py.Object{}
	if !ok {
		main, _ := m.contentType()
		items = append(items, py.Tuple{py.String(main), py.String("")})
		return py.NewListFromItems(items), nil
	}
	main, params := parseContentTypeValue(ct)
	items = append(items, py.Tuple{py.String(main), py.String("")})
	for _, p := range params {
		items = append(items, py.Tuple{py.String(p[0]), py.String(p[1])})
	}
	return py.NewListFromItems(items), nil
}

func msgGetParam(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		param   py.Object
		failobj py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:get_param",
		[]string{"param", "failobj"}, &param, &failobj); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(param)
	if err != nil {
		return nil, err
	}
	if v, ok := self.(*Message).getParam(s); ok {
		return py.String(v), nil
	}
	return failobj, nil
}

func msgGetBoundary(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if v, ok := self.(*Message).getParam("boundary"); ok {
		return py.String(v), nil
	}
	return py.None, nil
}

func msgSetBoundary(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var v py.Object
	if err := py.UnpackTuple(args, kwargs, "set_boundary", 1, 1, &v); err != nil {
		return nil, err
	}
	boundary, err := py.StrAsString(v)
	if err != nil {
		return nil, err
	}
	m := self.(*Message)
	if !m.isMultipart() {
		return nil, py.ExceptionNewf(py.ValueError, "Cannot set boundary for a singlepart message")
	}
	for _, bad := range []string{"\n", "\r"} {
		if strings.Contains(boundary, bad) {
			return nil, py.ExceptionNewf(py.ValueError, "Invalid boundary")
		}
	}
	m.setRawHeader("Content-Type", `multipart/mixed; boundary="`+boundary+`"`)
	return py.None, nil
}

func msgGetFilename(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		failobj py.Object = py.None
		unquote py.Object = py.True
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|$Op:get_filename",
		[]string{"failobj", "unquote"}, &failobj, &unquote); err != nil {
		return nil, err
	}
	m := self.(*Message)
	for _, header := range []string{"Content-Disposition", "Content-Type"} {
		v, ok := m.getRaw(header)
		if !ok {
			continue
		}
		_, params := parseContentTypeValue(v)
		for _, p := range params {
			if strings.EqualFold(p[0], "filename") {
				return py.String(p[1]), nil
			}
		}
	}
	return failobj, nil
}

func msgGetContentCharset(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		failobj py.Object = py.None
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:get_content_charset",
		[]string{"failobj"}, &failobj); err != nil {
		return nil, err
	}
	if v, ok := self.(*Message).getParam("charset"); ok {
		return py.String(strings.ToLower(v)), nil
	}
	return failobj, nil
}

func msgAsString(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var unixfrom py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|z:as_string", []string{"unixfrom"}, &unixfrom); err != nil {
		return nil, err
	}
	return py.String(self.(*Message).headerString()), nil
}

func msgAsBytes(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var unixfrom py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|z:as_bytes", []string{"unixfrom"}, &unixfrom); err != nil {
		return nil, err
	}
	return py.Bytes([]byte(self.(*Message).headerString())), nil
}

func msgDefects(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return self.(*Message).defectsList(), nil
}

func msgSetType(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var v py.Object
	if err := py.UnpackTuple(args, kwargs, "set_type", 1, 2, &v); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(v)
	if err != nil {
		return nil, err
	}
	m := self.(*Message)
	old, _ := m.getRaw("Content-Type")
	if old == "" {
		m.setRawHeader("Content-Type", s)
		return py.None, nil
	}
	_, params := parseContentTypeValue(old)
	if len(params) == 0 {
		m.setRawHeader("Content-Type", s)
		return py.None, nil
	}
	var parts []string
	parts = append(parts, s)
	for _, p := range params {
		parts = append(parts, p[0]+`="`+p[1]+`"`)
	}
	m.setRawHeader("Content-Type", strings.Join(parts, "; "))
	return py.None, nil
}

func msgAddHeader(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		name  py.Object
		value py.Object
	)
	if err := py.UnpackTuple(args, kwargs, "add_header", 2, 2, &name, &value); err != nil {
		return nil, err
	}
	n, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	v, err := py.StrAsString(value)
	if err != nil {
		return nil, err
	}
	m := self.(*Message)
	m.headers = append(m.headers, n)
	m.values = append(m.values, v)
	return py.None, nil
}

func msgReplaceHeader(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		name  py.Object
		value py.Object
	)
	if err := py.UnpackTuple(args, kwargs, "replace_header", 2, 2, &name, &value); err != nil {
		return nil, err
	}
	n, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	v, err := py.StrAsString(value)
	if err != nil {
		return nil, err
	}
	m := self.(*Message)
	i := m.indexOfHeader(n)
	if i < 0 {
		return nil, py.ExceptionNewf(py.KeyError, "%s", n)
	}
	m.headers[i] = n
	m.values[i] = v
	return py.None, nil
}

func msgAttach(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var sub py.Object
	if err := py.UnpackTuple(args, kwargs, "attach", 1, 1, &sub); err != nil {
		return nil, err
	}
	child, ok := sub.(*Message)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "attach() argument must be a Message")
	}
	m := self.(*Message)
	l, ok := m.payload.(*py.List)
	if !ok && m.payload != nil {
		if s, isStr := m.payload.(py.String); isStr && s != "" {
			return nil, py.ExceptionNewf(py.TypeError, "Attach() called after set_payload()")
		}
	}
	if !ok {
		l = py.NewListFromItems(nil)
		m.payload = l
	}
	l.Items = append(l.Items, child)
	return py.None, nil
}

func msgSetRaw(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		name  py.Object
		value py.Object
	)
	if err := py.UnpackTuple(args, kwargs, "set_raw", 2, 2, &name, &value); err != nil {
		return nil, err
	}
	n, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	v, err := py.StrAsString(value)
	if err != nil {
		return nil, err
	}
	self.(*Message).setRawHeader(n, v)
	return py.None, nil
}

func msgStr(self py.Object, args py.Tuple) (py.Object, error) {
	return py.String(self.(*Message).headerString()), nil
}

func msgRepr(self py.Object, args py.Tuple) (py.Object, error) {
	m := self.(*Message)
	return py.String(fmt.Sprintf("<%s object at %p\n%s>", MessageType.Name, m, m.headerString())), nil
}

func init() {
	MessageType = py.NewTypeX("email.message.Message", message_doc, messageNew, nil)

	globals := py.NewStringDict()
	globals.Set("Message", MessageType)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "email.message",
			Doc:  message_doc,
		},
		Globals: globals,
	})

	register := func(name string, fn interface{}, doc string) {
		MessageType.Dict.Set(name, py.MustNewMethod(name, fn, 0, doc))
	}

	register("__getitem__", msgGetitem, "Return the first header matching name, or None.")
	register("__setitem__", msgSetitem, "Set a header value.")
	register("__delitem__", msgDelitem, "Delete all occurrences of the named header.")
	register("__contains__", msgContains, "Return True if the named header is present.")
	register("__len__", msgLen, "Return the number of headers.")
	register("__iter__", msgIter, "Iterate over the header names.")
	register("__str__", msgStr, "Return the entire message flattened as a string.")
	register("__repr__", msgRepr, "Return a representation of the message.")
	register("get", msgGet, "Return the value of the named header field, or failobj (default None).")
	register("get_all", msgGetAll, "Return a list of all the values for the named header field.")
	register("keys", msgKeys, "Return a list of all header names.")
	register("values", msgValues, "Return a list of all header values.")
	register("items", msgItems, "Return a list of (name, value) header pairs.")
	register("is_multipart", msgIsMultipart, "Return True if the payload is a list of sub-Message objects.")
	register("walk", msgWalk, "Iterate over all the parts and subparts depth-first.")
	register("get_payload", msgGetPayload, "Return the message payload, or the i'th subpart when i is given.")
	register("set_payload", msgSetPayload, "Set the payload.")
	register("get_content_type", msgGetContentType, "Return the message's content type, lowercased.")
	register("get_content_maintype", msgGetContentMaintype, "Return the main part of the content type.")
	register("get_content_subtype", msgGetContentSubtype, "Return the subtype of the content type.")
	register("get_default_type", msgGetDefaultType, "Return the default content type.")
	register("set_default_type", msgSetDefaultType, "Set the default content type.")
	register("get_params", msgGetParams, "Return the Content-Type parameters.")
	register("get_param", msgGetParam, "Return a Content-Type parameter value.")
	register("get_boundary", msgGetBoundary, "Return the boundary parameter of a multipart, or None.")
	register("set_boundary", msgSetBoundary, "Set the boundary parameter.")
	register("get_filename", msgGetFilename, "Return the filename parameter.")
	register("get_content_charset", msgGetContentCharset, "Return the charset parameter.")
	register("as_string", msgAsString, "Return the entire message flattened as a string.")
	register("as_bytes", msgAsBytes, "Return the entire message flattened as bytes.")
	register("set_type", msgSetType, "Set the Content-Type after removing its parameters.")
	register("add_header", msgAddHeader, "Append a header.")
	register("replace_header", msgReplaceHeader, "Replace the first occurrence of a header.")
	register("attach", msgAttach, "Add a subpart to a multipart message.")
	register("set_raw", msgSetRaw, "Set a header without any parsing or folding.")
}
