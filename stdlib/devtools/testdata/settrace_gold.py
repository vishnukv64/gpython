import sys
events = []
def tracer(frame, event, arg):
    name = frame.f_code.co_name
    if name in ("main", "helper", "gen", "boom", "loop"):
        events.append((name, event, frame.f_lineno - frame.f_code.co_firstlineno, repr(arg) if event != "exception" else arg[0].__name__))
    return tracer

def helper(a):
    b = a + 1
    return b

def boom():
    raise ValueError("x")

def gen():
    yield 1
    yield 2

def loop():
    t = 0
    for i in range(2):
        t += i
    while t < 3:
        t += 1
    return t

def main():
    x = helper(1)
    try:
        boom()
    except ValueError:
        pass
    for v in gen():
        x += v
    loop()
    return x

sys.settrace(tracer)
main()
sys.settrace(None)
for e in events: print(e)
print(sys.gettrace())
