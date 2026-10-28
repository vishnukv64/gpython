import faulthandler, os, sys

HERE = os.path.dirname(os.path.abspath(__file__))

def grab(**kw):
    r, w = os.pipe()
    faulthandler.dump_traceback(file=w, **kw)
    os.close(w)
    data = os.read(r, 10000).decode()
    os.close(r)
    return data.replace(HERE + os.sep, "").replace(HERE, "")

def inner():
    return grab(all_threads=False)
def outer():
    return inner()
class C:
    def m(self):
        return grab(all_threads=False)

print(outer())
print(C().m())
print(eval(compile("grab(all_threads=False)", "<str>", "eval")))
first = grab(all_threads=True).splitlines()[0]
print(first.startswith("Current thread 0x") and first.endswith("(most recent call first):"))

print(faulthandler.is_enabled())
faulthandler.enable()
print(faulthandler.is_enabled())
print(faulthandler.disable(), faulthandler.disable(), faulthandler.is_enabled())
try:
    faulthandler.dump_traceback(file=-1)
except ValueError as e:
    print("ValueError", e)
