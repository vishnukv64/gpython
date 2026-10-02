"""io: in-memory text and binary streams.

Run with:  /tmp/gpy examples/stdlib/io_demo.py

StringIO is the text stream, BytesIO the binary one. Both support write,
read, readline, seek and tell. Note that len() on a bytes object is NOT
implemented in this interpreter, so binary sizes are reported via the stream
position instead.
"""

import io

print("--- module-level constants and classes ---")
print("DEFAULT_BUFFER_SIZE:", io.DEFAULT_BUFFER_SIZE)
print("SEEK_SET/CUR/END:   ", io.SEEK_SET, io.SEEK_CUR, io.SEEK_END)
print("StringIO:           ", io.StringIO)
print("BytesIO:            ", io.BytesIO)
print("open:               ", io.open)
print("TextIOWrapper:      ", io.TextIOWrapper)
print("UnsupportedOperation:", io.UnsupportedOperation)

print()
print("=== StringIO: an in-memory text file ===")
buffer = io.StringIO()
buffer.write("first line\n")
buffer.write("second line\n")
buffer.write("third line\n")
print("after writing three lines:")
print("  getvalue():       ", repr(buffer.getvalue()))

buffer.seek(0)
print("after seek(0):")
print("  readline():       ", repr(buffer.readline()))
print("  tell():           ", buffer.tell())
print("  readline():       ", repr(buffer.readline()))
print("  read(5):          ", repr(buffer.read(5)))
print("  read():           ", repr(buffer.read()), "(the rest of the stream)")

print()
print("--- seeking to an absolute position ---")
buffer.seek(6)
print("seek(6) then read(4):", repr(buffer.read(4)))
buffer.seek(0, io.SEEK_END)
print("seek(0, SEEK_END) then tell():", buffer.tell(), "(the total length)")
buffer.seek(0)
print("seek(0, SEEK_SET) then tell():", buffer.tell())

print()
print("--- StringIO can start from a string ---")
preset = io.StringIO("line one\nline two\n")
print("readline loop:")
while True:
    line = preset.readline()
    if not line:
        break
    print("  %r" % line)

print()
print("--- writing non-strings to StringIO fails ---")
text_buffer = io.StringIO()
try:
    text_buffer.write(42)
except TypeError as err:
    print("write(42) raises TypeError:", err)
print("write(str(42)) works:", end=" ")
text_buffer.write(str(42))
print(repr(text_buffer.getvalue()))

print()
print("=== BytesIO: an in-memory binary file ===")
binary = io.BytesIO()
binary.write(b"hello ")
binary.write(b"binary ")
binary.write(b"world")
print("getvalue():        ", binary.getvalue())

binary.seek(0)
print("read(5):           ", binary.read(5))
print("tell():            ", binary.tell())
print("read(7):           ", binary.read(7))
print("read():            ", binary.read())

print()
print("--- BytesIO from an existing bytes value ---")
frombytes = io.BytesIO(b"abcdef")
print("read(2):           ", frombytes.read(2))
frombytes.seek(0)
print("after seek(0):     ", frombytes.read())

print()
print("--- measuring a binary stream without len() on bytes ---")
measured = io.BytesIO()
measured.write(b"net")
measured.write(b"work")
measured.seek(0, io.SEEK_END)
print("write 3 + 4 bytes, position is now:", measured.tell())
print("(len() on a bytes object is not implemented here)")

print()
print("--- a worked example: building a text report ---")
report = io.StringIO()
report.write("=== Sales Report ===\n")
rows = [("widgets", 12, 2.50), ("gadgets", 4, 10.00), ("cogs", 30, 0.25)]
total = 0.0
for name, qty, price in rows:
    line_total = qty * price
    total += line_total
    report.write("%-10s %3d x %6.2f = %8.2f\n" % (name, qty, price, line_total))
report.write("-" * 36 + "\n")
report.write("%-10s %22.2f\n" % ("TOTAL", total))
print(report.getvalue())

print("--- the same, then read back line by line ---")
report.seek(0)
line_number = 0
while True:
    line = report.readline()
    if not line:
        break
    line_number += 1
print("the report contained", line_number, "lines")

print()
print("--- a worked example: a CSV parser over an in-memory buffer ---")
CSV = """\
name,dept,salary
ann,eng,120
bob,ops,95
cy,eng,110
"""
source = io.StringIO(CSV)
header = source.readline().strip().split(",")
print("header:            ", header)
records = []
while True:
    line = source.readline()
    if not line:
        break
    fields = line.strip().split(",")
    record = {}
    for index, column in enumerate(header):
        record[column] = fields[index]
    records.append(record)
for record in records:
    print("  %-4s %-4s %s" % (record["name"], record["dept"], record["salary"]))
print("total salary:      ", sum(int(r["salary"]) for r in records))
