// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package email

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vishnukv64/gpython/py"
)

const header_doc = `email.header - decode and encode RFC 2047 message headers.

Header is an object holding one header value as a sequence of (string, charset)
chunks; decode_header() splits an RFC 2047 header into (bytes, charset) pairs
and make_header() turns such a sequence back into a Header.`

// HeaderType is the class of a MIME header value.
var HeaderType *py.Type

// Charset header-encoding flags, mirroring email.charset.
const (
	headerEncNone     = 0
	headerEncQP       = 1
	headerEncBase64   = 2
	headerEncShortest = 3
)

const (
	defaultCharset = "us-ascii"
	unknown8bit    = "unknown-8bit"
	maxLineLen     = 78
	rfc2047Chrome  = 7
)

// charsetTable is CPython's email.charset.CHARSETS: input charset -> (header
// encoding, output charset).  The body encoding and a None output charset are
// dropped: neither is consulted by the header code.
var charsetTable = map[string]struct {
	henc int
	conv string
}{
	"iso-8859-1":   {headerEncQP, ""},
	"iso-8859-2":   {headerEncQP, ""},
	"iso-8859-3":   {headerEncQP, ""},
	"iso-8859-4":   {headerEncQP, ""},
	"iso-8859-9":   {headerEncQP, ""},
	"iso-8859-10":  {headerEncQP, ""},
	"iso-8859-13":  {headerEncQP, ""},
	"iso-8859-14":  {headerEncQP, ""},
	"iso-8859-15":  {headerEncQP, ""},
	"iso-8859-16":  {headerEncQP, ""},
	"windows-1252": {headerEncQP, ""},
	"viscii":       {headerEncQP, ""},
	"us-ascii":     {headerEncNone, ""},
	"big5":         {headerEncBase64, ""},
	"gb2312":       {headerEncBase64, ""},
	"euc-jp":       {headerEncBase64, "iso-2022-jp"},
	"shift_jis":    {headerEncBase64, "iso-2022-jp"},
	"iso-2022-jp":  {headerEncBase64, ""},
	"koi8-r":       {headerEncBase64, ""},
	"utf-8":        {headerEncShortest, "utf-8"},
}

// charsetAliases is CPython's ALIASES map.
var charsetAliases = map[string]string{
	"latin_1":  "iso-8859-1",
	"latin-1":  "iso-8859-1",
	"latin_2":  "iso-8859-2",
	"latin-2":  "iso-8859-2",
	"latin_3":  "iso-8859-3",
	"latin-3":  "iso-8859-3",
	"latin_4":  "iso-8859-4",
	"latin-4":  "iso-8859-4",
	"latin_5":  "iso-8859-9",
	"latin-5":  "iso-8859-9",
	"latin_6":  "iso-8859-10",
	"latin-6":  "iso-8859-10",
	"latin_7":  "iso-8859-13",
	"latin-7":  "iso-8859-13",
	"latin_8":  "iso-8859-14",
	"latin-8":  "iso-8859-14",
	"latin_9":  "iso-8859-15",
	"latin-9":  "iso-8859-15",
	"latin_10": "iso-8859-16",
	"latin-10": "iso-8859-16",
	"cp949":    "ks_c_5601-1987",
	"euc_jp":   "euc-jp",
	"euc_kr":   "euc-kr",
	"ascii":    "us-ascii",
}

// mimeCharset mirrors email.charset.Charset for the header code.  An empty
// codec string stands for CPython's None.
type mimeCharset struct {
	name          string // str(charset): the lower-cased, alias-resolved name
	headerEnc     int
	outputCharset string
	inputCodec    string
	outputCodec   string
}

// newCharset is Charset(input_charset).
func newCharset(input string) (*mimeCharset, error) {
	for _, r := range input {
		if r > 127 {
			return nil, py.ExceptionNewf(CharsetError, "%s", input)
		}
	}
	lower := strings.ToLower(input)
	name := lower
	if a, ok := charsetAliases[lower]; ok {
		name = a
	}
	henc := headerEncShortest
	conv := ""
	if row, ok := charsetTable[name]; ok {
		henc, conv = row.henc, row.conv
	}
	if conv == "" {
		conv = name
	}
	outputCharset := conv
	if a, ok := charsetAliases[conv]; ok {
		outputCharset = a
	}
	cs := &mimeCharset{
		name:          name,
		headerEnc:     henc,
		outputCharset: outputCharset,
		inputCodec:    name,
		outputCodec:   outputCharset,
	}
	// CODEC_MAP maps only us-ascii to None: garbage sent as 7-bit us-ascii is
	// passed through without a conversion.
	if name == "us-ascii" {
		cs.inputCodec = ""
	}
	if outputCharset == "us-ascii" {
		cs.outputCodec = ""
	}
	return cs, nil
}

// outputName is get_output_charset(): output_charset or input_charset.
func (c *mimeCharset) outputName() string {
	if c.outputCharset != "" {
		return c.outputCharset
	}
	return c.name
}

// --- codec helpers -------------------------------------------------------
//
// These deliberately do not call str.encode/bytes.decode: the interpreter's
// surrogateescape support does not round-trip a lone surrogate through str,
// and the header code depends on it for the unknown-8bit charset.

