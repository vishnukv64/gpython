"""logging: a levelled, hierarchical message log.

Run with:  /tmp/gpy examples/stdlib/logging_demo.py

basicConfig() sets up the root logger; getLogger() returns named loggers that
inherit its configuration. Messages below a logger's level are dropped.

Interpreter notes:
  * Formatter exists, but a format string using %(name)s-style fields is NOT
    usable in the format= argument -- the format substitution rejects the "("
    and raises `ValueError: "unsupported format character '('"`. The default
    "%(levelname)s:%(name)s:%(message)s" layout is what you get.
  * LogRecord cannot be constructed directly: `TypeError: cannot create
    'logging.LogRecord' instances`.
  * StreamHandler is accepted, but its output still reaches the real stdout,
    so per-stream capture is not demonstrated.
"""

import logging

print("--- levels, from least to most severe ---")
for name in ["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"]:
    print("  %-9s = %d" % (name, getattr(logging, name)))
print("getLevelName(20):", logging.getLevelName(20))
print("getLevelName(40):", logging.getLevelName(40))

print()
print("--- basicConfig(level=...) sets the threshold ---")
logging.basicConfig(level=logging.DEBUG)
root = logging.getLogger()
root.setLevel(logging.DEBUG)
print("root logger name:", logging.root.name)
print("effective level: ", logging.getLevelName(root.level))

print()
print("--- the module-level functions log to the root logger ---")
logging.debug("a debug message")
logging.info("an info message")
logging.warning("a warning")
logging.error("an error")
logging.critical("a critical error")
print("(each line above carries its level and logger name)")

print()
print("--- %s substitution in the message works ---")
logging.info("processed %d records in %.2f s", 42, 1.5)
logging.warning("unknown key %r, using default %s", "colour", "blue")

print()
print("--- named loggers are separate objects and are cached ---")
first = logging.getLogger("app.worker")
second = logging.getLogger("app.worker")
print("getLogger is stable:", first is second)
print("name:               ", first.name)
print("isEnabledFor(ERROR):", first.isEnabledFor(logging.ERROR))

print()
print("--- raising a logger's level suppresses lower messages ---")
noisy = logging.getLogger("app.quiet")
noisy.setLevel(logging.ERROR)
print("level set to ERROR")
noisy.info("this info line is suppressed")
noisy.warning("this warning is suppressed too")
noisy.error("only this error is shown")
print("isEnabledFor(INFO):", noisy.isEnabledFor(logging.INFO))
print("isEnabledFor(ERROR):", noisy.isEnabledFor(logging.ERROR))

print()
print("--- handlers and formatters are present ---")
print("Handler class:     ", logging.Handler is not None)
print("StreamHandler:     ", logging.StreamHandler is not None)
print("FileHandler:       ", logging.FileHandler is not None)
print("NullHandler:       ", logging.NullHandler is not None)
print("Formatter:         ", logging.Formatter is not None)
formatter = logging.Formatter()
print("a default Formatter:", type(formatter).__name__)
print("handler count on the root logger:", len(logging.root.handlers))

print()
print("--- a worked example: per-component loggers ---")


class Pipeline:
    """Runs named stages, each logging under its own child logger."""

    def __init__(self, name):
        self.name = name
        self.log = logging.getLogger("pipeline." + name)
        self.log.setLevel(logging.DEBUG)
        self.stages = []

    def stage(self, label, level):
        self.stages.append(label)
        self.log.log(level, "stage %r starting", label)
        self.log.log(level, "stage %r done", label)
        return self


pipeline = Pipeline("etl")
pipeline.stage("extract", logging.INFO)
pipeline.stage("transform", logging.WARNING)
pipeline.stage("load", logging.ERROR)
print("stages run:", pipeline.stages)

print()
print("--- the format-string limitation, shown directly ---")
try:
    logging.basicConfig(format="%(levelname)s|%(message)s")
    logging.getLogger("fmt").warning("this never gets the custom layout")
    print("(a %(field)s format string did not take effect)")
except ValueError as err:
    print("a %(field)s format raises ValueError:", err)

print()
print("--- a LogRecord cannot be built by hand ---")
try:
    logging.LogRecord("n", logging.INFO, "path", 1, "msg", (), None)
except TypeError as err:
    print("logging.LogRecord(...) raises TypeError:", err)
