"""PEP 649/749: annotations are evaluated lazily, on first use."""
import annotationlib
import typing
from annotationlib import Format, get_annotations


def show(label, f):
    try:
        print(label, "->", f())
    except BaseException as e:
        print(label, "->", type(e).__name__, e)


def f(x: Undefined, y: int = 3, *a: str, k: "z" = 1, **kw: float) -> Result: pass


print("defined without evaluating Undefined/Result")
refs = get_annotations(f, format=Format.FORWARDREF)
print({k: (v.__forward_arg__ if isinstance(v, annotationlib.ForwardRef) else v) for k, v in refs.items()})
print(get_annotations(f, format=Format.STRING))
show("VALUE", lambda: f.__annotations__)
show("VALUE via get_annotations", lambda: get_annotations(f))
Undefined = int
Result = str
z = bytes
print(f.__annotations__)
print(get_annotations(f, format=Format.VALUE))
print(f.__annotations__ is f.__annotations__)
print(callable(f.__annotate__), f.__annotate__(1) == f.__annotations__)
show("annotate(5)", lambda: f.__annotate__(5))


def g(x: int): pass


print(g.__annotations__)


def h(): pass


print(h.__annotations__, h.__annotate__)

calls = []


def tracked():
    calls.append(1)
    return int


def lazy(x: tracked()): pass


print("calls after def:", calls)
print(lazy.__annotations__, calls)
print(lazy.__annotations__, calls)


class C:
    Inner = int

    def k(self, q: Inner, r: "C") -> Inner: pass

    def m(self, other: C) -> None: pass


print(C.k.__annotations__)
show("class self reference", lambda: C.m.__annotations__)
print(get_annotations(C.m, format=Format.FORWARDREF)["other"] is C or "forwardref")

f.__annotations__ = {"a": 1}
print(f.__annotations__)
f.__annotate__ = lambda fmt: {"b": 2}
print(f.__annotations__ if False else f.__annotate__(1))


def deco(fn):
    return fn


@deco
def dec(x: later): pass


later = float
print(dec.__annotations__)
print(typing.get_type_hints(dec))
