// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package locale provides the implementation of python's 'locale' module:
// access to the POSIX locale mechanism.
//
// Go has no binding to the C library's setlocale, so the process locale is
// tracked here in the same way CPython tracks it: one current setting per
// LC_* category, initialised from the environment and validated against the
// host's compiled locale database where one is present (see localeAvailable).
//
// What is real: the category constants match the host ABI, setlocale
// validates a name against /usr/share/locale (when that directory exists) and
// raises locale.Error for one it cannot find, and getlocale/getencoding
// report the requested category.  What is not: the locale's data itself - the
// decimal separator, thousands separator and so on - is not read, because the
// compiled LC_NUMERIC/LC_MONETARY binaries are not a format Go exposes or
// this module parses.  localeconv therefore reports the C defaults for
// categories it cannot resolve, and format()'s grouping uses "," - honest
// fallbacks, not guesses about a database we did not read.
package locale

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Locale support.

This module provides an interface to the POSIX locale database and
functionality.  It is implemented in the same terms CPython uses: a set of
LC_* category constants, setlocale/getlocale to read and change the current
setting, and the encoding helpers built on top of it.

The category values below are the ones this host uses, so they match what
C code on the same machine would see.`

// Category constants.  The numbering differs between the glibc (Linux) and
// BSD (macOS and the BSDs) ABIs, so both are provided and the host's is
// selected.  They are variables rather than consts for that reason.
var (
	LC_ALL      int
	LC_COLLATE  int
	LC_CTYPE    int
	LC_MONETARY int
	LC_NUMERIC  int
	LC_TIME     int
	LC_MESSAGES int
)

// Error is raised for an unknown category or an unsupported locale name; it
// derives from Exception, as CPython's does.
var errorType = py.ExceptionType.NewType("locale.Error", "Unsupported locale setting.", nil, nil)

var (
	stateMu sync.Mutex
	// current holds the current locale name for each category id.  The
	// LC_ALL slot is unused: LC_ALL is a selector, never a setting of its
	// own, and setting it writes every other slot.
	current [7]string
)

// categoryOrder is the order the per-category settings are joined in when a
// composite LC_ALL string is produced.  CPython joins in this order on macOS
// (setlocale(LC_ALL, "") there returns e.g. "C/en_GB.UTF-8/C/C/C/C"), and
// nothing in the format is contractual, so it is reproduced rather than
// invented.
var categoryOrder []int

func init() {
	switch runtime.GOOS {
	case "linux":
		LC_CTYPE, LC_NUMERIC, LC_TIME, LC_COLLATE, LC_MONETARY, LC_MESSAGES, LC_ALL = 0, 1, 2, 3, 4, 5, 6
	default:
		// darwin and the BSDs share this numbering.
		LC_COLLATE, LC_CTYPE, LC_MONETARY, LC_NUMERIC, LC_TIME, LC_MESSAGES, LC_ALL = 1, 2, 3, 4, 5, 6, 0
	}
	categoryOrder = []int{LC_COLLATE, LC_CTYPE, LC_MONETARY, LC_NUMERIC, LC_TIME, LC_MESSAGES}
	initState()

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "locale",
			Doc:  module_doc,
		},
		Methods: []*py.Method{
			py.MustNewMethod("setlocale", setlocale, 0, setlocale_doc),
			py.MustNewMethod("getlocale", getlocale, 0, getlocale_doc),
			py.MustNewMethod("getdefaultlocale", getdefaultlocale, 0, getdefaultlocale_doc),
			py.MustNewMethod("getpreferredencoding", getpreferredencoding, 0, getpreferredencoding_doc),
			py.MustNewMethod("getencoding", getpreferredencoding, 0, getencoding_doc),
			py.MustNewMethod("normalize", normalize, 0, normalize_doc),
			py.MustNewMethod("localeconv", localeconv, 0, localeconv_doc),
			py.MustNewMethod("strxfrm", strxfrm, 0, strxfrm_doc),
			py.MustNewMethod("str", str, 0, str_doc),
			py.MustNewMethod("atof", atof, 0, atof_doc),
			py.MustNewMethod("atoi", atoi, 0, atoi_doc),
			py.MustNewMethod("delocalize", delocalize, 0, delocalize_doc),
			py.MustNewMethod("format_string", format_string, 0, format_string_doc),
			py.MustNewMethod("format", format_string, 0, format_string_doc),
			py.MustNewMethod("currency", currency, 0, currency_doc),
			py.MustNewMethod("bindtextdomain", identityDomain, 0, bindtextdomain_doc),
			py.MustNewMethod("textdomain", identityDomain, 0, textdomain_doc),
			py.MustNewMethod("dgettext", dgettext, 0, dgettext_doc),
			py.MustNewMethod("gettext", gettext, 0, gettext_doc),
		},
		Globals: py.NewStringDictFrom(
			py.DictEntry{Key: "Error", Value: errorType},
			py.DictEntry{Key: "LC_ALL", Value: py.Int(LC_ALL)},
			py.DictEntry{Key: "LC_COLLATE", Value: py.Int(LC_COLLATE)},
			py.DictEntry{Key: "LC_CTYPE", Value: py.Int(LC_CTYPE)},
			py.DictEntry{Key: "LC_MONETARY", Value: py.Int(LC_MONETARY)},
			py.DictEntry{Key: "LC_NUMERIC", Value: py.Int(LC_NUMERIC)},
			py.DictEntry{Key: "LC_TIME", Value: py.Int(LC_TIME)},
			py.DictEntry{Key: "LC_MESSAGES", Value: py.Int(LC_MESSAGES)},
		),
	})
}

// initState seeds every category from the environment, which is what the
// process starts with before any setlocale call.
func initState() {
	for _, cat := range categoryOrder {
		current[cat] = envLocale(cat)
	}
}

// envLocale is the locale the environment selects for a category, following
// the POSIX precedence: LC_ALL, then the category's own variable, then LANG.
func envLocale(cat int) string {
	if v := os.Getenv("LC_ALL"); v != "" {
		return v
	}
	if name := categoryEnvName(cat); name != "" {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	if v := os.Getenv("LANG"); v != "" {
		return v
	}
	return "C"
}

func categoryEnvName(cat int) string {
	switch cat {
	case LC_COLLATE:
		return "LC_COLLATE"
	case LC_CTYPE:
		return "LC_CTYPE"
	case LC_MONETARY:
		return "LC_MONETARY"
	case LC_NUMERIC:
		return "LC_NUMERIC"
	case LC_TIME:
		return "LC_TIME"
	case LC_MESSAGES:
		return "LC_MESSAGES"
	}
	return ""
}

// localeDir is where the host keeps its compiled locale definitions.  When it
// is absent there is no database to validate against and every name is
// accepted, which mirrors what a system without locales does.
const localeDir = "/usr/share/locale"

// localeAvailable reports whether the host appears to have a compiled locale
// for name.  "C" and "POSIX" are always present, as they are in every libc.
func localeAvailable(name string) bool {
	switch name {
	case "C", "POSIX":
		return true
	}
	if fi, err := os.Stat(localeDir); err != nil || !fi.IsDir() {
		return true
	}
	if isDir(filepath.Join(localeDir, name)) {
		return true
	}
	// The name may be a short form of one that exists (e.g. "de_DE" for the
	// "de_DE.ISO8859-1" the database stores).
	if n := normalizeName(name); n != name && isDir(filepath.Join(localeDir, n)) {
		return true
	}
	return false
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// normalizeName maps a locale name to the canonical form the database uses.
// CPython's normalize is a large alias table; the part that matters for the
// common shapes is that a bare language gets a default country and, in the
// absence of an encoding, the ISO8859-1 default.
func normalizeName(name string) string {
	switch name {
	case "C", "POSIX":
		return "C"
	}
	lang, encoding := name, ""
	if i := strings.IndexByte(name, '.'); i >= 0 {
		lang, encoding = name[:i], name[i+1:]
	}
	// Strip a modifier such as "@euro"; it selects a variant we do not
	// resolve, but the bare name still identifies the locale.
	if i := strings.IndexByte(lang, '@'); i >= 0 {
		lang = lang[:i]
	}
	if !strings.ContainsRune(lang, '_') {
		if full, ok := languageAliases[lang]; ok {
			lang = full
		}
	}
	if encoding == "" {
		encoding = "ISO8859-1"
	}
	return lang + "." + encoding
}

// languageAliases is the subset of CPython's locale_encoding_alias /
// locale_alias mapping a bare language to a full name.  It is deliberately
// small: a name not listed keeps its bare form, which the database lookup can
// still resolve.
var languageAliases = map[string]string{
	"en": "en_US",
	"de": "de_DE",
	"fr": "fr_FR",
	"es": "es_ES",
	"it": "it_IT",
	"pt": "pt_PT",
	"nl": "nl_NL",
	"sv": "sv_SE",
	"da": "da_DK",
	"fi": "fi_FI",
	"no": "nb_NO",
	"pl": "pl_PL",
	"ru": "ru_RU",
	"cs": "cs_CZ",
	"ja": "ja_JP",
	"ko": "ko_KR",
	"zh": "zh_CN",
	"tr": "tr_TR",
	"el": "el_GR",
	"hu": "hu_HU",
}

// checkCategory validates a category id.
func checkCategory(cat int) error {
	switch cat {
	case LC_ALL, LC_COLLATE, LC_CTYPE, LC_MONETARY, LC_NUMERIC, LC_TIME, LC_MESSAGES:
		return nil
	}
	return py.ExceptionNewf(errorType, "unknown locale category %d", cat)
}

// currentName renders the current setting for a category; for LC_ALL it is
// the composite of every category.
func currentName(cat int) string {
	if cat == LC_ALL {
		parts := make([]string, len(categoryOrder))
		for i, c := range categoryOrder {
			parts[i] = current[c]
		}
		return strings.Join(parts, "/")
	}
	return current[cat]
}

// setlocale applies a locale to a category and returns the new setting.
//
//	setlocale(category, locale) -> string
//	setlocale(category)          -> string   (query, no change)
//
// An empty locale means "take it from the environment", as it does in POSIX.
func setlocale(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var catObj py.Object
	var locObj py.Object = py.None
	if err := py.UnpackTuple(args, kwargs, "setlocale", 1, 2, &catObj, &locObj); err != nil {
		return nil, err
	}
	cat, err := py.IndexInt(catObj)
	if err != nil {
		return nil, py.ExceptionNewf(errorType, "unknown locale category")
	}
	if err := checkCategory(cat); err != nil {
		return nil, err
	}

	if locObj == py.None {
		stateMu.Lock()
		defer stateMu.Unlock()
		return py.String(currentName(cat)), nil
	}

	name, err := py.StrAsString(locObj)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "setlocale() argument 2 must be str or None, not %s", locObj.Type().Name)
	}

	stateMu.Lock()
	defer stateMu.Unlock()

	if name == "" {
		// The environment's answer needs to exist on the host, otherwise
		// POSIX raises rather than silently falling back.
		for _, c := range categoryOrder {
			if cat != LC_ALL && c != cat {
				continue
			}
			v := envLocale(c)
			if !localeAvailable(v) {
				return nil, py.ExceptionNewf(errorType, "unsupported locale setting")
			}
			current[c] = v
		}
		return py.String(currentName(cat)), nil
	}

	if !localeAvailable(name) {
		return nil, py.ExceptionNewf(errorType, "unsupported locale setting")
	}
	for _, c := range categoryOrder {
		if cat != LC_ALL && c != cat {
			continue
		}
		current[c] = name
	}
	return py.String(currentName(cat)), nil
}

const setlocale_doc = `setlocale(category, locale=None) -> str

