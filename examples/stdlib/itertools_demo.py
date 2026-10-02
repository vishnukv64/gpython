"""itertools: lazy building blocks for iteration.

Run with:  /tmp/gpy examples/stdlib/itertools_demo.py

Every function here returns an iterator, so nothing is computed until you
consume it. This demo materialises with list() so the output is visible.
"""

import itertools

print("--- producing sequences ---")
print("count from 5, first 4:      ", list(itertools.islice(itertools.count(5), 4)))
print("count with step 3:         ", list(itertools.islice(itertools.count(0, 3), 4)))
print("repeat 'x' four times:     ", list(itertools.repeat("x", 4)))
print("cycle 'ab', first 5:       ", list(itertools.islice(itertools.cycle("ab"), 5)))

print()
print("--- combining ---")
print("chain([1,2], [3], [4]):    ", list(itertools.chain([1, 2], [3], [4])))
print("zip_longest pad 0:", list(itertools.zip_longest([1, 2, 3], [4], fillvalue=0)))
print("product([1,2], 'ab'):      ", list(itertools.product([1, 2], "ab")))
print("product repeat=2:          ", list(itertools.product([0, 1], repeat=2)))
print("starmap(pow, ...):         ", list(itertools.starmap(pow, [(2, 3), (3, 2), (5, 0)])))

print()
print("--- selecting ---")
print("islice from 2 to 5:        ", list(itertools.islice(range(10), 2, 5)))
print("islice with step 2:        ", list(itertools.islice(range(10), 0, 8, 2)))
print("takewhile x < 3:           ", list(itertools.takewhile(lambda x: x < 3, [1, 2, 3, 1])))
print("dropwhile x < 3:           ", list(itertools.dropwhile(lambda x: x < 3, [1, 2, 3, 1])))
print("filterfalse even:          ", list(itertools.filterfalse(lambda x: x % 2 == 0, range(8))))
print("compress:                  ", list(itertools.compress("abcdef", [1, 0, 1, 0, 1, 0])))

print()
print("--- combining values pairwise ---")
print("accumulate running sum:    ", list(itertools.accumulate([1, 2, 3, 4])))
print("accumulate with max:       ", list(itertools.accumulate([3, 1, 4, 1, 5],
                                                                  lambda a, b: a if a > b else b)))
print("pairwise:                  ", list(itertools.pairwise([1, 2, 3, 4])))

print()
print("--- combinations and permutations ---")
letters = ["a", "b", "c"]
print("combinations of 2:  ", list(itertools.combinations(letters, 2)))
print("permutations of 2:  ", list(itertools.permutations(letters, 2)))
print("all permutations:   ", list(itertools.permutations("ab")))
print("count of combinations is C(3,2)=3:", len(list(itertools.combinations(letters, 2))))

print()
print("--- groupby groups *runs*, not all equal keys ---")
data = "aabbcca"
for key, group in itertools.groupby(data):
    print("  %r -> %s" % (key, list(group)))
print("note the final 'a' forms its own group; sorted the input first it would not.")

data = "aabbcca"
print("sorted first:", [(key, list(group)) for key, group in itertools.groupby(sorted(data))])

print()
print("--- tee splits one iterator into independent copies ---")
source = iter([1, 2, 3])
first, second = itertools.tee(source, 2)
print("first: ", list(first))
print("second:", list(second))
print("(tee buffers what one copy consumed before the other)")

print()
print("--- a worked example: sliding windows and running statistics ---")


def sliding_window(iterable, size):
    """Yield overlapping windows, using islice + tee."""
    copies = itertools.tee(iterable, size)
    # Advance copy i by i so the windows line up.
    for index, copy in enumerate(copies):
        for _ in range(index):
            next(copy, None)
    return zip(*copies)


readings = [10, 12, 11, 15, 14, 20]
print("readings:", readings)
print("windows of 3:", list(sliding_window(readings, 3)))
for window in sliding_window(readings, 3):
    average = sum(window) / float(len(window))
    print("  window %s average %.2f" % (list(window), average))

print()
print("--- an unbounded stream, consumed lazily ---")


def natural_numbers():
    n = 1
    while True:
        yield n
        n = n + 1


numbers = natural_numbers()
print("first 10:", list(itertools.islice(numbers, 10)))
print("next 5:  ", list(itertools.islice(numbers, 5)))

print()
print("--- what is not available ---")
print("itertools.batched exists:", hasattr(itertools, "batched"))
print("chain.from_iterable exists:", hasattr(itertools.chain, "from_iterable"))
