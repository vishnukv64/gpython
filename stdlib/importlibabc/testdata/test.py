"""importlib.abc: the abstract base classes of the import system."""

import importlib.abc as a
import typing

names = [
    "Loader", "ResourceLoader", "InspectLoader", "ExecutionLoader",
    "SourceLoader", "FileLoader", "Finder", "MetaPathFinder",
    "PathEntryFinder", "Traversable", "TraversableResources",
    "ResourceReader", "Protocol",
]
print("all present:", all(hasattr(a, n) for n in names))

print("Protocol is typing.Protocol:", a.Protocol is typing.Protocol)
print("module doc is a str:", isinstance(a.__doc__, str))

# isinstance works against the bases, which is why the module is imported.
print("object is a Loader:", isinstance(object(), a.Loader))
print("object is a Finder:", isinstance(object(), a.Finder))


class MyFinder(a.MetaPathFinder):
    def find_spec(self, name, path, target=None):
        return None


f = MyFinder()
print("subclass instance:", type(f).__name__)
print("isinstance own subclass:", isinstance(f, MyFinder))
print("isinstance of base:", isinstance(f, a.MetaPathFinder))
print("isinstance of Finder:", isinstance(f, a.Finder))

# The loader hierarchy nests the way CPython's does.
print("SourceLoader is a Loader:", issubclass(a.SourceLoader, a.Loader))
print("MetaPathFinder is a Finder:", issubclass(a.MetaPathFinder, a.Finder))
print("Loader is not a Finder:", issubclass(a.Loader, a.Finder))

doc = "finished"
