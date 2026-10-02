# string.Template, matching CPython's substitution and error messages.
import string

T = string.Template

assert T("$a-$b").substitute(a=1, b=2) == "1-2"
assert T("$a-$b").substitute({"a": 1}, b=2) == "1-2"
assert T("${a}:$a").substitute(a="x") == "x:x"
assert T("$$").substitute() == "$"
assert T("a$").safe_substitute() == "a$"

assert T("$t").template == "$t"

assert T("$a ${b} $a $$ $").get_identifiers() == ["a", "b"]
assert T("$a ${b} $$").is_valid() is True
assert T("$").is_valid() is False
assert T("${").is_valid() is False

assert T.delimiter == "$"
assert T.braceidpattern is None
assert T.idpattern == "(?a:[_a-z][_a-z0-9]*)"

# A missing key raises KeyError naming it.
try:
    T("$x").substitute()
    raise AssertionError("expected KeyError")
except KeyError as e:
    assert str(e) == "'x'", str(e)

# An invalid placeholder raises ValueError, with CPython's line/col text.
try:
    T("x$ y").substitute()
    raise AssertionError("expected ValueError")
except ValueError as e:
    assert str(e) == "Invalid placeholder in string: line 1, col 2", str(e)

try:
    T("ab\ncd$ x").substitute()
    raise AssertionError("expected ValueError")
except ValueError as e:
    assert str(e) == "Invalid placeholder in string: line 2, col 3", str(e)

# safe_substitute leaves a bad placeholder and a missing key alone.
assert T("x$ y $a").safe_substitute(a=1) == "x$ y 1"
assert T("$a$b").safe_substitute(a=1) == "1$b"

print("OK")
