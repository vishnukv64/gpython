"""threading.local subclass: class-body defaults, per-instance shadowing."""

import threading


class Holder(threading.local):
    buffer_index = 0
    name = "default"


h = Holder()
print("type:", type(h).__name__)
print("default:", h.buffer_index, h.name)

h.buffer_index = 7
print("after set:", h.buffer_index, "class:", Holder.buffer_index)

# A second holder is unaffected: the assignment went to THIS holder.
print("fresh:", Holder().buffer_index)

# A plain local still works.
plain = threading.local()
plain.x = 1
print("plain:", plain.x, plain.__dict__)

# A subclass is still a threading.local.
print("isinstance:", isinstance(h, threading.local))

doc = "finished"
