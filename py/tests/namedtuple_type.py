"""A namedtuple record reports its own class, and isclass distinguishes them."""
import inspect
from collections import namedtuple


class Point(namedtuple("Point", "x y")):
    @property
    def total(self):
        return self.x + self.y


p = Point(1, 2)
print("type name:", type(p).__name__)
print("isinstance:", isinstance(p, Point))
print("type is Point:", type(p) is Point)
print("fields:", p.x, p.y)
print("own property:", p.total)

print("isclass(Point):", inspect.isclass(Point))
print("isclass(p):", inspect.isclass(p))
print("isclass(int):", inspect.isclass(int))
print("isclass(3):", inspect.isclass(3))
print("isclass(namedtuple):", inspect.isclass(namedtuple))


# A plain class and its instance, the same distinction.
class C:
    pass


print("isclass(C):", inspect.isclass(C), "isclass(C()):", inspect.isclass(C()))

# Inherited namedtuple methods still work through the base.
print("len:", len(p), "index:", p[0], "iter:", list(p))

doc = "finished"
