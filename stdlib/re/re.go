// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package re provides the implementation of python's 're' module.
//
// It is built on Go's regexp package, with the differences bridged rather
// than ignored.  Three of them matter enough to be worth naming:
//
//   - Go's regexp works in bytes, Python's in CHARACTERS.  Every offset this
//     module returns (start, end, span) is converted to a character index,
//     because a byte offset is wrong for any non-ascii text.
//
//   - Go's MatchString is unanchored, Python's match() is anchored and
//     fullmatch() requires the whole string.  Those are implemented by
//     wrapping the pattern.
//
//   - Go's FindAll has its own rule about empty matches.  The scanning here
//     is done by hand so that an empty match advances the way Python's does.
//
// What is NOT supported raises re.error rather than mismatching: back
// references and lookaround, which Go's RE2 engine does not have.
package re

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Support for regular expressions (RE).

This module provides regular expression matching operations.  Both the
pattern and the string can be either strings or bytes, though only string
patterns are supported here.

The supported syntax is that of Go's RE2 engine, which means backreferences
and lookaround assertions are not available; using one raises re.error
rather than silently mismatching.`

var ErrorType = py.ExceptionType.NewType("re.error", "Exception raised for invalid regular expressions.", nil, nil)

// The flags, with CPython's values.
const (
	FlagASCII      = 256
	FlagIGNORECASE = 2
	FlagLOCALE     = 4
	FlagMULTILINE  = 8
	FlagDOTALL     = 16
	FlagUNICODE    = 32
	FlagVERBOSE    = 64
	FlagDEBUG      = 128
)

var flagsOnce = false

// Pattern is a compiled regular expression.
type Pattern struct {
	source string
	flags  int
	re     *regexp.Regexp
	// groupNames maps a group number to its name, and the name to its
	// number, as parsed from the pattern.
	groupNames map[int]string
	nameToNum  map[string]int
	ngroups    int
	// lifted carries the pattern with any negative lookahead REMOVED and the
	// assertions to re-check, or nil when the pattern had none - which is the
	// overwhelmingly common case and skips every check below.
	lifted *lifted
}

var PatternType = py.NewTypeX("re.Pattern", "A compiled regular expression.", nil, nil)

func (p *Pattern) Type() *py.Type { return PatternType }

// Match is one successful match.
type Match struct {
	pattern *Pattern
	text    string
	locs    []int // character offsets, two per group, -1 for a non-participating group
	re      *py.Object
}

var MatchType = py.NewTypeX("re.Match", "The result of a successful match.", nil, nil)

func (m *Match) Type() *py.Type { return MatchType }

func init() {
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "error", Value: ErrorType},
		py.DictEntry{Key: "PatternError", Value: ErrorType},
		py.DictEntry{Key: "compile", Value: py.MustNewMethod("compile", compileFn, 0, "Compile a regular expression pattern, returning a Pattern object.")},
		py.DictEntry{Key: "match", Value: py.MustNewMethod("match", matchFn, 0, "Try to apply the pattern at the start of the string.")},
		py.DictEntry{Key: "search", Value: py.MustNewMethod("search", searchFn, 0, "Scan through a string, looking for a match.")},
		py.DictEntry{Key: "fullmatch", Value: py.MustNewMethod("fullmatch", fullMatchFn, 0, "Try to apply the pattern to all of the string.")},
		py.DictEntry{Key: "findall", Value: py.MustNewMethod("findall", findAllFn, 0, "Return a list of all non-overlapping matches.")},
		py.DictEntry{Key: "finditer", Value: py.MustNewMethod("finditer", findIterFn, 0, "Return an iterator over all non-overlapping matches.")},
		py.DictEntry{Key: "split", Value: py.MustNewMethod("split", splitFn, 0, "Split the source string by the occurrences of the pattern.")},
		py.DictEntry{Key: "sub", Value: py.MustNewMethod("sub", subFn, 0, "Return the string with the matches replaced.")},
		py.DictEntry{Key: "subn", Value: py.MustNewMethod("subn", subNFn, 0, "Return (new_string, number_of_subs).")},
		py.DictEntry{Key: "escape", Value: py.MustNewMethod("escape", escapeFn, 0, "Escape special characters in a string.")},
		py.DictEntry{Key: "purge", Value: py.MustNewMethod("purge", purgeFn, 0, "Clear the regular expression cache.")},
		py.DictEntry{Key: "Pattern", Value: PatternType},
		py.DictEntry{Key: "Match", Value: MatchType},
		py.DictEntry{Key: "ASCII", Value: py.Int(FlagASCII)},
		py.DictEntry{Key: "IGNORECASE", Value: py.Int(FlagIGNORECASE)},
		py.DictEntry{Key: "I", Value: py.Int(FlagIGNORECASE)},
		py.DictEntry{Key: "LOCALE", Value: py.Int(FlagLOCALE)},
		py.DictEntry{Key: "L", Value: py.Int(FlagLOCALE)},
		py.DictEntry{Key: "MULTILINE", Value: py.Int(FlagMULTILINE)},
		py.DictEntry{Key: "M", Value: py.Int(FlagMULTILINE)},
		py.DictEntry{Key: "DOTALL", Value: py.Int(FlagDOTALL)},
		py.DictEntry{Key: "S", Value: py.Int(FlagDOTALL)},
		py.DictEntry{Key: "UNICODE", Value: py.Int(FlagUNICODE)},
		py.DictEntry{Key: "U", Value: py.Int(FlagUNICODE)},
		py.DictEntry{Key: "VERBOSE", Value: py.Int(FlagVERBOSE)},
		py.DictEntry{Key: "X", Value: py.Int(FlagVERBOSE)},
		py.DictEntry{Key: "DEBUG", Value: py.Int(FlagDEBUG)},
		py.DictEntry{Key: "NOFLAG", Value: py.Int(0)},
	)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "re",
			Doc:  module_doc,
		},
		Globals: globals,
	})
	_ = flagsOnce
}

func purgeFn(self py.Object, args py.Tuple) (py.Object, error) { return py.None, nil }

// translate rewrites the Python pattern into the Go one and reports the
// group numbering, which differs because the translation can add groups.
func translate(pattern string, flags int) (string, map[int]string, map[string]int, int, error) {
	// VERBOSE is not a flag Go's engine has, so it is applied here: unescaped
	// whitespace outside a character class is dropped and an unescaped '#'
	// starts a comment that runs to the end of the line.  This has to happen
	// before anything else looks at the pattern.
	if flags&FlagVERBOSE != 0 {
		pattern = stripVerbose(pattern)
	}
	// A leading inline (?x) or (?ix) turns VERBOSE on for the whole pattern,
	// so its effect has to be applied here too, not merely dropped by the
	// group rewriting below.  Only the leading position is handled: an inline
	// group in the middle would have to take effect from that point on, and
	// the scan that follows is positional, so re-running it there would be a
	// second pass over the same string.
	pattern = applyLeadingVerbose(pattern)
	// Possessive quantifiers (Python 3.11's "a*+") have no spelling in Go's
	// engine.  Go's RE2 does not backtrack, so the greedy quantifier it is
	// rewritten to accepts the same language; only a pattern that would rely
	// on atomic-group rejection at a specific offset could differ, and there
	// is no such construct in Python's grammar without a lookaround, which
	// this module rejects outright.
	pattern = stripPossessive(pattern)
	// Backreferences and lookaround have no equivalent: say so.
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '\\' {
			continue
		}
		if i+1 >= len(pattern) {
			break
		}
		next := pattern[i+1]
		if next >= '1' && next <= '9' {
			return "", nil, nil, 0, py.ExceptionNewf(ErrorType, "backreferences are not supported: Go's regexp engine has no equivalent")
		}
		i++
	}
	// Lookbehind, conditionals and backreferences still have no equivalent in
	// Go's engine and are refused by name.  NEGATIVE lookahead is handled now -
	// see lookahead.go - because pip cannot start without it.
	for _, unsupported := range []string{"(?<=", "(?<!", "(?(", "(?P="} {
		if strings.Contains(pattern, unsupported) {
			return "", nil, nil, 0, py.ExceptionNewf(ErrorType, "lookaround and conditionals are not supported: Go's regexp engine has no equivalent (%s)", unsupported)
		}
	}

	var b strings.Builder
	groupNames := map[int]string{}
	nameToNum := map[string]int{}
	groupNum := 0
	inClass := false

	// Class translation depends on the ASCII flag.  Each shorthand has a
	// negated twin; the negated forms must be written as their own class,
	// never by re-emitting the pattern seen so far.
	//
	// Each also has an "inner" spelling, used inside an enclosing character
	// class: there the shorthand's own brackets must be dropped, or "[\d\s]"
	// becomes the nested "[[0-9][\t\n...]]" and matches the wrong thing.
	digit := `\p{Nd}`
	digitInner := `\p{Nd}`
	notDigit := `\P{Nd}`
	word := `[\p{L}\p{N}\p{Pc}]`
	wordInner := `\p{L}\p{N}\p{Pc}`
	notWord := `[^\p{L}\p{N}\p{Pc}]`
	space := `[\t\n\v\f\r \x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`
	spaceInner := `\t\n\v\f\r \x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}`
	notSpace := `[^\t\n\v\f\r \x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`
	if flags&FlagASCII != 0 {
		digit = `[0-9]`
		digitInner = `0-9`
		notDigit = `[^0-9]`
		word = `[0-9A-Za-z_]`
		wordInner = `0-9A-Za-z_`
		notWord = `[^0-9A-Za-z_]`
		space = `[\t\n\v\f\r ]`
		spaceInner = `\t\n\v\f\r `
		notSpace = `[^\t\n\v\f\r ]`
	}

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		// A character class is literal: anchors, groups and classes inside
		// "[...]" are just characters, so the translation of "$", "(" and
		// friends must not run there.  inClass tracks that, and a ']' as the
		// first character of a class is a literal, not the end.
		if c == '[' && !inClass {
			inClass = true
			classStart := i
			b.WriteByte(c)
			i++
			if i < len(pattern) && pattern[i] == '^' {
				b.WriteByte('^')
				i++
			}
			if i < len(pattern) && pattern[i] == ']' {
				b.WriteByte(']')
				i++
			}
			// Copy the class body, translating escapes but not metacharacters.
			// The loop test has to skip an escaped '\]' - it is a literal in the
			// class, not the closing bracket, and stopping at it would end the
			// class early.
			for i < len(pattern) {
				ic := pattern[i]
				if ic == '\\' && i+1 < len(pattern) {
					nnext := pattern[i+1]
					switch nnext {
					case 'd':
						b.WriteString(digitInner)
					case 'w':
						b.WriteString(wordInner)
					case 's':
						b.WriteString(spaceInner)
					case 'D':
						b.WriteString(`\D`)
					case 'W':
						b.WriteString(`\W`)
					case 'S':
						b.WriteString(`\S`)
					default:
						b.WriteByte('\\')
						b.WriteByte(nnext)
					}
					// The escape consumes BOTH the backslash and the escaped
					// character, so advance past both.
					i += 2
					continue
				}
				if ic == ']' {
					break
				}
				b.WriteByte(ic)
				i++
			}
			_ = classStart
			if i < len(pattern) {
				b.WriteByte(']')
			}
			inClass = false
			continue
		}
		if c == '\\' && i+1 < len(pattern) {
			next := pattern[i+1]
			switch next {
			case 'd':
				b.WriteString(digit)
			case 'D':
				b.WriteString(notDigit)
			case 'w':
				b.WriteString(word)
			case 'W':
				b.WriteString(notWord)
			case 's':
				b.WriteString(space)
			case 'S':
				b.WriteString(notSpace)
			case 'Z':
				// Python's \Z is Go's \z.
				b.WriteString(`\z`)
			case 'A', 'b', 'B':
				b.WriteByte(next)
			case 'u', 'U', 'N':
				return "", nil, nil, 0, py.ExceptionNewf(ErrorType, `\%c escapes are not supported`, next)
			default:
				b.WriteByte('\\')
				b.WriteByte(next)
			}
			i++
			continue
		}
		if c == '(' {
			// A named group: (?P<name>...) is the same in Go, but the
			// numbering has to be tracked here.
			if strings.HasPrefix(pattern[i:], "(?P<") {
				end := strings.IndexByte(pattern[i:], '>')
				if end > 0 {
					name := pattern[i+4 : i+end]
					groupNum++
					groupNames[groupNum] = name
					nameToNum[name] = groupNum
				}
			} else if strings.HasPrefix(pattern[i:], "(?#") {
				// A comment group runs to the next ')'; Go has no such form,
				// and the content is dropped.
				end := strings.IndexByte(pattern[i:], ')')
				if end < 0 {
					return "", nil, nil, 0, py.ExceptionNewf(ErrorType, "missing ), unterminated comment")
				}
				i += end
				continue
			} else if flag, ok := pythonFlagGroup(pattern[i:]); ok {
				// (?a), (?x) and friends: forms Python accepts and Go does not.
				// The letters are rewritten into what Go accepts and the rest of
				// the group - the ')' of an inline form, or the ':' and body of
				// a scoped (?:...) form - is copied unchanged.
				//
				//   (?a) and (?L) constrain \d \w \s to ASCII, which is what a
				//   dropped letter means, since the ASCII classes are written
				//   explicitly by the character-class translation.
				//   (?x) is VERBOSE, which Go spells x.
				//   (?u) is the default in Python 3, so it is dropped.
				goFlagLetters := flag.goFlags()
				if flag.scoped {
					// A scoped group (?flags:...).  The ':' is part of the
					// rewritten head, so the loop resumes at the first byte of
					// the body.  An empty letter set still leaves the group
					// itself, as a non-capturing "(?:...)".
					b.WriteString("(?" + goFlagLetters + ":")
					i += flag.head
					continue
				}
				if goFlagLetters != "" {
					b.WriteString("(?" + goFlagLetters + ")")
				}
				// With no letters left the whole group is a no-op; dropping it
				// outright avoids the phantom capture group "()" would add.
				i += flag.head
				continue
			} else if i+1 < len(pattern) && pattern[i+1] != '?' {
				groupNum++
			}
		}
		// Python's $ matches at the end and just before a trailing newline;
		// Go's $ is end-of-text only.
		if c == '$' && flags&FlagMULTILINE == 0 {
			b.WriteString(`(?:\n?\z)`)
			continue
		}
		b.WriteByte(c)
	}
	return b.String(), groupNames, nameToNum, groupNum, nil
}

// applyLeadingVerbose handles a leading inline (?x) / (?ix) group: Python's
// VERBOSE takes effect from there to the end of the pattern.
func applyLeadingVerbose(pattern string) string {
	flag, ok := pythonFlagGroup(pattern)
	if !ok || flag.scoped || !strings.ContainsRune(flag.letters, 'x') {
		return pattern
	}
	return pattern[:2] + pattern[2:flag.head] + stripVerbose(pattern[flag.head:])
}

// stripPossessive rewrites a Python 3.11 possessive quantifier ("x*+", "x++",
// "x?+", "x{m,n}+") into the greedy form Go understands, by dropping the
// trailing '+'.  Escaped characters and character classes are copied verbatim.
func stripPossessive(pattern string) string {
	var b strings.Builder
	inClass := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			b.WriteByte(c)
			b.WriteByte(pattern[i+1])
			i++
			continue
		}
		if inClass {
			if c == ']' {
				inClass = false
			}
			b.WriteByte(c)
			continue
		}
		if c == '[' {
			inClass = true
			b.WriteByte(c)
			continue
		}
		// A '+', '*', '?' or '}' followed by '+' is a possessive quantifier.
		if i+1 < len(pattern) && pattern[i+1] == '+' && (c == '*' || c == '+' || c == '?' || c == '}') {
			b.WriteByte(c)
			i++
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// stripVerbose applies Python's VERBOSE (re.X) preprocessing: whitespace that
// is not escaped and not inside a character class is removed, and an unescaped
// '#' begins a comment running to the end of the line.  A backslash escapes the
// next character, and both the escape and the escaped character are kept.
func stripVerbose(pattern string) string {
	var b strings.Builder
	inClass := false
	for i := 0; i < len(pattern); {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			b.WriteByte(c)
			b.WriteByte(pattern[i+1])
			i += 2
			continue
		}
		if inClass {
			if c == ']' {
				inClass = false
			}
			b.WriteByte(c)
			i++
			continue
		}
		switch c {
		case '[':
			inClass = true
			b.WriteByte(c)
			i++
		case '#':
			for i < len(pattern) && pattern[i] != '\n' {
				i++
			}
		case ' ', '\t', '\n', '\r', '\v', '\f':
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// pythonOnlyFlag reports whether the flag letters contain one Go does not
// accept, which is what makes a group need rewriting.
func pythonOnlyFlag(letters string) bool {
	for _, r := range letters {
		switch r {
		case 'a', 'L', 'u', 'x':
			return true
		}
	}
	return false
}

// pythonFlagGroup recognises a Python inline-flag group, "(?<letters>" or
// "(?<letters>:", at the start of s, and returns the position just past the
// head.  Go accepts the letters i, m, s and U; Python's a, L, u and x are not
// Go's, so they are folded away here (a and L select ASCII \d\w\s, u is the
// default, and x is Go's own x).
func pythonFlagGroup(s string) (flagGroup, bool) {
	if !strings.HasPrefix(s, "(?") {
		return flagGroup{}, false
	}
	i := 2
	var letters strings.Builder
	for i < len(s) && s[i] >= 'a' && s[i] <= 'z' || i < len(s) && s[i] >= 'A' && s[i] <= 'Z' {
		letters.WriteByte(s[i])
		i++
	}
	if letters.Len() == 0 {
		return flagGroup{}, false
	}
	scoped := i < len(s) && s[i] == ':'
	if !scoped && (i >= len(s) || s[i] != ')') {
		return flagGroup{}, false
	}
	if !pythonOnlyFlag(letters.String()) {
		return flagGroup{}, false
	}
	return flagGroup{letters: letters.String(), head: i, scoped: scoped}, true
}

// flagGroup is one inline flag group: its letters, the offset of the byte after
// them (the ')' or ':'), and whether a ':' follows.
type flagGroup struct {
	letters string
	head    int
	scoped  bool
}

// goFlags is the letters of this group that Go understands.  Go's RE2 engine
// takes i, m, s and U; Python's VERBOSE x is NOT one of them, so it is dropped
// here too (see the note on inline x in translate).
func (f flagGroup) goFlags() string {
	var out []rune
	for _, r := range f.letters {
		switch r {
		case 'i', 'm', 's', 'U':
			out = append(out, r)
		}
	}
	return string(out)
}

// goFlags turns the Python flags into the Go inline ones.
func goFlags(flags int) string {
	out := ""
	if flags&FlagIGNORECASE != 0 {
		out += "i"
	}
	if flags&FlagMULTILINE != 0 {
		out += "m"
	}
	if flags&FlagDOTALL != 0 {
		out += "s"
	}
	return out
}

// compile builds a Pattern, applying the flag prefixes.
func compile(pattern string, flags int) (*Pattern, error) {
	translated, names, nameToNum, ngroups, err := translate(pattern, flags)
	if err != nil {
		return nil, err
	}
	// Negative lookahead is lifted out before Go sees the pattern: the engine
	// compiles the pattern with the assertions removed, and the match paths
	// re-check them.  A pattern with none takes the identical old path.
	var liftedForm *lifted
	if hasLookahead(translated) {
		l, lerr := lift(translated)
		if lerr != nil {
			return nil, lerr
		}
		translated = l.pattern
		liftedForm = &l
	}
	if prefix := goFlags(flags); prefix != "" {
		translated = "(?" + prefix + ")" + translated
	}
	compiled, err := regexp.Compile(translated)
	if err != nil {
		return nil, py.ExceptionNewf(ErrorType, "%s", cleanErr(err))
	}

	p := &Pattern{
		source:     pattern,
		flags:      flags,
		re:         compiled,
		groupNames: names,
		nameToNum:  nameToNum,
		ngroups:    ngroups,
		lifted:     liftedForm,
	}
	if flags&FlagASCII == 0 {
		// A str pattern is Unicode by default, which is what flags reports.
		p.flags |= FlagUNICODE
	}
	return p, nil
}

// cleanErr strips Go's "error parsing regexp:" prefix.
func cleanErr(err error) string {
	text := err.Error()
	text = strings.TrimPrefix(text, "error parsing regexp: ")
	return text
}

// charOffset converts a byte offset in text to a character offset.
func charOffset(text string, byteOffset int) int {
	if byteOffset <= 0 {
		return byteOffset
	}
	return len([]rune(text[:byteOffset]))
}

// byteOffset converts a character offset back to a byte offset.
func byteOffset(text string, charIndex int) int {
	if charIndex <= 0 {
		return 0
	}
	count := 0
	for i := range text {
		if count == charIndex {
			return i
		}
		count++
	}
	return len(text)
}

// buildMatch turns Go's index array into a Match, in character offsets.
func (p *Pattern) buildMatch(text string, idx []int) *Match {
	locs := make([]int, 2*(p.ngroups+1))
	for g := 0; g <= p.ngroups; g++ {
		start, end := -1, -1
		if 2*g+1 < len(idx) && idx[2*g] >= 0 {
			start = charOffset(text, idx[2*g])
			end = charOffset(text, idx[2*g+1])
		}
		locs[2*g] = start
		locs[2*g+1] = end
	}
	return &Match{pattern: p, text: text, locs: locs}
}

// groupIndex resolves a group reference: a number or a name.
func (m *Match) groupIndex(key py.Object) (int, error) {
	switch v := key.(type) {
	case py.Int:
		i, err := v.GoInt()
		if err != nil {
			return 0, err
		}
		if i < 0 || i > m.pattern.ngroups {
			return 0, py.ExceptionNewf(py.IndexError, "no such group")
		}
		return i, nil
	case py.String:
		if n, ok := m.pattern.nameToNum[string(v)]; ok {
			return n, nil
		}
		return 0, py.ExceptionNewf(py.IndexError, "no such group")
	}
	return 0, py.ExceptionNewf(py.IndexError, "no such group")
}

// groupText returns the text of a group, or None when it did not
// participate.
func (m *Match) groupText(i int) py.Object {
	start, end := m.locs[2*i], m.locs[2*i+1]
	if start < 0 || end < 0 {
		return py.None
	}
	runes := []rune(m.text)
	return py.String(string(runes[start:end]))
}

func init() {
	PatternType.Dict.Set("pattern", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.String(self.(*Pattern).source), nil
	}})
	PatternType.Dict.Set("flags", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*Pattern).flags), nil
	}})
	PatternType.Dict.Set("groups", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*Pattern).ngroups), nil
	}})
	PatternType.Dict.Set("groupindex", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		d := py.NewStringDict()
		for num, name := range self.(*Pattern).groupNames {
			d.Set(name, py.Int(num))
		}
		return d, nil
	}})
	PatternType.Dict.Set("__repr__", py.MustNewMethod("__repr__", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*Pattern)
		quoted, _ := py.ReprAsString(py.String(p.source))
		return py.String("re.compile(" + quoted + ")"), nil
	}, 0, "Return repr(self)."))

	patternMethod := func(name string, find func(*Pattern, string, int) *Match) *py.Method {
		return py.MustNewMethod(name, func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			p := self.(*Pattern)
			text, pos, endpos, err := patternArgs(name, args, kwargs)
			if err != nil {
				return nil, err
			}
			m := find(p, text, pos)
			if m == nil || m.locs[0] < 0 {
				return py.None, nil
			}
			_ = endpos
			return m, nil
		}, 0, name)
	}
	PatternType.Dict.Set("match", patternMethod("match", matchAt))
	PatternType.Dict.Set("search", patternMethod("search", searchIn))
	PatternType.Dict.Set("fullmatch", patternMethod("fullmatch", fullMatchAt))

	PatternType.Dict.Set("findall", py.MustNewMethod("findall", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p := self.(*Pattern)
		text, _, _, err := patternArgs("findall", args, kwargs)
		if err != nil {
			return nil, err
		}
		return p.findAll(text)
	}, 0, "Return a list of all non-overlapping matches."))

	PatternType.Dict.Set("finditer", py.MustNewMethod("finditer", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p := self.(*Pattern)
		text, _, _, err := patternArgs("finditer", args, kwargs)
		if err != nil {
			return nil, err
		}
		items, err := p.matchList(text)
		if err != nil {
			return nil, err
		}
		return py.NewIterator(items), nil
	}, 0, "Return an iterator over all non-overlapping matches."))

	PatternType.Dict.Set("split", py.MustNewMethod("split", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p := self.(*Pattern)
		text, _, _, err := patternArgs("split", args, kwargs)
		if err != nil {
			return nil, err
		}
		maxsplit := 0
		if v, ok := kwargs.Get("maxsplit"); ok {
			n, err := py.IndexInt(v)
			if err != nil {
				return nil, err
			}
			maxsplit = n
		}
		return p.split(text, maxsplit)
	}, 0, "Split the source string by the occurrences of the pattern."))

	PatternType.Dict.Set("sub", py.MustNewMethod("sub", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		res, err := subWith(self.(*Pattern), args, kwargs, false)
		return res, err
	}, 0, "Return the string with the matches replaced."))
	PatternType.Dict.Set("subn", py.MustNewMethod("subn", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return subWith(self.(*Pattern), args, kwargs, true)
	}, 0, "Return (new_string, number_of_subs)."))

	// The Match object.
	MatchType.Dict.Set("group", py.MustNewMethod("group", func(self py.Object, args py.Tuple) (py.Object, error) {
		m := self.(*Match)
		if len(args) == 0 {
			return m.groupText(0), nil
		}
		if len(args) == 1 {
			idx, err := m.groupIndex(args[0])
			if err != nil {
				return nil, err
			}
			return m.groupText(idx), nil
		}
		out := make(py.Tuple, len(args))
		for i, a := range args {
			idx, err := m.groupIndex(a)
			if err != nil {
				return nil, err
			}
			out[i] = m.groupText(idx)
		}
		return out, nil
	}, 0, "Return one or more subgroups of the match."))

	MatchType.Dict.Set("groups", py.MustNewMethod("groups", func(self py.Object, args py.Tuple) (py.Object, error) {
		m := self.(*Match)
		def := py.Object(py.None)
		if len(args) > 0 {
			def = args[0]
		}
		out := make(py.Tuple, m.pattern.ngroups)
		for g := 1; g <= m.pattern.ngroups; g++ {
			v := m.groupText(g)
			if v == py.None {
				v = def
			}
			out[g-1] = v
		}
		return out, nil
	}, 0, "Return a tuple of all the subgroups."))

	MatchType.Dict.Set("groupdict", py.MustNewMethod("groupdict", func(self py.Object, args py.Tuple) (py.Object, error) {
		m := self.(*Match)
		def := py.Object(py.None)
		if len(args) > 0 {
			def = args[0]
		}
		out := py.NewStringDict()
		for num, name := range m.pattern.groupNames {
			v := m.groupText(num)
			if v == py.None {
				v = def
			}
			out.Set(name, v)
		}
		return out, nil
	}, 0, "Return a dict of the named subgroups."))

	MatchType.Dict.Set("start", py.MustNewMethod("start", func(self py.Object, args py.Tuple) (py.Object, error) {
		m := self.(*Match)
		g := 0
		if len(args) > 0 {
			idx, err := m.groupIndex(args[0])
			if err != nil {
				return nil, err
			}
			g = idx
		}
		return py.Int(m.locs[2*g]), nil
	}, 0, "Return the start position of the match."))
	MatchType.Dict.Set("end", py.MustNewMethod("end", func(self py.Object, args py.Tuple) (py.Object, error) {
		m := self.(*Match)
		g := 0
		if len(args) > 0 {
			idx, err := m.groupIndex(args[0])
			if err != nil {
				return nil, err
			}
			g = idx
		}
		return py.Int(m.locs[2*g+1]), nil
	}, 0, "Return the end position of the match."))
	MatchType.Dict.Set("span", py.MustNewMethod("span", func(self py.Object, args py.Tuple) (py.Object, error) {
		m := self.(*Match)
		g := 0
		if len(args) > 0 {
			idx, err := m.groupIndex(args[0])
			if err != nil {
				return nil, err
			}
			g = idx
		}
		return py.Tuple{py.Int(m.locs[2*g]), py.Int(m.locs[2*g+1])}, nil
	}, 0, "Return the (start, end) positions of the match."))

	MatchType.Dict.Set("string", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.String(self.(*Match).text), nil
	}})
	MatchType.Dict.Set("re", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return self.(*Match).pattern, nil
	}})
	MatchType.Dict.Set("pos", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(0), nil
	}})
	MatchType.Dict.Set("endpos", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(len([]rune(self.(*Match).text))), nil
	}})
	MatchType.Dict.Set("lastindex", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		m := self.(*Match)
		last := -1
		for g := 1; g <= m.pattern.ngroups; g++ {
			if m.locs[2*g] >= 0 {
				last = g
			}
		}
		if last < 0 {
			// A match with no groups reports 1, as CPython does.
			if m.pattern.ngroups == 0 {
				return py.Int(1), nil
			}
			return py.None, nil
		}
		return py.Int(last), nil
	}})
	MatchType.Dict.Set("lastgroup", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		m := self.(*Match)
		last := -1
		for g := 1; g <= m.pattern.ngroups; g++ {
			if m.locs[2*g] >= 0 {
				last = g
			}
		}
		if name, ok := m.pattern.groupNames[last]; ok {
			return py.String(name), nil
		}
		return py.None, nil
	}})
}

// matchAt anchors the pattern at the start.
func matchAt(p *Pattern, text string, pos int) *Match {
	sub := text[byteOffset(text, pos):]
	// match() is anchored at the start but does NOT need to consume the
	// whole string, so the "whole" flag is false here.  Passing true made
	// match("hello world") return None while match("hello") worked, which
	// is exactly the kind of difference a short test hides.
	idx := p.anchoredMatch(sub, false)
	if idx == nil {
		return nil
	}
	return p.matchFromIndices(text, idx, byteOffset(text, pos))
}

// searchIn finds the first match anywhere from pos.
func searchIn(p *Pattern, text string, pos int) *Match {
	sub := text[byteOffset(text, pos):]
	var idx []int
	if p.lifted != nil {
		// The pattern carried a negative lookahead.  Candidates whose
		// assertions fail are skipped rather than ending the search.
		idx = p.matchWithAssertions(*p.lifted, sub, false)
	} else {
		idx = p.re.FindStringSubmatchIndex(sub)
	}
	if idx == nil {
		return nil
	}
	return p.matchFromIndices(text, idx, byteOffset(text, pos))
}

// fullMatchAt requires the whole (remaining) string to match.
func fullMatchAt(p *Pattern, text string, pos int) *Match {
	sub := text[byteOffset(text, pos):]
	// fullmatch() is anchored AND must consume the whole string.
	idx := p.anchoredMatch(sub, true)
	if idx == nil {
		return nil
	}
	return p.matchFromIndices(text, idx, byteOffset(text, pos))
}

// matchFromIndices builds a Match from a byte-index array that is relative
// to an offset into the text.  Both the shift and the character conversion
// happen here, which is the one place that knows both.
func (p *Pattern) matchFromIndices(text string, idx []int, base int) *Match {
	shifted := make([]int, len(idx))
	for i, v := range idx {
		if v < 0 {
			shifted[i] = -1
		} else {
			shifted[i] = v + base
		}
	}
	return p.buildMatch(text, shifted)
}

// anchoredMatch matches with the pattern anchored at the start; when whole
// is false the match must also consume the entire string.
//
// The result is a byte-index array in the same shape as the original
// pattern's, so the callers can treat it exactly as they treat a search
// result - including a nil entry for a group that did not participate.
func (p *Pattern) anchoredMatch(text string, whole bool) []int {
	// The pattern is wrapped rather than recompiled, so the group numbering
	// of the result is unchanged: the wrapper is a non-capturing group.
	// Go's regexp has no \A.  A leading ^ anchors at the start of the text,
	// but only while the multiline flag is off - and the pattern's own
	// prefix may turn it on.  The anchor is therefore placed OUTSIDE the
	// pattern's flag group, with the flags reset around it, so it means the
	// start of the text whatever the pattern asked for.
	body := p.re.String()
	translated, _, _, _, terr := translate(p.source, p.flags)
	if terr == nil {
		if prefix := goFlags(p.flags); prefix != "" {
			body = "(?" + prefix + ":" + translated + ")"
		} else {
			body = translated
		}
	}
	if p.lifted != nil {
		// Anchored, with assertions: only the first candidate can ever be
		// considered, and it is accepted only if its assertions hold.
		idx := p.matchWithAssertionsAnchored(*p.lifted, text, whole)
		if idx == nil {
			return nil
		}
		return idx
	}
	anchored, err := regexp.Compile("(?-m)^(?:" + body + ")")
	if err != nil {
		return nil
	}
	idx := anchored.FindStringSubmatchIndex(text)
	if idx == nil {
		return nil
	}
	if whole && idx[1] != len(text) {
		return nil
	}
	return idx
}

// wrapped is the pattern with the flag prefix, for re-anchoring.
func (p *Pattern) wrapped() string {
	translated, _, _, _, err := translate(p.source, p.flags)
	if err != nil {
		return p.source
	}
	if prefix := goFlags(p.flags); prefix != "" {
		return "(?" + prefix + ")" + translated
	}
	return translated
}

// nextMatch finds the match after a given character position, applying
// Python's rule that an empty match must advance by one.
func (p *Pattern) nextMatch(text string, from int, lastEnd int, lastWasEmpty bool) (*Match, int, bool) {
	runes := []rune(text)
	for start := from; start <= len(runes); start++ {
		sub := string(runes[start:])
		idx := p.re.FindStringSubmatchIndex(sub)
		if idx == nil {
			return nil, 0, false
		}
		// The match is relative to the slice: shift it back.
		absolute := make([]int, len(idx))
		for i, v := range idx {
			if v < 0 {
				absolute[i] = -1
			} else {
				absolute[i] = charOffset(sub, v) + start
			}
		}
		empty := absolute[0] == absolute[1]
		if empty && absolute[0] == lastEnd && lastWasEmpty {
			// An empty match at the previous match's end is skipped, which
			// is the rule Go's FindAll does not apply.
			continue
		}
		m := p.buildMatch(text, absolute)
		return m, absolute[1], empty
	}
	return nil, 0, false
}

// matchList returns every match, in order.
func (p *Pattern) matchList(text string) (py.Tuple, error) {
	items := []py.Object{}
	from := 0
	lastEnd := -1
	lastWasEmpty := false
	for {
		m, end, empty := p.nextMatch(text, from, lastEnd, lastWasEmpty)
		if m == nil {
			break
		}
		items = append(items, m)
		lastEnd = end
		lastWasEmpty = empty
		if empty {
			from = end + 1
		} else {
			from = end
		}
		if from > len([]rune(text)) {
			break
		}
	}
	return py.Tuple(items), nil
}

// findAll returns the matches as Python's findall does: the group text when
// the pattern has groups, otherwise the whole match.
func (p *Pattern) findAll(text string) (py.Object, error) {
	matches, err := p.matchList(text)
	if err != nil {
		return nil, err
	}
	out := []py.Object{}
	for _, item := range matches {
		m := item.(*Match)
		if p.ngroups == 0 {
			out = append(out, m.groupText(0))
			continue
		}
		if p.ngroups == 1 {
			out = append(out, m.groupText(1))
			continue
		}
		group := make(py.Tuple, p.ngroups)
		for g := 1; g <= p.ngroups; g++ {
			group[g-1] = m.groupText(g)
		}
		out = append(out, group)
	}
	return py.NewListFromItems(out), nil
}

// split breaks the text at each match, including the captured groups in the
// result as CPython does.
func (p *Pattern) split(text string, maxsplit int) (py.Object, error) {
	matches, err := p.matchList(text)
	if err != nil {
		return nil, err
	}
	runes := []rune(text)
	out := []py.Object{}
	last := 0
	count := 0
	for _, item := range matches {
		m := item.(*Match)
		if maxsplit > 0 && count >= maxsplit {
			break
		}
		out = append(out, py.String(string(runes[last:m.locs[0]])))
		for g := 1; g <= p.ngroups; g++ {
			out = append(out, m.groupText(g))
		}
		last = m.locs[1]
		count++
	}
	out = append(out, py.String(string(runes[last:])))
	return py.NewListFromItems(out), nil
}

// expand applies a replacement template, supporting \1 and \g<name>.
func (m *Match) expand(template string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(template); i++ {
		c := template[i]
		if c != '\\' || i+1 >= len(template) {
			b.WriteByte(c)
			continue
		}
		next := template[i+1]
		switch {
		case next >= '1' && next <= '9':
			g := int(next - '0')
			if g > m.pattern.ngroups {
				return "", py.ExceptionNewf(ErrorType, "invalid group reference %d", g)
			}
			text, err := py.StrAsString(m.groupText(g))
			if err != nil {
				continue
			}
			b.WriteString(text)
			i++
		case next == 'g':
			end := strings.IndexByte(template[i:], '>')
			if end < 0 || i+3 >= len(template) || template[i+2] != '<' {
				return "", py.ExceptionNewf(ErrorType, "missing > in group name")
			}
			name := template[i+3 : i+end]
			if n, ok := m.pattern.nameToNum[name]; ok {
				text, err := py.StrAsString(m.groupText(n))
				if err == nil {
					b.WriteString(text)
				}
			}
			i += end
		default:
			b.WriteByte(next)
			i++
		}
	}
	return b.String(), nil
}

func subWith(p *Pattern, args py.Tuple, kwargs py.StringDict, withCount bool) (py.Object, error) {
	if len(args) < 2 {
		return nil, py.ExceptionNewf(py.TypeError, "sub() needs a replacement and a string")
	}
	repl := args[0]
	var text string
	switch v := args[1].(type) {
	case py.String:
		text = string(v)
	case py.Bytes:
		text = string(v)
	default:
		return nil, py.ExceptionNewf(py.TypeError, "expected string or bytes-like object")
	}
	if v, ok := kwargs.Get("string"); ok {
		s, err := py.StrAsString(v)
		if err != nil {
			return nil, err
		}
		text = s
	}
	count := 0
	if v, ok := kwargs.Get("count"); ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		count = n
	}
	if len(args) >= 3 {
		n, err := py.IndexInt(args[2])
		if err != nil {
			return nil, err
		}
		count = n
	}

	matches, err := p.matchList(text)
	if err != nil {
		return nil, err
	}
	runes := []rune(text)
	var b strings.Builder
	last := 0
	done := 0
	for _, item := range matches {
		m := item.(*Match)
		if count > 0 && done >= count {
			break
		}
		b.WriteString(string(runes[last:m.locs[0]]))
		switch r := repl.(type) {
		case py.String:
			expanded, err := m.expand(string(r))
			if err != nil {
				return nil, err
			}
			b.WriteString(expanded)
		default:
			// A callable replacement receives the match.
			value, err := py.Call(repl, py.Tuple{m}, py.StringDict{})
			if err != nil {
				return nil, err
			}
			s, err := py.StrAsString(value)
			if err != nil {
				return nil, err
			}
			b.WriteString(s)
		}
		last = m.locs[1]
		done++
	}
	b.WriteString(string(runes[last:]))
	if withCount {
		return py.Tuple{py.String(b.String()), py.Int(done)}, nil
	}
	return py.String(b.String()), nil
}

// patternArgs reads (string, pos, endpos) from a pattern method's arguments.
func patternArgs(name string, args py.Tuple, kwargs py.StringDict) (string, int, int, error) {
	if len(args) < 1 {
		return "", 0, 0, py.ExceptionNewf(py.TypeError, "%s() missing required argument 'string'", name)
	}
	var text string
	switch v := args[0].(type) {
	case py.String:
		text = string(v)
	case py.Bytes:
		return "", 0, 0, py.ExceptionNewf(py.TypeError, "cannot use a bytes pattern on a string")
	default:
		return "", 0, 0, py.ExceptionNewf(py.TypeError, "expected string or bytes-like object")
	}
	pos, endpos := 0, len([]rune(text))
	if len(args) >= 2 {
		n, err := py.IndexInt(args[1])
		if err != nil {
			return "", 0, 0, err
		}
		pos = n
	}
	if len(args) >= 3 {
		n, err := py.IndexInt(args[2])
		if err != nil {
			return "", 0, 0, err
		}
		endpos = n
	}
	if v, ok := kwargs.Get("string"); ok {
		s, err := py.StrAsString(v)
		if err != nil {
			return "", 0, 0, err
		}
		text = s
	}
	if v, ok := kwargs.Get("pos"); ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return "", 0, 0, err
		}
		pos = n
	}
	if v, ok := kwargs.Get("endpos"); ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return "", 0, 0, err
		}
		endpos = n
	}
	return text, pos, endpos, nil
}

// The module-level functions build a Pattern and delegate.
func compileFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 {
		return nil, py.ExceptionNewf(py.TypeError, "compile() missing required argument 'pattern'")
	}
	source, err := py.StrAsString(args[0])
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "first argument must be string or compiled pattern")
	}
	flags := 0
	if len(args) >= 2 {
		n, err := py.IndexInt(args[1])
		if err != nil {
			return nil, err
		}
		flags = n
	}
	if v, ok := kwargs.Get("flags"); ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		flags = n
	}
	return compile(source, flags)
}

// asPattern accepts either a compiled pattern or a string.
func asPattern(v py.Object, flags int) (*Pattern, error) {
	if p, ok := v.(*Pattern); ok {
		return p, nil
	}
	source, err := py.StrAsString(v)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "first argument must be string or compiled pattern")
	}
	return compile(source, flags)
}

func commonArgs(name string, args py.Tuple, kwargs py.StringDict) (*Pattern, py.Tuple, error) {
	if len(args) < 2 {
		return nil, nil, py.ExceptionNewf(py.TypeError, "%s() needs a pattern and a string", name)
	}
	// The flags argument is positional in the module-level functions -
	// re.findall(pattern, string, flags) - and may also be a keyword.  Only
	// looking at the keyword made re.findall(p, s, re.I) ignore the flag.
	//
	// sub, subn and split are the exceptions: their THIRD argument is the
	// count / maxsplit, not flags, and the replacement or string sits between
	// - "re.sub(pattern, repl, string, count, flags)".  Reading args[2] as
	// flags here called IndexInt on the REPLACEMENT and made every
	// "re.sub(pattern, repl, string)" fail with "unsupported operand type(s)
	// for index: 'str'".
	if name == "sub" || name == "subn" || name == "split" {
		flags := 0
		if v, ok := kwargs.Get("flags"); ok {
			n, err := py.IndexInt(v)
			if err != nil {
				return nil, nil, err
			}
			flags = n
		}
		p, err := asPattern(args[0], flags)
		if err != nil {
			return nil, nil, err
		}
		return p, args[1:], nil
	}
	flags := 0
	if len(args) >= 3 {
		n, err := py.IndexInt(args[2])
		if err != nil {
			return nil, nil, err
		}
		flags = n
	}
	if v, ok := kwargs.Get("flags"); ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, nil, err
		}
		flags = n
	}
	p, err := asPattern(args[0], flags)
	if err != nil {
		return nil, nil, err
	}
	return p, args[1:], nil
}

func matchFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p, rest, err := commonArgs("match", args, kwargs)
	if err != nil {
		return nil, err
	}
	text, err := textOf(rest)
	if err != nil {
		return nil, err
	}
	m := matchAt(p, text, 0)
	if m == nil {
		return py.None, nil
	}
	return m, nil
}

func searchFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p, rest, err := commonArgs("search", args, kwargs)
	if err != nil {
		return nil, err
	}
	text, err := textOf(rest)
	if err != nil {
		return nil, err
	}
	m := searchIn(p, text, 0)
	if m == nil {
		return py.None, nil
	}
	return m, nil
}

func fullMatchFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p, rest, err := commonArgs("fullmatch", args, kwargs)
	if err != nil {
		return nil, err
	}
	text, err := textOf(rest)
	if err != nil {
		return nil, err
	}
	m := fullMatchAt(p, text, 0)
	if m == nil {
		return py.None, nil
	}
	return m, nil
}

func findAllFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p, rest, err := commonArgs("findall", args, kwargs)
	if err != nil {
		return nil, err
	}
	text, err := textOf(rest)
	if err != nil {
		return nil, err
	}
	return p.findAll(text)
}

func findIterFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p, rest, err := commonArgs("finditer", args, kwargs)
	if err != nil {
		return nil, err
	}
	text, err := textOf(rest)
	if err != nil {
		return nil, err
	}
	items, err := p.matchList(text)
	if err != nil {
		return nil, err
	}
	return py.NewIterator(items), nil
}

func splitFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p, rest, err := commonArgs("split", args, kwargs)
	if err != nil {
		return nil, err
	}
	text, err := textOf(rest)
	if err != nil {
		return nil, err
	}
	maxsplit := 0
	if v, ok := kwargs.Get("maxsplit"); ok {
		n, err := py.IndexInt(v)
		if err != nil {
			return nil, err
		}
		maxsplit = n
	}
	if len(rest) >= 2 {
		n, err := py.IndexInt(rest[1])
		if err != nil {
			return nil, err
		}
		maxsplit = n
	}
	return p.split(text, maxsplit)
}

func subFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p, rest, err := commonArgs("sub", args, kwargs)
	if err != nil {
		return nil, err
	}
	return subWith(p, rest, kwargs, false)
}

func subNFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	p, rest, err := commonArgs("subn", args, kwargs)
	if err != nil {
		return nil, err
	}
	return subWith(p, rest, kwargs, true)
}

// textOf extracts the string from the remaining arguments.
func textOf(rest py.Tuple) (string, error) {
	if len(rest) < 1 {
		return "", py.ExceptionNewf(py.TypeError, "expected string or bytes-like object")
	}
	switch v := rest[0].(type) {
	case py.String:
		return string(v), nil
	case py.Bytes:
		return string(v), nil
	}
	return "", py.ExceptionNewf(py.TypeError, "expected string or bytes-like object")
}

// escapeFn quotes the characters that would otherwise be special.
func escapeFn(self py.Object, args py.Tuple) (py.Object, error) {
	var value py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "escape", 1, 1, &value); err != nil {
		return nil, err
	}
	text, err := py.StrAsString(value)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "escape() argument must be a string")
	}
	var b strings.Builder
	// The set Python 3.7 and later escapes.
	for _, r := range text {
		if r < 128 && strings.ContainsRune(`()[]{}?*+-|^$\.&~# \t\n\r\v\f`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return py.String(b.String()), nil
}

// keep format and strconv referenced: they are used by the error paths and
// the group parsing above.
var (
	_ = fmt.Sprintf
	_ = strconv.Itoa
)

// init wires the Go interface the VM uses for str(), so that a Match or
// Pattern reprs sensibly.
func (m *Match) M__str__() (py.Object, error) { return m.groupText(0), nil }

func (m *Match) M__repr__() (py.Object, error) {
	text, err := py.StrAsString(m.groupText(0))
	if err != nil {
		text = ""
	}
	return py.String("<re.Match object; span=(" + strconv.Itoa(m.locs[0]) + ", " + strconv.Itoa(m.locs[1]) + "), match=" + quoted(text) + ">"), nil
}

func quoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}

func (m *Match) M__getitem__(key py.Object) (py.Object, error) {
	idx, err := m.groupIndex(key)
	if err != nil {
		return nil, err
	}
	return m.groupText(idx), nil
}

var (
	_ py.I__str__     = (*Match)(nil)
	_ py.I__repr__    = (*Match)(nil)
	_ py.I__getitem__ = (*Match)(nil)
)
