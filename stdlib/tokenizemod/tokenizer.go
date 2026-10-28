package tokenizemod

// A port of CPython 3.14's C tokenizer (Parser/lexer/lexer.c driven the way
// Python/Python-tokenize.c drives it), restricted to what the _tokenize module
// needs: a readline-fed tokenizer producing tokens with extra_tokens=True.
//
// The structure and the names follow lexer.c on purpose, so a diff against the
// C is easy to read.  Things the parser needs but tokenize.py never sees
// (type comments, the f-string expression text buffer, interactive prompts)
// are not ported.

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/vishnukv64/gpython/py"
)

// Token types (Include/internal/pycore_token.h).
const (
	tENDMARKER      = 0
	tNAME           = 1
	tNUMBER         = 2
	tSTRING         = 3
	tNEWLINE        = 4
	tINDENT         = 5
	tDEDENT         = 6
	tEXCLAMATION    = 54
	tOP             = 55
	tFSTRING_START  = 59
	tFSTRING_MIDDLE = 60
	tFSTRING_END    = 61
	tTSTRING_START  = 62
	tTSTRING_MIDDLE = 63
	tTSTRING_END    = 64
	tCOMMENT        = 65
	tNL             = 66
	tERRORTOKEN     = 67
)

// tok->done values.
const (
	eOK = iota
	eEOF
	eError
	eDedent
	eTabspace
	eTooDeep
	eLineCont
)

const (
	eofc            = -1
	maxIndent       = 100
	maxLevel        = 200
	maxFStringLevel = 150
	maxExprNesting  = 3
	tabSize         = 8
	altTabSize      = 1
)

type mode struct {
	fstring               bool // TOK_FSTRING_MODE, else TOK_REGULAR_MODE
	curlyBracketDepth     int
	curlyBracketExprStart int
	quote                 int
	quoteSize             int
	raw                   bool
	tstring               bool
	start, multiLineStart int
	firstLine             int
	inFormatSpec, inDebug bool
}

type tokState struct {
	readline func() (py.Object, error)
	encoding string

	buf       []byte
	cur       int // index into buf; the C "inp" is len(buf)
	lineStart int
	start     int // -1 is NULL
	mlStart   int // multi_line_start
	lineno    int
	firstLine int
	done      int

	atbol           bool
	pendin          int
	indent          int
	indstack        [maxIndent]int
	altindstack     [maxIndent]int
	level           int
	parenstack      [maxLevel]byte
	parenlineno     [maxLevel]int
	commentNewline  bool
	implicitNewline bool
	extra           bool

	modes   [maxFStringLevel]mode
	modeIdx int

	// the token produced by the last get(): p_start / p_end (-1 is NULL)
	ps, pe int

	err error // the pending Python exception, if any
}

func newTokState(readline func() (py.Object, error), encoding string, extra bool) *tokState {
	t := &tokState{readline: readline, encoding: encoding, extra: extra, atbol: true, start: -1, ps: -1, pe: -1}
	t.modes[0].curlyBracketExprStart = -1
	return t
}

func (t *tokState) mode() *mode         { return &t.modes[t.modeIdx] }
func (t *tokState) insideFString() bool { return t.modeIdx > 0 }
func (m *mode) insideExpr() bool        { return m.curlyBracketExprStart >= 0 }
func (t *tokState) prefix() string {
	if t.mode().tstring {
		return "t"
	}
	return "f"
}

// ---- errors -----------------------------------------------------------

func (t *tokState) setErr(typ *py.Type, args py.Tuple) {
	obj, err := py.ExceptionNew(typ, args, py.StringDict{})
	if err != nil {
		t.err = err
		return
	}
	t.err = obj.(*py.Exception)
}

// lineText is the current line up to the first newline or NUL, the text a C
// strcspn(line_start, "\n") would see.
func (t *tokState) lineText() string {
	line := t.buf[t.lineStart:]
	if i := bytes.IndexAny(line, "\n\x00"); i >= 0 {
		line = line[:i]
	}
	return string(line)
}

