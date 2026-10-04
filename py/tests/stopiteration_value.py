"""StopIteration.value carries a generator's return value."""

print("first arg:", StopIteration(42).value)
print("bare is None:", StopIteration().value)
print("args still set:", StopIteration(42).args)
print("two args take the first:", StopIteration(1, 2).value)

# It is writable, which PEP 380's algorithm relies on.
s = StopIteration()
s.value = 5
print("writable:", s.value)

# A generator's return value arrives as StopIteration.value, which is what
# "yield from" reads.
def sub():
    yield 1
    return "returned"


it = sub()
print("first yield:", next(it))
try:
    next(it)
except StopIteration as e:
    print("value after return:", e.value)

# A generator WITH a yield, whose final return carries the value: this is the
# PEP 380 shape, and what "yield from" is defined to read.
def with_yield():
    yield 1
    return "R"


g = with_yield()
next(g)
try:
    next(g)
except StopIteration as e:
    print("via return:", e.value)

# A generator with NO return value raises a StopIteration whose value is None.
def bare():
    yield 1


h = bare()
next(h)
try:
    next(h)
except StopIteration as e:
    print("bare:", e.value)

doc = "finished"
