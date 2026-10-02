"""Tests for the textwrap module.

Every expected value here was computed with CPython 3.14's textwrap, so these
are exact-match tests against the reference implementation, including the
word-separator rules for hyphens and em-dashes.
"""

import textwrap

def assertEqual(x, y):
    assert x == y, "got: %s, want: %s" % (repr(x), repr(y))

# wrap and fill
assertEqual(textwrap.wrap("The quick brown fox jumps over the lazy dog.", 20),
            ["The quick brown fox", "jumps over the lazy", "dog."])
assertEqual(textwrap.fill("The quick brown fox jumps over the lazy dog.", 20),
            "The quick brown fox\njumps over the lazy\ndog.")
assertEqual(textwrap.wrap("", 10), [])

# a long word is broken unless break_long_words is off
assertEqual(textwrap.wrap("Hello", 3), ["Hel", "lo"])
assertEqual(textwrap.wrap("supercalifragilisticexpialidocious", 10, break_long_words=False),
            ["supercalifragilisticexpialidocious"])

# hyphens break after the hyphen, but not when break_on_hyphens is off
assertEqual(textwrap.wrap("aaa-bbb-ccc-ddd", 5), ["aaa-", "bbb-", "ccc-", "ddd"])
assertEqual(textwrap.wrap("aaa-bbb-ccc-ddd", 5, break_on_hyphens=False),
            ["aaa-b", "bb-cc", "c-ddd"])
assertEqual(textwrap.wrap("Look, goof-ball -- use the -b option!", 15),
            ["Look, goof-ball", "-- use the -b", "option!"])

# indents consume width
assertEqual(textwrap.wrap("hello world", 20, initial_indent=">> ", subsequent_indent="   "),
            [">> hello world"])
assertEqual(textwrap.fill("hello world foo", 8, initial_indent=">> ", subsequent_indent="   "),
            ">> hello\n   world\n   foo")
assertEqual(textwrap.fill("aaa bbb ccc", 7, subsequent_indent="  "), "aaa bbb\n  ccc")

# whitespace handling
assertEqual(textwrap.wrap("   hello   world   ", 10), ["   hello", "world"])
assertEqual(textwrap.wrap("hello\tworld", 10, expand_tabs=True), ["hello", "world"])
assertEqual(textwrap.wrap("hello", 10, drop_whitespace=False), ["hello"])

# max_lines and placeholder
assertEqual(textwrap.wrap("a b c d e f", 5, max_lines=2), ["a b c", "d e f"])
assertEqual(textwrap.wrap("a b c d e f", 5, max_lines=2, placeholder=" [..]"), ["a b c", "d e f"])

# fill keeps a sentence's double space when fix_sentence_endings is set
assertEqual(textwrap.wrap("a. b c. d", 4, fix_sentence_endings=True), ["a.", "b c.", "d"])

# dedent
assertEqual(textwrap.dedent("\n    hello\n    world\n    "), "\nhello\nworld\n")
assertEqual(textwrap.dedent("  a\n    b\n"), "a\n  b\n")
assertEqual(textwrap.dedent("  a\n\n    b\n"), "a\n\n  b\n")

# indent
assertEqual(textwrap.indent("a\nb\nc\n", "  "), "  a\n  b\n  c\n")
assertEqual(textwrap.indent("a\n\nb\n", "> ", lambda l: l.strip() != ""), "> a\n\n> b\n")

# shorten
assertEqual(textwrap.shorten("The quick brown fox jumps over the lazy dog.", 30),
            "The quick brown fox [...]")
assertEqual(textwrap.shorten("The quick brown fox", 20, placeholder=" ..."), "The quick brown fox")
assertEqual(textwrap.shorten("Hello World", 5), "[...]")

# the TextWrapper class
w = textwrap.TextWrapper(20)
assertEqual(w.width, 20)
assertEqual(w.wrap("The quick brown fox jumps over the lazy dog."),
            ["The quick brown fox", "jumps over the lazy", "dog."])
assertEqual(w.fill("The quick brown fox jumps over the lazy dog."),
            "The quick brown fox\njumps over the lazy\ndog.")
for attr in ("width", "initial_indent", "subsequent_indent", "expand_tabs",
             "replace_whitespace", "drop_whitespace", "break_long_words",
             "break_on_hyphens", "max_lines", "placeholder"):
    assert hasattr(w, attr), "TextWrapper has no %s" % attr
# attributes are writable, as in CPython
w.initial_indent = ">> "
assertEqual(w.initial_indent, ">> ")
w.max_lines = None
assertEqual(w.max_lines, None)
w.max_lines = 3
assertEqual(w.max_lines, 3)
assertEqual(textwrap.TextWrapper(10, subsequent_indent="  ").subsequent_indent, "  ")

# errors
try:
    textwrap.TextWrapper(0).wrap("x")
    raise AssertionError("did not raise ValueError for width 0")
except ValueError:
    pass

print("textwrap: all tests pass")
