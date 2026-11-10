"""sys.breakpointhook, breakpoint(), PYTHONBREAKPOINT, displayhook and
excepthook, and os.get_terminal_size."""

import builtins
import io
import os
import sys
import types
import warnings

print("displayhook same:", sys.displayhook is sys.__displayhook__)
print("excepthook same:", sys.excepthook is sys.__excepthook__)
print("breakpointhook same:", sys.breakpointhook is sys.__breakpointhook__)
print(sys.breakpointhook.__name__)
print(callable(builtins.breakpoint), builtins.breakpoint.__name__)

calls = []
mod = types.ModuleType("hookmod")
mod.hook = lambda *a, **k: calls.append((a, k)) or "ret"
sys.modules["hookmod"] = mod
pkg = types.ModuleType("hookpkg")
pkg.__path__ = []
sub = types.ModuleType("hookpkg.sub")
sub.fn = lambda *a, **k: calls.append(("sub", a, k)) or 7
pkg.sub = sub
sys.modules["hookpkg"] = pkg
sys.modules["hookpkg.sub"] = sub
builtins.my_builtin_hook = lambda *a, **k: calls.append(("builtin", a, k)) or "b"


def run(env, *a, **k):
    if env is None:
        os.environ.pop("PYTHONBREAKPOINT", None)
    else:
        os.environ["PYTHONBREAKPOINT"] = env
    with warnings.catch_warnings(record=True) as w:
        warnings.simplefilter("always")
        try:
            r = breakpoint(*a, **k)
        except BaseException as e:
            r = "%s: %s" % (type(e).__name__, e)
    print(repr(env), "->", repr(r), [str(x.message) for x in w],
          [x.category.__name__ for x in w], calls[:])
    calls.clear()


run("0")
run("hookmod.hook", 1, 2, x=3)
run("hookpkg.sub.fn", "a")
run("my_builtin_hook", "b", k=1)
run("nosuchmod.thing")
run("hookmod.nothing")
run("hookmod.")
run(".hook")
run("print_nothing_here")

# the direct hook agrees with breakpoint()
os.environ["PYTHONBREAKPOINT"] = "hookmod.hook"
print(sys.breakpointhook(9, y=1), calls)
calls.clear()

# breakpoint() looks the hook up on sys each call
sys.breakpointhook = lambda *a, **k: ("replaced", a, k)
print(breakpoint(1, z=2))
sys.breakpointhook = sys.__breakpointhook__
del sys.breakpointhook
try:
    breakpoint()
except RuntimeError as e:
    print("RuntimeError:", e)
sys.breakpointhook = sys.__breakpointhook__
os.environ.pop("PYTHONBREAKPOINT")

# displayhook
out = io.StringIO()
old = sys.stdout
sys.stdout = out
try:
    builtins._ = "keep"
    sys.displayhook(None)
    kept = builtins._
    sys.displayhook([1, "a"])
    shown = builtins._
    sys.displayhook(0)
finally:
    sys.stdout = old
print(repr(out.getvalue()), kept, shown, builtins._)

# excepthook
err = io.StringIO()
old = sys.stderr
sys.stderr = err
try:
    try:
        1 / 0
    except ZeroDivisionError as e:
        sys.excepthook(type(e), e, e.__traceback__)
finally:
    sys.stderr = old
print(err.getvalue().splitlines()[-1])
try:
    sys.excepthook(1)
except TypeError as e:
    print("TypeError:", e)

# terminal size
ts = os.terminal_size((80, 24))
print(ts, tuple(ts), ts.columns, ts.lines, len(ts), type(ts).__module__, type(ts).__name__)
print(isinstance(ts, tuple), ts == (80, 24), ts[1])
try:
    os.terminal_size((1,))
except TypeError as e:
    print("TypeError:", e)
r, w = os.pipe()
for fd in (r, 987654):
    try:
        os.get_terminal_size(fd)
    except OSError as e:
        print(type(e).__name__, e.errno in (25, 19, 9), e.errno == 9 if fd == 987654 else "")
try:
    os.get_terminal_size("x")
except TypeError as e:
    print("TypeError:", e)
os.close(r)
os.close(w)