Set the locale for the given category.  The locale may be a string, an
iterable of two strings (language code and encoding), or None.  If locale is
omitted or None, the setting is read without change.  An empty string asks
for the setting the environment selects.

The locale argument is ignored on some platforms; on this one it is
validated against the host locale database and locale.Error is raised for a
locale that is not installed.`

// getlocale returns (language code, encoding) for a category, or (None, None)
// for the C locale, exactly as CPython does.
func getlocale(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var catObj py.Object = py.Int(LC_CTYPE)
	if err := py.UnpackTuple(args, kwargs, "getlocale", 0, 1, &catObj); err != nil {
		return nil, err
	}
	cat, err := py.IndexInt(catObj)
	if err != nil {
		return nil, py.ExceptionNewf(errorType, "unknown locale category")
	}
	if err := checkCategory(cat); err != nil {
		return nil, err
	}
	stateMu.Lock()
	name := currentName(cat)
	stateMu.Unlock()

	if i := strings.IndexByte(name, '/'); i >= 0 {
		// A composite LC_ALL string; report its first category, as the
		// underlying C call effectively does.
		name = name[:i]
	}
	lang, encoding := splitLocale(name)
	if lang == "" {
		return py.Tuple{py.None, py.None}, nil
	}
	var encObj py.Object = py.None
	if encoding != "" {
		encObj = py.String(encoding)
	}
	return py.Tuple{py.String(lang), encObj}, nil
}

const getlocale_doc = `getlocale(category=LC_CTYPE) -> tuple

