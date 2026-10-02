"""A standalone module (not part of a package) used to show plain imports.

`import helpers` from the script beside this file picks this up directly.
"""

GREETING = "hello from helpers"


def add(a, b):
    return a + b


def describe():
    return "helpers module, GREETING=%r" % GREETING


# Code here runs once, on first import only. Guarding it with __name__ means
# it does not run when the module is imported, only when it is run directly.
def _demo():
    print("helpers._demo() running, __name__ == %r" % __name__)


if __name__ == "__main__":
    _demo()
