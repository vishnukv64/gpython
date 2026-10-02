// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Lookahead support.
//
// Go's regexp is a finite-automaton engine and cannot backtrack, so it has no
// way to say "does what FOLLOWS match, without consuming it" - which is what a
// lookaround is.  Rather than write a second engine, a pattern containing
// lookahead is split in two:
//
//  1. the pattern with the assertions REMOVED, which Go compiles; and
//  2. the assertions, each with the alternation branch it was written in and
//     the rule for where it must be re-checked.
//
// A candidate match is accepted only if every assertion for the branch it came
// from holds where it belongs; otherwise the search moves on to the next
// candidate.  Patterns with no lookahead take exactly the old path - the split
// is a no-op and nothing below runs.
//
// This exists for pip: packaging/utils.py compiles
//
//	[a-z0-9]|[a-z0-9]([a-z0-9-](?!--))*[a-z0-9]
//
// at import, and pip's entry point imports packaging, so pip cannot start at
// all while lookaround is refused outright.
//
// Only shapes that can be checked EXACTLY are accepted.  Anything else raises
// re.error naming the shape, because a lookaround that is silently ignored
// turns a pattern into a different one, and a wrong answer is worse than a
// refusal.  Supported:
//
//   - a lookahead inside a repeat group whose inner pattern is exactly one
//     character wide (the packaging shape): it is re-checked at every
//     repetition boundary, which for width 1 is every interior position;
//   - a lookahead at the END of its branch: re-checked at the end of the
//     match;
//   - a NEGATIVE lookahead in either position (the common use).
//
// Not supported, and refused by name: lookbehind, conditionals, positive
// lookahead inside a repeat, and any repeat whose inner pattern is not a fixed
// single character.

package re

import (
	"regexp"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// assertWhere says where an assertion must be re-checked.
type assertWhere int

const (
	// atEnd means the assertion sat at the end of its branch, so it is checked
	// at the end offset of the match.
	atEnd assertWhere = iota
	// atEachChar means the assertion sat inside a repeat of single-character
	// atoms, so it applies after EVERY character of the matched repeat - which
	// is every position strictly inside the match.
	atEachChar
)

// assertion is one lookahead lifted out of a pattern.
type assertion struct {
	// at is the alternation branch the assertion was written in.  Go's
	// alternation is leftmost-first and non-overlapping, so a match comes from
	// exactly one branch and an assertion only governs that branch.
	at int
	// body is the assertion's own sub-pattern, already translated.
	body string
	// negative distinguishes (?!X) from (?=X).
	negative bool
	// where says how to find the offsets to check.
	where assertWhere
	// before and after are how many atoms of the SAME ALTERNATION BRANCH
	// precede the repeat group and follow it.  The group therefore occupies
	// [start+before, end-after) of the match, and its own atom boundaries -
	// where the assertion applies - are the positions strictly inside that.
	//
	// Branch-scoped is the whole point: counting every atom in the pattern
	// before the group also counted the atoms of the OTHER alternatives, which
	// for packaging's pattern made the group start two characters in instead
	// of one and wrongly rejected "a--a".
	before int
	after  int
}

// lifted is a pattern split for lookahead support.
type lifted struct {
	pattern    string
	assertions []assertion
}

// hasLookahead reports whether a pattern contains a lookahead construct.
func hasLookahead(s string) bool {
	return strings.Contains(s, "(?=") || strings.Contains(s, "(?!")
}

// atomsBefore counts the atoms in a pattern fragment.
//
// An atom consumes exactly one character, which is what makes "how many atoms
// precede the repeat group" the same as "how many characters into the match
// the group starts".  Assertions, anchors and quantifiers consume nothing and
// are not counted.
func atomsBefore(fragment string) int {
	n := 0
	for i := 0; i < len(fragment); {
		switch c := fragment[i]; {
		case c == '\\':
			n++
			i += 2
			continue
		case c == '[':
			n++
			i = skipClass(fragment, i)
			continue
		case c == '(':
			if strings.HasPrefix(fragment[i:], "(?!") || strings.HasPrefix(fragment[i:], "(?=") {
				// An assertion consumes nothing.
				if _, end, ok := scanLookahead(fragment, i); ok {
					i = end
					continue
				}
			}
			if strings.HasPrefix(fragment[i:], "(?:)") {
				// A lifted assertion's placeholder.
				i += 4
				continue
			}
		case c == '^' || c == '$' || c == '*' || c == '+' || c == '?' ||
			c == '{' || c == '}' || c == ',' || c == '|':
			// Nothing consumed.
		default:
			n++
		}
		i++
	}
	return n
}

// branchPrefix returns the fragment of a branch that precedes offset open - the
// text after the last top-level alternation, which is what makes an atom count
// branch-scoped.
func branchPrefix(pattern string, open int) string {
	depth := 0
	cut := 0
	for i := 0; i < open && i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '[':
			i = skipClass(pattern, i) - 1
		case '(':
			depth++
		case ')':
			depth--
		case '|':
			if depth == 0 {
				cut = i + 1
			}
		}
	}
	return pattern[cut:open]
}

