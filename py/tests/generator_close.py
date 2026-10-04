def g():
    try:
        yield 1
        yield 2
    finally:
        print("cleanup ran")

it = g()
print("next:", next(it))
print("close returns:", it.close())
print("already closed is a no-op:", it.close())

# A generator that refuses to close by yielding again.
def stubborn():
    while True:
        try:
            yield 1
        except GeneratorExit:
            print("refusing")
            yield 2

s = stubborn()
next(s)
try:
    s.close()
except RuntimeError as e:
    print("RuntimeError:", e)

# A generator that has not started: closing does nothing.
def never():
    yield 1

print("unstarted close:", never().close())

# An exhausted generator: also nothing.
def once():
    yield 1

o = once()
next(o)
try:
    next(o)
except StopIteration:
    pass
print("exhausted close:", o.close())

doc = "finished"
