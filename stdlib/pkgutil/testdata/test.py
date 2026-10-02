import pkgutil
import os
import tempfile

d = tempfile.mkdtemp()
os.mkdir(os.path.join(d, "pkg1"))
open(os.path.join(d, "pkg1", "__init__.py"), "w").write("")
open(os.path.join(d, "mod1.py"), "w").write("x = 1\n")
open(os.path.join(d, "mod2.py"), "w").write("y = 2\n")
open(os.path.join(d, "pkg1", "sub.py"), "w").write("z = 3\n")

# iter_modules lists a directory's modules, sorting packages before modules and
# marking packages with ispkg.
mi = list(pkgutil.iter_modules([d]))
assert [(m.name, m.ispkg) for m in mi] == [
    ("mod1", False), ("mod2", False), ("pkg1", True)], [(m.name, m.ispkg) for m in mi]

# ModuleInfo is the namedtuple CPython documents.
assert pkgutil.ModuleInfo._fields == ("module_finder", "name", "ispkg")
m0 = mi[0]
assert m0.module_finder is not None
assert m0.name == "mod1"
assert m0.ispkg is False

# iter_modules takes a prefix.
assert [m.name for m in pkgutil.iter_modules([d], "pre.")] == ["pre.mod1", "pre.mod2", "pre.pkg1"]

# A bare string path is refused, as CPython refuses it.
try:
    list(pkgutil.iter_modules(d))
except ValueError:
    pass
else:
    raise AssertionError("iter_modules accepted a bare string path")

# walk_packages descends into packages.
import sys
sys.path.insert(0, d)
assert sorted(m.name for m in pkgutil.walk_packages([d])) == [
    "mod1", "mod2", "pkg1", "pkg1.sub"]

# resolve_name resolves a module, an attribute, and a colon-separated pair.
assert pkgutil.resolve_name("os").__name__ == "os"
assert pkgutil.resolve_name("pkgutil").__name__ == "pkgutil"
assert pkgutil.resolve_name("pkgutil:ModuleInfo") is pkgutil.ModuleInfo
assert callable(pkgutil.resolve_name("os:getcwd"))

# A malformed name raises ValueError.
try:
    pkgutil.resolve_name(".:join")
except ValueError:
    pass
else:
    raise AssertionError("resolve_name accepted the malformed name '.:join'")
try:
    pkgutil.resolve_name(".join")
except ValueError:
    pass
else:
    raise AssertionError("resolve_name accepted the malformed name '.join'")

# get_data reads a package resource.
open(os.path.join(d, "pkg1", "data.txt"), "w").write("hello")
sys.path.insert(0, d)
assert pkgutil.get_data("pkg1", "data.txt") == b"hello"
# An absent resource is None.
assert pkgutil.get_data("pkg1", "nope.txt") is None

print("pkgutil ok")
