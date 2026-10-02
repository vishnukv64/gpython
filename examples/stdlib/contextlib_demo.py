"""contextlib: helpers for the `with` statement and cleanup.

Run with:  /tmp/gpy examples/stdlib/contextlib_demo.py

Shows @contextmanager (turn a generator into a context manager), suppress(),
closing(), redirect_stdout() and ExitStack.

Interpreter note: redirect_stdout() exists but does NOT capture output in this
interpreter -- print() still goes to the real stdout. The demo shows what it
actually does rather than pretending it works.
"""

import contextlib
import io

print("--- @contextmanager turns a generator into a context manager ---")


@contextlib.contextmanager
def section(title):
    print("  >> enter", title)
    try:
        yield title.upper()
    finally:
        print("  << exit ", title)


with section("loading") as name:
    print("  inside, name =", name)

print()
print("--- the manager still runs cleanup when the body raises ---")
try:
    with section("failing"):
        raise ValueError("boom")
except ValueError as err:
    print("  caught outside:", err)

print()
print("--- suppress(): swallow specific exceptions ---")
with contextlib.suppress(ValueError):
    raise ValueError("this is ignored")
print("the ValueError was suppressed, execution continued")
with contextlib.suppress(ValueError, KeyError):
    raise KeyError("also ignored")
print("a tuple of exception types works too")
try:
    with contextlib.suppress(ValueError):
        raise TypeError("not suppressed")
except TypeError as err:
    print("an unlisted type still propagates:", err)

print()
print("--- closing(): call .close() on the way out ---")


class Connection:
    def __init__(self, name):
        self.name = name
        self.open = True

    def close(self):
        self.open = False
        print("  closed", self.name)

    def run(self, query):
        if not self.open:
            raise RuntimeError("connection is closed")
        return "results for %r" % (query,)


conn = Connection("db-1")
with contextlib.closing(conn) as live:
    print("  query:", live.run("select 1"))
print("  conn.open after the with:", conn.open)

print()
print("--- nullcontext(): a with block that does nothing special ---")
with contextlib.nullcontext("a value") as value:
    print("  nullcontext yields:", value)

print()
print("--- redirect_stdout() exists but does not capture print() here ---")
buffer = io.StringIO()
with contextlib.redirect_stdout(buffer):
    print("  this line goes to the real stdout, not the buffer")
print("buffer contents after the with:", repr(buffer.getvalue()))
print("(CPython would have captured the line; this interpreter does not)")

print()
print("--- ExitStack: enter a dynamic number of managers ---")


class Resource:
    def __init__(self, name):
        self.name = name

    def __enter__(self):
        print("  open ", self.name)
        return self

    def __exit__(self, *details):
        print("  close", self.name)
        return False  # do not swallow exceptions


with contextlib.ExitStack() as stack:
    for name in ["first", "second", "third"]:
        stack.enter_context(Resource(name))
    print("  all three are open; they close in reverse order")

print()
print("--- what this module offers ---")
for name in ["contextmanager", "closing", "suppress", "nullcontext",
             "redirect_stdout", "redirect_stderr", "ExitStack",
             "ContextDecorator"]:
    print("  %-18s %s" % (name, callable(getattr(contextlib, name, None))))
print("  asynccontextmanager available:", hasattr(contextlib, "asynccontextmanager"))
