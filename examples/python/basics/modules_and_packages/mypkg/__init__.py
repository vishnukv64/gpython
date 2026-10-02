"""A small package used by the modules_and_packages examples.

This is the package's __init__.py. It runs when `import mypkg` happens, and it
re-exports `greet` from the submodule `.core` so users can write
`mypkg.greet(...)` without knowing which module the function lives in.
"""

# Package-level data.
VERSION = "1.0.0"
AUTHOR = "gpython examples"

# Relative import: `.core` means "the core module inside THIS package".
# This interpreter supports relative imports from within a package.
from .core import greet

# `__all__` lists the names a `from mypkg import *` would bring in.
__all__ = ["greet", "VERSION"]