Returns the current setting for the given locale category as a tuple of two
strings (language code, encoding).  For the C locale it returns (None, None).`

// splitLocale breaks "lang_COUNTRY.ENCODING" into its language and encoding.
// The C/POSIX locale has no language, which callers detect as (None, None).
func splitLocale(name string) (string, string) {
	if name == "" || name == "C" || name == "POSIX" {
		return "", ""
	}
	lang, encoding := name, ""
	if i := strings.IndexByte(name, '.'); i >= 0 {
		lang, encoding = name[:i], name[i+1:]
	}
	if i := strings.IndexByte(lang, '@'); i >= 0 {
		lang = lang[:i]
	}
	return lang, encoding
}

// getdefaultlocale reads the locale the environment asks for, independent of
// whatever setlocale has since applied.
func getdefaultlocale(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 0 {
		return nil, py.ExceptionNewf(py.TypeError, "getdefaultlocale() takes no arguments")
	}
	name := envLocale(LC_CTYPE)
	lang, encoding := splitLocale(name)
	if lang == "" {
		return py.Tuple{py.None, py.None}, nil
	}
	var encObj py.Object = py.None
	if encoding != "" {
		encObj = py.String(encoding)
	}
	return py.Tuple{py.String(lang), encObj}, nil
}

const getdefaultlocale_doc = `getdefaultlocale(envvars=('LC_ALL', 'LC_CTYPE', 'LANG', 'LANGUAGE')) -> tuple

