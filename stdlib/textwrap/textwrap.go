// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package textwrap provides the implementation of python's 'textwrap' module.
//
// TextWrapper and the wrap() algorithm are a direct port of CPython's
// Lib/textwrap.py, including the wordsep_re scanner, so that wrap/fill/shorten
// produce byte-identical output.  The scanner is hand-written rather than
// expressed with Go's regexp because CPython's word separator uses lookbehind
// assertions, which Go's RE2 does not support.
package textwrap

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Text wrapping and filling.`

// ---------------------------------------------------------------------------
// Character classes
//
// CPython's regexes use \w and \d, which are Unicode-aware.  These predicates
// cover the ASCII range exactly and treat any byte >= 0x80 as a word character,
// which matches \w's treatment of non-ASCII letters closely enough for the
// whitespace-splitting they drive.

// isWS reports whether c is one of CPython's hardcoded ASCII whitespace
// characters (note: not the Unicode whitespace set).
func isWS(c byte) bool {
	switch c {
	case '\t', '\n', '\v', '\f', '\r', ' ':
		return true
	}
	return false
}

// isWordChar approximates Python's \w (letters, digits and underscore).
func isWordChar(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// isLt approximates Python's [^\d\W]: a word character that is not a digit.
func isLt(c byte) bool {
	return isWordChar(c) && !(c >= '0' && c <= '9')
}

// isWP approximates Python's [\w!"'&.,?], the set after which an em-dash may
// start a new chunk.
func isWP(c byte) bool {
	if isWordChar(c) {
		return true
	}
	switch c {
	case '!', '"', '\'', '&', '.', ',', '?':
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// TextWrapper

// textWrapper is the Go state behind the TextWrapper class.
type textWrapper struct {
	width              int
	initialIndent      string
	subsequentIndent   string
	expandTabs         bool
	replaceWhitespace  bool
	fixSentenceEndings bool
	breakLongWords     bool
	dropWhitespace     bool
	breakOnHyphens     bool
	tabsize            int
	maxLines           int // 0 means None
	hasMaxLines        bool
	placeholder        string
}

var textWrapperType = py.NewTypeX("textwrap.TextWrapper",
	`TextWrapper(width=70, **kwargs)
