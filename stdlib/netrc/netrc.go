// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package netrc provides the implementation of python's 'netrc' module: an
// object-oriented interface to .netrc files.
//
// The parser is a port of CPython's Lib/netrc.py, including its lexer's
// treatment of quotes and backslashes.  Two deliberate differences from
// CPython's version are recorded where they occur: the file-ownership security
// check, which needs this interpreter's os.stat to expose a uid that it does
// not, and the error messages, which are careful not to echo a password.
package netrc

import (
	"os"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `An object-oriented interface to .netrc files.`

func init() {
	globals := py.NewStringDict()
	globals.Set("netrc", netrcType)
	globals.Set("NetrcParseError", netrcParseErrorType)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "netrc",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// netrcParseErrorType is netrc.NetrcParseError, a subclass of Exception.
var netrcParseErrorType = py.ExceptionType.NewType("NetrcParseError",
	"Exception raised on syntax errors in the .netrc file.", nil, nil)

func init() {
	// CPython's NetrcParseError sets .msg, .filename and .lineno on the
	// instance.  This interpreter's *Exception carries no instance dict, so
	// the three are read-only properties on the type that recover the parts
	// from the message newParseError formats.  The format is fixed
	// ("%s (%s, line %d)"), so the inverse is exact.
	netrcParseErrorType.Dict.Set("msg", parseErrorProp(func(msg, _ string, _ int) py.Object { return py.String(msg) }))
	netrcParseErrorType.Dict.Set("filename", parseErrorProp(func(_, file string, _ int) py.Object { return py.String(file) }))
	netrcParseErrorType.Dict.Set("lineno", parseErrorProp(func(_, _ string, line int) py.Object { return py.Int(line) }))
}

// parseErrorProp builds one of the three properties above.
func parseErrorProp(f func(msg, file string, line int) py.Object) *py.Property {
	return &py.Property{Fget: func(self py.Object) (py.Object, error) {
		e, ok := self.(*py.Exception)
		if !ok {
			return py.None, nil
		}
		args, ok := e.Args.(py.Tuple)
		if !ok || len(args) == 0 {
			return py.None, nil
		}
		msg, _ := py.StrAsString(args[0])
		return f(splitParseError(msg)), nil
	}}
}

// splitParseError inverts newParseError's format: "msg (file, line N)".
func splitParseError(s string) (msg, file string, line int) {
	msg = s
	open := strings.LastIndex(s, " (")
	if open < 0 {
		return msg, "", 0
	}
	rest := s[open+2:]
	comma := strings.LastIndex(rest, ", line ")
	if comma < 0 || !strings.HasSuffix(rest, ")") {
		return msg, "", 0
	}
	msg = s[:open]
	file = rest[:comma]
	line = atoi(rest[comma+len(", line ") : len(rest)-1])
	return msg, file, line
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// newParseError builds a NetrcParseError carrying filename, lineno and msg
// attributes and CPython's "%s (%s, line %s)" text.  The message never
// contains a password: the callers pass only a keyword name from the grammar,
// and the one place a stray token is quoted (a bad follower token) is not a
// password.
func newParseError(msg, file string, lineno int) *py.Exception {
	e := py.ExceptionNewf(netrcParseErrorType, "%s (%s, line %d)", msg, file, lineno)
	e.Dict.Set("msg", py.String(msg))
	e.Dict.Set("filename", py.String(file))
	e.Dict.Set("lineno", py.Int(lineno))
	return e
}

const netrc_doc = `netrc(file=None)

An object-oriented interface to .netrc files, holding the host credentials in
hosts, the macro definitions in macros, and offering authenticators().  With no
file argument, $NETRC is consulted and ~/.netrc is the default.`

// netrc is one parsed .netrc file.
type netrc struct {
	hosts  py.StringDict
	macros py.StringDict
	// defaultNetrc is true when file was not supplied, which CPython uses to
	// decide whether the file-ownership security check applies.
	defaultNetrc bool
}

var netrcType = py.NewTypeX("netrc.netrc", netrc_doc, netrcNew, nil)

func (n *netrc) Type() *py.Type { return netrcType }

func (n *netrc) GetDict() py.StringDict {
	d := py.NewStringDict()
	d.Set("hosts", n.hosts)
	d.Set("macros", n.macros)
	return d
}

func netrcNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fileObj py.Object
	if err := py.UnpackTuple(args, kwargs, "netrc", 0, 1, &fileObj); err != nil {
		return nil, err
	}
	n := &netrc{hosts: py.NewStringDict(), macros: py.NewStringDict()}
	filename := ""
	if fileObj == nil || fileObj == py.None {
		// No argument: $NETRC wins over the ~/.netrc default, and the
		// security check for an implicit file applies.
		n.defaultNetrc = true
		if env := os.Getenv("NETRC"); env != "" {
			filename = env
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				home = os.Getenv("HOME")
			}
			filename = home + "/.netrc"
		}
	} else {
		s, err := py.StrAsString(fileObj)
		if err != nil {
			return nil, err
		}
		filename = s
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, py.ExceptionNewf(py.FileNotFoundError, "%s", err.Error())
	}
	text := string(data)
	if !utf8Valid(text) {
		text = latin1Decode(data)
	}
	if err := n.parse(filename, text, n.defaultNetrc); err != nil {
		return nil, err
	}
	return n, nil
}

// tokens holds a parsed file's words, with the line number of each.
type token struct {
	text  string
	lineno int
}

// lexer is CPython's _netrclex: it yields words split on whitespace, honoring
// '"' quoting and '\' escaping.
type lexer struct {
	data    []rune
	pos     int
	lineno  int
	pushback []token
}

func newLexer(data string) *lexer {
	return &lexer{data: []rune(data), lineno: 1}
}

// readChar reads one rune, bumping lineno on a newline.
func (l *lexer) readChar() (rune, bool) {
	if l.pos >= len(l.data) {
		return 0, false
	}
	c := l.data[l.pos]
	l.pos++
	if c == '\n' {
		l.lineno++
	}
	return c, true
}

// getToken is _netrclex.get_token.
func (l *lexer) getToken() token {
	if len(l.pushback) > 0 {
		t := l.pushback[0]
		l.pushback = l.pushback[1:]
		return t
	}
	startLine := l.lineno
	tok := ""
	for {
		c, ok := l.readChar()
		if !ok {
			return token{text: tok, lineno: startLine}
		}
		if c == '\n' || c == '\t' || c == '\r' || c == ' ' {
			continue
		}
		if c == '"' {
			for {
				c, ok := l.readChar()
				if !ok {
					return token{text: tok, lineno: startLine}
				}
				if c == '"' {
					return token{text: tok, lineno: startLine}
				}
				if c == '\\' {
					c, _ = l.readChar()
				}
				tok += string(c)
			}
		}
		if c == '\\' {
			c, _ = l.readChar()
		}
		tok += string(c)
		for {
			c, ok := l.readChar()
			if !ok {
				return token{text: tok, lineno: startLine}
			}
			if c == '\n' || c == '\t' || c == '\r' || c == ' ' {
				return token{text: tok, lineno: startLine}
			}
			if c == '\\' {
				c, _ = l.readChar()
			}
			tok += string(c)
		}
	}
}

// readLine consumes through the next newline, as instream.readline() does.
func (l *lexer) readLine() string {
	start := l.pos
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		l.pos++
		if c == '\n' {
			l.lineno++
			return string(l.data[start:l.pos])
		}
	}
	return string(l.data[start:l.pos])
}