// unimplementedCodecs are charset names CPython's codec registry knows but
// this interpreter's does not.  They are named explicitly so the failure is a
// clear NotImplementedError - "this build cannot do it" - rather than the
// LookupError that means "no such codec", which CPython raises for a truly
// bogus name and which is kept for that case.  None of these is reachable from
// pip, whose headers are us-ascii, utf-8, latin-1 or unknown-8bit.
var unimplementedCodecs = map[string]bool{
	"big5": true, "gb2312": true, "euc-jp": true, "euc-kr": true,
	"shift-jis": true, "shift_jis": true, "iso-2022-jp": true, "koi8-r": true,
	"windows-1252": true, "cp1252": true,
	"iso-8859-2": true, "iso-8859-3": true, "iso-8859-4": true,
	"iso-8859-5": true, "iso-8859-6": true, "iso-8859-7": true,
	"iso-8859-8": true, "iso-8859-9": true, "iso-8859-10": true,
	"iso-8859-13": true, "iso-8859-14": true, "iso-8859-15": true,
	"iso-8859-16": true, "ks-c-5601-1987": true,
}

// canonicalCodec maps a codec name the way str.encode/bytes.decode do, for the
// encodings this interpreter implements.
func canonicalCodec(name string) (string, error) {
	n := strings.ToLower(strings.ReplaceAll(name, " ", ""))
	n = strings.ReplaceAll(n, "_", "-")
	switch n {
	case "utf-8", "utf8", "u8", "utf", "cp65001":
		return "utf-8", nil
	case "ascii", "us-ascii", "646":
		return "ascii", nil
	case "latin-1", "latin1", "iso-8859-1", "8859", "l1":
		return "latin-1", nil
	}
	if unimplementedCodecs[n] {
		return "", py.ExceptionNewf(py.NotImplementedError,
			"the %q codec is not implemented in gpython's email.header", name)
	}
	return "", py.ExceptionNewf(py.LookupError, "unknown encoding: %s", name)
}

// surrogateEscapeDecode is bytes.decode('us-ascii', 'surrogateescape'): every
// non-ascii byte is preserved as a single character.  CPython uses the lone
// surrogates U+DC80..U+DCFF for them, but a Go string cannot hold a lone
// surrogate - string(rune(0xdcff)) is silently replaced with U+FFFD - so the
// same information is carried in the private-use range U+E080..U+E0FF, one
// rune per escaped byte.  Everything downstream goes back through
// surrogateEscapeEncode, so the mapping never escapes this file.
func surrogateEscapeDecode(b []byte) string {
	runes := make([]rune, 0, len(b))
	for _, c := range b {
		if c < 0x80 {
			runes = append(runes, rune(c))
		} else {
			runes = append(runes, rune(0xe000+int(c)))
		}
	}
	return string(runes)
}

// surrogateEscapeEncode is str.encode('ascii', 'surrogateescape').
func surrogateEscapeEncode(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case 0xe080 <= r && r <= 0xe0ff:
			out = append(out, byte(r-0xe000))
		case r < 0x80:
			out = append(out, byte(r))
		default:
			out = append(out, '?')
		}
	}
	return out
}

// encodeString is str.encode(codec, errors); unknown-8bit is handled directly
// because the interpreter has no such codec.
func encodeString(codec, errors, s string) ([]byte, error) {
	if codec == unknown8bit {
		return surrogateEscapeEncode(s), nil
	}
	canon, err := canonicalCodec(codec)
	if err != nil {
		return nil, err
	}
	limit := 128
	if canon == "latin-1" {
		limit = 256
	}
	out := make([]byte, 0, len(s))
	if canon == "utf-8" {
		// utf-8 always represents every Python character.
		return []byte(s), nil
	}
	for i, r := range s {
		if int(r) < limit {
			out = append(out, byte(r))
			continue
		}
		switch errors {
		case "ignore":
		case "replace":
			out = append(out, '?')
		case "backslashreplace":
			out = append(out, fmt.Sprintf(`\x%02x`, r)...)
		case "xmlcharrefreplace":
			out = append(out, fmt.Sprintf("&#%d;", r)...)
		case "surrogateescape":
			if 0xdc80 <= r && r <= 0xdcff {
				out = append(out, byte(r-0xdc00))
				continue
			}
			fallthrough
		default: // strict
			if canon == "ascii" {
				return nil, py.ExceptionNewf(py.UnicodeEncodeError,
					"'ascii' codec can't encode character '\\x%x' in position %d: ordinal not in range(128)", r, i)
			}
			return nil, py.ExceptionNewf(py.UnicodeEncodeError,
				"'latin-1' codec can't encode character '\\x%x' in position %d: ordinal not in range(256)", r, i)
		}
	}
	return out, nil
}

