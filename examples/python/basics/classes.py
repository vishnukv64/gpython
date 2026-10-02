"""Classes: inheritance, super(), properties, static/class methods, dunders.

Run with:  /tmp/gpy examples/python/basics/classes.py

Interpreter differences this file works around
-----------------------------------------------
  * Looking up an INHERITED class attribute through the subclass object
    (`Sub.attr`) fails with AttributeError "'type' has no attribute 'attr'",
    as does calling an inherited static/class method through the subclass
    (`Sub.method()`). Inherited attributes work fine through an INSTANCE.
  * @property works for reading. `@x.setter` is not implemented
    ("'property' has no attribute 'setter'").
  * User classes can subclass Exception/ValueError but every construction of
    one fails, so custom exception classes are unusable (see exceptions.py).
"""

print("--- a first class ---")


class Point:
    """A 2D point."""

    kind = "point"                       # a class attribute, shared

    def __init__(self, x, y):
        self.x = x                       # instance attributes
        self.y = y

    def magnitude(self):
        return (self.x ** 2 + self.y ** 2) ** 0.5

    def __repr__(self):
        return "Point(%s, %s)" % (self.x, self.y)


p = Point(3, 4)
print("instance:          ", p)
print("repr():            ", repr(p))
print("instance attrs:    ", p.x, p.y)
print("method call:       ", p.magnitude())
print("shared class attr: ", p.kind, Point.kind)
# NOTE: printing the CLASS OBJECT itself (or type(p)) is unsafe whenever the
# class defines a __repr__ that reads instance attributes: the interpreter
# then calls __repr__ with the type bound as `self` and raises
# AttributeError: 'type' has no attribute 'x'. Use the class __name__ and
# isinstance() instead of printing the class object.
print("isinstance(p, Point):", isinstance(p, Point))
print("class __name__:      ", Point.__name__)
print("type(p) is Point:    ", type(p) is Point)

print()
print("--- instance vs class state ---")


class Counter:
    total = 0                            # class-level counter

    def __init__(self, name):
        self.name = name
        self.count = 0                   # per-instance counter

    def bump(self):
        self.count += 1
        Counter.total += 1
        return self.count


a = Counter("a")
b = Counter("b")
print("a.bump():          ", a.bump(), a.bump())
print("b.bump():          ", b.bump())
print("a.count            ", a.count, "(per-instance)")
print("b.count            ", b.count, "(per-instance)")
print("Counter.total      ", Counter.total, "(shared)")

print()
print("--- inheritance and super() ---")


class Animal:
    def __init__(self, name):
        self.name = name

    def speak(self):
        return "..."

    def describe(self):
        return "%s says %s" % (self.name, self.speak())


class Dog(Animal):
    def __init__(self, name, breed):
        super().__init__(name)           # call the base initialiser
        self.breed = breed

    def speak(self):
        return "Woof"

    def describe(self):
        # super() reaches the base method; this interpreter supports it.
        return super().describe() + " (a %s)" % self.breed


class Puppy(Dog):
    def speak(self):
        return "Yip"


d = Dog("Rex", "collie")
print("base method:       ", Animal("thing").speak())
print("overridden:        ", d.speak())
print("super() in __init__:", d.name, d.breed)
print("super() in method: ", d.describe())
print("three levels:      ", Puppy("Bit", "mutt").describe())
print("isinstance:        ", isinstance(d, Dog), isinstance(d, Animal), isinstance(d, Puppy))
# NOTE: issubclass needs a real class object; works for user classes:
print("issubclass:        ", issubclass(Dog, Animal), issubclass(Puppy, Animal))
print("inherited attr via instance:", d.name)

# Inherited CLASS attributes must be read through an instance here:
#   Dog.kind        -> AttributeError
#   Dog("x","y").kind -> works
print("inherited class attr (via instance):", end=" ")
try:
    print(d.kind)
except AttributeError as e:
    print("AttributeError:", e)

print()
print("--- class attributes on the subclass itself ---")


class Vehicle:
    wheels = 4


class Car(Vehicle):
    wheels = 4                           # defined directly, so readable
    doors = 4


print("Car.wheels (own):  ", Car.wheels)
print("Car.doors:         ", Car.doors)
print("Car().wheels:      ", Car().wheels)
print("Vehicle.wheels:    ", Vehicle.wheels)
try:
    print("Vehicle.doors:     ", Vehicle.doors)
except AttributeError as e:
    print("undefined on the base raises AttributeError:", e)

print()
print("--- staticmethod and classmethod ---")


class Temperature:
    @staticmethod
    def celsius_to_fahrenheit(c):
        return c * 9.0 / 5.0 + 32

    @classmethod
    def label(cls):
        return cls.__name__


print("staticmethod:      ", Temperature.celsius_to_fahrenheit(100))
print("classmethod:       ", Temperature.label())
print("class __name__:    ", Temperature.__name__)


class Fahrenheit(Temperature):
    pass


print("inherited via instance:", Fahrenheit().label())
# Calling the inherited classmethod through the subclass object fails here:
#   Fahrenheit.label() -> AttributeError: 'type' has no attribute 'label'
try:
    print("inherited via subclass (unsupported):", Fahrenheit.label())
except AttributeError as e:
    print("inherited via subclass raises AttributeError:", e)

