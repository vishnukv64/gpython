import asyncio, sys, types

async def agen(n):
    try:
        for i in range(n):
            await asyncio.sleep(0)
            yield i
    finally:
        print("agen finally")

async def echo():
    got = None
    while True:
        got = yield got
        if got == "stop":
            return

async def main():
    print([x async for x in agen(3)])
    print([x async for x in agen(6) if x % 2])
    print(await asyncio.gather(*[asyncio.sleep(0, i) async for i in agen(3)]))
    it = agen(5)
    async for x in it:
        if x == 1:
            break
    await it.aclose()
    e = echo()
    print(await e.__anext__(), await e.asend(7), await e.asend(8))
    try:
        await e.asend("stop")
    except StopAsyncIteration:
        print("stopped")
    g = agen(3)
    await g.__anext__()
    await g.aclose()
    print(type(g).__name__, hasattr(g, "__aiter__"))
    g = agen(3)
    await g.__anext__()
    try:
        await g.athrow(KeyError("k"))
    except KeyError as ex:
        print("athrow", ex)
    return "ok"

print(asyncio.run(main()))

@types.coroutine
def old():
    x = yield "hello"
    return x
async def user():
    return await old()
c = user()
print(c.send(None))
try:
    c.send(5)
except StopIteration as e:
    print(e.value)

def plain():
    yield 1
async def bad():
    await plain()
try:
    bad().send(None)
except TypeError as e:
    print(e)

# hooks
seen = []
sys.set_asyncgen_hooks(firstiter=lambda g: seen.append("first"), finalizer=lambda g: seen.append("fin"))
h = sys.get_asyncgen_hooks()
print(h[0] is not None, h[1] is not None)
async def one():
    yield 1
a = one().__anext__()
try:
    a.send(None)
except StopIteration as e:
    print(e.value)
print(seen)
sys.set_asyncgen_hooks(None, None)
print(tuple(sys.get_asyncgen_hooks()))
