# The behaviour of concurrent.futures, checked through the interpreter.
import concurrent.futures
import threading
import time

from concurrent.futures import (
    ThreadPoolExecutor, as_completed, wait, Future,
    ProcessPoolExecutor, CancelledError, TimeoutError,
    FIRST_COMPLETED, FIRST_EXCEPTION, ALL_COMPLETED,
)

# ---- ThreadPoolExecutor really runs the task and returns its value
e = ThreadPoolExecutor(2)
assert e.submit(lambda x: x * 2, 21).result() == 42

# ---- the pool actually ran N tasks on threads, not one at a time.
#
# Each task increments a shared counter and then spins until the counter
# reaches N.  That can only happen if the tasks are inside the functions at the
# same time, which requires more than one thread of control.
N = 4
lock = threading.Lock()
state = {'n': 0}
def rendezvous(i):
    with lock:
        state['n'] += 1
    deadline = time.time() + 5.0
    while state['n'] < N:
        if time.time() > deadline:
            return 'TIMEOUT'
        time.sleep(0.001)
    return i

with ThreadPoolExecutor(N) as pool:
    futures = [pool.submit(rendezvous, i) for i in range(N)]
    results = [f.result() for f in futures]
assert sorted(results) == list(range(N)), results
# and each ran on a DIFFERENT thread than the one that submitted them.
for f in futures:
    assert f.done() is True
    assert f.cancelled() is False
    assert f.running() is False

# ---- map yields the results in order
with ThreadPoolExecutor(3) as pool:
    assert list(pool.map(lambda x: x * x, [1, 2, 3, 4])) == [1, 4, 9, 16]
    # map over two iterables is map over the zipped pair
    assert list(pool.map(lambda a, b: a + b, [1, 2], [10, 20])) == [11, 22]

# ---- a task that raises propagates its exception type through result()
with ThreadPoolExecutor(1) as pool:
    f = pool.submit(lambda: 1 // 0)
    try:
        f.result()
    except ZeroDivisionError:
        pass
    else:
        raise AssertionError('result() did not raise ZeroDivisionError')
    # exception() answers the instance rather than raising
    exc = f.exception()
    assert isinstance(exc, ZeroDivisionError), exc
    assert f.done() is True

# ---- a successful future has exception() None
with ThreadPoolExecutor(1) as pool:
    f = pool.submit(lambda: 'ok')
    assert f.result() == 'ok'
    assert f.exception() is None

# ---- result(timeout=...) raises TimeoutError while the task is still running.
# threading.Event is not a real Event in this interpreter (it is a Lock), so
# the task blocks on time.sleep instead.
with ThreadPoolExecutor(1) as pool:
    f = pool.submit(lambda: time.sleep(0.5) or 'late')
    try:
        f.result(timeout=0.05)
    except TimeoutError:
        pass
    else:
        raise AssertionError('result(timeout) did not raise TimeoutError')
    assert f.result(timeout=5) == 'late'

# TimeoutError is the BUILTIN, as CPython keeps it.
assert TimeoutError is concurrent.futures.TimeoutError

# ---- cancel
with ThreadPoolExecutor(1) as pool:
    held = pool.submit(lambda: time.sleep(0.4) or 'held')
    queued = pool.submit(lambda: 'never')
    assert queued.cancel() is True
    assert queued.cancelled() is True
    assert queued.done() is True
    try:
        queued.result()
    except CancelledError:
        pass
    else:
        raise AssertionError('cancelled future gave a result')
    assert held.result(timeout=5) == 'held'

# A standalone future starts pending and is cancellable.
f = Future()
assert f.done() is False
assert f.cancelled() is False
assert f.running() is False
assert f.cancel() is True
assert f.cancelled() is True
assert f.cancel() is True

# set_result/set_exception complete a standalone future.
f = Future()
f.set_result(7)
assert f.result() == 7
f = Future()
f.set_exception(ValueError('bad'))
try:
    f.result()
except ValueError:
    pass
else:
    raise AssertionError('set_exception did not surface')

# ---- add_done_callback runs when the future completes
with ThreadPoolExecutor(1) as pool:
    seen = []
    f = pool.submit(lambda: 5)
    f.add_done_callback(lambda fut: seen.append(fut.result()))
    assert f.result() == 5
    # The callback runs on the worker; give it a moment, then it must have run.
    deadline = time.time() + 5.0
    while not seen and time.time() < deadline:
        time.sleep(0.005)
    assert seen == [5], seen

    # A callback added to an already-finished future runs immediately.
    seen2 = []
    f2 = pool.submit(lambda: 6)
    f2.result()
    f2.add_done_callback(lambda fut: seen2.append(fut.result()))
    assert seen2 == [6], seen2

# ---- wait and as_completed
with ThreadPoolExecutor(4) as pool:
    slow = [pool.submit(lambda: time.sleep(0.25) or 'slow') for _ in range(2)]
    fast = pool.submit(lambda: 'fast')

    done, not_done = wait([fast], return_when=ALL_COMPLETED)
    assert len(done) == 1 and len(not_done) == 0, (done, not_done)

    done, not_done = wait([fast] + slow, timeout=0.05, return_when=FIRST_COMPLETED)
    assert len(done) >= 1, (done, not_done)

    done, not_done = wait(slow, return_when=ALL_COMPLETED)
    assert len(done) == 2 and len(not_done) == 0, (done, not_done)

    completed = [f.result() for f in as_completed([fast] + slow)]
    assert sorted(completed) == ['fast', 'slow', 'slow'], completed

# A duplicate future is refused, because (done, not_done) must partition.
f = Future()
f.set_result(1)
try:
    wait([f, f])
except TypeError:
    pass
else:
    raise AssertionError('wait accepted a duplicate future')

# ---- the context manager shuts the pool down
with ThreadPoolExecutor(2) as pool:
    assert pool.submit(lambda: 1).result() == 1
try:
    pool.submit(lambda: 2)
except RuntimeError:
    pass
else:
    raise AssertionError('a shut-down executor accepted work')

# ---- ProcessPoolExecutor refuses, by name, rather than faking it
try:
    ProcessPoolExecutor()
except NotImplementedError as exc:
    assert 'ProcessPoolExecutor' in str(exc), str(exc)
else:
    raise AssertionError('ProcessPoolExecutor did not raise NotImplementedError')

# ---- the exception hierarchy callers catch by name
assert issubclass(CancelledError, Exception)
assert issubclass(concurrent.futures.InvalidStateError, Exception)
assert issubclass(concurrent.futures.BrokenExecutor, RuntimeError)

# _base is importable and carries Error, which 3.14 no longer re-exports.
import concurrent.futures._base as base
assert base.Error is concurrent.futures._base.Error
assert issubclass(CancelledError, base.Error)

print("concurrent.futures ok")
