package re

import (
	"testing"

	"github.com/vishnukv64/gpython/py"
)

// compileFor compiles a pattern the way the module does, without a Python
// context, so the lookahead machinery can be exercised directly.
func compileFor(t *testing.T, pattern string, flags int) *Pattern {
	t.Helper()
	p, err := compile(pattern, flags)
	if err != nil {
		t.Fatalf("compile(%q): %v", pattern, err)
	}
	return p
}

// fullMatches is a helper: does the pattern match the whole string?
func fullMatches(t *testing.T, p *Pattern, s string) bool {
	t.Helper()
	m := fullMatchAt(p, s, 0)
	return m != nil
}

func searches(t *testing.T, p *Pattern, s string) bool {
	t.Helper()
	m := searchIn(p, s, 0)
	return m != nil
}

// TestNegativeLookaheadInRepeat pins the packaging pattern, which is the reason
// lookahead is supported at all: pip cannot import without it.
//
// Every expectation is CPython 3.14's, and the whole language over {a,1,-,!} up
// to length 5 was checked against CPython separately (1365 strings, 0
// disagreements).  The handful here are the boundaries that exercise the
// assertion rather than the surrounding pattern.
func TestNegativeLookaheadInRepeat(t *testing.T) {
	const name = `[a-z0-9]|[a-z0-9]([a-z0-9-](?!--))*[a-z0-9]`
	p := compileFor(t, name, FlagASCII)

	accept := []string{
		"abc", "a-b", "a--b", "a--a", "a-b-c", "a", "1", "ab", "a-1", "9", "x",
		"a-a",
		// "1--1" is ACCEPTED by CPython, counter-intuitive as it looks: the
		// trailing alternative "[a-z0-9]" takes the first character and the
		// repeat group then handles "-", assertion-checked, before the final
		// "[a-z0-9]".  Listing it as rejected was MY error, caught by running
		// the test - the implementation was right.
		"1--1", "1--a", "a--1",
	}
	for _, s := range accept {
		if !fullMatches(t, p, s) {
			t.Errorf("fullmatch(%q) = false, CPython says true", s)
		}
	}

	reject := []string{
		"", "-", "--", "a--", "--a", "a--b--c", "!", "aa--a", "a---", "1---",
	}
	for _, s := range reject {
		if fullMatches(t, p, s) {
			t.Errorf("fullmatch(%q) = true, CPython says false", s)
		}
	}
}

// TestNegativeLookaheadPositions covers a negative lookahead at the end of a
// branch and at the start of the pattern.
func TestNegativeLookaheadPositions(t *testing.T) {
	end := compileFor(t, `a(?!b)`, 0)
	if !searches(t, end, "a") || searches(t, end, "ab") || !searches(t, end, "ac") {
		t.Error(`a(?!b) does not match CPython on "a" / "ab" / "ac"`)
	}

	word := compileFor(t, `foo(?!bar)`, 0)
	if !searches(t, word, "foo") || searches(t, word, "foobar") || !searches(t, word, "foobaz") {
		t.Error(`foo(?!bar) does not match CPython`)
	}

	start := compileFor(t, `(?!x)a`, 0)
	if !searches(t, start, "a") || !searches(t, start, "xa") {
		t.Error(`(?!x)a does not match CPython`)
	}
}

// TestUnsupportedLookaroundRefuses is the important one: a shape that cannot be
// checked exactly must raise rather than answer.  "\d+(?!%)" needs the engine
// to RETREAT from "12" to "1" to satisfy the assertion, which this
// implementation cannot do - and returning "no match" there would be a wrong
// answer, which is worse than a refusal.
func TestUnsupportedLookaroundRefuses(t *testing.T) {
	for _, pattern := range []string{
		`\d+(?!%)`,     // quantified prefix: needs backtracking
		`[a-z]+(?=\.)`, // positive lookahead
		`a(?=b)`,       // positive lookahead
		`(?<=a)b`,      // lookbehind
		`a(?(1)b|c)`,   // conditional
	} {
		if _, err := compile(pattern, 0); err == nil {
			t.Errorf("compile(%q) succeeded, but this shape cannot be checked exactly", pattern)
		} else if e, ok := err.(*py.Exception); ok {
			if msg, _ := py.StrAsString(e); len(msg) == 0 {
				t.Errorf("compile(%q): refusal has no message", pattern)
			}
		}
	}
}

// TestNoLookaheadUnaffected checks the common path still works and is not
// routed through any of the assertion machinery.
func TestNoLookaheadUnaffected(t *testing.T) {
	p := compileFor(t, `[a-z]+`, FlagASCII)
	if p.lifted != nil {
		t.Error("a pattern with no lookahead was lifted, which costs work for nothing")
	}
	if !searches(t, p, "abc123") || searches(t, p, "123") {
		t.Error("[a-z]+ does not match CPython")
	}
}
