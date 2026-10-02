"""Annotations: PEP 526 variable annotations and PEP 563 deferred annotations.

Run with:  /tmp/gpy examples/python/basics/annotations.py

This is a 3.4-era parser with two modern annotation features bolted on:
  * PEP 526 annotated assignments:  x: int = 5, and bare declarations x: int.
  * PEP 563 deferred evaluation: `from __future__ import annotations` makes
    function annotations strings, so forward references to names that do not
    exist yet (or ever) are fine.
Annotations are stored in __annotations__ but are NOT enforced: this
interpreter does not type-check anything.
"""

# Must be the first statement after the docstring in CPython; this
# interpreter accepts it here too. It makes annotations unevaluated strings.
from __future__ import annotations

print("--- annotated assignments (PEP 526) ---")
count: int = 10
ratio: float = 0.5
label: str = "widgets"
flag: bool = True
items: list = [1, 2, 3]
lookup: dict = {"a": 1}

print("count:  ", count)
print("ratio:  ", ratio)
print("label:  ", label)
print("flag:   ", flag)
print("items:  ", items)
print("lookup: ", lookup)

print()
print("--- a bare declaration has no value (and no runtime effect) ---")
pending: int
print("the name 'pending' is not even bound:", "pending" in dir())
pending = 5
print("after assigning it:                  ", pending)

print()
print("--- annotations are not enforced ---")
wrong: int = "this is a string, not an int"
print("annotated int, holds a str:", wrong)
number: int = "123"
print("still a str, not coerced:  ", type(number))

print()
print("--- annotations on class attributes ---")


class Configuration:
    name: str = "default"
    retries: int = 3
    verbose: bool = False          # a class attribute WITH a value
    cache: dict                   # declared only, never assigned

    def __init__(self, name):
        self.name = name
        self.timeout: int = 30     # an annotated instance attribute


cfg = Configuration("service")
print("class attr with value:  ", Configuration.retries)
print("instance override:      ", cfg.name)
print("annotated instance attr:", cfg.timeout)
print("declared-only class attr is absent:", hasattr(Configuration, "cache"))

print()
print("--- function annotations ---")


def area(width: float, height: float) -> float:
    """Both parameter and return annotations."""
    return width * height


print("result:            ", area(3.0, 4.0))
print("__annotations__:   ", area.__annotations__)
print("parameter names:   ", area.__code__.co_varnames if False else "(introspection limited here)")

print()
print("--- local variable annotations inside a function ---")


def process(values):
    total: int = 0
    running: list = []
    for v in values:
        total += v
        running.append(total)
    return total, running


print("process([1,2,3]):  ", process([1, 2, 3]))

print()
print("--- forward references need PEP 563 ---")
# Without the __future__ import, an annotation naming a class defined LATER in
# the file would fail at import time, because annotations are evaluated.
# With it, annotations are kept as unevaluated strings.

print("this module imported `from __future__ import annotations`; annotations on")
print("the functions above were never evaluated, so a name that does not exist")
print("in the annotations is harmless:")
print("  area.__annotations__ is stored but not resolved -> no NameError")


def make_thing() -> "SomethingDefinedMuchLater":
    return None


print("forward-referencing return annotation:", make_thing())


class SomethingDefinedMuchLater:
    pass


print("the class exists now:", SomethingDefinedMuchLater.__name__)

print()
print("--- what is NOT supported ---")
for label, code in [
    ("walrus :=", "(n := 5)"),
    ("PEP 695 `type X = int`", "type Alias = int"),
    ("PEP 695 generic `class C[T]`", "class C[T]: pass"),
]:
    try:
        exec(compile(code, "<probe>", "exec"))
        print("%-28s works" % label)
    except SyntaxError:
        print("%-28s SyntaxError (unsupported)" % label)
    except Exception as err:
        print("%-28s %s: %s" % (label, type(err), err))

print()
print("--- annotations on tuples and nested targets ---")
point: tuple = (1, 2)
print("annotated tuple:", point)
a: int = 1
b: str = "two"
print("multiple annotated names:", a, b)