// decodeString is bytes.decode(codec, errors).
func decodeString(codec, errors string, b []byte) (string, error) {
	if codec == unknown8bit {
		return surrogateEscapeDecode(b), nil
	}
	canon, err := canonicalCodec(codec)
	if err != nil {
		return "", err
	}
	switch canon {
	case "latin-1":
		runes := make([]rune, len(b))
		for i, c := range b {
			runes[i] = rune(c)
		}
		return string(runes), nil
	case "utf-8":
		if utf8.Valid(b) {
			return string(b), nil
		}
	}
	// ascii, or utf-8 with invalid input.
	runes := make([]rune, 0, len(b))
	for i := 0; i < len(b); {
		var r rune
		var size int
		if canon == "utf-8" {
			r, size = utf8.DecodeRune(b[i:])
			if r == utf8.RuneError && size <= 1 {
				r, size = utf8.RuneError, 1
			} else if r > 127 {
				runes = append(runes, r)
				i += size
				continue
			}
		} else {
			r, size = rune(b[i]), 1
		}
		if r > 127 {
			switch errors {
			case "ignore":
			case "replace":
				runes = append(runes, '\ufffd')
			case "backslashreplace":
				runes = append(runes, []rune(fmt.Sprintf(`\x%02x`, b[i]))...)
			case "surrogateescape":
				runes = append(runes, rune(0xdc00+int(b[i])))
			default: // strict
				return "", py.ExceptionNewf(py.UnicodeDecodeError,
					"'ascii' codec can't decode byte 0x%02x in position %d: ordinal not in range(128)", b[i], i)
			}
		} else {
			runes = append(runes, r)
		}
		i += size
	}
	return string(runes), nil
}

// --- header decode -------------------------------------------------------

// ecre matches an RFC 2047 encoded word, as CPython's email.header.ecre does.
var ecre = regexp.MustCompile(`=\?([^?]*?)\?([qQbB])\?(.*?)\?=`)

var qpUnquoteRe = regexp.MustCompile(`=[a-fA-F0-9]{2}`)

var embeddedHeaderRe = regexp.MustCompile(`\n[^ \t]+:`)

// reSplit is re.split with capturing groups: the text before each match, the
// groups of the match, and the text between matches, in order.
func reSplit(re *regexp.Regexp, s string) []string {
	var parts []string
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		parts = append(parts, s[last:m[0]])
		for g := 2; g+1 < len(m); g += 2 {
			if m[g] < 0 {
				parts = append(parts, "")
			} else {
				parts = append(parts, s[m[g]:m[g+1]])
			}
		}
		last = m[1]
	}
	return append(parts, s[last:])
}

// headerDecode is email.quoprimime.header_decode.
func headerDecode(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	return qpUnquoteRe.ReplaceAllStringFunc(s, func(m string) string {
		var v int
		fmt.Sscanf(m[1:], "%x", &v)
		return string(rune(v))
	})
}

// rawUnicodeEscape (bytes(s, 'raw-unicode-escape')) is shared with
// email.utils, where collapse_rfc2231_value relies on the same codec.

// a2bBase64 is CPython's binascii.a2b_base64 in its (default) lenient mode,
// which discards invalid characters and stops at the first complete pad
// sequence.  b64decode's strict decoder does not reproduce that behaviour.
func a2bBase64(data []byte) []byte {
	var table [256]byte
	for i := range table {
		table[i] = 255
	}
	for i := 0; i < 26; i++ {
		table['A'+i] = byte(i)
		table['a'+i] = byte(26 + i)
	}
	for i := 0; i < 10; i++ {
		table['0'+i] = byte(52 + i)
	}
	table['+'] = 62
	table['/'] = 63

	out := make([]byte, 0, ((len(data)+3)/4)*3)
	quadPos, pads := 0, 0
	var leftchar byte
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if ch == '=' {
			if quadPos >= 2 {
				pads++
				if quadPos+pads >= 4 {
					return out
				}
			}
			continue
		}
		v := table[ch]
		if v >= 64 {
			continue
		}
		pads = 0
		switch quadPos {
		case 0:
			quadPos, leftchar = 1, v
		case 1:
			quadPos = 2
			out = append(out, leftchar<<2|v>>4)
			leftchar = v & 0x0f
		case 2:
			quadPos = 3
			out = append(out, leftchar<<4|v>>2)
			leftchar = v & 0x03
		case 3:
			quadPos = 0
			out = append(out, leftchar<<6|v)
			leftchar = 0
		}
	}
	return out
}

type decodedWord struct {
	word    []byte
	charset string // "" stands for None
}

