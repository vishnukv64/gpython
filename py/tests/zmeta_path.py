"""sys.meta_path: a program may install its own finder."""

import sys

print("meta_path is a list:", isinstance(sys.meta_path, list))
print("path_hooks is a list:", isinstance(sys.path_hooks, list))


class Loader:
    """A PEP 451 loader: exec_module fills the module in."""

    def create_module(self, spec):
        return None

    def exec_module(self, module):
        module.VALUE = 42
        module.NAME = module.__name__


class Spec:
    def __init__(self, name, loader):
        self.name = name
        self.loader = loader
        self.origin = "<virtual>"
        self.submodule_search_locations = None
        self.has_location = False
        self.cached = None
        self.parent = ""


class Finder:
    def __init__(self, name):
        self.name = name
        self.asked = []

    def find_spec(self, name, path=None, target=None):
        self.asked.append(name)
        if name == self.name:
            return Spec(name, Loader())
        return None


finder = Finder("virtual_mod")
sys.meta_path.append(finder)

import virtual_mod

print("VALUE:", virtual_mod.VALUE)
print("NAME:", virtual_mod.NAME)
print("in sys.modules:", "virtual_mod" in sys.modules)
print("the finder was asked:", "virtual_mod" in finder.asked)

# A finder that does not claim the name is passed over, so the normal
# resolution still finds an ordinary module that was not yet imported.
import colorsys

print("colorsys still imports:", hasattr(colorsys, "rgb_to_hsv"))

# An error raised INSIDE a finder surfaces rather than being passed over: the
# point of inserting a finder is to be believed.
class BadFinder:
    def find_spec(self, name, path=None, target=None):
        raise RuntimeError("finder broke")


bad = BadFinder()
sys.meta_path.insert(0, bad)
try:
    import netrc
    print("no error from the bad finder")
except RuntimeError as e:
    print("finder error surfaces:", e)

# The finder is REMOVED again: the meta_path list belongs to the interpreter,
# and the test harness shares one across every script it runs - leaving a
# raiser on it made the NEXT test fail with this finder's error.
sys.meta_path.remove(bad)

doc = "finished"
