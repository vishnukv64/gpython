"""importlib.metadata against a dist-info built on the fly, plus the pieces it
needs: types.MethodType, a defaultdict subclass, namedtuple._replace, suppress."""
import collections
import contextlib
import email.message
import importlib.metadata as md
import os
import sys
import tempfile
import types
from importlib.metadata import _adapters  # noqa: F401  (import before sys.path is narrowed)

with tempfile.TemporaryDirectory() as root:
    info = os.path.join(root, "Demo_Pkg-1.2.3.dist-info")
    os.mkdir(info)
    with open(os.path.join(info, "METADATA"), "wb") as f:
        f.write(b"Metadata-Version: 2.1\nName: Demo-Pkg\nVersion: 1.2.3\n"
                b"Summary: a demo\nRequires-Dist: other (>=1)\n"
                b"Classifier: A\nClassifier: B\n\nlong description\n")
    with open(os.path.join(info, "entry_points.txt"), "wb") as f:
        f.write(b"[console_scripts]\ndemo = demo.cli:main\ndemo2 = demo.cli:other\n\n"
                b"[demo.plugins]\nfoo = demo.plug:Foo [extra]\n")
    sys.path.insert(0, root)

    print("version:", md.version("demo-pkg"))
    d = md.distribution("Demo.Pkg")
    print("name:", d.metadata["Name"], d.name, d.version)
    print("summary:", d.metadata["Summary"])
    print("classifiers:", d.metadata.get_all("Classifier"))
    print("requires:", d.requires)
    print("json keys:", sorted(d.metadata.json)[:4])
    print("files:", d.files)

    eps = md.entry_points(group="console_scripts")
    print(type(eps).__name__, sorted((e.name, e.value) for e in eps if e.dist.name == "Demo-Pkg"))
    print({"console_scripts", "demo.plugins"} <= md.entry_points().groups)
    (foo,) = md.entry_points(group="demo.plugins")
    print(foo)
    print(foo.name, foo.group, foo.module, foo.attr, foo.extras)
    print(md.entry_points(group="demo.plugins", name="foo").names)
    print(md.EntryPoints([foo]).select(name="foo") == md.EntryPoints([foo]))
    print([x.name for x in md.distributions() if x.name == "Demo-Pkg"])
    try:
        md.version("no-such-dist-xyz")
    except md.PackageNotFoundError as e:
        print("not found:", e)
    sys.path.remove(root)


class MyDict(collections.defaultdict):
    def __missing__(self, k):
        return getattr(self, "_frozen", super().__missing__)(k)

    def freeze(self):
        self._frozen = lambda k: self.default_factory()


dd = MyDict(list)
print(type(dd).__name__, isinstance(dd, collections.defaultdict))
dd["a"].append(1)
dd.freeze()
print(dict(dd), dd["zz"], dict(dd))


class A:
    def f(self):
        pass


m = types.MethodType(lambda self, x: (type(self).__name__, x), A())
print(m(5), types.MethodType is type(A().f))

P = collections.namedtuple("P", "a b")


class Q(P):
    pass


print(P(1, 2)._replace(b=9), Q(1, 2)._replace(a=0))
try:
    P(1, 2)._replace(c=1)
except TypeError as e:
    print(e)

with contextlib.suppress(OSError):
    os.stat(os.path.join(tempfile.gettempdir(), "no-such-file-xyz"))
print("suppressed")
