import reprlib

# A long list is truncated.
assert reprlib.repr(list(range(100))) == "[0, 1, 2, 3, 4, 5, ...]", reprlib.repr(list(range(100)))
# A long string keeps its head and tail.
assert reprlib.repr("x" * 100) == "'xxxxxxxxxxxx...xxxxxxxxxxxxx'", reprlib.repr("x" * 100)
# A self-referential list terminates.
l = []
l.append(l)
assert reprlib.repr(l) == "[[[[[[[...]]]]]]]", reprlib.repr(l)

# The default limits.
r = reprlib.Repr()
assert r.maxlist == 6
assert r.maxtuple == 6
assert r.maxdict == 4
assert r.maxset == 6
assert r.maxfrozenset == 6
assert r.maxstring == 30
assert r.maxlong == 40
assert r.maxother == 30
assert r.maxlevel == 6
assert r.indent is None

# Tuples, dicts, sets and frozensets.
assert reprlib.repr((1, 2, 3) * 20) == "(1, 2, 3, 1, 2, 3, ...)", reprlib.repr((1, 2, 3) * 20)
assert reprlib.repr({i: i for i in range(20)}) == "{0: 0, 1: 1, 2: 2, 3: 3, ...}"
assert reprlib.repr(set(range(20))) == "{0, 1, 2, 3, 4, 5, ...}"
assert reprlib.repr(frozenset(range(20))) == "frozenset({0, 1, 2, 3, 4, 5, ...})"

# Small values are exact, and empty containers keep their literal.
assert reprlib.repr((1,)) == "(1,)"
assert reprlib.repr(set()) == "set()"
assert reprlib.repr(frozenset()) == "frozenset()"
assert reprlib.repr({}) == "{}"
assert reprlib.repr(True) == "True"
assert reprlib.repr(None) == "None"
assert reprlib.repr(1.5) == "1.5"

# repr1 with an explicit level.
assert reprlib.Repr().repr1("abcdefghij" * 5, 0) == "'abcdefghijab...hijabcdefghij'"

# A long integer is truncated around the middle.
assert reprlib.repr(12345678901234567890123456789012345678901234567890) == "123456789012345678...2345678901234567890"

# Limit and fillvalue are settable.
r2 = reprlib.Repr()
r2.maxlist = 2
assert r2.repr([1, 2, 3]) == "[1, 2, ...]"
r5 = reprlib.Repr()
r5.fillvalue = "<<>>"
assert r5.repr(list(range(10))) == "[0, 1, 2, 3, 4, 5, <<>>]"

# indent turns on the multi-line form.
r3 = reprlib.Repr()
r3.indent = "  "
assert r3.repr([1, 2, 3]) == "[\n  1,\n  2,\n  3,\n]"
r4 = reprlib.Repr(indent=2)
assert repr(r4.repr([1, 2, 3])) == "'[\\n  1,\\n  2,\\n  3,\\n]'"

# Nested recursion is bounded by maxlevel.
assert reprlib.repr([1, 2, [3, [4]]]) == "[1, 2, [3, [4]]]"
r6 = reprlib.Repr(maxlevel=2)
assert r6.repr([1, [2, [3, [4]]]]) == "[1, [2, [...]]]"

print("reprlib ok")
