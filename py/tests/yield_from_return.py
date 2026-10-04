"""yield from delivers the sub-generator's return value (PEP 380)."""

def sub():
    yield 1
    yield 2
    return "RETURNED"


def outer():
    r = yield from sub()
    print("sub returned:", repr(r))
    yield 99


print("outer:", list(outer()))

# A sub-generator with no return value gives None.
def bare():
    yield 1


def wrap():
    r = yield from bare()
    print("bare returned:", repr(r))


print("wrap:", list(wrap()))

# yield from still forwards the yielded values, in order.
def three():
    yield "a"
    yield "b"


print("forwarded:", list((x for x in three())))

doc = "finished"
