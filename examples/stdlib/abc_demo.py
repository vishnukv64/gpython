"""abc: abstract base classes.

Run with:  /tmp/gpy examples/stdlib/abc_demo.py

This interpreter implements abc.ABC, abc.ABCMeta, the @abstractmethod /
@abstractclassmethod / @abstractstaticmethod / @abstractproperty decorators
and the get_cache_token / update_abstractmethods helpers. Note that
abstractness is NOT enforced: subclassing an ABC without implementing every
abstract method does not raise TypeError when instantiated.
"""

import abc

print("--- abc exposes ---")
print("ABC:                ", abc.ABC)
print("ABCMeta:            ", abc.ABCMeta)
print("abstractmethod:     ", abc.abstractmethod)
print("abstractproperty:   ", abc.abstractproperty)
print("abstractclassmethod:", abc.abstractclassmethod)
print("abstractstaticmethod:", abc.abstractstaticmethod)

print()
print("--- a classic shape hierarchy ---")


class Shape(abc.ABC):
    """An abstract shape. Subclasses must provide area()."""

    @abc.abstractmethod
    def area(self):
        """Return the area of the shape."""

    @abc.abstractmethod
    def perimeter(self):
        """Return the perimeter of the shape."""

    def describe(self):
        """A concrete method that uses the abstract ones."""
        return "%s with area %.2f and perimeter %.2f" % (
            self.__class__.__name__ if False else type(self).__name__ if False else "shape",
            self.area(),
            self.perimeter(),
        )


class Rectangle(Shape):
    def __init__(self, width, height):
        self.width = width
        self.height = height

    def area(self):
        return self.width * self.height

    def perimeter(self):
        return 2 * (self.width + self.height)

    def __repr__(self):
        return "Rectangle(%s, %s)" % (self.width, self.height)


class Circle(Shape):
    def __init__(self, radius):
        self.radius = radius

    def area(self):
        return 3.141592653589793 * self.radius ** 2

    def perimeter(self):
        return 2 * 3.141592653589793 * self.radius

    def __repr__(self):
        return "Circle(%s)" % self.radius


rect = Rectangle(3, 4)
circ = Circle(1)
print("rect.area():        ", rect.area())
print("rect.perimeter():   ", rect.perimeter())
print("circ.area():        ", "%.4f" % circ.area())
print("isinstance(rect, Shape):", isinstance(rect, Shape))
print("class names:        ", Rectangle.__name__, Circle.__name__)

print()
print("--- abstractness is not enforced here ---")
# In CPython, Shape() would raise TypeError because area/perimeter are
# abstract. This interpreter does not check, so the object is created.
try:
    broken = Shape()
    print("Shape() was constructed anyway -> abstract checking is not enforced")
except TypeError as err:
    print("Shape() raised TypeError as CPython would:", err)

print()
print("--- abstract methods can still be called through super() ---")


class Base(abc.ABC):
    @abc.abstractmethod
    def value(self):
        return "base default"


class Child(Base):
    def value(self):
        return "child overrides: " + Base.value(self)


print("Child().value():    ", Child().value())

print()
print("--- abstractclassmethod and abstractstaticmethod ---")


class Factory(abc.ABC):
    @abc.abstractclassmethod
    def build(cls):
        """Create an instance of the concrete class."""

    @abc.abstractstaticmethod
    def tag():
        """Return a static identifier."""


class ConcreteFactory(Factory):
    def __init__(self, name):
        self.name = name

    @classmethod
    def build(cls):
        return cls("built by the classmethod")

    @staticmethod
    def tag():
        return "concrete"


print("ConcreteFactory.tag():   ", ConcreteFactory.tag())
print("ConcreteFactory.build(): ", ConcreteFactory.build().name)
print("isinstance of Factory:   ", isinstance(ConcreteFactory.build(), Factory))

print()
print("--- abstractproperty ---")


class HasName(abc.ABC):
    @abc.abstractproperty
    def name(self):
        """Every subclass must provide this property."""


class Person(HasName):
    def __init__(self, name):
        self._name = name

    @property
    def name(self):
        return self._name


print("Person('Ada').name: ", Person("Ada").name)

print()
print("--- an interface-style ABC with several implementations ---")


class Serialiser(abc.ABC):
    """Anything that can turn a record into a string."""

    @abc.abstractmethod
    def dump(self, record):
        """Serialise one record."""

    def dump_all(self, records):
        """A concrete method built on the abstract one."""
        return [self.dump(r) for r in records]


class LineSerialiser(Serialiser):
    def dump(self, record):
        # Sorted key=value pairs, joined by commas.
        parts = ["%s=%s" % (k, record[k]) for k in sorted(record.keys())]
        return ",".join(parts)


class BracketedSerialiser(Serialiser):
    def dump(self, record):
        parts = ["%s:%s" % (k, record[k]) for k in sorted(record.keys())]
        return "{" + "; ".join(parts) + "}"


records = [{"b": 2, "a": 1}, {"d": 4, "c": 3}]
for serialiser in [LineSerialiser(), BracketedSerialiser()]:
    name = LineSerialiser.__name__ if False else "serialiser"
    print("  %-22s -> %s" % (serialiser.__class__.__name__ if False else "output",
                             serialiser.dump_all(records)))

print()
print("--- abc.get_cache_token ---")
print("get_cache_token() exists:", hasattr(abc, "get_cache_token"))
if hasattr(abc, "get_cache_token"):
    print("its value is an int:  ", isinstance(abc.get_cache_token(), int))

print()
print("--- a worked example: a checked plugin registry ---")


class Plugin(abc.ABC):
    @abc.abstractmethod
    def name(self):
        """The plugin's identifier."""

    @abc.abstractmethod
    def run(self, payload):
        """Do the plugin's work."""


class UpperPlugin(Plugin):
    def name(self):
        return "upper"

    def run(self, payload):
        return payload.upper()


class ReversePlugin(Plugin):
    def name(self):
        return "reverse"

    def run(self, payload):
        return payload[::-1]


registry = {}
for plugin in [UpperPlugin(), ReversePlugin()]:
    registry[plugin.name()] = plugin

print("registered plugins: ", sorted(registry.keys()))
for key in sorted(registry.keys()):
    plugin = registry[key]
    print("  %-8s('%s') -> %s" % (key, "hello", plugin.run("hello")))
    print("           is a Plugin:", isinstance(plugin, Plugin))
