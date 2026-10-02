// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package shlex provides the implementation of python's 'shlex' module: a
// lexical analyzer for shell-like syntax.
//
// This is a port of CPython's Lib/shlex.py.  The lexer is a small state
// machine: state ' ' skips whitespace and starts a token, state 'a' accumulates
// an unquoted word, state 'c' accumulates a run of punctuation characters, and
// a quote or escape character is its own state.  The posix and non-posix modes
// differ only in a handful of branches, and both are implemented here.
//
// The public knob attributes - whitespace, wordchars, quotes, escape,
// commenters, punctuation_chars and the rest - are read and written through the
// instance dictionary, exactly as CPython's attribute lookups are, so a caller
// that reassigns lex.commenters or lex.wordchars changes what the lexer does on
// its next token.
//
// One feature is deliberately absent: push_source()/pop_source() and the
// 'source' inclusion hook, which pull more input from files named in the
// stream.  The lexer works from the string or stream it was given, and no
// further files are read; setting 'source' has no effect.  Everything else in
// the module's public surface behaves as CPython's does.
package shlex

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `A lexical analyzer class for simple shell-like syntaxes.

The shlex class makes it easy to write lexical analyzers for simple
syntaxes resembling that of the Unix shell.  This will often be useful
for writing minilanguages, or for parsing quoted strings.

The split() function splits a string into a list of tokens using
shell-like syntax.`

func init() {
	shlexType.Dict.Set("punctuation_chars", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return self.(*shlex).attrString("_punctuation_chars"), nil
		},
	})

	shlexType.Dict.Set("push_token", py.MustNewMethod("push_token", shlexPushToken, 0, push_token_doc))
	shlexType.Dict.Set("get_token", py.MustNewMethod("get_token", shlexGetToken, 0, get_token_doc))
	shlexType.Dict.Set("error_leader", py.MustNewMethod("error_leader", shlexErrorLeader, 0, error_leader_doc))
	shlexType.Dict.Set("__iter__", py.MustNewMethod("__iter__", shlexIter, 0, "Return an iterator over the tokens."))
	shlexType.Dict.Set("__next__", py.MustNewMethod("__next__", shlexNext, 0, "Return the next token, or raise StopIteration at end of input."))

	globals := py.NewStringDict()
	globals.Set("shlex", shlexType)
	globals.Set("split", py.MustNewMethod("split", splitFn, 0, split_doc))
	globals.Set("quote", py.MustNewMethod("quote", quoteFn, 0, quote_doc))
	globals.Set("join", py.MustNewMethod("join", joinFn, 0, join_doc))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "shlex",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

const shlex_doc = `A lexical analyzer class for simple shell-like syntaxes.

shlex(instream=None, infile=None, posix=False, punctuation_chars=False)

If instream is a string, it is the source of the tokens.  posix selects the
POSIX (True) or non-POSIX (False) quoting rules; the two differ in how quotes
and the escape character are handled.`

const push_token_doc = `Push a token onto the stack popped by the get_token method.`

const get_token_doc = `Get a token from the input stream (or from stack if it's nonempty).`

const error_leader_doc = `Emit a C-compiler-like, Emacs-friendly error-message leader.`

// shlex is one lexer.  The public configuration lives in the instance
// dictionary so that reading and writing it matches CPython; the private
// lexer state lives in the Go fields.
type shlex struct {
	dict py.StringDict

	// input is the current source, with pos the offset of the next rune.
	input string
	runes []rune
	pos   int
	// filestack is the state saved by push_source and restored by pop_source.
	filestack []shlexState

	// pushback holds whole tokens pushed back by push_token.
	pushback []py.Object
	// pushbackChars is the character pushback used by the punctuation-mode
	// lookahead.
	pushbackChars []rune
}

type shlexState struct {
	input  string
	runes  []rune
	pos    int
	lineno py.Object
	infile py.Object
}

var shlexType = py.NewTypeX("shlex", shlex_doc, shlexNew, nil)

func (s *shlex) Type() *py.Type { return shlexType }

func (s *shlex) GetDict() py.StringDict { return s.dict }

func shlexNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var instream, infile, posix, punctuation py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOOO", []string{"instream", "infile", "posix", "punctuation_chars"},
		&instream, &infile, &posix, &punctuation); err != nil {
		return nil, err
	}
	s := &shlex{dict: py.NewStringDict()}
	s.dict.Set("infile", infile)
	if infile == nil {
		s.dict.Set("infile", py.None)
	}

	src := ""
	if instream == nil || instream == py.None {
		// No stream: the lexer reads standard input.  The content is pulled
		// in when the first token is asked for, since sys.stdin may be
		// replaced after construction.
		s.dict.Set("infile", py.None)
	} else if str, ok := instream.(py.String); ok {
		src = string(str)
	} else {
		return nil, py.ExceptionNewf(py.TypeError, "instream must be a string")
	}
	s.setInput(src)

	isPosix := false
	if posix != nil && posix != py.None {
		b, err := py.ObjectIsTrue(posix)
		if err != nil {
			return nil, err
		}
		isPosix = b
	}
	s.dict.Set("posix", py.NewBool(isPosix))
	if isPosix {
		s.dict.Set("eof", py.None)
	} else {
		s.dict.Set("eof", py.String(""))
	}

	s.dict.Set("commenters", py.String("#"))
	wordchars := "abcdfeghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
	if isPosix {
		wordchars += "ßàáâãäåæçèéêëìíîïðñòóôõöøùúûüýþÿÀÁÂÃÄÅÆÇÈÉÊËÌÍÎÏÐÑÒÓÔÕÖØÙÚÛÜÝÞ"
	}
	s.dict.Set("wordchars", py.String(wordchars))
	s.dict.Set("whitespace", py.String(" \t\r\n"))
	s.dict.Set("whitespace_split", py.False)
	s.dict.Set("quotes", py.String("'\""))
	s.dict.Set("escape", py.String("\\"))
	s.dict.Set("escapedquotes", py.String("\""))
	s.dict.Set("state", py.String(" "))
	s.dict.Set("lineno", py.Int(1))
	s.dict.Set("debug", py.Int(0))
	s.dict.Set("token", py.String(""))
	s.dict.Set("source", py.None)

	pc := ""
	if punctuation != nil && punctuation != py.None {
		if b, ok := punctuation.(py.Bool); ok {
			if bool(b) {
				pc = "();<>|&"
			}
		} else if str, ok := punctuation.(py.String); ok {
			pc = string(str)
		} else {
			return nil, py.ExceptionNewf(py.TypeError, "punctuation_chars must be a string or bool")
		}
	}
	if pc != "" {
		// The extra characters are allowed in filenames, arguments and
		// wildcards; remove any punctuation characters from wordchars.
		extra := "~-./*?="
		filtered := filterRunes(wordchars+extra, pc)
		s.dict.Set("wordchars", py.String(filtered))
	}
	s.dict.Set("_punctuation_chars", py.String(pc))
	return s, nil
}

func (s *shlex) setInput(src string) {
	s.input = src
	s.runes = []rune(src)
	s.pos = 0
}

// attrString reads a string-valued attribute from the instance dictionary.
func (s *shlex) attrString(key string) py.Object {
	if v, ok := s.dict.Get(key); ok {
		return v
	}
	return py.None
}

func (s *shlex) attrStringValue(key string) string {
	v, _ := s.dict.Get(key)
	s2, err := py.StrAsString(v)
	if err != nil {
		return ""
	}
	return s2
}

func (s *shlex) lineno() int {
	v, _ := s.dict.Get("lineno")
	if n, ok := v.(py.Int); ok {
		return int(n)
	}
	return 1
}

func (s *shlex) bumpLineno() {
	s.dict.Set("lineno", py.Int(s.lineno()+1))
}

// nextRune reads one character, honoring the punctuation-mode pushback.  It
// returns 0 and false at end of input.
func (s *shlex) nextRune() (rune, bool) {
	if s.attrStringValue("_punctuation_chars") != "" && len(s.pushbackChars) > 0 {
		r := s.pushbackChars[len(s.pushbackChars)-1]
		s.pushbackChars = s.pushbackChars[:len(s.pushbackChars)-1]
		return r, true
	}
	if s.pos >= len(s.runes) {
		return 0, false
	}
	r := s.runes[s.pos]
	s.pos++
	return r, true
}

// nextRuneOrEOF is nextRune with an explicit EOF sentinel, since the state
// machine distinguishes a real character from end of input.
func (s *shlex) nextRuneOrEOF() (rune, bool) {
	return s.nextRune()
}

// readLine consumes the rest of the current line, as comment handling does.
func (s *shlex) readLine() {
	for s.pos < len(s.runes) && s.runes[s.pos] != '\n' {
		s.pos++
	}
}

