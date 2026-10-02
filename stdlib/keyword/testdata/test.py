# The behaviour of keyword, checked through the interpreter.
import keyword

# kwlist holds every keyword, and is a list of str in CPython's order.
assert isinstance(keyword.kwlist, list), keyword.kwlist
assert len(keyword.kwlist) == 35, len(keyword.kwlist)
assert keyword.kwlist[:4] == ['False', 'None', 'True', 'and'], keyword.kwlist[:4]
assert keyword.kwlist[-1] == 'yield', keyword.kwlist[-1]

# softkwlist is separate and disjoint from kwlist.
assert len(keyword.softkwlist) == 4, keyword.softkwlist
assert keyword.softkwlist == ['_', 'case', 'match', 'type'], keyword.softkwlist
for w in keyword.softkwlist:
    assert w not in keyword.kwlist, w

# iskeyword answers for every listed word and for nothing else.
for w in keyword.kwlist:
    assert keyword.iskeyword(w) is True, w
for w in keyword.softkwlist + ['foo', 'bar', 'Print', 'TRUE', '']:
    if w in keyword.kwlist:
        continue
    assert keyword.iskeyword(w) is False, w

# issoftkeyword answers for the soft list and NOT for the hard one, so the two
# predicates are genuinely different.
for w in keyword.softkwlist:
    assert keyword.issoftkeyword(w) is True, w
assert keyword.issoftkeyword('for') is False
assert keyword.issoftkeyword('foo') is False
assert keyword.iskeyword('match') is False
assert keyword.iskeyword('type') is False

# A non-string argument is refused by name, which is what a caller that passes
# a non-identifier by mistake should see.
for bad in [1, None, 1.5, b'for']:
    try:
        keyword.iskeyword(bad)
    except TypeError:
        pass
    else:
        raise AssertionError("iskeyword accepted " + repr(bad))
    try:
        keyword.issoftkeyword(bad)
    except TypeError:
        pass
    else:
        raise AssertionError("issoftkeyword accepted " + repr(bad))

# A wrong number of arguments is a TypeError, not a crash.
try:
    keyword.iskeyword()
except TypeError:
    pass
else:
    raise AssertionError("iskeyword() with no arguments did not raise")

print("keyword ok")
