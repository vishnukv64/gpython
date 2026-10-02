"""string: text constants and simple text helpers.

Run with:  /tmp/gpy examples/stdlib/string_demo.py

This module is mostly a bag of useful string constants. The one function,
capwords(), capitalises the words in a string.

Interpreter note: string.Template is NOT present, so $-substitution by hand is
shown instead -- see examples/KNOWN_ISSUES.md.
"""

import string

print("--- the alphabets ---")
print("ascii_lowercase:", string.ascii_lowercase)
print("ascii_uppercase:", string.ascii_uppercase)
print("ascii_letters:  ", string.ascii_letters)
print("digits:         ", string.digits)
print("hexdigits:      ", string.hexdigits)
print("octdigits:      ", string.octdigits)

print()
print("--- whitespace and punctuation ---")
print("whitespace:  ", repr(string.whitespace))
print("punctuation: ", string.punctuation)
print("printable length:  ", len(string.printable))
print("printable is letters + digits + punctuation + whitespace:",
      len(string.printable) ==
      len(string.ascii_letters) + len(string.digits) + len(string.punctuation) + len(string.whitespace))

print()
print("--- capwords() capitalises every word ---")
for text in ["hello world", "the QUICK brown fox", "a b c", "already Good"]:
    print("  %-22r -> %r" % (text, string.capwords(text)))
print("capwords() is the module's own helper; the equivalent long hand is:")
sample = "one two"
print("  capwords:                     ", repr(string.capwords(sample)))
print("  first char upper + rest lower:", repr(" ".join([word[0].upper() + word[1:].lower()
                                                    for word in sample.split()])))
print("  (str.capitalize() does not exist in this interpreter, so the")
print("   equivalent is written out by hand)")

print()
print("--- the constants are plain strings, so all str operations apply ---")
print("digits is a str:      ", isinstance(string.digits, str))
print("digits[3]:            ", string.digits[3])
print("digits reversed:      ", string.digits[::-1])

print()
print("--- a worked example: validation helpers ---")


def is_identifier(text):
    """Roughly, a valid Python identifier: letters, digits, underscores."""
    if not text:
        return False
    first, rest = text[0], text[1:]
    if first not in string.ascii_letters + "_":
        return False
    for character in rest:
        if character not in string.ascii_letters + string.digits + "_":
            return False
    return True


def is_int(text):
    """True when every character is a digit (after an optional sign)."""
    if text.startswith("-") or text.startswith("+"):
        text = text[1:]
    if not text:
        return False
    for character in text:
        if character not in string.digits:
            return False
    return True


def has_punctuation(text):
    return any([character in string.punctuation for character in text])


for candidate in ["name", "_private", "with space", "3lives", "snake_case_42"]:
    print("  is_identifier(%-16r) = %s" % (candidate, is_identifier(candidate)))
print()
for candidate in ["123", "-42", "4.5", "007", ""]:
    print("  is_int(%-6r) = %s" % (candidate, is_int(candidate)))
print()
for candidate in ["plain text", "has,comma", "a-b"]:
    print("  has_punctuation(%-12r) = %s" % (candidate, has_punctuation(candidate)))

print()
print("--- substituting with $ placeholders, since Template is absent ---")


def fill(template, values):
    """A very small $name substituter, standing in for string.Template."""
    result = template
    for key in values:
        result = result.replace("$" + key, str(values[key]))
    return result


TEMPLATE = "Dear $name, your order $order shipped on $date."
values = {"name": "ann", "order": "A-1001", "date": "2024-01-15"}
print("template:", TEMPLATE)
print("filled:  ", fill(TEMPLATE, values))
print("left placeholders after filling only some:")
print("  ", fill(TEMPLATE, {"name": "bob"}))

print()
print("--- what is not here ---")
for name in ["Template", "Formatter", "ascii_lowercase", "capwords"]:
    print("  string.%-16s %s" % (name, hasattr(string, name)))
