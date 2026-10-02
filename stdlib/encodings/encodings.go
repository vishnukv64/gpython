// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package encodings provides the per-codec modules: encodings.utf_8,
// encodings.ascii, encodings.latin_1 and the aliases table.
//
// CPython spells its codecs as "one module per encoding, each with
// IncrementalDecoder/IncrementalEncoder/Codec classes", and code reaches for
// them by name: "importlib.import_module(f'encodings.{name}').IncrementalDecoder"
// is how charset_normalizer -- a dependency of requests -- probes an encoding.
// So these are registered as dotted modules rather than folded into codecs.
//
// Only the encodings the interpreter can actually convert are provided.
// A name with no table raises LookupError instead of guessing, because a codec
// that silently mis-decodes is worse than one that is absent.

package encodings

import (
	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/codecs"
)

const module_doc = `encodings -- the per-codec modules used by codecs.lookup.`

// codecModule builds one encodings.<name> module.
//
// The classes are built on codecs.IncrementalDecoder/Encoder, and carry
// "_encoding" so the base constructor knows which conversion to perform.
func codecModule(name, canonical string, aliasNames ...string) *py.ModuleImpl {
	globals := py.NewStringDict()

	globals.Set("IncrementalDecoder", codecs.LookupIncrementalDecoder(canonical))
	globals.Set("IncrementalEncoder", codecs.LookupIncrementalEncoder(canonical))
	globals.Set("StreamReader", codecs.LookupIncrementalDecoder(canonical))
	globals.Set("StreamWriter", codecs.LookupIncrementalEncoder(canonical))

	// CPython's encodings modules expose these two, and code that walks an
	// encoding checks them to find out what it is dealing with.
	globals.Set("getregentry", codecs.LookupIncrementalDecoder(canonical))

	aliases := py.NewList()
	for _, a := range aliasNames {
		aliases.Append(py.String(a))
	}
	globals.Set("aliases", aliases)

	return &py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "encodings." + name,
			Doc:  "Codec for " + canonical + ".",
		},
		Globals: globals,
	}
}

func init() {
	// The name mappings CPython's encodings.aliases carries.  Only the aliases
	// for encodings this interpreter actually implements are listed - an entry
	// whose target is missing would turn a clean LookupError into a confusing
	// one after the lookup had already succeeded.
	// CPython's aliases maps NON-CANONICAL names onto the module name, and
	// deliberately does not contain the canonical ones: aliases["utf8"] is
	// "utf_8" but aliases["utf_8"] is absent, and aliases["latin1"] is
	// "latin_1" while aliases["latin-1"] is absent.  Adding the extra
	// spellings looked helpful and was simply wrong - code that uses this table
	// to decide whether a name is already canonical gets the opposite answer.
	aliases := py.NewStringDictFrom(
		py.DictEntry{Key: "utf8", Value: py.String("utf_8")},
		py.DictEntry{Key: "utf", Value: py.String("utf_8")},
		py.DictEntry{Key: "u8", Value: py.String("utf_8")},
		py.DictEntry{Key: "cp65001", Value: py.String("utf_8")},
		py.DictEntry{Key: "utf8_ucs2", Value: py.String("utf_8")},
		py.DictEntry{Key: "utf8_ucs4", Value: py.String("utf_8")},
		py.DictEntry{Key: "utf_8_sig", Value: py.String("utf_8_sig")},
		py.DictEntry{Key: "utf_8_signed", Value: py.String("utf_8_sig")},
		py.DictEntry{Key: "646", Value: py.String("ascii")},
		py.DictEntry{Key: "us_ascii", Value: py.String("ascii")},
		py.DictEntry{Key: "us", Value: py.String("ascii")},
		py.DictEntry{Key: "ansi_x3.4_1968", Value: py.String("ascii")},
		py.DictEntry{Key: "ansi_x3_4_1968", Value: py.String("ascii")},
		py.DictEntry{Key: "ansi_x3.4_1986", Value: py.String("ascii")},
		py.DictEntry{Key: "cp367", Value: py.String("ascii")},
		py.DictEntry{Key: "csascii", Value: py.String("ascii")},
		py.DictEntry{Key: "ibm367", Value: py.String("ascii")},
		py.DictEntry{Key: "iso646_us", Value: py.String("ascii")},
		py.DictEntry{Key: "iso_646.irv_1991", Value: py.String("ascii")},
		py.DictEntry{Key: "latin", Value: py.String("latin_1")},
		py.DictEntry{Key: "latin1", Value: py.String("latin_1")},
		py.DictEntry{Key: "iso8859_1", Value: py.String("latin_1")},
		py.DictEntry{Key: "8859", Value: py.String("latin_1")},
		py.DictEntry{Key: "cp819", Value: py.String("latin_1")},
		py.DictEntry{Key: "l1", Value: py.String("latin_1")},
		py.DictEntry{Key: "iso_8859_1", Value: py.String("latin_1")},
		py.DictEntry{Key: "iso_8859_1_1987", Value: py.String("latin_1")},
		py.DictEntry{Key: "csisolatin1", Value: py.String("latin_1")},
		py.DictEntry{Key: "utf16", Value: py.String("utf_16")},
		py.DictEntry{Key: "u16", Value: py.String("utf_16")},
		py.DictEntry{Key: "utf_16le", Value: py.String("utf_16_le")},
		py.DictEntry{Key: "utf_16be", Value: py.String("utf_16_be")},
		py.DictEntry{Key: "utf32", Value: py.String("utf_32")},
		py.DictEntry{Key: "u32", Value: py.String("utf_32")},
		py.DictEntry{Key: "utf_32le", Value: py.String("utf_32_le")},
		py.DictEntry{Key: "utf_32be", Value: py.String("utf_32_be")},
		py.DictEntry{Key: "unicode_1_1_utf_7", Value: py.String("utf_7")},
	)

	aliasesGlobals := py.NewStringDict()
	aliasesGlobals.Set("aliases", aliases)
	aliasesGlobals.Set("__doc__", py.String("Aliases for the encodings this interpreter provides."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "encodings.aliases",
			Doc:  "Aliases for the encodings this interpreter provides.",
		},
		Globals: aliasesGlobals,
	})

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "encodings",
			Doc:  module_doc,
		},
		Globals: py.NewStringDict(),
	})

	// One module per implemented encoding.
	for _, enc := range []struct {
		name      string
		canonical string
		aliases   []string
	}{
		{"utf_8", "utf-8", []string{"utf8", "utf", "u8", "cp65001"}},
		{"utf_8_sig", "utf-8", []string{"utf-8-sig"}},
		{"ascii", "ascii", []string{"646", "us-ascii", "ansi-x3.4-1968"}},
		{"latin_1", "latin-1", []string{"latin", "latin1", "iso8859-1", "8859", "cp819", "l1"}},
		{"iso8859_1", "latin-1", []string{"iso8859-1", "latin1"}},
		{"utf_16", "utf-16", []string{"utf16", "U16"}},
		{"utf_16_le", "utf-16-le", []string{"utf-16-le"}},
		{"utf_16_be", "utf-16-be", []string{"utf-16-be"}},
		{"utf_32", "utf-32", []string{"utf32", "U32"}},
		{"utf_32_le", "utf-32-le", []string{"utf-32-le"}},
		{"utf_32_be", "utf-32-be", []string{"utf-32-be"}},
	} {
		py.RegisterModule(codecModule(enc.name, enc.canonical, enc.aliases...))
	}
}
