from typing import Generic, TypeVar
A = TypeVar("A"); B = TypeVar("B")
class X:
    def __class_getitem__(cls, _):
        return cls
class Y(X, Generic[A, B]):
    def __class_getitem__(cls, _):
        return cls
class Z(Y[A, B]): pass
print(Z, Y[int, str])
