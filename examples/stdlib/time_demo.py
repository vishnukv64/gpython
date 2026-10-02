"""time: clocks and sleeping.

Run with:  /tmp/gpy examples/stdlib/time_demo.py

Covers time.time(), time_ns(), sleep() and the module's clock list.

Interpreter notes (all verified below):
  * gmtime(), localtime(), mktime(), ctime(), asctime(), strftime(),
    strptime(), monotonic(), perf_counter(), process_time() and
    get_clock_info() all raise NotImplementedError -- only the wall-clock
    seconds and sleep() are implemented.
  * Use the datetime module for calendar arithmetic and formatting.
"""

import datetime
import time

print("--- the wall clock ---")
now = time.time()
print("time.time():     ", now)
print("it is a float:   ", isinstance(now, float))
print("seconds since the epoch:", int(now))
print("roughly", round(now / 86400.0 / 365.25, 1), "years since 1970")

print()
print("--- time_ns() is the same instant in nanoseconds ---")
nanos = time.time_ns()
print("time.time_ns():  ", nanos)
print("it is an int:    ", isinstance(nanos, int))
print("nanos / 1e9 agrees with time.time() to within a second:",
      abs(nanos / 1e9 - time.time()) < 1.0)

print()
print("--- time advances ---")
before = time.time()
time.sleep(0.05)
after = time.time()
elapsed = after - before
print("slept for 0.05 s; measured:", round(elapsed, 4), "s")
print("at least the requested duration:", elapsed >= 0.05)
print("time.sleep returns None:", time.sleep(0) is None)

print()
print("--- what this module actually provides ---")
names = ["time", "time_ns", "sleep", "clock", "clock_gettime", "clock_settime",
         "clock_getres", "gmtime", "localtime", "mktime", "ctime", "asctime",
         "strftime", "strptime", "monotonic", "perf_counter", "process_time",
         "get_clock_info", "tzset"]
for name in names:
    print("  time.%-16s %s" % (name, hasattr(time, name)))

print()
print("--- the ones that raise NotImplementedError ---")
# mktime/ctime/asctime need exactly one argument, so pass a 9-tuple.
EPOCH_TUPLE = (2020, 1, 1, 0, 0, 0, 0, 0, 0)
for name in ["gmtime", "localtime", "mktime", "ctime", "asctime",
             "monotonic", "perf_counter", "process_time", "tzset"]:
    function = getattr(time, name)
    try:
        if name in ["mktime", "ctime", "asctime"]:
            function(EPOCH_TUPLE)
        else:
            function()
        print("  time.%-16s returned a value" % (name,))
    except NotImplementedError:
        print("  time.%-16s raises NotImplementedError" % (name,))
try:
    time.get_clock_info("time")
except NotImplementedError:
    print("  time.%-16s raises NotImplementedError" % ("get_clock_info",))
try:
    time.strftime("%Y", EPOCH_TUPLE)
except NotImplementedError:
    print("  time.%-16s raises NotImplementedError" % ("strftime",))
try:
    time.strptime("2020", "%Y")
except NotImplementedError:
    print("  time.%-16s raises NotImplementedError" % ("strptime",))
print("time.clock() does work and returns wall-clock seconds:", time.clock() > 0)

print()
print("--- calendar work belongs to datetime ---")
stamp = time.time()
moment = datetime.datetime.fromtimestamp(stamp)
print("fromtimestamp of time.time():", moment)
print("strftime through datetime:   ", moment.strftime("%Y-%m-%d %H:%M:%S"))
print("(time.strftime is NotImplementedError; datetime.strftime works)")

print()
print("--- a worked example: timing a block of work ---")


class Stopwatch:
    """Measures elapsed wall-clock time using time.time()."""

    def __init__(self, label):
        self.label = label
        self.started = None
        self.elapsed = None

    def __enter__(self):
        self.started = time.time()
        return self

    def __exit__(self, *details):
        self.elapsed = time.time() - self.started
        return False

    def report(self):
        return "%-22s %8.4f s" % (self.label, self.elapsed)


with Stopwatch("sleep 10 ms") as watch:
    time.sleep(0.01)
print(watch.report())

with Stopwatch("sleep 30 ms") as watch:
    time.sleep(0.03)
print(watch.report())

with Stopwatch("a busy loop") as watch:
    total = 0
    for value in range(200000):
        total = total + value
print(watch.report())
print("the loop summed to", total)

print()
print("--- a crude rate measurement ---")
with Stopwatch("1000 sleeps of 0") as watch:
    for _ in range(1000):
        time.sleep(0)
print(watch.report())
print("that is roughly %.0f calls per second" % (1000 / watch.elapsed,))

print()
print("--- the same measurement with datetime instead ---")
before = datetime.datetime.now()
time.sleep(0.02)
after = datetime.datetime.now()
# datetime subtraction is not implemented, so compare timestamps.
delta = after.timestamp() - before.timestamp()
print("elapsed via timestamps:", round(delta, 4), "s")
print("(datetime - datetime raises TypeError here; timestamps work)")
