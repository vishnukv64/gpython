"""The with statement: the context manager protocol.

Run with:  /tmp/gpy examples/python/basics/with_statement.py

This interpreter supports the with statement, single and multiple context
managers, the __enter__/__exit__ protocol, and contextlib.suppress /
contextlib.contextmanager.
"""

print("--- the simplest case: files ---")
# open() is a context manager: the file is closed on leaving the block.
with open("/tmp/gpy_with_demo.txt", "w") as f:
    f.write("line one\n")
    f.write("line two\n")
print("file written")

with open("/tmp/gpy_with_demo.txt") as f:
    contents = f.read()
print("read back:          ", repr(contents))
print("file has been closed by the with block")

print()
print("--- writing your own context manager ---")


class Resource:
    """Prints when it is entered and left, and optionally suppresses errors."""

    def __init__(self, name, swallow=False):
        self.name = name
        self.swallow = swallow

    def __enter__(self):
        print("  >>> acquiring %s" % self.name)
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        print("  <<< releasing %s" % self.name)
        if exc_type is not None:
            print("      an exception passed through: %s" % exc_type)
        return self.swallow             # True swallows the exception


with Resource("database") as res:
    print("  working inside the block; res.name =", res.name)
print("after the block")

print()
print("--- __enter__ can return any object ---")


class ReturnsValue:
    def __enter__(self):
        return {"answer": 42}

    def __exit__(self, exc_type, exc_value, traceback):
        return False


with ReturnsValue() as data:
    print("the value bound by `as`:", data)
    print("its 'answer':          ", data["answer"])

print()
print("--- an exception propagates unless __exit__ returns True ---")
try:
    with Resource("transaction"):
        raise ValueError("something went wrong")
except ValueError as err:
    print("the exception escaped and was caught here:", err)

print()
print("returning True from __exit__ swallows the exception:")
with Resource("lenient", swallow=True):
    raise ValueError("this will be swallowed")
print("execution continued past the with block")

print()
print("--- multiple context managers in one with ---")
with Resource("first") as a, Resource("second") as b:
    print("  inside both blocks")
print("exiting happens in reverse order")

print()
print("--- nested with is equivalent ---")
with Resource("outer"):
    with Resource("inner"):
        print("  innermost")
print("nested form left the same way")

print()
print("--- contextlib.suppress ---")
import contextlib

with contextlib.suppress(ValueError):
    print("about to raise, but suppress() will absorb it")
    raise ValueError("ignored")
print("carried on after the suppressed exception")

# An exception of a different type is NOT suppressed.
try:
    with contextlib.suppress(ValueError):
        raise KeyError("different type")
except KeyError as err:
    print("a KeyError was not suppressed:", err)

print()
print("--- contextlib.contextmanager: build one from a generator ---")


@contextlib.contextmanager
def section(title):
    print("  === %s ===" % title)
    try:
        yield title.upper()          # the value bound by `as`
    finally:
        print("  --- end of %s ---" % title)


with section("heading") as name:
    print("  the yielded value is:", name)

print()
print("--- a worked example: timing a block ---")
import time


@contextlib.contextmanager
def timed(label):
    start = time.time()
    yield
    elapsed = time.time() - start
    print("  %s took %.6f seconds" % (label, elapsed))


with timed("a short loop"):
    total = 0
    for i in range(1000):
        total += i
print("  total =", total)

print()
print("--- a worked example: opening and closing a file in one line ---")


class TempFile:
    def __init__(self, path):
        self.path = path

    def __enter__(self):
        self.handle = open(self.path, "w")
        return self.handle

    def __exit__(self, exc_type, exc_value, traceback):
        self.handle.close()
        return False                    # never swallow


with TempFile("/tmp/gpy_with_demo2.txt") as handle:
    handle.write("written through a custom context manager\n")
with open("/tmp/gpy_with_demo2.txt") as handle:
    print("verified contents:", handle.read().strip())

print()
print("--- cleanup runs even when an exception is pending ---")
log = []


class Audit:
    def __enter__(self):
        log.append("enter")
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        log.append("exit")
        return False


try:
    with Audit():
        log.append("body")
        raise RuntimeError("boom")
except RuntimeError:
    log.append("caught")
print("log order:          ", log)