Object for wrapping/filling text.  The public interface consists of the wrap()
and fill() methods; the other methods are just there for subclasses.`,
	textWrapperNew, nil)

func (w *textWrapper) Type() *py.Type { return textWrapperType }

func textWrapperNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	w := &textWrapper{
		width:             70,
		expandTabs:        true,
		replaceWhitespace: true,
		breakLongWords:    true,
		dropWhitespace:    true,
		breakOnHyphens:    true,
		tabsize:           8,
		placeholder:       " [...]",
	}
	// width is the only positional parameter and every other option is
	// keyword-only, as in CPython.  The signature is parsed by hand rather than
	// with ParseTupleAndKeywords because that helper rejects keywords absent
	// from its kwlist, and this constructor has twelve of them with mixed
	// types (and max_lines is None-or-int).
	var width py.Object = py.Int(70)
	switch len(args) {
	case 0:
	case 1:
		width = args[0]
	default:
		return nil, py.ExceptionNewf(py.TypeError, "TextWrapper() takes at most 1 positional argument (%d given)", len(args))
	}
	// width may also be given by keyword; positional wins if both appear, as
	// in CPython.
	if v, ok := kwargs["width"]; ok {
		if len(args) == 0 {
			width = v
		}
		delete(kwargs, "width")
	}
	n, err := py.IndexInt(width)
	if err != nil {
		return nil, err
	}
	w.width = n

	setString := func(key string, dst *string) error {
		if v, ok := kwargs[key]; ok {
			s, err := py.StrAsString(v)
			if err != nil {
				return err
			}
			*dst = s
			delete(kwargs, key)
		}
		return nil
	}
	setBool := func(key string, dst *bool) error {
		if v, ok := kwargs[key]; ok {
			b, err := py.ObjectIsTrue(v)
			if err != nil {
				return err
			}
			*dst = b
			delete(kwargs, key)
		}
		return nil
	}
	setInt := func(key string, dst *int) error {
		if v, ok := kwargs[key]; ok {
			n, err := py.IndexInt(v)
			if err != nil {
				return err
			}
			*dst = n
			delete(kwargs, key)
		}
		return nil
	}

	if err := setString("initial_indent", &w.initialIndent); err != nil {
		return nil, err
	}
	if err := setString("subsequent_indent", &w.subsequentIndent); err != nil {
		return nil, err
	}
	for key, dst := range map[string]*bool{
		"expand_tabs":          &w.expandTabs,
		"replace_whitespace":   &w.replaceWhitespace,
		"fix_sentence_endings": &w.fixSentenceEndings,
		"break_long_words":     &w.breakLongWords,
		"drop_whitespace":      &w.dropWhitespace,
		"break_on_hyphens":     &w.breakOnHyphens,
	} {
		if err := setBool(key, dst); err != nil {
			return nil, err
		}
	}
	if err := setInt("tabsize", &w.tabsize); err != nil {
		return nil, err
	}
	if v, ok := kwargs["max_lines"]; ok {
		if v != py.None {
			n, err := py.IndexInt(v)
			if err != nil {
				return nil, err
			}
			w.maxLines = n
			w.hasMaxLines = true
		}
		delete(kwargs, "max_lines")
	}
	if err := setString("placeholder", &w.placeholder); err != nil {
		return nil, err
	}
	return w, nil
}

// ---------------------------------------------------------------------------
// The wrapping algorithm, ported from CPython's TextWrapper.

// mungeWhitespace expands tabs and turns other whitespace into spaces.
func (w *textWrapper) mungeWhitespace(text string) string {
	if w.expandTabs {
		text = expandTabs(text, w.tabsize)
	}
	if w.replaceWhitespace {
		// CPython translates every character of _whitespace to a space.
		b := []byte(text)
		for i, c := range b {
			if isWS(c) {
				b[i] = ' '
			}
		}
		text = string(b)
	}
	return text
}

// expandTabs implements str.expandtabs(tabsize).
func expandTabs(s string, tabsize int) string {
	var b strings.Builder
	col := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\t':
			if tabsize > 0 {
				n := tabsize - col%tabsize
				for j := 0; j < n; j++ {
					b.WriteByte(' ')
				}
				col += n
			}
		case '\n', '\r':
			b.WriteByte(c)
			col = 0
		default:
			b.WriteByte(c)
			col++
		}
	}
	return b.String()
}

// splitChunks is CPython's _split: it isolates whitespace runs, em-dashes and
// (optionally hyphenated) words so that _wrapChunks can treat each as
// indivisible.
func (w *textWrapper) splitChunks(text string) []string {
	if !w.breakOnHyphens {
		// wordsep_simple_re: split on whitespace runs, keeping the separators.
		var chunks []string
		i := 0
		for i < len(text) {
			if isWS(text[i]) {
				j := i
				for j < len(text) && isWS(text[j]) {
					j++
				}
				chunks = append(chunks, text[i:j])
				i = j
				continue
			}
			j := i
			for j < len(text) && !isWS(text[j]) {
				j++
			}
			chunks = append(chunks, text[i:j])
			i = j
		}
		return chunks
	}

	var chunks []string
	i := 0
	n := len(text)
	for i < n {
		// Alternative 1: a run of whitespace.
		if isWS(text[i]) {
			j := i
			for j < n && isWS(text[j]) {
				j++
			}
			chunks = append(chunks, text[i:j])
			i = j
			continue
		}
		// Alternative 2: an em-dash between words.
		if text[i] == '-' {
			j := i
			for j < n && text[j] == '-' {
				j++
			}
			if j-i >= 2 && i > 0 && isWP(text[i-1]) && j < n && isWordChar(text[j]) {
				chunks = append(chunks, text[i:j])
				i = j
				continue
			}
		}
		// Alternative 3: a word, possibly hyphenated.  The word body is [^ws]+?,
		// which is non-greedy: the shortest extension that lets the suffix group
		// match wins.  Note the suffix's first alternative consumes a hyphen, so
		// a chunk that breaks after a hyphen ends WITH that hyphen.
		end := i + 1
		suffixEnd := -1
		for ; end < n; end++ {
			if e := wordSuffixEnd(text, end); e >= 0 {
				suffixEnd = e
				break
			}
		}
		if suffixEnd < 0 {
			suffixEnd = n
		}
		chunks = append(chunks, text[i:suffixEnd])
		i = suffixEnd
	}
	return chunks
}

// wordSuffixEnd returns the chunk end if the suffix group matches at position p,
// or -1 if it does not.  The group is
//
//	-(?<=[lt]{2}-|(?<=[lt]-[lt]-)(?=[lt]-?[lt]) | (?=[ws]|\z) | (?<=[wp])(?=-{2,}\w)
//
// with every assertion evaluated after any characters the alternative consumed.
func wordSuffixEnd(text string, p int) int {
	n := len(text)

	// First alternative: consume the '-' at p, then assert.
	if p < n && text[p] == '-' {
		hyphenBehind := false
		// (?<=[lt]{2}-): the three characters before p+1 are lt, lt, '-'.
		if p-2 >= 0 && isLt(text[p-2]) && isLt(text[p-1]) {
			hyphenBehind = true
		}
		// (?<=[lt]-[lt]-): the four characters before p+1 are lt, '-', lt, '-'.
		if !hyphenBehind && p-3 >= 0 && isLt(text[p-3]) && text[p-2] == '-' && isLt(text[p-1]) {
			hyphenBehind = true
		}
		if hyphenBehind && lookaheadLtHyphenLt(text, p+1) {
			return p + 1
		}
	}
	// Second alternative: a zero-width end-of-word assertion.
	if p >= n || isWS(text[p]) {
		return p
	}
	// Third alternative: a zero-width assertion that an em-dash follows.
	if p-1 >= 0 && isWP(text[p-1]) {
		j := p
		for j < n && text[j] == '-' {
			j++
		}
		if j-p >= 2 && j < n && isWordChar(text[j]) {
			return p
		}
	}
	return -1
}

// lookaheadLtHyphenLt implements (?=[lt]-?[lt]) at position q.
func lookaheadLtHyphenLt(text string, q int) bool {
	n := len(text)
	if q >= n || !isLt(text[q]) {
		return false
	}
	if q+1 < n && isLt(text[q+1]) {
		return true
	}
	if q+2 < n && text[q+1] == '-' && isLt(text[q+2]) {
		return true
	}
	return false
}

// sentenceEndRe reports whether the chunk ends a sentence, for
// fix_sentence_endings: a lowercase letter, then [.!?], then an optional quote.
func sentenceEnd(chunk string) bool {
	n := len(chunk)
	if n < 2 {
		return false
	}
	if chunk[n-1] == '"' || chunk[n-1] == '\'' {
		n--
	}
	if n < 2 {
		return false
	}
	c := chunk[n-1]
	if c != '.' && c != '!' && c != '?' {
		return false
	}
	p := chunk[n-2]
	return p >= 'a' && p <= 'z'
}

// fixSentenceEndings doubles the space after a sentence-ending chunk, matching
// CPython's _fix_sentence_endings.
func fixSentenceEndings(chunks []string) {
	i := 0
	for i < len(chunks)-1 {
		if chunks[i+1] == " " && sentenceEnd(chunks[i]) {
			chunks[i+1] = "  "
			i += 2
		} else {
			i++
		}
	}
}

// handleLongWord is CPython's _handle_long_word.
func (w *textWrapper) handleLongWord(chunks *[]string, curLine *[]string, curLen, width int) {
	spaceLeft := width - curLen
	if width < 1 {
		spaceLeft = 1
	}
	c := *chunks
	if w.breakLongWords && spaceLeft > 0 {
		end := spaceLeft
		chunk := c[len(c)-1]
		if w.breakOnHyphens && len(chunk) > spaceLeft {
			// Break after the last hyphen before spaceLeft, but only when
			// there is a non-hyphen before it.
			hyphen := strings.LastIndex(chunk[:spaceLeft], "-")
			if hyphen > 0 && strings.Trim(chunk[:hyphen], "-") != "" {
				end = hyphen + 1
			}
		}
		if end > len(chunk) {
			end = len(chunk)
		}
		*curLine = append(*curLine, chunk[:end])
		c[len(c)-1] = chunk[end:]
	} else if len(*curLine) == 0 {
		*curLine = append(*curLine, c[len(c)-1])
		*chunks = c[:len(c)-1]
	}
}

// wrapChunks is CPython's _wrap_chunks.
func (w *textWrapper) wrapChunks(chunks []string) ([]string, error) {
	if w.width <= 0 {
		return nil, py.ExceptionNewf(py.ValueError, "invalid width %d (must be > 0)", w.width)
	}
	if w.hasMaxLines {
		indent := w.initialIndent
		if w.maxLines > 1 {
			indent = w.subsequentIndent
		}
		if len(indent)+len(strings.TrimLeft(w.placeholder, " \t\n\r\v\f")) > w.width {
			return nil, py.ExceptionNewf(py.ValueError, "placeholder too large for max width")
		}
	}

	// Reverse the chunks so the next one is always the last element.
	for i, j := 0, len(chunks)-1; i < j; i, j = i+1, j-1 {
		chunks[i], chunks[j] = chunks[j], chunks[i]
	}

	var lines []string
	for len(chunks) > 0 {
		var curLine []string
		curLen := 0

		indent := w.initialIndent
		if len(lines) > 0 {
			indent = w.subsequentIndent
		}
		width := w.width - len(indent)

		// Drop a leading whitespace chunk, but not at the very start.
		if w.dropWhitespace && strings.TrimSpace(chunks[len(chunks)-1]) == "" && len(lines) > 0 {
			chunks = chunks[:len(chunks)-1]
		}

		for len(chunks) > 0 {
			l := len(chunks[len(chunks)-1])
			if curLen+l <= width {
				curLine = append(curLine, chunks[len(chunks)-1])
				chunks = chunks[:len(chunks)-1]
				curLen += l
			} else {
				break
			}
		}

		if len(chunks) > 0 && len(chunks[len(chunks)-1]) > width {
			w.handleLongWord(&chunks, &curLine, curLen, width)
			curLen = 0
			for _, s := range curLine {
				curLen += len(s)
			}
		}

		if w.dropWhitespace && len(curLine) > 0 && strings.TrimSpace(curLine[len(curLine)-1]) == "" {
			curLen -= len(curLine[len(curLine)-1])
			curLine = curLine[:len(curLine)-1]
		}

		if len(curLine) > 0 {
			if !w.hasMaxLines || len(lines)+1 < w.maxLines ||
				(len(chunks) == 0 || w.dropWhitespace && len(chunks) == 1 && strings.TrimSpace(chunks[0]) == "") && curLen <= width {
				lines = append(lines, indent+strings.Join(curLine, ""))
			} else {
				for len(curLine) > 0 {
					if strings.TrimSpace(curLine[len(curLine)-1]) != "" && curLen+len(w.placeholder) <= width {
						curLine = append(curLine, w.placeholder)
						lines = append(lines, indent+strings.Join(curLine, ""))
						return lines, nil
					}
					curLen -= len(curLine[len(curLine)-1])
					curLine = curLine[:len(curLine)-1]
				}
				if len(lines) > 0 {
					prevLine := strings.TrimRight(lines[len(lines)-1], " \t\n\r\v\f")
					if len(prevLine)+len(w.placeholder) <= w.width {
						lines[len(lines)-1] = prevLine + w.placeholder
						return lines, nil
					}
				}
				lines = append(lines, indent+strings.TrimLeft(w.placeholder, " \t\n\r\v\f"))
				return lines, nil
			}
		}
	}
	return lines, nil
}

// wrapText is CPython's TextWrapper.wrap.
func (w *textWrapper) wrapText(text string) ([]string, error) {
	chunks := w.splitChunks(w.mungeWhitespace(text))
	if w.fixSentenceEndings {
		fixSentenceEndings(chunks)
	}
	return w.wrapChunks(chunks)
}

// ---------------------------------------------------------------------------

func wrapMethod(self py.Object, args py.Tuple) (py.Object, error) {
	w := self.(*textWrapper)
	var text py.Object
	if err := py.UnpackTuple(args, nil, "wrap", 1, 1, &text); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(text)
	if err != nil {
		return nil, err
	}
	lines, err := w.wrapText(s)
	if err != nil {
		return nil, err
	}
	return py.NewListFromStrings(lines), nil
}

func fillMethod(self py.Object, args py.Tuple) (py.Object, error) {
	w := self.(*textWrapper)
	var text py.Object
	if err := py.UnpackTuple(args, nil, "fill", 1, 1, &text); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(text)
	if err != nil {
		return nil, err
	}
	lines, err := w.wrapText(s)
	if err != nil {
		return nil, err
	}
	return py.String(strings.Join(lines, "\n")), nil
}

// ---------------------------------------------------------------------------
// Module-level functions

// wrapperFromKwargs builds a TextWrapper from the module-level helpers'
// keyword arguments.
func wrapperFromKwargs(widthObj py.Object, kwargs py.StringDict) (*textWrapper, error) {
	// Reuse the class constructor so attribute handling lives in one place.
	args := py.Tuple{widthObj}
	kw := py.StringDict{}
	for k, v := range kwargs {
		kw[k] = v
	}
	o, err := textWrapperNew(textWrapperType, args, kw)
	if err != nil {
		return nil, err
	}
	return o.(*textWrapper), nil
}

// moduleArgs splits the text/width positional pair off kwargs, leaving the
// remaining options for the TextWrapper constructor.
func moduleArgs(args py.Tuple, kwargs py.StringDict, name string, needWidth bool) (string, py.Object, py.StringDict, error) {
	if len(args) == 0 || len(args) > 2 {
		return "", nil, nil, py.ExceptionNewf(py.TypeError, "%s() takes 1 or 2 positional arguments (%d given)", name, len(args))
	}
	text, err := py.StrAsString(args[0])
	if err != nil {
		return "", nil, nil, err
	}
	var width py.Object = py.Int(70)
	if len(args) == 2 {
		width = args[1]
	}
	if v, ok := kwargs["width"]; ok {
		if len(args) < 2 {
			width = v
		}
		delete(kwargs, "width")
	}
	if _, ok := kwargs["text"]; ok {
		return "", nil, nil, py.ExceptionNewf(py.TypeError, "%s() got multiple values for argument 'text'", name)
	}
	if !needWidth {
		if len(args) < 2 {
			return "", nil, nil, py.ExceptionNewf(py.TypeError, "%s() missing 1 required positional argument: 'width'", name)
		}
	}
	return text, width, kwargs, nil
}

func moduleWrap(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	text, width, opts, err := moduleArgs(args, kwargs, "wrap", true)
	if err != nil {
		return nil, err
	}
	w, err := wrapperFromKwargs(width, opts)
	if err != nil {
		return nil, err
	}
	lines, err := w.wrapText(text)
	if err != nil {
		return nil, err
	}
	return py.NewListFromStrings(lines), nil
}

func moduleFill(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	text, width, opts, err := moduleArgs(args, kwargs, "fill", true)
	if err != nil {
		return nil, err
	}
	w, err := wrapperFromKwargs(width, opts)
	if err != nil {
		return nil, err
	}
	lines, err := w.wrapText(text)
	if err != nil {
		return nil, err
	}
	return py.String(strings.Join(lines, "\n")), nil
}

func moduleShorten(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	text, width, opts, err := moduleArgs(args, kwargs, "shorten", false)
	if err != nil {
		return nil, err
	}
	w, err := wrapperFromKwargs(width, opts)
	if err != nil {
		return nil, err
	}
	w.hasMaxLines = true
	w.maxLines = 1
	// shorten collapses runs of whitespace before wrapping.
	lines, err := w.wrapText(strings.Join(strings.Fields(text), " "))
	if err != nil {
		return nil, err
	}
	return py.String(strings.Join(lines, "\n")), nil
}

// dedent removes the common leading whitespace from every line.
//
// This follows CPython's implementation: the margin is the length of the
// longest common prefix of the lexicographically smallest and largest
// non-blank lines, restricted to spaces and tabs.  Lines consisting entirely of
// whitespace are normalised to the empty string.
func dedent(self py.Object, args py.Tuple) (py.Object, error) {
	var textObj py.Object
	if err := py.UnpackTuple(args, nil, "dedent", 1, 1, &textObj); err != nil {
		return nil, err
	}
	text, err := py.StrAsString(textObj)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(text, "\n")

	var nonBlank []string
	for _, l := range lines {
		if l != "" && !isAllSpace(l) {
			nonBlank = append(nonBlank, l)
		}
	}
	if len(nonBlank) == 0 {
		// No non-blank lines: every line is whitespace, so every line becomes
		// empty and the joins are all newlines.
		for i, l := range lines {
			if isAllSpace(l) {
				lines[i] = ""
			}
		}
		return py.String(strings.Join(lines, "\n")), nil
	}
	l1, l2 := nonBlank[0], nonBlank[0]
	for _, l := range nonBlank {
		if l < l1 {
			l1 = l
		}
		if l > l2 {
			l2 = l
		}
	}
	margin := 0
	for margin = 0; margin < len(l1); margin++ {
		c := l1[margin]
		if c != l2[margin] || (c != ' ' && c != '\t') {
			break
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if isAllSpace(l) {
			out[i] = ""
		} else if margin <= len(l) {
			out[i] = l[margin:]
		} else {
			out[i] = l
		}
	}
	return py.String(strings.Join(out, "\n")), nil
}

// isAllSpace reports whether s consists only of whitespace characters.  Note
// that Python's str.isspace() is False for the empty string, which is why the
// empty check is separate at the call sites.
func isAllSpace(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isWS(s[i]) {
			return false
		}
	}
	return true
}

// splitLinesKeepEnds implements str.splitlines(keepends=True) for the ASCII
// line boundaries.
func splitLinesKeepEnds(s string) []string {
	var out []string
	start := 0
	i := 0
	for i < len(s) {
		c := s[i]
		var width int
		switch c {
		case '\n', '\v', '\f', '\r':
			width = 1
			if c == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				width = 2
			}
		}
		if width > 0 {
			out = append(out, s[start:i+width])
			i += width
			start = i
			continue
		}
		i++
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func indent(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// indent(text, prefix, predicate=None) - predicate may be positional or
	// keyword, so both are handled rather than leaving it to UnpackTuple.
	if len(args) < 2 || len(args) > 3 {
		return nil, py.ExceptionNewf(py.TypeError, "indent() takes 2 or 3 positional arguments (%d given)", len(args))
	}
	text, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	prefix, err := py.StrAsString(args[1])
	if err != nil {
		return nil, err
	}
	var predicate py.Object = py.None
	if len(args) == 3 {
		predicate = args[2]
	}
	if p, ok := kwargs["predicate"]; ok {
		if len(args) == 3 {
			return nil, py.ExceptionNewf(py.TypeError, "indent() got multiple values for argument 'predicate'")
		}
		predicate = p
	}
	var b strings.Builder
	for _, line := range splitLinesKeepEnds(text) {
		// CPython uses `not line.isspace()`: an empty string is not space, but
		// splitlines never yields an empty line.
		use := !isAllSpace(line)
		if predicate != py.None {
			r, err := py.Call(predicate, py.Tuple{py.String(line)}, nil)
			if err != nil {
				return nil, err
			}
			if use, err = py.ObjectIsTrue(r); err != nil {
				return nil, err
			}
		}
		if use {
			b.WriteString(prefix)
		}
		b.WriteString(line)
	}
	return py.String(b.String()), nil
}

// ---------------------------------------------------------------------------

// attribute names, in the order CPython's TextWrapper documents them.
var wrapperAttrs = []struct {
	name  string
	isInt bool
}{
	{"width", true},
	{"initial_indent", false},
	{"subsequent_indent", false},
	{"expand_tabs", false},
	{"replace_whitespace", false},
	{"fix_sentence_endings", false},
	{"break_long_words", false},
	{"drop_whitespace", false},
	{"break_on_hyphens", false},
	{"tabsize", true},
	{"max_lines", true},
	{"placeholder", false},
}

func init() {
	textWrapperType.Dict["wrap"] = py.MustNewMethod("wrap", wrapMethod, 0,
		"wrap(text) -> [lines]\nReformat the single paragraph in 'text' so it fits in lines of no\nmore than 'self.width' columns, and return a list of wrapped lines.")
	textWrapperType.Dict["fill"] = py.MustNewMethod("fill", fillMethod, 0,
		"fill(text) -> str\nReformat the single paragraph in 'text' to fit in lines of no more\nthan 'self.width' columns, and return a new string containing the\nentire wrapped paragraph.")

	// The attributes are exposed as properties so a script can read and write
	// them the way CPython allows.
	for _, attr := range wrapperAttrs {
		name := attr.name
		isInt := attr.isInt
		if name == "max_lines" {
			textWrapperType.Dict[name] = &py.Property{
				Fget: func(self py.Object) (py.Object, error) {
					w := self.(*textWrapper)
					if !w.hasMaxLines {
						return py.None, nil
					}
					return py.Int(w.maxLines), nil
				},
				Fset: func(self, value py.Object) error {
					w := self.(*textWrapper)
					if value == py.None {
						w.hasMaxLines = false
						w.maxLines = 0
						return nil
					}
					n, err := py.IndexInt(value)
					if err != nil {
						return err
					}
					w.hasMaxLines = true
					w.maxLines = n
					return nil
				},
			}
			continue
		}
		if isInt {
			textWrapperType.Dict[name] = &py.Property{
				Fget: func(self py.Object) (py.Object, error) {
					w := self.(*textWrapper)
					switch name {
					case "width":
						return py.Int(w.width), nil
					case "tabsize":
						return py.Int(w.tabsize), nil
					}
					return py.None, nil
				},
				Fset: func(self, value py.Object) error {
					w := self.(*textWrapper)
					n, err := py.IndexInt(value)
					if err != nil {
						return err
					}
					switch name {
					case "width":
						w.width = n
					case "tabsize":
						w.tabsize = n
					}
					return nil
				},
			}
			continue
		}
		// bool or string attribute
		textWrapperType.Dict[name] = &py.Property{
			Fget: func(self py.Object) (py.Object, error) {
				w := self.(*textWrapper)
				switch name {
				case "initial_indent":
					return py.String(w.initialIndent), nil
				case "subsequent_indent":
					return py.String(w.subsequentIndent), nil
				case "placeholder":
					return py.String(w.placeholder), nil
				}
				var b bool
				switch name {
				case "expand_tabs":
					b = w.expandTabs
				case "replace_whitespace":
					b = w.replaceWhitespace
				case "fix_sentence_endings":
					b = w.fixSentenceEndings
				case "break_long_words":
					b = w.breakLongWords
				case "drop_whitespace":
					b = w.dropWhitespace
				case "break_on_hyphens":
					b = w.breakOnHyphens
				}
				return py.NewBool(b), nil
			},
			Fset: func(self, value py.Object) error {
				w := self.(*textWrapper)
				switch name {
				case "initial_indent", "subsequent_indent", "placeholder":
					s, err := py.StrAsString(value)
					if err != nil {
						return err
					}
					switch name {
					case "initial_indent":
						w.initialIndent = s
					case "subsequent_indent":
						w.subsequentIndent = s
					default:
						w.placeholder = s
					}
					return nil
				}
				b, err := py.ObjectIsTrue(value)
				if err != nil {
					return err
				}
				switch name {
				case "expand_tabs":
					w.expandTabs = b
				case "replace_whitespace":
					w.replaceWhitespace = b
				case "fix_sentence_endings":
					w.fixSentenceEndings = b
				case "break_long_words":
					w.breakLongWords = b
				case "drop_whitespace":
					w.dropWhitespace = b
				case "break_on_hyphens":
					w.breakOnHyphens = b
				}
				return nil
			},
		}
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "textwrap",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("wrap", moduleWrap, 0, "wrap(text, width=70, **kwargs) -> [str]\nWrap a single paragraph of text, returning a list of wrapped lines."),
			py.MustNewMethod("fill", moduleFill, 0, "fill(text, width=70, **kwargs) -> str\nFill a single paragraph of text, returning a new string."),
			py.MustNewMethod("shorten", moduleShorten, 0, "shorten(text, width, **kwargs) -> str\nCollapse and truncate the given text to fit in the given width."),
			py.MustNewMethod("dedent", dedent, 0, "dedent(text) -> str\nRemove any common leading whitespace from every line in `text`."),
			py.MustNewMethod("indent", indent, 0, "indent(text, prefix, predicate=None) -> str\nAdds 'prefix' to the beginning of selected lines in 'text'."),
		},
		Globals: py.StringDict{
			"TextWrapper": textWrapperType,
		},
	})
}
