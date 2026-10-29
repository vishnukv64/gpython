"""Traceback caret lines (~~^^^) under the failing expression, as CPython 3.14."""
import sys
import traceback


def call(f, *a):
    try: f(*a)
    except BaseException as e:
        text = "".join(traceback.format_exception(e, chain=False))
        sys.stdout.write(text.replace(__file__, "FILE"))
d = {}
class K:
    def m(self): return self.nope
def f1(a, b): return a + b
def f2(a): return a[5]
def f3(a): return a.missing
def f4(x): return len(x)
def f5(x):
    y = int(x)
    return y
def f6(a, b, c): return a * (b + c) - 1
def f7(a): 
    return (a
            + 1)
def f8(a): return [1][a] + 2
def f9(): raise ValueError("boom")
def f10(a): return a.b.c.d
def f11(a): x = a(1)(2); return x
def f12(): return 1/0
def f13(a): d["k"]["j"]
def f14(a): return -a
def f15(a): return a < 3
def f16(a):
    for i in a: pass
def f17(n):
    return call(f17, n) if n else [][0]
def f18(a):
    foo = lambda q: q.zz
    return foo(a)
def f19(a): return unknown_name + 1
def f20(a): return "é" + a + "日本"
def f21(a):
    return f1(
        a,
        None
    )
call(f1, 1, "x"); call(f2, [1]); call(f3, 1); call(f4, 3); call(f5, "q"); call(f6, 1, "a", 2)
call(f7, "a"); call(f8, 3); call(f9); call(f10, 1); call(f11, lambda x: 3); call(f12); call(f13, 1)
call(f14, "s"); call(f15, "s"); call(f16, 3); call(K().m); call(f18, 1); call(f19, 1); call(f20, 1); call(f21, 1)
