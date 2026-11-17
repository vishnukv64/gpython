def show(name, f):
    try:
        print(name, f())
    except RecursionError:
        print(name, "RecursionError")

x = []
x.append(x)
z = []
z.append(z)
d = {}
d["a"] = d
e = {}
e["a"] = e
tp = ([],)
tp[0].append(tp)
s = set()
fs = frozenset([1])

show("list repr", lambda: repr(x))
show("list str", lambda: str(x))
show("list self eq", lambda: x == x)
show("list cross eq", lambda: x == z)
show("list cross ne", lambda: x != z)
show("list lt", lambda: x < z)
show("list in self", lambda: x in [x])
show("list in cross", lambda: x in [z])
show("dict repr", lambda: repr(d))
show("dict self eq", lambda: d == d)
show("dict cross eq", lambda: d == e)
show("tuple repr", lambda: repr(tp))
show("tuple self eq", lambda: tp == tp)
show("frozenset repr", lambda: repr(fs))
show("fstring", lambda: f"{x}")
show("format", lambda: "{}".format(x))

class R:
    def __repr__(self):
        return repr(self)

show("user repr", lambda: repr(R()))

from collections import deque, OrderedDict, defaultdict
q = deque()
q.append(q)
show("deque repr", lambda: repr(q))
o = OrderedDict()
o["a"] = o
show("ordereddict repr", lambda: repr(o))
dd = defaultdict(list)
dd[0] = dd
show("defaultdict repr", lambda: repr(dd))

doc="finished"