// decodeHeader is email.header.decode_header.
func decodeHeader(header py.Object) (py.Object, error) {
	if h, ok := header.(*HeaderObj); ok {
		items := make([]py.Object, 0, len(h.chunks))
		for _, c := range h.chunks {
			// CPython re-encodes with str(charset) - the input charset - not
			// the output codec, and only unknown-8bit goes through
			// surrogateescape.
			b, err := encodeString(c.cs.name, "strict", c.s)
			if err != nil {
				return nil, err
			}
			items = append(items, py.Tuple{py.Bytes(b), py.String(c.cs.name)})
		}
		return py.NewListFromItems(items), nil
	}
	s, ok := header.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "expected str or Header, got %s", header.Type().Name)
	}
	text := string(s)
	if !ecre.MatchString(text) {
		return py.NewListFromItems([]py.Object{
			py.Tuple{py.String(text), py.None},
		}), nil
	}
	type word struct {
		s        string
		encoding string // "" = unencoded
		charset  string
	}
	var words []word
	for _, line := range pySplitlines(text) {
		parts := reSplit(ecre, line)
		first := true
		for len(parts) > 0 {
			unencoded := parts[0]
			parts = parts[1:]
			if first {
				unencoded = strings.TrimLeftFunc(unencoded, pyIsSpace)
				first = false
			}
			if unencoded != "" {
				words = append(words, word{s: unencoded})
			}
			if len(parts) >= 3 {
				charset := strings.ToLower(parts[0])
				encoding := strings.ToLower(parts[1])
				encoded := parts[2]
				parts = parts[3:]
				words = append(words, word{s: encoded, encoding: encoding, charset: charset})
			}
		}
	}
	// Remove whitespace-only words that sit between two encoded words.
	var droplist []int
	for n, w := range words {
		if n > 1 && w.encoding != "" && words[n-2].encoding != "" && isSpaceString(words[n-1].s) {
			droplist = append(droplist, n-1)
		}
	}
	for i := len(droplist) - 1; i >= 0; i-- {
		d := droplist[i]
		words = append(words[:d], words[d+1:]...)
	}

	decoded := make([]decodedWord, 0, len(words))
	for _, w := range words {
		switch w.encoding {
		case "":
			decoded = append(decoded, decodedWord{word: rawUnicodeEscape(w.s), charset: w.charset})
		case "q":
			decoded = append(decoded, decodedWord{word: rawUnicodeEscape(headerDecode(w.s)), charset: w.charset})
		case "b":
			enc := w.s
			if paderr := len(enc) % 4; paderr != 0 {
				enc += strings.Repeat("=", 4-paderr)
			}
			decoded = append(decoded, decodedWord{word: a2bBase64(rawUnicodeEscape(enc)), charset: w.charset})
		}
	}

	var collapsed []decodedWord
	var last []byte
	lastCharset := ""
	haveLast := false
	for _, d := range decoded {
		if !haveLast {
			last, lastCharset, haveLast = d.word, d.charset, true
			continue
		}
		if d.charset != lastCharset {
			collapsed = append(collapsed, decodedWord{word: last, charset: lastCharset})
			last, lastCharset = d.word, d.charset
			continue
		}
		if lastCharset == "" {
			last = append(append(last, ' '), d.word...)
		} else {
			last = append(last, d.word...)
		}
	}
	if haveLast {
		collapsed = append(collapsed, decodedWord{word: last, charset: lastCharset})
	}

	items := make([]py.Object, 0, len(collapsed))
	for _, c := range collapsed {
		var cs py.Object = py.None
		if c.charset != "" {
			cs = py.String(c.charset)
		}
		items = append(items, py.Tuple{py.Bytes(c.word), cs})
	}
	return py.NewListFromItems(items), nil
}

func decodeHeaderFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var header py.Object
	if err := py.UnpackTuple(args, kwargs, "decode_header", 1, 1, &header); err != nil {
		return nil, err
	}
	return decodeHeader(header)
}

// --- Header --------------------------------------------------------------

type headerChunk struct {
	s  string
	cs *mimeCharset
}

// HeaderObj is the python email.header.Header.
type HeaderObj struct {
	charset        *mimeCharset
	continuationWS string
	chunks         []headerChunk
	maxlinelen     int
	headerlen      int
	Dict           py.StringDict
}

func (h *HeaderObj) Type() *py.Type         { return HeaderType }
func (h *HeaderObj) GetDict() py.StringDict { return h.Dict }

func headerNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		s              py.Object = py.None
		charsetArg     py.Object = py.None
		maxlinelenArg  py.Object = py.None
		headerNameArg  py.Object = py.None
		continuationWS py.Object = py.String(" ")
		errorsArg      py.Object = py.String("strict")
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOOOOO:Header",
		[]string{"s", "charset", "maxlinelen", "header_name", "continuation_ws", "errors"},
		&s, &charsetArg, &maxlinelenArg, &headerNameArg, &continuationWS, &errorsArg); err != nil {
		return nil, err
	}
	h := &HeaderObj{Dict: py.NewStringDict()}

	cs, err := charsetFromArg(charsetArg)
	if err != nil {
		return nil, err
	}
	h.charset = cs
	ws, ok := continuationWS.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.TypeError, "continuation_ws must be str, not %s", continuationWS.Type().Name)
	}
	h.continuationWS = string(ws)

	if s != py.None {
		if err := h.appendChunk(s, h.charset, pyStr(errorsArg)); err != nil {
			return nil, err
		}
	}
	if maxlinelenArg == py.None {
		h.maxlinelen = maxLineLen
	} else {
		n, err := pyIntValue(maxlinelenArg)
		if err != nil {
			return nil, err
		}
		h.maxlinelen = n
	}
	if headerNameArg == py.None {
		h.headerlen = 0
	} else {
		name, err := py.StrAsString(headerNameArg)
		if err != nil {
			return nil, err
		}
		h.headerlen = len(name) + 2
	}
	return h, nil
}

// charsetFromArg resolves a constructor/append charset argument: None means the
// default us-ascii, a string is looked up, anything else is refused because
// this interpreter exposes no Charset class to pass in.
func charsetFromArg(arg py.Object) (*mimeCharset, error) {
	switch v := arg.(type) {
	case py.NoneType:
		return newCharset(defaultCharset)
	case py.String:
		return newCharset(string(v))
	default:
		return nil, py.ExceptionNewf(py.TypeError, "charset must be str or None, not %s", arg.Type().Name)
	}
}

func pyIntValue(o py.Object) (int, error) {
	i, ok := o.(py.Int)
	if !ok {
		return 0, py.ExceptionNewf(py.TypeError, "an integer is required, not %s", o.Type().Name)
	}
	return int(i), nil
}

