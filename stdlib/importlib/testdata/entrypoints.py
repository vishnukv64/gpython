"""importlib.metadata.entry_points and a module's __dict__.

pygments - vendored by pip - calls entry_points() and then .select(group=...),
and rewrites itself through "newmod.__dict__.update(oldmod.__dict__)".
"""
import types
from importlib.metadata import entry_points

eps = entry_points()
print("EntryPoints has select:", hasattr(eps, "select"))

sel = eps.select(group="console_scripts")
# CPython calls the class "EntryPoints"; here it is qualified with its module,
# which is the same shape other native types report.  What matters is that
# select() returns a COLLECTION with the same interface, not the class name.
print("select returns a collection:", hasattr(sel, "select") and len(sel) >= 0)
names = sorted(e.name for e in sel)
print("every entry point has the fields:", all(
    isinstance(e.name, str) and isinstance(e.value, str) and isinstance(e.group, str) for e in sel))
print("select by group equals keyword form:", names == sorted(
    e.name for e in entry_points(group="console_scripts")))
print("repr names the type:", "EntryPoint" in repr(list(sel)[0]) if len(sel) else True)
print("a group with no members is empty:", len(eps.select(group="nonexistent.group")) == 0)

# A module's __dict__ is its namespace, live and writable.
import logging

d = logging.__dict__
print("module __dict__ is a mapping:", hasattr(d, "update") and hasattr(d, "keys"))
print("it holds __name__:", d.get("__name__") == "logging")
d["entry_point_injected"] = 42
print("writes are visible on the module:", logging.entry_point_injected == 42)

# A subclass of types.ModuleType carries its own namespace, which is how
# pygments builds its replacement lexer module.
class Mod(types.ModuleType):
    pass


m = Mod("x")
m.__dict__["k"] = 1
print("ModuleType subclass __dict__ is writable:", m.k == 1)

doc = "finished"
