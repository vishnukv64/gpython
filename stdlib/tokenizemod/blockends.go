package tokenizemod

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// BlockEnds maps the line of each "def" / "class" statement in src to the last
// line of its block.  The parser records no end positions, so ast.parse takes
// them from here (pyclbr reads end_lineno).  src must already have parsed.
func BlockEnds(src string) map[int]int {
	lines := strings.SplitAfter(src, "\n")
	i := 0
	it := &tokenizerIter{t: newTokState(func() (py.Object, error) {
		if i >= len(lines) {
			return nil, py.StopIteration
		}
		i++
		return py.String(lines[i-1]), nil
	}, "", true)}

	type open struct{ line, depth int }
	var stack []open
	ends := map[int]int{}
	depth, lastNewline := 0, 0
	lineStart, pushed, header, afterAsync := true, false, false, false
	for {
		o, err := it.M__next__()
		if err != nil {
			return ends
		}
		tok := o.(py.Tuple)
		typ, text, line := int(tok[0].(py.Int)), string(tok[1].(py.String)), int(tok[2].(py.Tuple)[0].(py.Int))
		if typ == tNL || typ == tCOMMENT {
			continue
		}
		if typ == tENDMARKER {
			for _, e := range stack {
				ends[e.line] = lastNewline
			}
			return ends
		}
		if header {
			header = false
			if typ != tINDENT { // one-line body: the block ended with the header line
				ends[stack[len(stack)-1].line] = lastNewline
				stack = stack[:len(stack)-1]
			}
		}
		switch typ {
		case tINDENT:
			depth++
			continue
		case tDEDENT:
			depth--
			for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
				ends[stack[len(stack)-1].line] = lastNewline
				stack = stack[:len(stack)-1]
			}
			continue
		case tNEWLINE:
			lastNewline = line
			lineStart, header, pushed, afterAsync = true, pushed, false, false
			continue
		}
		if lineStart && typ == tNAME && text == "async" {
			afterAsync = true
			lineStart = false
			continue
		}
		if (lineStart || afterAsync) && typ == tNAME && (text == "def" || text == "class") {
			stack = append(stack, open{line, depth})
			pushed = true
		}
		lineStart, afterAsync = false, false
	}
}
