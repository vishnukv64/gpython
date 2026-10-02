"""typing: type hints.

Run with:  /tmp/gpy examples/stdlib/typing_demo.py

Type hints are annotations that the interpreter records but does not enforce.
This module supplies the names you write them with: List, Dict, Optional,
Union, Callable, TypeVar, Generic, Protocol, TypedDict, NamedTuple and so on.

Interpreter notes (all verified below):
  * get_type_hints() returns {} -- PEP 563 deferred annotations are not
    resolved back into objects.
  * get_origin() and get_args() return the alias itself, not the (origin, args)
    pair CPython gives: get_origin(List[int]) is "typing.List[int]".
  * A NamedTuple built with the functional form works, but its fields are not
    reachable as attributes (`point.x` raises AttributeError) and it is not a
    tuple instance; index it or use _asdict().
  * A NamedTuple subclass written with the class syntax fails with
    `TypeError: cannot create 'method' instances`.
  * Calling a TypedDict constructor raises `TypeError: cannot create 'TD'
    instances`.
"""

import typing

print("--- the common aliases ---")
print("List[int]:           ", typing.List[int])
print("Dict[str, int]:      ", typing.Dict[str, int])
print("Optional[int]:       ", typing.Optional[int])
print("Union[int, str]:     ", typing.Union[int, str])
print("Tuple[int, ...]:     ", typing.Tuple[int, ...])
print("Callable[[int], str]:", typing.Callable[[int], str])
print("Set[str]:            ", typing.Set[str])
print("FrozenSet[int]:      ", typing.FrozenSet[int])
print("Iterable[int]:       ", typing.Iterable[int])

print()
print("--- qualifying forms ---")
print("Final[int]:          ", typing.Final[int])
print("ClassVar[str]:       ", typing.ClassVar[str])
print("Literal['a', 'b']:   ", typing.Literal["a", "b"])
print("Annotated-ish Self:  ", typing.Self)
print("Any:                 ", typing.Any)
print("TYPE_CHECKING:       ", typing.TYPE_CHECKING, "(always False at runtime)")

print()
print("--- TypeVar declares a generic parameter ---")
T = typing.TypeVar("T")
print("TypeVar('T'):         ", T)
print("a second one:         ", typing.TypeVar("K"))
bound = typing.TypeVar("N", bound=int)
print("with a bound:         ", bound)

print()
print("--- writing hints on a function: they are recorded, not enforced ---")


def scale(value: int, factor: int = 2) -> int:
    return value * factor


print("scale(5):        ", scale(5))
print("scale('ab', 3):  ", scale("ab", 3), "(the str hint is not enforced)")
print("annotations dict:", scale.__annotations__ if hasattr(scale, "__annotations__") else "(absent)")

print()
print("--- get_type_hints() returns nothing here ---")
print("get_type_hints(scale):", typing.get_type_hints(scale))
print("(PEP 563 defers annotations to strings, and this interpreter does not")
print(" resolve them back into typing objects)")

print()
print("--- get_origin and get_args return the alias itself ---")
alias = typing.List[int]
print("the alias:                ", alias)
print("get_origin(List[int]):    ", typing.get_origin(alias))
print("get_args(List[int]):      ", typing.get_args(alias))
print("(CPython returns (list,) and (int,) respectively; compare to the alias)")

print()
print("--- NamedTuple: the functional form works, but as a plain named tuple ---")
Point = typing.NamedTuple("Point", [("x", int), ("y", int)])
point = Point(3, 4)
print("Point(3, 4):    ", point)
print("the type:       ", type(point).__name__)
print("field names:    ", point._fields)
print("as a dict:      ", point._asdict())
print("index access:   ", point[0], point[1])
print()
print("NOTE: the fields are NOT reachable as attributes here -- `point.x` raises")
print("`AttributeError: 'Point' object has no attribute 'x'`. Use indexing or")
print("_asdict() instead. isinstance(point, tuple) is also False:", isinstance(point, tuple))

print()
print("--- NamedTuple: the class syntax does not work ---")
try:
    namespace = {}
    exec(compile("class Coord(typing.NamedTuple):\n    x: int\n    y: int\n", "<demo>", "exec"),
         {"typing": typing})
    print("class syntax produced:", namespace.get("Coord"))
except Exception as err:
    print("class Coord(typing.NamedTuple) -> %s: %s" % (type(err).__name__, err))
    print("(use the functional form above instead)")

print()
print("--- TypedDict declares a dict shape ---")
Movie = typing.TypedDict("Movie", {"title": str, "year": int})
print("TypedDict('Movie', ...):", Movie)
print("is_protocol(Movie):     ", typing.is_protocol(Movie))
try:
    Movie(title="Alien", year=1979)
except TypeError as err:
    print("calling it -> TypeError:", err)
print("a plain dict is what you use:",
      dict(title="Alien", year=1979) if False else {"title": "Alien", "year": 1979})

print()
print("--- Protocol declares a structural interface ---")


class Sized(typing.Protocol):
    def __len__(self):
        ...


print("Sized protocol defined:", Sized)
print("runtime_checkable present:", callable(typing.runtime_checkable))
print("is_protocol(Sized):      ", typing.is_protocol(Sized))

print()
print("--- decorators that are no-ops at runtime ---")
print("overload:           ", callable(typing.overload))
print("final:              ", callable(typing.final))
print("no_type_check:      ", callable(typing.no_type_check))
print("dataclass_transform: ", callable(typing.dataclass_transform))


@typing.no_type_check
def unchecked(value):
    return value


print("a @no_type_check function still runs:", unchecked(1))

print()
print("--- cast() is a runtime no-op that returns its value ---")
print("cast(int, 'a string'):", repr(typing.cast(int, "a string")))
print("it is unchanged, not converted")

print()
print("--- a worked example: a typed record and a renderer ---")
Record = typing.NamedTuple("Record", [("name", str), ("score", int), ("tag", str)])


def summarise(records: typing.List[Record]) -> typing.List[str]:
    """No hint is enforced, but the shape documents the contract.

    Fields are read by index: attribute access on a NamedTuple does not work
    in this interpreter.
    """
    lines = []
    for record in records:
        lines.append("%-8s %3d [%s]" % (record[0], record[1], record[2]))
    return lines


records = [Record("ann", 90, "gold"), Record("bob", 75, "silver"), Record("cat", 60, "bronze")]
for line in summarise(records):
    print(" ", line)
print("total score:", sum([record[1] for record in records]))
print("field names: ", records[0]._fields)
print("first record as a dict:", records[0]._asdict())
