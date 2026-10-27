import abc, inspect
class A(abc.ABC):
    @abc.abstractmethod
    def f(self): pass
class B(A):
    def f(self): return 1
class C: pass
class D(abc.ABC): pass
class E(A): pass
print(inspect.isabstract(A), inspect.isabstract(B), inspect.isabstract(C),
      inspect.isabstract(D), inspect.isabstract(E), inspect.isabstract(3), inspect.isabstract(A()) if False else "")
