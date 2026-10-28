from __future__ import annotations

import dataclasses
from dataclasses import dataclass, field


@dataclass(frozen=True, init=False)
class A:
    x: int
    y: str | None = None

    def __init__(self, x, y=None):
        object.__setattr__(self, "x", x)
        object.__setattr__(self, "y", y)


class Sub(A):
    def to_dict(self):
        return dataclasses.asdict(self, dict_factory=lambda items: {k: v for k, v in items if v is not None})


@dataclass
class B:
    a: A
    d: dict = field(default_factory=dict)
    n: A | None = None


s = Sub(1)
print(dataclasses.is_dataclass(s), [f.name for f in dataclasses.fields(s)], s.to_dict())
print(dataclasses.asdict(B(A(1, "q"))))
print(dataclasses.asdict(B(A(1, "q")), dict_factory=lambda items: [k for k, _ in items]))
