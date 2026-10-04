# gpython known issues

Interpreter limitations found by **running** the examples in this directory.
Every entry below was re-verified against the interpreter built from this
checkout; anything that had been fixed is listed under "Fixed since" at the end
rather than left here as a stale claim.

The comparison reference is **CPython 3.14** (`python3`), because this
interpreter reports `sys.version_info` **3.10** and a 3.10-era interpreter must
be measured against one that can express what it accepts.  `/usr/bin/python3` on
a stock macOS is 3.9 and cannot parse `match` at all — a reference that errors is
not a reference that disagrees.

## Still missing

### Language

- **PEP 695 type syntax is not implemented** — `type X = int` and
  `class C[T]:` are SyntaxErrors. That is consistent with the 3.10 the
  interpreter reports (both are 3.12 features).
- **`async` is a hard keyword**, as it is in CPython 3.7+, and **async
  generators are not supported** (`async def` containing `yield`).
  `async def`, `await`, `async with` and `async for` all work; there is no
  event loop, so a coroutine is driven by hand (`coro.send(None)`).

### Standard library

- **`mmap` is absent.** This is what stops `pip install`: the network path
  imports it through `pip._vendor.cachecontrol`, so the failure is a
  `ModuleNotFoundError` at import rather than anything about downloading.
- **`csv.register_dialect` takes no keyword arguments** — it accepts a name
  only, so `csv.register_dialect("x", delimiter=";")` is a TypeError.
- **`asyncio` is absent**, and so is every event loop. It is not merely
  unimplemented: it needs `selectors` and a socket layer that is not there.

### Not attempted

- **C extension modules (`.so`/`.pyd`) cannot be loaded.** Packages with
  compiled dependencies are out of reach; this is the barrier the upstream
  README describes.

## Behaviour that differs, and cannot be made identical

- **Set iteration order for non-integer members.** CPython randomises string
  hashes per process, so `list(set(["a","b","c"]))` genuinely differs between
  two runs of CPython itself. For small integers gpython follows CPython's slot
  rule (`hash & 7`) and agrees in about half of random cases; the remainder
  needs CPython's exact probing and resize behaviour, which is an implementation
  detail of an unordered collection.
- **Iterator class names.** CPython 3.14 says `list_iterator` and
  `str_ascii_iterator`; 3.9 and 3.10 say `iterator`, which is what gpython
  reports.
- **Memory addresses** in any repr, and **timing** in any benchmark.

## Fixed since the previous revision of this file

- **`async def`, `await`, `async with` and `async for`** now parse and run, with
  a real coroutine type (`types.CoroutineType`, `inspect.iscoroutine`) and
  `StopAsyncIteration`.  There is still no event loop.
- **`str.rsplit`** works.
- **`threading.Condition`** exists, alongside `Event`, `Semaphore` and
  `Barrier`, which are now real primitives rather than aliases of `Lock`.
- **`sys.version_info` and `sys.implementation.version`** are real struct
  sequences, so `.major`/`.minor` work as 3.9+ code spells them; and
  `platform.python_version_tuple()` returns strings read from the live sys
  rather than hardcoded ints that had drifted to 3.4.0.
- **`\b`, `\B` and `\A`** in a regex are word/string boundaries.  The
  translator used to write the letter without the backslash, so `"\bnumpy"`
  became the literal `"bnumpy"` — every boundary in every pattern, silently.
- **A Python subclass of an exception can hold attributes**, and an
  `Exception.__init__` may take keyword-only arguments.


Everything that used to be listed here and no longer is. Each was re-verified by
running it:

- All four crash/hang entries: `hashlib.file_digest`, `pprint.pformat` on a
  self-referencing container, `type(threading.local())`, and a double
  `threading.Lock.acquire`.
- `str`/`bytes` methods: `zfill`, `center`, `ljust`, `rjust`, `expandtabs`,
  `translate`, `maketrans`, `removeprefix`, `removesuffix`, `rfind`, `rindex`,
  `partition`, `rpartition`, `casefold`, `swapcase`.
- `codecs.encode`/`decode` no longer return tuples at module level.
- Ordering comparisons work for `list`, `tuple`, `set` and user classes with
  `__lt__`, not only `int`/`float`/`str`.
- Type and instance objects are hashable, and so are functions, methods and
  modules.
- `weakref.ref` and containers.
- `time.strftime`, `time.sleep`, `time.monotonic`.
- `math.isqrt`, `math.gcd`, `math.comb`.
- `typing.get_type_hints`.
- `types.MappingProxyType` is usable.
- `sys.meta_path`, `sys.version_info`, `sys.modules`, `sys.executable`.
- `os.path.abspath`, `os.path.expandvars`, `os.path.lexists`, and
  `os.path.join` follows POSIX's rule for an absolute component.
- `csv.register_dialect` without keywords; `textwrap.shorten`;
  `datetime` arithmetic; `marshal.dumps`/`loads`; `re.VERBOSE`.
- The modules that used to be missing — including `plistlib`, `zipimport`,
  `_imp` and `importlib.abc` — now exist.

## What does work, which used to be the headline problem

`pip` runs: `pip --version`, `pip --help` and `pip list` all produce output
matching CPython. `import re` works, including lookbehind. Relative imports
inside a package work. `-c`, `-m <module>`, `-m <package>` and `PYTHONPATH` all
work. Packages get `__path__`, and namespace packages (PEP 420) work with
`__path__` collecting every matching `sys.path` entry.