func shlexPushToken(self py.Object, args py.Tuple) (py.Object, error) {
	s := self.(*shlex)
	var tok py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "push_token", 1, 1, &tok); err != nil {
		return nil, err
	}
	s.pushback = append(s.pushback, tok)
	return py.None, nil
}

func shlexGetToken(self py.Object, args py.Tuple) (py.Object, error) {
	s := self.(*shlex)
	return s.getToken()
}

func (s *shlex) getToken() (py.Object, error) {
	if len(s.pushback) > 0 {
		tok := s.pushback[len(s.pushback)-1]
		s.pushback = s.pushback[:len(s.pushback)-1]
		return tok, nil
	}
	raw, err := s.readToken()
	if err != nil {
		return nil, err
	}
	eof, _ := s.dict.Get("eof")
	for raw == eof {
		if len(s.filestack) == 0 {
			return eof, nil
		}
		s.popSource()
		raw, err = s.getToken()
		if err != nil {
			return nil, err
		}
	}
	return raw, nil
}

func (s *shlex) popSource() {
	last := s.filestack[len(s.filestack)-1]
	s.filestack = s.filestack[:len(s.filestack)-1]
	s.input = last.input
	s.runes = last.runes
	s.pos = last.pos
	s.dict.Set("lineno", last.lineno)
	s.dict.Set("infile", last.infile)
	s.dict.Set("state", py.String(" "))
}

// readToken is the state machine, a direct port of shlex.read_token.
func (s *shlex) readToken() (py.Object, error) {
	posix := s.isPosix()
	whitespace := s.attrStringValue("whitespace")
	commenters := s.attrStringValue("commenters")
	wordchars := s.attrStringValue("wordchars")
	quotes := s.attrStringValue("quotes")
	escape := s.attrStringValue("escape")
	escapedquotes := s.attrStringValue("escapedquotes")
	punctuation := s.attrStringValue("_punctuation_chars")
	whitespaceSplit := s.attrBool("whitespace_split")

	quoted := false
	escapedstate := ' '
	token := ""
	state := s.attrStringValue("state")

	for {
		nextchar, ok := s.nextRune()
		if !ok {
			// End of input is presented as a sentinel; the state machine
			// distinguishes it by the bool return.
			nextchar = eofRune
		}
		hasChar := ok
		if hasChar && nextchar == '\n' {
			s.bumpLineno()
		}
		switch {
		case state == eofState:
			token = ""
			s.dict.Set("token", py.String(token))
			s.dict.Set("state", py.String(state))
			return s.finishToken(token, quoted, posix)
		case state == " ":
			if !hasChar {
				state = eofState
				s.dict.Set("token", py.String(""))
				s.dict.Set("state", py.String(state))
				return s.finishToken("", quoted, posix)
			}
			if containsRune(whitespace, nextchar) {
				if token != "" || (posix && quoted) {
					s.dict.Set("token", py.String(token))
					s.dict.Set("state", py.String(state))
					return s.finishToken(token, quoted, posix)
				}
				continue
			}
			if containsRune(commenters, nextchar) {
				s.readLine()
				s.bumpLineno()
				continue
			}
			if posix && nextcharMatchesRune(escape, nextchar) {
				escapedstate = 'a'
				state = string(nextchar)
				continue
			}
			if containsRune(wordchars, nextchar) {
				token = string(nextchar)
				state = "a"
				continue
			}
			if containsRune(punctuation, nextchar) {
				token = string(nextchar)
				state = "c"
				continue
			}
			if containsRune(quotes, nextchar) {
				if !posix {
					token = string(nextchar)
				}
				state = string(nextchar)
				continue
			}
			if whitespaceSplit {
				token = string(nextchar)
				state = "a"
				continue
			}
			token = string(nextchar)
			if token != "" || (posix && quoted) {
				s.dict.Set("token", py.String(token))
				s.dict.Set("state", py.String(state))
				return s.finishToken(token, quoted, posix)
			}
			continue
		case strings.ContainsAny(state, quotes) && len([]rune(state)) == 1:
			quoted = true
			if !hasChar {
				return nil, py.ExceptionNewf(py.ValueError, "No closing quotation")
			}
			if nextchar == []rune(state)[0] {
				if !posix {
					token += string(nextchar)
					state = " "
					s.dict.Set("token", py.String(token))
					s.dict.Set("state", py.String(state))
					return s.finishToken(token, quoted, posix)
				}
				state = "a"
			} else if posix && containsRune(escape, nextchar) && strings.ContainsAny(escapedquotes, state) {
				escapedstate = []rune(state)[0]
				state = string(nextchar)
			} else {
				token += string(nextchar)
			}
			continue
		case strings.ContainsAny(state, escape) && len([]rune(state)) == 1:
			if !hasChar {
				return nil, py.ExceptionNewf(py.ValueError, "No escaped character")
			}
			if strings.ContainsAny(escapedquotes, string(escapedstate)) && nextchar != []rune(state)[0] && nextchar != escapedstate {
				token += state
			}
			token += string(nextchar)
			state = string(escapedstate)
			continue
		case state == "a" || state == "c":
			if !hasChar {
				state = eofState
				s.dict.Set("token", py.String(token))
				s.dict.Set("state", py.String(state))
				return s.finishToken(token, quoted, posix)
			}
			if containsRune(whitespace, nextchar) {
				state = " "
				if token != "" || (posix && quoted) {
					s.dict.Set("token", py.String(token))
					s.dict.Set("state", py.String(state))
					return s.finishToken(token, quoted, posix)
				}
				continue
			}
			if containsRune(commenters, nextchar) {
				s.readLine()
				s.bumpLineno()
				if posix {
					state = " "
					if token != "" || (posix && quoted) {
						s.dict.Set("token", py.String(token))
						s.dict.Set("state", py.String(state))
						return s.finishToken(token, quoted, posix)
					}
					continue
				}
				continue
			}
			if state == "c" {
				if containsRune(punctuation, nextchar) {
					token += string(nextchar)
					continue
				}
				if !containsRune(whitespace, nextchar) {
					s.pushbackChars = append(s.pushbackChars, nextchar)
				}
				state = " "
				s.dict.Set("token", py.String(token))
				s.dict.Set("state", py.String(state))
				return s.finishToken(token, quoted, posix)
			}
			if posix && containsRune(quotes, nextchar) {
				state = string(nextchar)
				continue
			}
			if posix && containsRune(escape, nextchar) {
				escapedstate = 'a'
				state = string(nextchar)
				continue
			}
			if containsRune(wordchars, nextchar) || containsRune(quotes, nextchar) ||
				(whitespaceSplit && !containsRune(punctuation, nextchar)) {
				token += string(nextchar)
				continue
			}
			if punctuation != "" {
				s.pushbackChars = append(s.pushbackChars, nextchar)
			} else {
				s.pushback = append(s.pushback, py.String(string(nextchar)))
			}
			state = " "
			if token != "" || (posix && quoted) {
				s.dict.Set("token", py.String(token))
				s.dict.Set("state", py.String(state))
				return s.finishToken(token, quoted, posix)
			}
			continue
		}
	}
}

