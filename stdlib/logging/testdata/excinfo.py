"""exc_info=True attaches the traceback of the exception being handled."""

import logging
import sys


class Sink(logging.Handler):
    """A handler defined in Python, which also checks that a handler subclass
    can hold its own attributes and reports its own class."""

    def __init__(self):
        super().__init__()
        self.lines = []

    def emit(self, record):
        self.lines.append(self.format(record))


sink = Sink()
lg = logging.getLogger("excinfo")
lg.setLevel(logging.DEBUG)
lg.addHandler(sink)

print("handler type:", type(sink).__name__)
print("isinstance:", isinstance(sink, Sink))
print("own attribute kept:", isinstance(sink.lines, list))

lg.warning("no exception")
print("plain line:", repr(sink.lines[-1]))

try:
    raise ValueError("boom")
except ValueError:
    lg.critical("Exception:", exc_info=True)

text = sink.lines[-1]
print("has heading:", text.startswith("Exception:Traceback (most recent call last):"))
print("names the exception:", text.rstrip().endswith("ValueError: boom"))

doc = "finished"
