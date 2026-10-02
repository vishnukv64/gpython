"""Tests for the pprint module.

Every expected value here was computed with CPython 3.14's pprint, so these are
exact-match tests against the reference implementation, including where the line
breaks fall.
"""

import pprint
import io

def assertEqual(x, y):
    assert x == y, "got: %s, want: %s" % (repr(x), repr(y))

# short containers stay on one line
assertEqual(pprint.pformat([1, 2, 3]), "[1, 2, 3]")
assertEqual(pprint.pformat({"a": 1, "b": [1, 2, {"c": 3}]}), "{'a': 1, 'b': [1, 2, {'c': 3}]}")
assertEqual(pprint.pformat("hello"), "'hello'")
assertEqual(pprint.pformat((1,)), "(1,)")
assertEqual(pprint.pformat([]), "[]")
assertEqual(pprint.pformat({}), "{}")

# a width that does not fit breaks the container one item per line
assertEqual(pprint.pformat(list(range(30)), width=40),
            "[0,\n 1,\n 2,\n 3,\n 4,\n 5,\n 6,\n 7,\n 8,\n 9,\n 10,\n 11,\n 12,\n 13,\n 14,\n"
            " 15,\n 16,\n 17,\n 18,\n 19,\n 20,\n 21,\n 22,\n 23,\n 24,\n 25,\n 26,\n 27,\n 28,\n 29]")
assertEqual(pprint.pformat([[[1, 2], [3, 4]], [[5, 6]]], width=20), "[[[1, 2], [3, 4]],\n [[5, 6]]]")
assertEqual(pprint.pformat({"key1": "value1", "key2": "value2"}, width=20),
            "{'key1': 'value1',\n 'key2': 'value2'}")

# depth truncates with [...]
assertEqual(pprint.pformat([1, [2, [3, [4]]]], depth=2), "[1, [2, [...]]]")

# dict ordering
assertEqual(pprint.pformat({"b": 1, "a": 2}), "{'a': 2, 'b': 1}")
assertEqual(pprint.pformat({"b": 1, "a": 2}, sort_dicts=False), "{'b': 1, 'a': 2}")

# indent and compact
assertEqual(pprint.pformat([1, 2, 3], indent=8, width=20), "[1, 2, 3]")
assertEqual(pprint.pformat(list(range(10)), width=30, compact=True), "[0, 1, 2, 3, 4, 5, 6, 7, 8, 9]")

# pprint writes to a stream
buf = io.StringIO()
pprint.pprint({"a": 1, "b": [1, 2, 3]}, stream=buf, width=20)
assertEqual(buf.getvalue(), "{'a': 1,\n 'b': [1, 2, 3]}\n")

# pp defaults sort_dicts to False
buf2 = io.StringIO()
pprint.pp({"b": 1, "a": 2}, stream=buf2)
assertEqual(buf2.getvalue(), "{'b': 1, 'a': 2}\n")

# the PrettyPrinter class
pp = pprint.PrettyPrinter(width=20)
assertEqual(pp.pformat([[[1, 2], [3, 4]], [[5, 6]]]), "[[[1, 2], [3, 4]],\n [[5, 6]]]")
buf3 = io.StringIO()
pprint.PrettyPrinter(stream=buf3).pprint([1, 2, 3])
assertEqual(buf3.getvalue(), "[1, 2, 3]\n")

# isreadable / isrecursive
assertEqual(pprint.isreadable([1, 2, 3]), True)
assertEqual(pprint.isrecursive([1, 2, 3]), False)

print("pprint: all tests pass")