// appendChunk is Header.append.
func (h *HeaderObj) appendChunk(s py.Object, cs *mimeCharset, errors string) error {
	var text string
	switch v := s.(type) {
	case py.String:
		text = string(v)
	case py.Bytes:
		inputCodec := cs.inputCodec
		if inputCodec == "" {
			inputCodec = "us-ascii"
		}
		decoded, err := decodeString(inputCodec, errors, []byte(v))
		if err != nil {
			return err
		}
		text = decoded
	default:
		return py.ExceptionNewf(py.TypeError, "expected str or bytes, got %s", s.Type().Name)
	}
	// Ensure the bytes can be decoded to the output charset, so an error is
	// raised early; a us-ascii failure upgrades the chunk to utf-8.
	outputCharset := cs.outputCodec
	if outputCharset == "" {
		outputCharset = "us-ascii"
	}
	if outputCharset != unknown8bit {
		if _, err := encodeString(outputCharset, errors, text); err != nil {
			if !isUnicodeEncodeError(err) {
				return err
			}
			if outputCharset != "us-ascii" {
				return err
			}
			up, uerr := newCharset("utf-8")
			if uerr != nil {
				return uerr
			}
			cs = up
		}
	}
	h.chunks = append(h.chunks, headerChunk{s: text, cs: cs})
	return nil
}

func isUnicodeEncodeError(err error) bool {
	e, ok := err.(*py.Exception)
	return ok && e.Base == py.UnicodeEncodeError
}

// normalize collapses runs of identical charsets into one chunk.
func (h *HeaderObj) normalize() {
	var chunks []headerChunk
	var lastCS *mimeCharset
	var last []string
	for _, c := range h.chunks {
		if lastCS != nil && c.cs.name == lastCS.name {
			last = append(last, c.s)
			continue
		}
		if lastCS != nil {
			chunks = append(chunks, headerChunk{s: strings.Join(last, " "), cs: lastCS})
		}
		last = []string{c.s}
		lastCS = c.cs
	}
	if lastCS != nil {
		chunks = append(chunks, headerChunk{s: strings.Join(last, " "), cs: lastCS})
	}
	h.chunks = chunks
}

func nonctext(r rune) bool {
	return pyIsSpace(r) || r == '(' || r == ')' || r == '\\'
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}

// headerStr is Header.__str__.
func (h *HeaderObj) headerStr() string {
	h.normalize()
	var uchunks []string
	lastcs := "us-ascii"
	lastspace := false
	for _, c := range h.chunks {
		nextcs := c.cs.name
		s := c.s
		if nextcs == unknown8bit {
			s = asciiReplaceDecode(surrogateEscapeEncode(s))
		}
		if len(uchunks) > 0 {
			hasspace := s != "" && nonctext(firstRune(s))
			if lastcs != "us-ascii" {
				if nextcs == "us-ascii" && !hasspace {
					uchunks = append(uchunks, " ")
				}
			} else if nextcs != "us-ascii" && !lastspace {
				uchunks = append(uchunks, " ")
			}
		}
		lastspace = s != "" && nonctext(lastRune(s))
		lastcs = nextcs
		uchunks = append(uchunks, s)
	}
	return strings.Join(uchunks, "")
}

// asciiReplaceDecode is bytes.decode('ascii', 'replace').
func asciiReplaceDecode(b []byte) string {
	runes := make([]rune, 0, len(b))
	for _, c := range b {
		if c < 0x80 {
			runes = append(runes, rune(c))
		} else {
			runes = append(runes, '\ufffd')
		}
	}
	return string(runes)
}

func headerStr(self py.Object, args py.Tuple) (py.Object, error) {
	return py.String(self.(*HeaderObj).headerStr()), nil
}

func headerRepr(self py.Object, args py.Tuple) (py.Object, error) {
	return py.String(fmt.Sprintf("<%s object at %p>", HeaderType.Name, self.(*HeaderObj))), nil
}

func headerEq(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return py.NotImplemented, nil
	}
	h := self.(*HeaderObj)
	return py.Eq(args[0], py.String(h.headerStr()))
}

func headerNe(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 1 {
		return py.NotImplemented, nil
	}
	r, err := py.Eq(self, args[0])
	if err != nil {
		return nil, err
	}
	if r == py.NotImplemented {
		return py.NotImplemented, nil
	}
	b, err := py.MakeBool(r)
	if err != nil {
		return nil, err
	}
	return py.NewBool(b == py.False), nil
}

func headerAppend(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		s          py.Object
		charsetArg py.Object = py.None
		errorsArg  py.Object = py.String("strict")
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:append",
		[]string{"s", "charset", "errors"}, &s, &charsetArg, &errorsArg); err != nil {
		return nil, err
	}
	h := self.(*HeaderObj)
	var cs *mimeCharset
	if charsetArg == py.None {
		cs = h.charset
	} else {
		var err error
		if cs, err = charsetFromArg(charsetArg); err != nil {
			return nil, err
		}
	}
	if err := h.appendChunk(s, cs, pyStr(errorsArg)); err != nil {
		return nil, err
	}
	return py.None, nil
}