// syntaxerror is _PyTokenizer_syntaxerror: col is -1 for "up to cur".
func (t *tokState) syntaxerror(col int, msg string) int {
	if col == -1 {
		col = utf8.RuneCount(t.buf[t.lineStart:t.cur])
	}
	text := t.lineText()
	t.setErr(py.SyntaxError, py.Tuple{py.String(msg), py.Tuple{
		py.String("<string>"), py.Int(t.lineno), py.Int(col), py.String(text), py.Int(t.lineno), py.Int(col)}})
	t.done = eError
	return tERRORTOKEN
}

func (t *tokState) indenterror() int {
	t.done = eTabspace
	t.cur = len(t.buf)
	return tERRORTOKEN
}

// tokenizerError is _tokenizer_error in Python-tokenize.c.
func (t *tokState) tokenizerError() {
	var typ = py.SyntaxError
	msg := "unknown tokenization error"
	switch t.done {
	case eEOF:
		off := len(t.buf)
		obj, err := py.ExceptionNew(py.SyntaxError, py.Tuple{py.String("unexpected EOF in multi-line statement")}, py.StringDict{})
		if err != nil {
			t.err = err
			return
		}
		e := obj.(*py.Exception)
		e.Dict.Set("filename", py.String("<string>"))
		e.Dict.Set("lineno", py.Int(t.lineno))
		e.Dict.Set("offset", py.Int(off))
		e.Dict.Set("end_lineno", py.Int(t.lineno))
		t.err = e
		return
	case eDedent:
		msg, typ = "unindent does not match any outer indentation level", py.IndentationError
	case eTabspace:
		msg, typ = "inconsistent use of tabs and spaces in indentation", py.TabError
	case eTooDeep:
		msg, typ = "too many levels of indentation", py.IndentationError
	case eLineCont:
		msg = "unexpected character after line continuation character"
	}
	line := ""
	if n := len(t.buf); n > 0 {
		line = strings.ToValidUTF8(string(t.buf[:n-1]), "\uFFFD")
	}
	offset := utf8.RuneCountInString(line) + 1
	t.setErr(typ, py.Tuple{py.String(msg), py.Tuple{
		py.String("<string>"), py.Int(t.lineno), py.Int(offset), py.String(line), py.None, py.None}})
}

// ---- input ------------------------------------------------------------

func (t *tokState) underflow() bool {
	if t.start == -1 && !t.insideFString() {
		t.buf = t.buf[:0]
		t.cur = 0
	}
	raw, err := t.readline()
	var line string
	switch {
	case err != nil:
		if !py.IsException(py.StopIteration, err) {
			t.err, t.done = err, eError
			return false
		}
	case t.encoding != "":
		b, ok := raw.(py.Bytes)
		if !ok {
			t.setErr(py.TypeError, py.Tuple{py.String("readline() returned a non-bytes object")})
			t.done = eError
			return false
		}
		dec, err := py.GetAttrString(b, "decode")
		if err == nil {
			var s py.Object
			s, err = py.Call(dec, py.Tuple{py.String(t.encoding), py.String("replace")}, py.StringDict{})
			if err == nil {
				line = string(s.(py.String))
			}
		}
		if err != nil {
			t.err, t.done = err, eError
			return false
		}
	default:
		s, ok := raw.(py.String)
		if !ok {
			t.setErr(py.TypeError, py.Tuple{py.String("readline() returned a non-string object")})
			t.done = eError
			return false
		}
		line = string(s)
	}
	t.lineStart = t.cur
	t.buf = append(t.buf, line...)
	if len(t.buf) == t.cur {
		t.done = eEOF
		return false
	}
	t.implicitNewline = false
	if t.buf[len(t.buf)-1] != '\n' {
		t.buf = append(t.buf, '\n')
		t.implicitNewline = true
	}
	t.lineno++
	return true
}

