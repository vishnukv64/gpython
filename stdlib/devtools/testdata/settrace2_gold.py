import sys

log = []

def prof(frame, event, arg):
    if frame.f_code.co_name in ("a", "b", "c", "g"):
        log.append((frame.f_code.co_name, event, repr(arg)))

def a():
    return b() + 1

def b():
    return c()

def c():
    raise KeyError("k")

def g():
    yield 1
    return 7

def run():
    try:
        a()
    except KeyError:
        pass
    for _ in g():
        pass

sys.setprofile(prof)
run()
sys.setprofile(None)
print("profile events")
for e in log: print(e)
print(sys.getprofile())

# local trace function: None means "do not trace lines in this frame"
lines = []
def t_global(frame, event, arg):
    if frame.f_code.co_name == "traced":
        return t_local
    return None
def t_local(frame, event, arg):
    lines.append((event, frame.f_lineno - frame.f_code.co_firstlineno))
    return t_local
def traced():
    x = 1
    y = 2
    return x + y
def untraced():
    return 5
sys.settrace(t_global)
traced(); untraced()
sys.settrace(None)
print("local", lines)

# f_trace assigned from inside a tracer, and visible afterwards
seen = []
def t2(frame, event, arg):
    if frame.f_code.co_name == "inner":
        frame.f_trace = lambda f, e, a: seen.append(e)
    return None
def inner():
    return 1
sys.settrace(t2)
inner()
sys.settrace(None)
print("f_trace", seen)

# the tracer raising stops tracing and propagates
def bad(frame, event, arg):
    raise RuntimeError("tracer failed")
try:
    sys.settrace(bad)
    inner()
except RuntimeError as e:
    print("raised", e)
print(sys.gettrace())

# the frame being traced
frames = []
def t3(frame, event, arg):
    if event == "call" and frame.f_code.co_name == "inner":
        frames.append((frame.f_back.f_code.co_name, frame.f_lineno - frame.f_code.co_firstlineno, frame.f_trace))
    return None
sys.settrace(t3)
inner()
sys.settrace(None)
print("frame", frames)