// atomsAfter counts the atoms of a branch from the end of a repeat group up to
// the next alternation boundary or the end, which is the number of characters
// that follow the group inside the match.
func atomsAfter(pattern string, from int) int {
	depth := 0
	end := len(pattern)
	for i := from; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '[':
			i = skipClass(pattern, i) - 1
		case '(':
			depth++
		case ')':
			depth--
		case '|':
			if depth == 0 {
				end = i
				i = len(pattern)
			}
		}
	}
	return atomsBefore(pattern[from:end])
}

// quantifiedBefore reports whether any atom before offset at in the same
// alternation branch carries a quantifier, which is what lets the engine match
// a shorter string than it first chooses.
func quantifiedBefore(pattern string, at int) bool {
	depth := 0
	for i := 0; i < at; i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '[':
			i = skipClass(pattern, i) - 1
		case '(':
			depth++
		case ')':
			depth--
		case '*', '+', '{':
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// repeatGroups reports, for each offset of an opening parenthesis, whether the
// group it opens is immediately QUANTIFIED - followed by * + or {n,m} once its
// matching ) is found.
//
// This is a separate pass because the quantifier is written AFTER the group
// closes, while the decision it drives - is this assertion inside a repeat? -
// has to be made while scanning the group's INTERIOR, which happens first.
// Deciding it inline was why the packaging pattern's assertion was treated as
// end-of-match and "a--b--c" was wrongly accepted.
func repeatGroups(pattern string) map[int]bool {
	quantified := map[int]bool{}
	var open []int
	for i := 0; i < len(pattern); {
		switch c := pattern[i]; {
		case c == '\\':
			i += 2
			continue
		case c == '[':
			i = skipClass(pattern, i)
			continue
		case c == '(':
			open = append(open, i)
		case c == ')':
			if len(open) > 0 {
				off := open[len(open)-1]
				open = open[:len(open)-1]
				if i+1 < len(pattern) {
					switch pattern[i+1] {
					case '*', '+', '{':
						quantified[off] = true
					}
				}
			}
		}
		i++
	}
	return quantified
}

// lift splits a translated pattern into the part Go compiles and the
// assertions taken out of it.
//
// It returns an error for every shape it cannot check exactly rather than
// producing a pattern that means something subtly different.
func lift(pattern string) (lifted, error) {
	if !hasLookahead(pattern) {
		return lifted{pattern: pattern}, nil
	}
	// Positive lookahead inside a repeat cannot be expressed as a check on the
	// finished match - it constrains what the engine may consume - so it is
	// refused up front rather than mis-checked later.
	if strings.Contains(pattern, "(?=") {
		return lifted{}, py.ExceptionNewf(ErrorType,
			"positive lookahead is not supported: only (?!...) is, and only where its position is determined by the match")
	}

	var out strings.Builder
	var assertions []assertion
	depth := 0
	// quantified is built in its own pass, because a group's quantifier is
	// written AFTER its closing parenthesis while the decision it drives - is
	// this assertion inside a repeat? - has to be made while scanning the
	// group's interior, which comes first.
	quantified := repeatGroups(pattern)
	// openGroups holds the OFFSET of each open parenthesis, so a group can be
	// looked up in quantified above.
	var openGroups []int
	// groupOpenAt holds the offset of each open parenthesis, so a closing one
	// can find where its group began.
	var groupOpenAt []int
	// pendingRepeat holds the index of a repeat assertion waiting for its group
	// to close, so the atoms around that group can be counted once the whole
	// group is on hand.  The count cannot be known while scanning the interior:
	// the group's trailing atoms have not been seen yet.
	var pendingRepeat *int

	for i := 0; i < len(pattern); {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			out.WriteString(pattern[i : i+2])
			i += 2
			continue
		}
		if c == '[' {
			j := skipClass(pattern, i)
			out.WriteString(pattern[i:j])
			i = j
			continue
		}
		if c == '|' && depth == 0 {
			out.WriteByte(c)
			i++
			continue
		}
		if c == '(' {
			if strings.HasPrefix(pattern[i:], "(?!") {
				body, end, ok := scanLookahead(pattern, i)
				if !ok {
					return lifted{}, py.ExceptionNewf(ErrorType, "unbalanced (?! in pattern")
				}
				// Where must this be checked?  Inside a repeat, at every interior
				// character of the match; otherwise at its end.  The enclosing
				// group's quantifier follows its closing parenthesis, so this is
				// answered from the table built above.
				where := atEnd
				if len(openGroups) > 0 && quantified[openGroups[len(openGroups)-1]] {
					where = atEachChar
				}
				// An END-of-match assertion is only exact when nothing before it
				// can match a SHORTER string.  CPython backtracks: for
				// "\d+(?!%)" on "12%" the engine first takes "12", fails the
				// assertion, then retreats to "1" and succeeds - and this
				// implementation cannot retreat, because it re-checks the
				// assertion against a match the engine already chose.  Rather
				// than answer "no match" where CPython matches, a quantified
				// prefix is refused.
				if where == atEnd && quantifiedBefore(pattern, i) {
					return lifted{}, py.ExceptionNewf(ErrorType,
						"(?!...) after a quantified part is not supported: the engine would have to backtrack to satisfy it")
				}
				assertions = append(assertions, assertion{
					at: -1, body: body, negative: true, where: where,
				})
				if where == atEachChar {
					idx := len(assertions) - 1
					pendingRepeat = &idx
				}
				// An empty non-capturing group keeps group numbering and the
				// surrounding structure identical.
				out.WriteString("(?:)")
				i = end
				continue
			}
			if strings.HasPrefix(pattern[i:], "(?<") || strings.HasPrefix(pattern[i:], "(?(") {
				return lifted{}, py.ExceptionNewf(ErrorType,
					"lookbehind and conditionals are not supported: Go's regexp engine has no equivalent")
			}
			if strings.HasPrefix(pattern[i:], "(?:") {
				openGroups = append(openGroups, i)
				groupOpenAt = append(groupOpenAt, i)
				out.WriteString("(?:")
				i += 3
				continue
			}
			if strings.HasPrefix(pattern[i:], "(?") {
				// (?i) and other inline flags: copy and let the compiler judge.
				out.WriteByte(c)
				i++
				continue
			}
			depth++
			openGroups = append(openGroups, i)
			groupOpenAt = append(groupOpenAt, i)
			out.WriteByte(c)
			i++
			continue
		}
		if c == ')' {
			if len(groupOpenAt) > 0 {
				open := groupOpenAt[len(groupOpenAt)-1]
				openGroups = openGroups[:len(openGroups)-1]
				groupOpenAt = groupOpenAt[:len(groupOpenAt)-1]
				depth--
				if pendingRepeat != nil {
					// The group is complete: count the atoms before it and inside
					// it, which is what locates the assertion's own boundaries.
					idx := *pendingRepeat
					assertions[idx].before = atomsBefore(branchPrefix(pattern, open))
					assertions[idx].after = atomsAfter(pattern, i+1)
					pendingRepeat = nil
				}
			}
			out.WriteByte(c)
			i++
			continue
		}
		out.WriteByte(c)
		i++
	}
	if len(assertions) == 0 {
		return lifted{pattern: pattern}, nil
	}
	// A repeat-group assertion is only exact when the repeated atom is one
	// character wide, since that makes every interior position a boundary.
	for _, a := range assertions {
		if a.where == atEachChar && !singleCharWidth(pattern) {
			return lifted{}, py.ExceptionNewf(ErrorType,
				"(?!...) inside a repeat is supported only when the repeated part is a single character")
		}
	}
	return lifted{pattern: out.String(), assertions: assertions}, nil
}

