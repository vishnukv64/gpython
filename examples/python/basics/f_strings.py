"""f-strings: formatted string literals (PEP 498).

Run with:  /tmp/gpy examples/python/basics/f_strings.py

Supported here: expressions, attribute access, calls, indexing, conversion
flags !r/!s, nested quotes via different quote characters, and a good range
of format specs (fill/align/width, .precision, integer radix and zero-pad).

NOT supported: the sign flag (`{n:+d}` raises
ValueError: Invalid format specifier "+d"), and reusing the SAME quote
character inside the braces (`f"{d["k"]}"` is a syntax error).
"""

name = "Ada"
age = 36
pi = 3.14159
count = 7

print("--- the basics ---")
print(f"hello {name}")
print(f"{name} is {age} years old")

print()
print("--- expressions inside the braces ---")
print(f"next year: {age + 1}")
print(f"squared:   {age * age}")
print(f"average:   {(10 + 20 + 30) / 3}")
print(f"max:       {max(3, 9, 4)}")
print(f"truthy:    {not not name}")

print()
print("--- calling methods and functions ---")
print(f"upper:     {name.upper()}")
print(f"lower:     {name.lower()}")
print(f"length:    {len(name)}")
print(f"joined:    {'-'.join([name, 'B'])}")

print()
print("--- attribute access and indexing ---")
print(f"first char: {name[0]}")
print(f"last char:  {name[-1]}")
print(f"slice:      {name[0:2]}")

# Nested quotes: use differ&ent quote characters, or chr() for the same one.
mapping = {"lang": "Python", "year": 1991}
print(f"dict value: {mapping['lang']}")
print(f"dict again: {mapping[chr(121) + chr(101) + chr(97) + chr(114)]}")

class Record:
    pass

rec = Record()
rec.title = "Dune"
rec.year = 1965
print(f"attribute:  {rec.title} ({rec.year})")

print()
print("--- conversion flags !r and !s ---")
text = "quoted"
print(f"!r: {text!r}")
print(f"!s: {text!s}")
print(f"default: {text}")
print(f"repr of a number: {pi!r}")

print()
print("--- format specs: strings ---")
print(f"right:     [{name:>10}]")
print(f"left:      [{name:<10}]")
print(f"center:    [{name:^10}]")
print(f"fill:      [{name:*>10}]")
print(f"fill all:  [{name:*^11}]")
# `.precision` on a string is IGNORED here (CPython truncates to 2 chars).
print(f"truncate:  [{name:.2}]  <- .precision on a string is ignored")

print()
print("--- format specs: integers ---")
print(f"plain:     {count}")
print(f"zero pad:  {count:05d}")
print(f"width:     [{count:6}]")
print(f"right:     [{count:>6}]")
print(f"hex:       {255:x}")
print(f"HEX:       {255:X}")
print(f"octal:     {255:o}")
print(f"binary:    {255:b}")
print(f"plain hex: {255:x}")

print()
print("--- format specs: floats ---")
print(f"fixed 2dp: {pi:.2f}")
print(f"fixed 4dp: {pi:.4f}")
print(f"width+prec:[{pi:10.3f}]")
print(f"left+prec: [{pi:<10.2f}]")
print(f"percent:   {0.256:.1%}")
print(f"exponent:  {12345.678:.2e}")

# The sign flag '+' is not implemented and raises inside the f-string
# formatter, so a sign is added by hand when needed.
print("sign by hand:", ("+" if age >= 0 else "-") + str(abs(age)))
try:
    print(f"{age:+d}")
except ValueError as err:
    print("the '+' flag raises:", err)

print()
print("--- nested f-strings and expressions spanning calls ---")
inner = "deep"
print(f"nested: {f'inner is {inner}'}")
print(f"conditional: {'adult' if age >= 18 else 'minor'}")

print()
print("--- multi-line f-strings ---")


def summary(title, items):
    return (
        f"Report: {title}\n"
        f"  items:  {len(items)}\n"
        f"  first:  {items[0]}\n"
        f"  total:  {sum(items)}\n"
    )


print(summary("numbers", [4, 8, 15, 16, 23, 42]))

print("--- raw f-strings work too ---")
print(fr"a raw path {name}\n has a literal backslash-n, not a newline")
print(rf"the other prefix order works as well: {age}")

print()
print("--- building a table ---")
rows = [("apples", 3, 1.25), ("oranges", 12, 0.75), ("pears", 5, 2.10)]
total = 0.0
for item, qty, price in rows:
    print(f"  {item:<9} {qty:>3} x {price:>5.2f} = {qty * price:>7.2f}")
    total += qty * price
print(f"  {'TOTAL':<9} {'':>3}   {'':>5}   {total:>7.2f}")

print()
print("--- an f-string in a comprehension ---")
squares = [f"{n}^2={n * n}" for n in range(1, 6)]
print(f"result: {', '.join(squares)}")

print()
print("--- debugging: the expression itself in the label ---")
# (f"{x=}" debug syntax is not supported; spell the label out.)
width, height = 4, 7
print(f"width = {width}, height = {height}, area = {width * height}")
