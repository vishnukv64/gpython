import inspect, sys, functools

def deco(f):
    return f


@deco
def plain(a, b=(1,
                2)):
    """doc

    more"""
    x = '''
    not code
'''
    return a


def oneline(): return 1


class Base:
    """Doc."""

    attr = 1

    def method(self):
        return {
            'a': 1,
        }

    @staticmethod
    def sm():
        pass

    @property
    def prop(self):
        return 2


@deco
class Decorated(Base):
    def other(self): pass


lam = lambda x: (
    x + 1)


def gen():
    yield 1


async def co():
    pass


@functools.wraps(plain)
def wrapper(*a):
    return plain(*a)


def show(label, fn, *args):
    try:
        r = fn(*args)
    except Exception as e:
        print(label, 'ERR', type(e).__name__)
        return
    if isinstance(r, tuple) and isinstance(r[0], list):
        print(label, r[1], repr(''.join(r[0])))
    else:
        print(label, repr(r) if not isinstance(r, str) or len(r) < 60 else repr(r))


def base(x):
    return None if x is None else x.replace(chr(92), '/').rsplit('/', 1)[-1]

for name, obj in [('plain', plain), ('oneline', oneline), ('Base', Base), ('Base.method', Base.method),
                  ('sm', Base.sm), ('Decorated', Decorated), ('Decorated.other', Decorated.other),
                  ('lam', lam), ('gen', gen), ('co', co), ('wrapper', wrapper),
                  ('bound', Base().method), ('code', plain.__code__)]:
    show(name + ' src', inspect.getsource, obj)
    show(name + ' lines', inspect.getsourcelines, obj)
    print(name, 'file', base(inspect.getsourcefile(obj)), base(inspect.getfile(obj)))
    r = inspect.findsource(obj)
    print(name, 'find', r[1], len(r[0]) > 10)

me = sys.modules[__name__]
print('module', len(inspect.getsource(me).splitlines()) > 50, inspect.getsourcelines(me)[1], base(inspect.getsourcefile(me)))
for label, bad in (("int", 3), ("sys", sys), ("len", len), ("str", "str"), ("type int", int)):
    show(label, lambda o: type(o).__name__ if False else inspect.getsource(o).__class__.__name__, bad)
show('frame', inspect.getsource, sys._getframe())
try:
    1/0
except ZeroDivisionError as e:
    show('tb', inspect.getsourcelines, e.__traceback__)
