package py

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Caret lines under a traceback's source line ("~~~^^^^"), as CPython 3.13's
// traceback.StackSummary.format_frame_summary prints them.  This is a port of
// that function and of _should_show_carets, _extract_caret_anchors_from_line_segment
// and _display_width.  Parsing source is the one thing the py package cannot
// do (the parser imports py), so the compiler supplies it through the hooks.

// CaretShape describes a parsed segment for the anchor search: Kind is 1 for
// a binary operation, 2 for a subscript, 3 for a call.  The positions use the
// segment as it was parsed, wrapped in "(\n" ... "\n)", so lines are 1-based
// counting that extra line and columns are UTF-8 byte offsets.
type CaretShape struct {
	Kind                          int
	LeftEndLine, LeftEndCol       int // end of the left operand / value / func
	RightStartLine, RightStartCol int // start of the right operand (binary only)
	EndLine, EndCol               int // end of the whole expression
}

var (
	// CaretShapeOf parses "(\n"+segment+"\n)" and describes it, or reports ok
	// false when it is not one expression of a kind that has anchors.
	CaretShapeOf func(segment string) (shape CaretShape, ok bool)
	// FullLineCall parses src as a module and, when its first statement is
	// "return f(...)" or "name = f(...)" with f a plain name, returns the
	// position of that call.
	FullLineCall func(src string) (line, endLine, col, endCol int, ok bool)
	// EastAsianWide reports whether r is East Asian Wide or Fullwidth.
	EastAsianWide func(r rune) bool
)

func pyIsSpace(r rune) bool { return unicode.IsSpace(r) }

func pyRStrip(s string) string { return strings.TrimRightFunc(s, pyIsSpace) }

func pyLStrip(s string) string { return strings.TrimLeftFunc(s, pyIsSpace) }

// byteToCharOffset is _byte_offset_to_character_offset.
func byteToCharOffset(s string, off int) int {
	if off > len(s) {
		off = len(s)
	}
	if off < 0 {
		off = 0
	}
	return utf8.RuneCountInString(s[:off])
}

