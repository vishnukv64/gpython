import threading

e = threading.Event()
print("unset:", e.is_set())
e.set()
print("set:", e.is_set())
print("wait when set:", e.wait())
e.clear()
print("cleared:", e.is_set())

s = threading.Semaphore(2)
print("sem acquire:", s.acquire(), s.acquire())
s.release()
print("released, acquire again:", s.acquire())
# Balanced: release the two still held before using the context manager, or it
# would block for ever with no other thread to release - as it does in CPython.
s.release()
s.release()
with s:
    print("context manager works")

c = threading.Condition()
with c:
    print("condition context works")
print("has notify:", hasattr(c, "notify"), hasattr(c, "notify_all"))

b = threading.Barrier(2)
print("barrier parties:", b.parties)

print("Event is Lock:", threading.Event is threading.Lock)

doc = "finished"
