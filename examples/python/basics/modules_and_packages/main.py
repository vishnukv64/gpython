"""Imports: modules, packages, submodules, relative imports and aliases.

Run from this directory with:
    cd examples/python/basics/modules_and_packages && /tmp/gpy main.py

The layout this file exercises:

    modules_and_packages/
        main.py                 <- this script
        helpers.py              <- a standalone module beside the script
        mypkg/
            __init__.py         <- defines VERSION, AUTHOR; re-exports greet
            core.py             <- greet(), shout(), Greeter
            sub/
                __init__.py     <- imports the sibling `deeper`
                deeper.py       <- uses the two-dot relative import ..core

This interpreter runs a script with the script's own directory on sys.path,
so the plain `import` statements below find these files.
"""

print("--- importing a module beside the script ---")
import helpers
print("helpers.GREETING:  ", helpers.GREETING)
print("helpers.add(2, 3): ", helpers.add(2, 3))
print("helpers.describe():", helpers.describe())

print()
print("--- `from X import Y` ---")
from helpers import add, GREETING
print("add(10, 20):       ", add(10, 20))
print("GREETING:          ", GREETING)

print()
print("--- `import X as Y` ---")
import helpers as h
print("h.add(1, 2):       ", h.add(1, 2))

from helpers import add as plus
print("plus(3, 4):        ", plus(3, 4))

print()
print("--- importing a package (its __init__.py runs) ---")
import mypkg
print("mypkg.VERSION:     ", mypkg.VERSION)
print("mypkg.AUTHOR:      ", mypkg.AUTHOR)
# __init__.py re-exported greet with `from .core import greet`.
print("mypkg.greet('x'):  ", mypkg.greet("world"))

print()
print("--- importing a submodule of a package ---")
import mypkg.core
print("core.MODULE_NAME:  ", mypkg.core.MODULE_NAME)
print("core.shout('x'):   ", mypkg.core.shout("quiet"))
print("core.Greeter class is usable:", mypkg.core.Greeter("Hey").greet("there"))

print()
print("--- `from package.module import name` ---")
from mypkg.core import greet, shout, Greeter
print("greet:             ", greet("direct"))
print("shout:             ", shout("direct"))
print("Greeter:           ", Greeter("Yo").greet("you"))

print()
print("--- relative imports inside the package ---")
# mypkg/sub/__init__.py did `from . import deeper`, and
# mypkg/sub/deeper.py did `from ..core import greet`.
from mypkg.sub import deeper
print("deeper.NAME:       ", deeper.NAME)
print("deeper.identify(): ", deeper.identify())
print("deeper.shout:      ", deeper.shout_greeting("deep"))

# Importing the sub-package alone exposes its sibling through its __init__.
import mypkg.sub
print("mypkg.sub.deeper.identify():", mypkg.sub.deeper.identify())

print()
print("--- the module object itself ---")
print("module has __name__:", helpers.__name__)
print("module in sys.modules:", end=" ")
import sys
try:
    found = "helpers" in sys.modules
    print(found)
except TypeError:
    # sys.modules is present but membership on it may be limited here.
    print("(membership on sys.modules is not supported here)")

print()
print("--- re-importing returns the cached module, it is not re-executed ---")
import helpers as again
print("same object as before:", again is h)
print("its data is unchanged:", again.GREETING)

print()
print("--- an import can appear inside a function ---")


def local_import():
    import helpers as inner
    return inner.add(100, 200)


print("local_import():    ", local_import())

print()
print("--- a failing import raises ImportError ---")
try:
    import definitely_not_a_real_module
except ImportError as err:
    print("ImportError:", err)

print()
print("done")
