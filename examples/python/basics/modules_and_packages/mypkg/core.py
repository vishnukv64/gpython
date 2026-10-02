"""The `core` submodule of the mypkg package.

Demonstrates a plain module inside a package: module-level constants,
functions, and a class.
"""

MODULE_NAME = "mypkg.core"


def greet(name):
    """Return a friendly greeting."""
    return "Hello, " + name


def shout(name):
    """A second function, so the module has more than one name to import."""
    return greet(name).upper()


class Greeter:
    """A small class, to show that classes travel across modules too."""

    def __init__(self, greeting="Hi"):
        self.greeting = greeting

    def greet(self, name):
        return "%s, %s!" % (self.greeting, name)

    def __repr__(self):
        return "Greeter(%r)" % self.greeting
