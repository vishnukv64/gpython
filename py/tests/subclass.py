"""An instance of a Python subclass of a builtin carries its value.

Every expectation here is CPython 3.14's.
"""


class L(list):
    pass


class T(tuple):
    pass


class S(set):
    pass


class D(dict):
    pass


# Construction, and the container protocols reading the carried value.
print("L:", L([1, 2]), L())
print("T:", T((1, 2)), T())
print("S:", sorted(S({3, 1})), len(S()))
print("D:", D({"a": 1}), D())

l = L([1, 2, 3])
l.append(4)
l[0] = 9
print("mutated:", l, len(l))
print("sliced:", l[1:3], "negative:", l[-1])
print("membership:", 9 in l, 99 in l)
print("iterated:", [x for x in l])
print("equality:", L([1, 2]) == L([1, 2]), L([1]) == T((1,)))

# A tuple subclass hashes exactly as its tuple does, so the two key the same
# entry.  "() + ('a',)" is the shape that PANICKED the process while the
# instance representation was being built: M__add__ wrote the second operand
# at the wrong offset and left a nil element.
print("concat:", T(("a",)) + ("b",), () + ("b",), ("a",) + ())
print("hash agrees:", hash(T((1, 2))) == hash((1, 2)))
print("as key:", {(1, 2): "v"}[T((1, 2))])
print("tuple hash works:", isinstance(hash((1, 2)), int))

# Truthiness follows the container: an empty subclass instance is falsey.
print("truthiness:", bool(T()), bool(T((1,))), bool(L()), bool(L([0])))

# __repr__/__str__ of the class win over the carried value's.
class R(tuple):
    def __repr__(self):
        return "R!" + ".".join(self)


print("override wins:", repr(R(("x",))))

# A subclass whose base is not a container is unaffected.
class Plain:
    pass


print("plain instance:", type(Plain()).__name__, bool(Plain()))

doc = "finished"

doc = "finished"
