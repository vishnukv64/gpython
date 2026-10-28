package re

import (
	"regexp"
	"strings"

	"github.com/dlclark/regexp2"
	"github.com/vishnukv64/gpython/py"
)

// A pattern with a positive lookahead cannot be run on Go's RE2 engine (see
// lookahead.go for the shapes that can be emulated). Such patterns fall back
// to a backtracking engine, regexp2. Named groups are turned into plain ones
// so group numbering stays positional, as in Python.
type backtrack struct {
	search   *regexp2.Regexp
	anchored *regexp2.Regexp
}

var namedGroup = regexp.MustCompile(`\(\?P<[A-Za-z_][A-Za-z0-9_]*>`)

func newBacktrack(translated string, flags int) (*backtrack, error) {
	body := namedGroup.ReplaceAllString(translated, "(")
	if prefix := goFlags(flags); prefix != "" {
		body = "(?" + prefix + ":" + body + ")"
	}
	s, err := regexp2.Compile(body, regexp2.None)
	if err != nil {
		return nil, py.ExceptionNewf(ErrorType, "%s", err.Error())
	}
	a, err := regexp2.Compile(`\A(?:`+body+`)`, regexp2.None)
	if err != nil {
		return nil, py.ExceptionNewf(ErrorType, "%s", err.Error())
	}
	return &backtrack{s, a}, nil
}

// find returns Go-style byte-index submatches, or nil.
func (b *backtrack) find(text string, anchored, whole bool) []int {
	re := b.search
	if anchored {
		re = b.anchored
	}
	m, err := re.FindStringMatch(text)
	if err != nil || m == nil {
		return nil
	}
	// regexp2 reports rune offsets; convert to byte offsets.
	byteAt := make([]int, 0, len(text)+1)
	for i := range text {
		byteAt = append(byteAt, i)
	}
	byteAt = append(byteAt, len(text))
	gs := m.Groups()
	idx := make([]int, 0, 2*len(gs))
	for _, g := range gs {
		if len(g.Captures) == 0 {
			idx = append(idx, -1, -1)
			continue
		}
		c := g.Captures[len(g.Captures)-1]
		idx = append(idx, byteAt[c.Index], byteAt[c.Index+c.Length])
	}
	if whole && idx[1] != len(text) {
		return nil
	}
	return idx
}

func needsBacktrack(translated string) bool {
	// An atomic group "(?>...)" (Python 3.11) is the other construct only a
	// backtracking engine has; CPython's own fnmatch.translate emits one.
	return strings.Contains(translated, "(?=") || strings.Contains(translated, "(?>")
}
