"""Reflected operators, and functools.lru_cache on a method."""

import functools


class Summable:
    """Only __radd__, which is what sum() needs to include it."""

    def __init__(self, v):
        self.v = v

    def __radd__(self, other):
        if other == 0:
            return self
        return Summable(self.v + other.v)

    def __repr__(self):
        return "Summable(%r)" % (self.v,)


print("radd:", 0 + Summable(3))
# sum() starts from 0, so the FIRST addition is int + Summable - exactly the
# case a reflected method exists for.  The later ones are Summable + Summable,
# which CPython refuses (measured), so a single element is what is summed here.
print("sum:", sum([Summable(5)], 0))


class Adder:
    """A __add__ written in Python, reached through the operator."""

    def __init__(self, v):
        self.v = v

    def __add__(self, other):
        return Adder(self.v + other.v)

    def __repr__(self):
        return "Adder(%r)" % (self.v,)


print("add:", Adder(1) + Adder(2))


class WithCached:
    def __init__(self, v):
        self.v = v

    @functools.lru_cache(maxsize=8)
    def scaled(self, factor):
        return self.v * factor


c = WithCached(7)
print("cached method:", c.scaled(3))
print("again:", c.scaled(3))
print("plain cached fn:", functools.lru_cache()(lambda x: x + 1)(41))

doc = "finished"
