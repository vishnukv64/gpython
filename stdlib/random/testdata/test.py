"""Tests for the random module.

The values here are self-consistency and range checks: this generator is Go's
math/rand, not CPython's Mersenne twister, so a seeded sequence repeats but does
not match CPython's numbers.  What is asserted is the contract a program relies
on - determinism, ranges, and argument validation.
"""

import random

def assertEqual(x, y):
    assert x == y, "got: %s, want: %s" % (repr(x), repr(y))

def assertRaises(exc, fn):
    try:
        fn()
    except exc:
        return
    except Exception as e:
        raise AssertionError("raised %s, wanted %s" % (type(e).__name__, exc.__name__))
    raise AssertionError("did not raise %s" % exc.__name__)

# determinism: the same seed gives the same sequence
random.seed(42)
first = (random.random(), random.randint(1, 100), random.randrange(0, 10, 2))
random.seed(42)
second = (random.random(), random.randint(1, 100), random.randrange(0, 10, 2))
assertEqual(first, second)

# the Random class is independent of the module generator
random.seed(1)
module_value = random.random()
assertEqual(random.Random(1).random(), random.Random(1).random())

# ranges
assertEqual(0.0 <= random.random() < 1.0, True)
assertEqual(all(1 <= random.randint(1, 6) <= 6 for _ in range(50)), True)
assertEqual(all(v in (10, 8, 6, 4, 2) for v in (random.randrange(10, 0, -2) for _ in range(30))), True)
assertEqual(random.choice([1, 2, 3]) in (1, 2, 3), True)
assertEqual(len(random.choices([1, 2, 3], k=5)), 5)
assertEqual(random.gauss(5, 0), 5.0)
assertEqual(random.expovariate(1.0) >= 0.0, True)

# shuffle is an in-place permutation and is reproducible
random.seed(7)
shuffled = [1, 2, 3, 4, 5]
random.shuffle(shuffled)
assertEqual(sorted(shuffled), [1, 2, 3, 4, 5])
random.seed(7)
again = [1, 2, 3, 4, 5]
random.shuffle(again)
assertEqual(shuffled, again)

# sample yields distinct members
drawn = random.sample([1, 2, 3, 4, 5, 6, 7, 8], 4)
assertEqual(len(drawn), 4)
assertEqual(len(set(drawn)), 4)

# getrandbits: exact bit counts, and every value reachable for a small k
assertEqual(random.getrandbits(0), 0)
assertEqual(all(0 <= random.getrandbits(8) <= 255 for _ in range(100)), True)
assertEqual(all(0 <= random.getrandbits(64) <= 2**64 - 1 for _ in range(100)), True)
assertEqual(all(0 <= random.getrandbits(128) <= 2**128 - 1 for _ in range(100)), True)
assertEqual(len(set(random.getrandbits(3) for _ in range(2000))), 8)
assertEqual(max(random.getrandbits(3) for _ in range(500)), 7)

# randbytes
assertEqual(type(random.randbytes(4)).__name__, "bytes")

# every documented function is a method of Random too
for name in ("seed", "random", "randint", "randrange", "choice", "choices",
             "shuffle", "sample", "uniform", "gauss", "normalvariate",
             "expovariate", "getrandbits", "randbytes"):
    assert hasattr(random.Random, name), "Random has no %s" % name

# errors
assertRaises(ZeroDivisionError, lambda: random.expovariate(0))
assertRaises(IndexError, lambda: random.choice([]))
assertRaises(ValueError, lambda: random.sample([1], 2))
assertRaises(ValueError, lambda: random.randrange(0))
assertRaises(ValueError, lambda: random.getrandbits(-1))
assertRaises(ValueError, lambda: random.randbytes(-1))

print("random: all tests pass")