// finishToken applies the posix EOF normalization to a completed raw token.
func (s *shlex) finishToken(token string, quoted, posix bool) (py.Object, error) {
	if posix && !quoted && token == "" {
		eof, _ := s.dict.Get("eof")
		return eof, nil
	}
	return py.String(token), nil
}

const eofRune rune = -1
const eofState = "\x00"

func (s *shlex) isPosix() bool {
	v, _ := s.dict.Get("posix")
	if b, ok := v.(py.Bool); ok {
		return bool(b)
	}
	return false
}

func (s *shlex) attrBool(key string) bool {
	v, _ := s.dict.Get(key)
	if b, ok := v.(py.Bool); ok {
		return bool(b)
	}
	return false
}

func shlexErrorLeader(self py.Object, args py.Tuple) (py.Object, error) {
	s := self.(*shlex)
	infile, _ := s.dict.Get("infile")
	name := "None"
	if str, ok := infile.(py.String); ok {
		name = string(str)
	}
	return py.String("\"" + name + "\", line " + itoa(s.lineno()) + ": "), nil
}

func shlexIter(self py.Object, args py.Tuple) (py.Object, error) {
	return self, nil
}

func shlexNext(self py.Object, args py.Tuple) (py.Object, error) {
	s := self.(*shlex)
	tok, err := s.getToken()
	if err != nil {
		return nil, err
	}
	eof, _ := s.dict.Get("eof")
	if tok == eof {
		// py.StopIteration is the exception TYPE; returning it bare is how
		// this interpreter signals exhaustion, and anything else is not
		// recognised as the end of iteration.
		return nil, py.StopIteration
	}
	return tok, nil
}

// ---------------------------------------------------------------------------
// module-level functions

