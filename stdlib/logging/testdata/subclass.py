import logging


class Verbose(logging.Logger):
    def verbose(self, msg):
        return "VERBOSE: " + msg


logging.setLoggerClass(Verbose)
lg = logging.getLogger("demo.sub")
print("type:", type(lg).__name__)
print("isinstance:", isinstance(lg, Verbose))
print("verbose:", lg.verbose("hi"))
print("name:", lg.name)
print("level:", lg.level)
# Identity is stable across getLogger calls.
print("same object:", logging.getLogger("demo.sub") is lg)
# The class a logger reports is what a native accessor sees too.
print("module:", Verbose.__module__ and "set")

doc = "finished"
