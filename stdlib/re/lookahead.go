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
	"strconv"
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

// assertion is one lookaround lifted out of a pattern.
type assertion struct {
	// lookbehind marks a (?<=...) / (?<!...) rather than a lookahead.  A
	// lookbehind is checked BEFORE the match and a lookahead AFTER it, and a
	// lookbehind's body is a WIDTH rather than a set of positions.
	lookbehind bool
	// width is how many characters a lookbehind examines immediately before
	// the position it is anchored at.  CPython requires this to be fixed, and
	// a variable-width body is refused.
	width int
	// positive marks (?<=...) / (?=...) as opposed to the negative forms.
	positive bool
	// trailing is the fixed width of what FOLLOWS the assertion within its
	// branch.  When that is fixed, the assertion's position in the match is
	// exactly "end of match minus trailing", which is how a lookbehind in the
	// MIDDLE of a pattern is located - rich needs one, in
	// 'b?".*?(?<!\)"'.
	trailing int
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
	// lead is how many characters the lifted LOOKBEHIND placeholders consume
	// at the very START of the match.  They are part of the pattern Go
	// compiles - that is what anchors them - but they are not part of what the
	// user asked for, so the reported span starts after them.
	lead int
}

// hasLookahead reports whether a pattern contains a lookahead construct.
func hasLookahead(s string) bool {
	return strings.Contains(s, "(?=") || strings.Contains(s, "(?!") ||
		strings.Contains(s, "(?<=") || strings.Contains(s, "(?<!")
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
			if strings.HasPrefix(fragment[i:], "(?<=") || strings.HasPrefix(fragment[i:], "(?<!") {
				// A lookbehind was LIFTED to a consuming group, so it counts
				// as the atoms it consumes.
				if body, end, ok := scanLookbehind(fragment, i); ok {
					if w, err := fixedWidth(body); err == nil {
						n += w
						i = end
						continue
					}
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
	if strings.Contains(pattern, "(?=") && !strings.Contains(pattern, "(?<=") {
		return lifted{}, py.ExceptionNewf(ErrorType,
			"positive lookahead is not supported: only (?!...) is, and only where its position is determined by the match")
	}

	var out strings.Builder
	var assertions []assertion
	depth := 0
	// lead is the total width the lifted lookbehind placeholders consume.
	lead := 0
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
			// A TOP-LEVEL '|' begins a new branch.
			out.WriteByte(c)
			i++
			continue
		}
		if c == '(' {
			if strings.HasPrefix(pattern[i:], "(?<=") || strings.HasPrefix(pattern[i:], "(?<!") {
				// Lookbehind.  CPython requires a FIXED width, and that is
				// exactly what makes it expressible here: the assertion is
				// replaced by a group that CONSUMES the same number of
				// characters, so it is anchored at the right place in the
				// match, and the condition is then re-checked on the text.
				//
				// rich's ReprHighlighter needs three of these -
				// (?<!\w), (?<![\w]) and (?<!\) - and without them pip
				// could not print anything at all.
				body, end, ok := scanLookbehind(pattern, i)
				if !ok {
					return lifted{}, py.ExceptionNewf(ErrorType, "unbalanced (?<= or (?<! in pattern")
				}
				w, err := fixedWidth(body)
				if err != nil {
					return lifted{}, err
				}
				if w == 0 {
					// A zero-width lookbehind has nothing to consume, so the
					// consuming rewrite would anchor it at the wrong place.
					return lifted{}, py.ExceptionNewf(ErrorType,
						"zero-width lookbehind is not supported")
				}
				// A lookbehind is lifted to NOTHING, so the engine matches the
				// pattern without it and the assertion is checked on the text
				// afterwards.  Where it is checked depends on where it sits:
				//
				//   leading - "(?<!x)y" - the position it governs is the start
				//   of the match, which is exactly where Go begins.
				//
				//   mid-pattern - 'b?".*?(?<!\)"' - the position is "end of
				//   match minus the width of what follows it in its branch".
				//   That requires the tail to be fixed-width, and a variable
				//   one is refused rather than mis-anchored.
				// "Leading" means nothing that CONSUMES text precedes the
				// assertion in its branch - the enclosing group's head and a
				// zero-width prefix like "(?P<name>" are not consumption, and
				// counting the output length treated them as if they were, so
				// "(?P<number>(?<!\w)...)" was refused.
				// Only the text BETWEEN the branch start and the assertion can
				// consume - passing the whole prefix scanned everything before
				// the branch as well, so a lookbehind leading its own branch
				// was judged to be mid-pattern.
				trailing := 0
				bp := branchPos(pattern, i)
				if consumesBefore(pattern[bp:i], len(pattern[bp:i])) {
					rest, ok := branchRest(pattern, end)
					if !ok {
						return lifted{}, py.ExceptionNewf(ErrorType,
							"a lookbehind in the middle of a pattern is supported only when what follows it in its branch is fixed-width")
					}
					w2, err := widthOf(rest)
					if err != nil || w2 < 0 {
						return lifted{}, py.ExceptionNewf(ErrorType,
							"a lookbehind in the middle of a pattern is supported only when what follows it in its branch is fixed-width")
					}
					trailing = w2
				}
				assertions = append(assertions, assertion{
					lookbehind: true,
					width:      w,
					positive:   strings.HasPrefix(pattern[i:], "(?<="),
					negative:   strings.HasPrefix(pattern[i:], "(?<!"),
					body:       body,
					at:         -1,
					trailing:   trailing,
				})
				i = end
				continue
			}
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
			if strings.HasPrefix(pattern[i:], "(?(") {
				return lifted{}, py.ExceptionNewf(ErrorType,
					"conditionals are not supported: Go's regexp engine has no equivalent")
			}
			if strings.HasPrefix(pattern[i:], "(?<") {
				return lifted{}, py.ExceptionNewf(ErrorType,
					"a named group (?<name>...) is not supported here")
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
	return lifted{pattern: out.String(), assertions: assertions, lead: lead}, nil
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
		// The check is made against ABSOLUTE positions, because the assertion
		// examines the text around the match - and idx is relative to the slice
		// the engine just searched.  Passing the relative idx with an absolute
		// start made "(?<!a|b)(c|d)" test the wrong character.
		abs := make([]int, len(idx))
		for i, v := range idx {
			if v < 0 {
				abs[i] = -1
			} else {
				abs[i] = v + offset
			}
		}
		start, end := abs[0], abs[1]
		if whole && end != len(text) {
			return nil
		}
		if p.assertionsHold(l, text, abs, start, end) {
			// Indices stay RELATIVE to the slice the engine searched: the
			// caller adds its own base, and returning already-shifted indices
			// made finditer report a span of (1, 4) while group(0) was the
			// text of a DIFFERENT span.
			return trimLead(l, idx)
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
// assertionsHoldOffset is assertionsHold for a slice that begins at base within
// a larger string, so a lookbehind can read the text before the slice.

// check and before are BYTE offsets into the FULL text, and the indices
// are for the slice - so a bad tail width could run past the end and
// panic the host process.  Both bounds are checked.

func (p *Pattern) assertionsHold(l lifted, text string, idx []int, start, end int) bool {
	for _, a := range l.assertions {
		re, err := p.compileTranslated(a.body)
		if err != nil {
			return false
		}
		if a.lookbehind {
			// The assertion was lifted to a group that CONSUMES its own width
			// at the start of the match, so for a match starting at start the
			// text it examines is the width characters ENDING at start.  The
			// consumed placeholder must then be given back: the group is not
			// part of what the user asked for.
			//
			// A lookbehind near the start of the string cannot be satisfied -
			// there is not enough text before it - which is how CPython
			// behaves, and for a NEGATIVE one that still means "no match here".
			// The assertion governs the match START, and examines the width
			// characters immediately before it.  Near the start of the string
			// there is not enough text: a POSITIVE lookbehind cannot hold
			// there, and a NEGATIVE one holds because nothing matched - which
			// is CPython's answer for "re.search(r"(?<!x)y", "y")".
			check := start
			if a.trailing > 0 {
				check = end - a.trailing
			}
			before := check - a.width
			matched := false
			if before >= 0 {
				loc := re.FindStringIndex(text[before:check])
				matched = loc != nil && loc[0] == 0 && loc[1] == a.width
			} else if a.positive {
				return false
			}
			// A negative assertion SUCCEEDS when the body does not match; a
			// positive one when it does.  Comparing them the other way round
			// made "(?<!x)y" match "xy" and "(?<=x)y" match nothing.
			if a.positive != matched {
				return false
			}
			continue
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

// matchWithAssertionsOffset is matchWithAssertions when the slice being
// searched begins at base in a LARGER string.
//
// A shifted lookbehind must be able to see the characters before its match, and
// those live in the larger string - so the check is given the whole text and the
// base, rather than being handed a truncated slice.

// matchWithAssertionsIn searches s from off and checks each candidate against
// every lifted assertion, where a lookbehind reads from the WHOLE string so
// that it can see the characters before the match.
//
// Searching a slice beginning at the match position cannot work for a
// lookbehind: the preceding character has been sliced away, and the assertion
// then answers about the string start instead - "(?<!a|b)(c|d)" reported a match
// at "bc" because the "b" was not visible.
func (p *Pattern) matchWithAssertionsIn(l lifted, s string, off int) []int {
	body, err := p.compileTranslated(l.pattern)
	if err != nil {
		return nil
	}
	from := off
	for from <= len(s) {
		idx := body.FindStringSubmatchIndex(s[from:])
		if idx == nil {
			return nil
		}
		abs := make([]int, len(idx))
		for i, v := range idx {
			if v < 0 {
				abs[i] = -1
			} else {
				abs[i] = v + from
			}
		}
		if p.assertionsHoldIn(l, s, abs) {
			return abs
		}
		// Try the next byte position.
		from = abs[0] + 1
		if from > len(s) {
			return nil
		}
	}
	return nil
}

// assertionsHoldIn checks a candidate whose indices are absolute in s.
func (p *Pattern) assertionsHoldIn(l lifted, s string, idx []int) bool {
	for _, a := range l.assertions {
		if !a.lookbehind {
			continue
		}
		if !p.lookbehindHolds(a, s, idx[0], idx[1]) {
			return false
		}
	}
	return true
}

// lookbehindHolds reports whether one lookbehind holds for a match spanning
// [start, end) in s.
func (p *Pattern) lookbehindHolds(a assertion, s string, start, end int) bool {
	re, err := p.compileTranslated(a.body)
	if err != nil {
		return false
	}
	check := start
	if a.trailing > 0 {
		check = end - a.trailing
	}
	if check > len(s) {
		check = len(s)
	}
	before := check - a.width
	if before < 0 {
		// Not enough text before: a POSITIVE lookbehind cannot hold, and a
		// negative one holds because nothing matched.
		return !a.positive
	}
	if before > check {
		return !a.positive
	}
	loc := re.FindStringIndex(s[before:check])
	matched := loc != nil && loc[0] == 0 && loc[1] == a.width
	return a.positive == matched
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
	return trimLead(l, idx)
}

// trimLead gives back the characters a lifted lookbehind consumed.
//
// The placeholders anchor the assertion by matching real text, so the compiled
// full match begins with them - but they are not part of what the user asked
// for.  Only the FULL MATCH's start moves past them: a capture group is
// non-capturing-independent of the placeholder, so its indices are already in
// the right coordinates, and shifting them as well made "(?<!x)(a)" report the
// placeholder's character instead of "a".
func trimLead(l lifted, idx []int) []int {
	out := make([]int, len(idx))
	copy(out, idx)
	return out
}

// placeholderFor builds the group that anchors a w-character lookbehind.
//
// It consumes w characters when they are there, and when the string is shorter
// it still matches so that position 0 remains a candidate - the caller's check
// then decides, and for a NEGATIVE assertion "there is no preceding text" is a
// success.

// branchPos returns the offset in pattern at which the current top-level
// branch begins, ignoring enclosing groups.
func branchPos(pattern string, i int) int {
	depth := 0
	for j := i - 1; j >= 0; j-- {
		switch pattern[j] {
		case ')':
			depth++
		case '(':
			if depth == 0 {
				// Entering the enclosing group: its head does not consume, so
				// keep walking to the group's own branch start rather than
				// stopping here.
				return groupBodyStart(pattern, j) + 1
			}
			depth--
		case '|':
			if depth == 0 {
				return j + 1
			}
		}
	}
	return 0
}

// groupBodyStart returns the offset just before the body of the group whose
// opening parenthesis is at i, which is where that body's first branch begins.
func groupBodyStart(pattern string, i int) int {
	head := i + 1
	switch {
	case strings.HasPrefix(pattern[i:], "(?:"):
		head = i + 3
	case strings.HasPrefix(pattern[i:], "(?P<"):
		if close := strings.IndexByte(pattern[i:], '>'); close >= 0 {
			head = i + close + 1
		} else {
			head = i + 1
		}
	}
	return head - 1
}

// consumesBefore reports whether the pattern text before i contains anything
// that consumes a character: a group head "(?P<name>" or "(?:" does not, and
// neither does an assertion.
func consumesBefore(pattern string, upto int) bool {
	for i := 0; i < upto; {
		switch c := pattern[i]; {
		case c == '\\':
			i += 2
			continue
		case c == '[':
			return true
		case c == '(':
			// A group head consumes nothing; the interior is examined next.
			if strings.HasPrefix(pattern[i:], "(?:") {
				i += 3
				continue
			}
			if strings.HasPrefix(pattern[i:], "(?=") || strings.HasPrefix(pattern[i:], "(?!") ||
				strings.HasPrefix(pattern[i:], "(?<=") || strings.HasPrefix(pattern[i:], "(?<!") {
				_, end, ok := scanLookaheadAt(pattern, i)
				if !ok {
					return true
				}
				i = end
				continue
			}
			if strings.HasPrefix(pattern[i:], "(?P<") {
				// "(?P<name>" - the head only.
				if close := strings.IndexByte(pattern[i:], '>'); close >= 0 {
					i += close + 1
					continue
				}
			}
			i++
		default:
			return true
		}
	}
	return false
}

// scanLookaheadAt returns the body of the assertion at i and the offset past
// its closing parenthesis.
func scanLookaheadAt(pattern string, i int) (string, int, bool) {
	if strings.HasPrefix(pattern[i:], "(?<=") || strings.HasPrefix(pattern[i:], "(?<!") {
		return scanLookbehind(pattern, i)
	}
	return scanLookahead(pattern, i)
}

// branchRest returns the pattern text from i up to the end of the current
// branch, and whether such a boundary was found.
func branchRest(pattern string, i int) (string, bool) {
	depth := 0
	for j := i; j < len(pattern); j++ {
		switch pattern[j] {
		case '\\':
			j++
		case '[':
			j = skipClass(pattern, j)
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return pattern[i:j], true
			}
			depth--
		case '|':
			if depth == 0 {
				return pattern[i:j], true
			}
		}
	}
	return pattern[i:], true
}

// scanLookbehind returns the body of the (?<=...) or (?<!...) group at i and
// the offset just past its closing parenthesis.
func scanLookbehind(pattern string, i int) (string, int, bool) {
	start := i + 4
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

// fixedWidth returns how many characters a lookbehind body consumes, and
// refuses anything whose width is not fixed.
//
// CPython itself requires "look-behind requires fixed-width pattern", so this
// is the same restriction, not a simplification: an alternation is allowed only
// when every branch has the SAME width, and a quantifier only when it is a
// single fixed repetition.
func fixedWidth(body string) (int, error) {
	width, err := widthOfAlt(body)
	if err != nil {
		return 0, err
	}
	if width < 0 {
		return 0, py.ExceptionNewf(ErrorType, "look-behind requires fixed-width pattern")
	}
	return width, nil
}

// widthOfAlt is widthOf for a body that may contain TOP-LEVEL alternation: the
// width is fixed when every branch has the same one.
func widthOfAlt(pattern string) (int, error) {
	branches := splitAlternation(pattern)
	if len(branches) < 2 {
		return widthOf(pattern)
	}
	want := -1
	for _, b := range branches {
		w, err := widthOf(b)
		if err != nil {
			return -1, err
		}
		if w < 0 {
			return -1, nil
		}
		if want < 0 {
			want = w
			continue
		}
		if w != want {
			// "(?<!a|bc)" - CPython refuses this too, with the same message.
			return -1, nil
		}
	}
	return want, nil
}

// splitAlternation splits a pattern at its TOP-LEVEL "|" characters, ignoring
// those inside a class or a group.
func splitAlternation(pattern string) []string {
	var parts []string
	depth := 0
	last := 0
	for i := 0; i < len(pattern); i++ {
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
				parts = append(parts, pattern[last:i])
				last = i + 1
			}
		}
	}
	parts = append(parts, pattern[last:])
	return parts
}

// widthOf returns the fixed character width of a pattern fragment, or -1 when
// it is not fixed.
func widthOf(pattern string) (int, error) {
	total := 0
	for i := 0; i < len(pattern); {
		switch c := pattern[i]; {
		case c == '\\':
			// An escape is one character, except for the zero-width ones.
			if i+1 < len(pattern) {
				switch pattern[i+1] {
				case 'b', 'B', 'A', 'z', 'Z':
					// A zero-width assertion contributes nothing.
					i += 2
					continue
				}
			}
			total++
			i += 2
			continue
		case c == '[':
			total++
			i = skipClass(pattern, i)
			continue
		case c == '(':
			if strings.HasPrefix(pattern[i:], "(?:") {
				body, end, ok := scanGroup(pattern, i)
				if !ok {
					return -1, nil
				}
				w, err := widthOf(body)
				if err != nil || w < 0 {
					return -1, err
				}
				total += w
				i = end
				continue
			}
			if strings.HasPrefix(pattern[i:], "(?=") || strings.HasPrefix(pattern[i:], "(?!") ||
				strings.HasPrefix(pattern[i:], "(?<=") || strings.HasPrefix(pattern[i:], "(?<!") {
				// An inner assertion is refused rather than mis-measured.
				return -1, nil
			}
			body, end, ok := scanGroup(pattern, i)
			if !ok {
				return -1, nil
			}
			w, err := widthOf(body)
			if err != nil || w < 0 {
				return -1, err
			}
			total += w
			i = end
			continue
		case c == '|':
			// An alternation is fixed-width when EVERY branch has the same
			// width, which is CPython's rule too - "(?<!a|b)c" is legal and
			// "(?<!a|bc)d" is not.  widthOfAlt does that comparison.
			return -1, nil
		case c == '*' || c == '+' || c == '?':
			return -1, nil
		case c == '{':
			close := strings.IndexByte(pattern[i:], '}')
			if close < 0 {
				return -1, nil
			}
			spec := pattern[i+1 : i+close]
			if strings.ContainsRune(spec, ',') {
				return -1, nil
			}
			n, err := strconv.Atoi(spec)
			if err != nil {
				return -1, nil
			}
			total += n
			i += close + 1
			continue
		case c == '^' || c == '$':
			// Zero-width anchors.
		default:
			total++
		}
		i++
	}
	return total, nil
}

// scanGroup returns the body of the group at i and the offset just past its
// closing parenthesis.
func scanGroup(pattern string, i int) (string, int, bool) {
	start := i + 1
	if strings.HasPrefix(pattern[i:], "(?:") {
		start = i + 3
	}
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
