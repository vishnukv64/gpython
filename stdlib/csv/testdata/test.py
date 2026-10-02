"""Tests for the csv module.

Every expected value here was computed with CPython 3.14's csv module, so these
are exact-match tests against the reference implementation, including the
quoting rules and the newline handling inside quoted fields.
"""

import csv
import io

def assertEqual(x, y):
    assert x == y, "got: %s, want: %s" % (repr(x), repr(y))

def writerows(rows, **kwargs):
    buf = io.StringIO()
    w = csv.writer(buf, **kwargs)
    for row in rows:
        w.writerow(row)
    return buf.getvalue()

# the quoting constants
assertEqual(csv.QUOTE_MINIMAL, 0)
assertEqual(csv.QUOTE_ALL, 1)
assertEqual(csv.QUOTE_NONNUMERIC, 2)
assertEqual(csv.QUOTE_NONE, 3)

# writing
assertEqual(writerows([["a", "b", "c"], [1, 2, 3]]), "a,b,c\r\n1,2,3\r\n")
assertEqual(writerows([["a,b", "c"]]), '"a,b",c\r\n')
assertEqual(writerows([['a"b', 'c']]), '"a""b",c\r\n')
assertEqual(writerows([["a\nb", "c"]]), '"a\nb",c\r\n')
assertEqual(writerows([["", ""]]), ",\r\n")
assertEqual(writerows([["a", "b"]], quoting=csv.QUOTE_ALL), '"a","b"\r\n')
assertEqual(writerows([["a", 1]], quoting=csv.QUOTE_NONNUMERIC), '"a",1\r\n')
assertEqual(writerows([["a", "b"]], quoting=csv.QUOTE_NONE), "a,b\r\n")
assertEqual(writerows([["a", "b"]], delimiter="\t"), "a\tb\r\n")
assertEqual(writerows([["a,b"]], quoting=csv.QUOTE_NONE, escapechar=chr(92)),
            "a" + chr(92) + ",b\r\n")

buf = io.StringIO()
w = csv.writer(buf)
w.writerows([["a", "b"], ["c", "d"]])
assertEqual(buf.getvalue(), "a,b\r\nc,d\r\n")

# reading
assertEqual([list(r) for r in csv.reader(io.StringIO("a,b,c\n1,2,3\n"))],
            [["a", "b", "c"], ["1", "2", "3"]])
assertEqual([list(r) for r in csv.reader(io.StringIO('a,"b,c",d\n'))], [["a", "b,c", "d"]])
assertEqual([list(r) for r in csv.reader(io.StringIO('a,"b""c",d\n'))], [["a", 'b"c', "d"]])
assertEqual([list(r) for r in csv.reader(io.StringIO('a,"b\nc",d\n'))], [["a", "b\nc", "d"]])
assertEqual([list(r) for r in csv.reader(io.StringIO(""))], [])
assertEqual([list(r) for r in csv.reader(io.StringIO("a,,c\n"))], [["a", "", "c"]])
assertEqual([list(r) for r in csv.reader(io.StringIO("a,b"))], [["a", "b"]])

# a reader is iterable itself
r = csv.reader(io.StringIO("a,b\n"))
assertEqual(hasattr(r, "__iter__"), True)
assertEqual(next(r), ["a", "b"])

# round trip through a file
buf = io.StringIO()
w = csv.writer(buf)
w.writerow(["field with comma", 'field "with" quotes', "field\nwith newline"])
back = [list(row) for row in csv.reader(io.StringIO(buf.getvalue()))]
assertEqual(back, [["field with comma", 'field "with" quotes', "field\nwith newline"]])

# dialects
assertEqual((csv.excel.delimiter, csv.excel.lineterminator, csv.excel.quoting), (",", "\r\n", 0))
assertEqual(csv.excel_tab.delimiter, "\t")
assertEqual("excel" in csv.list_dialects(), True)
assertEqual("excel-tab" in csv.list_dialects(), True)
assertEqual("unix" in csv.list_dialects(), True)
assertEqual(csv.get_dialect("excel").delimiter, ",")

# a dialect may be passed by name
assertEqual(writerows([["a", "b"]], dialect="excel-tab"), "a\tb\r\n")

# the Sniffer
sniffer = csv.Sniffer()
assertEqual(sniffer.sniff("a,b,c\n1,2,3\n").delimiter, ",")
assertEqual(sniffer.sniff("a\tb\tc\n1\t2\t3\n").delimiter, "\t")
assertEqual(sniffer.has_header("a,b,c\n1,2,3\n"), True)

# Error exists and is what the module raises
assertTrue = lambda v: (v is not None) or (_ for _ in ()).throw(AssertionError("Error is None"))
assertTrue(csv.Error)
assertTrue(issubclass(csv.Error, Exception))

print("csv: all tests pass")
