"""re: regular expressions.

Run with:  /tmp/gpy examples/stdlib/re_demo.py

Covers match/search/findall/finditer, groups, sub/subn, split, compile with
flags, fullmatch and escape.

Interpreter notes:
  * The verbose flag re.X / re.VERBOSE is accepted but does not take effect --
    whitespace and comments in the pattern are not stripped, so a pattern
    relying on them fails to match.
  * Named group syntax (?P<name>...) works and groupdict() is populated.
"""

import re

print("--- the four core searches ---")
print("match anchors at the start:")
print("  match(r'\\d+', '123abc'):  ", re.match(r"\d+", "123abc").group())
print("  match(r'\\d+', 'abc123'):  ", re.match(r"\d+", "abc123"))
print("search scans anywhere:")
print("  search(r'\\d+', 'abc123'): ", re.search(r"\d+", "abc123").group())
print("fullmatch requires the whole string:")
print("  fullmatch(r'\\d+', '123'): ", re.fullmatch(r"\d+", "123").group())
print("  fullmatch(r'\\d+', '123a'):", re.fullmatch(r"\d+", "123a"))

print("\n--- all occurrences ---")
text = "call 555-1234 or 555-9876 today"
print("text:", text)
print("findall of the phone pattern:", re.findall(r"\d{3}-\d{4}", text))
print("finditer gives match objects:")
for match in re.finditer(r"\d{3}-\d{4}", text):
    print("  %s at span %s" % (match.group(), match.span()))
print("findall with one group returns the group:")
print("  ", re.findall(r"(\d{3})-\d{4}", text))
print("findall with two groups returns tuples:")
print("  ", re.findall(r"(\d{3})-(\d{4})", text))

print()
print("--- groups ---")
match = re.match(r"(\w+)@(\w+)\.(\w+)", "ann@example.com")
print("whole match: ", match.group())
print("group(1):    ", match.group(1))
print("groups():    ", match.groups())
print("start/end/span:", match.start(), match.end(), match.span())
print("named groups:")
match = re.match(r"(?P<user>\w+)@(?P<host>\w+)", "ann@example")
print("  groupdict():", match.groupdict())
print("  group('user'):", match.group("user"))

print()
print("--- replacing ---")
text = "the rain in spain"
print("sub(r'ai', 'AI', text):     ", re.sub(r"ai", "AI", text))
print("sub with a count limit:     ", re.sub(r"ai", "AI", text, count=1))
print("sub with a function:        ", re.sub(r"\w+", lambda m: m.group().upper(), text))
print("subn returns (result, n):   ", re.subn(r"ai", "AI", text))
print("backreferences in the replacement:")
print("  re.sub(r'(\\w+)@(\\w+)', r'\\2!\\1', 'ann@example'):",
      re.sub(r"(\w+)@(\w+)", r"\2!\1", "ann@example"))

print()
print("--- splitting ---")
print("split on whitespace:      ", re.split(r"\s+", "a  b   c"))
print("split on a comma + space: ", re.split(r",\s*", "a, b,c"))
print("split with a maxsplit:    ", re.split(r",", "a,b,c,d", maxsplit=2))
print("split keeping the groups: ", re.split(r"([,;])", "a,b;c"))

print()
print("--- compiling once and reusing ---")
pattern = re.compile(r"(?P<key>\w+)=(?P<value>\w+)")
print("compiled pattern:", pattern)
for text in ["a=1", "b=2", "not a pair"]:
    found = pattern.search(text)
    print("  %-12s -> %s" % (text, found.groupdict() if found else None))

print()
print("--- flags ---")
print("IGNORECASE:", re.compile(r"abc", re.I).match("ABC").group())
print("MULTILINE: ", re.findall(r"^\w+", "one\ntwo\nthree", re.M))
print("DOTALL:    ", re.compile(r"a.b", re.S).match("a\nb").group())
print("ASCII/UNICODE flag constants exist:", re.ASCII, re.UNICODE)
print()
print("VERBOSE is accepted but does not strip whitespace/comments here:")
verbose_pattern = re.compile(r"\d+   # a number", re.X)
print("  pattern matched against '12':", verbose_pattern.match("12"))

print()
print("--- escaping makes a literal string safe as a pattern ---")
raw = "file(1).txt"
escaped = re.escape(raw)
print("raw:      ", raw)
print("escaped:  ", escaped)
print("it matches the literal:", re.match(escaped, raw) is not None)
print("without escaping, the brackets are a character class:")
print("  ", re.match(r"file(1).txt", "file1.txt"))

print()
print("--- errors ---")
try:
    re.compile("(")
except re.error as err:
    print("an unbalanced group raises re.error:", err)
try:
    re.compile("[z-a]")
except re.error as err:
    print("a reversed class raises re.error:", err)

print()
print("--- a worked example: a simple log parser ---")
LOG = [
    "2024-01-15 08:00:01 INFO  starting up",
    "2024-01-15 08:00:02 WARN  disk at 85%",
    "2024-01-15 08:00:05 ERROR failed to connect",
    "this line is malformed",
]
LINE = re.compile(r"^(?P<date>\d{4}-\d{2}-\d{2}) (?P<time>\d{2}:\d{2}:\d{2}) "
                  r"(?P<level>[A-Z]+)\s+(?P<message>.+)$")

parsed = []
unparsed = []
for line in LOG:
    found = LINE.match(line)
    if found:
        parsed.append(found.groupdict())
    else:
        unparsed.append(line)

print("parsed %d of %d lines" % (len(parsed), len(LOG)))
for record in parsed:
    print("  %s %s [%s] %s"
          % (record["date"], record["time"], record["level"], record["message"]))
print("malformed lines:", unparsed)
print()
print("counts by level, via findall:")
levels = [record["level"] for record in parsed]
for level in ["INFO", "WARN", "ERROR"]:
    print("  %-6s %d" % (level, levels.count(level)))
