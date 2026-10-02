"""Control flow: if/elif/else, while, for, break, continue, and loop else.

Run with:  /tmp/gpy examples/python/basics/control_flow.py
"""

print("--- if / elif / else ---")
for score in [95, 82, 71, 40]:
    if score >= 90:
        grade = "A"
    elif score >= 80:
        grade = "B"
    elif score >= 70:
        grade = "C"
    else:
        grade = "F"
    print("score %3d -> %s" % (score, grade))

print()
print("--- conditional expressions ---")
for n in [3, -3, 0]:
    print("n=%2d  sign=%s  abs=%s" % (n, "negative" if n < 0 else "non-negative", n if n >= 0 else -n))

print()
print("--- nested conditionals and combined tests ---")
for n in range(1, 16):
    if n % 15 == 0:
        word = "FizzBuzz"
    elif n % 3 == 0:
        word = "Fizz"
    elif n % 5 == 0:
        word = "Buzz"
    else:
        word = str(n)
    print(word, end=" ")
print()

print()
print("--- truthiness in conditions ---")
for value in [0, 1, "", "text", [], [0], None]:
    if value:
        print("%-8r -> truthy" % (value,))
    else:
        print("%-8r -> falsy" % (value,))

print()
print("--- while loops ---")
countdown = 5
while countdown > 0:
    print("countdown", countdown)
    countdown -= 1
print("liftoff")

print()
print("--- while with break ---")
n = 0
while True:
    n += 1
    if n * n > 50:
        break
print("first square above 50:", n, "->", n * n)

print()
print("--- while with continue ---")
n = 0
evens = []
while n < 10:
    n += 1
    if n % 2:
        continue
    evens.append(n)
print("even numbers:", evens)

print()
print("--- for over different iterables ---")
print("range:      ", end=" ")
for i in range(3):
    print(i, end=" ")
print()
print("range step: ", end=" ")
for i in range(0, 10, 3):
    print(i, end=" ")
print()
print("list:       ", end=" ")
for item in ["a", "b", "c"]:
    print(item, end=" ")
print()
print("string:     ", end=" ")
for ch in "abc":
    print(ch, end=" ")
print()
print("tuple:      ", end=" ")
for t in (1, 2, 3):
    print(t, end=" ")
print()
print("dict:       ", end=" ")
for key in {"x": 1, "y": 2}:
    print(key, end=" ")
print()
print("dict items: ", end=" ")
for key, value in {"x": 1, "y": 2}.items():
    print("%s=%s" % (key, value), end=" ")
print()
print("enumerate:  ", end=" ")
for i, ch in enumerate("abc"):
    print("%d:%s" % (i, ch), end=" ")
print()
print("zip:        ", end=" ")
for a, b in zip([1, 2], ["x", "y"]):
    print("%s%s" % (a, b), end=" ")
print()
print("generator:  ", end=" ")
for g in (x * 2 for x in range(3)):
    print(g, end=" ")
print()

print()
print("--- break ---")
for i in range(10):
    if i == 4:
        print("stopping at", i)
        break
    print("tick", i)

print()
print("--- continue ---")
odd = []
for i in range(10):
    if i % 2 == 0:
        continue
    odd.append(i)
print("odd numbers:", odd)

print()
print("--- loop else (runs when no break occurred) ---")
for n in [1, 2, 3]:
    print("checking", n)
else:
    print("for-else ran: nothing broke out")

for n in [1, 2, 3]:
    if n == 2:
        break
else:
    print("this for-else does NOT run because of the break")
print("after the breaking loop")

n = 0
while n < 3:
    n += 1
else:
    print("while-else ran: condition became false at n =", n)

n = 0
while True:
    n += 1
    if n >= 2:
        break
else:
    print("this while-else does NOT run because of the break")
print("after the breaking while")

print()
print("--- search with a loop-else idiom ---")
haystack = ["red", "green", "blue"]
for colour in haystack:
    if colour == "green":
        print("found green")
        break
else:
    print("green not found")

for colour in haystack:
    if colour == "purple":
        break
else:
    print("purple not found (for-else fired)")

print()
print("--- pass and nested loops ---")
for i in range(3):
    for j in range(3):
        if j > i:
            break
        print("(%d,%d)" % (i, j), end=" ")
print()

# `pass` is a placeholder that does nothing.
def not_implemented_yet():
    pass

print("pass compiles and runs:", not_implemented_yet() is None)
