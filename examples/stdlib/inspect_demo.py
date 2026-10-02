"""inspect: ask questions about live objects.

Run with:  /tmp/gpy examples/stdlib/inspect_demo.py

Covers the type predicates (isfunction, isclass, ismodule, ...), getdoc() and
cleandoc(), currentframe() and the generator checks.

Interpreter note: inspect.signature() raises NotImplementedError -- this
interpreter does not retain the argument binding needed to report a signature.
inspect.getmembers() is not present either, and a Python-level function's
__doc__ is the fixed string "A python function" rather than its real
docstring. See examples/KNOWN_ISSUES.md.
"""

import inspect


def plain_function(a, b=1):
    """A plain function with a docstring."""
    return a + b


class PlainClass:
    """A plain class."""

    attribute = 1

    def method(self):
        return self.attribute


generator_function = None


def make_generator():
    yield 1


module_alias = inspect

print("--- type predicates ---")
cases = [
    ("plain_function", plain_function),
    ("PlainClass", PlainClass),
    ("PlainClass()", PlainClass()),
    ("inspect module", module_alias),
    ("len (builtin)", len),
    ("int (type)", int),
    ("make_generator()", make_generator()),
    ("range(3)", range(3)),
    ("'text'", "text"),
]
for label, value in cases:
    print("  %-16s function=%-5s class=%-5s module=%-5s method=%-5s builtin=%-5s generator=%s"
          % (label,
             inspect.isfunction(value),
             inspect.isclass(value),
             inspect.ismodule(value),
             inspect.ismethod(value),
             inspect.isbuiltin(value),
             inspect.isgenerator(value)))

print()
print("--- isgenerator vs isgeneratorfunction ---")
generated = make_generator()
print("a generator object:      ", inspect.isgenerator(generated))
print("the function that makes one:", inspect.isgeneratorfunction(make_generator))
print("a plain function:        ", inspect.isgeneratorfunction(plain_function))
print("first item from it:      ", next(generated))

print()
print("--- getdoc() reads __doc__ ---")
print("class docstring:   ", repr(inspect.getdoc(PlainClass)))
print("builtin doc length:", len(inspect.getdoc(int)), "characters")
print("builtin doc starts:", repr(inspect.getdoc(int)[:40]))
print("a doc-less object: ", repr(inspect.getdoc(object())))
print()
print("NOTE: a Python-level function does NOT report its own docstring here --")
print("__doc__ is the fixed string 'A python function' for every function:")
print("  plain_function.__doc__  ->", repr(plain_function.__doc__))
print("  (the source docstring is\n%r)" % ("  A plain function with a docstring.",))
print("  a function with no docstring gives the same:", repr(inspect.getdoc(lambda: None)))
print("  classes DO keep their docstring, as shown above.")

print()
print("--- cleandoc() strips indentation from a docstring ---")
raw = "\n    First line.\n\n    Second line, indented further.\n    "
print("raw:      ", repr(raw))
print("cleandoc: ", repr(inspect.cleandoc(raw)))

print()
print("--- currentframe() gives the running frame ---")
frame = inspect.currentframe()
print("a frame object:  ", type(frame).__name__)
print("it is not None:  ", frame is not None)

print()
print("--- a worked example: a self-describing registry ---")


class Registry:
    def __init__(self):
        self.entries = []

    def register(self, obj):
        self.entries.append(obj)
        return self

    def describe(self):
        lines = []
        for obj in self.entries:
            if inspect.isfunction(obj):
                kind = "function"
            elif inspect.isclass(obj):
                kind = "class"
            elif inspect.isbuiltin(obj):
                kind = "builtin"
            else:
                kind = "other"
            doc = inspect.getdoc(obj)
            if doc and not inspect.isfunction(obj):
                summary = inspect.cleandoc(doc).split("\n")[0]
            elif inspect.isfunction(obj):
                summary = "(functions do not report their own docstring here)"
            else:
                summary = "(no docstring)"
            lines.append((kind, getattr(obj, "__name__", repr(obj)), summary))
        return lines


def shipping_cost(weight):
    """Work out a postage price.

    Priced per kilogram, rounded up.
    """
    return weight * 2


class Parcel:
    """A box with a weight."""


registry = Registry().register(shipping_cost).register(Parcel).register(len)
for kind, name, summary in registry.describe():
    print("  %-9s %-12s %s" % (kind, name, summary))

print()
print("--- what is missing ---")
try:
    inspect.signature(plain_function)
except NotImplementedError as err:
    print("inspect.signature() raises NotImplementedError:", err)
print("inspect.getmembers exists:", hasattr(inspect, "getmembers"))
