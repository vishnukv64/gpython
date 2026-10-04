# gpython

gpython is a part re-implementation, part port of the Python 3
interpreter in Go.  Although there are many areas of improvement,
it stands as an noteworthy achievement in capability and potential.

This repository is a **fork** of [go-python/gpython](https://github.com/go-python/gpython)
maintained under [vishnukv64](https://github.com/vishnukv64).  The upstream
project is kept as the `upstream` git remote, which is read-only here: work
lands on this fork only.  All original copyright notices and the upstream
lineage are preserved; see [License](#license).

gpython includes:

  * lexer, parser, and compiler
  * runtime and high-level convenience functions
  * a file-system import system: packages, submodules, relative imports
  * multi-context interpreter instancing
  * easy embedding into your Go application
  * interactive mode (REPL)

## What this fork adds

The import system was the main obstacle to running real Python code, and it
has been rebuilt along CPython's lines:

  * **Packages work.** `import pkg`, `import pkg.sub`, `from pkg import sub`,
    `from pkg.sub import name`, `from . import sibling` and `from .sub import name`
    all resolve, both from `sys.path` and from an installed package tree.
    Every imported module gets a `__path__` (packages) and `__package__`,
    and submodules are bound onto their parent.
  * **`PYTHONPATH`** is honoured, and a script is run with its own directory
    as `sys.path[0]` — the rule that lets it import its neighbours.
  * **`-c`, `-m <module>`, `-m <package>`** (runs `__main__`) and `--version`.

Several long-standing bugs that broke ordinary Python were fixed along the way:

  * Decorated functions compiled incorrectly: the decorators were emitted
    after the function's defaults, so `MAKE_FUNCTION` consumed them.  This
    was the cause of a hard panic on class bodies using `@property`.
  * `property` was missing from `builtins` and could not be constructed; it
    is now implemented and behaves as a descriptor, as do `staticmethod` and
    `classmethod` when read from a class.
  * `_make_function` asserted argument types unguarded and panicked instead
    of raising.
  * Absolute script paths never resolved (`path.Join(".", "/x")` strips the
    leading separator).
  * Class objects now expose `__name__`, `__qualname__`, `__doc__`,
    `__bases__` and `__dict__`.

Numeric literals accept underscores (`1_000`, `0x_FF`, `1_0.5`), and the
`__future__` module exists so `from __future__ import ...` stops failing.
Class objects also expose `__mro__`.

Beyond the import system, the interpreter now runs **pip**: `pip --version`,
`pip --help` and `pip list` all work.  Reaching that required a long list of
language and library gaps, each found by running real code rather than by
reading it — among them regular-expression lookbehind, `str.translate` and the
rest of the missing `str` methods, case mappings that expand (`"ß".upper()` is
`"SS"`), `sys.exc_info` for exceptions raised by the interpreter itself,
descriptors and `__set_name__`, `__init_subclass__` with the class keywords,
the reflected and Python-defined operators, `dict.keys()` as a live set-like
view, `sys.meta_path`, and `os.path.join` following POSIX's rule for an
absolute component.

## Status and limitations

This is honest about where the interpreter stands:

  * The grammar accepts the features of **Python 3.10**: f-strings, numeric
    underscores, the walrus operator, positional-only `/` parameters and
    `match`/`case` all parse and run.  `except*` (3.11) and PEP 695 type
    parameters (3.12) are SyntaxErrors, which is what `sys.version_info`
    reports.  **`async def` and `await` are the one large language feature not
    implemented.**
  * The standard library is a mix of Go ports of the C modules and Go
    implementations of the pure-Python ones.  `itertools`, `functools`,
    `collections`, `json`, `datetime`, `typing`, `re`, `logging`, `plistlib`,
    `zipimport` and many more import and work.
  * **C extension modules (`.so`/`.pyd`) cannot be loaded at all**, and there
    is no plan to change that.  Packages with compiled dependencies are
    therefore out of reach, which is the barrier the upstream README
    describes.
  * Namespace packages (PEP 420: directories without `__init__.py`) work, and
    `__path__` collects every matching `sys.path` entry, which is what lets two
    distributions contribute to one namespace.
  * `sys.meta_path` exists and a program may install its own finder, but this
    interpreter's own file finder is built in rather than expressed as an
    entry on it.

`pip` runs: `pip --version`, `pip --help` and `pip list` all work, with help and
list matching CPython's own output.  Installing packages does not, because that
needs the C extension barrier above to fall.

## Install

With Go installed:

    go install github.com/vishnukv64/gpython@latest

Or build from a checkout:

    go build -o gpython .
    ./gpython -c 'print("hello")'

## Objectives

gpython started as an experiment to investigate how hard porting Python to
Go might be.  It turns out that all those C modules are a significant barrier
to making gpython a complete replacement to CPython.

However, to those who want to embed a highly popular and known language
into their Go application, gpython could be a great choice over less
capable (or lesser known) alternatives.

## Getting Started

The [embedding example](examples/embedding) demonstrates how to easily embed
and invoke gpython from any Go application.

gpython is able to run multiple interpreter instances simultaneously,
allowing you to embed gpython naturally into your Go application.  This makes
it possible to use gpython in a server situation where complete interpreter
independence is paramount.  See this in action in the
[multi-context example](examples/multi-context).

## Other Projects of Interest

  * [grumpy](https://github.com/grumpyhome/grumpy) - a python to go transpiler

## Community

Upstream: [go-python@googlegroups.com](https://groups.google.com/forum/#!forum/go-python),
or the Gophers Slack in the `#go-python` channel.

## License

This is licensed under the MIT licence, however it contains code which
was ported fairly directly directly from the CPython source code under
the [PSF LICENSE](https://github.com/python/cpython/blob/main/LICENSE).
