"""functools: small tools for working with functions.

Run with:  /tmp/gpy examples/stdlib/functools_demo.py

Covers reduce, partial, wraps/update_wrapper, lru_cache/cache, singledispatch,
cmp_to_key, total_ordering and partialmethod.

Interpreter notes, all checked below:
  * operator is not available, so reduce() is given a lambda instead.
  * partial() with positional bound arguments works; binding then supplying a
    keyword for the same parameter raises TypeError.
  * partialmethod() is present but does not pass `self` through, so calling one
    as a method raises TypeError.
  * total_ordering cannot synthesise the comparisons here because < on plain
    objects is not implemented.
"""

import functools

print("--- reduce folds a sequence into one value ---")
print("reduce(add, [1, 2, 3, 4]):",
      functools.reduce(lambda a, b: a + b, [1, 2, 3, 4]))
print("reduce with initial 100: ",
      functools.reduce(lambda a, b: a + b, [1, 2, 3, 4], 100))
print("reduce max:              ",
      functools.reduce(lambda a, b: a if a > b else b, [3, 1, 4, 1, 5, 9, 2]))

print()
print("--- partial binds arguments up front (positional binding works) ---")


def volume(length, width, height):
    return length * width * height


print("full call:                 ", volume(2, 3, 4))
flat = functools.partial(volume, 2)
print("partial(volume, 2)(3, 4):  ", flat(3, 4))
print("partial(volume, 2, 3)(4):  ", functools.partial(volume, 2, 3)(4))
keyword_only = functools.partial(volume, height=10)
print("partial(volume, height=10)(2, 3):", keyword_only(2, 3))

print()
print("--- wraps() copies the wrapped function's metadata ---")


def traced(fn):
    @functools.wraps(fn)
    def wrapper(*args, **kwargs):
        print("  calling", fn.__name__, "with", args)
        return fn(*args, **kwargs)
    return wrapper


@traced
def greet(name):
    """Say hello."""
    return "hello " + name


print(greet("world"))
print("name preserved:", greet.__name__)
print("doc preserved: ", greet.__doc__)

print()
print("--- lru_cache remembers results ---")

calls = []


@functools.lru_cache(maxsize=None)
def slow_square(n):
    calls.append(n)
    return n * n


print("slow_square(4):", slow_square(4))
print("slow_square(4):", slow_square(4), "(served from cache)")
print("slow_square(5):", slow_square(5))
print("underlying calls:", calls)
print("cache_info():", slow_square.cache_info())


@functools.cache
def fib(n):
    if n < 2:
        return n
    return fib(n - 1) + fib(n - 2)


print("fib(20) computed with @cache:", fib(20))


class Counter:
    def __init__(self):
        self.calls = 0

    def __call__(self, x):
        self.calls += 1
        return x * 2


@functools.cache
def cached_increment(x):
    return x + 1


print("cached_increment is memoised:", cached_increment(1) == cached_increment(1))

print()
print("--- singledispatch picks an implementation by type ---")


@functools.singledispatch
def describe(value):
    return "a plain object"


@describe.register(int)
def _(value):
    return "an int: %d" % (value,)


@describe.register(str)
def _(value):
    return "a string of length %d" % (len(value),)


for value in [1, "abc", [1, 2], 4.5]:
    print("  %-8r -> %s" % (value, describe(value)))

print()
print("--- cmp_to_key lets a three-way comparison drive sorted() ---")


def by_length_then_alpha(a, b):
    if len(a) != len(b):
        return len(a) - len(b)
    if a < b:
        return -1
    if a > b:
        return 1
    return 0


words = ["bbb", "a", "cc", "aa", "d"]
print("words:          ", words)
print("sorted by cmp:  ", sorted(words, key=functools.cmp_to_key(by_length_then_alpha)))

print()
print("--- update_wrapper is the machinery behind wraps ---")


def original():
    """The original docstring."""


def replacement():
    pass


functools.update_wrapper(replacement, original)
print("name after update_wrapper:", replacement.__name__)
print("doc after update_wrapper: ", replacement.__doc__)

print()
print("--- total_ordering: the decorator exists but cannot work here ---")


@functools.total_ordering
class Rank:
    def __init__(self, value):
        self.value = value

    def __eq__(self, other):
        return self.value == other.value

    def __lt__(self, other):
        return self.value < other.value


try:
    print(Rank(1) < Rank(2))
except TypeError as err:
    print("Rank(1) < Rank(2) raises TypeError:", err)
    print("(comparison operators on instances are not implemented)")

print()
print("--- partialmethod: present, but it does not bind self here ---")


class Calculator:
    def add(self, a, b):
        return a + b

    add_ten = functools.partialmethod(add, 10)


print("Calculator.__dict__['add_ten']:", Calculator.__dict__.get("add_ten"))
try:
    print("Calculator().add_ten(5):", Calculator().add_ten(5))
except TypeError as err:
    print("Calculator().add_ten(5) raises TypeError:", err)
    print("(the bound self is not passed through, so `a` receives 10)")

print()
print("--- module surface ---")
for name in ["reduce", "partial", "partialmethod", "wraps", "update_wrapper",
             "lru_cache", "cache", "singledispatch", "singledispatchmethod",
             "cmp_to_key", "total_ordering"]:
    print("  %-22s %s" % (name, hasattr(functools, name)))
