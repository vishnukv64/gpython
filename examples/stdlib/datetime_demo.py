"""datetime: dates, times, durations.

Run with:  /tmp/gpy examples/stdlib/datetime_demo.py

Covers date, datetime, time, timedelta and timezone, plus formatting and
parsing with strftime/strptime.

Interpreter notes (all verified by running this file):
  * Arithmetic between two datetimes, or between a date and a timedelta, is not
    implemented -- `date(...) + timedelta(days=1)` raises TypeError. timedelta
    objects can be built and inspected, but not compared or combined.
  * date.replace() and datetime.timetuple() are absent.
  * fromisoformat() is absent on both date and datetime.
"""

from datetime import date, datetime, time, timedelta, timezone

print("--- constructing values ---")
d = date(2023, 5, 17)
t = time(14, 30, 15)
dt = datetime(2023, 5, 17, 14, 30, 15)
print("date:     ", d)
print("time:     ", t)
print("datetime: ", dt)
print("repr:     ", repr(dt))

print()
print("--- reading the components ---")
print("dt.year/month/day:  ", dt.year, dt.month, dt.day)
print("dt.hour/minute/sec: ", dt.hour, dt.minute, dt.second)
print("d.weekday():        ", d.weekday(), "(Monday is 0)")
print("d.isoformat():      ", d.isoformat())
print("dt.isoformat():     ", dt.isoformat())

print()
print("--- formatting ---")
print("strftime %Y-%m-%d:      ", dt.strftime("%Y-%m-%d"))
print("strftime %H:%M:%S:      ", dt.strftime("%H:%M:%S"))
print("strftime %A %B:         ", dt.strftime("%A %B"))
print("strftime %%Y literal:   ", dt.strftime("100%% of %Y"))

print()
print("--- parsing with strptime ---")
parsed = datetime.strptime("2023-05-17 14:30:15", "%Y-%m-%d %H:%M:%S")
print("parsed:      ", parsed)
print("same as built:", parsed == dt)
print("parsed date: ", datetime.strptime("2023-05-17", "%Y-%m-%d"))

print()
print("--- timedelta describes a duration ---")
day = timedelta(days=1)
shift = timedelta(days=1, hours=2, minutes=3)
print("timedelta(days=1):                  ", repr(day))
print("timedelta(days=1, hours=2, min=3):  ", repr(shift))
print("shift.total_seconds():              ", shift.total_seconds())
print("one day in seconds:                 ", day.total_seconds())
print("negative duration:                  ", repr(timedelta(hours=-3)))
print("its total_seconds():                ", timedelta(hours=-3).total_seconds())

print()
print("--- timedeltas are inert in this interpreter ---")
try:
    result = dt + day
    print("datetime + timedelta:", result)
except TypeError as err:
    print("datetime + timedelta raises TypeError:", err)
try:
    print("timedelta + timedelta:", day + timedelta(hours=1))
except TypeError as err:
    print("timedelta + timedelta raises TypeError:", err)
try:
    print("date - date:", date(2023, 5, 20) - date(2023, 5, 17))
except TypeError as err:
    print("date - date raises TypeError:", err)
try:
    print("timedelta comparison:", day > timedelta(hours=1))
except TypeError as err:
    print("timedelta > timedelta raises TypeError:", err)

print()
print("--- 'now' is real ---")
today = date.today()
now = datetime.now()
print("date.today():   ", today)
print("datetime.now(): ", now)
print("datetime.utcnow():", datetime.utcnow())
print("datetime.today():", datetime.today())
print("now is a datetime:", isinstance(now, datetime))

print()
print("--- timezone.utc exists ---")
print("timezone.utc:", timezone.utc)
print("timezone(timedelta(hours=5)):", timezone(timedelta(hours=5)))
print("the only public attribute is:", [n for n in dir(timezone.utc) if not n.startswith("_")])
try:
    timezone.utc.utcoffset(None)
except AttributeError as err:
    print("timezone.utc.utcoffset(None) raises AttributeError:", err)

print()
print("--- a worked example: a simple event log ---")


class Event:
    def __init__(self, stamp, kind, detail):
        self.stamp = stamp
        self.kind = kind
        self.detail = detail

    def render(self):
        return "%s [%-6s] %s" % (self.stamp.strftime("%Y-%m-%d %H:%M"), self.kind, self.detail)


class EventLog:
    def __init__(self, label):
        self.label = label
        self.events = []
        self.opened = datetime.now()

    def add(self, stamp, kind, detail):
        self.events.append(Event(stamp, kind, detail))
        return self

    def duration_seconds(self):
        """Seconds since the log opened, without any datetime subtraction."""
        elapsed = datetime.now().timestamp() - self.opened.timestamp()
        return int(elapsed)

    def report(self):
        lines = ["event log: %s (opened %s)" % (self.label, self.opened.strftime("%H:%M:%S"))]
        for event in self.events:
            lines.append("  " + event.render())
        lines.append("  elapsed: %d s" % (self.duration_seconds(),))
        return lines


log = EventLog("nightly-job")
log.add(datetime(2023, 5, 17, 2, 0, 0), "start", "job began")
log.add(datetime(2023, 5, 17, 2, 0, 45), "warn", "slow query")
log.add(datetime(2023, 5, 17, 2, 3, 10), "done", "job finished")
for line in log.report():
    print(line)

print()
print("--- what is missing, checked directly ---")
print("date.replace exists:      ", hasattr(date(2023, 1, 1), "replace"))
print("datetime.replace exists:  ", hasattr(dt, "replace"))
print("datetime.timetuple exists:", hasattr(dt, "timetuple"))
print("date.fromisoformat exists:", hasattr(date, "fromisoformat"))
print("datetime.combine exists:  ", hasattr(datetime, "combine"))
print("datetime.combine works:   ", datetime.combine(date(2023, 1, 1), time(1, 2, 3)))
print("timedelta.total_seconds:  ", hasattr(timedelta(), "total_seconds"))
