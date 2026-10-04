import sys, traceback

def probe(label, fn):
    try:
        fn()
    except BaseException as e:
        t = sys.exc_info()[0]
        print(label, t.__name__ if t else None)

probe("IndexError", lambda: [][5])
probe("ZeroDivision", lambda: 1 / 0)
probe("KeyError", lambda: {}["nope"])
probe("AttributeError", lambda: (1).nope)
probe("ValueError", lambda: int("x"))
probe("FileNotFound", lambda: open("/no-such-file-xyz"))

# Outside any handler the state is cleared.
print("outside:", sys.exc_info())

# format_exc names the right exception type.
try:
    [][5]
except IndexError:
    line = traceback.format_exc().splitlines()[-1]
    print("format_exc last line:", line)

# A nested handler restores the outer state on exit.
try:
    try:
        [][5]
    except IndexError:
        try:
            {}["k"]
        except KeyError:
            print("inner:", sys.exc_info()[0].__name__)
        print("after inner:", sys.exc_info()[0].__name__)
except BaseException:
    pass
print("after all:", sys.exc_info())

doc = "finished"
