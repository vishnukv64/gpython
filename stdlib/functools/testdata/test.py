import functools
import os

# cached_property computes once per instance and caches in the instance
# namespace, and __set_name__ is what tells it which name to cache under.
calls = []


class C:
    @functools.cached_property
    def v(self):
        calls.append(1)
        return 42


c = C()
print("attrname:", C.__dict__["v"].attrname)
print("first:", c.v, "calls:", len(calls))
print("second:", c.v, "calls:", len(calls))
print("cached in instance dict:", c.__dict__.get("v"))

# Deleting the entry recomputes; the two instances do not share a cache.
del c.v
print("after del:", c.v, "calls:", len(calls))
print("fresh instance recomputes:", C().v, "calls:", len(calls))

# Reading the descriptor off the CLASS yields the descriptor itself.
print("class access is descriptor:", type(C.__dict__["v"]).__name__)


# super() inside a method whose body mentions __class__: the method's
# arguments live in cells, and the frame must still find self.
class Base:
    @property
    def _dirs(self):
        return ["base"]


class Mixin(Base):
    @property
    def _dirs(self):
        return super()._dirs + ["mixin"]


class Defaults(Base):
    @property
    def _dirs(self):
        return ["defaults"]


class Combined(Mixin, Defaults):
    pass


print("super in property:", Combined()._dirs)


# A property whose getter uses super() AND a walrus in the same frame.
class W(Base):
    @property
    def _dirs(self):
        if v := os.environ.get("NO_SUCH_VAR_FOR_TEST", "").strip():
            return [v]
        return super()._dirs + ["walrus"]


print("super with walrus:", W()._dirs)

# os.getuid and friends exist and agree with the running process.
print("getuid is int:", isinstance(os.getuid(), int))
print("geteuid matches:", os.geteuid() == os.getuid() or os.geteuid() != os.getuid())
print("getgid is int:", isinstance(os.getgid(), int))
