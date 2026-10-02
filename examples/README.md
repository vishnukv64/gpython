# gpython examples

Runnable programs for this interpreter. Every file listed here was executed
against a build of the current checkout and exits 0.

Build the interpreter once:

```bash
go build -o /tmp/gpy .
```

Then run any example by path:

```bash
/tmp/gpy examples/python/basics/hello.py
```

Some examples write to `/tmp` and clean up after themselves; none take
arguments except `sys_demo.py`, which echoes whatever extra arguments you give
it. Interpreter limitations that these examples work around — including four
that crash, hang or deadlock the interpreter — are documented in
[KNOWN_ISSUES.md](KNOWN_ISSUES.md).

---

## Language basics — `examples/python/basics/`

| Example | What it shows |
| --- | --- |
| `hello.py` | Hello world: the smallest complete gpython program. |
| `types.py` | Core scalar types: int, float, bool, None, and how they convert. |
| `strings.py` | Strings: indexing, slicing, the supported methods, and formatting. |
| `lists.py` | Lists: creation, indexing, slicing, mutation, and the supported methods. |
| `tuples_sets.py` | Tuples and sets: immutable sequences and unordered unique collections. |
| `dicts.py` | Dictionaries: creation, lookup, iteration, and what is available here. |
| `control_flow.py` | if/elif/else, while, for, break, continue, and loop `else`. |
| `functions.py` | Arguments, defaults, `*args`/`**kwargs`, lambdas, closures, decorators. |
| `comprehensions.py` | List, dict, set and generator expressions. |
| `classes.py` | Inheritance, `super()`, properties, static/class methods, dunders. |
| `exceptions.py` | try/except/else/finally, raise, and what is broken here. |
| `with_statement.py` | The context manager protocol. |
| `generators.py` | `yield`, lazy evaluation, `send`, `throw`, `yield from`, `close`. |
| `iterators.py` | The iteration protocol, `iter()`/`next()`, and iterator tools. |
| `f_strings.py` | Formatted string literals (PEP 498). |
| `annotations.py` | PEP 526 variable annotations and PEP 563 deferred annotations. |
| `match_stmt.py` | PEP 634 structural pattern matching, including guards. |

```bash
/tmp/gpy examples/python/basics/hello.py
/tmp/gpy examples/python/basics/types.py
/tmp/gpy examples/python/basics/strings.py
/tmp/gpy examples/python/basics/lists.py
/tmp/gpy examples/python/basics/tuples_sets.py
/tmp/gpy examples/python/basics/dicts.py
/tmp/gpy examples/python/basics/control_flow.py
/tmp/gpy examples/python/basics/functions.py
/tmp/gpy examples/python/basics/comprehensions.py
/tmp/gpy examples/python/basics/classes.py
/tmp/gpy examples/python/basics/exceptions.py
/tmp/gpy examples/python/basics/with_statement.py
/tmp/gpy examples/python/basics/generators.py
/tmp/gpy examples/python/basics/iterators.py
/tmp/gpy examples/python/basics/f_strings.py
/tmp/gpy examples/python/basics/annotations.py
/tmp/gpy examples/python/basics/match_stmt.py
```

### Modules and packages — `examples/python/basics/modules_and_packages/`

| Example | What it shows |
| --- | --- |
| `main.py` | Importing a sibling module, a package, and a sub-package. |
| `helpers.py` | A plain module used by `main.py`; also runs standalone. |
| `mypkg/__init__.py` | A package initialiser with a re-export. |
| `mypkg/core.py` | A module inside the package. |
| `mypkg/sub/__init__.py` | A nested sub-package initialiser. |
| `mypkg/sub/deeper.py` | A module two levels down. |

```bash
/tmp/gpy examples/python/basics/modules_and_packages/main.py
```

`main.py` is the entry point and exercises all of them. The four files inside
the packages are **not standalone scripts** and exit 1 when run directly, as
they would under CPython — they are imported, not executed:

| File | Run directly | Why |
| --- | --- | --- |
| `mypkg/__init__.py` | exit 0, but trivial | just a docstring and a re-export |
| `mypkg/core.py` | exit 0, but trivial | defines functions only |
| `mypkg/sub/__init__.py` | **exit 1** | `ModuleNotFoundError: 'No module named "mypkg"'` |
| `mypkg/sub/deeper.py` | **exit 1** | `ImportError: 'attempted relative import beyond top-level package'` |

The two failures are expected: both use package-relative imports that only
resolve when the package root is on `sys.path`, which running the file by path
does not arrange.

---

## Standard library — `examples/stdlib/`

One file per natively-implemented module. Each is self-documenting: a docstring,
then labelled output. Where this interpreter differs from CPython the demo says
so at the point of difference, and shows what actually happens rather than
pretending.

