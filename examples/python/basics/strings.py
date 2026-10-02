"""Strings: indexing, slicing, the supported methods, and formatting.

Run with:  /tmp/gpy examples/python/basics/strings.py

Which string methods exist in this interpreter
------------------------------------------------
Implemented: count, endswith, find, join, lower, lstrip, replace, rstrip,
             split, startswith, strip, upper
Missing (AttributeError): format, format_map, zfill, ljust, rjust, center,
             partition, rpartition, splitlines, index, rindex, title,
             capitalize, swapcase, isdigit/isalpha/isalnum, encode, translate,
             expandtabs, and the builtin str.maketrans.
This file only uses methods that actually exist.
"""

print("--- literals and escapes ---")
print("single:      ", 'plain')
print("double:      ", "plain")
print("triple-quoted:", repr("""line1
line2"""))
print("escape seqs: ", "tab[\t] newline[\\n] quote[\"] backslash[\\]")
print("raw string:  ", r"a\nb is not a newline")
print("unicode:     ", "caf\u00e9", "\u2603", "snowman length:", len("\u2603"))
print("concatenation of adjacent literals:", "abc" "def")

print()
print("--- indexing and slicing ---")
s = "Python"
print("string:      ", s)
print("s[0], s[-1]: ", s[0], s[-1])
print("s[1:4]:      ", s[1:4])
print("s[:3]:       ", s[:3])
print("s[3:]:       ", s[3:])
print("s[::2]:      ", s[::2])
print("s[::-1]:     ", s[::-1])
print("out of range: ", s[0:100])       # slices clamp, they do not raise

print()
print("--- the implemented methods ---")
t = "  Hello, World  "
print("strip:       ", "[" + t.strip() + "]")
print("lstrip:      ", "[" + t.lstrip() + "]")
print("rstrip:      ", "[" + t.rstrip() + "]")
print("lstrip('H'): ", "[" + "Hello".lstrip("H") + "]")
print("upper/lower: ", s.upper(), s.lower())
print("replace:     ", "banana".replace("a", "A"), "banana".replace("a", "A", 1))
print("find:        ", "banana".find("na"), "banana".find("zz"))
print("count:       ", "banana".count("a"), "banana".count("zz"))
print("split(sep):  ", "a,b,,c".split(","))
print("split():     ", "a  b   c".split())          # whitespace runs collapse
print("split(sep,n):", "a,b,c".split(",", 1))
print("startswith:  ", "abc".startswith("ab"), "abc".startswith(("x", "a")))
print("endswith:    ", "abc".endswith("bc"), "abc".endswith(("x", "c")))
print("join:        ", ",".join(["a", "b", "c"]))
print("join ints:   ", ", ".join(str(i) for i in range(4)))

print()
print("--- operators ---")
print("in / not in: ", "ell" in "hello", "z" in "hello", "z" not in "hello")
print("repeat:      ", "ab" * 3)
print("concat:      ", "a" + "b")
print("compare:     ", "a" < "b", "abc" == "abc", "A" < "a")
print("len:         ", len(s))
print("iteration:   ", [c for c in "abc"], list("abc"))

print()
print("--- repr vs str ---")
print("str:         ", "a\nb")
print("repr:        ", repr("a\nb"), repr("it's"), repr('say "hi"'))

print()
print("--- percent formatting (str.format() is NOT implemented) ---")
print("%s %d %.3f %x" % ("text", 42, 3.14159, 255))
print("width:  [%6s] [%-6s]" % ("ab", "ab"))
print("zero/digits: %05d %+d" % (42, 42))
print("%r and %s, %r and %s" % ("a", "a", "b", "b"))
print("literal percent: 100%%" % ())

print()
print("--- f-strings (supported) ---")
name = "World"
count = 3
pi = 3.14159
print("plain:       ", f"Hello, {name}!")
print("expression:  ", f"{count} squared is {count * count}")
print("call:        ", f"{name.upper()} has {len(name)} letters")
print("indexing:    ", f"{name[0]}...{name[-1]}")
print("conversion:  ", f"{name!r} vs {name!s}")
print("float spec:  ", f"{pi:.2f} | {pi:10.3f} | {pi:<10.2f}")
# Note: the '+d' sign flag is not implemented in this interpreter's f-string
# formatter (ValueError: Invalid format specifier "+d"); '05d' and '>6' are.
print("int spec:    ", f"{count:05d} | {count:>6}")
print("radix spec:  ", f"{255:x} | {255:o} | {255:b}")
print("str spec:    ", f"[{name:>10}][{name:*^11}][{name:<10}]")
notes = {"lang": "Python"}
print("dict lookup: ", f"lang = {notes['lang']}")

print()
print("--- the missing methods, demonstrated honestly ---")
for meth, call in [("format", "\"{}\".format(1)"),
                   ("zfill", "\"1\".zfill(3)"),
                   ("title", "\"a b\".title()"),
                   ("splitlines", "\"a\\nb\".splitlines()"),
                   ("index", "\"abc\".index(\"b\")")]:
    try:
        eval(compile(call, "<probe>", "eval"))
        print("%-12s unexpectedly works" % meth)
    except AttributeError:
        print("%-12s AttributeError (not implemented)" % meth)
