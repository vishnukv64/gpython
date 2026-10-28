# A module whose body raises is removed from sys.modules, so importing it again
# runs the body again (CPython behaviour; the tracemalloc decline relies on it).
import sys, os, tempfile

d = tempfile.mkdtemp()
with open(os.path.join(d, "failing_mod_zz.py"), "w") as f:
    f.write("COUNT = 1\nraise ValueError('boom')\n")
sys.path.insert(0, d)
for i in range(2):
    try:
        import failing_mod_zz
    except ValueError as e:
        print("raised", e, "failing_mod_zz" in sys.modules)