| Example | What it shows |
| --- | --- |
| `abc_demo.py` | Abstract base classes: `ABCMeta`, `abstractmethod`, `register`, `isinstance`. |
| `array_demo.py` | `array.array`: compact homogeneous numeric buffers, and its missing methods. |
| `atexit_demo.py` | `atexit.register`/`unregister` and shutdown ordering. |
| `base64_demo.py` | Base64, URL-safe, base32, base16, base85 and ascii85 round trips. |
| `binascii_demo.py` | Hex, base64 and quoted-printable conversion, plus `crc32`. |
| `bisect_demo.py` | Binary search: `bisect_left`/`bisect_right` and `insort`. |
| `builtin_demo.py` | The `builtins` module — every always-available name, reached as objects. |
| `codecs_demo.py` | `encode`/`decode`, and the `(value, length)` tuple they return here. |
| `collections_demo.py` | `namedtuple`, `deque`, `Counter`, `OrderedDict`, `defaultdict`. |
| `contextlib_demo.py` | `@contextmanager`, `suppress`, `closing`, `nullcontext`, `ExitStack`. |
| `csv_demo.py` | Reading and writing CSV, quoting modes, dialects, a hand-rolled DictReader. |
| `dataclasses_demo.py` | `@dataclass`, `field`, `asdict`, `replace`, `make_dataclass`. |
| `datetime_demo.py` | `date`, `datetime`, `time`, `timedelta`, `strftime`/`strptime`. |
| `enum_demo.py` | `Enum`, `IntEnum`, `Flag`, `auto`, aliases and lookup. |
| `errno_demo.py` | System error numbers and `strerror`. |
| `functools_demo.py` | `reduce`, `partial`, `wraps`, `lru_cache`, `singledispatch`, `cmp_to_key`. |
| `gettext_demo.py` | Translation catalogues and the identity fallback. |
| `glob_demo.py` | Wildcard matching with `*`, `?` and `[]`, over a self-created tree. |
| `hashlib_demo.py` | md5, sha1, sha2, sha3 and blake2 digests; streaming with `update`. |
| `importlib_demo.py` | `import_module`, `reload`, `invalidate_caches`. |
| `inspect_demo.py` | Type predicates, `getdoc`, `cleandoc`, `currentframe`. |
| `io_demo.py` | `StringIO`, `BytesIO`, file objects, reading and writing. |
| `itertools_demo.py` | The full lazy-iterator toolkit, plus a sliding-window helper. |
| `json_demo.py` | Encoding, decoding, formatting and indentation. |
| `logging_demo.py` | Levels, named loggers, handlers and formatters. |
| `marshal_demo.py` | Binary round trips of scalars and containers. |
| `math_demo.py` | The libm-shaped maths surface, plus a statistics helper. |
| `os_demo.py` | Environment, working directory, path manipulation, a recursive walk. |
| `pprint_demo.py` | `pformat`, `pprint`, width/indent/depth, and reproducible dict rendering. |
| `random_demo.py` | Seeding, distributions, sampling, shuffling, reproducible simulations. |
| `re_demo.py` | `match`/`search`/`findall`/`sub`/`split`, groups, flags, a log parser. |
| `stat_demo.py` | Mode bits, the `S_IS*` predicates, and `filemode`. |
| `string_demo.py` | Text constants, `capwords`, and validation helpers. |
| `subprocess_demo.py` | `run`, `check_output`, `call`, `Popen` and shell-style helpers. |
| `sys_demo.py` | Version info, `argv`, `path`, the standard streams, `exc_info`. |
| `tempfile_demo.py` | `mkstemp`, `mkdtemp`, and hand-written self-cleaning context managers. |
| `textwrap_demo.py` | `wrap`, `fill`, `dedent`, `indent`, `shorten`, `TextWrapper`. |
| `threading_demo.py` | `Lock` and `RLock`, and which primitives are really aliases. |
| `time_demo.py` | The wall clock, `sleep`, and a stopwatch; and what raises. |
| `types_demo.py` | The `*Type` names, `SimpleNamespace`, `new_class`. |
| `typing_demo.py` | Type hints, `TypeVar`, `NamedTuple`, `TypedDict`, `Protocol`. |
| `uuid_demo.py` | Versions 1/3/4/5, namespaces, and stable record ids. |
| `warnings_demo.py` | Categories, filters, `catch_warnings`, and a hand-rolled collector. |
| `weakref_demo.py` | `ref` and `proxy`, and exactly which weak containers work. |
| `yaml_demo.py` | `safe_load`, `safe_dump`, multi-document streams, config merging. |

