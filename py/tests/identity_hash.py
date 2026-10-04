"""Functions, methods and modules hash by identity."""

import os
import sys


def plain():
    pass


class C:
    def meth(self):
        pass


# A function and a method are usable as dict keys.  Without this a FUNCTION was
# unhashable, and the import system keys a table by one.
d = {}
d[plain] = "function"
d[C.meth] = "method"
d[C] = "class"
d[sys] = "module"
print("keys:", len(d))
print("function lookup:", d[plain])
print("method lookup:", d[C.meth])
print("class lookup:", d[C])
print("module lookup:", d[sys])

# Two distinct functions are two distinct keys, and the same one is stable.
def other():
    pass


print("distinct functions distinct:", len({plain: 1, other: 2}))
print("same function same key:", {plain: 1}[plain])

# Modules are distinct from each other.
print("two modules distinct:", len({sys: 1, os: 2}))
print("module membership:", sys in d, os in d)

# hash() returns an int for each.
print("hash types:", isinstance(hash(plain), int), isinstance(hash(sys), int))

doc = "finished"