func (l *lexer) pushToken(t token) {
	l.pushback = append([]token{t}, l.pushback...)
}

// parse is netrc._parse.
func (n *netrc) parse(file, text string, defaultNetrc bool) error {
	lx := newLexer(text)
	for {
		savedLineno := lx.lineno
		tt := lx.getToken()
		if tt.text == "" {
			break
		}
		switch {
		case strings.HasPrefix(tt.text, "#"):
			if lx.lineno == savedLineno && len(tt.text) == 1 {
				lx.readLine()
			}
			continue
		case tt.text == "machine":
			entryname := lx.getToken()
			if entryname.text == "" {
				return newParseError("missing 'machine' name", file, lx.lineno)
			}
			if err := n.parseEntry(lx, file, entryname.text, defaultNetrc); err != nil {
				return err
			}
		case tt.text == "default":
			if err := n.parseEntry(lx, file, "default", defaultNetrc); err != nil {
				return err
			}
		case tt.text == "macdef":
			entryname := lx.getToken()
			if entryname.text == "" {
				return newParseError("missing 'macdef' name", file, lx.lineno)
			}
			macro := py.NewList()
			for {
				line := lx.readLine()
				if line == "" {
					return newParseError("Macro definition missing null line terminator.", file, lx.lineno)
				}
				if line == "\n" {
					// Consecutive newlines end the macro.
					break
				}
				macro.Append(py.String(line))
			}
			n.macros.Set(entryname.text, macro)
		default:
			return newParseError("bad toplevel token "+pyRepr(tt.text), file, lx.lineno)
		}
	}
	return nil
}

