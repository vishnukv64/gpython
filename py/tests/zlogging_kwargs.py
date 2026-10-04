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

# A base Handler writes nowhere, so its stream is None rather than a crash.
print("base stream:", logging.Handler().stream)

doc = "finished"