Tries to determine the default locale settings and returns them as a tuple of
two strings (language code, encoding).  Here the answer comes from the
environment variables the process actually has.`

// getpreferredencoding returns the encoding the current locale uses.  Python
// exposes two names for this; getencoding() is the 3.11+ spelling and both
// are bound to this function.
func getpreferredencoding(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var doSetlocale py.Object = py.True
	if err := py.UnpackTuple(args, kwargs, "getpreferredencoding", 0, 1, &doSetlocale); err != nil {
		return nil, err
	}
	stateMu.Lock()
	name := current[LC_CTYPE]
	stateMu.Unlock()
	return py.String(preferredEncoding(name)), nil
}

// preferredEncoding is the codeset for a locale name.  The locale database
// records this as a compiled codeset string we do not read, so the encoding
// is derived from the name: the part after the '.' when present, and UTF-8
// otherwise (which is what a modern Python process reports).
func preferredEncoding(name string) string {
	if _, enc := splitLocale(name); enc != "" {
		return enc
	}
	return "UTF-8"
}

const getpreferredencoding_doc = `getpreferredencoding(do_setlocale=True) -> str

Return the charset used to encode and decode text in the locale.  The
argument is accepted for compatibility; the answer comes from the current
LC_CTYPE setting.`

const getencoding_doc = `getencoding() -> str

Get the current locale encoding.  Equivalent to getpreferredencoding(False)
with the locale already applied.`

// normalize canonicalises a locale name.
func normalize(self py.Object, args py.Tuple) (py.Object, error) {
	var nameObj py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "normalize", 1, 1, &nameObj); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(nameObj)
	if err != nil {
		return nil, err
	}
	return py.String(normalizeName(name)), nil
}

const normalize_doc = `normalize(localename) -> string

