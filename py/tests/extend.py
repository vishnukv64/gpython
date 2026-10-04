print([].__class__)
a = [1]
a.extend([2, 3])
print(a)
a.extend((4,))
print(a)
a.extend("ab")
print(a)
a.extend({"k": "v"})
print(a)
a.extend(x for x in range(2))
print(a)
a.extend((x * 2 for x in [1, 2]))
print(a)
a.extend(range(2))
print(a)
a.extend(set([9]))
print([x for x in a if isinstance(x, int)])
try:
    a.extend(1)
except TypeError as e:
    print("TypeError:", e)

doc = "finished"
