"""builtins: the module holding Python's always-available names.

Run with:  /tmp/gpy examples/stdlib/builtin_demo.py

The Go package for this lives in stdlib/builtin/, but it registers itself under
the name `builtins` -- which is exactly what CPython calls it. Every name you
use without importing anything lives here. This demo reaches them through the
module object instead of through the implicit namespace, which is useful when
you want to pass a builtin around as a value.

Interpreter note: `help` is the only common builtin missing from this
interpreter. There is also no `sys.modules` cache, so this is the one module
you can import whose contents are already in scope.
"""

import builtins

print("--- the module itself ---")
print("builtins.__name__:", builtins.__name__)
print("how many names:    ", len(dir(builtins)))
print("note: the Go package is stdlib/builtin/, but the import name is `builtins`")

print()
print("--- reaching builtins as objects ---")
print("builtins.abs:   ", builtins.abs)
print("builtins.abs(-3):", builtins.abs(-3))
print("builtins.len('abc'):", builtins.len("abc"))
print("builtins.max(1, 5): ", builtins.max(1, 5))
print("builtins.min([4, 2, 9]):", builtins.min([4, 2, 9]))
print("builtins.sum([1, 2, 3]):", builtins.sum([1, 2, 3]))
print("builtins.sorted([3, 1, 2]):", builtins.sorted([3, 1, 2]))
print("builtins.range(3):", list(builtins.range(3)))
print("builtins.str(42):", repr(builtins.str(42)))
print("builtins.int('7'):", builtins.int("7"))
print("builtins.float('2.5'):", builtins.float("2.5"))
print("builtins.bool(0):", builtins.bool(0))

print()
print("--- passing them around as values ---")
apply = builtins.map
print("map through a variable:", list(apply(builtins.abs, [-1, -2, -3])))
chooser = builtins.max
print("max through a variable: ", chooser([10, 30, 20]))

print()
print("--- the exception classes live here too ---")
for name in ["Exception", "BaseException", "ValueError", "TypeError",
             "KeyError", "IndexError", "AttributeError", "NameError",
             "ZeroDivisionError", "StopIteration", "RuntimeError",
             "NotImplementedError", "OSError", "FileNotFoundError",
             "ImportError", "ModuleNotFoundError"]:
    kind = getattr(builtins, name, None)
    print("  %-22s %s" % (name, kind))
print("ValueError subclasses Exception:",
      issubclass(builtins.ValueError, builtins.Exception))
print("FileNotFoundError subclasses OSError:",
      issubclass(builtins.FileNotFoundError, builtins.OSError))

print()
print("--- the type constructors ---")
for name in ["int", "float", "str", "list", "dict", "tuple", "set",
             "frozenset", "object", "type", "bool", "complex"]:
    kind = getattr(builtins, name, None)
    if kind is None:
        print("  %-10s absent" % (name,))
        continue
    try:
        sample = kind()
        print("  %-10s -> %r" % (name, sample))
    except Exception as err:
        print("  %-10s -> %s: %s" % (name, type(err).__name__, err))

print()
print("--- the implicit namespace and the module agree ---")
print("abs is builtins.abs:      ", abs is builtins.abs)
print("len is builtins.len:      ", len is builtins.len)
print("print is builtins.print:  ", print is builtins.print)
print("ValueError is builtins.ValueError:", ValueError is builtins.ValueError)

print()
print("--- what is missing ---")
print("hasattr(builtins, 'help'):", hasattr(builtins, "help"))
print("hasattr(builtins, 'input'):", hasattr(builtins, "input"))
print("hasattr(builtins, 'breakpoint'):", hasattr(builtins, "breakpoint"))
print("hasattr(builtins, 'bytearray'):", hasattr(builtins, "bytearray"))
print("hasattr(builtins, 'memoryview'):", hasattr(builtins, "memoryview"))
print("hasattr(builtins, '__import__'):", hasattr(builtins, "__import__"))

print()
print("--- __import__ is the machinery behind the import statement ---")
imported = builtins.__import__("json")
print("__import__('json'):", imported.__name__)
print("it has dumps:      ", callable(imported.dumps))

print()
print("--- a worked example: a safe evaluator with no namespace access ---")


class SafeOps:
    """A tiny expression evaluator over a fixed table of builtins.

    Because the operations are looked up on the builtins module by name, the
    evaluator never needs access to the caller's globals.
    """

    def __init__(self):
        self.operations = {}
        for name in ["abs", "len", "sum", "max", "min", "sorted", "str", "int", "float"]:
            operation = getattr(builtins, name, None)
            if operation is not None:
                self.operations[name] = operation

    def call(self, name, *arguments):
        operation = self.operations.get(name)
        if operation is None:
            raise builtins.NameError("unknown operation: %s" % (name,))
        try:
            return operation(*arguments)
        except builtins.TypeError as err:
            raise builtins.TypeError("%s: %s" % (name, err))

    def available(self):
        return sorted(self.operations.keys())


ops = SafeOps()
print("available:", ops.available())
print()
print("ops.call('abs', -5):           ", ops.call("abs", -5))
print("ops.call('len', 'hello'):      ", ops.call("len", "hello"))
print("ops.call('sum', [1, 2, 3, 4]):  ", ops.call("sum", [1, 2, 3, 4]))
print("ops.call('sorted', [3, 1, 2]):  ", ops.call("sorted", [3, 1, 2]))
print("ops.call('max', [5, 9, 2]):     ", ops.call("max", [5, 9, 2]))
print("ops.call('int', '17'):          ", ops.call("int", "17"))
print()
for label, name, arguments in [("unknown name", "eval", ("1+1",)),
                               ("bad argument", "len", (5,))]:
    try:
        ops.call(name, *arguments)
        print("  %-14s succeeded unexpectedly" % (label,))
    except builtins.NameError as err:
        print("  %-14s -> NameError: %s" % (label, err))
    except builtins.TypeError as err:
        print("  %-14s -> TypeError: %s" % (label, err))