func (t *tokState) nextc() int {
	for {
		if t.cur != len(t.buf) {
			t.cur++
			return int(t.buf[t.cur-1])
		}
		if t.done != eOK {
			return eofc
		}
		if !t.underflow() {
			t.cur = len(t.buf)
			return eofc
		}
		t.lineStart = t.cur
		if bytes.IndexByte(t.buf[t.lineStart:], 0) >= 0 {
			t.syntaxerror(-1, "source code cannot contain null bytes")
			t.cur = len(t.buf)
			return eofc
		}
	}
}

func (t *tokState) backup(c int) {
	if c != eofc {
		t.cur--
	}
}

// ---- character classes ------------------------------------------------

func isIdentStart(c int) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 128
}
func isIdentChar(c int) bool { return isIdentStart(c) || c >= '0' && c <= '9' }
func isDigit(c int) bool     { return c >= '0' && c <= '9' }
func isXDigit(c int) bool {
	return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

var oneChar = map[byte]int{'(': 7, ')': 8, '[': 9, ']': 10, ':': 11, ',': 12, ';': 13, '+': 14, '-': 15,
	'*': 16, '/': 17, '|': 18, '&': 19, '<': 20, '>': 21, '=': 22, '.': 23, '%': 24, '{': 25, '}': 26,
	'~': 31, '^': 32, '@': 49, '!': tEXCLAMATION}

var twoChars = map[string]int{"!=": 28, "%=": 40, "&=": 41, "**": 35, "*=": 38, "+=": 36, "-=": 37, "->": 51,
	"//": 47, "/=": 39, ":=": 53, "<<": 33, "<=": 29, "<>": 28, "==": 27, ">=": 30, ">>": 34, "@=": 49 + 1,
	"^=": 43, "|=": 42}

var threeChars = map[string]int{"**=": 46, "...": 52, "//=": 48, "<<=": 44, ">>=": 45}

func tokOneChar(c int) int {
	if v, ok := oneChar[byte(c)]; ok {
		return v
	}
	return tOP
}

func tokTwoChars(c1, c2 int) int {
	if c1 < 0 || c2 < 0 {
		return tOP
	}
	if v, ok := twoChars[string([]byte{byte(c1), byte(c2)})]; ok {
		return v
	}
	return tOP
}

func tokThreeChars(c1, c2, c3 int) int {
	if c1 < 0 || c2 < 0 || c3 < 0 {
		return tOP
	}
	if v, ok := threeChars[string([]byte{byte(c1), byte(c2), byte(c3)})]; ok {
		return v
	}
	return tOP
}

// ---- the lexer --------------------------------------------------------

// mk is MAKE_TOKEN: record p_start / p_end and return the type.
func (t *tokState) mk(typ, ps, pe int) int {
	t.ps, t.pe = ps, pe
	return typ
}

func (t *tokState) decimalTail() (int, bool) {
	var c int
	for {
		for {
			c = t.nextc()
			if !isDigit(c) {
				break
			}
		}
		if c != '_' {
			break
		}
		c = t.nextc()
		if !isDigit(c) {
			t.backup(c)
			t.syntaxerror(-1, "invalid decimal literal")
			return 0, false
		}
	}
	return c, true
}

func (t *tokState) continuationLine() int {
	c := t.nextc()
	if c == '\r' {
		c = t.nextc()
	}
	if c != '\n' {
		t.done = eLineCont
		return -1
	}
	c = t.nextc()
	if c == eofc {
		t.done = eEOF
		t.cur = len(t.buf)
		return -1
	}
	t.backup(c)
	return c
}

// number lexes a numeric literal whose first character c was just read (or,
// with fromDot, whose "." and following digit c were).  It leaves cur at the
// end of the literal and reports false after raising a syntax error.
func (t *tokState) number(c int, fromDot bool) bool {
	afterDot := fromDot
	// radix lexes the digits of a 0x / 0o / 0b literal.
	radix := func(name string, ok func(int) bool, decimalDigitsAreErrors bool) bool {
		c = t.nextc()
		for {
			if c == '_' {
				c = t.nextc()
			}
			if !ok(c) {
				if decimalDigitsAreErrors && isDigit(c) {
					t.syntaxerror(-1, "invalid digit '"+string(rune(c))+"' in "+name+" literal")
					return false
				}
				t.backup(c)
				t.syntaxerror(-1, "invalid "+name+" literal")
				return false
			}
			for {
				c = t.nextc()
				if !ok(c) {
					break
				}
			}
			if c != '_' {
				break
			}
		}
		if decimalDigitsAreErrors && isDigit(c) {
			t.syntaxerror(-1, "invalid digit '"+string(rune(c))+"' in "+name+" literal")
			return false
		}
		t.backup(c)
		return true
	}
	if !fromDot {
		if c == '0' {
			c = t.nextc()
			switch c {
			case 'x', 'X':
				return radix("hexadecimal", isXDigit, false)
			case 'o', 'O':
				return radix("octal", func(c int) bool { return c >= '0' && c < '8' }, true)
			case 'b', 'B':
				return radix("binary", func(c int) bool { return c == '0' || c == '1' }, true)
			}
			nonzero := false
			for {
				if c == '_' {
					c = t.nextc()
					if !isDigit(c) {
						t.backup(c)
						t.syntaxerror(-1, "invalid decimal literal")
						return false
					}
				}
				if c != '0' {
					break
				}
				c = t.nextc()
			}
			if isDigit(c) {
				nonzero = true
				var ok bool
				if c, ok = t.decimalTail(); !ok {
					return false
				}
			}
			if nonzero && !t.extra && c != '.' && c != 'e' && c != 'E' && c != 'j' && c != 'J' {
				t.backup(c)
				t.syntaxerror(utf8.RuneCount(t.buf[t.lineStart:t.start])+1,
					"leading zeros in decimal integer literals are not permitted; use an 0o prefix for octal integers")
				return false
			}
		} else {
			var ok bool
			if c, ok = t.decimalTail(); !ok {
				return false
			}
		}
		if c == '.' {
			c = t.nextc()
			afterDot = true
		}
	}
	if afterDot && isDigit(c) {
		var ok bool
		if c, ok = t.decimalTail(); !ok {
			return false
		}
	}
	if c == 'e' || c == 'E' {
		e := c
		c = t.nextc()
		if c == '+' || c == '-' {
			c = t.nextc()
			if !isDigit(c) {
				t.backup(c)
				t.syntaxerror(-1, "invalid decimal literal")
				return false
			}
		} else if !isDigit(c) {
			t.backup(c)
			t.backup(e)
			return true
		}
		var ok bool
		if c, ok = t.decimalTail(); !ok {
			return false
		}
	}
	if c == 'j' || c == 'J' {
		c = t.nextc()
	}
	t.backup(c)
	return true
}

func (t *tokState) stringPrefixError(sawB, sawR, sawU, sawF, sawT bool) int {
	pair := func(a, b string) int {
		return t.syntaxerror(utf8.RuneCount(t.buf[t.lineStart:t.start])+1, "'"+a+"' and '"+b+"' prefixes are incompatible")
	}
	switch {
	case sawU && sawB:
		return pair("u", "b")
	case sawU && sawR:
		return pair("u", "r")
	case sawU && sawF:
		return pair("u", "f")
	case sawU && sawT:
		return pair("u", "t")
	case sawB && sawF:
		return pair("b", "f")
	case sawB && sawT:
		return pair("b", "t")
	case sawF && sawT:
		return pair("f", "t")
	}
	return 0
}

// get is tok_get.
func (t *tokState) get() int {
	t.ps, t.pe = -1, -1
	m := t.mode()
	if m.fstring {
		return t.getFStringMode(m)
	}
	return t.getNormalMode(m)
}

func (t *tokState) getNormalMode(cur *mode) int {
	var c int
nextline:
	for {
		t.start = -1
		blankline := false
		if t.atbol {
			col, altcol := 0, 0
			t.atbol = false
			contLineCol := 0
			for {
				c = t.nextc()
				if c == ' ' {
					col++
					altcol++
				} else if c == '\t' {
					col = (col/tabSize + 1) * tabSize
					altcol = (altcol/altTabSize + 1) * altTabSize
				} else if c == '\014' {
					col, altcol = 0, 0
				} else if c == '\\' {
					if contLineCol == 0 {
						contLineCol = col
					}
					if c = t.continuationLine(); c == -1 {
						return t.mk(tERRORTOKEN, -1, -1)
					}
				} else {
					break
				}
			}
			t.backup(c)
			if c == '#' || c == '\n' || c == '\r' {
				blankline = true
			}
			if !blankline && t.level == 0 {
				if contLineCol != 0 {
					col, altcol = contLineCol, contLineCol
				}
				switch {
				case col == t.indstack[t.indent]:
					if altcol != t.altindstack[t.indent] {
						return t.mk(t.indenterror(), -1, -1)
					}
				case col > t.indstack[t.indent]:
					if t.indent+1 >= maxIndent {
						t.done = eTooDeep
						t.cur = len(t.buf)
						return t.mk(tERRORTOKEN, -1, -1)
					}
					if altcol <= t.altindstack[t.indent] {
						return t.mk(t.indenterror(), -1, -1)
					}
					t.pendin++
					t.indent++
					t.indstack[t.indent] = col
					t.altindstack[t.indent] = altcol
				default:
					for t.indent > 0 && col < t.indstack[t.indent] {
						t.pendin--
						t.indent--
					}
					if col != t.indstack[t.indent] {
						t.done = eDedent
						t.cur = len(t.buf)
						return t.mk(tERRORTOKEN, -1, -1)
					}
					if altcol != t.altindstack[t.indent] {
						return t.mk(t.indenterror(), -1, -1)
					}
				}
			}
		}
		t.start = t.cur
		if t.pendin != 0 {
			if t.pendin < 0 {
				t.pendin++
				if t.extra {
					return t.mk(tDEDENT, t.cur, t.cur)
				}
				return t.mk(tDEDENT, -1, -1)
			}
			t.pendin--
			if t.extra {
				return t.mk(tINDENT, 0, t.cur)
			}
			return t.mk(tINDENT, -1, -1)
		}
		c = t.nextc()
		t.backup(c)

	again:
		for {
			t.start = -1
			for {
				c = t.nextc()
				if c != ' ' && c != '\t' && c != '\014' {
					break
				}
			}
			t.start = t.cur - 1
			if c == '#' {
				for c != eofc && c != '\n' && c != '\r' {
					c = t.nextc()
				}
				if t.extra {
					t.backup(c)
					t.commentNewline = blankline
					return t.mk(tCOMMENT, t.start, t.cur)
				}
			}
			if c == eofc {
				if t.level > 0 {
					return t.mk(tERRORTOKEN, -1, -1)
				}
				if t.done == eEOF {
					return t.mk(tENDMARKER, -1, -1)
				}
				return t.mk(tERRORTOKEN, -1, -1)
			}

			if isIdentStart(c) {
				var sawB, sawR, sawU, sawF, sawT bool
				for {
					switch {
					case !sawB && (c == 'b' || c == 'B'):
						sawB = true
					case !sawU && (c == 'u' || c == 'U'):
						sawU = true
					case !sawR && (c == 'r' || c == 'R'):
						sawR = true
					case !sawF && (c == 'f' || c == 'F'):
						sawF = true
					case !sawT && (c == 't' || c == 'T'):
						sawT = true
					default:
						goto prefixDone
					}
					c = t.nextc()
					if c == '"' || c == '\'' {
						if r := t.stringPrefixError(sawB, sawR, sawU, sawF, sawT); r != 0 {
							return t.mk(tERRORTOKEN, -1, -1)
						}
						if sawF || sawT {
							return t.fstringStart(c)
						}
						return t.plainString(c)
					}
				}
			prefixDone:
				for isIdentChar(c) {
					c = t.nextc()
				}
				t.backup(c)
				return t.mk(tNAME, t.start, t.cur)
			}

			if c == '\r' {
				c = t.nextc()
			}
			if c == '\n' {
				t.atbol = true
				if blankline || t.level > 0 {
					if t.extra {
						t.commentNewline = false
						return t.mk(tNL, t.start, t.cur)
					}
					continue nextline
				}
				if t.commentNewline && t.extra {
					t.commentNewline = false
					return t.mk(tNL, t.start, t.cur)
				}
				return t.mk(tNEWLINE, t.start, t.cur-1)
			}

			if c == '.' {
				c = t.nextc()
				if isDigit(c) {
					if !t.number(c, true) {
						return t.mk(tERRORTOKEN, -1, -1)
					}
					return t.mk(tNUMBER, t.start, t.cur)
				} else if c == '.' {
					c = t.nextc()
					if c == '.' {
						return t.mk(tOP, t.start, t.cur)
					}
					t.backup(c)
					t.backup('.')
				} else {
					t.backup(c)
				}
				return t.mk(tOP, t.start, t.cur)
			}

			if isDigit(c) {
				if !t.number(c, false) {
					return t.mk(tERRORTOKEN, -1, -1)
				}
				return t.mk(tNUMBER, t.start, t.cur)
			}

			if c == '\'' || c == '"' {
				return t.plainString(c)
			}

			if c == '\\' {
				if c = t.continuationLine(); c == -1 {
					return t.mk(tERRORTOKEN, -1, -1)
				}
				continue again
			}

			// f-string / t-string replacement-field punctuation
			if (c == ':' || c == '}' || c == '!' || c == '{') && t.insideFString() && cur.insideExpr() {
				bump := 0
				if c != '{' {
					bump = 1
				}
				cursor := cur.curlyBracketDepth - bump
				if c == ':' && cursor == cur.curlyBracketExprStart {
					cur.fstring = true
					cur.inFormatSpec = true
					return t.mk(tOP, t.start, t.cur)
				}
			}

			{
				c2 := t.nextc()
				typ := tokTwoChars(c, c2)
				if typ != tOP {
					c3 := t.nextc()
					if t3 := tokThreeChars(c, c2, c3); t3 != tOP {
						typ = t3
					} else {
						t.backup(c3)
					}
					return t.mk(typ, t.start, t.cur)
				}
				t.backup(c2)
			}

			switch c {
			case '(', '[', '{':
				if t.level >= maxLevel {
					return t.mk(t.syntaxerror(-1, "too many nested parentheses"), -1, -1)
				}
				t.parenstack[t.level] = byte(c)
				t.parenlineno[t.level] = t.lineno
				t.level++
				if t.insideFString() {
					cur.curlyBracketDepth++
				}
			case ')', ']', '}':
				if t.insideFString() && cur.curlyBracketDepth == 0 && c == '}' {
					return t.mk(t.syntaxerror(-1, t.prefix()+"-string: single '}' is not allowed"), -1, -1)
				}
				if t.level > 0 {
					t.level--
				}
				if t.insideFString() {
					cur.curlyBracketDepth--
					if cur.curlyBracketDepth < 0 {
						return t.mk(t.syntaxerror(-1, t.prefix()+"-string: unmatched '"+string(rune(c))+"'"), -1, -1)
					}
					if c == '}' && cur.curlyBracketDepth == cur.curlyBracketExprStart {
						cur.curlyBracketExprStart--
						cur.fstring = true
						cur.inFormatSpec = false
						cur.inDebug = false
					}
				}
			}
			if c < 0x20 || c == 0x7f {
				return t.mk(t.syntaxerror(-1, "invalid non-printable character U+"+hex4(c)), -1, -1)
			}
			if c == '=' && cur.insideExpr() {
				cur.inDebug = true
			}
			typ := tokOneChar(c)
			return t.mk(typ, t.start, t.cur)
		}
	}
}

func hex4(c int) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[c>>12&15], digits[c>>8&15], digits[c>>4&15], digits[c&15]})
}

