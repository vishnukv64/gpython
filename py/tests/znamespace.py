"""Namespace packages (PEP 420): a directory with no __init__.py."""

import os
import sys

# The directory holding this test, so "nspkg" beside it is found.
here = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, here)

import nspkg

print("imported:", type(nspkg).__name__)
print("has __file__:", getattr(nspkg, "__file__", None))
# The LENGTH depends on how many sys.path entries match, which differs between
# a run from the repository root and one from this directory - so what is pinned
# is that the path is non-empty and absolute, not its arity.
print("__path__ non-empty:", len(nspkg.__path__) > 0)
print("__path__ absolute:", all(os.path.isabs(p) for p in nspkg.__path__))

# Its submodules resolve, both by dotted import and through fromlist.
from nspkg.inner import mod

print("submodule X:", mod.X)

import nspkg.other

print("sibling Y:", nspkg.other.Y)

from nspkg import other

print("fromlist Y:", other.Y)
print("bound on parent:", nspkg.other.Y)

doc = "finished"
