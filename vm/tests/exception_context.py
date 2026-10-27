import traceback
def show(e):
    c = e.__context__
    print(type(e).__name__, type(c).__name__ if c else None, type(e.__cause__).__name__ if e.__cause__ else None, e.__suppress_context__)
try:
    try:
        raise KeyError("a")
    except KeyError:
        raise ValueError("b")
except ValueError as e:
    show(e)
try:
    try:
        1/0
    except ZeroDivisionError:
        [][1]
except IndexError as e:
    show(e)
try:
    try:
        raise KeyError("a")
    except KeyError:
        raise ValueError("b") from None
except ValueError as e:
    show(e)
try:
    try:
        raise KeyError("a")
    except KeyError as k:
        raise ValueError("b") from k
except ValueError as e:
    show(e)
try:
    raise TypeError("plain")
except TypeError as e:
    show(e)
def f():
    raise RuntimeError("inner")
try:
    try:
        raise KeyError("x")
    except KeyError:
        f()
except RuntimeError as e:
    show(e)
try:
    try:
        raise KeyError("x")
    except KeyError as k:
        raise k
except KeyError as e:
    show(e)
try:
    try:
        1/0
    except ZeroDivisionError:
        raise ValueError("v")
except ValueError:
    s = traceback.format_exc()
    print("During handling" in s, s.count("Traceback"))
def show(tag, e):
    c = e.__context__
    print(tag, type(e).__name__, type(c).__name__ if c is not None else None, e.__suppress_context__)
try:
    raise KeyError("a")
except KeyError:
    pass
try:
    raise ValueError("after")
except ValueError as e:
    show("after-handler", e)
class G:
    def __getattr__(self, n): raise AttributeError(n)
hasattr(G(), "x")
for i in iter([1, 2]): pass
try:
    raise ValueError("clean")
except ValueError as e:
    show("after-go-catch", e)
def gen():
    try:
        yield 1
    except KeyError:
        raise ValueError("g")
g = gen(); next(g)
try:
    g.throw(KeyError("t"))
except ValueError as e:
    show("gen-throw", e)
try:
    try:
        raise KeyError("x")
    finally:
        raise ValueError("fin")
except ValueError as e:
    show("finally", e)
try:
    raise ValueError("x") from 5
except TypeError as e:
    print("bad-cause", e)
try:
    try:
        raise KeyError("a")
    except KeyError as a:
        try:
            raise TypeError("b")
        except TypeError as b:
            raise a
except KeyError as e:
    show("cycle", e)
    print(type(e.__context__.__context__).__name__ if e.__context__.__context__ else None)
def inner():
    try:
        raise OSError("o")
    except OSError:
        pass
try:
    try:
        raise KeyError("k")
    except KeyError:
        inner()
        raise ValueError("v")
except ValueError as e:
    show("after-callee-handler", e)
try:
    try:
        raise KeyError("k")
    except KeyError:
        int("x")
except ValueError as e:
    show("go-error", e)