// plainString lexes a string literal whose opening quote c was just read.
func (t *tokState) plainString(c int) int {
	quote := c
	quoteSize, endQuoteSize := 1, 0
	hasEscapedQuote := false
	t.firstLine = t.lineno
	t.mlStart = t.lineStart
	c = t.nextc()
	if c == quote {
		c = t.nextc()
		if c == quote {
			quoteSize = 3
		} else {
			endQuoteSize = 1
		}
	}
	if c != quote {
		t.backup(c)
	}
	for endQuoteSize != quoteSize {
		c = t.nextc()
		if t.done == eError {
			return t.mk(tERRORTOKEN, -1, -1)
		}
		if c == eofc || (quoteSize == 1 && c == '\n') {
			t.cur = t.start + 1
			t.lineStart = t.mlStart
			start := t.lineno
			t.lineno = t.firstLine
			if t.insideFString() {
				m := t.mode()
				if m.quote == quote && m.quoteSize == quoteSize {
					return t.mk(t.syntaxerror(-1, t.prefix()+"-string: expecting '}'"), -1, -1)
				}
			}
			if quoteSize == 3 {
				t.syntaxerror(-1, "unterminated triple-quoted string literal (detected at line "+itoa(start)+")")
			} else if hasEscapedQuote {
				t.syntaxerror(-1, "unterminated string literal (detected at line "+itoa(start)+"); perhaps you escaped the end quote?")
			} else {
				t.syntaxerror(-1, "unterminated string literal (detected at line "+itoa(start)+")")
			}
			return t.mk(tERRORTOKEN, -1, -1)
		}
		if c == quote {
			endQuoteSize++
		} else {
			endQuoteSize = 0
			if c == '\\' {
				c = t.nextc()
				if c == quote {
					hasEscapedQuote = true
				}
				if c == '\r' {
					t.nextc() // the \n of an escaped \r\n line ending
				}
			}
		}
	}
	return t.mk(tSTRING, t.start, t.cur)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for ; n > 0; n /= 10 {
		i--
		b[i] = byte('0' + n%10)
	}
	return string(b[i:])
}

