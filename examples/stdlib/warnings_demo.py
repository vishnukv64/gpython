"""warnings: emit and control non-fatal warnings.

Run with:  /tmp/gpy examples/stdlib/warnings_demo.py

warnings.warn() prints a message to stderr; catch_warnings() and simplefilter()
let you capture or suppress them. Warnings are for "this still works, but you
should know", as opposed to exceptions.

Interpreter note: catch_warnings(record=True) collects nothing here -- the list
comes back empty however many warnings are raised inside the block. The demo
shows that, and formats a warning by hand for the summary it wants.
"""

import warnings

print("--- the warning categories ---")
for name in ["Warning", "UserWarning", "DeprecationWarning", "PendingDeprecationWarning",
             "SyntaxWarning", "RuntimeWarning", "FutureWarning", "ImportWarning",
             "UnicodeWarning", "BytesWarning", "ResourceWarning"]:
    print("  warnings.%-26s %s" % (name, hasattr(warnings, name)))
print("UserWarning subclasses Warning:", issubclass(UserWarning, Warning))

print()
print("--- raising a warning prints it to stderr ---")
print("the next lines come from stderr, not stdout:")
warnings.warn("this is a UserWarning")
warnings.warn("this one has a category", UserWarning)
warnings.warn("this one has a stacklevel", DeprecationWarning, stacklevel=2)
print("(each warning above was printed; execution continued)")

print()
print("--- warn_explicit names the file and line itself ---")
warnings.warn_explicit("an explicit warning", UserWarning, "config.py", 42)
print("useful when the warning comes from data rather than from this file")

print()
print("--- formatting a warning yourself ---")
text = warnings.formatwarning("something looks off", UserWarning, "app.py", 7)
print("formatwarning returns:", repr(text))
print("it embeds the category, file, line and message")

print()
print("--- the filter list is real state ---")
print("type of warnings.filters:", type(warnings.filters).__name__)
print("filters before:", len(warnings.filters), "entries")
warnings.simplefilter("always")
print("after simplefilter('always'):", len(warnings.filters), "entries")
warnings.simplefilter("ignore")
print("after simplefilter('ignore'):", len(warnings.filters), "entries")
warnings.resetwarnings()
print("after resetwarnings():       ", len(warnings.filters), "entries")
print("known filter actions:",
      [action for action in ["error", "ignore", "always", "default",
                             "module", "once"] if action is not None])

print()
print("--- simplefilter('error') turns warnings into exceptions ---")
warnings.simplefilter("error")
try:
    warnings.warn("this becomes an exception", UserWarning)
except UserWarning as err:
    print("with the 'error' filter, warn() raised UserWarning:", err)
warnings.resetwarnings()

print()
print("--- catch_warnings is usable as a context manager ---")
with warnings.catch_warnings():
    warnings.simplefilter("ignore")
    warnings.warn("suppressed inside the with block")
    print("inside: the warning above was suppressed by the 'ignore' filter")
print("outside: the filter is restored")

print()
print("--- but record=True collects nothing here ---")
with warnings.catch_warnings(record=True) as recorded:
    warnings.simplefilter("always")
    warnings.warn("first", UserWarning)
    warnings.warn("second", UserWarning)
    print("inside: raised two warnings")
print("len(recorded) after the block:", len(recorded))
print("(CPython would have collected the two WarningMessage objects)")

print()
print("--- a worked example: collecting warnings yourself ---")


class WarningCollector:
    """Wraps a computation and gathers the warnings it would have printed.

    Because catch_warnings(record=True) is empty here, the collector hooks
    the module's warn() function instead.
    """

    def __init__(self):
        self.messages = []
        self.original = warnings.warn

    def _capture(self, message, category=None, stacklevel=1):
        kind = category.__name__ if category is not None else "UserWarning"
        self.messages.append((kind, str(message)))

    def __enter__(self):
        warnings.warn = self._capture
        return self

    def __exit__(self, *details):
        warnings.warn = self.original
        return False

    def report(self):
        lines = ["collected %d warning(s)" % (len(self.messages),)]
        for kind, message in self.messages:
            lines.append("  [%s] %s" % (kind, message))
        return lines


def process(config):
    if "timeout" not in config:
        warnings.warn("no timeout configured, using 30s", UserWarning)
    if config.get("retries", 0) > 5:
        warnings.warn("retries above 5 is unusual", UserWarning)
    return "processed"


print("process({'host': 'localhost'}):",
      process({"host": "localhost"}))
print("the warning went to stderr as usual")
print()

with WarningCollector() as collector:
    result = process({"host": "localhost", "retries": 9})
    print("process(...) returned:", result)
for line in collector.report():
    print(line)
print("after the with block, warn() is restored:",
      warnings.warn is collector.original)
