"""match statements: PEP 634 structural pattern matching.

Run with:  /tmp/gpy examples/python/basics/match_stmt.py

Two things to know about this interpreter's match support:
  1. `match` is recognised only when it is NOT the very first statement of a
     module body (a docstring first makes it work; a leading comment does not).
     This file therefore opens with a docstring.
  2. Guards (`case X if cond:`) parse but crash at run time with
     SystemError: Unknown ast node *ast.MatchGuard. Guards are NOT used here;
     the equivalent is done with an if inside the case body.
"""

print("--- matching literal values ---")


def classify(value):
    match value:
        case 0:
            return "zero"
        case 1:
            return "one"
        case 2:
            return "two"
        case _:
            return "many"


for n in [0, 1, 2, 7]:
    print("classify(%s) ->" % n, classify(n))

print()
print("--- matching strings and None ---")


def describe(value):
    match value:
        case None:
            return "nothing"
        case "":
            return "empty string"
        case "yes":
            return "affirmative"
        case _:
            return "something else"


for v in [None, "", "yes", "no", 0]:
    print("describe(%r) ->" % (v,), describe(v))

print()
print("--- matching several values with | ---")


def weekday_kind(day):
    match day:
        case "sat" | "sun":
            return "weekend"
        case "mon" | "tue" | "wed" | "thu" | "fri":
            return "weekday"
        case _:
            return "unknown"


for day in ["mon", "sat", "sun", "x"]:
    print("%-4s -> %s" % (day, weekday_kind(day)))

print()
print("--- capture patterns bind a name ---")


def unpack(value):
    match value:
        case 0:
            return "exactly zero"
        case other:
            return "captured %r" % (other,)


print("unpack(0):   ", unpack(0))
print("unpack(99):  ", unpack(99))
print("unpack('hi'):", unpack("hi"))

print()
print("--- sequence patterns ---")
# NOTE: the empty-sequence pattern `case []:` crashes this interpreter's
# parser with SystemError: 'interface conversion: interface {} is nil, not
# []ast.Expr'. Match a length instead. See KNOWN_ISSUES.md.


def shape(value):
    match value:
        case [x]:
            return "one element: %s" % x
        case [x, y]:
            return "two elements: %s, %s" % (x, y)
        case [x, *rest]:
            return "first %s, then %s more" % (x, len(rest))
        case _:
            if isinstance(value, list) and len(value) == 0:
                return "empty sequence (via the fallback case)"
            return "not a sequence"


for v in [[], [1], [1, 2], [1, 2, 3, 4], "ab"]:
    print("shape(%r) ->" % (v,), shape(v))

print()
print("--- mapping patterns ---")


def read_point(value):
    match value:
        case {"x": x, "y": y}:
            return "point at %s, %s" % (x, y)
        case {"x": x}:
            return "on the x axis at %s" % x
        case {}:
            return "empty mapping"
        case _:
            return "not a mapping"


for v in [{"x": 1, "y": 2}, {"x": 5}, {}, 42]:
    print("read_point(%r) ->" % (v,), read_point(v))

print()
print("--- class patterns with `as` ---")


def type_of(value):
    match value:
        case int() as n:
            return "int, squared is %s" % (n * n)
        case str() as s:
            return "string of length %s" % len(s)
        case list() as items:
            return "list of %s" % len(items)
        case _:
            return "another type"


for v in [4, "hello", [1, 2, 3], 1.5]:
    print("type_of(%r) ->" % (v,), type_of(v))

print()
print("--- tuples in patterns ---")


def point_kind(value):
    match value:
        case (0, 0):
            return "origin"
        case (x, 0):
            return "on the x axis at %s" % x
        case (0, y):
            return "on the y axis at %s" % y
        case (x, y):
            return "at %s, %s" % (x, y)
        case _:
            return "not a 2-tuple"


for v in [(0, 0), (3, 0), (0, 4), (1, 2), (1, 2, 3)]:
    print("point_kind(%r) ->" % (v,), point_kind(v))

print()
print("--- when a case does not match, execution falls through ---")
result = "unchanged"
value = 99
match value:
    case 1:
        result = "one"
    case 2:
        result = "two"
print("value 99 matched nothing, result is still:", result)

print()
print("--- guards are broken here; use an if inside the case body ---")


def sign(value):
    match value:
        case 0:
            return "zero"
        case n:
            # A guard (`case n if n < 0:`) crashes in this interpreter, so
            # the condition is tested inside the body instead.
            if n < 0:
                return "negative"
            return "positive"


for v in [-5, 0, 5]:
    print("sign(%s) ->" % v, sign(v))

print()
print("--- matching with no case at all is legal ---")
match 42:
    case 1:
        print("one")
print("no case matched 42 and nothing happened")

print()
print("--- a worked example: a tiny command interpreter ---")


def dispatch(command):
    match command.split():
        case ["quit"]:
            return "goodbye"
        case ["add", a, b]:
            return "%s + %s = %s" % (a, b, int(a) + int(b))
        case ["echo", *words]:
            return " ".join(words)
        case [single]:
            return "unknown command: %s" % single
        case _:
            return "cannot parse: %s" % command


for cmd in ["quit", "add 2 3", "echo hello there world", "help", "a b c d"]:
    print("%-22s -> %s" % (cmd, dispatch(cmd)))
