"""Iterators: the iteration protocol, iter()/next(), and iterator tools.

Run with:  /tmp/gpy examples/python/basics/iterators.py

Note: the builtin reversed() is NOT implemented in this interpreter; use
slicing (seq[::-1]) or sorted(..., reverse=True) instead.
"""

print("--- iterating over the builtin types ---")
for label, obj in [("list", [1, 2, 3]), ("tuple", (1, 2, 3)), ("string", "abc"),
                   ("dict", {"x": 1, "y": 2}), ("set", {1, 2}), ("range", range(3))]:
    # NB: "%s" % <container> prints Go-internal struct noise in this
    # interpreter, so containers are printed directly, not through a %s
    # format operand (see KNOWN_ISSUES.md).
    print("%-8s ->" % label, [item for item in obj])

print()
print("--- iter() and next(): the protocol by hand ---")
values = [10, 20, 30]
it = iter(values)
print("iterator object:    ", it)
print("next(it):           ", next(it))
print("next(it):           ", next(it))
print("next(it):           ", next(it))
print("next(it, default):  ", next(it, "no more items"))

try:
    next(it)
except StopIteration as err:
    print("next on an exhausted iterator raises StopIteration:", repr(err))

print()
print("--- what a for loop does under the hood ---")
manual = []
iterator = iter(["a", "b", "c"])
while True:
    try:
        item = next(iterator)
    except StopIteration:
        break
    manual.append(item)
print("manual loop:        ", manual)
print("for loop:           ", [x for x in ["a", "b", "c"]])
print("identical:          ", manual == ["a", "b", "c"])

print()
print("--- writing a custom iterator class ---")


class Countdown:
    """Iterates from `start` down to 1."""

    def __init__(self, start):
        self.current = start

    def __iter__(self):
        return self                      # the object is its own iterator

    def __next__(self):
        if self.current <= 0:
            raise StopIteration
        value = self.current
        self.current -= 1
        return value


print("for loop:           ", [n for n in Countdown(4)])
print("comprehension:      ", [n for n in Countdown(3)])
# NOTE: list(Countdown(...)) does NOT work: the builtin list() does not catch
# StopIteration raised by a user-defined __next__ and it propagates as an
# uncaught error. A for loop or comprehension does catch it. See
# KNOWN_ISSUES.md.
print("next() by hand:     ", end="")
c = Countdown(3)
print(next(c), next(c), next(c))

# Because the object is its own iterator, it is exhausted after one pass.
c2 = Countdown(2)
print("exhausted class:    ", [n for n in c2], "then", [n for n in c2])

print()
print("--- a reusable iterable vs a one-shot iterator ---")


class Fib:
    def __init__(self, count):
        self.count = count

    def __iter__(self):
        # A FRESH iterator every time, so the object can be looped repeatedly.
        a, b = 0, 1
        for _ in range(self.count):
            yield a
            a, b = b, a + b


fibs = Fib(8)
print("first pass:         ", list(fibs))
print("second pass:        ", list(fibs), "(works: __iter__ returns a new generator)")

print()
print("--- built-in iterator helpers ---")
data = [30, 10, 20]
print("enumerate:          ", list(enumerate(data)))
print("enumerate(start=1): ", list(enumerate(data, 1)))
print("zip:                ", list(zip([1, 2, 3], "abc")))
print("zip stops shortest: ", list(zip([1, 2, 3], "ab")))
print("map:                ", list(map(str, [1, 2])))
print("filter:             ", list(filter(lambda n: n > 15, data)))
print("sorted:             ", sorted(data))
print("sorted reverse:     ", sorted(data, reverse=True))
print("reversed via slice: ", data[::-1])
print("sum of range:       ", sum(range(5)))
print("range as a sequence:", len(range(10)), range(10)[5], 7 in range(10))

print()
print("reversed() is not implemented here:")
try:
    reversed([1, 2, 3])
except NameError as err:
    print("  reversed(...) raises NameError:", err)

print()
print("--- itertools: the standard iterator toolkit ---")
import itertools

print("chain:              ", list(itertools.chain([1, 2], "ab", (3,))))
print("islice:             ", list(itertools.islice(range(100), 3)))
print("islice(start,stop): ", list(itertools.islice(range(100), 2, 5)))
print("islice step:        ", list(itertools.islice(range(100), 0, 10, 3)))
print("repeat:             ", list(itertools.repeat("x", 3)))
print("count:              ", list(itertools.islice(itertools.count(5, 2), 4)))
print("cycle:              ", list(itertools.islice(itertools.cycle("ab"), 5)))
print("accumulate:         ", list(itertools.accumulate([1, 2, 3, 4])))
print("combinations:       ", list(itertools.combinations([1, 2, 3], 2)))
print("permutations:       ", list(itertools.permutations([1, 2, 3], 2)))
print("product:            ", list(itertools.product([0, 1], repeat=2)))
print("zip_longest:        ", list(itertools.zip_longest([1], [2, 3], fillvalue=0)))
print("compress:           ", list(itertools.compress([1, 2, 3, 4], [1, 0, 1, 0])))
print("takewhile:          ", list(itertools.takewhile(lambda n: n < 4, [1, 2, 5, 1])))
print("dropwhile:          ", list(itertools.dropwhile(lambda n: n < 4, [1, 2, 5, 1])))
print("filterfalse:        ", list(itertools.filterfalse(lambda n: n % 2, range(6))))
print("starmap:            ", list(itertools.starmap(pow, [(2, 3), (3, 2)])))
print("groupby:            ", end=" ")
for key, group in itertools.groupby("aabbc"):
    print("%s%s" % (key, list(group)), end=" ")
print()

print()
print("--- a worked example: a chunking helper ---")


def chunks(iterable, size):
    """Yield successive lists of at most `size` items."""

    iterator = iter(iterable)
    while True:
        block = list(itertools.islice(iterator, size))
        if not block:
            return                       # a bare return ends the generator
        yield block


print("chunks of 2:        ", list(chunks(range(7), 2)))
print("chunks of 3:        ", list(chunks("abcdefg", 3)))

print()
print("--- a worked example: pairing and pairing up ---")
names = ["ada", "bob", "cyd", "dan"]
scores = [91, 82, 77]
print("zip (shortest wins):", list(zip(names, scores)))
print("zip_longest padded: ", list(itertools.zip_longest(names, scores, fillvalue=0)))
pairs = list(zip(names, scores))
print("best score:         ", max(scores))
print("pairs over 80:      ", [p for p in pairs if p[1] > 80])
