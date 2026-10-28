import weakref, gc, atexit


class A:
    pass


def cb(*a, **k):
    print("cb", a, sorted(k.items()))


a = A()
f = weakref.finalize(a, cb, 1, 2, x=3)
print(f.alive, f.atexit, f.peek()[0] is a, f.peek()[2:])
f()
print(f.alive, f.peek(), f.detach())
f()

b = A()
g = weakref.finalize(b, cb, "detached")
print(g.detach()[0] is b, g.alive)
g()


def make():
    o = A()
    return weakref.finalize(o, cb, "collected")


h = make()
gc.collect()
print("after collect", h.alive)
h()

d = A()
k1 = weakref.finalize(d, cb, "exit1")
k2 = weakref.finalize(d, cb, "exit2")
k3 = weakref.finalize(d, cb, "noexit")
k3.atexit = False
print(k3.atexit, k1.atexit)
for bad in (1, "s", None):
    try:
        weakref.finalize(bad, cb)
    except TypeError as e:
        print(e)
try:
    weakref.finalize()
except TypeError as e:
    print(e)
try:
    k1(1)
except TypeError as e:
    print("TypeError")
atexit._run_exitfuncs()
print(k2.alive, k3.alive)
print("end")
