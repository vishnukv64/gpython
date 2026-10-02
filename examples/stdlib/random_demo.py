"""random: pseudo-random numbers.

Run with:  /tmp/gpy examples/stdlib/random_demo.py

Seeding with a fixed value makes the sequence reproducible, which is what the
demo does everywhere so its output is stable. The module-level functions share
one generator; random.Random instances have their own.

Interpreter note: the module-level functions and random.Random(seed) produce
the same stream for the same seed here, so seeding either gives reproducible
sequences. Do not rely on the values matching CPython's.
"""

import random

print("--- seeding makes a sequence reproducible ---")
random.seed(42)
first_run = [random.random() for _ in range(3)]
print("seed(42), three random() values:", first_run)
random.seed(42)
second_run = [random.random() for _ in range(3)]
print("seed(42) again:                ", second_run)
print("identical:", first_run == second_run)

print()
print("--- random() is in [0.0, 1.0) ---")
random.seed(7)
values = [random.random() for _ in range(5)]
print("values:", values)
print("all in range:", all([0.0 <= value < 1.0 for value in values]))

print()
print("--- integers ---")
random.seed(1)
print("randint(1, 6) rolls:      ", [random.randint(1, 6) for _ in range(6)])
random.seed(1)
print("randrange(10):            ", [random.randrange(10) for _ in range(6)])
random.seed(1)
print("randrange(0, 10, 2) evens:", [random.randrange(0, 10, 2) for _ in range(6)])
random.seed(1)
print("getrandbits(8):           ", [random.getrandbits(8) for _ in range(4)])

print()
print("--- floating point distributions ---")
random.seed(3)
print("uniform(-1, 1):  ", [round(random.uniform(-1, 1), 4) for _ in range(4)])
random.seed(3)
print("gauss(100, 15):  ", [round(random.gauss(100, 15), 3) for _ in range(4)])
random.seed(3)
print("normalvariate(0, 1):", [round(random.normalvariate(0, 1), 4) for _ in range(4)])
random.seed(3)
print("expovariate(1.0):   ", [round(random.expovariate(1.0), 4) for _ in range(4)])

print()
print("--- choosing from a sequence ---")
items = ["ann", "bob", "cat", "dan", "eve"]
random.seed(11)
print("choice:  ", random.choice(items))
random.seed(11)
print("choices(k=3):", random.choices(items, k=3))
random.seed(11)
print("sample(3):   ", random.sample(items, 3))
print("(choice and sample never repeat an element; choices may)")

print()
print("--- shuffling is in place and returns None ---")
deck = list(range(1, 11))
random.seed(5)
result = random.shuffle(deck)
print("shuffle returned:", result)
print("shuffled deck:   ", deck)
random.seed(5)
same = list(range(1, 11))
random.shuffle(same)
print("same seed, same order:", same == deck)
print("it is a permutation: ", sorted(deck) == list(range(1, 11)))

print()
print("--- a private generator, independent of the module-level one ---")
private = random.Random(99)
print("Random(99) first three:", [round(private.random(), 4) for _ in range(3)])
print("seeding it again restarts:")
private.seed(99)
print("                       ", [round(private.random(), 4) for _ in range(3)])

print()
print("--- a worked example: drawing a reproducible sample ---")


class Sampler:
    """A deterministic sampler over a roster."""

    def __init__(self, roster, seed):
        self.roster = list(roster)
        self.generator = random.Random(seed)

    def draw(self, count):
        return self.generator.sample(self.roster, count)

    def simulate(self, rounds, sides):
        """Roll `sides`-sided dice, `rounds` times, and tally the results."""
        tally = {}
        for _ in range(rounds):
            roll = self.generator.randint(1, sides)
            tally[roll] = tally.get(roll, 0) + 1
        return tally


roster = ["ann", "bob", "cat", "dan", "eve", "fay", "gil"]
sampler = Sampler(roster, 2024)
print("roster:", roster)
print("draw of 3 (seed 2024):", sampler.draw(3))
print("another draw from the same sampler:", sampler.draw(3))
print("a fresh sampler with the same seed repeats the first draw:",
      Sampler(roster, 2024).draw(3) == Sampler(roster, 2024).draw(3))

tally = Sampler(roster, 8).simulate(600, 6)
print()
print("600 rolls of a six-sided die, tallied (counts are not uniform exactly):")
for face in sorted(tally):
    bar = "#" * (tally[face] // 5)
    print("  %d: %3d %s" % (face, tally[face], bar))
print("total rolls counted:", sum([tally[face] for face in tally]))
