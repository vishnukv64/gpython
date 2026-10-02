"""atexit: run cleanup functions when the interpreter exits.

Run with:  /tmp/gpy examples/stdlib/atexit_demo.py

Handlers run in reverse order of registration, after the last statement of the
program. They are useful for flushing state you cannot wrap in a `with` block.
"""

import atexit

print("--- registering handlers ---")
print("register returns the function:", atexit.register(lambda: None).__class__.__name__)

# These run at interpreter shutdown, after the final print() below.
atexit.register(lambda: print("  [handler 1] closing the audit log"))
atexit.register(lambda: print("  [handler 2] deleting the scratch directory"))
atexit.register(lambda: print("  [handler 3] flushing the metrics buffer"))

print("three handlers registered")


def noisy():
    print("  [handler 4] this one gets unregistered")

print("unregister is available:", callable(atexit.unregister))
atexit.register(noisy)
atexit.unregister(noisy)
print("handler 4 was unregistered, so it never runs")

print()
print("--- a realistic use: recording a temporary file for cleanup ---")
import os
import tempfile

scratch = tempfile.mkdtemp()
path = os.path.join(scratch, "work.txt")
with open(path, "w") as handle:
    handle.write("intermediate results\n")
print("created:", path)
print("exists before exit:", os.path.exists(path))


def cleanup():
    os.remove(path)
    os.rmdir(scratch)
    print("  [handler 5] removed", path)

atexit.register(cleanup)

print()
print("--- end of program ---")
print("handlers now fire in reverse registration order:")
print("(5, 3, 2, 1)")
