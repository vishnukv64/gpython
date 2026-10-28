from typing import Protocol, runtime_checkable


@runtime_checkable
class S(Protocol):
    def m(self): ...


@runtime_checkable
class T(Protocol):
    x: int

    def m(self): ...


class A:
    x = 1

    def m(self):
        pass


class B:
    def m(self):
        pass


print(isinstance(A(), S), isinstance(B(), S), isinstance(3, S))
print(isinstance(A(), T), isinstance(B(), T))

