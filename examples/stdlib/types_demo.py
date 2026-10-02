"""types: names for the built-in object types.

Run with:  /tmp/gpy examples/stdlib/types_demo.py

The module's main job is to give you a name for each built-in type, so you can
write isinstance(obj, types.FunctionType) instead of comparing type names as
strings. SimpleNamespace is a small mutable attribute bag.

Interpreter notes:
  * types.MappingProxyType() is unusable -- constructing one raises
    `TypeError: non-tuple sequence`.
  * Type objects are not hashable: `hash(int)` raises
    `TypeError: unhashable type: 'object'` and `hash(types.NoneType)` raises
    `TypeError: unhashable type: 'type'`. They cannot be used as dict keys.
  * `type(some_function).__name__` returns a property object, not a string,
    even though `some_function.__name__` is correct.
  See examples/KNOWN_ISSUES.md.
"""

import types


def a_function():
    return None


class AClass:
    def a_method(self):
        return None


generator = (value for value in [1])

print("--- the type objects ---")
pairs = [
    ("FunctionType (a def)", a_function),
    ("LambdaType", lambda: None),
    ("MethodType (a bound method)", AClass().a_method),
    ("ModuleType (this module)", types),
    ("NoneType", None),
    ("GeneratorType", generator),
    ("BuiltinFunctionType (len)", len),
    ("BuiltinMethodType (a list append)", [].append),
]
for label, value in pairs:
    print("  %-30s %s" % (label, type(value).__name__))

print()
print("--- isinstance() against the named types ---")
print("isinstance(a_function, types.FunctionType):     ", isinstance(a_function, types.FunctionType))
print("isinstance(lambda, types.LambdaType):           ", isinstance(lambda: None, types.LambdaType))
print("isinstance(AClass().a_method, types.MethodType):", isinstance(AClass().a_method, types.MethodType))
print("isinstance(types, types.ModuleType):            ", isinstance(types, types.ModuleType))
print("isinstance(None, types.NoneType):               ", isinstance(None, types.NoneType))
print("isinstance(generator, types.GeneratorType):     ", isinstance(generator, types.GeneratorType))
print("isinstance(len, types.BuiltinFunctionType):     ", isinstance(len, types.BuiltinFunctionType))

print()
print("--- FunctionType and LambdaType are the same object ---")
print("types.FunctionType is types.LambdaType:", types.FunctionType is types.LambdaType)

print()
print("--- SimpleNamespace is a mutable attribute bag ---")
namespace = types.SimpleNamespace()
print("empty:              ", namespace)
namespace.name = "widget"
namespace.count = 3
print("after assignment:   ", namespace)
print("attribute access:   ", namespace.name, namespace.count)
namespace.count = 4
print("attributes are mutable:", namespace)

with_kwargs = types.SimpleNamespace(host="localhost", port=8080)
print()
print("constructed with keyword arguments:")
print("  host:", with_kwargs.host)
print("  port:", with_kwargs.port)
print("  is it a ModuleType? ", isinstance(with_kwargs, types.ModuleType))

print()
print("--- new_class() builds a class at runtime ---")
Created = types.new_class("Created")
print("types.new_class('Created'):", Created)
instance = Created()
print("instance:                  ", instance)
print("isinstance of it:          ", isinstance(instance, Created))

print()
print("--- other things the module exposes ---")
for name in ["CodeType", "FrameType", "TracebackType", "CellType",
             "MappingProxyType", "ModuleSpec", "NoneType", "SimpleNamespace"]:
    print("  types.%-18s %s" % (name, hasattr(types, name)))

print()
print("--- MappingProxyType is present but unusable ---")
try:
    types.MappingProxyType({"a": 1})
except TypeError as err:
    print("types.MappingProxyType({'a': 1}) -> TypeError:", err)
    print("(CPython returns a read-only view of the mapping)")

print()
print("--- a worked example: dispatch with isinstance ---")


class Serialiser:
    """Renders a value by asking isinstance() about it.

    CPython would key a dict on the type objects themselves. Two things stop
    that here: type objects are unhashable, and type(some_function).__name__
    returns a property object rather than a string (see below). isinstance()
    sidesteps both.
    """

    def render(self, value):
        if value is None:
            return "null"
        if isinstance(value, bool):
            return "bool(%s)" % (value,)
        if isinstance(value, int):
            return "int(%d)" % (value,)
        if isinstance(value, float):
            return "float(%.3f)" % (value,)
        if isinstance(value, str):
            return "str(%r)" % (value,)
        if isinstance(value, list):
            return "list[%d]" % (len(value),)
        if isinstance(value, dict):
            return "dict{%d}" % (len(value),)
        if isinstance(value, types.FunctionType):
            return "function(%s)" % (value.__name__,)
        return "unknown type: " + str(type(value))


serialiser = Serialiser()
for value in [None, True, 7, 2.5, "text", [1, 2, 3], {"a": 1}, a_function, set([1])]:
    print("  %-22r -> %s" % (value, serialiser.render(value)))

print()
print("--- two problems with the type objects themselves ---")
for label, value in [("hash(int)", int), ("hash(types.NoneType)", types.NoneType),
                     ("hash(types.FunctionType)", types.FunctionType)]:
    try:
        hash(value)
        print("  %-26s OK" % (label,))
    except TypeError as err:
        print("  %-26s TypeError: %s" % (label, err))
print("so a dict can not be keyed by a class here.")
print()
print("and the type of a Python function reports a property, not a name:")
print("  type(a_function):            ", type(a_function))
print("  type(a_function).__name__:  ", type(a_function).__name__)
print("  str() of it:                ", str(type(a_function).__name__))
print("  a_function.__name__ itself: ", repr(a_function.__name__))
print("(use the instance's own __name__, or isinstance(), rather than type(x).__name__)")