func headerEncode(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		splitchars py.Object = py.String(";, \t")
		maxlenArg  py.Object = py.None
		linesepArg py.Object = py.String("\n")
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOO:encode",
		[]string{"splitchars", "maxlinelen", "linesep"}, &splitchars, &maxlenArg, &linesepArg); err != nil {
		return nil, err
	}
	h := self.(*HeaderObj)
	split, err := py.StrAsString(splitchars)
	if err != nil {
		return nil, err
	}
	linesep, err := py.StrAsString(linesepArg)
	if err != nil {
		return nil, err
	}
	maxlinelen := h.maxlinelen
	if maxlenArg != py.None {
		if maxlinelen, err = pyIntValue(maxlenArg); err != nil {
			return nil, err
		}
	}
	out, err := h.encode(split, maxlinelen, linesep)
	if err != nil {
		return nil, err
	}
	return py.String(out), nil
}

func (h *HeaderObj) encode(splitchars string, maxlinelen int, linesep string) (string, error) {
	h.normalize()
	if maxlinelen == 0 {
		// A maxlinelen of 0 means don't wrap.
		maxlinelen = 1000000
	}
	f := newValueFormatter(h.headerlen, maxlinelen, h.continuationWS, splitchars)
	var lastcs string
	var hasspace, lastspace bool
	haveState := false
	for _, c := range h.chunks {
		if haveState {
			hasspace = c.s != "" && nonctext(firstRune(c.s))
			if lastcs != "us-ascii" {
				if !hasspace || c.cs.name != "us-ascii" {
					f.addTransition()
				}
			} else if c.cs.name != "us-ascii" && !lastspace {
				f.addTransition()
			}
		}
		lastspace = c.s != "" && nonctext(lastRune(c.s))
		lastcs = c.cs.name
		hasspace = false
		haveState = true
		lines := pySplitlines(c.s)
		if len(lines) > 0 {
			if err := f.feed("", lines[0], c.cs); err != nil {
				return "", err
			}
		} else if err := f.feed("", "", c.cs); err != nil {
			return "", err
		}
		for _, line := range lines[1:] {
			f.newline()
			if c.cs.headerEnc != headerEncNone {
				if err := f.feed(h.continuationWS, " "+strings.TrimLeftFunc(line, pyIsSpace), c.cs); err != nil {
					return "", err
				}
			} else {
				sline := strings.TrimLeftFunc(line, pyIsSpace)
				fws := line[:len(line)-len(sline)]
				if err := f.feed(fws, sline, c.cs); err != nil {
					return "", err
				}
			}
		}
		if len(lines) > 1 {
			f.newline()
		}
	}
	if len(h.chunks) > 0 {
		f.addTransition()
	}
	value := f.str(linesep)
	if embeddedHeaderRe.MatchString(value) {
		return "", py.ExceptionNewf(HeaderParseError,
			"header value appears to contain an embedded header: %s", pyReprString(value))
	}
	return value, nil
}