print()
print("--- property (read-only; @x.setter is unimplemented) ---")


class Circle:
    def __init__(self, radius):
        self._radius = radius

    @property
    def radius(self):
        return self._radius

    @property
    def area(self):
        return 3.14159 * self._radius ** 2

    # A plain method is how mutation is exposed, since setters are missing.
    def scale(self, factor):
        self._radius *= factor


c = Circle(2)
print("property read:     ", c.radius, c.area)
print("property is computed, not stored:", c.area)
c.scale(3)
print("after scale(3):    ", c.radius, c.area)
try:
    c.radius = 10
except AttributeError as e:
    print("property setter is missing:", e)

print()
print("--- dunder methods ---")


class Vector:
    def __init__(self, x, y):
        self.x = x
        self.y = y

    def __repr__(self):
        return "Vector(%s, %s)" % (self.x, self.y)

    def __str__(self):
        return "<%s, %s>" % (self.x, self.y)

    def __len__(self):
        return 2

    def __getitem__(self, index):
        # NOTE: this interpreter does not dispatch v[index] to __getitem__;
        # it returns the instance itself instead of calling this method.
        # The method is defined and callable explicitly, but `v[0]` below
        # shows the interpreter's broken behaviour, not this code.
        if index == 0:
            return self.x
        if index == 1:
            return self.y
        raise IndexError("Vector index out of range")

    def __contains__(self, value):
        return value == self.x or value == self.y

    def __iter__(self):
        return iter([self.x, self.y])

    def __eq__(self, other):
        # NOTE: user-defined __eq__ is not consulted by this interpreter's
        # `==` operator; it always compares identity for instances. The
        # method is still callable explicitly, which is what we show here.
        return isinstance(other, Vector) and self.x == other.x and self.y == other.y

    def __hash__(self):
        return hash((self.x, self.y)) if False else self.x * 31 + self.y


v = Vector(3, 4)
print("repr:              ", repr(v))
print("str:               ", str(v))
print("len:               ", len(v))
# v[0] does NOT call __getitem__ here -- it yields the instance again.
print("indexing v[0]:     ", v[0], "(should be 3; __getitem__ not dispatched)")
print("explicit __getitem__:", v.__getitem__(0))
print("membership 3 in v: ", 3 in v, 99 in v)
print("iteration:         ", list(v))
print("explicit __eq__:   ", v.__eq__(Vector(3, 4)), "(method callable directly)")
print("hash:              ", hash(v))
print("identity ==:       ", v == v, v == Vector(3, 4), "(user __eq__ not wired to ==)")

print()
print("--- operator overloading: what works ---")
for label, code in [
    ("v + w (__add__)", "Vector(1,1) + Vector(2,2)"),
    ("v < w (__lt__)", "Vector(1,1) < Vector(2,2)"),
    ("v * 2 (__mul__)", "Vector(1,1) * 2"),
]:
    try:
        eval(compile(code, "<probe>", "eval"), {"Vector": Vector})
        print("%-20s works" % label)
    except TypeError as e:
        print("%-20s TypeError: %s" % (label, e))

print()
print("--- context manager protocol ---")


class Managed:
    def __init__(self, name):
        self.name = name

    def __enter__(self):
        print("  entering", self.name)
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        print("  leaving", self.name, "(exception:", exc_type, ")")
        return False                     # do not swallow exceptions


with Managed("resource") as m:
    print("  inside, m.name =", m.name)

print("after the with block")

print()
print("--- callable instances ---")
# __call__ is registered but the call path for user instances is not wired,
# so show the supported alternative: a plain method.
class Multiplier:
    def __init__(self, factor):
        self.factor = factor

    def apply(self, value):
        return value * self.factor

    def __call__(self, value):
        return value * self.factor


tripler = Multiplier(3)
print("via method:        ", tripler.apply(5))
try:
    print("via __call__:      ", tripler(5))
except TypeError as e:
    print("via __call__ raises TypeError:", e)

print()
print("--- instances hold arbitrary attributes ---")


class Bag:
    pass


bag = Bag()
bag.anything = "value"
bag.number = 42
print("dynamic attributes:", bag.anything, bag.number)
print("hasattr:           ", hasattr(bag, "anything"), hasattr(bag, "missing"))
print("getattr default:   ", getattr(bag, "missing", "fallback"))
setattr(bag, "added", True)
print("setattr:           ", bag.added)

print()
print("--- a worked example: a tiny inventory ---")


class Item:
    def __init__(self, name, price, qty):
        self.name = name
        self.price = price
        self.qty = qty

    @property
    def value(self):
        return self.price * self.qty

    def __repr__(self):
        return "Item(%r, %s, %s)" % (self.name, self.price, self.qty)


class Inventory:
    def __init__(self):
        self.items = []

    def add(self, item):
        self.items.append(item)

    @property
    def total(self):
        return sum(i.value for i in self.items)

    @property
    def count(self):
        return len(self.items)

    def __repr__(self):
        return "Inventory(%d items, total %s)" % (self.count, self.total)


inv = Inventory()
inv.add(Item("widget", 2.50, 4))
inv.add(Item("gadget", 10.00, 2))
print(repr(inv))
for item in inv.items:
    print("  %-8s %5.2f x %d = %6.2f" % (item.name, item.price, item.qty, item.value))
print("grand total:       ", inv.total)
