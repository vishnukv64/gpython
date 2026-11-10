package re

import (
	"regexp/syntax"
	"strings"
	"unicode/utf8"
)

// A scanner runs successive leftmost-first searches over ONE text with a
// memoising backtracker (the algorithm of Go's regexp backtracker) whose
// visited set is sized to the text and cleared only over the span a search
// touched. Go's own engines either cap the input size (backtracker) or are
// several times slower (NFA), and restart their setup per call; findall/sub
// over a large text needs neither limit.
type scanner struct {
	prog     *syntax.Prog
	text     string
	ni       int // instructions per position in the visited bitmap
	visited  []uint32
	maxPos   int
	jobs     []job
	cap      []int
	matchcap []int
	prefix   string
	anchored bool // the pattern can only match at the text start
}

type job struct {
	pc  uint32
	arg bool
	pos int
}

// maxScanBits bounds the visited bitmap (32 MiB); bigger texts use the
// regexp package's own engine.
const maxScanBits = 1 << 28

type progInfo struct {
	prog *syntax.Prog
	ncap int
}

// scanProg parses the pattern the way regexp.Compile does.
func (p *Pattern) scanProg() *progInfo {
	p.progOnce.Do(func() {
		if p.re == nil {
			return
		}
		re, err := syntax.Parse(p.re.String(), syntax.Perl)
		if err != nil {
			return
		}
		ncap := 2 * (re.MaxCap() + 1)
		prog, err := syntax.Compile(re.Simplify())
		if err != nil {
			return
		}
		p.progInfo = &progInfo{prog, ncap}
	})
	return p.progInfo
}

// newScanner returns nil when the pattern or text must use the regexp engine.
func (p *Pattern) newScanner(text string) *scanner {
	if p.bt != nil || p.lifted != nil {
		return nil
	}
	info := p.scanProg()
	if info == nil {
		return nil
	}
	ni := len(info.prog.Inst)
	bits := ni * (len(text) + 1)
	if bits > maxScanBits {
		return nil
	}
	s := &scanner{
		prog:     info.prog,
		text:     text,
		ni:       ni,
		visited:  make([]uint32, (bits+31)/32),
		cap:      make([]int, info.ncap),
		matchcap: make([]int, info.ncap),
	}
	s.prefix, _ = info.prog.Prefix()
	s.anchored = info.prog.StartCond()&syntax.EmptyBeginText != 0
	return s
}

func (s *scanner) shouldVisit(pc uint32, pos int) bool {
	n := pos*s.ni + int(pc)
	if s.visited[n/32]&(1<<(n&31)) != 0 {
		return false
	}
	s.visited[n/32] |= 1 << (n & 31)
	if pos > s.maxPos {
		s.maxPos = pos
	}
	return true
}

func (s *scanner) push(pc uint32, pos int, arg bool) {
	if s.prog.Inst[pc].Op != syntax.InstFail && (arg || s.shouldVisit(pc, pos)) {
		s.jobs = append(s.jobs, job{pc, arg, pos})
	}
}

// find returns the submatch byte indices of the first match starting at or
// after from, or nil. Context (\b, ^, $) is read from the whole text.
func (s *scanner) find(from int) []int {
	defer s.reset(from)
	for i := range s.matchcap {
		s.matchcap[i] = -1
		s.cap[i] = -1
	}
	s.maxPos = from
	end := len(s.text)
	for pos, width := from, 1; pos <= end && width != 0; pos += width {
		if s.anchored && pos > 0 {
			return nil
		}
		if s.prefix != "" {
			adv := strings.Index(s.text[pos:], s.prefix)
			if adv < 0 {
				return nil
			}
			pos += adv
		}
		s.cap[0] = pos
		if s.try(uint32(s.prog.Start), pos) {
			return s.matchcap // valid until the next find
		}
		if pos < end {
			_, width = utf8.DecodeRuneInString(s.text[pos:])
		} else {
			width = 0
		}
	}
	return nil
}

// reset clears the visited bits a search touched.
func (s *scanner) reset(from int) {
	lo := from * s.ni / 32
	hi := min(((s.maxPos+1)*s.ni)/32+1, len(s.visited))
	clear(s.visited[lo:hi])
}

func (s *scanner) context(pos int) syntax.EmptyOp {
	r1, r2 := rune(-1), rune(-1)
	if pos > 0 {
		r1, _ = utf8.DecodeLastRuneInString(s.text[:pos])
	}
	if pos < len(s.text) {
		r2, _ = utf8.DecodeRuneInString(s.text[pos:])
	}
	return syntax.EmptyOpContext(r1, r2)
}

func (s *scanner) try(pc uint32, pos int) bool {
	s.jobs = s.jobs[:0]
	s.push(pc, pos, false)
	for len(s.jobs) > 0 {
		l := len(s.jobs) - 1
		pc, pos, arg := s.jobs[l].pc, s.jobs[l].pos, s.jobs[l].arg
		s.jobs = s.jobs[:l]
		goto Skip
	CheckAndLoop:
		if !s.shouldVisit(pc, pos) {
			continue
		}
	Skip:
		inst := &s.prog.Inst[pc]
		switch inst.Op {
		case syntax.InstAlt, syntax.InstAltMatch:
			if arg {
				arg = false
				pc = inst.Arg
				goto CheckAndLoop
			}
			s.push(pc, pos, true)
			pc = inst.Out
			goto CheckAndLoop
		case syntax.InstRune:
			if pos >= len(s.text) {
				continue
			}
			r, w := utf8.DecodeRuneInString(s.text[pos:])
			if !inst.MatchRune(r) {
				continue
			}
			pos += w
			pc = inst.Out
			goto CheckAndLoop
		case syntax.InstRune1:
			if pos >= len(s.text) {
				continue
			}
			r, w := utf8.DecodeRuneInString(s.text[pos:])
			if r != inst.Rune[0] {
				continue
			}
			pos += w
			pc = inst.Out
			goto CheckAndLoop
		case syntax.InstRuneAny:
			if pos >= len(s.text) {
				continue
			}
			_, w := utf8.DecodeRuneInString(s.text[pos:])
			pos += w
			pc = inst.Out
			goto CheckAndLoop
		case syntax.InstRuneAnyNotNL:
			if pos >= len(s.text) {
				continue
			}
			r, w := utf8.DecodeRuneInString(s.text[pos:])
			if r == '\n' {
				continue
			}
			pos += w
			pc = inst.Out
			goto CheckAndLoop
		case syntax.InstCapture:
			if arg {
				s.cap[inst.Arg] = pos
				continue
			}
			if int(inst.Arg) < len(s.cap) {
				s.push(pc, s.cap[inst.Arg], true)
				s.cap[inst.Arg] = pos
			}
			pc = inst.Out
			goto CheckAndLoop
		case syntax.InstEmptyWidth:
			if syntax.EmptyOp(inst.Arg)&^s.context(pos) != 0 {
				continue
			}
			pc = inst.Out
			goto CheckAndLoop
		case syntax.InstNop:
			pc = inst.Out
			goto CheckAndLoop
		case syntax.InstMatch:
			s.cap[1] = pos
			copy(s.matchcap, s.cap)
			return true
		}
	}
	return false
}
