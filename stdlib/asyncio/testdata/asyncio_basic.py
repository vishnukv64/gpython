import asyncio

async def worker(name, delay, out):
    await asyncio.sleep(delay)
    out.append(name)
    return name.upper()

async def failing():
    await asyncio.sleep(0)
    raise ValueError("boom")

async def main():
    out = []
    print(await asyncio.gather(worker("a", 0.3, out), worker("b", 0.1, out), worker("c", 0.2, out)))
    print(out)

    r = await asyncio.gather(worker("x", 0, out), failing(), return_exceptions=True)
    print(r[0], repr(r[1]))
    try:
        await asyncio.gather(failing())
    except ValueError as e:
        print("gather raised", e)

    t = asyncio.create_task(worker("t", 0.01, out), name="mytask")
    print(t.get_name(), t.done())
    print(await t, t.done(), t.result())

    # wait_for / timeout
    try:
        await asyncio.wait_for(asyncio.sleep(5), 0.02)
    except asyncio.TimeoutError:
        print("wait_for timeout")
    try:
        async with asyncio.timeout(0.02):
            await asyncio.sleep(5)
    except TimeoutError:
        print("timeout ctx")
    print(await asyncio.wait_for(worker("w", 0.01, out), 1))

    # cancellation
    async def sleeper():
        try:
            await asyncio.sleep(10)
        except asyncio.CancelledError:
            print("sleeper cancelled")
            raise
        finally:
            print("sleeper cleanup")
    t = asyncio.create_task(sleeper())
    await asyncio.sleep(0.01)
    t.cancel()
    try:
        await t
    except asyncio.CancelledError:
        print("await cancelled", t.cancelled())

    # Queue
    q = asyncio.Queue(maxsize=2)
    async def producer():
        for i in range(5):
            await q.put(i)
        await q.put(None)
    async def consumer():
        got = []
        while (v := await q.get()) is not None:
            got.append(v)
        return got
    p = asyncio.create_task(producer())
    print(await consumer())
    await p

    # Event, Lock, Semaphore
    ev = asyncio.Event()
    async def waiter(i):
        await ev.wait()
        return i
    ws = [asyncio.create_task(waiter(i)) for i in range(3)]
    await asyncio.sleep(0)
    ev.set()
    print(await asyncio.gather(*ws))

    lock = asyncio.Lock()
    order = []
    async def locked(i):
        async with lock:
            order.append(("in", i))
            await asyncio.sleep(0.005)
            order.append(("out", i))
    await asyncio.gather(*(locked(i) for i in range(3)))
    print(order)

    sem = asyncio.Semaphore(2)
    cur = mx = 0
    async def semtask():
        nonlocal cur, mx
        async with sem:
            cur += 1
            mx = max(mx, cur)
            await asyncio.sleep(0.005)
            cur -= 1
    await asyncio.gather(*(semtask() for _ in range(6)))
    print("max concurrent", mx)

    # TaskGroup
    results = []
    async with asyncio.TaskGroup() as tg:
        for i in range(3):
            tg.create_task(worker(f"g{i}", 0.1 * (3 - i), results))
    print(results)
    try:
        async with asyncio.TaskGroup() as tg:
            tg.create_task(failing())
            tg.create_task(sleeper())
    except ExceptionGroup as eg:
        print("TaskGroup", [str(e) for e in eg.exceptions])

    # run_in_executor, call_soon, call_later
    loop = asyncio.get_running_loop()
    print(await loop.run_in_executor(None, sum, [1, 2, 3]))
    fut = loop.create_future()
    loop.call_soon(fut.set_result, "soon")
    print(await fut)
    fut = loop.create_future()
    loop.call_later(0.01, fut.set_result, "later")
    print(await fut)
    done, pending = await asyncio.wait([asyncio.create_task(worker("p", 0.001, out))])
    print(len(done), len(pending))

print(asyncio.run(main()))
