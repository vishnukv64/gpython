"""enum: enumerations, IntEnum and auto().

Run with:  /tmp/gpy examples/stdlib/enum_demo.py

Supported here: class-style Enum and IntEnum, .name/.value, iteration, lookup
by value, identity comparison, auto(), and the module-level helpers.
Not supported: the functional API `enum.Enum("E", "A B")` and `enum.Flag`
both raise TypeError "cannot create ... instances".
"""

import enum

print("--- a plain Enum ---")


class Colour(enum.Enum):
    RED = 1
    GREEN = 2
    BLUE = 3


print("members:            ", Colour.RED, Colour.GREEN, Colour.BLUE)
print("member types:       ", type(Colour.RED))
print(".name of RED:       ", Colour.RED.name)
print(".value of RED:      ", Colour.RED.value)
print("str() vs repr():    ", str(Colour.RED), "|", repr(Colour.RED))

print()
print("--- iteration, in definition order ---")
for member in Colour:
    print("  %-6s = %s" % (member.name, member.value))
print("names:              ", [m.name for m in Colour])
print("values:             ", [m.value for m in Colour])

print()
print("--- lookup by value ---")
print("Colour(2):          ", Colour(2))
print("Colour(2) is GREEN: ", Colour(2) is Colour.GREEN)
print("Colour(3).name:     ", Colour(3).name)

print()
print("--- identity comparison (the point of enums) ---")
print("RED is RED:         ", Colour.RED is Colour.RED)
print("RED is GREEN:       ", Colour.RED is Colour.GREEN)
print("RED == RED:         ", Colour.RED == Colour.RED)

print()
print("--- an IntEnum behaves like an int ---")


class Status(enum.IntEnum):
    OK = 200
    CREATED = 201
    NOT_FOUND = 404
    ERROR = 500


print("Status.OK:          ", Status.OK)
print("its .name/.value:   ", Status.OK.name, Status.OK.value)
print("compares to int:    ", Status.OK == 200, Status.ERROR == 500)
# NOTE: IntEnum members reject both arithmetic and int() here:
#   Status.OK + 1  -> TypeError: unsupported operand type(s) for +
#   int(Status.OK) -> TypeError: unsupported operand type(s) for int
# Use .value instead.
print("via .value:         ", Status.OK.value, Status.NOT_FOUND.value)
for label, code in [("Status.OK + 1", "Status.OK + 1"), ("int(Status.OK)", "int(Status.OK)")]:
    try:
        eval(compile(code, "<probe>", "eval"), {"Status": Status})
        print("%-16s works" % label)
    except TypeError as err:
        print("%-16s TypeError: %s" % (label, err))

# IntEnum members compare to ints, but do NOT sort through the builtin
# sorted() unless a key extracts .value first.
codes = [Status.ERROR, Status.OK, Status.NOT_FOUND, Status.CREATED]
print("sorted by value:    ", [s.name for s in sorted(codes, key=lambda s: s.value)])

print()
print("--- auto() assigns successive values ---")


class Direction(enum.Enum):
    NORTH = enum.auto()
    EAST = enum.auto()
    SOUTH = enum.auto()
    WEST = enum.auto()


print("members:            ", [(d.name, d.value) for d in Direction])

print()
print("--- enums with non-integer values ---")


class HttpMethod(enum.Enum):
    GET = "GET"
    POST = "POST"
    PUT = "PUT"
    DELETE = "DELETE"


for method in HttpMethod:
    print("  %-7s -> %r" % (method.name, method.value))

print()
print("--- an alias shares a member (but keeps its own name here) ---")
# Two names sharing one value create an alias. In CPython iteration shows only
# the FIRST name; here all distinct names are enumerated, and an alias keeps
# its own .name while still being the same member object.


class Weekday(enum.Enum):
    MONDAY = 1
    TUESDAY = 2
    WEDNESDAY = 3
    THU = 3            # an alias for WEDNESDAY


print("enumerated members: ", [m.name for m in Weekday])
print("Weekday.THU is WEDNESDAY:", Weekday.THU is Weekday.WEDNESDAY)
print("Weekday.THU.name:   ", Weekday.THU.name)
print("Weekday.THU.value:  ", Weekday.THU.value)

print()
print("--- enum members are NOT hashable here ---")
# dict keys need hashability, so a mapping keyed by enum member fails with
# TypeError: unhashable type: 'enum.member'. Map by .name or .value instead.
try:
    prices = {Colour.RED: 1.50}
    print("enum member as a dict key worked:", prices)
except TypeError as err:
    print("enum member as a dict key raises:", err)

# The supported spelling:
prices = {Colour.RED.name: 1.50, Colour.BLUE.name: 2.25}
for colour in Colour:
    print("  %-6s price: %s" % (colour.name, prices.get(colour.name, "n/a")))

print()
print("--- the module-level helpers ---")
print("enum.Enum:          ", enum.Enum)
print("enum.IntEnum:       ", enum.IntEnum)
print("enum.auto:          ", enum.auto)
print("enum.unique:        ", enum.unique)
print("enum.member:        ", enum.member)
print("enum.nonmember:     ", enum.nonmember)

print()
print("--- the functional API and Flag are NOT supported here ---")
try:
    dynamic = enum.Enum("Dynamic", "A B")
    print("enum.Enum('Dynamic', 'A B') ->", dynamic)
except TypeError as err:
    print("enum.Enum('Dynamic', 'A B') raises TypeError:", err)

try:
    flags = enum.Flag("F", "X Y")
    print("enum.Flag('F', 'X Y') ->", flags)
except TypeError as err:
    print("enum.Flag('F', 'X Y') raises TypeError:", err)

print()
print("--- a worked example: a state machine ---")


class State(enum.Enum):
    IDLE = "idle"
    RUNNING = "running"
    DONE = "done"


# Enum members are unhashable here, so the transition table is keyed by name.
TRANSITIONS = {
    State.IDLE.name: State.RUNNING,
    State.RUNNING.name: State.DONE,
    State.DONE.name: State.IDLE,
}


def advance(state):
    return TRANSITIONS[state.name]


state = State.IDLE
print("starting at:        ", state.value)
for step in range(4):
    state = advance(state)
    print("  step %d -> %s" % (step + 1, state.value))

print()
print("--- a worked example: readable exit codes ---")


class Exit(enum.IntEnum):
    SUCCESS = 0
    USAGE = 2
    NO_INPUT = 66
    SOFTWARE = 70


for code in Exit:
    print("  %-9s = %3d" % (code.name, code.value))
print("the shell would see: ", Exit.NO_INPUT.value, "(int(Exit.NO_INPUT) is not supported)")
print("describe by value:   ", Exit(70).name)
