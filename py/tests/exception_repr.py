"""Exception repr, and the last line of an uncaught traceback."""

import traceback

# repr of an exception is "Name(arg, arg)" - NEVER the trailing comma a
# one-element tuple carries.
print("repr 1arg:", repr(IndexError("whatever")))
print("repr ValueError:", repr(ValueError("v")))
print("repr KeyError:", repr(KeyError("k")))
print("repr TypeError:", repr(TypeError("t")))
print("repr 2args:", repr(Exception("a", "b")))
print("repr noargs:", repr(ValueError()))

# str() is the message, and KeyError is the one whose message is the repr of
# its argument.
print("str 1arg:", str(IndexError("whatever")))
print("str KeyError:", str(KeyError("k")))
print("str KeyError int:", str(KeyError(42)))
print("str 2args:", str(Exception("a", "b")))

# A tuple still reprs with its trailing comma - that is a different thing.
print("tuple 1:", repr(("a",)))
print("tuple 2:", repr(("a", "b")))

# format_exception_only is the "Name: message" line.
try:
    raise IndexError("whatever")
except IndexError as e:
    print("only:", repr(traceback.format_exception_only(type(e), e)[0].strip()))

doc = "finished"