// displayWidth is _display_width: the width of the first offset characters of
// line (the whole line when offset < 0).
func displayWidth(line string, offset int) int {
	r := []rune(line)
	if offset < 0 || offset > len(r) {
		offset = len(r)
	}
	ascii := true
	for _, c := range r {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii || EastAsianWide == nil {
		return offset
	}
	w := 0
	for _, c := range r[:offset] {
		if EastAsianWide(c) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// dedent is textwrap.dedent over lines: the margin common to the non-blank
// lines is removed, and blank lines become empty.
func dedent(lines []string) []string {
	margin, set := "", false
	for _, l := range lines {
		if strings.Trim(l, " \t") == "" {
			continue
		}
		ind := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if !set {
			margin, set = ind, true
			continue
		}
		n := 0
		for n < len(margin) && n < len(ind) && margin[n] == ind[n] {
			n++
		}
		margin = margin[:n]
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if strings.Trim(l, " \t") == "" {
			out[i] = ""
		} else {
			out[i] = strings.TrimPrefix(l, margin)
		}
	}
	return out
}

type anchors struct {
	leftEndLine, leftEndOff, rightStartLine, rightStartOff int
	primary, secondary                                     byte
}

// extractAnchors is _extract_caret_anchors_from_line_segment.
func extractAnchors(segment string) *anchors {
	if CaretShapeOf == nil {
		return nil
	}
	shape, ok := CaretShapeOf(segment)
	if !ok {
		return nil
	}
	lines := splitLinesKeep(segment)
	if len(lines) == 0 {
		return nil
	}
	line := func(n int) []rune {
		if n < 0 || n >= len(lines) {
			return nil
		}
		return []rune(lines[n])
	}
	normalize := func(lineno, offset int) int {
		if lineno < 0 || lineno >= len(lines) {
			return 0
		}
		return byteToCharOffset(lines[lineno], offset)
	}
	nextValid := func(lineno, col int) (int, int, bool) {
		for lineno < len(lines) && col >= len(line(lineno)) {
			col = 0
			lineno++
		}
		if lineno >= len(lines) {
			return 0, 0, false
		}
		return lineno, col, true
	}
	increment := func(lineno, col int) (int, int, bool) { return nextValid(lineno, col+1) }
	nextLine := func(lineno, col int) (int, int, bool) { return nextValid(lineno+1, 0) }
	incrementUntil := func(lineno, col int, stop func(rune) bool) (int, int, bool) {
		for {
			if lineno >= len(lines) || col >= len(line(lineno)) {
				return 0, 0, false
			}
			ch := line(lineno)[col]
			var ok bool
			switch {
			case ch == '\\' || ch == '#':
				lineno, col, ok = nextLine(lineno, col)
			case !stop(ch):
				lineno, col, ok = increment(lineno, col)
			default:
				return lineno, col, true
			}
			if !ok {
				return 0, 0, false
			}
		}
	}
	// setup_positions: -2 since end_lineno is 1-based and the segment was
	// wrapped in an extra bracket and newline.
	setup := func(endLine, endCol int, force bool) (int, int, bool) {
		lineno := endLine - 2
		col := normalize(lineno, endCol)
		if force {
			return nextValid(lineno, col)
		}
		return lineno, col, true
	}
	switch shape.Kind {
	case 1:
		lineno, col, ok := setup(shape.LeftEndLine, shape.LeftEndCol, true)
		if !ok {
			return nil
		}
		lineno, col, ok = incrementUntil(lineno, col, func(x rune) bool { return !unicode.IsSpace(x) && x != ')' })
		if !ok {
			return nil
		}
		rightCol := col + 1
		l := line(lineno)
		if rightCol < len(l) &&
			(shape.RightStartLine-2 > lineno || rightCol < normalize(shape.RightStartLine-2, shape.RightStartCol)) &&
			!unicode.IsSpace(l[rightCol]) && l[rightCol] != '\\' && l[rightCol] != '#' {
			rightCol++
		}
		return &anchors{lineno, col, lineno, rightCol, '~', '^'}
	case 2, 3:
		open := '['
		if shape.Kind == 3 {
			open = '('
		}
		ll, lc, ok := setup(shape.LeftEndLine, shape.LeftEndCol, true)
		if !ok {
			return nil
		}
		ll, lc, ok = incrementUntil(ll, lc, func(x rune) bool { return x == open })
		if !ok {
			return nil
		}
		rl, rc, _ := setup(shape.EndLine, shape.EndCol, false)
		return &anchors{ll, lc, rl, rc, '~', '^'}
	}
	return nil
}

// splitLinesKeep is str.splitlines() without the line ends.
func splitLinesKeep(s string) []string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// shouldShowCarets is StackSummary._should_show_carets.
func shouldShowCarets(startOffset, endOffset int, allLines []string, a *anchors) bool {
	if FullLineCall != nil {
		if line, endLine, col, endCol, ok := FullLineCall(strings.Join(allLines, "\n")); ok {
			if line == 1 && endLine == len(allLines) && col == startOffset && endCol == endOffset {
				return false
			}
		}
	}
	if a != nil {
		return true
	}
	first, last := []rune(allLines[0]), []rune(allLines[len(allLines)-1])
	if startOffset <= len(first) && pyLStrip(string(first[:startOffset])) != "" {
		return true
	}
	if endOffset <= len(last) && pyRStrip(string(last[endOffset:])) != "" {
		return true
	}
	return false
}

// frameSourceLines is the part of a File entry after its header: the source
// line(s) and the caret line(s), indented by four spaces, as the strings
// format_frame_summary appends to the row.
func frameSourceLines(filename string, lineno, endLineno, colno, endColno int, hasCols bool) []string {
	if endLineno < lineno {
		endLineno = lineno
	}
	var lines []string
	any := false
	for n := lineno; n <= endLineno; n++ {
		l, ok := SourceLine(filename, n)
		if ok {
			any = true
		}
		lines = append(lines, pyRStrip(l))
	}
	if !any {
		return nil
	}
	original := lines
	dedented := dedent(original)
	if strings.TrimSpace(strings.Join(dedented, "\n")) == "" {
		return nil
	}
	if !hasCols {
		return []string{"    " + strings.TrimSpace(original[0]) + "\n"}
	}
	firstLine := original[0]
	lastLine := original[endLineno-lineno]
	startOffset := byteToCharOffset(firstLine, colno)
	endOffset := byteToCharOffset(lastLine, endColno)
	allLines := dedented[:endLineno-lineno+1]
	dedentChars := len([]rune(firstLine)) - len([]rune(allLines[0]))
	startOffset = max(0, startOffset-dedentChars)
	endOffset = max(0, endOffset-dedentChars)

	dpStart := displayWidth(allLines[0], startOffset)
	dpEnd := displayWidth(allLines[len(allLines)-1], endOffset)

	// the exact code segment of the instruction
	joined := []rune(strings.Join(allLines, "\n"))
	lastLen := len([]rune(allLines[len(allLines)-1]))
	segStart, segEnd := min(startOffset, len(joined)), len(joined)-(lastLen-endOffset)
	if segEnd < segStart {
		segEnd = segStart
	}
	segment := string(joined[segStart:segEnd])

	a := extractAnchors(segment)
	show := shouldShowCarets(startOffset, endOffset, allLines, a)

	significant := map[int]bool{0: true, len(allLines) - 1: true}
	leftEnd, rightStart := 0, 0
	primary, secondary := byte('^'), byte('^')
	if a != nil {
		leftEnd, rightStart = a.leftEndOff, a.rightStartOff
		if a.leftEndLine == 0 {
			leftEnd += startOffset
		}
		if a.rightStartLine == 0 {
			rightStart += startOffset
		}
		if a.leftEndLine < len(allLines) && a.rightStartLine < len(allLines) {
			leftEnd = displayWidth(allLines[a.leftEndLine], leftEnd)
			rightStart = displayWidth(allLines[a.rightStartLine], rightStart)
		}
		primary, secondary = a.primary, a.secondary
		for d := -1; d <= 1; d++ {
			significant[a.leftEndLine+d] = true
			significant[a.rightStartLine+d] = true
		}
	}
	delete(significant, -1)
	delete(significant, len(allLines))

	var result []string
	outputLine := func(n int) {
		result = append(result, allLines[n]+"\n")
		if !show {
			return
		}
		numSpaces := len([]rune(allLines[n])) - len([]rune(pyLStrip(allLines[n])))
		numCarets := displayWidth(allLines[n], -1)
		if n == len(allLines)-1 {
			numCarets = dpEnd
		}
		carets := make([]byte, 0, numCarets)
		for col := 0; col < numCarets; col++ {
			switch {
			case col < numSpaces || (n == 0 && col < dpStart):
				carets = append(carets, ' ')
			case a != nil &&
				(n > a.leftEndLine || (n == a.leftEndLine && col >= leftEnd)) &&
				(n < a.rightStartLine || (n == a.rightStartLine && col < rightStart)):
				carets = append(carets, secondary)
			default:
				carets = append(carets, primary)
			}
		}
		result = append(result, string(carets)+"\n")
	}
	var order []int
	for n := range significant {
		order = append(order, n)
	}
	sort.Ints(order)
	for i, n := range order {
		if i > 0 {
			switch diff := n - order[i-1]; {
			case diff == 2:
				outputLine(n - 1)
			case diff > 2:
				result = append(result, "...<"+itoa(diff-1)+" lines>...\n")
			}
		}
		outputLine(n)
	}

	// textwrap.indent(textwrap.dedent("".join(result)), "    ", lambda line: True)
	text := strings.TrimSuffix(strings.Join(result, ""), "\n")
	rows := dedent(strings.Split(text, "\n"))
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = "    " + r + "\n"
	}
	return out
}
