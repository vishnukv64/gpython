# Source lines under each frame.  CPython 3.13+ adds "~~~^^^" caret lines under
# calls and operators, which gpython cannot print (the compiler keeps no end
# columns), so the multi-frame output here is checked through extract_tb, and
# the formatted text only for a single `raise` statement, which has no carets.
import io
import sys
import traceback

HERE = __file__


def norm(text):
    return text.replace(HERE, "FILE")


def inner():
    raise ValueError("boom")


def outer():
    inner()


def rec(n):
    if n:
        rec(n - 1)
    else:
        raise KeyError("deep")


try:
    raise ValueError("top")
except ValueError:
    print(norm(traceback.format_exc()), end="")
    buf = io.StringIO()
    traceback.print_exc(file=buf)
    print(norm(buf.getvalue()), end="")

try:
    outer()
except ValueError as e:
    for fs in traceback.extract_tb(e.__traceback__):
        print(fs.name, fs.lineno, repr(fs.line))

try:
    rec(3)
except KeyError as e:
    for fs in traceback.extract_tb(e.__traceback__):
        print(fs.name, fs.lineno, repr(fs.line))

try:
    exec(compile("x = 1\nraise ValueError('in string')", "<string>", "exec"))
except ValueError as e:
    for fs in traceback.extract_tb(e.__traceback__):
        print(fs.name, fs.lineno, repr(fs.line))