// fstringStart handles the f_string_quote label: the opening quote c of an
// f- or t-string was just read, after its prefix.
func (t *tokState) fstringStart(c int) int {
	quote := c
	quoteSize := 1
	t.firstLine = t.lineno
	t.mlStart = t.lineStart
	afterQuote := t.nextc()
	if afterQuote == quote {
		afterAfter := t.nextc()
		if afterAfter == quote {
			quoteSize = 3
		} else {
			t.backup(afterAfter)
			t.backup(afterQuote)
		}
	}
	if afterQuote != quote {
		t.backup(afterQuote)
	}
	ps, pe := t.start, t.cur
	if t.modeIdx+1 >= maxFStringLevel {
		return t.mk(t.syntaxerror(-1, "too many nested f-strings or t-strings"), -1, -1)
	}
	t.modeIdx++
	m := t.mode()
	*m = mode{fstring: true, quote: quote, quoteSize: quoteSize, start: t.start, multiLineStart: t.lineStart,
		firstLine: t.lineno, curlyBracketExprStart: -1}
	lower := func(b byte) byte { return b | 0x20 }
	switch lower(t.buf[t.start]) {
	case 't':
		m.raw = lower(t.buf[t.start+1]) == 'r'
		m.tstring = true
	case 'f':
		m.raw = lower(t.buf[t.start+1]) == 'r'
	case 'r':
		m.raw = true
		m.tstring = lower(t.buf[t.start+1]) == 't'
	}
	if m.tstring {
		return t.mk(tTSTRING_START, ps, pe)
	}
	return t.mk(tFSTRING_START, ps, pe)
}

