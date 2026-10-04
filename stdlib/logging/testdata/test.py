"""logging.Filter, including pip's own use of it.

Every expectation here is CPython 3.14's.
"""

import logging
import io
from logging import Filter

rec = lambda name: logging.LogRecord(name, 40, "", 0, "m", None, None)

# The base Filter admits a record from the named logger OR ANY OF ITS CHILDREN.
# The child rule is the whole point - "Filter('a')" passes "a.b".
print("own logger:", Filter("a").filter(rec("a")))
print("child:", Filter("a").filter(rec("a.b")))
print("grandchild:", Filter("a").filter(rec("a.b.c")))
print("unrelated:", Filter("a").filter(rec("ab")))
print("other:", Filter("a").filter(rec("b")))
print("empty name admits all:", Filter().filter(rec("anything")))
print("name attribute:", Filter("x").name)

# pip's ExcludeLoggerFilter calls super().filter.  A subclass of a NATIVE type
# keeps its override on the class, and calling it DIRECTLY works - so the
# behaviour pip depends on is available, and this pins it.
# A subclass of a NATIVE type is a KNOWN LIMITATION: the instance does not know
# its own subclass, so "type(n)" reports logging.Filter, __init__ never runs,
# instance attributes are not stored, and a bound call reaches the native base
# method rather than the override.  Only the UNBOUND form works.  This is the
# same root cause as "class L(list)" being refused - see examples/KNOWN_ISSUES.
class Derived(Filter):
    def filter(self, record):
        return "OVERRIDE"

d = Derived("x")
print("unbound override runs:", Derived.filter(d, rec("x")))
# CPython reports "Derived" here, and now so does this interpreter: an instance
# of a Python subclass of a native type carries its subclass.  This used to pin
# the OPPOSITE - the native base - as a known limitation, and pinning a bug as
# expected behaviour makes fixing it look like a regression, which is exactly
# how it surfaced.
print("instance type name is the subclass:", type(d).__name__ == "Derived")

# A filter attached to a logger runs before dispatch: a base Filter naming that
# logger PASSES its records, so it writes; naming another drops them.
buf = io.StringIO()
lg = logging.getLogger("filt"); lg.setLevel(logging.DEBUG)
lg.addHandler(logging.StreamHandler(buf))
lg.addFilter(Filter("filt"))
lg.warning("kept")
print("base filter keeps its own:", repr(buf.getvalue()))
lg.addFilter(Filter("someoneelse"))
lg.warning("dropped")
print("a non-matching filter drops:", repr(buf.getvalue()))

doc = "finished"
