"""traceback: the rendering of an exception and the frames it passed through.

The output is asserted, not just produced: every line here is what CPython 3.14
prints for the same program, so a change to the format shows up as a diff.
"""

import traceback

# A traceback is a chain of frames, oldest first, and __traceback__ is where a
# program finds it.  The file name is this file, and the line numbers are the
# ones below - so the rendered text is stable.
def inner():
    raise ValueError("boom")


def outer():
    inner()


try:
    outer()
except ValueError as e:
    tb = e.__traceback__
    names = []
    while tb is not None:
        names.append(tb.tb_frame.f_code.co_name)
        tb = tb.tb_next
    print("frame order:", names)
    print("has tb_lineno:", all(isinstance(x, int) for x in [e.__traceback__.tb_lineno]))
    print("tb_next of innermost is None:", e.__traceback__.tb_next.tb_next.tb_next is None)

# Only the exception NAME and MESSAGE, with no traceback.
try:
    raise TypeError("t")
except TypeError as e:
    print("only:", "".join(traceback.format_exception_only(e)).strip())

# An exception with no arguments prints as just its name, with no colon.
try:
    raise ValueError()
except ValueError as e:
    print("empty:", "".join(traceback.format_exception_only(e)).strip())

# A program's own exception class.
class MyErr(Exception):
    pass


try:
    raise MyErr("custom")
except MyErr as e:
    print("custom:", "".join(traceback.format_exception_only(e)).strip())

# The exception state belongs to the HANDLER, so it is gone once the block ends.
import sys

try:
    raise KeyError("gone")
except KeyError:
    print("in handler:", sys.exc_info()[0].__name__)
print("after handler:", sys.exc_info()[0])

# KeyError reports the REPR of its key, which is what tells a string key apart
# from a non-string one.
print("KeyError str:", repr(str(KeyError("k"))))
print("IndexError str:", repr(str(IndexError("i"))))

# format_exc with nothing being handled.
print("format_exc outside:", repr(traceback.format_exc()))

# The traceback module can walk one.
try:
    outer()
except ValueError as e:
    frames = traceback.extract_tb(e.__traceback__)
    print("extract_tb names:", [f.name for f in frames])
    print("extract_tb file is this file:", frames[0].filename.endswith("test.py"))
    walked = list(traceback.walk_tb(e.__traceback__))
    print("walk_tb count:", len(walked))
    print("walk_tb pairs are (frame, lineno):", all(len(p) == 2 for p in walked))

# The test harness requires this last, as the marker that the script ran to the
# end rather than dying part way through.
doc = "finished"
