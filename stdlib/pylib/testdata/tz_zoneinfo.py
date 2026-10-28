# gpython also ships a tzdata package; use it (not the system files) when it is
# importable, so the embedded data is what gets checked.  CPython has none here
# and uses the system zoneinfo, which must give the same answers.
import zoneinfo
try:
    import tzdata
    zoneinfo.reset_tzpath(to=[])
except ImportError:
    pass
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError, available_timezones
from datetime import datetime, timedelta, timezone, date
import pickle

ny = ZoneInfo('America/New_York')
print(ny, ny.key, str(ny), repr(ny).endswith("(key='America/New_York')"))
for d in [datetime(2024, 3, 10, 1, 59), datetime(2024, 3, 10, 2, 30), datetime(2024, 3, 10, 3, 0),
          datetime(2024, 11, 3, 0, 59), datetime(2024, 11, 3, 1, 30), datetime(2024, 11, 3, 1, 30, fold=1),
          datetime(2024, 11, 3, 2, 0), datetime(2024, 7, 1), datetime(2024, 1, 1), datetime(1883, 11, 18, 12),
          datetime(2037, 7, 1), datetime(2060, 7, 1), datetime(2060, 1, 1), datetime(1900, 1, 1)]:
    d = d.replace(tzinfo=ny)
    print(d, d.utcoffset(), d.tzname(), d.dst(), d.timestamp())
utc = datetime(2024, 3, 10, 6, 59, tzinfo=timezone.utc)
for t in (0, 1, 60, 3600):
    x = (utc + timedelta(seconds=t)).astimezone(ny)
    print(x, x.fold)
for k in ['Europe/London', 'Asia/Kolkata', 'Asia/Tokyo', 'Australia/Sydney', 'UTC', 'America/Sao_Paulo', 'Pacific/Apia', 'Asia/Kathmandu']:
    z = ZoneInfo(k)
    for m in (1, 4, 7, 10):
        d = datetime(2023, m, 15, 12, tzinfo=z)
        print(k, m, d.utcoffset(), d.dst(), d.tzname())
print(datetime(2024, 7, 1, 12, tzinfo=timezone.utc).astimezone(ZoneInfo('Asia/Kolkata')))
print(ZoneInfo('America/New_York') is ZoneInfo('America/New_York'), ZoneInfo.no_cache('UTC') is ZoneInfo.no_cache('UTC'))
print(pickle.loads(pickle.dumps(ny)) is ny)
for key in ('Nope/Zone', '../etc/passwd', '', 'America', '/abs'):
    try: ZoneInfo(key)
    except (ZoneInfoNotFoundError, ValueError, OSError) as e: print(type(e).__name__)
tz = available_timezones()
print(len(tz) > 400, 'America/New_York' in tz, 'UTC' in tz)
ZoneInfo.clear_cache()
print(ZoneInfo('America/New_York') is ny)
print(datetime(2024, 6, 1, tzinfo=ny) - datetime(2024, 1, 1, tzinfo=ny))
print(datetime(2024, 3, 9, 12, tzinfo=ny) + timedelta(days=1))
print(ny.utcoffset(None), ny.tzname(None), ny.dst(None))
print(datetime.now(ny).tzinfo is ny, date(2024, 1, 1).isoformat())
