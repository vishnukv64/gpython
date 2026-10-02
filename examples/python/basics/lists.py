"""Lists: creation, indexing, slicing, mutation, and the supported methods.

Run with:  /tmp/gpy examples/python/basics/lists.py

This interpreter implements only three list methods: append, extend, sort.
Everything else (insert, remove, pop, index, count, reverse, copy, clear) is
missing, so this file shows the supported ways to do each job instead.
"""

print("--- creating lists ---")
print("literal:      ", [1, 2, 3])
print("empty:        ", [])
print("mixed types:  ", [1, "two", 3.0, True, None])
print("nested:       ", [[1, 2], [3, [4, 5]]])
print("from range:   ", list(range(5)))
print("from string:  ", list("abc"))
print("from tuple:   ", list((1, 2, 3)))
print("repeat:       ", [0] * 4)
print("concatenate:  ", [1, 2] + [3, 4])
print("comprehension:", [x * x for x in range(6)])

print()
print("--- indexing and slicing ---")
nums = [10, 20, 30, 40, 50]
print("list:         ", nums)
print("nums[0]:      ", nums[0])
print("nums[-1]:     ", nums[-1])
print("nums[1:3]:    ", nums[1:3])
print("nums[:2]:     ", nums[:2])
print("nums[3:]:     ", nums[3:])
print("nums[::2]:    ", nums[::2])
print("nums[::-1]:   ", nums[::-1])
print("len:          ", len(nums))
print("membership:   ", 30 in nums, 99 in nums, 99 not in nums)

print()
print("--- the implemented methods ---")
work = [3, 1, 2]
print("start:        ", work)
work.append(4)
print("append(4):    ", work)
work.extend([5, 6])
print("extend:       ", work)
work.sort()
print("sort():       ", work)
work.sort(reverse=True)
print("sort(rev):    ", work)
work.sort(key=lambda n: -n)
print("sort(key=):   ", work)

print()
print("--- doing the missing methods by hand ---")
data = [10, 20, 30, 40]

# insert: no list.insert, so rebuild the list around the new element.
pos, value = 1, 15
inserted = data[:pos] + [value] + data[pos:]
print("insert at 1:  ", inserted)

# remove/pop: slice around the element, or del by index.
removed = [x for x in data if x != 30]
print("remove 30:    ", removed)
tail = data[-1]
del data[-1]
print("pop last:     ", tail, "leaves", data)

# count and index: derive them with generators.
sample = [1, 2, 2, 3]
print("count of 2:   ", sum(1 for x in sample if x == 2))
print("index of 3:   ", [i for i, x in enumerate(sample) if x == 3][0])

# reverse: slicing is the shortest route.
print("reverse:      ", sample[::-1])
copy = sample[:]
print("copy:         ", copy, "independent:", copy is not sample)

print()
print("--- del, slice assignment, and augmented assignment ---")
mutable = [1, 2, 3, 4, 5]
del mutable[0]
print("del [0]:      ", mutable)
del mutable[1:3]
print("del [1:3]:    ", mutable)
mutable[0] = 99
print("assign [0]:   ", mutable)
mutable[1:1] = [7, 8]
print("slice insert: ", mutable)
mutable += [100]
print("+= :          ", mutable)
mutable *= 2
print("*= :          ", mutable)

print()
print("--- sorting ---")
words = ["banana", "kiwi", "apple", "fig"]
print("default:      ", sorted(words))
print("by length:    ", sorted(words, key=len))
print("reverse:      ", sorted(words, reverse=True))
print("by last char: ", sorted(words, key=lambda w: w[-1]))
pairs = [(2, "b"), (1, "a"), (3, "c")]
print("tuples by fst (lambda key, not the tuple itself):")
# Sorting tuples directly needs tuple <, which is not implemented here, so a
# key function extracts the comparable field.
print("              ", sorted(pairs, key=lambda p: p[0]))

print()
print("--- useful builtins over lists ---")
values = [4, 8, 15, 16, 23, 42]
print("min/max:      ", min(values), max(values))
print("sum:          ", sum(values))
print("sum(start):   ", sum(values, 100))
print("any/all:      ", any(x > 40 for x in values), all(x > 0 for x in values))
print("sorted copy:  ", sorted(values))
print("enumerate:    ", list(enumerate(values[:3])))
print("zip:          ", list(zip(values[:3], "abc")))
print("map:          ", list(map(lambda x: x * 2, values[:3])))
print("filter:       ", list(filter(lambda x: x % 2 == 0, values)))

print()
print("--- a worked example: running totals ---")
readings = [12, 7, 30, 4]
running = []
total = 0
for r in readings:
    total += r
    running.append(total)
print("readings:     ", readings)
print("running totals:", running)
print("final total:  ", total)
print("average:      ", "%.2f" % (total / len(readings)))
