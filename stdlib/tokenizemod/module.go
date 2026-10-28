// Package tokenizemod provides gpython's _tokenize: the C tokenizer behind
// CPython's tokenize.py, ported to Go.
package tokenizemod

import (
	"unicode/utf8"

	"github.com/vishnukv64/gpython/py"
)

type tokenizerIter struct {
	t        *tokState
	done     bool
	lastLine int // tok->lineno when lastText was cut
	lastText string
}

var tokenizerIterType = py.NewTypeX("TokenizerIter", "Token iterator over a readline callable.", tokenizerIterNew, nil)

func (it *tokenizerIter) Type() *py.Type { return tokenizerIterType }

func tokenizerIterNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "tokenizeriter() takes exactly 1 positional argument (%d given)", len(args))
	}
	extraObj, ok := kwargs.Get("extra_tokens")
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "tokenizeriter() missing required argument 'extra_tokens' (pos 2)")
	}
	extra, err := py.ObjectIsTrue(extraObj)
	if err != nil {
		return nil, err
	}
	enc := ""
	if e, ok := kwargs.Get("encoding"); ok && e != py.None {
		s, isStr := e.(py.String)
		if !isStr {
			return nil, py.ExceptionNewf(py.TypeError, "tokenizeriter() argument 'encoding' must be str, not %s", e.Type().Name)
		}
		enc = string(s)
	}
	readline := args[0]
	return &tokenizerIter{t: newTokState(func() (py.Object, error) { return py.Call(readline, py.Tuple{}, py.StringDict{}) }, enc, extra)}, nil
}

func (it *tokenizerIter) M__iter__() (py.Object, error) { return it, nil }

func chars(b []byte) int { return utf8.RuneCount(b) }

func (it *tokenizerIter) M__next__() (py.Object, error) {
	t := it.t
	if it.done {
		return nil, py.StopIteration
	}
	typ := t.get()
	if typ == tERRORTOKEN {
		if t.err == nil {
			t.tokenizerError()
		}
		return nil, t.err
	}
	str := ""
	if t.ps >= 0 && t.pe >= 0 {
		str = string(t.buf[t.ps:t.pe])
	}
	trailing := typ == tENDMARKER || (typ == tDEDENT && t.done == eEOF)

	stringLit := typ == tSTRING || typ == tFSTRING_MIDDLE || typ == tTSTRING_MIDDLE
	lineStart := t.lineStart
	if stringLit {
		lineStart = t.mlStart
	}
	line := ""
	if !(t.extra && trailing) {
		if t.lineno != it.lastLine {
			size := len(t.buf) - lineStart
			if size >= 1 && t.implicitNewline {
				size--
			}
			it.lastText = string(t.buf[lineStart : lineStart+size])
		}
		line = it.lastText
	}

	lineno, endLineno := t.lineno, t.lineno
	if stringLit {
		lineno = t.firstLine
	}
	col, endCol := -1, -1
	if t.ps >= 0 && t.ps >= lineStart {
		col = chars(t.buf[lineStart:t.ps])
	}
	if t.pe >= 0 && t.pe >= t.lineStart {
		if lineno == endLineno {
			endCol = col + chars(t.buf[t.ps:t.pe])
		} else {
			endCol = chars(t.buf[t.lineStart:t.pe])
		}
	}
	it.lastLine = lineno

	if t.extra {
		if trailing {
			lineno++
			endLineno = lineno
			col, endCol = 0, 0
		}
		switch {
		case typ > tDEDENT && typ < tOP:
			typ = tOP
		case typ == tNEWLINE:
			if !t.implicitNewline {
				str = "\n"
				if t.buf[t.start] == '\r' {
					str = "\r\n"
				}
			}
			endCol++
		case typ == tNL:
			if t.implicitNewline {
				str = ""
			}
		}
	}
	if typ == tENDMARKER {
		it.done = true
	}
	return py.Tuple{py.Int(typ), py.String(str),
		py.Tuple{py.Int(lineno), py.Int(col)}, py.Tuple{py.Int(endLineno), py.Int(endCol)}, py.String(line)}, nil
}

var _ py.I__iter__ = (*tokenizerIter)(nil)
var _ py.I__next__ = (*tokenizerIter)(nil)

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_tokenize", Doc: "The C tokenizer behind tokenize.py."},
		PreSetGlobals: func(g py.StringDict) {
			g.Set("TokenizerIter", tokenizerIterType)
		},
	})
}
