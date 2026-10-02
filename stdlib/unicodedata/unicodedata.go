// Package unicodedata implements the unicodedata module: access to the
// Unicode character database.
//
// The database is generated from this machine's CPython (see tools/gen_ucd.py),
// so the answers agree with the Unicode version the reference interpreter
// ships.  The Go standard library's unicode package is deliberately not used
// for the tables: it tracks its own Unicode version, and a category or name
// that disagreed with CPython would be worse than none.  Only strconv and the
// utf8 decoder are used from outside.
//
// The tables are embedded as one gzip stream, decoded on first use.  Canonical
// normalisation (NFC, NFD, NFKC, NFKD) is implemented here in full: the
// decomposition table, the canonical composition pairs and the Hangul
// algorithm are all derived from CPython, so no dependency on x/text is needed
// (and none is available in this module graph).
package unicodedata

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Access to the Unicode Character Database (UCD)`

// ---------------------------------------------------------------------------
// Table decoding
// ---------------------------------------------------------------------------

// tables holds the decoded database, built once on first use.
type tables struct {
	cat    []catRange
	bidi   []strRange
	ccc    []intRange
	ea     []strRange
	mir    []catRange
	num    map[rune]numeric
	decomp map[rune]string
	comp   map[[2]rune]rune
	names  map[rune]string
}

var (
	tablesOnce sync.Once
	tablesVal  *tables
	tablesErr  error
)

func ucd() (*tables, error) {
	tablesOnce.Do(func() {
		tablesVal, tablesErr = buildTables()
	})
	return tablesVal, tablesErr
}

type catRange struct {
	lo, hi rune
	val    string
}

type strRange struct {
	lo, hi rune
	val    string
}

type intRange struct {
	lo, hi rune
	val    int
}

type numeric struct {
	decimal, digit, num string
}

// parseRune reads a hexadecimal code point.
func parseRune(s string) rune {
	v, err := strconv.ParseInt(s, 16, 32)
	if err != nil {
		return 0
	}
	return rune(v)
}

// rangeStart finds the index of the last range starting at or before r, or -1.
func rangeStart(n int, lo func(int) rune, r rune) int {
	return sort.Search(n, func(i int) bool { return lo(i) > r }) - 1
}

func buildTables() (*tables, error) {
	blob, err := decodeBlob()
	if err != nil {
		return nil, err
	}
	t := &tables{
		num:    map[rune]numeric{},
		decomp: map[rune]string{},
		comp:   map[[2]rune]rune{},
		names:  map[rune]string{},
	}

	for _, sec := range splitSections(blob) {
		recs := strings.Split(sec.body, ";")
		for _, rec := range recs {
			if rec == "" {
				continue
			}
			switch sec.name {
			case "cat":
				if f := strings.SplitN(rec, ":", 3); len(f) == 3 {
					t.cat = append(t.cat, catRange{parseRune(f[0]), parseRune(f[1]), f[2]})
				}
			case "bidi":
				if f := strings.SplitN(rec, ":", 3); len(f) == 3 {
					t.bidi = append(t.bidi, strRange{parseRune(f[0]), parseRune(f[1]), f[2]})
				}
			case "ccc":
				if f := strings.SplitN(rec, ":", 3); len(f) == 3 {
					n, _ := strconv.Atoi(f[2])
					t.ccc = append(t.ccc, intRange{parseRune(f[0]), parseRune(f[1]), n})
				}
			case "ea":
				if f := strings.SplitN(rec, ":", 3); len(f) == 3 {
					t.ea = append(t.ea, strRange{parseRune(f[0]), parseRune(f[1]), f[2]})
				}
			case "mir":
				if f := strings.SplitN(rec, ":", 2); len(f) == 2 {
					t.mir = append(t.mir, catRange{parseRune(f[0]), parseRune(f[1]), ""})
				}
			case "num":
				if f := strings.SplitN(rec, ":", 4); len(f) == 4 {
					t.num[parseRune(f[0])] = numeric{f[1], f[2], f[3]}
				}
			case "decomp":
				if i := strings.IndexByte(rec, ':'); i >= 0 {
					t.decomp[parseRune(rec[:i])] = rec[i+1:]
				}
			case "comp":
				if colon := strings.IndexByte(rec, ':'); colon >= 0 {
					if pair := strings.SplitN(rec[:colon], ",", 2); len(pair) == 2 {
						t.comp[[2]rune{parseRune(pair[0]), parseRune(pair[1])}] = parseRune(rec[colon+1:])
					}
				}
			case "names":
				if i := strings.IndexByte(rec, ':'); i >= 0 {
					t.names[parseRune(rec[:i])] = rec[i+1:]
				}
			}
		}
	}
	return t, nil
}

type section struct {
	name string
	body string
}

// splitSections walks the tagged sections of the decoded blob.
func splitSections(blob string) []section {
	var out []section
	var name string
	var body strings.Builder
	flush := func() {
		if name != "" {
			out = append(out, section{name: name, body: body.String()})
		}
		body.Reset()
	}
	for _, line := range strings.Split(blob, "\n") {
		if strings.HasPrefix(line, "\x01") {
			flush()
			name = line[1:]
			continue
		}
		body.WriteString(line)
	}
	flush()
	return out
}

// ---------------------------------------------------------------------------
// Lookups
// ---------------------------------------------------------------------------

func (t *tables) category(r rune) string {
	i := rangeStart(len(t.cat), func(i int) rune { return t.cat[i].lo }, r)
	if i < 0 || r > t.cat[i].hi {
		return "Cn"
	}
	return t.cat[i].val
}

func (t *tables) bidirectional(r rune) string {
	i := rangeStart(len(t.bidi), func(i int) rune { return t.bidi[i].lo }, r)
	if i < 0 || r > t.bidi[i].hi {
		return ""
	}
	return t.bidi[i].val
}

func (t *tables) combining(r rune) int {
	i := rangeStart(len(t.ccc), func(i int) rune { return t.ccc[i].lo }, r)
	if i < 0 || r > t.ccc[i].hi {
		return 0
	}
	return t.ccc[i].val
}

func (t *tables) eastAsianWidth(r rune) string {
	i := rangeStart(len(t.ea), func(i int) rune { return t.ea[i].lo }, r)
	if i < 0 || r > t.ea[i].hi {
		return "N"
	}
	return t.ea[i].val
}

func (t *tables) mirrored(r rune) bool {
	i := rangeStart(len(t.mir), func(i int) rune { return t.mir[i].lo }, r)
	return i >= 0 && r <= t.mir[i].hi
}

// name resolves the name, including the algorithmic ranges, whose names are
// built from the code point rather than stored.
func (t *tables) name(r rune) (string, bool) {
	if n, ok := t.names[r]; ok {
		return n, true
	}
	for _, a := range algorithmicNames {
		if r < a.lo || r > a.hi {
			continue
		}
		switch {
		case a.hexDigits > 0:
			return a.prefix + padHex(r, a.hexDigits), true
		case a.prefix == hangulPrefix:
			return a.prefix + hangulName(r), true
		}
		return a.prefix, true
	}
	return "", false
}

const hangulPrefix = "HANGUL SYLLABLE "

// The Hangul syllable composition constants from the Unicode standard.
const (
	hangulSBase  = 0xAC00
	hangulLCount = 19
	hangulVCount = 21
	hangulTCount = 28
	hangulNCount = hangulVCount * hangulTCount
	hangulSCount = hangulLCount * hangulNCount
)

var (
	jamoL = []string{"G", "GG", "N", "D", "DD", "R", "M", "B", "BB", "S", "SS", "", "J", "JJ", "C", "K", "T", "P", "H"}
	jamoV = []string{"A", "AE", "YA", "YAE", "EO", "E", "YEO", "YE", "O", "WA", "WAE", "OE", "YO", "U", "WEO", "WE", "WI", "YU", "EU", "YI", "I"}
	jamoT = []string{"", "G", "GG", "GS", "N", "NJ", "NH", "D", "L", "LG", "LM", "LB", "LS", "LT", "LP", "LH", "M", "B", "BS", "S", "SS", "NG", "J", "C", "K", "T", "P", "H"}
)

// hangulName is the algorithmic name of a Hangul syllable.
func hangulName(r rune) string {
	s := int(r) - hangulSBase
	if s < 0 || s >= hangulSCount {
		return ""
	}
	return jamoL[s/hangulNCount] + jamoV[(s%hangulNCount)/hangulTCount] + jamoT[s%hangulTCount]
}

// padHex writes a code point in at least n upper case hexadecimal digits, which
// is the form the algorithmic names use ("CJK UNIFIED IDEOGRAPH-4E00").
func padHex(r rune, n int) string {
	s := strings.ToUpper(strconv.FormatInt(int64(r), 16))
	for len(s) < n {
		s = "0" + s
	}
	return s
}

// lookupName finds the code point for a name, case insensitively, including the
// algorithmic ranges and the name aliases.
func (t *tables) lookupName(name string) (rune, bool) {
	upper := strings.ToUpper(name)
	if r, ok := nameAliases[upper]; ok {
		return r, true
	}
	// The stored table is keyed by code point, so a reverse index is built on
	// demand; lookups are rare next to the other calls.
	reverseNames.Do(func() {
		reverseNameMap = make(map[string]rune, len(t.names))
		for r, n := range t.names {
			reverseNameMap[n] = r
		}
	})
	if r, ok := reverseNameMap[upper]; ok {
		return r, true
	}
	for _, a := range algorithmicNames {
		if a.hexDigits == 0 || !strings.HasPrefix(upper, a.prefix) {
			continue
		}
		hex := upper[len(a.prefix):]
		v, err := strconv.ParseInt(hex, 16, 32)
		if err != nil {
			continue
		}
		r := rune(v)
		if r >= a.lo && r <= a.hi && padHex(r, a.hexDigits) == hex {
			return r, true
		}
	}
	if strings.HasPrefix(upper, hangulPrefix) {
		return lookupHangul(upper[len(hangulPrefix):])
	}
	return 0, false
}

var (
	reverseNames   sync.Once
	reverseNameMap map[string]rune
)

// lookupHangul composes a Hangul syllable name from its jamo letters, choosing
// the longest matching jamo at each position because several are prefixes of
// others ("G" and "GG").
func lookupHangul(s string) (rune, bool) {
	l, rest, ok := matchJamo(jamoL, s, false)
	if !ok {
		return 0, false
	}
	v, rest, ok := matchJamo(jamoV, rest, false)
	if !ok {
		return 0, false
	}
	tt, rest, ok := matchJamo(jamoT, rest, true)
	if !ok || rest != "" {
		return 0, false
	}
	return rune(hangulSBase + (l*hangulVCount+v)*hangulTCount + tt), true
}

// matchJamo takes the longest jamo that prefixes s.  allowEmpty permits the
// empty trail consonant.
func matchJamo(list []string, s string, allowEmpty bool) (int, string, bool) {
	best := -1
	for i, v := range list {
		if v == "" {
			if allowEmpty {
				best = i
			}
			continue
		}
		if !strings.HasPrefix(s, v) {
			continue
		}
		if best < 0 || len(v) > len(list[best]) {
			best = i
		}
	}
	if best < 0 {
		return 0, s, false
	}
	return best, s[len(list[best]):], true
}

// ---------------------------------------------------------------------------
// Normalisation
// ---------------------------------------------------------------------------

// decompose expands r into out, recursively, and reports whether it decomposed.
// Compatibility mappings are only followed when compat is set, which is what
// separates NFKC/NFKD from NFC/NFD.
func (t *tables) decompose(r rune, compat bool, out *[]rune) bool {
	d, ok := t.decomp[r]
	if !ok {
		return false
	}
	if strings.HasPrefix(d, "<") {
		if !compat {
			return false
		}
		i := strings.IndexByte(d, '>')
		if i < 0 {
			return false
		}
		d = strings.TrimSpace(d[i+1:])
	}
	for _, part := range strings.Fields(d) {
		sub := parseRune(part)
		// A decomposition may itself decompose; both levels are expanded.
		if !t.decompose(sub, compat, out) {
			*out = append(*out, sub)
		}
	}
	return true
}

// canonicalOrder is the Unicode canonical ordering algorithm: a stable sort of
// each run of non-starters by combining class.  The runs are short and the
// algorithm is defined as adjacent transpositions, so it is written that way.
func (t *tables) canonicalOrder(rs []rune) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0; j-- {
			cj := t.combining(rs[j])
			if cj == 0 {
				break
			}
			prev := t.combining(rs[j-1])
			// Never swap across a starter, and only when the earlier class
			// is strictly greater.
			if prev == 0 || prev <= cj {
				break
			}
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

// compose is canonical composition.  A character C may combine with the last
// starter when no character between them blocks it: it is blocked if some
// intervening character has combining class 0, or one greater than or equal to
// its own.
func (t *tables) compose(rs []rune) []rune {
	if len(rs) == 0 {
		return rs
	}
	out := make([]rune, 0, len(rs))
	out = append(out, rs[0])
	starterPos := 0
	lastClass := t.combining(rs[0])
	for i := 1; i < len(rs); i++ {
		ch := rs[i]
		class := t.combining(ch)
		if lastClass == 0 || lastClass < class {
			if c, ok := t.composePair(out[starterPos], ch); ok {
				out[starterPos] = c
				continue
			}
		}
		if class == 0 {
			starterPos = len(out)
		}
		out = append(out, ch)
		lastClass = class
	}
	return out
}

// composePair composes two code points, via the Hangul algorithm or the stored
// canonical composition pairs.
func (t *tables) composePair(a, b rune) (rune, bool) {
	if c, ok := hangulCompose(a, b); ok {
		return c, true
	}
	c, ok := t.comp[[2]rune{a, b}]
	return c, ok
}

// hangulCompose is Hangul syllable composition (Unicode standard, 3.12):
// L + V -> LV, and LV + T -> LVT.
func hangulCompose(a, b rune) (rune, bool) {
	if s := int(a) - hangulSBase; s >= 0 && s < hangulSCount {
		if s%hangulTCount == 0 && b >= 0x11A8 && b <= 0x11C2 {
			return rune(hangulSBase + s + (int(b) - 0x11A7)), true
		}
		return 0, false
	}
	if a >= 0x1100 && a <= 0x1112 && b >= 0x1161 && b <= 0x1175 {
		return rune(hangulSBase + (int(a)-0x1100)*hangulNCount + (int(b)-0x1161)*hangulTCount), true
	}
	return 0, false
}

// normalize implements unicodedata.normalize.
func (t *tables) normalize(form, s string) (string, error) {
	compat := form == "NFKC" || form == "NFKD"
	switch form {
	case "NFC", "NFD", "NFKC", "NFKD":
	default:
		return "", py.ExceptionNewf(py.ValueError, "invalid normalization form")
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if !t.decompose(r, compat, &out) {
			out = append(out, r)
		}
	}
	t.canonicalOrder(out)
	if form == "NFC" || form == "NFKC" {
		out = t.compose(out)
	}
	return string(out), nil
}

// ---------------------------------------------------------------------------
// Name aliases
//
// The control characters and a few others have no name of their own; CPython
// keeps a small alias table for them, which lookup() consults first.
// ---------------------------------------------------------------------------

var nameAliases = map[string]rune{
	"NULL":                         0x0000,
	"START OF HEADING":             0x0001,
	"START OF TEXT":                0x0002,
	"END OF TEXT":                  0x0003,
	"END OF TRANSMISSION":          0x0004,
	"ENQUIRY":                      0x0005,
	"ACKNOWLEDGE":                  0x0006,
	"ALERT":                        0x0007,
	"BELL":                         0x0007,
	"BACKSPACE":                    0x0008,
	"CHARACTER TABULATION":         0x0009,
	"HORIZONTAL TABULATION":        0x0009,
	"LINE FEED":                    0x000A,
	"LINE FEED (LF)":               0x000A,
	"NEW LINE":                     0x000A,
	"END OF LINE":                  0x000A,
	"LINE TABULATION":              0x000B,
	"VERTICAL TABULATION":          0x000B,
	"FORM FEED":                    0x000C,
	"FORM FEED (FF)":               0x000C,
	"CARRIAGE RETURN":              0x000D,
	"CARRIAGE RETURN (CR)":         0x000D,
	"SHIFT OUT":                    0x000E,
	"SHIFT IN":                     0x000F,
	"DATA LINK ESCAPE":             0x0010,
	"DEVICE CONTROL ONE":           0x0011,
	"DEVICE CONTROL TWO":           0x0012,
	"DEVICE CONTROL THREE":         0x0013,
	"DEVICE CONTROL FOUR":          0x0014,
	"NEGATIVE ACKNOWLEDGE":         0x0015,
	"SYNCHRONOUS IDLE":             0x0016,
	"END OF TRANSMISSION BLOCK":    0x0017,
	"CANCEL":                       0x0018,
	"END OF MEDIUM":                0x0019,
	"SUBSTITUTE":                   0x001A,
	"ESCAPE":                       0x001B,
	"INFORMATION SEPARATOR FOUR":   0x001C,
	"FILE SEPARATOR":               0x001C,
	"INFORMATION SEPARATOR THREE":  0x001D,
	"GROUP SEPARATOR":              0x001D,
	"INFORMATION SEPARATOR TWO":    0x001E,
	"RECORD SEPARATOR":             0x001E,
	"INFORMATION SEPARATOR ONE":    0x001F,
	"UNIT SEPARATOR":               0x001F,
	"DELETE":                       0x007F,
	"ZWNBSP":                       0xFEFF,
	"BYTE ORDER MARK":              0xFEFF,
	"ZERO WIDTH NO-BREAK SPACE":    0xFEFF,
	"OBJECT REPLACEMENT CHARACTER": 0xFFFC,
	"REPLACEMENT CHARACTER":        0xFFFD,
}

// ---------------------------------------------------------------------------
// Python facing helpers
// ---------------------------------------------------------------------------

// charArg extracts the single character an argument must be, with CPython's
// error messages.
func charArg(o py.Object, name string) (rune, error) {
	s, ok := o.(py.String)
	if !ok {
		return 0, py.ExceptionNewf(py.TypeError,
			"%s() argument must be a unicode character, not %s", name, o.Type().Name)
	}
	rs := []rune(string(s))
	if len(rs) != 1 {
		return 0, py.ExceptionNewf(py.TypeError,
			"%s(): argument must be a unicode character, not a string of length %d", name, len(rs))
	}
	return rs[0], nil
}

func strArg(o py.Object, name string, pos int) (string, error) {
	s, ok := o.(py.String)
	if !ok {
		return "", py.ExceptionNewf(py.TypeError, "%s() argument %d must be str, not %s", name, pos, o.Type().Name)
	}
	return string(s), nil
}

func init() {
	methods := []*py.Method{
		py.MustNewMethod("category", categoryMethod, 0, "Returns the general category assigned to the character chr as string."),
		py.MustNewMethod("bidirectional", bidirectionalMethod, 0, "Returns the bidirectional class assigned to the character chr as string."),
		py.MustNewMethod("combining", combiningMethod, 0, "Returns the canonical combining class assigned to the character chr as integer."),
		py.MustNewMethod("decimal", decimalMethod, 0, "Returns the decimal value assigned to the character chr as integer."),
		py.MustNewMethod("digit", digitMethod, 0, "Returns the digit value assigned to the character chr as integer."),
		py.MustNewMethod("numeric", numericMethod, 0, "Returns the numeric value assigned to the character chr as float."),
		py.MustNewMethod("mirrored", mirroredMethod, 0, "Returns the mirrored property assigned to the character chr as integer."),
		py.MustNewMethod("decomposition", decompositionMethod, 0, "Returns the character decomposition mapping assigned to the character chr as string."),
		py.MustNewMethod("east_asian_width", eastAsianWidthMethod, 0, "Returns the east asian width assigned to the character chr as string."),
		py.MustNewMethod("name", nameMethod, 0, "Returns the name assigned to the character chr as a string."),
		py.MustNewMethod("lookup", lookupMethod, 0, "Look up character by name."),
		py.MustNewMethod("normalize", normalizeMethod, 0, "Return the normal form 'form' for the Unicode string unistr."),
		py.MustNewMethod("is_normalized", isNormalizedMethod, 0, "Return whether the Unicode string unistr is in the normal form 'form'."),
	}

	// The globals are built with Set rather than a composite literal so that
	// each entry can carry its own comment.
	globals := py.NewStringDict()
	globals.Set("unidata_version", py.String(unidataVersion))
	// UCD is the version packed as the C API reports it: 16.0.0.
	globals.Set("UCD", py.Int(16<<16))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "unicodedata",
			Doc:  module_doc,
		},
		Methods: methods,
		Globals: globals,
	})
}

func categoryMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var o py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "category", 1, 1, &o); err != nil {
		return nil, err
	}
	r, err := charArg(o, "category")
	if err != nil {
		return nil, err
	}
	return py.String(t.category(r)), nil
}

func bidirectionalMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var o py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "bidirectional", 1, 1, &o); err != nil {
		return nil, err
	}
	r, err := charArg(o, "bidirectional")
	if err != nil {
		return nil, err
	}
	return py.String(t.bidirectional(r)), nil
}

func combiningMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var o py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "combining", 1, 1, &o); err != nil {
		return nil, err
	}
	r, err := charArg(o, "combining")
	if err != nil {
		return nil, err
	}
	return py.Int(t.combining(r)), nil
}

func mirroredMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var o py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "mirrored", 1, 1, &o); err != nil {
		return nil, err
	}
	r, err := charArg(o, "mirrored")
	if err != nil {
		return nil, err
	}
	if t.mirrored(r) {
		return py.Int(1), nil
	}
	return py.Int(0), nil
}

func decompositionMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var o py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "decomposition", 1, 1, &o); err != nil {
		return nil, err
	}
	r, err := charArg(o, "decomposition")
	if err != nil {
		return nil, err
	}
	return py.String(t.decomp[r]), nil
}

func eastAsianWidthMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var o py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "east_asian_width", 1, 1, &o); err != nil {
		return nil, err
	}
	r, err := charArg(o, "east_asian_width")
	if err != nil {
		return nil, err
	}
	return py.String(t.eastAsianWidth(r)), nil
}

// numericMethod covers decimal(), digit() and numeric(), which share the same
// shape: a value and an optional default that is returned when there is none.
func numericMethod(self py.Object, args py.Tuple) (py.Object, error) {
	return numericish(args, "numeric", func(n numeric) string { return n.num })
}

func decimalMethod(self py.Object, args py.Tuple) (py.Object, error) {
	return numericish(args, "decimal", func(n numeric) string { return n.decimal })
}

func digitMethod(self py.Object, args py.Tuple) (py.Object, error) {
	return numericish(args, "digit", func(n numeric) string { return n.digit })
}

func numericish(args py.Tuple, name string, pick func(numeric) string) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var o py.Object
	var def py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, name, 1, 2, &o, &def); err != nil {
		return nil, err
	}
	r, err := charArg(o, name)
	if err != nil {
		return nil, err
	}
	if v, ok := t.num[r]; ok {
		if s := pick(v); s != "" {
			if name == "numeric" {
				// numeric() reports a float, so "1/2" becomes 0.5.
				f, err := parseNumeric(s)
				if err != nil {
					return nil, err
				}
				return py.Float(f), nil
			}
			n, err := strconv.Atoi(s)
			if err != nil {
				return nil, py.ExceptionNewf(py.ValueError, "not a %s", name)
			}
			return py.Int(n), nil
		}
	}
	if def != nil {
		return def, nil
	}
	return nil, py.ExceptionNewf(py.ValueError, "not a %s", name)
}

// parseNumeric understands the value forms the database uses: an integer, and
// the "1/2" style fractions.
func parseNumeric(s string) (float64, error) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		num, err1 := strconv.ParseFloat(s[:i], 64)
		den, err2 := strconv.ParseFloat(s[i+1:], 64)
		if err1 != nil || err2 != nil {
			return 0, py.ExceptionNewf(py.ValueError, "not a numeric value")
		}
		return num / den, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, py.ExceptionNewf(py.ValueError, "not a numeric value")
	}
	return f, nil
}

func nameMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var o py.Object
	var def py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "name", 1, 2, &o, &def); err != nil {
		return nil, err
	}
	r, err := charArg(o, "name")
	if err != nil {
		return nil, err
	}
	if n, ok := t.name(r); ok {
		return py.String(n), nil
	}
	if def != nil {
		return def, nil
	}
	return nil, py.ExceptionNewf(py.ValueError, "no such name")
}

func lookupMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var o py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "lookup", 1, 1, &o); err != nil {
		return nil, err
	}
	name, err := strArg(o, "lookup", 1)
	if err != nil {
		return nil, err
	}
	r, ok := t.lookupName(name)
	if !ok {
		return nil, py.ExceptionNewf(py.KeyError, "undefined character name '%s'", name)
	}
	return py.String(string(r)), nil
}

func normalizeMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var formObj, dataObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "normalize", 2, 2, &formObj, &dataObj); err != nil {
		return nil, err
	}
	form, err := strArg(formObj, "normalize", 1)
	if err != nil {
		return nil, err
	}
	data, err := strArg(dataObj, "normalize", 2)
	if err != nil {
		return nil, err
	}
	out, err := t.normalize(form, data)
	if err != nil {
		return nil, err
	}
	return py.String(out), nil
}

func isNormalizedMethod(self py.Object, args py.Tuple) (py.Object, error) {
	t, err := ucd()
	if err != nil {
		return nil, err
	}
	var formObj, dataObj py.Object
	if err := py.UnpackTuple(args, py.StringDict{}, "is_normalized", 2, 2, &formObj, &dataObj); err != nil {
		return nil, err
	}
	form, err := strArg(formObj, "is_normalized", 1)
	if err != nil {
		return nil, err
	}
	data, err := strArg(dataObj, "is_normalized", 2)
	if err != nil {
		return nil, err
	}
	out, err := t.normalize(form, data)
	if err != nil {
		return nil, err
	}
	return py.Bool(out == data), nil
}
