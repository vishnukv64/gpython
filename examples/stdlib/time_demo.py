"""time: clocks, conversions and formatting.

Run with:  gpython examples/stdlib/time_demo.py
The output matches CPython 3.14 line for line (run with TZ=UTC for the
local-time lines to be stable).

Covers the wall clock (time, time_ns, sleep), the monotonic and CPU clocks,
struct_time conversions (gmtime, localtime, mktime), and text formatting and
parsing (ctime, asctime, strftime, strptime).
"""

import datetime
import time

print("--- the wall clock ---")
now = time.time()
print("time.time() is a float:", isinstance(now, float))
nanos = time.time_ns()
print("time.time_ns() is an int:", isinstance(nanos, int))
print("they agree to within a second:", abs(nanos / 1e9 - time.time()) < 1.0)

before = time.time()
time.sleep(0.05)
print("sleep(0.05) took at least 0.05 s:", time.time() - before >= 0.05)
print("time.sleep returns None:", time.sleep(0) is None)

print()
print("--- clocks for measuring ---")
for name in ["time", "monotonic", "perf_counter", "process_time"]:
    info = time.get_clock_info(name)
    print("  %-13s monotonic=%-5s adjustable=%s" % (name, info.monotonic, info.adjustable))
start = time.perf_counter()
sum(range(100000))
print("perf_counter measures elapsed time:", time.perf_counter() - start >= 0)
print("time.clock was removed in Python 3.8:", not hasattr(time, "clock"))

print()
print("--- struct_time conversions ---")
epoch = time.gmtime(0)
print("gmtime(0):", epoch)
print("fields by name:", epoch.tm_year, epoch.tm_mon, epoch.tm_mday, epoch.tm_wday)
t = time.gmtime(1_700_000_000)
print("gmtime(1_700_000_000)[:6]:", t[:6])
print("strftime:", time.strftime("%Y-%m-%d %H:%M:%S %a %j", t))
print("asctime:", time.asctime(t))
print("ctime(0) is asctime(localtime(0)):", time.ctime(0) == time.asctime(time.localtime(0)))
print("mktime(localtime(x)) round-trips:", time.mktime(time.localtime(1_700_000_000)) == 1_700_000_000.0)

print()
print("--- parsing text ---")
parsed = time.strptime("2024-02-29 13:45", "%Y-%m-%d %H:%M")
print("strptime:", parsed[:6], "day of year", parsed.tm_yday)

print()
print("--- what the arguments must be ---")
for call, args in [(time.ctime, ("not a number",)),
                   (time.ctime, ((2020, 1, 1, 0, 0, 0, 0, 0, 0),)),
                   (time.gmtime, (float("nan"),)),
                   (time.asctime, ((2020, 1),)),
                   (time.sleep, (-1,))]:
    try:
        call(*args)
    except (TypeError, ValueError, OverflowError) as e:
        print("  %s%r -> %s: %s" % (call.__name__, args, type(e).__name__, e))

print()
print("--- datetime does calendar arithmetic ---")
moment = datetime.datetime.fromtimestamp(1_700_000_000, datetime.timezone.utc)
print("fromtimestamp:", moment)
print("plus a week:", moment + datetime.timedelta(days=7))
print("difference:", datetime.datetime(2024, 3, 1) - datetime.datetime(2024, 2, 1))


class Stopwatch:
    """Measures elapsed time with the monotonic perf_counter clock."""

    def __enter__(self):
        self.started = time.perf_counter()
        return self

    def __exit__(self, *details):
        self.elapsed = time.perf_counter() - self.started
        return False


print()
print("--- a worked example: timing a block ---")
with Stopwatch() as watch:
    time.sleep(0.02)
print("slept 20 ms, measured at least that:", watch.elapsed >= 0.02)
