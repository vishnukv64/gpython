class Base:
    registry = []

    def __init_subclass__(cls, **kw):
        cls.kw = kw
        Base.registry.append(cls.__name__)


class A(Base):
    pass


class B(Base, kind="x", n=3):
    pass


print("registry:", Base.registry)
print("A.kw:", A.kw)
print("B.kw:", B.kw)
print("Base has no kw:", getattr(Base, "kw", "MISSING"))

# The hook is not called for the class that declares it.
print("Base not in registry:", "Base" not in Base.registry)

# A subclass of a subclass triggers the inherited hook again.
class C(A, tag=1):
    pass


print("registry now:", Base.registry)
print("C.kw:", C.kw)


# object.__init_subclass__ is the default and takes no arguments.
class Plain:
    pass


print("plain ok:", Plain.__name__)

try:
    class Bad(Plain, nope=1):
        pass
except TypeError as e:
    print("TypeError:", e)

doc = "finished"
