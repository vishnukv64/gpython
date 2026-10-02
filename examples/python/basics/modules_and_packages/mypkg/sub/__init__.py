"""A sub-package of mypkg (mypkg.sub).

It pulls in its own submodule with a RELATIVE import, which is the idiomatic
way and resolves against *this* package: "from . import deeper" means
mypkg.sub.deeper.
"""

from . import deeper

__all__ = ["deeper"]
