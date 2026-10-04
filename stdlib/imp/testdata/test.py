"""_imp: the import lock.

The three lock functions are what pkg_resources uses, and they are reentrant -
the import machinery holds the lock across an import, and an import may itself
trigger another.
"""

import _imp

print("starts unlocked:", _imp.lock_held() is False)
print("the three names are callable:", all(callable(getattr(_imp, n, None))
      for n in ("acquire_lock", "release_lock", "lock_held")))


def acquire():
    return _imp.acquire_lock()


def release():
    return _imp.release_lock()


def held():
    return _imp.lock_held()


acquire()
print("locked after one acquire:", held() is True)

# Reentrant: a second acquire from the same holder must not deadlock, and one
# release must not unlock it outright.
acquire()
print("still locked after two acquires:", held() is True)
release()
print("still locked after one release:", held() is True)
release()
print("unlocked after two releases:", held() is False)
# Releasing a lock that is not held RAISES in CPython, and raising is right: a
# caller that releases without holding has a bug worth hearing about.
try:
    release()
except RuntimeError as e:
    print("releasing unheld raises:", e)

doc = "finished"
