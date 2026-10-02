// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fnmatch provides the implementation of python's 'fnmatch' module:
// unix shell style filename matching.
//
// The module is a direct port of CPython's Lib/fnmatch.py.  A pattern is
// translated to a regular expression string by translate(); fnmatch and
// fnmatchcase compile that string and match it against the name.  The
// translation is exposed verbatim, including its (?s:...)\z wrapper and its
// (?>...) atomic groups, because callers are documented to join several
// translate() results with "|" to build a matcher for many patterns at once.
//
// fnmatch lowercases both arguments through os.path.normcase first; on a
// case-sensitive filesystem normcase is the identity, so this module matches
// a name and a pattern with their cases intact, exactly as CPython does on
// posix.  fnmatchcase never case-folds.
package fnmatch

import (
	"regexp"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Filename matching with shell patterns.

fnmatch(FILENAME, PATTERN) matches according to the local convention.
fnmatchcase(FILENAME, PATTERN) always takes case in account.

The functions operate by translating the pattern into a regular
expression.  They cache the compiled regular expressions for speed.

The function translate(PATTERN) returns a regular expression
corresponding to PATTERN.  (It does not compile it.)`

func init() {
	globals := py.NewStringDict()

	globals.Set("fnmatch", py.MustNewMethod("fnmatch", fnmatch, 0, fnmatch_doc))
	globals.Set("fnmatchcase", py.MustNewMethod("fnmatchcase", fnmatchcase, 0, fnmatchcase_doc))
	globals.Set("filter", py.MustNewMethod("filter", filterFn, 0, filter_doc))
	globals.Set("filterfalse", py.MustNewMethod("filterfalse", filterfalseFn, 0, filterfalse_doc))
	globals.Set("translate", py.MustNewMethod("translate", translateFn, 0, translate_doc))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "fnmatch",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

const fnmatch_doc = `Test whether FILENAME matches PATTERN.

Patterns are Unix shell style:

*       matches everything
?       matches any single character
[seq]   matches any character in seq
[!seq]  matches any char not in seq

An initial period in FILENAME is not special.
Both FILENAME and PATTERN are first case-normalized
if the operating system requires it.
If you don't want this, use fnmatchcase(FILENAME, PATTERN).`

const fnmatchcase_doc = `Test whether FILENAME matches PATTERN, including case.

This is a version of fnmatch() which doesn't case-normalize
its arguments.`

const filter_doc = `Construct a list from those elements of the iterable NAMES that match PAT.`

const filterfalse_doc = `Construct a list from those elements of the iterable NAMES that do not match PAT.`

const translate_doc = `Translate a shell PATTERN to a regular expression.

There is no way to quote meta-characters.`

func fnmatch(self py.Object, args py.Tuple) (py.Object, error) {
	var name, pat py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "fnmatch", 2, 2, &name, &pat); err != nil {
		return nil, err
	}
	n, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	p, err := py.StrAsString(pat)
	if err != nil {
		return nil, err
	}
	// os.path.normcase lowercases on a case-insensitive filesystem; on this
	// one it is the identity, so the call is preserved for shape but is a
	// no-op.
	return py.NewBool(matchCase(normcase(n), normcase(p))), nil
}

func fnmatchcase(self py.Object, args py.Tuple) (py.Object, error) {
	var name, pat py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "fnmatchcase", 2, 2, &name, &pat); err != nil {
		return nil, err
	}
	n, err := py.StrAsString(name)
	if err != nil {
		return nil, err
	}
	p, err := py.StrAsString(pat)
	if err != nil {
		return nil, err
	}
	return py.NewBool(matchCase(n, p)), nil
}

func filterFn(self py.Object, args py.Tuple) (py.Object, error) {
	var names, pat py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "filter", 2, 2, &names, &pat); err != nil {
		return nil, err
	}
	p, err := py.StrAsString(pat)
	if err != nil {
		return nil, err
	}
	p = normcase(p)
	items, err := namesToObjects(names)
	if err != nil {
		return nil, err
	}
	out := py.NewList()
	for _, item := range items {
		name, err := py.StrAsString(item)
		if err != nil {
			return nil, err
		}
		if matchCase(normcase(name), p) {
			// filter returns the original element, so a bytes name comes back
			// as bytes, as in CPython.
			out.Append(item)
		}
	}
	return out, nil
}

func filterfalseFn(self py.Object, args py.Tuple) (py.Object, error) {
	var names, pat py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "filterfalse", 2, 2, &names, &pat); err != nil {
		return nil, err
	}
	p, err := py.StrAsString(pat)
	if err != nil {
		return nil, err
	}
	p = normcase(p)
	items, err := namesToObjects(names)
	if err != nil {
		return nil, err
	}
	out := py.NewList()
	for _, item := range items {
		name, err := py.StrAsString(item)
		if err != nil {
			return nil, err
		}
		if !matchCase(normcase(name), p) {
			out.Append(item)
		}
	}
	return out, nil
}

func translateFn(self py.Object, args py.Tuple) (py.Object, error) {
	var pat py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "translate", 1, 1, &pat); err != nil {
		return nil, err
	}
	p, err := py.StrAsString(pat)
	if err != nil {
		return nil, err
	}
	return py.String(translate(p)), nil
}

// namesToObjects converts an iterable of names to a slice of objects.
func namesToObjects(names py.Object) ([]py.Object, error) {
	items, err := py.SequenceList(names)
	if err != nil {
		return nil, err
	}
	return items.Items, nil
}

// normcase is os.path.normcase: identity on a case-sensitive filesystem.
func normcase(s string) string {
	return s
}

