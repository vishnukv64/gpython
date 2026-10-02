"""gettext: internationalising user-facing strings.

Run with:  /tmp/gpy examples/stdlib/gettext_demo.py

gettext looks up a translation catalogue for a message id and falls back to the
id itself when no catalogue is installed. NullTranslations is the identity
implementation; translation() returns one when the catalogue is missing and
fallback=True.

Interpreter note: translation() returns a NullTranslations instance (never a
GNUTranslations) because .mo catalogue parsing is not implemented. Every lookup
therefore falls through to the original English string, which is what the
fallback path in CPython does too.
"""

import gettext

print("--- the identity catalogue: NullTranslations ---")
catalogue = gettext.NullTranslations()
print("type:      ", type(catalogue).__name__)
print("gettext:   ", repr(catalogue.gettext("Hello")))
print("ngettext(1):", repr(catalogue.ngettext("one file", "many files", 1)))
print("ngettext(5):", repr(catalogue.ngettext("one file", "many files", 5)))

print()
print("--- module-level shortcuts ---")
print("gettext:      ", repr(gettext.gettext("Save")))
print("ngettext(0):  ", repr(gettext.ngettext("item", "items", 0)))
print("ngettext(1):  ", repr(gettext.ngettext("item", "items", 1)))
print("ngettext(2):  ", repr(gettext.ngettext("item", "items", 2)))
print("dgettext:     ", repr(gettext.dgettext("messages", "Open")))
print("pgettext:     ", repr(gettext.pgettext("toolbar", "Open")))
print("dpgettext:    ", repr(gettext.dpgettext("messages", "toolbar", "Open")))
print("npgettext:    ", repr(gettext.npgettext("toolbar", "tab", "tabs", 3)))

print()
print("--- translation() with fallback=True never fails ---")
catalogue = gettext.translation("definitely-not-installed", languages=["zz"], fallback=True)
print("returned:  ", type(catalogue).__name__)
print("a lookup:  ", repr(catalogue.gettext("Fallback text")))
print("isinstance of NullTranslations:", isinstance(catalogue, gettext.NullTranslations))

print()
print("--- the class hierarchy ---")
print("NullTranslations is the base:", gettext.NullTranslations is gettext.Translations)
print("Translations alias present:", hasattr(gettext, "Translations"))
print("GNUTranslations present:  ", hasattr(gettext, "GNUTranslations"))

print()
print("--- a worked example: a tiny i18n layer that degrades gracefully ---")


def make_translator(domain="demo", language="zz"):
    try:
        return gettext.translation(domain, languages=[language], fallback=True)
    except Exception as err:
        print("  catalogue lookup failed (%s), using the identity catalogue" % (type(err).__name__,))
        return gettext.NullTranslations()


class Messages:
    def __init__(self, domain, language):
        self.translator = make_translator(domain, language)
        self.domain = domain
        self.language = language

    def _(self, text):
        return self.translator.gettext(text)

    def n(self, singular, plural, count):
        return self.translator.ngettext(singular, plural, count)

    def report(self, files, errors):
        lines = []
        lines.append(self._("Report for %s") % (self.domain,))
        lines.append("  " + self.n("%d file scanned", "%d files scanned", files) % (files,))
        lines.append("  " + self.n("%d error found", "%d errors found", errors) % (errors,))
        return lines


for language in ["zz", "en"]:
    messages = Messages("demo", language)
    print("language=%s -> %s" % (language, type(messages.translator).__name__))
    for line in messages.report(7, 1):
        print(line)

print()
print("--- a catalogue without a .mo file is an identity mapping ---")
print("so every translated string equals its message id.")
print("install() and textdomain() are callable:", callable(gettext.install), callable(gettext.textdomain))
print("bindtextdomain() is callable:", callable(gettext.bindtextdomain))
