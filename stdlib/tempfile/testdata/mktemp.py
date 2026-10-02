# tempfile.mktemp returns a name, and does not create it.
import os
import tempfile

p = tempfile.mktemp()
assert isinstance(p, str), p
assert not os.path.exists(p), p

p = tempfile.mktemp(dir="/tmp")
assert p.startswith("/tmp/"), p
assert not os.path.exists(p), p

p = tempfile.mktemp(suffix=".suf")
assert p.endswith(".suf"), p
assert not os.path.exists(p), p

p = tempfile.mktemp(prefix="pfx-")
assert os.path.basename(p).startswith("pfx-"), p
assert not os.path.exists(p), p

# Two calls give two names.
assert tempfile.mktemp() != tempfile.mktemp()

# A bytes component mixed with str raises, as CPython's does.
try:
    tempfile.mktemp(suffix=b".b")
    raise AssertionError("expected TypeError for a bytes suffix")
except TypeError:
    pass

print("OK")