// matchCase compiles the translated pattern and matches it against name.
func matchCase(name, pat string) bool {
	re, err := regexp.Compile(translate(pat))
	if err != nil {
		// A pattern that will not compile cannot match anything.  translate
		// escapes everything a literal could smuggle in, so this is
		// unreachable for string patterns, but a panic here would be worse
		// than a non-match.
		return false
	}
	return re.MatchString(name)
}

// translate is a direct port of CPython's fnmatch.translate.
//
// It emits CPython's exact output, including the atomic groups "(?>.*?" that
// the Go regexp engine does not accept.  Go's regexp is leftmost-first without
// backtracking, which is precisely what an atomic group approximates, so the
// two spellings of an interior STAR match identically; callers receive the
// CPython text either way.
func translate(pat string) string {
	parts, starIndices := translateParts(pat)
	if len(starIndices) == 0 {
		return "(?s:" + strings.Join(parts, "") + ")\\z"
	}
	buffer := append([]string{}, parts[:starIndices[0]]...)
	i := starIndices[0] + 1
	for _, j := range starIndices[1:] {
		buffer = append(buffer, "(?>.*?")
		buffer = append(buffer, parts[i:j]...)
		buffer = append(buffer, ")")
		i = j + 1
	}
	buffer = append(buffer, ".*")
	buffer = append(buffer, parts[i:]...)
	return "(?s:" + strings.Join(buffer, "") + ")\\z"
}

// translateParts is CPython's fnmatch._translate: it splits the pattern into
// regular-expression fragments and records the position of each STAR.
func translateParts(pat string) (parts []string, starIndices []int) {
	n := len(pat)
	i := 0
	for i < n {
		c := pat[i]
		i++
		switch c {
		case '*':
			starIndices = append(starIndices, len(parts))
			parts = append(parts, ".*")
			// compress consecutive `*` into one
			for i < n && pat[i] == '*' {
				i++
			}
		case '?':
			parts = append(parts, ".")
		case '[':
			j := i
			if j < n && pat[j] == '!' {
				j++
			}
			if j < n && pat[j] == ']' {
				j++
			}
			for j < n && pat[j] != ']' {
				j++
			}
			if j >= n {
				parts = append(parts, `\[`)
			} else {
				stuff := pat[i:j]
				if !strings.ContainsRune(stuff, '-') {
					stuff = strings.ReplaceAll(stuff, `\`, `\\`)
				} else {
					// CPython mutates its loop index as it splits the class body
					// at each '-'; splitRangeChunks does the same scan without a
					// mutable index.
					chunks := splitRangeChunks(pat, i, j)
					// Remove empty ranges -- invalid in RE.
					for k := len(chunks) - 1; k > 0; k-- {
						if chunks[k-1][len(chunks[k-1])-1] > chunks[k][0] {
							chunks[k-1] = chunks[k-1][:len(chunks[k-1])-1] + chunks[k][1:]
							chunks = append(chunks[:k], chunks[k+1:]...)
						}
					}
					escaped := make([]string, len(chunks))
					for ci, ch := range chunks {
						ch = strings.ReplaceAll(ch, `\`, `\\`)
						ch = strings.ReplaceAll(ch, "-", `\-`)
						escaped[ci] = ch
					}
					stuff = strings.Join(escaped, "-")
				}
				i = j + 1
				if stuff == "" {
					// Empty range: never match.
					parts = append(parts, "(?!)")
				} else if stuff == "!" {
					// Negated empty range: match any character.
					parts = append(parts, ".")
				} else {
					stuff = setopsSub(stuff)
					if stuff[0] == '!' {
						stuff = "^" + stuff[1:]
					} else if stuff[0] == '^' || stuff[0] == '[' {
						stuff = `\` + stuff
					}
					parts = append(parts, "["+stuff+"]")
				}
			}
		default:
			parts = append(parts, reEscape(c))
		}
	}
	return parts, starIndices
}

// splitRangeChunks is the chunk-splitting half of CPython's `'-' in stuff`
// branch: it splits the character-class body at each '-' into alternating
// literal pieces and range endpoints.  CPython mutates its loop index as it
// goes; doing it in one pass here keeps the same result.
func splitRangeChunks(pat string, i, j int) []string {
	chunks := []string{}
	k := i + 1
	if pat[i] == '!' {
		k = i + 2
	}
	start := i
	for k < j {
		idx := strings.IndexByte(pat[k:j], '-')
		if idx < 0 {
			break
		}
		pos := k + idx
		chunks = append(chunks, pat[start:pos])
		start = pos + 1
		k = pos + 3
	}
	chunk := pat[start:j]
	if chunk != "" {
		chunks = append(chunks, chunk)
	} else if len(chunks) > 0 {
		chunks[len(chunks)-1] += "-"
	}
	return chunks
}

// setopsSub escapes the set operators &&, ~~ and || inside a character class,
// as CPython's _re_setops_sub does.
func setopsSub(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '&' || c == '~' || c == '|' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// reEscape escapes a single character the way re.escape does: every character
// with special meaning is backslash-escaped, and so are the whitespace and
// shell-ish characters CPython's re._special_chars_map lists.
func reEscape(c byte) string {
	switch c {
	case '(', ')', '[', ']', '{', '}', '?', '*', '+', '-', '|', '^', '$',
		'\\', '.', '&', '~', '#', ' ':
		return `\` + string(c)
	case '\t':
		return "\\\t"
	case '\n':
		return "\\\n"
	case '\r':
		return "\\\r"
	case '\v':
		return "\\\x0b"
	case '\f':
		return "\\\x0c"
	}
	return string(c)
}
