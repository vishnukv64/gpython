"""dataclasses: classes that generate __init__, __repr__ and __eq__.

Run with:  /tmp/gpy examples/stdlib/dataclasses_demo.py

Decorating a class with @dataclass fills in the boilerplate. field() gives
defaults and per-field options; asdict()/astuple()/fields()/replace() inspect
and copy instances.

Interpreter note: dataclasses prints a "*** FIXME need to get the current vm
globals somehow" line to stderr the first time it runs. That is noise from the
implementation, not an error -- the program still exits 0.
"""

import dataclasses
from dataclasses import dataclass, field

print("--- a plain dataclass ---")


@dataclass
class Point:
    x: int
    y: int


p = Point(3, 4)
print("constructed:   ", p)
print("repr is auto:  ", repr(p))
print("fields:        ", [f.name for f in dataclasses.fields(p)])
print("attribute keys:", [f.name for f in dataclasses.fields(Point)])

print()
print("--- __eq__ is generated for you ---")
print("Point(3, 4) == Point(3, 4):", Point(3, 4) == Point(3, 4))
print("Point(3, 4) == Point(3, 5):", Point(3, 4) == Point(3, 5))

print()
print("--- default values and field() ---")


@dataclass
class Task:
    title: str
    done: bool = False
    tags: list = field(default_factory=list)


a = Task("write docs")
b = Task("ship it", True)
print("defaults:      ", a)
print("overridden:    ", b)
print("tags is per-instance, not shared:", a.tags is not b.tags)
a.tags.append("urgent")
print("after mutating a.tags:", a)
print("b.tags is untouched:  ", b)

print()
print("--- asdict() and astuple() ---")
print("asdict: ", dataclasses.asdict(b))
print("astuple:", dataclasses.astuple(b))
print("is_dataclass(Task):", dataclasses.is_dataclass(Task))
print("is_dataclass(Task(...)):", dataclasses.is_dataclass(b))
print("is_dataclass(int):", dataclasses.is_dataclass(int))

print()
print("--- replace() makes a modified copy ---")
copy = dataclasses.replace(b, title="ship it later", done=False)
print("original:", b)
print("copy:    ", copy)
print("they are different objects:", copy is not b)

print()
print("--- make_dataclass() at runtime ---")


def make(name, names):
    """Build a dataclass whose fields are all plain strings."""
    specs = []
    for field_name in names:
        specs.append((field_name, str, field(default="")))
    return dataclasses.make_dataclass(name, specs)


Config = make("Config", ["host", "port"])
instance = Config()
print("generated class:  ", instance)
instance.host = "localhost"
instance.port = "8080"
print("after assignment: ", instance)
print("generated fields: ", [f.name for f in dataclasses.fields(Config)])

print()
print("--- a worked example: a tiny inventory ---")


@dataclass
class Item:
    sku: str
    price: float
    quantity: int = 0

    def value(self):
        return self.price * self.quantity


stock = [
    Item("A-1", 2.50, 3),
    Item("B-2", 1.25, 10),
    Item("C-3", 19.99),
]
for item in stock:
    print("  %-8s %-8r price=%5.2f qty=%2d value=%7.2f"
          % (item.sku, dataclasses.is_dataclass(item), item.price, item.quantity, item.value()))
print("total inventory value: %.2f" % (sum([item.value() for item in stock])))

print()
print("--- what the module exposes ---")
for name in ["dataclass", "field", "fields", "asdict", "astuple", "replace",
             "is_dataclass", "make_dataclass", "FrozenInstanceError"]:
    print("  %-20s %s" % (name, hasattr(dataclasses, name)))
print("  KW_ONLY sentinel exposed:", hasattr(dataclasses, "KW_ONLY"))
print("  MISSING sentinel exposed:", hasattr(dataclasses, "MISSING"))
