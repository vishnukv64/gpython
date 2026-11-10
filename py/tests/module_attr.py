"""AttributeError on a module names the module."""

import shutil
import sys, types, os
D = []


def t(f):
    try:
        f()
    except Exception as e:
        # the temp dir becomes <T>; Windows writes '<T>\circ_a.py' (as CPython)
        print(type(e).__name__, str(e).replace(D[0], "<T>").replace("<T>" + os.sep, "<T>/") if D else e)
t(lambda: sys.nope)
t(lambda: os.nope)
import posixpath  # os.path is ntpath on Windows; posixpath exists everywhere
t(lambda: posixpath.nope)
import json
t(lambda: json.nope)
m = types.ModuleType("mm")
t(lambda: m.x)
del m.__name__
t(lambda: m.x)
m2 = types.ModuleType("m2"); m2.__name__ = 5
t(lambda: m2.x)
m3 = types.ModuleType("m3")
def ga(name):
    if name == "ok": return 1
    raise AttributeError(f"custom {name}")
m3.__getattr__ = ga
print(m3.ok)
t(lambda: m3.bad)
m4 = types.ModuleType("m4")
m4.__getattr__ = lambda n: 1/0
t(lambda: m4.q)
m5 = types.ModuleType("m5")
class S: _initializing = True
m5.__spec__ = S()
t(lambda: m5.z)
import tempfile
d = os.path.realpath(tempfile.mkdtemp())
D.append(d)
# Closed explicitly: gpython has no refcounting, and Windows will not remove
# a file that is still open.
with open(d+"/circ_a.py","w") as fh:
    fh.write("import circ_b\n")
with open(d+"/circ_b.py","w") as fh:
    fh.write("import circ_a\nprint(circ_a.nothing)\n")
sys.path.insert(0, d)
t(lambda: __import__("circ_a"))
t(lambda: getattr(sys, "zz"))
print(getattr(sys, "zz", "dflt"), hasattr(sys, "zz"))
try:
    del sys.zz
except Exception as e: print(type(e).__name__, e)
try:
    del m.nothere
except Exception as e: print(type(e).__name__, e)
shutil.rmtree(d)