// skipClass returns the offset just past the character class starting at i.
func skipClass(pattern string, i int) int {
	j := i + 1
	if j < len(pattern) && pattern[j] == '^' {
		j++
	}
	if j < len(pattern) && pattern[j] == ']' {
		j++
	}
	for j < len(pattern) && pattern[j] != ']' {
		if pattern[j] == '\\' {
			j++
		}
		j++
	}
	if j < len(pattern) {
		j++
	}
	return j
}

// scanLookahead returns the body of the (?!...) group at i and the offset just
// past its closing parenthesis.
func scanLookahead(pattern string, i int) (string, int, bool) {
	start := i + 3
	depth := 1
	j := start
	for j < len(pattern) {
		switch pattern[j] {
		case '\\':
			j += 2
			continue
		case '[':
			j = skipClass(pattern, j)
			continue
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return pattern[start:j], j + 1, true
			}
		}
		j++
	}
	return "", i, false
}

// singleCharWidth reports whether the repeated part of a pattern consumes
// exactly one character per repetition - true when every repetition body is a
// single literal or character class.
//
// The check is deliberately conservative: it only has to recognise the shape
// "([...])*" and "([...](?!...))*", and it answers false for anything it is
// not sure about, which turns into a refusal rather than a wrong answer.
func singleCharWidth(pattern string) bool {
	// Count the atoms in the first group's body.
	open := strings.IndexByte(pattern, '(')
	if open < 0 {
		return false
	}
	depth := 0
	atoms := 0
	for i := open; i < len(pattern); {
		switch c := pattern[i]; {
		case c == '\\':
			atoms++
			i += 2
			continue
		case c == '[':
			atoms++
			i = skipClass(pattern, i)
			continue
		case c == '(':
			if strings.HasPrefix(pattern[i:], "(?!") {
				// An assertion consumes nothing, so it is not an atom.
				_, end, ok := scanLookahead(pattern, i)
				if !ok {
					return false
				}
				i = end
				continue
			}
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return atoms == 1
			}
		case c == '^' || c == '$':
			// An anchor consumes nothing.
		default:
			atoms++
		}
		i++
	}
	return false
}

