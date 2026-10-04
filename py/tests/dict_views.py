"""dict views: live, sized, and set-like for keys."""

d = {"a": 1, "b": 2}
k = d.keys()
v = d.values()
i = d.items()

print("keys type:", type(k).__name__)
print("values type:", type(v).__name__)
print("items type:", type(i).__name__)

print("len k v i:", len(k), len(v), len(i))
print("key membership:", "a" in k, "z" in k)
print("value membership:", 1 in v, 9 in v)
print("item membership:", ("a", 1) in i, ("a", 2) in i, "a" in i)

# LIVE: a mutation after creation is visible.
d["c"] = 3
print("after insert:", len(k), "c" in k, sorted(k))
del d["a"]
print("after delete:", len(k), "a" in k, sorted(k))

print("repr keys:", repr(k))
print("repr values:", repr(v))

# A keys view behaves as a set.
d2 = {"x": 0, "b": 2, "c": 3}
print("keys &: sorted", sorted(d2.keys() & d.keys()))
print("keys |: sorted", sorted(d2.keys() | d.keys()))
print("keys -: sorted", sorted(d2.keys() - d.keys()))
print("keys ==:", d2.keys() == d.keys())
print("keys == set:", set(["b", "c"]) == d.keys())

# Iteration repeats rather than being consumed, unlike an iterator.
print("twice:", sorted(k), sorted(k))

doc = "finished"
