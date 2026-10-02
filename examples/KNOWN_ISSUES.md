# gpython known issues

Interpreter limitations found by actually running the examples in this
directory. Every reproduction below was executed against the interpreter built
from the current checkout (`go build -o /tmp/gpy .`), and the error text is
pasted verbatim from that run.

Two kinds of entry appear here:

- **Crash / hang** — the interpreter panics, deadlocks or loops forever. There
  is no Python-level exception to catch; the process dies or never returns.
  These are the serious ones.
- **Missing or behaving differently** — a clean Python exception, or a silently
  different result. Ordinary to work around once you know.

Version this was measured against: `sys.version` reports
`Gpython dev (none, unknown)`, `sys.version_info` is `(3, 4, 0, 'final', 0)`.

---

## 1. Crash and hang

### 1.1 `hashlib.file_digest()` panics the interpreter

Calling `file_digest` dereferences a nil function pointer. The process dies
with a SIGSEGV; no Python exception is raised.

```python
import hashlib
hashlib.file_digest(open("/tmp/f", "wb"), "md5")
```

```
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x18 pc=...]

goroutine 1 [running]:
github.com/vishnukv64/gpython/py.Call({0x0, 0x0?}, {0x1, ...}, 0x0)
	.../py/internal.go:195 +0x2c8
github.com/vishnukv64/gpython/stdlib/hashlib.fileDigestFn(...)
	.../stdlib/hashlib/hashlib.go:341 +0x454
```

The hash functions themselves (`md5`, `sha256`, `blake2b`, …) work fine — see
`stdlib/hashlib_demo.py`, which hashes file contents it read itself instead.

### 1.2 `pprint.pformat()` on a self-referencing container hangs when `width <= 20`

At the default width a recursive container prints correctly. Narrow it and the
formatter never returns: no output, no exception, no exit.

```python
import pprint
r = []
r.append(r)
print(pprint.pformat(r, width=20))   # never returns
```

Observed boundary, running the same snippet at each width:

| `width` | result |
| --- | --- |
| 80 | prints `[<Recursion on list>]` |
| 40 | prints |
| 21 | prints |
| 20 | **hangs** (killed at 5 s, exit 124) |
| 19 | **hangs** |
| 10 | **hangs** |

The same is true of a self-referencing dict. `pprint.isrecursive()` answers
correctly, so test with that rather than by formatting.

### 1.3 A second `acquire()` on a `threading.Lock` deadlocks the interpreter

`threading.Lock` is not reentrant, which is correct, but the second acquire does
not raise — the runtime detects that every goroutine is blocked and aborts the
process.

```python
import threading
lock = threading.Lock()
lock.acquire()
lock.acquire()      # never returns a value
```

```
held
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [sync.Mutex.Lock]:
internal/sync.runtime_SemacquireMutex(...)
	/opt/homebrew/Cellar/go/1.25.6/libexec/src/runtime/sema.go:95 +0x28
...
```

Use `threading.RLock` when the same holder needs to re-enter.

### 1.4 `type(threading.local()).__name__` panics

`threading.local()` itself works and holds attributes. Asking for the *name* of
its type crashes.

```python
import threading
local = threading.local()
local.x = 1
print(local.x)                      # 1        -- fine
print(type(local))                  # <class 'threading.local'>  -- fine
print(type(local).__name__)         # crashes
```

```
1
panic: interface conversion: py.Object is *py.Type, not *threading.Local

goroutine 1 [running]:
github.com/vishnukv64/gpython/stdlib/threading.init.0.func2(...)
	.../stdlib/threading/threading.go:145 +0x328
```

---

## 2. `threading` is largely a re-export of one mutex

`Event`, `Semaphore` and `Barrier` are not implemented as such — they are all
aliases of `threading.Lock`. This is not an exception you can catch; the names
exist and the objects construct, they just have the wrong API.

```python
import threading
print(threading.Event is threading.Lock)       # True
print(threading.Semaphore is threading.Lock)   # True
print(threading.Barrier is threading.Lock)     # True
```

Consequences:

```python
import threading
e = threading.Event()
e.set()
```

