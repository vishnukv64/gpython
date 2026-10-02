"""A module inside a sub-package (mypkg.sub.deeper).

It reaches OUT of its own package with the two-dot relative import
`from ..core import greet`, which resolves to mypkg.core.
"""

from ..core import greet

NAME = "mypkg.sub.deeper"


def identify():
    """Report which module this is."""
    return NAME


def shout_greeting(name):
    """Greet using a function imported from two levels up the package."""
    return greet(name).upper()
