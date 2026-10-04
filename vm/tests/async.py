"""async def / await / async with / async for (PEP 492), sans asyncio.

Coroutines are driven by hand (coro.send(None)/coro.close()) since the event
loop is out of scope.  The output is pinned against CPython 3.14.
"""

import inspect
import types


def drive(coro):
    """Run a coroutine to completion and return is (result, error-type)."""
    try:
        while True:
            sent = coro.send(None)
    except StopIteration as e:
        return ("ok", e.value)
    except Exception as e:
        return ("err", "%s: %s" % (type(e).__name__, e))


async def add(a, b):
    return a + b


async def main():
    x = await add(1, 2)
    return x * 10


c = main()
print(type(c).__name__)
print(isinstance(c, types.CoroutineType))
print(inspect.iscoroutine(c))
print(drive(c))


async def pure():
    return 99


print(drive(pure()))

c = add(3, 4)
try:
    c.send(99)
except TypeError as e:
    print("non-none first send:", e)
c.close()

c = add(3, 4)
print(drive(c))
try:
    c.send(None)
except RuntimeError as e:
    print("reuse:", e)

c = add(1, 2)
try:
    next(c)
except TypeError as e:
    print("next:", e)
c.close()
c = add(1, 2)
try:
    iter(c)
except TypeError as e:
    print("iter:", e)
c.close()
c = add(1, 2)
try:
    for x in c:
        pass
except TypeError as e:
    print("for over coroutine:", e)
c.close()


class Suspend:
    def __await__(self):
        yield


async def g():
    return "inner"


async def chain():
    a = await g()
    b = await g()
    return a + b


print(drive(chain()))


class Ctx:
    async def __aenter__(self):
        print("aenter")
        return 42

    async def __aexit__(self, *args):
        print("aexit", args[0])
        return False


async def withbody():
    try:
        async with Ctx() as v:
            print("body", v)
        print("after")
    finally:
        print("finally")


print(drive(withbody()))


class Quiet:
    async def __aenter__(self):
        return None

    async def __aexit__(self, t, v, tb):
        print("quiet-exit", t.__name__ if t else None)
        return True


async def suppressed():
    async with Quiet():
        raise RuntimeError("silenced")
    return "ok"


print(drive(suppressed()))


async def propagating():
    async with Ctx():
        raise ValueError("prop")


c = propagating()
try:
    c.send(None)
except ValueError as e:
    print("propagated:", e)


class AIter:
    def __init__(self, n):
        self.n = n
        self.i = 0

    def __aiter__(self):
        return self

    async def __anext__(self):
        if self.i >= self.n:
            raise StopAsyncIteration
        self.i += 1
        return self.i * 2


async def forbody():
    total = 0
    async for x in AIter(3):
        total += x
    return total


print(drive(forbody()))


async def forbody_orelse():
    got = []
    async for x in AIter(0):
        got.append(x)
    else:
        got.append("else")
    return got


print(drive(forbody_orelse()))


# close() runs a suspended coroutine's finally clauses.
async def closer():
    try:
        await Suspend()
        print("unreachable")
    finally:
        print("closed-finally")


c = closer()
c.send(None)
c.close()
try:
    c.send(None)
except RuntimeError as e:
    print("send after close:", e)

doc = "finished"
