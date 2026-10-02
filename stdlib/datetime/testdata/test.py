# Behaviour of the date/datetime arithmetic and ISO parsing.
import datetime as d

def assertEqual(got, want):
    assert got == want, "got %r want %r" % (got, want)

# date +/- timedelta, and date - date.
assertEqual(d.date(2024, 1, 1) + d.timedelta(days=1), d.date(2024, 1, 2))
assertEqual(d.timedelta(days=1) + d.date(2024, 1, 1), d.date(2024, 1, 2))
assertEqual(d.date(2024, 1, 2) - d.timedelta(days=1), d.date(2024, 1, 1))
assertEqual(d.date(2024, 1, 2) - d.date(2024, 1, 1), d.timedelta(days=1))
assertEqual(d.date(2024, 1, 1) - d.date(2023, 1, 1), d.timedelta(days=365))
assertEqual(d.date(2024, 3, 1) + d.timedelta(days=1), d.date(2024, 3, 2))
assertEqual(d.date(2024, 1, 31) + d.timedelta(days=1), d.date(2024, 2, 1))
assertEqual((d.date(2024, 1, 1) + d.timedelta(days=400)), d.date(2025, 2, 4))

# datetime +/- timedelta, and datetime - datetime.
assertEqual(d.datetime(2024, 1, 1) + d.timedelta(hours=25), d.datetime(2024, 1, 2, 1, 0))
assertEqual(d.datetime(2024, 1, 1) - d.timedelta(days=1), d.datetime(2023, 12, 31))
assertEqual(d.datetime(2024, 1, 2) - d.datetime(2024, 1, 1), d.timedelta(days=1))
assertEqual(d.datetime(2024, 1, 1, 12) - d.datetime(2024, 1, 1, 10), d.timedelta(hours=2))

# An unsupported operand is a TypeError, not a silent answer.
try:
    d.date(2024, 1, 1) + 1
    raise AssertionError("expected TypeError")
except TypeError:
    pass

# fromisoformat accepts the documented forms.
assertEqual(d.date.fromisoformat("2024-01-01"), d.date(2024, 1, 1))
assertEqual(d.date.fromisoformat("20240101"), d.date(2024, 1, 1))
assertEqual(d.date.fromisoformat("2024-W01-1"), d.date(2024, 1, 1))
assertEqual(d.datetime.fromisoformat("2024-01-01T12:30:45.5"),
            d.datetime(2024, 1, 1, 12, 30, 45, 500000))
assertEqual(d.datetime.fromisoformat("2024-01-01"), d.datetime(2024, 1, 1))
assertEqual(d.datetime.fromisoformat("2024-01-01 12:30:45").second, 45)

try:
    d.date.fromisoformat("not-a-date")
    raise AssertionError("expected ValueError")
except ValueError as e:
    assertEqual(str(e), "Invalid isoformat string: 'not-a-date'")

# ordinals round-trip.
assertEqual(d.date(2024, 1, 1).toordinal(), 738886)
assertEqual(d.date(1, 1, 1).toordinal(), 1)
assertEqual(d.date.fromordinal(1), d.date(1, 1, 1))
assertEqual(d.date.fromordinal(738886), d.date(2024, 1, 1))

print("OK")
