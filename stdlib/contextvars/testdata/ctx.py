import contextvars, threading
from contextvars import ContextVar, copy_context, Context

v = ContextVar("v", default=10)
print(v.name, v.get(), v.get(5))
t = v.set(1)
print(v.get(), t.old_value is contextvars.Token.MISSING, t.var is v)
t2 = v.set(2)
v.reset(t2)
print(v.get())
try:
    v.reset(t2)
except RuntimeError as e:
    print("RuntimeError")
v.reset(t)
print(v.get())

nodef = ContextVar("nodef")
try:
    nodef.get()
except LookupError:
    print("LookupError")
print(nodef.get(None))

nodef.set("a")
ctx = copy_context()
def f(x):
    nodef.set(x)
    return nodef.get()
print(ctx.run(f, "b"), nodef.get(), ctx[nodef], nodef in ctx, len(ctx))
print(ctx.get(v), list(ctx.keys()) == [nodef], dict(ctx.items()) == {nodef: "b"})
def reenter():
    try:
        ctx.run(lambda: None)
    except RuntimeError:
        return "reenter RuntimeError"
print(ctx.run(reenter))
try:
    ctx[v]
except KeyError:
    print("KeyError")

out = []
def worker():
    out.append(nodef.get("fresh"))
    nodef.set("thread")
th = threading.Thread(target=worker)
th.start(); th.join()
print(out, nodef.get())
print(Context().run(nodef.get, "x"))