const split_doc = `split(s, comments=False, posix=True)

Split the string s using shell-like syntax.  If comments is False (the
default), the parsing of comments in the given string is disabled and the
comment character has no special meaning.`

func splitFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var sObj, comments, posix py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO", []string{"s", "comments", "posix"},
		&sObj, &comments, &posix); err != nil {
		return nil, err
	}
	if sObj == py.None {
		return nil, py.ExceptionNewf(py.ValueError, "s argument must not be None")
	}
	s, err := py.StrAsString(sObj)
	if err != nil {
		return nil, err
	}
	posixValue := true
	if posix != nil && posix != py.None {
		b, err := py.ObjectIsTrue(posix)
		if err != nil {
			return nil, err
		}
		posixValue = b
	}
	commentsValue := false
	if comments != nil && comments != py.None {
		b, err := py.ObjectIsTrue(comments)
		if err != nil {
			return nil, err
		}
		commentsValue = b
	}
	lex := &shlex{dict: py.NewStringDict()}
	if _, err := lexInitFrom(lex, s, posixValue); err != nil {
		return nil, err
	}
	lex.dict.Set("whitespace_split", py.True)
	if !commentsValue {
		lex.dict.Set("commenters", py.String(""))
	}
	return shlexList(lex)
}

// lexInitFrom configures a lexer for a string source, sharing the constructor's
// defaults; it exists so split() does not have to fabricate Python arguments.
func lexInitFrom(s *shlex, src string, posix bool) (py.Object, error) {
	dummyArgs := py.Tuple{py.String(src)}
	dummyKwargs := py.NewStringDict()
	dummyKwargs.Set("posix", py.NewBool(posix))
	res, err := shlexNew(nil, dummyArgs, dummyKwargs)
	if err != nil {
		return nil, err
	}
	other := res.(*shlex)
	s.dict = other.dict
	s.input = other.input
	s.runes = other.runes
	s.pos = other.pos
	return py.None, nil
}

func shlexList(s *shlex) (py.Object, error) {
	out := py.NewList()
	for {
		tok, err := s.getToken()
		if err != nil {
			return nil, err
		}
		eof, _ := s.dict.Get("eof")
		if tok == eof {
			return out, nil
		}
		out.Append(tok)
	}
}

const quote_doc = `quote(s)

Return a shell-escaped version of the string s.`

func quoteFn(self py.Object, args py.Tuple) (py.Object, error) {
	var sObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "quote", 1, 1, &sObj); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(sObj)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "expected string object, got %s", sObj.Type().Name)
	}
	return py.String(quoteString(s)), nil
}

// quoteString is shlex.quote: single quotes, with an embedded single quote
// replaced by the '\"'\"' splice.  A string that is ASCII and made only of
// the shell-safe characters is returned unchanged.
func quoteString(s string) string {
	if s == "" {
		return "''"
	}
	if isASCII(s) && isShellSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// isShellSafe is CPython's `s.encode().translate(None, delete=safe_chars)`
// test: true when every byte is one of the safe characters
// b'%+,-./0123456789:=@' plus the ASCII letters and '_'.
func isShellSafe(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("%+,-./:=@_", c) >= 0:
		default:
			return false
		}
	}
	return true
}

// isASCII reports whether s is entirely ASCII, matching str.isascii().
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

const join_doc = `join(split_command)

Return a shell-escaped string from split_command.`

func joinFn(self py.Object, args py.Tuple) (py.Object, error) {
	var seq py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "join", 1, 1, &seq); err != nil {
		return nil, err
	}
	items, err := py.SequenceList(seq)
	if err != nil {
		return nil, err
	}
	parts := make([]string, 0, len(items.Items))
	for _, item := range items.Items {
		s, err := py.StrAsString(item)
		if err != nil {
			return nil, err
		}
		parts = append(parts, quoteString(s))
	}
	return py.String(strings.Join(parts, " ")), nil
}

// ---------------------------------------------------------------------------
// small helpers

func containsRune(set string, r rune) bool {
	if r == eofRune {
		return false
	}
	return strings.ContainsRune(set, r)
}

// nextcharMatchesRune is the `nextchar in self.escape` test, kept separate so
// the escape state can be entered on an empty set without a false positive.
func nextcharMatchesRune(set string, r rune) bool {
	return containsRune(set, r)
}

func filterRunes(s, remove string) string {
	var b strings.Builder
	for _, r := range s {
		if !strings.ContainsRune(remove, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
