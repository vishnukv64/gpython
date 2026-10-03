import logging, logging.handlers as H, os
import tempfile
os.chdir(tempfile.mkdtemp())
h = H.RotatingFileHandler("r.log", maxBytes=150, backupCount=2)
lg = logging.getLogger("rot"); lg.addHandler(h); lg.setLevel(logging.DEBUG)
for i in range(12):
    lg.warning("message %d", i)
print("files:", sorted(f for f in os.listdir(".") if f.startswith("r.log")))
print("lines:", len(open("r.log").read().strip().splitlines()))
print("first line:", repr(open("r.log").read().strip().splitlines()[0]))
mh = H.MemoryHandler(capacity=100, target=logging.NullHandler())
lg2 = logging.getLogger("mem"); lg2.addHandler(mh); lg2.setLevel(logging.DEBUG)
lg2.warning("held")
print("buffered:", len(mh.buffer))
mh.close()
print("after close:", len(mh.buffer))
# logging.handlers exports NONE of the parent's classes in CPython.
print("exports parent classes:", any(hasattr(H, n) for n in ("Handler", "StreamHandler", "FileHandler", "NullHandler")))
# Handlers that need a platform service are PRESENT here, where CPython has
# them too; whether constructing one works is a platform question, so the test
# only asserts the name exists.
print("SysLogHandler present:", hasattr(H, "SysLogHandler"))

doc = "finished"
