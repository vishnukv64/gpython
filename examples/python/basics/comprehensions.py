"""Comprehensions: list, dict, set and generator expressions.

Run with:  /tmp/gpy examples/python/basics/comprehensions.py

This interpreter supports all four comprehension forms, including nested
`for` clauses, conditions, and generator expressions as function arguments.
"""

print("--- list comprehensions ---")
print("squares:            ", [x * x for x in range(6)])
print("with a filter:      ", [x for x in range(10) if x % 3 == 0])
print("if and else:        ", ["even" if x % 2 == 0 else "odd" for x in range(5)])
print("calling a function: ", [len(w) for w in ["a", "bb", "ccc"]])
print("nested for:         ", [(x, y) for x in range(2) for y in "ab"])
print("two conditions:     ", [x for x in range(20) if x % 2 == 0 if x % 3 == 0])
print("over a dict:        ", [k for k in {"a": 1, "b": 2}])
print("over a string:      ", [c for c in "hello" if c not in "aeiou"])

print()
print("--- the same thing written as a loop ---")
loop_version = []
for x in range(6):
    loop_version.append(x * x)
print("loop:               ", loop_version)
print("comprehension:      ", [x * x for x in range(6)])
print("identical:          ", loop_version == [x * x for x in range(6)])

print()
print("--- dict comprehensions ---")
print("square map:         ", {n: n * n for n in range(5)})
print("from two lists:     ", {k: v for k, v in zip("abc", [1, 2, 3])})
print("inverted:           ", {v: k for k, v in {"a": 1, "b": 2}.items()})
print("with a filter:      ", {n: n % 2 for n in range(6) if n % 2 == 0})
print("nested dict:        ", {n: {n: n * 2} for n in range(3)})
print("words by length:    ", {w: len(w) for w in ["to", "be", "or", "not"]})

print()
print("--- set comprehensions ---")
print("squares:            ", sorted({x * x for x in range(-3, 4)}))
print("unique lengths:     ", sorted({len(w) for w in "a bb ccc dd".split()}))
print("filtered:           ", sorted({x % 5 for x in range(20) if x % 2 == 0}))
print("dedup a list:       ", sorted({3, 1, 2, 1, 3, 2}))

print()
print("--- generator expressions ---")
# A generator expression uses parentheses instead of brackets and produces
# values lazily. As a sole function argument the parentheses can be omitted.
print("sum of squares:     ", sum(x * x for x in range(6)))
print("max word length:    ", max(len(w) for w in "one three four".split()))
print("any negative:       ", any(x < 0 for x in [1, -2, 3]))
print("all positive:       ", all(x > 0 for x in [1, 2, 3]))
print("joined:             ", ", ".join(str(x) for x in range(4)))
print("count matching:     ", sum(1 for x in range(100) if x % 7 == 0))

# A generator expression object is single-use and lazy.
gen = (x * 2 for x in range(4))
print("materialised once:  ", list(gen))
print("exhausted after:    ", list(gen), "(a generator is consumed once)")

print()
print("--- variable scope in comprehensions ---")
x = "outer"
result = [x for x in range(3)]
print("comprehension output:", result)
print("the outer x survives:", x)

print()
print("--- building nested structures ---")
matrix = [[row * 3 + col for col in range(3)] for row in range(3)]
print("matrix:             ", matrix)
print("flattened:          ", [n for row in matrix for n in row])
print("transposed:         ", [[row[i] for row in matrix] for i in range(3)])
print("row sums:           ", [sum(row) for row in matrix])
print("diagonal:           ", [matrix[i][i] for i in range(3)])

print()
print("--- a worked example: parsing records ---")
raw = "alice:90,bob:75,carol:82"
records = [entry.split(":") for entry in raw.split(",")]
print("split records:      ", records)
scores = {name: int(score) for name, score in records}
print("as a dict:          ", scores)
passing = sorted([name for name, score in records if int(score) >= 80])
print("passing (>=80):     ", passing)
average = sum(int(score) for _, score in records) / len(records)
print("average:            ", "%.2f" % average)

print()
print("--- a worked example: word frequencies, three ways ---")
text = "b a n a n a"
print("counts via loop:")
counts = {}
for word in text.split():
    counts[word] = counts.get(word, 0) + 1
print("                   ", counts)
print("unique letters:     ", sorted({c for c in text if c != " "}))
print("letters per word:   ", [[c for c in w] for w in text.split()])
