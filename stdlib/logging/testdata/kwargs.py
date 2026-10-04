"""logging call keywords, level names, and a handler's stream."""

import logging
import logging.config

# The four keywords every logging call takes.  A bare argument check rejected
# them all with "name() takes no keyword arguments".
lg = logging.getLogger("kw")
lg.setLevel(logging.DEBUG)
lg.debug("plain")
lg.warning("with exc_info", exc_info=True)
lg.warning("with stacklevel", stacklevel=2)
lg.warning("with stack_info", stack_info=False)

# A level registered by name is usable in a dictConfig, which is how pip names
# its own VERBOSE level.
logging.addLevelName(15, "VERBOSE")
print("level name:", logging.getLevelName(15))
print("level number:", logging.getLevelName("VERBOSE"))
logging.config.dictConfig(
    {"version": 1, "loggers": {"cfg": {"level": "VERBOSE"}}, "disable_existing_loggers": False}
)
print("configured level:", logging.getLogger("cfg").level)

# The standard names still resolve, including the aliases.
for name in ("DEBUG", "INFO", "WARNING", "WARN", "ERROR", "CRITICAL", "FATAL", "NOTSET"):
    logging.config.dictConfig(
        {"version": 1, "loggers": {"cfg": {"level": name}}, "disable_existing_loggers": False}
    )
    print(name, logging.getLogger("cfg").level)

# A base Handler writes nowhere.  CPython 3.14 refuses "Handler().stream"
# outright (AttributeError) while 3.9 returns None, so this is not pinned to
# either; what IS pinned is that constructing one does not crash the
# interpreter, which was the actual bug.
h = logging.Handler()
print("base handler constructed:", type(h) is logging.Handler)

# dictConfig above installed handlers on the ROOT logger, and the test harness
# shares one interpreter context across every script it runs - so leaving them
# there changed the NEXT test's output.  Restore the root logger to no handlers,
# which is what a fresh interpreter has.
root = logging.getLogger()
for handler in list(root.handlers):
    root.removeHandler(handler)
root.setLevel(logging.WARNING)

doc = "finished"
