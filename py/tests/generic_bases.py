import typing
from collections.abc import MutableMapping, Sequence

K = typing.TypeVar("K")
V = typing.TypeVar("V")


class R(typing.Generic[K, V], typing.MutableMapping[K, V]):
    def __init__(self):
        self.d = {}

    def __getitem__(self, k):
        return self.d[k]

    def __setitem__(self, k, v):
        self.d[k] = v

    def __delitem__(self, k):
        del self.d[k]

    def __iter__(self):
        return iter(self.d)

    def __len__(self):
        return len(self.d)


r = R()
r["a"] = 1
r.update(b=2)
print([c.__name__ for c in R.__mro__])
print(isinstance(r, MutableMapping), dict(r), r.pop("a"), len(r))
print(R.__orig_bases__[0].__origin__.__name__, len(R.__orig_bases__))


class S(Sequence[K]):
    def __getitem__(self, i):
        return [1, 2][i]

    def __len__(self):
        return 2


print([c.__name__ for c in S.__mro__ if c.__name__ != 'Generic'], list(S()))


class Box(typing.Generic[K]):
    def __init__(self, v):
        self.v = v


print(Box[int](3).v, [c.__name__ for c in Box.__mro__])


class P:
    @classmethod
    def __class_getitem__(cls, item):
        return (cls.__name__, item)


class Q(P):
    pass


print(Q[int])
