"""errno: standard system error numbers and their messages.

Run with:  /tmp/gpy examples/stdlib/errno_demo.py
"""

import errno
import os

print("--- common errno constants ---")
for name in ["EPERM", "ENOENT", "ESRCH", "EINTR", "EIO", "EACCES", "EEXIST",
             "EISDIR", "ENOTDIR", "EINVAL", "EMFILE", "ENOSPC",
             "EPIPE", "EAGAIN", "EWOULDBLOCK", "ETIMEDOUT"]:
    print("  %-12s = %s" % (name, getattr(errno, name, "not defined here")))

print()
print("--- strerror turns a number into a message ---")
for number in [errno.EPERM, errno.ENOENT, errno.EACCES, errno.EEXIST, errno.EINVAL]:
    print("  %-3d %s" % (number, errno.strerror(number)))

print()
print("--- errorcode maps a number back to its name ---")
for number in [errno.ENOENT, errno.EEXIST, errno.EACCES, errno.EAGAIN]:
    print("  %-3d -> %s" % (number, errno.errorcode[number]))

print()
print("--- EAGAIN and EWOULDBLOCK are the same number here ---")
print("EAGAIN:      ", errno.EAGAIN)
print("EWOULDBLOCK: ", errno.EWOULDBLOCK)
print("equal:       ", errno.EAGAIN == errno.EWOULDBLOCK)

print()
print("--- how errno shows up in practice: catching OSError ---")
# In CPython an OSError carries .errno; this interpreter's OSError has no
# errno attribute, so the numeric code is not reachable from the exception
# object here. Catching by the concrete subclass still works.
try:
    open("/absolutely/no/such/path/at/all")
except FileNotFoundError as err:
    print("FileNotFoundError caught; its str is:", err)
    print("  (errno.ENOENT is", errno.ENOENT, "=", errno.strerror(errno.ENOENT), ")")

try:
    os.mkdir("/tmp")
except (OSError, SystemError) as err:
    # NOTE: this interpreter raises a SystemError for a failing os.mkdir,
    # not the OSError that CPython raises.
    print("os.mkdir on an existing directory raised", type(err))
    print("  (EEXIST is", errno.EEXIST, "=", errno.strerror(errno.EEXIST), ")")

print()
print("--- the full list is large; count what is defined ---")
names = sorted([n for n in dir(errno) if n.startswith("E")])
print("names starting with E: ", len(names))
print("first ten:              ", names[:10])
print("errorcode entries:      ", len(errno.errorcode))

print()
print("--- a worked example: a friendly error reporter ---")


def explain(exception):
    """Turn a caught exception into a friendly sentence."""
    name = "unknown"
    # Try to recover a numeric errno from the message, since the exception
    # object itself does not expose .errno in this interpreter.
    message = str(exception)
    for candidate in names:
        number = getattr(errno, candidate)
        if str(number) in message:
            name = candidate
            break
    text = errno.strerror(getattr(errno, name)) if name != "unknown" else message
    return "%s (%s)" % (text, name)


for path in ["/no/such/file", "/tmp"]:
    try:
        handle = open(path)
        handle.close()
        print("  %-16s opened fine" % path)
    except Exception as err:
        print("  %-16s -> %s" % (path, type(err)))
        print("  %-16s    %s" % ("", explain(err)))

print()
print("--- constants are plain ints ---")
print("isinstance(errno.ENOENT, int):", isinstance(errno.ENOENT, int))
print("arithmetic works:            ", errno.ENOENT + 0)
print("building a message:          ", "error %d: %s" % (errno.EACCES, errno.strerror(errno.EACCES)))
