"""Generators: yield, lazy evaluation, send, throw, yield from, close.

Run with:  /tmp/gpy examples/python/basics/generators.py

Note: this interpreter supports yield, yield-from, next(), send() and
throw(). The generator .close() method is NOT implemented and raises
NotImplementedError.
"""

print("--- a first generator ---")


def countdown(n):
    print("  countdown(%d) begins" % n)
    while n > 0:
        print("  about to yield", n)
        yield n
        n -= 1
    print("  countdown finished")


# Calling it does not run any code; it builds a generator object.
gen = countdown(3)
print("generator object:   ", gen)
print("now consuming it:")
for value in gen:
    print("  received", value)

print()
print("--- laziness: the body runs only when asked ---")


def noisy():
    print("  body started")
    yield 1
    print("  body continued")
    yield 2
    print("  body finished")


print("creating the generator (nothing runs yet)")
lazy = noisy()
print("first next() calls into the body:")
print("  next ->", next(lazy))
print("second next():")
print("  next ->", next(lazy))

print()
print("--- StopIteration and next() with a default ---")


def three():
    yield "a"
    yield "b"
    yield "c"


it = three()
print("next:               ", next(it))
print("next:               ", next(it))
print("next:               ", next(it))
print("next with default:  ", next(it, "exhausted"))
try:
    next(it)
except StopIteration as err:
    print("bare next() raises StopIteration:", repr(err))

print()
print("--- generators are single-use ---")


def letters():
    yield "x"
    yield "y"


g = letters()
print("first pass:         ", list(g))
print("second pass:        ", list(g), "(already exhausted)")

print()
print("--- generator expressions vs generator functions ---")
squares = (n * n for n in range(5))
print("genexpr object:     ", squares)
print("consumed:           ", list(squares))
print("equivalent defn:    ", list(n * n for n in range(5)))
print("as a function arg:  ", sum(n * n for n in range(5)))

print()
print("--- infinite generators with islice ---")
import itertools


def naturals(start=0):
    n = start
    while True:
        yield n
        n += 1


natural = naturals(10)
print("first five:         ", list(itertools.islice(natural, 5)))
print("next five:          ", list(itertools.islice(natural, 5)))


def fibonacci():
    a, b = 0, 1
    while True:
        yield a
        a, b = b, a + b


print("fibonacci:          ", list(itertools.islice(fibonacci(), 10)))


def take(n, iterable):
    """The classic lazy take, built on islice."""
    return list(itertools.islice(iterable, n))


print("take(3, naturals()):", take(3, naturals()))

print()
print("--- yield from: delegating to a sub-generator ---")


def inner():
    yield 1
    yield 2


def outer():
    yield 0
    yield from inner()          # forwards every value from the inner one
    yield 3


print("outer():            ", list(outer()))


def chained():
    yield from [10, 20]
    yield from (30, 40)
    yield from "ab"


print("chained sources:    ", list(chained()))

print()
print("--- send(): generators are two-way ---")


def accumulator():
    total = 0
    while True:
        received = yield total          # pauses here, waiting for a value
        if received is None:
            break
        total += received


acc = accumulator()
print("first next primes it:", next(acc))
print("send(10):           ", acc.send(10))
print("send(5):            ", acc.send(5))
print("send(1):            ", acc.send(1))

print()
print("--- throw(): inject an exception at the pause point ---")


def guarded():
    try:
        yield "ready"
    except ValueError as err:
        yield "recovered from %s" % err
    finally:
        pass


g2 = guarded()
print("primed:             ", next(g2))
print("after throw:        ", g2.throw(ValueError("boom")))

print()
print("--- pipelines of generators ---")
# chained generators give a streaming pipeline without materialising lists.


def numbers():
    for n in range(1, 21):
        yield n


def evens(source):
    for n in source:
        if n % 2 == 0:
            yield n


def squared(source):
    for n in source:
        yield n * n


pipeline = squared(evens(numbers()))
print("evens squared 1..20:", list(pipeline))
print("sum of that stream: ", sum(squared(evens(numbers()))))

print()
print("--- close() is not implemented here ---")
g3 = three()
print("first value:        ", next(g3))
try:
    g3.close()
    print("close() worked")
except NotImplementedError:
    # NOTE: do not print the exception itself here -- str(err) on an
    # exception with no args panics the interpreter (see KNOWN_ISSUES.md).
    print("close() raises NotImplementedError (unimplemented)")

print()
print("--- a worked example: reading lines lazily ---")


def parse_lines(text):
    for line in text.split("\n"):
        line = line.strip()
        if not line or line.startswith("#"):
            continue                     # skip blanks and comments
        key, value = line.split("=", 1)  # str.partition() is unimplemented here
        yield key.strip(), value.strip()


CONFIG = """\
# a comment line

name = gpython
version = 3.4
"""

print("parsed pairs:")
for key, value in parse_lines(CONFIG):
    print("  %-8s -> %s" % (key, value))
print("as a dict:          ", dict(parse_lines(CONFIG)))
