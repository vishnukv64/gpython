"""A sub-package of mypkg (mypkg.sub).

NOTE on this interpreter: `from . import deeper` inside a package's
__init__.py raises ImportError 'cannot import name deeper'. A submodule can
still be reached with an absolute import path (`import mypkg.sub.deeper`) or
with `from mypkg.sub.deeper import shout`. To keep this package working, the
sibling is pulled in with the absolute form instead of the relative one.
"""

from mypkg.sub import deeper

__all__ = ["deeper"]
