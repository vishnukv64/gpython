import re


class A:
    def __repr__(self):
        return "A!"


class B(A):
    def __str__(self):
        return "B-str"


def addr_free(s):
    return re.sub(r"0x[0-9a-f]+", "0xADDR", s)


for o in (3, "x", [1, 2], {"k": 1}, None, 1.5, (1,), A(), B()):
    # object.__str__(x) is repr(x) for every x, whatever x's own __str__ does.
    print(type(o).__name__, addr_free(object.__str__(o)) == addr_free(repr(o)))
print(object.__str__(B()), object.__str__(ValueError("a")))
print(str(B()), str(A()), str(ValueError("a")))
print(object.__str__.__name__ if hasattr(object.__str__, "__name__") else "__str__")