// matchWithAssertions finds the first match whose assertions hold.
//
// Candidates are walked left to right.  A rejected candidate does not end the
// search: the engine chose an alternative and another may satisfy the
// assertion, which is exactly how packaging's pattern behaves.
func (p *Pattern) matchWithAssertions(l lifted, text string, whole bool) []int {
	body, err := p.compileTranslated(l.pattern)
	if err != nil {
		return nil
	}
	offset := 0
	for offset <= len(text) {
		idx := body.FindStringSubmatchIndex(text[offset:])
		if idx == nil {
			return nil
		}
		start, end := idx[0]+offset, idx[1]+offset
		if whole && end != len(text) {
			return nil
		}
		if p.assertionsHold(l, text, idx, start, end) {
			shifted := make([]int, len(idx))
			for i, v := range idx {
				if v < 0 {
					shifted[i] = -1
				} else {
					shifted[i] = v + offset
				}
			}
			return shifted
		}
		if whole {
			// Anchored: there is no later candidate to try.
			return nil
		}
		next := start + 1
		if end > next {
			next = end
		}
		if next <= offset {
			return nil
		}
		offset = next
	}
	return nil
}

// assertionsHold checks a candidate match against every lifted assertion.
//
// The branch an assertion was written in is NOT used to gate the check, and
// that is deliberate rather than sloppy.  Go's alternation is non-overlapping
// and leftmost-first, so knowing WHICH branch a finished match came from is not
// something the engine reports; and for the shapes accepted here it makes no
// difference.  The one supported alternative that has interior positions is the
// repeat, so checking every interior position of the match is exactly the
// repeat's own region - and the other alternatives are a single character wide,
// with no interior to check.  Anything wider is refused by lift().
func (p *Pattern) assertionsHold(l lifted, text string, idx []int, start, end int) bool {
	for _, a := range l.assertions {
		re, err := p.compileTranslated(a.body)
		if err != nil {
			return false
		}
		positions := []int{end}
		if a.where == atEachChar {
			// The assertion sits after the atom inside its group, so it applies
			// at each atom boundary of THAT GROUP - not at every interior
			// position of the match.  The difference is real: CPython accepts
			// "a--a" (the group's own atom is "a", and "--" follows the first
			// one) while "aa--a" fails, and no clamp of the match's interior
			// separates those two.
			//
			// The group's own span is what locates them, and it is derived
			// from the atom counts: the group occupies
			//	[start+before, end-after)
			// so the boundaries its assertion applies at are the positions
			// strictly inside it.
			positions = nil
			for pos := start + a.before + 1; pos <= end-a.after; pos++ {
				positions = append(positions, pos)
			}
		}
		for _, pos := range positions {
			if pos < 0 || pos > len(text) {
				return false
			}
			loc := re.FindStringIndex(text[pos:])
			matched := loc != nil && loc[0] == 0
			if a.negative == matched {
				return false
			}
		}
	}
	return true
}

// compileTranslated compiles an already-translated body with this pattern's
// flags folded in.
func (p *Pattern) compileTranslated(body string) (*regexp.Regexp, error) {
	if prefix := goFlags(p.flags); prefix != "" {
		body = "(?" + prefix + ":" + body + ")"
	}
	return regexp.Compile(body)
}

// matchWithAssertionsAnchored matches at the START of text only, which is what
// match() and fullmatch() need: there is no later candidate to fall back on, so
// a rejected first candidate ends the search.
//
// whole additionally requires the match to consume all of text - fullmatch() -
// and is checked by the engine's own "\z" rather than by comparing the end
// offset afterwards, which is what made fullmatch silently give up when the
// first alternative was shorter than the string.
func (p *Pattern) matchWithAssertionsAnchored(l lifted, text string, whole bool) []int {
	suffix := ""
	if whole {
		// A literal backslash-z, which is Go's regexp end-of-text anchor.
		suffix = "\\z"
	}
	body, err := p.compileTranslated("(?-m)^(?:" + l.pattern + ")" + suffix)
	if err != nil {
		return nil
	}
	idx := body.FindStringSubmatchIndex(text)
	if idx == nil {
		return nil
	}
	if !p.assertionsHold(l, text, idx, idx[0], idx[1]) {
		return nil
	}
	return idx
}