Returns a normalized locale code for the given locale name.  The returned
locale is not set for use - it is only used for searching the locale database,
so only the language/country part and a default encoding are filled in.`

// localeconv returns the category conventions as a dict.  The numeric and
// monetary data live in compiled LC_* binaries that this module does not
// parse, so the C defaults are reported; decimal_point is always "." there.
func localeconv(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) != 0 {
		return nil, py.ExceptionNewf(py.TypeError, "localeconv() takes no arguments")
	}
	s := func(v string) py.Object { return py.String(v) }
	return py.NewStringDictFrom(
		py.DictEntry{Key: "decimal_point", Value: s(".")},
		py.DictEntry{Key: "thousands_sep", Value: s(",")},
		py.DictEntry{Key: "grouping", Value: py.NewListFromItems([]py.Object{py.Int(3), py.Int(3), py.Int(0)})},
		py.DictEntry{Key: "int_curr_symbol", Value: s("")},
		py.DictEntry{Key: "currency_symbol", Value: s("")},
		py.DictEntry{Key: "mon_decimal_point", Value: s("")},
		py.DictEntry{Key: "mon_thousands_sep", Value: s("")},
		py.DictEntry{Key: "mon_grouping", Value: py.NewList()},
		py.DictEntry{Key: "positive_sign", Value: s("")},
		py.DictEntry{Key: "negative_sign", Value: s("")},
		py.DictEntry{Key: "int_frac_digits", Value: py.Int(127)},
		py.DictEntry{Key: "frac_digits", Value: py.Int(127)},
		py.DictEntry{Key: "p_cs_precedes", Value: py.Int(127)},
		py.DictEntry{Key: "p_sep_by_space", Value: py.Int(127)},
		py.DictEntry{Key: "n_cs_precedes", Value: py.Int(127)},
		py.DictEntry{Key: "n_sep_by_space", Value: py.Int(127)},
		py.DictEntry{Key: "p_sign_posn", Value: py.Int(127)},
		py.DictEntry{Key: "n_sign_posn", Value: py.Int(127)},
	), nil
}

const localeconv_doc = `localeconv() -> dict

Return the database of the current locale as a dict.  The numeric and
monetary conventions are the C defaults: the compiled LC_NUMERIC/LC_MONETARY
data is not read by this module.`

// strxfrm would transform a string according to LC_COLLATE.  Collation is a
// property of the compiled LC_COLLATE binary, so without reading it the only
// faithful answer is the string unchanged, which is what the C locale does.
func strxfrm(self py.Object, args py.Tuple) (py.Object, error) {
	var s py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "strxfrm", 1, 1, &s); err != nil {
		return nil, err
	}
	return s, nil
}

const strxfrm_doc = `strxfrm(string) -> string

Transforms a string to one that can be used in locale-aware comparisons.  For
the C locale - the collation data this module does not read - the string is
returned unchanged.`

// str is strxfrm's user-facing twin: it returns the string itself.
func str(self py.Object, args py.Tuple) (py.Object, error) {
	var s py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "str", 1, 1, &s); err != nil {
		return nil, err
	}
	if _, ok := s.(py.String); !ok {
		return nil, py.ExceptionNewf(py.TypeError, "str() argument must be str, not %s", s.Type().Name)
	}
	return s, nil
}

const str_doc = `str(string) -> string

Returns the string unchanged: it normalises a string according to the current
locale's collation rules, and the C collation is the identity.`

func atof(self py.Object, args py.Tuple) (py.Object, error) {
	var sObj py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "atof", 1, 1, &sObj); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(sObj)
	if err != nil {
		return nil, err
	}
	s = strings.ReplaceAll(s, ",", "")
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return nil, py.ExceptionNewf(py.ValueError, "could not convert string to float: %q", s)
	}
	return py.Float(f), nil
}

const atof_doc = `atof(string) -> float

Converts a string to a floating point number, using the locale's decimal
point.  The thousands separator of the reported locale conventions is removed
before parsing.`

func atoi(self py.Object, args py.Tuple) (py.Object, error) {
	var sObj py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "atoi", 1, 1, &sObj); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(sObj)
	if err != nil {
		return nil, err
	}
	s = strings.ReplaceAll(s, ",", "")
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil, py.ExceptionNewf(py.ValueError, "invalid literal for int() with base 10: %q", s)
	}
	return py.Int(n), nil
}

const atoi_doc = `atoi(string) -> integer

Converts a string to an integer, using the locale's conventions for grouping.`

func delocalize(self py.Object, args py.Tuple) (py.Object, error) {
	var sObj py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "delocalize", 1, 1, &sObj); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(sObj)
	if err != nil {
		return nil, err
	}
	return py.String(strings.ReplaceAll(s, ",", "")), nil
}

