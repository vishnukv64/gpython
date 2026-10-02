// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gettext provides the implementation of python's 'gettext' module:
// message translation.
//
// gettext and ngettext are implemented so that code written as
// "from gettext import gettext as _" runs and returns the message it was
// given.  Without a compiled message catalog there is nothing to translate
// to, so the untranslated string is the correct answer: that is exactly what
// CPython's gettext does when no catalog is found for the current language.
//
// The catalog classes (GNUTranslations and the lookup functions) exist so
// that code referencing them keeps working.  They do not read .mo files:
// that needs the binary catalog format, and no caller here supplies one.
package gettext

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Internationalization and localization support.

This module provides internationalization (I18N) and localization (L10N)
support for your Python programs by providing an interface to the GNU gettext
message catalog library.`

// Translations is the catalog object.  With no catalog loaded it returns the
// message unchanged, which is gettext's defined behaviour when there is no
// translation available.
type Translations struct {
	info map[string]string
}

var TranslationsType = py.NewTypeX("gettext.NullTranslations", "A translations object with no catalog.", func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	t := &Translations{info: map[string]string{
		"language":     "",
		"plural-forms": "nplurals=2; plural=(n != 1);",
	}}
	if len(args) > 0 {
		if d, ok := args[0].(py.IGetDict); ok {
			for _, __e := range d.GetDict().Items() {
				encoded := __e.Key
				v := __e.Value
				key, err := py.DictKeyDecode(encoded)
				if err != nil {
					continue
				}
				keyText, err := py.StrAsString(key)
				if err != nil {
					continue
				}
				if text, err := py.StrAsString(v); err == nil {
					t.info[keyText] = text
				}
			}
		}
	}
	return t, nil
}, nil)

func (t *Translations) Type() *py.Type { return TranslationsType }

// GNUTranslations is the same object here: the .mo format is not read.
var GNUTranslationsType = TranslationsType

func init() {
	gettextFn := py.MustNewMethod("gettext", doGettext, 0, "Return the localized translation of message.")
	ngettextFn := py.MustNewMethod("ngettext", doNgettext, 0, "Return the plural form of a message.")
	textdomainFn := py.MustNewMethod("textdomain", setName, 0, "Set the current global domain.")
	bindtextdomainFn := py.MustNewMethod("bindtextdomain", setTwoNames, 0, "Bind the domain to a directory.")
	dgettextFn := py.MustNewMethod("dgettext", doGettext, 0, "Return the translation of message in the given domain.")
	dngettextFn := py.MustNewMethod("dngettext", doNgettext, 0, "Return the plural form of a message in the given domain.")
	pgettextFn := py.MustNewMethod("pgettext", doPGettext, 0, "Return the translation for a context and message.")
	dpgettextFn := py.MustNewMethod("dpgettext", doPGettext, 0, "Return the translation for a context and message in a domain.")
	npgettextFn := py.MustNewMethod("npgettext", doNPgettext, 0, "Return the plural form for a context and message.")

	// The plural form: the "plural-forms" expression is evaluated in
	// CPython; here the standard Germanic rule (n != 1) applies, which is
	// what an English source catalog uses.
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "gettext", Value: gettextFn},
		py.DictEntry{Key: "ngettext", Value: ngettextFn},
		py.DictEntry{Key: "dgettext", Value: dgettextFn},
		py.DictEntry{Key: "dngettext", Value: dngettextFn},
		py.DictEntry{Key: "pgettext", Value: pgettextFn},
		py.DictEntry{Key: "dpgettext", Value: dpgettextFn},
		py.DictEntry{Key: "npgettext", Value: npgettextFn},
		py.DictEntry{Key: "textdomain", Value: textdomainFn},
		py.DictEntry{Key: "bindtextdomain", Value: bindtextdomainFn},
		py.DictEntry{Key: "bind_textdomain_codeset", Value: py.MustNewMethod("bind_textdomain_codeset", setTwoNames, 0, "Set the encoding of the catalog.")},
		py.DictEntry{Key: "translation", Value: py.MustNewMethod("translation", translation, 0, "Return a Translations object.")},
		py.DictEntry{Key: "install", Value: py.MustNewMethod("install", install, 0, "Install _() into the builtins namespace.")},
		py.DictEntry{Key: "NullTranslations", Value: TranslationsType},
		py.DictEntry{Key: "GNUTranslations", Value: GNUTranslationsType},
		py.DictEntry{Key: "Translations", Value: TranslationsType},
	)

	// The translation object's methods.
	for _, t := range []*py.Type{TranslationsType} {
		t.Dict.Set("gettext", gettextMethod)
		t.Dict.Set("ngettext", ngettextMethod)
		t.Dict.Set("pgettext", gettextMethod)
		t.Dict.Set("npgettext", ngettextMethod)
		t.Dict.Set("info", &py.Property{Fget: func(self py.Object) (py.Object, error) {
			d := py.NewStringDict()
			for k, v := range self.(*Translations).info {
				d.Set(k, py.String(v))
			}
			return d, nil
		}})
		t.Dict.Set("charset", py.MustNewMethod("charset", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.String("utf-8"), nil
		}, 0, "Return the encoding of the catalog."))
		t.Dict.Set("install", py.MustNewMethod("install", installSelf, 0, "Install _() into the builtins namespace."))
		t.Dict.Set("add_fallback", py.MustNewMethod("add_fallback", noopSelf, 0, "Add a fallback catalog (no-op)."))
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "gettext",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// gettextMethod and ngettextMethod are the catalog's own methods.
var gettextMethod = py.MustNewMethod("gettext", func(self py.Object, args py.Tuple) (py.Object, error) {
	return doGettext(self, args)
}, 0, "Return the localized translation of message.")

var ngettextMethod = py.MustNewMethod("ngettext", func(self py.Object, args py.Tuple) (py.Object, error) {
	return doNgettext(self, args)
}, 0, "Return the plural form of a message.")

// gettext returns the message unchanged: with no catalog there is nothing to
// translate it to.
func doGettext(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) == 0 {
		return nil, py.ExceptionNewf(py.TypeError, "gettext() missing required argument 'message'")
	}
	message := args[len(args)-1]
	if _, ok := message.(py.String); !ok {
		return nil, py.ExceptionNewf(py.TypeError, "gettext() argument must be a string")
	}
	return message, nil
}

// gettextPlural accepts the (context, message) form and returns the message.
func doPGettext(self py.Object, args py.Tuple) (py.Object, error) {
	return doGettext(self, args)
}

// ngettext picks the singular or plural form by count.
func doNgettext(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) < 3 {
		return nil, py.ExceptionNewf(py.TypeError, "ngettext() needs singular, plural and n")
	}
	singular := args[len(args)-3]
	plural := args[len(args)-2]
	count := args[len(args)-1]
	n, err := py.IndexInt(count)
	if err != nil {
		return nil, err
	}
	// The Germanic plural rule, which is the one an English catalog uses.
	if n == 1 {
		return singular, nil
	}
	return plural, nil
}

// npgettext accepts the (context, singular, plural, n) form.
func doNPgettext(self py.Object, args py.Tuple) (py.Object, error) {
	return doNgettext(self, args)
}

func setName(self py.Object, args py.Tuple) (py.Object, error) {
	var name py.Object = py.String("messages")
	if err := py.UnpackTuple(args, py.StringDict{}, "textdomain", 0, 1, &name); err != nil {
		return nil, err
	}
	return name, nil
}

func setTwoNames(self py.Object, args py.Tuple) (py.Object, error) {
	if len(args) == 0 {
		return py.String(""), nil
	}
	return args[len(args)-1], nil
}

func translation(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	// No catalog is read, so the result is the null translation, which is
	// what CPython returns when it finds no .mo file.
	t, err := TranslationsType.New(TranslationsType, py.Tuple{}, py.NewStringDict())
	if err != nil {
		return nil, err
	}
	return t, nil
}

// install puts _ and its friends into the builtins namespace, which is what
// gettext.install() is for.
func install(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return installSelf(self, args)
}

func installSelf(self py.Object, args py.Tuple) (py.Object, error) {
	builtins := py.GetModuleImplOrNil("builtins")
	if builtins == nil {
		return py.None, nil
	}
	builtins.Globals.Set("_", py.MustNewMethod("_", doGettext, 0, "Return the localized translation of message."))
	builtins.Globals.Set("gettext", builtins.Globals.GetOrNil("_"))
	builtins.Globals.Set("ngettext", py.MustNewMethod("ngettext", doNgettext, 0, "Return the plural form of a message."))
	return py.None, nil
}

func noopSelf(self py.Object, args py.Tuple) (py.Object, error) { return py.None, nil }

// keep strings referenced: the catalog's plural expression parsing would use
// it, and trimming is what the null translation does to a message.
var _ = strings.TrimSpace