// pyReprString renders a Go string the way Python's str.__repr__ would, which
// the embedded-header error message interpolates.
func pyReprString(s string) string {
	q := "'"
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		q = "\""
	}
	var b strings.Builder
	b.WriteString(q)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r == rune(q[0]) {
				b.WriteString(`\` + q)
			} else if r < 0x20 || r == 0x7f {
				b.WriteString(fmt.Sprintf(`\x%02x`, r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteString(q)
	return b.String()
}

// makeHeaderFunc is email.header.make_header.
func makeHeaderFunc(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var (
		decodedSeq     py.Object
		maxlinelenArg  py.Object = py.None
		headerNameArg  py.Object = py.None
		continuationWS py.Object = py.String(" ")
	)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OOO:make_header",
		[]string{"decoded_seq", "maxlinelen", "header_name", "continuation_ws"},
		&decodedSeq, &maxlinelenArg, &headerNameArg, &continuationWS); err != nil {
		return nil, err
	}
	h, err := headerNew(HeaderType, py.Tuple{py.None, py.None, maxlinelenArg, headerNameArg, continuationWS}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	obj := h.(*HeaderObj)

	iter, err := py.Iter(decodedSeq)
	if err != nil {
		return nil, err
	}
	next := iter.(py.I__next__)
	for {
		item, err := next.M__next__()
		if err == py.StopIteration {
			break
		}
		if err != nil {
			return nil, err
		}
		pair, ok := item.(py.Tuple)
		if !ok || len(pair) != 2 {
			return nil, py.ExceptionNewf(py.TypeError, "decoded_seq must yield (string, charset) pairs")
		}
		s, csArg := pair[0], pair[1]
		var cs *mimeCharset
		if csArg == py.None {
			cs = nil
		} else if csArg, err = nilOrCharset(csArg); err != nil {
			return nil, err
		} else if cs, err = charsetFromArg(csArg); err != nil {
			return nil, err
		}
		if cs == nil {
			cs = obj.charset
		}
		if err := obj.appendChunk(s, cs, "strict"); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

// nilOrCharset normalises the charset element of a decoded pair.
func nilOrCharset(o py.Object) (py.Object, error) {
	switch o.(type) {
	case py.NoneType, py.String:
		return o, nil
	default:
		return nil, py.ExceptionNewf(py.TypeError, "charset must be str or None, not %s", o.Type().Name)
	}
}

// --- value formatter -----------------------------------------------------

type accPart struct{ fws, part string }

type accumulator struct {
	parts       []accPart
	initialSize int
}

func (a *accumulator) length() int {
	n := a.initialSize
	for _, p := range a.parts {
		n += len(p.fws) + len(p.part)
	}
	return n
}

func (a *accumulator) str() string {
	var b strings.Builder
	for _, p := range a.parts {
		b.WriteString(p.fws)
		b.WriteString(p.part)
	}
	return b.String()
}

func (a *accumulator) push(fws, part string) { a.parts = append(a.parts, accPart{fws, part}) }

func (a *accumulator) pop() accPart {
	if len(a.parts) == 0 {
		return accPart{" ", ""}
	}
	p := a.parts[len(a.parts)-1]
	a.parts = a.parts[:len(a.parts)-1]
	return p
}

func (a *accumulator) partCount() int { return len(a.parts) }

func (a *accumulator) popFrom(i int) []accPart {
	popped := append([]accPart{}, a.parts[i:]...)
	a.parts = a.parts[:i]
	return popped
}

func (a *accumulator) reset(start []accPart) {
	a.parts = append([]accPart{}, start...)
	a.initialSize = 0
}

func (a *accumulator) isOnlyWS() bool {
	return a.initialSize == 0 && (len(a.parts) == 0 || isSpaceString(a.str()))
}

type valueFormatter struct {
	maxlen     int
	contWS     string
	contWSLen  int
	splitchars string
	lines      []string
	cur        *accumulator
}

func newValueFormatter(headerlen, maxlen int, contWS, splitchars string) *valueFormatter {
	return &valueFormatter{
		maxlen:     maxlen,
		contWS:     contWS,
		contWSLen:  len(contWS),
		splitchars: splitchars,
		cur:        &accumulator{initialSize: headerlen},
	}
}

func (f *valueFormatter) maxlengths() func() int {
	first := true
	return func() int {
		if first {
			first = false
			return f.maxlen - f.cur.length()
		}
		return f.maxlen - f.contWSLen
	}
}

func (f *valueFormatter) str(linesep string) string {
	f.newline()
	return strings.Join(f.lines, linesep)
}

func (f *valueFormatter) newline() {
	e := f.cur.pop()
	if !(e.fws == " " && e.part == "") {
		f.cur.push(e.fws, e.part)
	}
	if f.cur.length() > 0 {
		if f.cur.isOnlyWS() && len(f.lines) > 0 {
			f.lines[len(f.lines)-1] += f.cur.str()
		} else {
			f.lines = append(f.lines, f.cur.str())
		}
	}
	f.cur.reset(nil)
}

func (f *valueFormatter) addTransition() { f.cur.push(" ", "") }

func (f *valueFormatter) feed(fws, string_ string, cs *mimeCharset) error {
	if cs.headerEnc == headerEncNone {
		f.asciiSplit(fws, string_)
		return nil
	}
	encodedLines, err := headerEncodeLines(cs, string_, f.maxlengths())
	if err != nil {
		return err
	}
	if len(encodedLines) == 0 {
		return nil
	}
	if encodedLines[0] != nil {
		f.appendChunk(fws, *encodedLines[0])
	}
	if len(encodedLines) == 1 {
		return nil
	}
	f.newline()
	last := encodedLines[len(encodedLines)-1]
	f.cur.push(f.contWS, *last)
	for _, l := range encodedLines[1 : len(encodedLines)-1] {
		f.lines = append(f.lines, f.contWS+*l)
	}
	return nil
}

func (f *valueFormatter) asciiSplit(fws, string_ string) {
	parts := splitFWS(fws + string_)
	if len(parts) > 0 && parts[0] != "" {
		parts = append([]string{""}, parts...)
	} else if len(parts) > 0 {
		parts = parts[1:]
	}
	for i := 0; i+1 < len(parts); i += 2 {
		f.appendChunk(parts[i], parts[i+1])
	}
}

func (f *valueFormatter) appendChunk(fws, string_ string) {
	f.cur.push(fws, string_)
	if f.cur.length() <= f.maxlen {
		return
	}
	found := -1
search:
	for _, ch := range f.splitchars {
		for i := f.cur.partCount() - 1; i > 0; i-- {
			if pyIsSpace(ch) {
				fs := f.cur.parts[i].fws
				if fs != "" && firstRune(fs) == ch {
					found = i
					break search
				}
			}
			prev := f.cur.parts[i-1].part
			if prev != "" && lastRune(prev) == ch {
				found = i
				break search
			}
		}
	}
	if found < 0 {
		p := f.cur.pop()
		if f.cur.initialSize > 0 {
			f.newline()
			if p.fws == "" {
				p.fws = " "
			}
		}
		f.cur.push(p.fws, p.part)
		return
	}
	remainder := f.cur.popFrom(found)
	f.lines = append(f.lines, f.cur.str())
	f.cur.reset(remainder)
}

// splitFWS is re.split("([ \t]+)", s): the text before each run of folding
// whitespace, the run itself, and finally the text after the last run - which
// is the empty string when s ends with whitespace.  The trailing empty field
// matters: _ascii_split feeds the parts in pairs and the last one carries the
// chunk's trailing whitespace.
func splitFWS(s string) []string {
	var parts []string
	start := 0
	i := 0
	for i < len(s) {
		if s[i] != ' ' && s[i] != '\t' {
			i++
			continue
		}
		parts = append(parts, s[start:i])
		j := i
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		parts = append(parts, s[i:j])
		i, start = j, j
	}
	parts = append(parts, s[start:])
	return parts
}

// --- RFC 2047 header encoders -------------------------------------------

func base64HeaderLength(b []byte) int {
	groups := len(b) / 3
	n := groups * 4
	if len(b)%3 != 0 {
		n += 4
	}
	return n
}

// quopriHeaderMap is CPython's _QUOPRI_HEADER_MAP.
var quopriHeaderMap [256]string

func init() {
	for i := 0; i < 256; i++ {
		quopriHeaderMap[i] = fmt.Sprintf("=%02X", i)
	}
	safe := "-!*+/" + "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz" + "0123456789"
	for i := 0; i < len(safe); i++ {
		c := safe[i]
		quopriHeaderMap[c] = string(c)
	}
	quopriHeaderMap[' '] = "_"
}

func quopriHeaderLength(b []byte) int {
	n := 0
	for _, c := range b {
		n += len(quopriHeaderMap[c])
	}
	return n
}

func base64HeaderEncode(b []byte, charset string) string {
	if len(b) == 0 {
		return ""
	}
	return fmt.Sprintf("=?%s?b?%s?=", charset, base64.StdEncoding.EncodeToString(b))
}

func quopriHeaderEncode(b []byte, charset string) string {
	if len(b) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, c := range b {
		sb.WriteString(quopriHeaderMap[c])
	}
	return fmt.Sprintf("=?%s?q?%s?=", charset, sb.String())
}

// headerEncodeLines is Charset.header_encode_lines.
func headerEncodeLines(cs *mimeCharset, text string, maxlengths func() int) ([]*string, error) {
	codec := cs.outputCodec
	if codec == "" {
		codec = "us-ascii"
	}
	headerBytes, err := encodeString(codec, "strict", text)
	if err != nil {
		return nil, err
	}
	useBase64 := false
	switch cs.headerEnc {
	case headerEncBase64:
		useBase64 = true
	case headerEncQP:
		useBase64 = false
	case headerEncShortest:
		useBase64 = base64HeaderLength(headerBytes) < quopriHeaderLength(headerBytes)
	}
	outCharset := cs.outputName()
	extra := len(outCharset) + rfc2047Chrome

	var lines []*string
	var current []rune
	maxlen := maxlengths() - extra
	for _, ch := range text {
		current = append(current, ch)
		thisLine := string(current)
		b, err := encodeString(outCharset, "strict", thisLine)
		if err != nil {
			return nil, err
		}
		length := quopriHeaderLength(b)
		if useBase64 {
			length = base64HeaderLength(b)
		}
		if length > maxlen {
			current = current[:len(current)-1]
			if len(lines) == 0 && len(current) == 0 {
				lines = append(lines, nil)
			} else {
				jb, err := encodeString(codec, "strict", string(current))
				if err != nil {
					return nil, err
				}
				s := encodeOneLine(useBase64, jb, codec)
				lines = append(lines, &s)
			}
			current = []rune{ch}
			maxlen = maxlengths() - extra
		}
	}
	jb, err := encodeString(codec, "strict", string(current))
	if err != nil {
		return nil, err
	}
	s := encodeOneLine(useBase64, jb, codec)
	lines = append(lines, &s)
	return lines, nil
}

func encodeOneLine(useBase64 bool, b []byte, codec string) string {
	if useBase64 {
		return base64HeaderEncode(b, codec)
	}
	return quopriHeaderEncode(b, codec)
}

// --- shared string helpers ----------------------------------------------

// pyIsSpace is str.isspace() for a single character.  Go's unicode.IsSpace
// lacks the C0 separators Python counts as whitespace.
func pyIsSpace(r rune) bool {
	if r < 0x80 {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0x1c, 0x1d, 0x1e, 0x1f:
			return true
		}
		return false
	}
	return unicode.IsSpace(r)
}

func isSpaceString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !pyIsSpace(r) {
			return false
		}
	}
	return true
}

// pySplitlines splits on Python's line boundaries, without keeping the ends.
func pySplitlines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		w := 0
		switch {
		case r == '\n' || r == '\v' || r == '\f' || r == 0x1c || r == 0x1d || r == 0x1e || r == 0x85 || r == '\u2028' || r == '\u2029':
			w = size
		case r == '\r':
			w = size
			if i+size < len(s) && s[i+size] == '\n' {
				w = size + 1
			}
		}
		if w > 0 {
			out = append(out, s[start:i])
			i += w
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func init() {
	HeaderType = py.NewTypeX("email.header.Header", header_doc, headerNew, nil)

	globals := py.NewStringDict()
	globals.Set("Header", HeaderType)
	globals.Set("decode_header", py.MustNewMethod("decode_header", decodeHeaderFunc, 0,
		"decode_header(header) -> list of (bytes, charset) decoded parts."))
	globals.Set("make_header", py.MustNewMethod("make_header", makeHeaderFunc, 0,
		"make_header(decoded_seq, maxlinelen=None, header_name=None, continuation_ws=' ') -> Header"))

	register := func(name string, fn interface{}, doc string) {
		HeaderType.Dict.Set(name, py.MustNewMethod(name, fn, 0, doc))
	}
	register("__str__", headerStr, "Return the string value of the header.")
	register("__repr__", headerRepr, "Return a representation of the header.")
	register("__eq__", headerEq, "Compare the header to a Header or a string.")
	register("__ne__", headerNe, "Compare the header to a Header or a string.")
	register("append", headerAppend, "Append a string to the MIME header.")
	register("encode", headerEncode, "Encode the header into an RFC 2047 format.")

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "email.header",
			Doc:  header_doc,
		},
		Globals: globals,
	})
}