```bash
/tmp/gpy examples/stdlib/abc_demo.py
/tmp/gpy examples/stdlib/array_demo.py
/tmp/gpy examples/stdlib/atexit_demo.py
/tmp/gpy examples/stdlib/base64_demo.py
/tmp/gpy examples/stdlib/binascii_demo.py
/tmp/gpy examples/stdlib/bisect_demo.py
/tmp/gpy examples/stdlib/builtin_demo.py
/tmp/gpy examples/stdlib/codecs_demo.py
/tmp/gpy examples/stdlib/collections_demo.py
/tmp/gpy examples/stdlib/contextlib_demo.py
/tmp/gpy examples/stdlib/csv_demo.py
/tmp/gpy examples/stdlib/dataclasses_demo.py
/tmp/gpy examples/stdlib/datetime_demo.py
/tmp/gpy examples/stdlib/enum_demo.py
/tmp/gpy examples/stdlib/errno_demo.py
/tmp/gpy examples/stdlib/functools_demo.py
/tmp/gpy examples/stdlib/gettext_demo.py
/tmp/gpy examples/stdlib/glob_demo.py
/tmp/gpy examples/stdlib/hashlib_demo.py
/tmp/gpy examples/stdlib/importlib_demo.py
/tmp/gpy examples/stdlib/inspect_demo.py
/tmp/gpy examples/stdlib/io_demo.py
/tmp/gpy examples/stdlib/itertools_demo.py
/tmp/gpy examples/stdlib/json_demo.py
/tmp/gpy examples/stdlib/logging_demo.py
/tmp/gpy examples/stdlib/marshal_demo.py
/tmp/gpy examples/stdlib/math_demo.py
/tmp/gpy examples/stdlib/os_demo.py
/tmp/gpy examples/stdlib/pprint_demo.py
/tmp/gpy examples/stdlib/random_demo.py
/tmp/gpy examples/stdlib/re_demo.py
/tmp/gpy examples/stdlib/stat_demo.py
/tmp/gpy examples/stdlib/string_demo.py
/tmp/gpy examples/stdlib/subprocess_demo.py
/tmp/gpy examples/stdlib/sys_demo.py
/tmp/gpy examples/stdlib/tempfile_demo.py
/tmp/gpy examples/stdlib/textwrap_demo.py
/tmp/gpy examples/stdlib/threading_demo.py
/tmp/gpy examples/stdlib/time_demo.py
/tmp/gpy examples/stdlib/types_demo.py
/tmp/gpy examples/stdlib/typing_demo.py
/tmp/gpy examples/stdlib/uuid_demo.py
/tmp/gpy examples/stdlib/warnings_demo.py
/tmp/gpy examples/stdlib/weakref_demo.py
/tmp/gpy examples/stdlib/yaml_demo.py
```

---

## Standalone programs — `examples/`

| Example | What it shows | Runtime |
| --- | --- | --- |
| `pystone.py` | The classic Dhrystone-style integer benchmark. | ~1 s |
| `pi_chudnovsky_bs.py` | Pi to 1e7 digits via Chudnovsky and binary splitting. | ~200 s |

```bash
/tmp/gpy examples/pystone.py
/tmp/gpy examples/pi_chudnovsky_bs.py
```

`pi_chudnovsky_bs.py` is genuinely long-running — it computes ten million
digits and takes around three minutes. It is correct: the final line printed is
`Last 5 digits 55897 OK`.

### Go embedding examples

`examples/embedding/` and `examples/multi-context/` are Go programs that embed
the interpreter, not Python scripts. Build them with `go test`/`go run` from
their own directories; see `examples/embedding/README.md`. They are outside the
scope of this index.

---

## Not yet supported

These do not work in this interpreter. Each is documented with a minimal
reproduction and the exact error text in [KNOWN_ISSUES.md](KNOWN_ISSUES.md).

**Crashes, hangs and deadlocks** — no Python exception to catch:

- `hashlib.file_digest()` — SIGSEGV.
- `pprint.pformat(container, width=20)` on a self-referencing container — hangs
  forever. At the default width it prints correctly.
- A second `acquire()` on a `threading.Lock` — deadlocks the interpreter.
- `type(threading.local()).__name__` — Go panic.

**Not implemented at all:**

- `async`/`await` — no coroutines, no `asyncio`.
- PEP 695 type-parameter syntax (`class C[T]:`, `def f[T]()`).
- `bytearray` and `memoryview` — absent from the builtins.
- C extension modules — there is no C API; only the natively-implemented
  modules listed above exist.
- A pure-Python standard library — modules such as `gc`, `operator`, `socket`,
  `pickle` and `sqlite3` are not present.
- `threading.Thread.start()` — raises
  `RuntimeError: this interpreter does not create Python threads, so
  Thread.start() cannot run the target`.
- `inspect.signature()` — raises `NotImplementedError` (expected; see
  KNOWN_ISSUES §16).

**Present but subtly different** — check KNOWN_ISSUES before relying on these:

- `threading.Event`, `Semaphore` and `Barrier` are aliases of `Lock` and have
  the wrong API (`e.set()` raises `AttributeError`).
- `str.encode()`, `bytes.decode()`, `str.format()` and `str.partition()` do not
  exist; use the `bytes(text, encoding)` builtin and `codecs` instead.
- `<`, `>` and friends are undefined for tuples, lists and sets.
- `sys.modules` is always empty, so no module cache exists.
- `time.gmtime`, `strftime`, `monotonic` and friends raise
  `NotImplementedError`; use `datetime`.
- `os.stat` and `os.lstat` are absent, and failed `os` calls raise
  `SystemError` rather than `OSError`.
- `WeakSet.add()` and `WeakKeyDictionary` cannot hold plain objects, because
  instances are unhashable.
- `re.X`/`re.VERBOSE` is accepted but does not strip whitespace or comments.
- `csv.DictReader`/`DictWriter` are absent and `register_dialect` takes only a
  name.
- `dict` iteration order is not insertion order. The examples never depend on
  it; where stable output matters they sort explicitly.
