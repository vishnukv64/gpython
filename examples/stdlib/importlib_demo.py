"""importlib: importing modules by name at runtime.

Run with:  /tmp/gpy examples/stdlib/importlib_demo.py

Covers import_module(), reload() and invalidate_caches(), plus the difference
between import_module() and the __import__() builtin.

Interpreter notes:
  * import_module() resolves only top-level modules -- "os.path" raises
    ImportError, where CPython returns the posixpath module.
  * A missing module raises ModuleNotFoundError, a subclass of ImportError.
  * sys.modules stays empty, so a module imported here is never registered in
    the module cache and sys.modules["json"] raises KeyError.
"""

import importlib
import sys

print("--- import_module() takes a string name ---")
json_module = importlib.import_module("json")
print("type:     ", type(json_module).__name__)
print("__name__: ", json_module.__name__)
print("it has dumps:", callable(json_module.dumps))
print()
print("NOTE: sys.modules is always empty in this interpreter, so the imported")
print("module is NOT registered there:")
print("  len(sys.modules):          ", len(sys.modules))
print("  sys.modules.get('json'):   ", sys.modules.get("json"))

print()
print("--- importing several by name ---")
for name in ["os", "re", "hashlib", "itertools"]:
    module = importlib.import_module(name)
    print("  %-10s -> %s" % (name, module.__name__))

print()
print("--- a missing module raises ModuleNotFoundError ---")
try:
    importlib.import_module("definitely_not_a_module")
except ModuleNotFoundError as err:
    print("ModuleNotFoundError:", err)
print("ModuleNotFoundError is an ImportError:", issubclass(ModuleNotFoundError, ImportError))

print()
print("--- dotted sub-modules are not resolved ---")
try:
    importlib.import_module("os.path")
except ImportError as err:
    print("import_module('os.path') -> ImportError:", err)
    print("(CPython returns the underlying posixpath module here)")

print()
print("--- reload() re-runs a module and returns it ---")
reloaded = importlib.reload(json_module)
print("reload returned:      ", reloaded.__name__)
print("same module object:   ", reloaded is json_module)
try:
    importlib.reload("not a module")
except TypeError as err:
    print("reload('not a module') -> TypeError:", err)

print()
print("--- invalidate_caches() is a no-op here, but safe to call ---")
print("return value:", importlib.invalidate_caches())
print("(there is no per-directory finder cache to clear)")

print()
print("--- a worked example: pick a formatter by configured name ---")


class FormatterRegistry:
    """Looks up a dump/load implementation from a module/function pair."""

    def __init__(self):
        self.entries = {}

    def register(self, kind, module_name, dump_name, load_name):
        self.entries[kind] = (module_name, dump_name, load_name)
        return self

    def codec(self, kind):
        module_name, dump_name, load_name = self.entries[kind]
        module = importlib.import_module(module_name)
        return getattr(module, dump_name), getattr(module, load_name)


registry = FormatterRegistry()
registry.register("json", "json", "dumps", "loads")
registry.register("yaml", "yaml", "safe_dump", "safe_load")
registry.register("marshal", "marshal", "dumps", "loads")

payload = {"name": "widget", "sizes": [1, 2]}
for kind in ["json", "yaml", "marshal"]:
    try:
        dump, load = registry.codec(kind)
        encoded = dump(payload)
        decoded = load(encoded)
        print("  %-8s encoded=%r" % (kind, encoded))
        print("  %-8s decoded=%r" % ("", decoded))
        print("  %-8s round trip equal: %s" % ("", decoded == payload))
    except Exception as err:
        print("  %-8s failed: %s: %s" % (kind, type(err).__name__, err))

print()
print("--- import_module vs __import__ ---")
print("__import__('json')       ->", __import__("json").__name__)
print("import_module('json')    ->", importlib.import_module("json").__name__)
print("both return a module object; neither registers it in sys.modules here:")
print("  len(sys.modules):", len(sys.modules))
