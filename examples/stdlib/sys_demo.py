"""sys: interpreter state and command-line interface.

Run with:  /tmp/gpy examples/stdlib/sys_demo.py   (add extra arguments to see them echoed)

Covers version info, argv, the module search path, the standard streams and
exit(). sys.modules is always empty in this interpreter -- modules are not
registered in a cache.

Interpreter notes:
  * sys.getrecursionlimit(), sys.setrecursionlimit(), sys.getsizeof(),
    sys.getdefaultencoding() and sys.intern() raise NotImplementedError.
  * sys.modules is an empty dict, so `"json" in sys.modules` is always False.
"""

import sys

print("--- version information ---")
print("sys.version:      ", sys.version)
print("sys.version_info: ", tuple(sys.version_info))
print("sys.hexversion:   ", hex(sys.hexversion))
print("sys.platform:     ", sys.platform)
print("sys.byteorder:    ", sys.byteorder)
print("sys.maxsize:      ", sys.maxsize)
print("sys.api_version:  ", sys.api_version)

print()
print("--- where this interpreter lives ---")
print("sys.executable:  ", sys.executable)
print("sys.prefix:      ", sys.prefix)
print("sys.exec_prefix: ", sys.exec_prefix)
print("sys.base_prefix: ", sys.base_prefix)

print()
print("--- the command line ---")
print("sys.argv:        ", sys.argv)
print("sys.argv[0] is this script's name:", sys.argv[0])
print("extra arguments would appear after it")
if len(sys.argv) > 1:
    for index, argument in enumerate(sys.argv[1:], start=1):
        print("  argument %d: %r" % (index, argument))
else:
    print("  (no extra arguments were given; try: /tmp/gpy %s one two)" % (sys.argv[0],))

print()
print("--- the module search path ---")
print("sys.path:", sys.path)
print("it is a list:", type(sys.path).__name__)

print()
print("--- the standard streams ---")
for name in ["stdin", "stdout", "stderr"]:
    stream = getattr(sys, name)
    print("  sys.%-6s -> %s object" % (name, type(stream).__name__))
print("sys.__stdout__ is the original stdout:", sys.__stdout__ is not None)
print("sys.ps1/sys.ps2 are the REPL prompts:", getattr(sys, "ps1", "(absent)"), getattr(sys, "ps2", "(absent)"))

print()
print("--- the builtin module list ---")
print("sys.builtin_module_names:")
for name in sys.builtin_module_names:
    print("  ", name)

print()
print("--- sys.modules is always empty ---")
print("type:              ", type(sys.modules).__name__)
print("len(sys.modules):  ", len(sys.modules))
print("'json' in sys.modules:", "json" in sys.modules)
print("(so import_module() and `import` do not register modules in a cache here)")

print()
print("--- what is not implemented ---")
for name in ["getrecursionlimit", "setrecursionlimit", "getsizeof",
             "getdefaultencoding", "intern"]:
    function = getattr(sys, name, None)
    if function is None:
        print("  sys.%-20s absent" % (name,))
        continue
    try:
        function(1) if name in ["getsizeof", "intern"] else function()
        print("  sys.%-20s OK" % (name,))
    except NotImplementedError:
        print("  sys.%-20s raises NotImplementedError" % (name,))

print()
print("--- exiting ---")
print("sys.exit is callable:", callable(sys.exit))
print("sys.exc_info() outside an except block:", sys.exc_info())
try:
    raise ValueError("sample")
except ValueError:
    kind, value, traceback = sys.exc_info()
    print("inside an except block it reports:")
    print("  type:  ", kind)
    print("  value: ", value)


def guarded(value):
    """Return None instead of propagating, using exc_info()."""
    try:
        return int(value)
    except ValueError:
        kind, _, _ = sys.exc_info()
        return "failed with " + str(kind)


print()
print("--- a worked example: exc_info() for context-free error reporting ---")
for candidate in ["42", "not a number"]:
    print("  guarded(%-14r) = %r" % (candidate, guarded(candidate)))

print()
print("--- a small argument parser, using only sys.argv ---")


def parse(arguments):
    """Split --flag switches from bare values."""
    flags = []
    values = []
    for argument in arguments:
        if argument.startswith("--"):
            flags.append(argument[2:])
        else:
            values.append(argument)
    return flags, values


flags, values = parse(sys.argv[1:])
print("flags found:  ", flags)
print("values found: ", values)
print("(run the script again with arguments to see this change)")
