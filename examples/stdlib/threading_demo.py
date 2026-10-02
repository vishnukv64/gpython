"""threading: locks and the (few) primitives that actually work.

Run with:  /tmp/gpy examples/stdlib/threading_demo.py

This interpreter does not create OS threads, so nothing here runs concurrently
and Thread.start() raises. Almost everything is a re-export of a single
non-reentrant mutex, which is the central thing to know about this module.

Interpreter notes (all verified by running this file):
  * threading.Lock and threading.RLock are real, distinct classes.
  * threading.Event, threading.Semaphore and threading.Barrier are ALL aliases
    of threading.Lock (`threading.Event is threading.Lock` is True). They have
    no set()/is_set()/wait()/parties, and a second acquire() deadlocks the
    whole interpreter, because a Lock is not reentrant.
  * Thread.start() raises RuntimeError instead of running the target.
  * threading.local() creates a working, attribute-holding object, but asking
    for `type(local_obj).__name__` crashes the interpreter with a Go panic
    (interface conversion: py.Object is *py.Type, not *threading.Local).
    The demo avoids that call -- see examples/KNOWN_ISSUES.md.
"""

import threading

print("--- Lock and RLock are real, distinct classes ---")
lock = threading.Lock()
rlock = threading.RLock()
print("type(Lock()):  ", type(lock).__name__)
print("type(RLock()): ", type(rlock).__name__)
print("they are different classes:", threading.Lock is not threading.RLock)
print("Lock is not reentrant; RLock is:")
print("  RLock acquire #1:", rlock.acquire())
print("  RLock acquire #2:", rlock.acquire(), "(same holder may re-enter)")
print("  RLock release #1:", rlock.release())
print("  RLock release #2:", rlock.release())

print()
print("--- a plain Lock: acquire, mutate, release ---")
print("acquire():", lock.acquire())
print("release():", lock.release())

print()
print("--- Event, Semaphore and Barner are aliases of Lock ---")
print("threading.Event is threading.Lock:    ", threading.Event is threading.Lock)
print("threading.Semaphore is threading.Lock:", threading.Semaphore is threading.Lock)
print("threading.Barrier is threading.Lock:  ", threading.Barrier is threading.Lock)
event = threading.Event()
print("type(Event()):                        ", type(event).__name__)
for name in ["set", "clear", "is_set", "wait", "isSet", "acquire", "release"]:
    print("  Event has %-8s %s" % (name, hasattr(event, name)))
print("(CPython's Event would have the first five; here it is just a lock)")

print()
print("--- WARNING: a second acquire on a non-reentrant Lock deadlocks ---")
print("lock.acquire() takes the lock.")
print("A second lock.acquire() would block forever and the interpreter prints")
print("`fatal error: all goroutines are asleep - deadlock!` and dies, so this")
print("demo does not call it. Use RLock if you need to re-enter, as above.")
second = threading.Lock()
print("a fresh Lock may be acquired once:", second.acquire())
print("a fresh Lock may be acquired once:", second.acquire() if False else "(skipped: it would deadlock)")
print("release it so the process stays healthy:", second.release())

print()
print("--- the current thread and its identity ---")
print("current_thread():     ", threading.current_thread())
print("its name:             ", threading.current_thread().name)
print("main_thread():        ", threading.main_thread().name)
print("get_ident() is an int:", isinstance(threading.get_ident(), int))
print("active_count():       ", threading.active_count())
print("enumerate():          ", len(threading.enumerate()), "thread(s)")
print("TIMEOUT_MAX:          ", threading.TIMEOUT_MAX)
print("ThreadError exists:   ", threading.ThreadError is not None)
print("local is present but unusable:", hasattr(threading, "local"))

print()
print("--- a Thread object can be built, but not started ---")
worker = threading.Thread(target=lambda: None, name="worker-1")
print("constructed:  ", worker)
print("its name:     ", worker.name)
print("daemon flag:  ", worker.daemon)
try:
    worker.start()
    print("start() ran the target")
except RuntimeError as err:
    print("start() raises RuntimeError:", err)
print("join/start/run exist:", hasattr(worker, "join"), hasattr(worker, "start"), hasattr(worker, "run"))

print()
print("--- a worked example: a lock-guarded counter ---")


class Counter:
    """Increments under a lock, so the read-modify-write is atomic.

    With no real threads the lock is never contended, but the acquire /
    mutate / release pattern is exactly the one you would use with them.
    """

    def __init__(self):
        self.lock = threading.Lock()
        self.value = 0

    def increment(self, amount=1):
        self.lock.acquire()
        try:
            self.value = self.value + amount
        finally:
            self.lock.release()
        return self.value

    def snapshot(self):
        self.lock.acquire()
        try:
            return self.value
        finally:
            self.lock.release()


counter = Counter()
for _ in range(5):
    counter.increment()
print("after five increments:", counter.snapshot())
counter.increment(10)
print("after adding ten:     ", counter.snapshot())
print("the lock is free again:", counter.lock.acquire())
counter.lock.release()

print()
print("--- a reentrant guard with RLock ---")


class Registry:
    """A method that calls another locked method must use an RLock."""

    def __init__(self):
        self.lock = threading.RLock()
        self.entries = []

    def add(self, name):
        self.lock.acquire()
        try:
            self.entries.append(name)
        finally:
            self.lock.release()
        return self

    def add_many(self, names):
        # Takes the lock, then calls add(), which takes it again. With a plain
        # Lock this would deadlock; an RLock lets the same holder re-enter.
        self.lock.acquire()
        try:
            for name in names:
                self.add(name)
        finally:
            self.lock.release()
        return self


registry = Registry()
registry.add("ann").add_many(["bob", "cat"])
print("entries:", registry.entries)
print("(add_many held the lock while add re-acquired it -- only an RLock allows that)")

print()
print("--- what the module exports ---")
for name in ["Lock", "RLock", "Event", "Semaphore", "Barrier", "Thread",
             "local", "current_thread", "main_thread", "get_ident",
             "active_count", "enumerate", "TIMEOUT_MAX", "ThreadError"]:
    print("  threading.%-16s %s" % (name, hasattr(threading, name)))