func (t *tokState) getFStringMode(cur *mode) int {
	endQuoteSize := 0
	unicodeEscape := false
	middle := func() int {
		if cur.tstring {
			return tTSTRING_MIDDLE
		}
		return tFSTRING_MIDDLE
	}
	t.start = t.cur
	t.firstLine = t.lineno
	startChar := t.nextc()
	if startChar == '{' {
		peek1 := t.nextc()
		t.backup(peek1)
		t.backup(startChar)
		if peek1 != '{' {
			cur.curlyBracketExprStart++
			if cur.curlyBracketExprStart >= maxExprNesting {
				return t.mk(t.syntaxerror(-1, t.prefix()+"-string: expressions nested too deeply"), -1, -1)
			}
			cur.fstring = false
			return t.getNormalMode(cur)
		}
	} else {
		t.backup(startChar)
	}

	// end of the string?
	atEnd := true
	for i := 0; i < cur.quoteSize; i++ {
		q := t.nextc()
		if q != cur.quote {
			t.backup(q)
			atEnd = false
			break
		}
	}
	if atEnd {
		t.modeIdx--
		if cur.tstring {
			return t.mk(tTSTRING_END, t.start, t.cur)
		}
		return t.mk(tFSTRING_END, t.start, t.cur)
	}

	t.mlStart = t.lineStart
	for endQuoteSize != cur.quoteSize {
		c := t.nextc()
		if t.done == eError {
			return t.mk(tERRORTOKEN, -1, -1)
		}
		inFormatSpec := cur.inFormatSpec && cur.insideExpr()
		if c == eofc || (cur.quoteSize == 1 && c == '\n') {
			if inFormatSpec && c == '\n' {
				if cur.quoteSize == 1 {
					return t.mk(t.syntaxerror(-1, t.prefix()+"-string: newlines are not allowed in format specifiers for single quoted "+t.prefix()+"-strings"), -1, -1)
				}
				t.backup(c)
				cur.fstring = false
				cur.inFormatSpec = false
				return t.mk(middle(), t.start, t.cur)
			}
			t.cur = cur.start + 1
			t.lineStart = cur.multiLineStart
			start := t.lineno
			t.lineno = cur.firstLine
			if cur.quoteSize == 3 {
				t.syntaxerror(-1, "unterminated triple-quoted "+t.prefix()+"-string literal (detected at line "+itoa(start)+")")
				return t.mk(tERRORTOKEN, -1, -1)
			}
			return t.mk(t.syntaxerror(-1, "unterminated "+t.prefix()+"-string literal (detected at line "+itoa(start)+")"), -1, -1)
		}
		if c == cur.quote {
			endQuoteSize++
			continue
		}
		endQuoteSize = 0
		if c == '{' {
			peek := t.nextc()
			if peek != '{' || inFormatSpec {
				t.backup(peek)
				t.backup(c)
				cur.curlyBracketExprStart++
				if cur.curlyBracketExprStart >= maxExprNesting {
					return t.mk(t.syntaxerror(-1, t.prefix()+"-string: expressions nested too deeply"), -1, -1)
				}
				cur.fstring = false
				cur.inFormatSpec = false
				return t.mk(middle(), t.start, t.cur)
			}
			return t.mk(middle(), t.start, t.cur-1)
		} else if c == '}' {
			if unicodeEscape {
				return t.mk(middle(), t.start, t.cur)
			}
			peek := t.nextc()
			if peek == '}' && !inFormatSpec && cur.curlyBracketDepth == 0 {
				return t.mk(middle(), t.start, t.cur-1)
			}
			t.backup(peek)
			t.backup(c)
			cur.fstring = false
			cur.inFormatSpec = false
			return t.mk(middle(), t.start, t.cur)
		} else if c == '\\' {
			peek := t.nextc()
			if peek == '\r' {
				peek = t.nextc()
			}
			if peek == '{' || peek == '}' {
				t.backup(peek)
				continue
			}
			if !cur.raw && peek == 'N' {
				peek = t.nextc()
				if peek == '{' {
					unicodeEscape = true
				} else {
					t.backup(peek)
				}
			}
		}
	}
	for i := 0; i < cur.quoteSize; i++ {
		t.backup(cur.quote)
	}
	return t.mk(middle(), t.start, t.cur)
}
