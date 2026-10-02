"""Core scalar types: int, float, bool, None, and how they convert.

Run with:  /tmp/gpy examples/python/basics/types.py

Note on this interpreter: the `bool()` constructor is not implemented, so
truthiness is demonstrated with `not not x` instead of `bool(x)`.
"""

print("--- integers ---")
print("literal:           ", 42)
print("negative:          ", -7)
print("hex/octal/binary:  ", 0xFF, 0o17, 0b1010)
print("underscore literal:", 1_000_000)          # PEP 515 separators
print("arbitrary precision:", 2 ** 100)
print("floor division:    ", 17 // 5)             # -> 3
print("negative floor:    ", -17 // 5)            # -> -4, floors toward -inf
print("modulo:            ", 17 % 5, -17 % 5)
print("divmod:            ", divmod(17, 5))
print("power:             ", 2 ** 10, pow(2, 10), pow(2, 10, 1000))
print("absolute value:    ", abs(-3), abs(-3.5))

print()
print("--- floats ---")
print("literal:           ", 3.5)
print("true division:     ", 7 / 2)               # always float
print("binary rounding:   ", 0.1 + 0.2)           # not exactly 0.3
print("one third:         ", 1 / 3)
print("scientific:        ", 1.5e3)
print("special values:    ", float("inf"), float("nan"))
print("inf compares big:  ", float("inf") > 1e300)
print("nan never equals:  ", float("nan") == float("nan"))
print("mixed int/float eq:", 1.0 == 1)

print()
print("--- booleans ---")
# True and False are real objects even though bool() cannot be called here.
print("literals:          ", True, False)
print("arithmetic:        ", True + 1, True * 3)   # bool is a kind of int
print("equality to ints:  ", True == 1, False == 0)
print("logical operators: ", 1 and 2, 0 and 2, 1 or 2, 0 or 2)
print("negation:          ", not True, not 0)

print()
print("--- truthiness (via not not, since bool() is unimplemented) ---")
for label, value in [("0", 0), ("1", 1), ("0.0", 0.0), ("''", ""), ("'x'", "x"),
                     ("[]", []), ("[1]", [1]), ("None", None)]:
    print("%-6s truthy -> %s" % (label, not not value))

print()
print("--- None ---")
print("the None value:    ", None)
print("its type:          ", type(None))
print("identity check:    ", None is None, [] is None)

print()
print("--- conversions ---")
print("int('42'):         ", int("42"))
print("int('ff', 16):     ", int("ff", 16))
print("int('101', 2):     ", int("101", 2))
print("int(3.9):          ", int(3.9))            # truncates toward zero
print("int(-3.9):         ", int(-3.9))
print("float('1.5'):      ", float("1.5"))
print("float(3):          ", float(3))
print("str(42):           ", str(42))
print("str(1.0):          ", str(1.0))
print("str(True):         ", str(True))
print("str(None):         ", str(None))
print("repr vs str:       ", repr("a"), str("a"))

print()
print("--- formatting scalars ---")
# The str.format() method is not implemented here, so the % operator and the
# builtin format() function are the two supported formatting routes.
print("percent verbs:     ", "%s|%d|%.2f|%r" % ("x", 1, 1.5, "q"))
print("percent width:     ", "[%-6s][%6s]" % ("a", "b"))
print("percent zero-pad:  ", "%05d %x %o" % (42, 255, 8))
print("format() builtin:  ", format(3.14159, ".3f"), format(255, "x"), format("a", ">4"))
print("str() of a float:  ", str(round(2.567)))
