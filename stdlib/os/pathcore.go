package os

// Pure string implementations of CPython's posixpath and ntpath.  Neither
// touches path/filepath, whose behaviour depends on GOOS: both flavours must
// give the same answer on every host.  Mirrors Lib/posixpath.py and
// Lib/ntpath.py (3.14); pathgolden_test.go pins the output to CPython's.

import (
	"errors"
	"os"
	"os/user"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

// pathErr is a ValueError message, worded as CPython words it.
type pathErr string

func (e pathErr) Error() string { return string(e) }

var (
	errMixAbs  = pathErr("Can't mix absolute and relative paths")
	errMixRoot = pathErr("Can't mix rooted and not-rooted paths")
	errDrive   = pathErr("Paths don't have the same drive")
)

func errMount(path, start string) error {
	p, _ := py.ReprAsString(py.String(path))
	s, _ := py.ReprAsString(py.String(start))
	return errors.New("path is on mount " + p + ", start on mount " + s)
}

const ntSeps = `\/`

func isASCIIWord(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isNTSep(c byte) bool { return c == '\\' || c == '/' }

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// firstCharLen is the byte length of the first character of s (s non-empty).
func firstCharLen(s string) int {
	w := 1
	if s[0] >= 0x80 {
		for w < len(s) && s[w]&0xC0 == 0x80 {
			w++
		}
	}
	return w
}

// splitExt is genericpath._splitext.
func splitExt(p string, altsep bool, sepc byte) (string, string) {
	sep := strings.LastIndexByte(p, sepc)
	if altsep {
		if a := strings.LastIndexByte(p, '/'); a > sep {
			sep = a
		}
	}
	dot := strings.LastIndexByte(p, '.')
	if dot > sep {
		for i := sep + 1; i < dot; i++ {
			if p[i] != '.' {
				return p[:dot], p[dot:]
			}
		}
	}
	return p, ""
}

// lessStrs is Python's list ordering on lists of str.
func lessStrs(a, b []string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// commonComps is the tail of commonpath: the leading components of first
// that min(lists) and max(lists) share.
func commonComps(first []string, lists [][]string) []string {
	s1, s2 := lists[0], lists[0]
	for _, l := range lists[1:] {
		if lessStrs(l, s1) {
			s1 = l
		}
		if lessStrs(s2, l) {
			s2 = l
		}
	}
	for i, c := range s1 {
		if c != s2[i] {
			return first[:min(i, len(first))]
		}
	}
	return first[:min(len(s1), len(first))]
}

func nonDot(p, sep string) []string {
	var out []string
	for _, c := range strings.Split(p, sep) {
		if c != "" && c != "." {
			out = append(out, c)
		}
	}
	return out
}

// relList is the shared tail of relpath: ".." for what is left of start, then
// what is left of path.
func relList(startRest, pathRest, sep string, eq func(a, b string) bool) string {
	var sl, pl []string
	if startRest != "" {
		sl = strings.Split(startRest, sep)
	}
	if pathRest != "" {
		pl = strings.Split(pathRest, sep)
	}
	i := 0
	for i < len(sl) && i < len(pl) && eq(sl[i], pl[i]) {
		i++
	}
	var out []string
	for j := i; j < len(sl); j++ {
		out = append(out, "..")
	}
	out = append(out, pl[i:]...)
	if len(out) == 0 {
		return "."
	}
	return strings.Join(out, sep)
}

// ---------------------------------------------------------------- posix

func posixSplitroot(p string) (drive, root, tail string) {
	if p == "" || p[0] != '/' {
		return "", "", p
	}
	if len(p) < 2 || p[1] != '/' || len(p) > 2 && p[2] == '/' {
		return "", "/", p[1:]
	}
	return "", "//", p[2:]
}

func posixJoin(parts []string) string {
	path := parts[0]
	for _, b := range parts[1:] {
		switch {
		case strings.HasPrefix(b, "/"):
			path = b
		case path == "" || strings.HasSuffix(path, "/"):
			path += b
		default:
			path += "/" + b
		}
	}
	return path
}

func posixSplit(p string) (head, tail string) {
	i := strings.LastIndexByte(p, '/') + 1
	head, tail = p[:i], p[i:]
	if strings.Trim(head, "/") != "" {
		head = strings.TrimRight(head, "/")
	}
	return
}

func posixNormpath(p string) string {
	if p == "" {
		return "."
	}
	_, initial, rest := posixSplitroot(p)
	var comps []string
	for _, c := range strings.Split(rest, "/") {
		if c == "" || c == "." {
			continue
		}
		n := len(comps)
		if c != ".." || initial == "" && n == 0 || n > 0 && comps[n-1] == ".." {
			comps = append(comps, c)
		} else if n > 0 {
			comps = comps[:n-1]
		}
	}
	if out := initial + strings.Join(comps, "/"); out != "" {
		return out
	}
	return "."
}

func posixCommonpath(paths []string) (string, error) {
	abs := strings.HasPrefix(paths[0], "/")
	lists := make([][]string, len(paths))
	for i, p := range paths {
		if strings.HasPrefix(p, "/") != abs {
			return "", errMixAbs
		}
		lists[i] = nonDot(p, "/")
	}
	out := strings.Join(commonComps(lists[0], lists), "/")
	if abs {
		out = "/" + out
	}
	return out, nil
}

func posixAbs(p, cwd string) string {
	if !strings.HasPrefix(p, "/") {
		p = posixJoin([]string{cwd, p})
	}
	return posixNormpath(p)
}

func posixRelpath(path, start, cwd string) string {
	st := strings.TrimLeft(posixAbs(start, cwd), "/")
	pt := strings.TrimLeft(posixAbs(path, cwd), "/")
	return relList(st, pt, "/", func(a, b string) bool { return a == b })
}

func posixExpanduser(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	i := strings.IndexByte(p[1:], '/') + 1
	if i == 0 {
		i = len(p)
	}
	var home string
	if i == 1 {
		h, ok := os.LookupEnv("HOME")
		if !ok {
			u, err := user.Current()
			if err != nil || u.HomeDir == "" {
				return p
			}
			h = u.HomeDir
		}
		home = h
	} else {
		u, err := user.Lookup(p[1:i])
		if err != nil || u.HomeDir == "" {
			return p
		}
		home = u.HomeDir
	}
	if out := strings.TrimRight(home, "/") + p[i:]; out != "" {
		return out
	}
	return "/"
}

func posixExpandvars(p string) string {
	if !strings.Contains(p, "$") {
		return p
	}
	var b strings.Builder
	for i := 0; i < len(p); {
		if p[i] != '$' {
			b.WriteByte(p[i])
			i++
			continue
		}
		j, end, name := i+1, 0, ""
		if j < len(p) && p[j] == '{' {
			if k := strings.IndexByte(p[j:], '}'); k > 0 {
				end, name = j+k+1, p[j+1:j+k]
			}
		} else {
			e := j
			for e < len(p) && isASCIIWord(p[e]) {
				e++
			}
			if e > j {
				end, name = e, p[j:e]
			}
		}
		if end == 0 {
			b.WriteByte('$')
			i++
			continue
		}
		if v, ok := os.LookupEnv(name); ok {
			b.WriteString(v)
		} else {
			b.WriteString(p[i:end])
		}
		i = end
	}
	return b.String()
}

// ---------------------------------------------------------------- nt

// ntSplitroot is ntpath.splitroot.  Offsets are bytes; every separator is
// ASCII, so they agree with CPython's character offsets except for the drive
// letter test, which looks at the second *character* (firstCharLen).
func ntSplitroot(p string) (drive, root, tail string) {
	if p == "" {
		return "", "", p
	}
	if isNTSep(p[0]) {
		if len(p) > 1 && isNTSep(p[1]) {
			start := 2
			if len(p) >= 8 && isASCII(p[:8]) && strings.EqualFold(strings.ReplaceAll(p[:8], "/", `\`), `\\?\UNC\`) {
				start = 8
			}
			i := indexNTSep(p, start)
			if i < 0 {
				return p, "", ""
			}
			j := indexNTSep(p, i+1)
			if j < 0 {
				return p, "", ""
			}
			return p[:j], p[j : j+1], p[j+1:]
		}
		return "", p[:1], p[1:]
	}
	w := firstCharLen(p)
	if len(p) > w && p[w] == ':' {
		if len(p) > w+1 && isNTSep(p[w+1]) {
			return p[:w+1], p[w+1 : w+2], p[w+2:]
		}
		return p[:w+1], "", p[w+1:]
	}
	return "", "", p
}

func indexNTSep(p string, from int) int {
	for i := from; i < len(p); i++ {
		if isNTSep(p[i]) {
			return i
		}
	}
	return -1
}

func ntNormcase(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "/", `\`))
}

func ntIsabs(s string) bool {
	if s == "" {
		return false
	}
	if len(s) >= 2 && isNTSep(s[0]) && isNTSep(s[1]) {
		return true
	}
	w := firstCharLen(s)
	return len(s) > w+1 && s[w] == ':' && isNTSep(s[w+1])
}

func ntJoin(parts []string) string {
	rd, rr, rp := ntSplitroot(parts[0])
	for _, p := range parts[1:] {
		pd, pr, pp := ntSplitroot(p)
		if pr != "" {
			if pd != "" || rd == "" {
				rd = pd
			}
			rr, rp = pr, pp
			continue
		} else if pd != "" && pd != rd {
			if !strings.EqualFold(pd, rd) {
				rd, rr, rp = pd, pr, pp
				continue
			}
			rd = pd
		}
		if rp != "" && !isNTSep(rp[len(rp)-1]) {
			rp += `\`
		}
		rp += pp
	}
	if rp != "" && rr == "" && rd != "" && !strings.ContainsRune(`:\/`, rune(rd[len(rd)-1])) {
		return rd + `\` + rp
	}
	return rd + rr + rp
}

func ntSplit(p string) (head, tail string) {
	d, r, p := ntSplitroot(p)
	i := len(p)
	for i > 0 && !isNTSep(p[i-1]) {
		i--
	}
	return d + r + strings.TrimRight(p[:i], ntSeps), p[i:]
}

func ntNormpath(p string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	d, root, rest := ntSplitroot(p)
	prefix := d + root
	comps := strings.Split(rest, `\`)
	for i := 0; i < len(comps); {
		switch {
		case comps[i] == "" || comps[i] == ".":
			comps = append(comps[:i], comps[i+1:]...)
		case comps[i] == "..":
			if i > 0 && comps[i-1] != ".." {
				comps = append(comps[:i-1], comps[i+1:]...)
				i--
			} else if i == 0 && root != "" {
				comps = comps[1:]
			} else {
				i++
			}
		default:
			i++
		}
	}
	if prefix == "" && len(comps) == 0 {
		return "."
	}
	return prefix + strings.Join(comps, `\`)
}

func ntAbs(p, cwd string) string {
	if !ntIsabs(p) {
		p = ntJoin([]string{cwd, p})
	}
	return ntNormpath(p)
}

func ntRelpath(path, start, cwd string) (string, error) {
	sa, pa := ntAbs(start, cwd), ntAbs(path, cwd)
	sd, _, sr := ntSplitroot(sa)
	pd, _, pr := ntSplitroot(pa)
	if ntNormcase(sd) != ntNormcase(pd) {
		return "", errMount(pd, sd)
	}
	return relList(sr, pr, `\`, func(a, b string) bool { return ntNormcase(a) == ntNormcase(b) }), nil
}

func ntCommonpath(paths []string) (string, error) {
	type root struct{ d, r string }
	rs := make([]root, len(paths))
	lists := make([][]string, len(paths))
	for i, p := range paths {
		d, r, rest := ntSplitroot(strings.ToLower(strings.ReplaceAll(p, "/", `\`)))
		rs[i] = root{d, r}
		lists[i] = nonDot(rest, `\`)
	}
	for _, r := range rs {
		if r.d != rs[0].d {
			return "", errDrive
		}
	}
	d, root0, rest := ntSplitroot(strings.ReplaceAll(paths[0], "/", `\`))
	for _, r := range rs {
		if r.r != rs[0].r {
			if d != "" {
				return "", errMixAbs
			}
			return "", errMixRoot
		}
	}
	return d + root0 + strings.Join(commonComps(nonDot(rest, `\`), lists), `\`), nil
}

func ntExpanduser(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	i := 1
	for i < len(p) && !isNTSep(p[i]) {
		i++
	}
	home, ok := os.LookupEnv("USERPROFILE")
	if !ok {
		hp, ok := os.LookupEnv("HOMEPATH")
		if !ok {
			return p
		}
		hd, _ := os.LookupEnv("HOMEDRIVE")
		home = ntJoin([]string{hd, hp})
	}
	if i != 1 {
		cur, ok := os.LookupEnv("USERNAME")
		if target := p[1:i]; !ok || target != cur {
			h, t := ntSplit(home)
			if !ok || cur != t {
				return p
			}
			home = ntJoin([]string{h, target})
		}
	}
	return home + p[i:]
}

func ntExpandvars(p string) string {
	if !strings.ContainsAny(p, "$%") {
		return p
	}
	var b strings.Builder
	lookup := func(name, whole string) {
		if v, ok := os.LookupEnv(name); ok {
			b.WriteString(v)
		} else {
			b.WriteString(whole)
		}
	}
	for i := 0; i < len(p); {
		c := p[i]
		end := 0 // end of the match starting at i; 0 = no match
		switch c {
		case '\'': // quoted text is left alone
			end = len(p)
			if k := strings.IndexByte(p[i+1:], '\''); k >= 0 {
				end = i + 1 + k + 1
			}
			b.WriteString(p[i:end])
		case '%':
			if i+1 < len(p) && p[i+1] == '%' {
				b.WriteByte('%')
				end = i + 2
			} else if k := strings.IndexByte(p[i+1:], '%'); k >= 0 {
				end = i + 1 + k + 1
				lookup(p[i+1:end-1], p[i:end])
			} else {
				end = len(p)
				b.WriteString(p[i:end])
			}
		case '$':
			j := i + 1
			switch {
			case j < len(p) && p[j] == '$':
				b.WriteByte('$')
				end = j + 1
			case j < len(p) && p[j] == '{':
				if k := strings.IndexByte(p[j:], '}'); k < 0 {
					end = len(p)
					b.WriteString(p[i:end])
				} else {
					end = j + k + 1
					lookup(p[j+1:end-1], p[i:end])
				}
			default:
				e := j
				for e < len(p) && (isASCIIWord(p[e]) || p[e] == '-') {
					e++
				}
				if e > j {
					end = e
					lookup(p[j:e], p[i:e])
				}
			}
		}
		if end == 0 {
			b.WriteByte(c)
			i++
		} else {
			i = end
		}
	}
	return b.String()
}

var ntReservedNames = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true}

func init() {
	for _, c := range "123456789\u00b9\u00b2\u00b3" {
		ntReservedNames["COM"+string(c)] = true
		ntReservedNames["LPT"+string(c)] = true
	}
}

func ntIsreserved(p string) bool {
	_, _, rest := ntSplitroot(p)
	for _, name := range strings.Split(strings.ReplaceAll(rest, "/", `\`), `\`) {
		if name == "" || name == "." || name == ".." {
			continue
		}
		if last := name[len(name)-1]; last == '.' || last == ' ' {
			return true
		}
		if strings.IndexFunc(name, func(r rune) bool { return r < 32 || strings.ContainsRune(`"*:<>?|/\`, r) }) >= 0 {
			return true
		}
		base, _, _ := strings.Cut(name, ".")
		if ntReservedNames[strings.ToUpper(strings.TrimRight(base, " "))] {
			return true
		}
	}
	return false
}
