import code
import cmd
import getopt
import graphlib
import io
import sched
import sys

# getopt
print(getopt.getopt(["-a", "-b", "val", "--long=1", "--flag", "rest", "more"],
                    "ab:", ["long=", "flag"]))
print(getopt.gnu_getopt(["x", "-a", "y"], "a", []))
try:
    getopt.getopt(["-z"], "a")
except getopt.GetoptError as e:
    print("GetoptError", e.msg, e.opt)

# graphlib
ts = graphlib.TopologicalSorter({"d": {"b", "c"}, "c": {"a"}, "b": {"a"}})
ts.prepare()
order = []
while ts.is_active():
    ready = sorted(ts.get_ready())
    order.append(ready)
    ts.done(*ready)
print(order)
print(list(graphlib.TopologicalSorter({1: {2}, 2: {3}}).static_order()))
try:
    list(graphlib.TopologicalSorter({1: {2}, 2: {1}}).static_order())
except graphlib.CycleError as e:
    print("CycleError", sorted(e.args[1]))


# cmd
class Shell(cmd.Cmd):
    prompt = "> "

    def do_greet(self, arg):
        """greet NAME"""
        print("hello", arg)

    def do_quit(self, arg):
        return True

    def default(self, line):
        print("unknown:", line)


out = io.StringIO()
sh = Shell(stdout=out)
print(sh.onecmd("greet world"))
print(sh.onecmd("nope 1 2"))
print(sh.onecmd("quit"))
sh.onecmd("help greet")
sh.onecmd("help")
print(out.getvalue(), end="")
print(sh.parseline("greet  a b"))
print(sh.completenames("gr"))

# code
ii = code.InteractiveInterpreter({})
print(ii.runsource("x = 40 + 2"))
print(ii.runsource("print(x)"))
_err = sys.stderr
sys.stderr = io.StringIO()
try:
    print(ii.runsource("def f(:"))
finally:
    sys.stderr = _err
print(ii.runsource("for i in range(2):"))
print(ii.locals["x"])
print(code.compile_command("if x:") is None, code.compile_command("1 + 1") is not None)

# sched, driven by a fake clock
now = [0.0]
slept = []


def fake_sleep(d):
    slept.append(d)
    now[0] += d


s = sched.scheduler(lambda: now[0], fake_sleep)
log = []
s.enterabs(5, 1, log.append, ("five",))
s.enter(2, 1, log.append, ("two",))
s.enter(2, 0, log.append, ("two-high",))
ev = s.enter(3, 1, log.append, ("cancelled",))
s.cancel(ev)
print(len(s.queue))
s.run()
print(log, slept, now[0], s.empty())
