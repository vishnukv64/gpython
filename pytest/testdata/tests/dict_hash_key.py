# Keys whose type defines __hash__ and __eq__.
#
# Such a key is stored in a dict's table, not in the string encoding, because
# only the table can tell two objects sharing a hash apart.  These assertions
# are the regression tests for the two bugs that followed from encoding the
# pair (type name, hash) and nothing else: decode raised "corrupt dict key" for
# every accessor, and a colliding hash silently overwrote an unrelated entry.

doc = "dict key with __hash__"


class K:
    def __init__(self, v):
        self.v = v

    def __hash__(self):
        return hash(self.v)

    def __eq__(self, o):
        return isinstance(o, K) and self.v == o.v

    def __repr__(self):
        return "K(%d)" % self.v


d = {K(1): "a", K(2): "b"}
assert len(d) == 2
assert d[K(1)] == "a"
assert d[K(2)] == "b"

# Every accessor decodes the stored key; these raised "corrupt dict key".
#
# Containers of user objects are compared by repr, not by ==: comparing two
# lists or two tuples of them is a SEPARATE pre-existing gap - Eq does not
# dispatch a Python __eq__ for a class instance, so "[K(1)] == [K(1)]" is False
# at baseline too, for every type.  That is not this bug and is not touched
# here.
assert repr(list(d.keys())) == "[K(1), K(2)]"
assert repr(list(d.items())) == "[(K(1), 'a'), (K(2), 'b')]"
assert repr(sorted(d, key=lambda k: k.v)) == "[K(1), K(2)]"
assert repr(d) == "{K(1): 'a', K(2): 'b'}"
assert d == {K(2): "b", K(1): "a"}

# Lookup by an equal but distinct object still finds the entry.
assert K(1) in d
assert K(3) not in d

# dict(d) re-files the key rather than copying a code from the other table.
e = dict(d)
assert len(e) == 2
assert repr(list(e.keys())) == "[K(1), K(2)]"

# Re-assigning keeps one entry; deleting removes it.
d[K(1)] = "z"
assert len(d) == 2 and d[K(1)] == "z"
d.pop(K(1))
assert len(d) == 1
assert repr(d) == "{K(2): 'b'}"

# popitem decodes the key before dropping it.
f = {K(7): "x"}
item = f.popitem()
assert repr(item) == "(K(7), 'x')"
assert len(f) == 0
# "{**d}", "|", "|=", update and copy all re-file the keys.
assert {**e} == e
assert len(e | {K(9): "n"}) == 3
g = {K(3): "c"}
g |= e
assert len(g) == 3
h = {}
h.update(e)
assert len(h) == 2 and h[K(2)] == "b"
assert len(e.copy()) == 2

# A dict comprehension builds keys through the same path.
assert {k.v: v for k, v in e.items()} == {1: "a", 2: "b"}

doc = "colliding hash"


class C:
    def __init__(self, v):
        self.v = v

    def __hash__(self):
        return 7

    def __eq__(self, o):
        return isinstance(o, C) and self.v == o.v

    def __repr__(self):
        return "C(%d)" % self.v


# Equal hashes are a bucket; __eq__ decides.  Two different objects sharing a
# hash were one entry, so the second silently overwrote the first.
c = {C(1): "a", C(2): "b"}
assert len(c) == 2
assert c[C(1)] == "a"
assert c[C(2)] == "b"

# A third object equal to an existing key updates in place, not appends.
c[C(1)] = "z"
assert len(c) == 2 and c[C(1)] == "z"
assert repr(c) == "{C(1): 'z', C(2): 'b'}"

del c[C(2)]
assert len(c) == 1

doc = "set members with __hash__"


class S:
    def __init__(self, v):
        self.v = v

    def __hash__(self):
        return 7

    def __eq__(self, o):
        return isinstance(o, S) and self.v == o.v

    def __repr__(self):
        return "S(%d)" % self.v


s = {S(1), S(2)}
assert len(s) == 2
assert S(1) in s
assert S(3) not in s
# Set repr order is hash order, which Go's map does not reproduce; compare the
# members as a sorted multiset instead.
assert sorted(repr(x) for x in s) == ["S(1)", "S(2)"]
assert s == {S(2), S(1)}

# Set algebra goes through the members, because each set's code for a member
# is private to it.
assert len(s | {S(3)}) == 3
assert len(s & {S(1)}) == 1
assert len(s - {S(1)}) == 1
assert len(s ^ {S(2), S(3)}) == 2
assert s.issubset({S(1), S(2), S(9)})
assert {S(1), S(2), S(9)}.issuperset(s)
assert not s.isdisjoint({S(1)})

s.discard(S(1))
assert len(s) == 1 and S(1) not in s
t = s.copy()
t.add(S(4))
assert len(t) == 2 and len(s) == 1
t.clear()
assert len(t) == 0

# A member equal to an existing one updates in place.
u = {S(1)}
u.add(S(1))
assert len(u) == 1
assert repr(u.pop()) == "S(1)"

doc = "finished"
