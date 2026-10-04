class A:
    pass

a = A()
a.__dict__ = {"x": 1}
print("x:", a.x, "dict:", a.__dict__)

# Reading it back gives the live namespace.
d = a.__dict__
d["y"] = 2
print("y:", a.y)

# An ordinary key named __dict__ on a class is different.
class B:
    pass
b = B()
print("empty:", b.__dict__)

# A dataclass copy, the way rich does it.
from dataclasses import dataclass


@dataclass
class O:
    size: object
    n: int
    extra: object = None


o = O("s", 3)
c = O.__new__(O)
c.__dict__ = o.__dict__.copy()
print("copy:", c.size, c.n, c.extra)
c.n = 9
print("independent:", o.n, c.n)

doc = "finished"