```
AttributeError: 'threading.Lock' object has no attribute 'set'
```

`is_set`, `wait` and `isSet` are absent for the same reason. And because a
`Lock` is not reentrant, the obvious `Semaphore(2)` followed by two acquires
deadlocks exactly as in §1.3.

`Thread.start()` does not run the target:

```python
import threading
threading.Thread(target=lambda: None).start()
```

```
RuntimeError: this interpreter does not create Python threads, so Thread.start() cannot run the target
```

---

## 3. `str` and `bytes` methods that are missing

These all raise `AttributeError`; the encodings they would provide have to be
done through the two-argument `bytes(text, encoding)` builtin or `codecs`.

| Expression | Error |
| --- | --- |
| `"a".encode()` | `AttributeError: 'str' has no attribute 'encode'` |
| `b"a".decode()` | `AttributeError: 'bytes' has no attribute 'decode'` |
| `"{}".format(1)` | `AttributeError: 'str' has no attribute 'format'` |
| `"a b".partition(" ")` | `AttributeError: 'str' has no attribute 'partition'` |
| `"a\nb".splitlines()` | `AttributeError: 'str' has no attribute 'splitlines'` |
| `"ab".capitalize()` | `AttributeError: 'str' has no attribute 'capitalize'` |

Workarounds used in these examples:

- text to bytes: `bytes(text, "utf-8")`
- bytes to text: `codecs.decode(blob, "ascii")[0]` — note the `[0]`, because
  `codecs.decode` returns a `(str, length)` tuple here. See §4.
- splitting lines: `text.split("\n")`

---

## 4. `codecs.encode` / `codecs.decode` return tuples

Both return a `(value, length)` pair rather than the bare value CPython
returns, so the result needs a `[0]`.

```python
import codecs
print(codecs.encode("café", "utf-8"))
```

```
(b'caf\xc3\xa9', 5)
```

```python
print(codecs.decode(b"caf\xc3\xa9", "utf-8"))
```

```
('café', 5)
```

Only `utf-8`, `ascii` and `latin-1` are available; anything else (for example
`codecs.encode(b"hi", "hex")`) raises `LookupError`.

---

## 5. Ordering comparisons are limited to `int`, `float` and `str`

`<`, `>`, `<=` and `>=` are not defined for tuples, lists or sets:

```python
print((1, 2) < (1, 3))
```
```
TypeError: unsupported operand type(s) for <: 'tuple' and 'tuple'
```

```python
print([1] < [2])
```
```
TypeError: unsupported operand type(s) for <: 'list' and 'list'
```

```python
print({1} < {1, 2})
```
```
TypeError: unsupported operand type(s) for <: 'set' and 'set'
```

`==` and `!=` on those types work normally. `sorted()` on a list of numbers or
strings works; sorting a list of tuples does not. This also means
`functools.total_ordering` cannot synthesise the missing comparisons.

---

## 6. Type and instance objects are unhashable

```python
print(hash(int))
```
```
TypeError: unhashable type: 'object'
```

```python
class C:
    pass
print(hash(C))
```
```
TypeError: unhashable type: 'type'
```

```python
class C:
    pass
print(hash(C()))
```
```
TypeError: unhashable type: 'C'
```

So a class cannot be used as a dict key, and `isinstance` dispatch has to be
written out rather than table-driven on `type(value)` (see
`stdlib/types_demo.py`).

Relatedly, the type object of a Python function reports a *property*, not a
name:

```python
def f():
    pass
print(type(f).__name__)
```
```
<property instance at 0x140000eeb40>
```

`f.__name__` itself is the correct string `'f'`; only the route through `type()`
is broken.

An explicit `__hash__` method satisfies `hash()` but does **not** make the
object usable in a `WeakSet` or `WeakKeyDictionary`:

```python
import weakref

class H:
    def __hash__(self):
        return id(self)

weakref.WeakSet().add(H())
```
```
TypeError: unhashable type: 'H'
```

---

## 7. `weakref` containers

`WeakValueDictionary` works, because its keys are plain strings.
`WeakSet.add()` and `WeakKeyDictionary[k] = v` both hash the member and fail
with the error in §6.

