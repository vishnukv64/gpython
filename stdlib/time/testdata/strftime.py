# strftime directives and strptime parsing, against fixed inputs.
import time

g = time.gmtime(0)
assert repr(g) == ("time.struct_time(tm_year=1970, tm_mon=1, tm_mday=1, tm_hour=0, "
                   "tm_min=0, tm_sec=0, tm_wday=3, tm_yday=1, tm_isdst=0)"), repr(g)
assert len(g) == 9
assert tuple(g) == (1970, 1, 1, 0, 0, 0, 3, 1, 0)
assert g[0] == 1970 and g.tm_year == 1970 and g.tm_wday == 3
assert g[:3] == (1970, 1, 1)

assert time.strftime("%Y", g) == "1970"
assert time.strftime("%m", g) == "01"
assert time.strftime("%d", g) == "01"
assert time.strftime("%H:%M:%S", g) == "00:00:00"
assert time.strftime("%j", g) == "001"
assert time.strftime("%A", g) == "Thursday"
assert time.strftime("%a", g) == "Thu"
assert time.strftime("%B", g) == "January"
assert time.strftime("%b", g) == "Jan"
assert time.strftime("%p", g) == "AM"
assert time.strftime("%I", g) == "12"
assert time.strftime("%y", g) == "70"
assert time.strftime("%Z", g) == "UTC"
assert time.strftime("%w", g) == "4"
assert time.strftime("%U", g) == "00"
assert time.strftime("%W", g) == "00"
assert time.strftime("%%", g) == "%"

# strptime returns a full struct_time.
t = time.strptime("2024-01-02", "%Y-%m-%d")
assert (t.tm_year, t.tm_mon, t.tm_mday) == (2024, 1, 2), repr(t)
assert t.tm_wday == 1, repr(t)  # 2024-01-02 is a Tuesday
assert t.tm_yday == 2, repr(t)

t = time.strptime("13:30", "%H:%M")
assert (t.tm_hour, t.tm_min) == (13, 30), repr(t)

t = time.strptime("11:30 PM", "%I:%M %p")
assert t.tm_hour == 23, repr(t)

t = time.strptime("2024 100", "%Y %j")
assert (t.tm_mon, t.tm_mday) == (4, 9), repr(t)  # day 100 of 2024

t = time.strptime("75", "%y")
assert t.tm_year == 1975, repr(t)

assert time.asctime(g) == "Thu Jan  1 00:00:00 1970", time.asctime(g)

# A format that does not match raises ValueError.
try:
    time.strptime("nonsense", "%Y-%m-%d")
    raise AssertionError("expected ValueError")
except ValueError:
    pass

print("OK")