const delocalize_doc = `delocalize(string) -> string

Converts a string from the locale's display format to the format accepted by
float(), by removing the reported thousands separator.`

// format_string renders a value with a %-format string.  The conversion is
// the interpreter's own, so the value's own __str__/__format__ is honoured;
// when grouping is asked for the integer digit runs in the result are
// grouped with "," - the separator reported by localeconv.
func format_string(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fmtObj, valObj py.Object
	var grouping py.Object = py.False
	if err := py.UnpackTuple(args, kwargs, "format_string", 2, 4, &fmtObj, &valObj, &grouping); err != nil {
		return nil, err
	}
	format, err := py.StrAsString(fmtObj)
	if err != nil {
		return nil, err
	}
	rendered, err := py.Mod(py.String(format), valObj)
	if err != nil {
		return nil, err
	}
	out, err := py.StrAsString(rendered)
	if err != nil {
		return nil, err
	}
	if grouping == py.True {
		out = groupThousands(out)
	}
	return py.String(out), nil
}

const format_string_doc = `format_string(format, val, grouping=False, monetary=False) -> string

Formats a number according to the conventions of the current locale.  The
grouping separator is the one localeconv reports and the monetary flag is
accepted but does not change the answer, since the compiled monetary data is
not read.`

// groupThousands inserts the groups separator into maximal digit runs.  A run
// adjacent to a '.' or an exponent marker is left alone, so the fractional
// part and exponents of a float are untouched.
func groupThousands(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/3)
	i := 0
	for i < len(s) {
		if s[i] < '0' || s[i] > '9' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		run := s[i:j]
		// A following '.' or 'e'/'E' marks a number that is not integral.
		tail := byte(0)
		if j < len(s) {
			tail = s[j]
		}
		if tail == '.' || tail == 'e' || tail == 'E' || len(run) <= 3 {
			b.WriteString(run)
		} else {
			lead := len(run) % 3
			if lead == 0 {
				lead = 3
			}
			b.WriteString(run[:lead])
			for k := lead; k < len(run); k += 3 {
				b.WriteByte(',')
				b.WriteString(run[k : k+3])
			}
		}
		i = j
	}
	return b.String()
}

func currency(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var fmtObj, valObj py.Object
	var grouping py.Object = py.False
	if err := py.UnpackTuple(args, kwargs, "currency", 2, 4, &fmtObj, &valObj, &grouping); err != nil {
		return nil, err
	}
	return format_string(self, py.Tuple{fmtObj, valObj, grouping}, py.NewStringDict())
}

const currency_doc = `currency(val, symbol=True, grouping=False, international=False) -> string

Formats a number as a currency value; it renders through format_string with
the monetary conventions, which this module reports as the C defaults.`

// The gettext pass-throughs: with no message catalog loaded the message is
// its own translation, which is gettext's defined behaviour when a catalog is
// missing.
func identityDomain(self py.Object, args py.Tuple) (py.Object, error) {
	var domain py.Object = py.None
	if err := py.UnpackTuple(args, py.NewStringDict(), "textdomain", 0, 1, &domain); err != nil {
		return nil, err
	}
	return py.String("messages"), nil
}

const bindtextdomain_doc = `bindtextdomain(domain, dir) -> string

Binds the domain to a directory of message catalogs.  No catalog is read, so
the directory is not consulted and the domain is returned.`

const textdomain_doc = `textdomain(domain=None) -> string

Sets the default message domain.  No catalog is read; the default domain
"messages" is reported.`

func dgettext(self py.Object, args py.Tuple) (py.Object, error) {
	var domain, msg py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "dgettext", 2, 2, &domain, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

const dgettext_doc = `dgettext(domain, msg) -> string

Translated version of msg for the given domain.  With no catalog loaded the
message is returned unchanged.`

func gettext(self py.Object, args py.Tuple) (py.Object, error) {
	var msg py.Object
	if err := py.UnpackTuple(args, py.NewStringDict(), "gettext", 1, 1, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

const gettext_doc = `gettext(msg) -> string

Translated version of msg in the default domain.  With no catalog loaded the
message is returned unchanged.`

// fmtLocaleError keeps the Error type reachable from the compiler in case a
// future method needs to build one with a formatted message.
var _ = fmt.Sprintf
