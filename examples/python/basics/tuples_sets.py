"""Tuples and sets: immutable sequences and unordered unique collections.

Run with:  /tmp/gpy examples/python/basics/tuples_sets.py

Notes on this interpreter:
  * tuple.count() and tuple.index() are not implemented.
  * Tuple ordering comparisons (a < b) are not implemented; == works.
  * frozenset cannot be constructed ("cannot create 'frozenset' instances").
  * Set methods (union, intersection, issubset, ...) are not implemented; use
    the operators | & - ^ and the builtin set() instead. add() does exist.
"""

print("=== TUPLES ===")
print()
print("--- creating tuples ---")
print("literal:       ", (1, 2, 3))
print("parenthesised: ", ("a", "b"))
print("without parens:", 1, 2, 3,
      "-> the repr is", (1, 2, 3))
print("singleton:     ", (42,), "length", len((42,)))
# NOTE: this interpreter reprs the singleton tuple (42,) as "(42)" -- the
# trailing comma that distinguishes a 1-tuple from a plain int is dropped.
print("singleton repr should be (42,); this interpreter prints:", repr((42,)))
print("empty:         ", (), "length", len(()))
print("mixed:         ", (1, "two", 3.5, None))
print("nested:        ", (1, (2, 3), (4, (5, 6))))
print("from list:     ", tuple([1, 2, 3]))
print("from string:   ", tuple("abc"))
print("from range:    ", tuple(range(4)))
print("concatenate:   ", (1, 2) + (3, 4))
print("repeat:        ", ("x",) * 3)
print("tuple comp:    ", tuple(x * x for x in range(4)))

print()
print("--- indexing and slicing ---")
t = ("a", "b", "c", "d", "e")
print("tuple:         ", t)
print("t[0], t[-1]:   ", t[0], t[-1])
print("t[1:3]:        ", t[1:3])
print("t[:2]:         ", t[:2])
print("t[3:]:         ", t[3:])
print("t[::2]:        ", t[::2])
print("t[::-1]:       ", t[::-1])
print("membership:    ", "c" in t, "z" in t)
print("len:           ", len(t))

print()
print("--- tuples are immutable ---")
immutable = (1, 2, 3)
try:
    immutable[0] = 99
except TypeError as e:
    print("assigning to a tuple raises TypeError:", e)
try:
    del immutable[0]
except TypeError as e:
    print("deleting from a tuple raises TypeError:", e)

print()
print("--- equality works, ordering does not ---")
print("(1,2) == (1,2):", (1, 2) == (1, 2))
print("(1,2) == (1,3):", (1, 2) == (1, 3))
try:
    print((1, 2) < (1, 3))
except TypeError as e:
    print("(1,2) < (1,3) raises TypeError:", e)
# Work around it by comparing element-wise with a key function.
pairs = [(2, "b"), (1, "a"), (3, "c")]
print("sort via key:  ", sorted(pairs, key=lambda p: p[0]))

print()
print("--- unpacking ---")
point = (3, 4)
x, y = point
print("x, y:          ", x, y)
x, y = y, x                             # the classic swap
print("swapped x, y:  ", x, y)
first, *middle, last = (1, 2, 3, 4, 5)
print("star unpack:   ", first, middle, last)
(a1, a2), a3 = (1, 2), 3
print("nested unpack: ", a1, a2, a3)
for index, (name, score) in enumerate([("amy", 90), ("ben", 80)]):
    print("loop unpack:   ", index, name, score)

print()
print("--- a worked example: named records without classes ---")
# A tuple per row is the classic lightweight record.
LANGUAGES = [
    ("Python", 1991, "Guido van Rossum"),
    ("Go", 2009, "Robert Griesemer et al."),
]
print("sorted by year (key fn):")
for name, year, author in sorted(LANGUAGES, key=lambda row: row[1]):
    print("  %-7s %d  %s" % (name, year, author))

print()
print("=== SETS ===")
print()
print("--- creating sets ---")
print("literal:       ", {1, 2, 3})
print("from list:     ", set([3, 1, 2, 1, 3]))
print("from string:   ", set("hello"))
print("from range:    ", set(range(4)))
print("empty (set()): ", set(), "length", len(set()))
print("comprehension: ", {x * x for x in range(6)})
print("note: {} is an empty DICT, not an empty set: type({}) is", type({}))

print()
print("--- set membership is by value, order is undefined ---")
s = {1, 2, 3}
print("set:           ", s)
print("membership:    ", 2 in s, 99 in s)
print("len:           ", len(s))
print("add():         ", end="")
s.add(4)
print(s)
print("iterate:       ", sorted(s))       # sort for a stable print

print()
print("--- set algebra: operators, not methods ---")
a = {1, 2, 3, 4}
b = {3, 4, 5, 6}
print("a:             ", sorted(a))
print("b:             ", sorted(b))
print("union  a | b:  ", sorted(a | b))
print("inter  a & b:  ", sorted(a & b))
print("diff   a - b:  ", sorted(a - b))
print("symdif a ^ b:  ", sorted(a ^ b))
print("equality:      ", {1, 2} == {2, 1}, {1} == {1, 2})
print("subset test:   ", ({1} & {1, 2}) == {1}, "via intersection, as issubset() is absent")

print()
print("--- removing from a set without remove()/discard() ---")
# set.remove / set.discard / set.pop / set.clear are all missing; rebuild.
data = {1, 2, 3, 4}
data = {x for x in data if x != 2}
print("remove 2:      ", sorted(data))
data = set()
print("clear:         ", data)

print()
print("--- the missing pieces ---")
for label, code in [
    ("frozenset([1])", "frozenset([1])"),
    ("{1}.union({2})", "{1}.union({2})"),
    ("{1}.issubset({1,2})", "{1}.issubset({1,2})"),
    ("{1}.remove(1)", "{1}.remove(1)"),
]:
    try:
        eval(compile(code, "<probe>", "eval"))
        print("%-22s works" % label)
    except (AttributeError, TypeError) as e:
        print("%-22s %s" % (label, type(e)))

print()
print("--- a worked example: unique words ---")
essay = "the cat sat on the mat the cat slept"
words = essay.split()
unique = set(words)
print("all words:     ", words)
print("unique count:  ", len(unique))
print("unique sorted: ", sorted(unique))
duplicated = sorted({w for w in unique if sum(1 for x in words if x == w) > 1})
print("repeated:      ", duplicated)
