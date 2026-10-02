"""csv: read and write comma-separated values.

Run with:  /tmp/gpy examples/stdlib/csv_demo.py

csv.reader/csv.writer work on any file-like object. csv.DictReader and
csv.DictWriter are NOT present in this interpreter, so the demo builds dicts
from the header row by hand.

Interpreter notes:
  * csv.DictReader and csv.DictWriter are absent; see the hand-rolled
    dict_reader() below.
  * csv.register_dialect() accepts only a dialect name -- extra arguments,
    keyword or positional, raise TypeError.
  * str.splitlines() is absent, so files are split on "\\n" here.
"""

import csv
import io

print("--- the raw module surface ---")
print("reader:", callable(csv.reader), " writer:", callable(csv.writer))
print("DictReader available:", hasattr(csv, "DictReader"))
print("DictWriter available:", hasattr(csv, "DictWriter"))
print("built-in dialects:", csv.list_dialects())

print()
print("--- reading ---")
TEXT = "name,qty\nwidget,3\ngadget,10\n"
rows = list(csv.reader(io.StringIO(TEXT)))
print("text:", repr(TEXT))
for index, row in enumerate(rows):
    print("  row %d: %s" % (index, row))
print("every field is a str:", all([isinstance(field, str) for field in rows[0]]))

print()
print("--- a hand-rolled DictReader, since csv.DictReader is absent ---")


def dict_reader(lines):
    """Yield each data row as a dict keyed by the first (header) row."""
    rows_iter = csv.reader(iter(lines))
    header = None
    for index, row in enumerate(rows_iter):
        if index == 0:
            header = row
            continue
        record = {}
        for column, key in enumerate(header):
            record[key] = row[column]
        yield record


for record in dict_reader(TEXT.split("\n")):
    print("  ", record)
print("NOTE: dict printing order is not insertion order in this interpreter.")

print()
print("--- writing ---")
out = io.StringIO()
writer = csv.writer(out)
writer.writerow(["name", "qty"])
writer.writerow(["widget", 3])
writer.writerow(["gadget", 10])
print("written text:", repr(out.getvalue()))
print("(the writer uses \\r\\n line endings, as the csv module requires)")

print()
print("--- quoting: fields containing the delimiter get quoted ---")
quoted = io.StringIO()
csv.writer(quoted).writerow(["plain", "has,comma", 'has"quote', "multi\nline"])
print(repr(quoted.getvalue()))
print("reading it back:", list(csv.reader(io.StringIO(quoted.getvalue()))))

print()
print("--- and a field can be left unquoted with QUOTE_NONE ---")
none_quoted = io.StringIO()
csv.writer(none_quoted, quoting=csv.QUOTE_NONE).writerow(["a", "b"])
print("QUOTE_NONE:", repr(none_quoted.getvalue()))
all_quoted = io.StringIO()
csv.writer(all_quoted, quoting=csv.QUOTE_ALL).writerow(["a", "b c"])
print("QUOTE_ALL: ", repr(all_quoted.getvalue()))
print("constants: QUOTE_MINIMAL=%d QUOTE_ALL=%d QUOTE_NONE=%d"
      % (csv.QUOTE_MINIMAL, csv.QUOTE_ALL, csv.QUOTE_NONE))

print()
print("--- dialects ---")
print("excel delimiter:    ", repr(csv.get_dialect("excel").delimiter))
print("excel-tab delimiter:", repr(csv.get_dialect("excel-tab").delimiter))
tabbed = io.StringIO()
csv.writer(tabbed, dialect="excel-tab").writerow(["a", "b"])
print("excel-tab output:", repr(tabbed.getvalue()))

print()
print("--- a worked example: total a sales file ---")
SALES = io.StringIO(
    "product,units,price\n"
    "widget,3,2.50\n"
    "gadget,10,1.25\n"
    "doohickey,1,19.99\n"
)
total = 0.0
for record in dict_reader(SALES.getvalue().split("\n")):
    line_total = int(record["units"]) * float(record["price"])
    total = total + line_total
    print("  %-10s %2s x %5s = %7.2f"
          % (record["product"], record["units"], record["price"], line_total))
print("grand total: %.2f" % (total,))

print()
print("--- register_dialect() is present but very restricted ---")
print("existing dialects:", csv.list_dialects())
try:
    csv.register_dialect("mine", delimiter=";")
except TypeError as err:
    print("with keyword args -> TypeError:", err)
try:
    csv.register_dialect("mine", ";")
except TypeError as err:
    print("with a positional delimiter -> TypeError:", err)
print("registering a bare name works:", csv.register_dialect("mine"))
print("after registering:", csv.list_dialects())
bare = io.StringIO()
csv.writer(bare, dialect="mine").writerow(["a", "b"])
print("it uses the default comma:", repr(bare.getvalue()))