```python
import weakref
class C:
    pass
weakref.WeakSet().add(C())
```
```
TypeError: unhashable type: 'C'
```

```python
import weakref
class C:
    pass
weakref.WeakKeyDictionary()[C()] = 1
```
```
TypeError: unhashable type: 'C'
```

Also: `weakref.getweakrefcount()` and `getweakrefs()` always report `0` and
`[]` even right after refs have been created, `WeakValueDictionary` has no
`items()`, and there is no `gc` module at all:

```python
import gc
```
```
ModuleNotFoundError: No module named "gc"
```

---

## 8. `os` gaps

`os.stat` and `os.lstat` do not exist, so there is no `st_mode`, size or
timestamp to read off a file:

```python
import os
os.stat("/tmp")
```
```
AttributeError: 'module' has no attribute 'stat'
```

`os.path.getsize`, `isfile`, `isdir` and `exists` do work and cover most needs.

Filesystem failures raise `SystemError`, not `OSError`:

```python
import os
os.remove("/no/such/file")
```
```
SystemError: remove /no/such: no such file or directory
```

```python
import os
os.mkdir("/tmp")
```
```
SystemError: mkdir /tmp: file exists
```

`os.write` is absent (`AttributeError: 'module' has no attribute 'write'`), so
a file descriptor from `tempfile.mkstemp()` has to be wrapped with
`os.fdopen(fd, "w")` before writing.

---

## 9. `time` is mostly unimplemented

Only `time()`, `time_ns()`, `sleep()` and `clock()` are implemented. Everything
else raises `NotImplementedError` with an empty message:

| Call | Error |
| --- | --- |
| `time.gmtime()` | `NotImplementedError:` |
| `time.localtime()` | `NotImplementedError:` |
| `time.mktime(t)` | `NotImplementedError:` |
| `time.ctime(t)` | `NotImplementedError:` |
| `time.asctime(t)` | `NotImplementedError:` |
| `time.strftime(fmt)` | `NotImplementedError:` |
| `time.strptime(s, fmt)` | `NotImplementedError:` |
| `time.monotonic()` | `NotImplementedError:` |
| `time.perf_counter()` | `NotImplementedError:` |
| `time.process_time()` | `NotImplementedError:` |
| `time.get_clock_info("time")` | `NotImplementedError:` |
| `time.tzset()` | `NotImplementedError:` |

Use the `datetime` module for formatting and calendar work.

---

## 10. `sys` gaps

```python
import sys
sys.getsizeof(1)
```
```
NotImplementedError:
```

The same empty `NotImplementedError` comes from `sys.getrecursionlimit()`,
`sys.setrecursionlimit()`, `sys.intern()` and `sys.getdefaultencoding()`.

`sys.modules` exists but is always empty, so nothing is ever registered in a
module cache:

```python
import sys, json
print(len(sys.modules), sys.modules.get("json"))
```
```
0 None
```

Module objects otherwise work: `import_module` and `__import__` both hand back
a usable module.

---

## 11. `math` integer helpers are absent

These raise `AttributeError: 'module' has no attribute '<name>'`:
`gcd`, `lcm`, `comb`, `perm`, `isqrt`, `isclose`, `prod`, `nextafter`, `tau`.

```python
import math
math.gcd(4, 6)
```
```
AttributeError: 'module' has no attribute 'gcd'
```

The libm-shaped functions (`sqrt`, `log`, `fsum`, `hypot`, `erf`, …) are all
present and correct.

---

## 12. `typing` introspection does not resolve

`get_type_hints()` returns an empty dict, and `get_origin`/`get_args` hand back
the alias itself rather than the `(origin, args)` pair:

```python
import typing
print(typing.get_type_hints(lambda a: None))
print(typing.get_origin(typing.List[int]))
```
```
{}
typing.List[int]
```

A functional `NamedTuple` builds, but its fields are not attributes and it is
not a tuple:

```python
import typing
N = typing.NamedTuple("N", [("x", int)])
print(N(1).x)
```
```
AttributeError: 'N' object has no attribute 'x'
```

