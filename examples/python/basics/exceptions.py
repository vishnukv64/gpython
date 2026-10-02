"""Exceptions: try/except/else/finally, raise, and what is broken here.

Run with:  /tmp/gpy examples/python/basics/exceptions.py

IMPORTANT interpreter limitation
--------------------------------
The BUILT-IN exception hierarchy works normally: try/except/else/finally,
raise, bare `raise` to re-raise, `raise X from Y`, exception arguments and
except-as all behave. But USER-DEFINED exception classes do not work at all:

    class MyError(Exception):
        pass
    raise MyError("boom")

fails with TypeError: "'BaseException' object is not callable"
(or, when raised bare, TypeError: "catching 'BaseException' that does not
inherit from BaseException is not allowed").

So this file teaches the exception machinery using the built-in exception
types, and demonstrates the custom-class failure explicitly at the end.
"""

print("--- catching a built-in exception ---")
try:
    value = int("not a number")
except ValueError as err:
    print("caught ValueError:  ", err)
    print("its type:           ", type(err))
    print("its args:           ", err.args)

print()
print("--- except / else / finally, all four clauses ---")


def divide(a, b):
    try:
        result = a / b
    except ZeroDivisionError as err:
        print("  except:  %s" % err)
        return None
    except TypeError as err:
        print("  except:  %s" % err)
        return None
    else:
        print("  else:    division succeeded")
        return result
    finally:
        print("  finally: this always runs")


print("divide(10, 4):")
print("  ->", divide(10, 4))
print("divide(10, 0):")
print("  ->", divide(10, 0))
print("divide('a', 2):")
print("  ->", divide("a", 2))

print()
print("--- catching several types with one clause ---")
for value in ["12", "abc", None]:
    try:
        print("int(%r) ->" % (value,), int(value))
    except (ValueError, TypeError) as err:
        print("int(%r) raised %s: %s" % (value, type(err), err))

print()
print("--- clause order matters: the first match wins ---")
try:
    raise KeyError("missing")
except LookupError as err:
    print("caught as LookupError (KeyError is a subclass):", err)
except KeyError as err:
    print("never reached")
except Exception as err:
    print("never reached either")

print()
print("--- raising exceptions ---")


def validate_age(age):
    if age < 0:
        raise ValueError("age cannot be negative: %s" % age)
    if age > 150:
        raise ValueError("age is implausible: %s" % age)
    return age


for candidate in [30, -1, 200]:
    try:
        print("validate_age(%s) ->" % candidate, validate_age(candidate))
    except ValueError as err:
        print("validate_age(%s) raised:" % candidate, err)

print()
print("--- raising a class rather than an instance ---")
try:
    raise RuntimeError
except RuntimeError as err:
    print("raise RuntimeError ->", repr(err), "args:", err.args)

try:
    raise RuntimeError("with a message")
except RuntimeError as err:
    print("raise RuntimeError('...') ->", err)

print()
print("--- re-raising with a bare raise ---")


def retry_once(operation):
    try:
        return operation()
    except ValueError:
        print("  first attempt failed, re-raising")
        raise                            # re-raises the same exception


try:
    retry_once(lambda: int("nope"))
except ValueError as err:
    print("outer handler saw:", err)

print()
print("--- raise ... from ... (exception chaining) ---")


def load_config():
    # `from None` suppresses the chained context; `from err` records it.
    raise KeyError("config") from None


try:
    load_config()
except KeyError as err:
    print("chained-raise caught:", err)

print()
print("--- the except-as name does not leak out of the block ---")
try:
    raise ValueError("scoped")
except ValueError as scoped:
    print("inside the handler, scoped =", scoped)
print("after the handler, 'scoped' has been deleted:",
      "scoped" in dir())

print()
print("--- the common built-in exceptions ---")
for maker, description in [
    (lambda: 1 / 0, "ZeroDivisionError"),
    (lambda: [1, 2][9], "IndexError"),
    (lambda: {"a": 1}["b"], "KeyError"),
    (lambda: 1 + "x", "TypeError"),
    (lambda: undefined_name_xyz, "NameError"),
    (lambda: (1).no_such_method(), "AttributeError"),
    (lambda: int("x"), "ValueError"),
    (lambda: open("/no/such/path/at/all"), "FileNotFoundError"),
]:
    try:
        maker()
        print("%-20s did not raise" % description)
    except Exception as err:
        print("%-20s -> %s" % (description, type(err)))

print()
print("--- assert ---")


def checked(value):
    assert value > 0, "value must be positive, got %s" % value
    return value


print("checked(5):        ", checked(5))
try:
    checked(-1)
except AssertionError as err:
    print("checked(-1) raised AssertionError:", err)

print()
print("--- finally runs even when returning from inside try ---")


def with_cleanup(log):
    try:
        log.append("try")
        return "returned value"
    finally:
        log.append("finally")


log = []
print("result:            ", with_cleanup(log))
print("log order:         ", log)

print()
print("--- nested try blocks ---")


def nested():
    try:
        try:
            raise ValueError("inner")
        except ValueError as err:
            print("  inner handler:", err)
            raise TypeError("converted")     # a new exception
    except TypeError as err:
        print("  outer handler:", err)


nested()

print()
print("--- catching everything ---")
try:
    raise IndexError("whatever")
except Exception as err:
    print("Exception catches any normal error:", repr(err))
try:
    raise IndexError("whatever")
except BaseException as err:
    print("BaseException catches it too:      ", repr(err))

print()
print("--- the user-defined exception classes that do NOT work ---")


class MyError(Exception):
    """A custom exception type: defined fine, but cannot be constructed."""


print("the name is bound, but to a BaseException instance, not a class: the",
      "class statement did not produce a usable type object")
print("isinstance(MyError, type) ->", isinstance(MyError, type))

# Every attempt to CONSTRUCT the subclass fails.
try:
    err = MyError("boom")
    print("constructing MyError('boom') worked:", err)
except BaseException as exc:
    print("constructing MyError('boom') ->", type(exc), "-", exc)

try:
    err = MyError()
    print("constructing MyError() worked:", err)
except BaseException as exc:
    print("constructing MyError()      ->", type(exc), "-", exc)

# Raising one bare also fails, because catching it is rejected.
try:
    raise MyError
except BaseException as exc:
    print("raise MyError               ->", type(exc), "-", exc)
except Exception as exc:
    print("raise MyError (via Exception) ->", type(exc), "-", exc)

# Workaround used elsewhere in these examples: raise a built-in with a
# message that carries the extra context, and inspect err.args if needed.
try:
    raise ValueError("MyError: boom (code=7)")
except ValueError as err:
    print("workaround, raise ValueError:", err)
    print("the message can encode a code:", err.args[0].split("code=")[1][0])
