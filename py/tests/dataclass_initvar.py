from __future__ import annotations
import dataclasses
from dataclasses import InitVar, dataclass, field
from typing import ClassVar

@dataclass
class E:
    message: str = field(init=False)
    n: int = field(init=False, default=3)
    exc: InitVar[Exception] = field(init=True)
    indent: InitVar[str] = field(default="")
    K: ClassVar[int] = 7

    def __post_init__(self, exc, indent):
        self.message = indent + str(exc)

e = E(ValueError("x"), "> ")
print(e, e.message, e.n, E.K)
print([f.name for f in dataclasses.fields(e)], dataclasses.asdict(e))
print(InitVar[str])