Index it (`N(1)[0]`) or use `_asdict()`. The class-based form fails earlier:

```python
import typing
class C(typing.NamedTuple):
    x: int
```
```
TypeError: cannot create 'method' instances
```

And a `TypedDict` cannot be instantiated:

```python
import typing
T = typing.TypedDict("T", {"a": int})
T(a=1)
```
```
TypeError: cannot create 'T' instances
```

---

## 13. `types.MappingProxyType` is unusable

```python
import types
types.MappingProxyType({"a": 1})
```
```
TypeError: non-tuple sequence
```

`types.SimpleNamespace`, `types.new_class` and the `*Type` names all work.

---

## 14. Small signatures that differ

### `textwrap.shorten` needs a positional width

```python
import textwrap
textwrap.shorten("a b c", width=5)
```
```
TypeError: shorten() missing 1 required positional argument: 'width'
```

`textwrap.shorten(text, 5)` works. `wrap()` and `fill()` accept `width=` as a
keyword normally.

### `csv.register_dialect` accepts only a name

```python
import csv
csv.register_dialect("mine", delimiter=";")
```
```
TypeError: register_dialect() takes no keyword arguments
```

```python
csv.register_dialect("mine", ";")
```
```
TypeError: register_dialect() takes exactly 1 arguments (2 given)
```

`csv.register_dialect("mine")` alone succeeds, but the dialect it registers
carries no settings. `csv.DictReader` and `csv.DictWriter` are also absent
(`AttributeError: 'module' has no attribute 'DictReader'`).

### `re.X` / `re.VERBOSE` does not strip whitespace or comments

The flag is accepted, but a pattern relying on it fails to match:

```python
import re
print(re.compile(r"\d+  # a number", re.X).match("12"))
```
```
None
```

`re.I`, `re.M` and `re.S` behave correctly.

### `datetime` arithmetic

`timedelta` objects can be built and inspected, but not combined or compared:

```python
from datetime import datetime, timedelta
print(datetime(2023, 5, 17) + timedelta(days=1))
```
```
TypeError: unsupported operand type(s) for +: 'datetime.datetime' and 'datetime.timedelta'
```

`date.replace()`, `datetime.timetuple()` and `fromisoformat()` are absent.
`datetime.now()`, `timestamp()`, `strftime` and `strptime` all work.

### `marshal.dump` / `marshal.load` are not implemented

```python
import marshal
marshal.dump(1, None)
```
```
SystemError: dump not implemented
```

`dumps`/`loads` work, including for nested containers. Code objects are not
marshallable — `marshal.dumps(compile("x=1", "<s>", "exec"))` raises
`ValueError: unmarshallable object`, where CPython round-trips a `.pyc`
through exactly that call.

### `os.path` extras

`os.path.normpath`, `relpath`, `getsize`, `isfile`, `isdir` and `exists` all
work. There is no `os.path.commonpath`:

```python
import os
os.path.commonpath(["/a/b", "/a/c"])
```
```
AttributeError: 'module' has no attribute 'commonpath'
```

---

## 15. Modules that do not exist

`import gc`, `import operator`, `import builtin`, `import abcmachinery` and
`import future` all fail with
`ModuleNotFoundError: No module named "<name>"`. The two Go packages
`stdlib/abcmachinery/` and `stdlib/future/` are internal machinery (backing
`abc` and `__future__`) and are not importable under those names. `stdlib/builtin/`
registers as **`builtins`**, which is what you import.

---

## 16. Two things that look like bugs but are not

Recorded so nobody re-reports them:

- **`dict` iteration order is not insertion order.** Keys come back in an
  arbitrary order, so no example here depends on it; where a stable rendering
  was needed the demo sorts explicitly. This is a known, separately-tracked
  issue.
- **`inspect.signature()` raises `NotImplementedError`** with the message
  `inspect.signature is not implemented: this interpreter does not keep the
  argument binding needed to report it`. Expected, not a surprise.

`inspect.getmembers()` is also absent, and a Python-level function's `__doc__`
is the fixed string `"A python function"` rather than its real docstring —
class docstrings and builtin docstrings are correct.