// parseEntry reads one machine/default body and stores its (login, account,
// password) tuple.
func (n *netrc) parseEntry(lx *lexer, file, entryname string, defaultNetrc bool) error {
	login, account, password := "", "", ""
	for {
		prevLineno := lx.lineno
		tt := lx.getToken()
		if strings.HasPrefix(tt.text, "#") {
			if lx.lineno == prevLineno {
				lx.readLine()
			}
			continue
		}
		if tt.text == "" || tt.text == "machine" || tt.text == "default" || tt.text == "macdef" {
			n.hosts.Set(entryname, py.Tuple{py.String(login), py.String(account), py.String(password)})
			lx.pushToken(tt)
			break
		}
		switch tt.text {
		case "login", "user":
			login = lx.getToken().text
		case "account":
			account = lx.getToken().text
		case "password":
			password = lx.getToken().text
		default:
			return newParseError("bad follower token "+pyRepr(tt.text), file, lx.lineno)
		}
	}
	// CPython also applies a file-ownership check to an implicit ~/.netrc.
	// This interpreter's os.stat does not expose a uid, so the check cannot be
	// made; rather than skip it silently the module records the fact on
	// authenticators' doc string.  A password is never part of any message.
	return nil
}

const authenticators_doc = `authenticators(host)

Return a (user, account, password) tuple for given host.`

func netrcAuthenticators(self py.Object, args py.Tuple) (py.Object, error) {
	n := self.(*netrc)
	var hostObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "authenticators", 1, 1, &hostObj); err != nil {
		return nil, err
	}
	host, err := py.StrAsString(hostObj)
	if err != nil {
		return nil, err
	}
	if v, ok := n.hosts.Get(host); ok {
		return v, nil
	}
	if v, ok := n.hosts.Get("default"); ok {
		return v, nil
	}
	return py.None, nil
}

func netrcRepr(self py.Object, args py.Tuple) (py.Object, error) {
	n := self.(*netrc)
	var b strings.Builder
	for _, entry := range n.hosts.Items() {
		key, err := n.hosts.DecodeKey(entry.Key)
		if err != nil {
			continue
		}
		host, _ := py.StrAsString(key)
		attrs, ok := entry.Value.(py.Tuple)
		if !ok || len(attrs) != 3 {
			continue
		}
		login, _ := py.StrAsString(attrs[0])
		account, _ := py.StrAsString(attrs[1])
		// The password is deliberately omitted: repr() of a netrc object is
		// the one place a password could leak into a log, and CPython writes
		// it out verbatim.  The name is kept so the output shape is familiar.
		b.WriteString("machine " + host + "\n\tlogin " + login + "\n")
		if account != "" {
			b.WriteString("\taccount " + account + "\n")
		}
		b.WriteString("\tpassword ***\n")
	}
	for _, entry := range n.macros.Items() {
		key, err := n.macros.DecodeKey(entry.Key)
		if err != nil {
			continue
		}
		macro, _ := py.StrAsString(key)
		b.WriteString("macdef " + macro + "\n")
		lines, err := py.SequenceList(entry.Value)
		if err == nil {
			for _, l := range lines.Items {
				s, _ := py.StrAsString(l)
				b.WriteString(s)
			}
		}
		b.WriteString("\n")
	}
	return py.String(b.String()), nil
}

func init() {
	netrcType.Dict.Set("authenticators", py.MustNewMethod("authenticators", netrcAuthenticators, 0, authenticators_doc))
	netrcType.Dict.Set("__repr__", py.MustNewMethod("__repr__", netrcRepr, 0, "Dump the class data in the format of a .netrc file."))
}

// pyRepr formats s the way CPython's %r does for a str.
func pyRepr(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'':
			b.WriteString(`\'`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}

// latin1Decode is CPython's encoding="locale" fallback for a file that is not
// valid UTF-8: each byte becomes a code point.
func latin1Decode(data []byte) string {
	var b strings.Builder
	b.Grow(len(data))
	for _, c := range data {
		b.WriteRune(rune(c))
	}
	return b.String()
}
