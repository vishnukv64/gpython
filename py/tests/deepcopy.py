import copy

class P:
    def __init__(s): s.a = [1, 2]; s.t = (1, [2])
class Cyc:
    pass
class Sl:
    __slots__ = ('x', 'y')
class SlD(Sl):
    pass
class DC:
    def __init__(s): s.v = [1]
    def __deepcopy__(s, memo):
        n = DC(); n.v = ['custom'] + copy.deepcopy(s.v, memo); return n
class CC:
    def __copy__(s): return 'shallow-hook'
class LS(list): pass
class DS(dict): pass
class St:
    def __init__(s): s.a = [1]; s.skip = 'x'
    def __getstate__(s): return {'a': s.a}
    def __setstate__(s, d): s.a = d['a']; s.restored = True
class Red:
    def __init__(s, n): s.n = n
    def __reduce__(s): return (Red, (s.n,))
class ListRed:
    def __init__(s): s.items = [[1], [2]]
    def __reduce__(s): return (ListRed2, (), None, iter(s.items), None)
class ListRed2:
    def __init__(s): s.l = []
    def append(s, x): s.l.append(x)
class DictRed:
    def __reduce__(s): return (DictRed2, (), None, None, iter([('k', [1])]))
class DictRed2(dict): pass

def show(label, v): print(label, v)

r = P(); c = copy.deepcopy(r)
show('inst', (c is r, c.a == r.a, c.a is r.a, c.t[1] is r.t[1], type(c).__name__))
a = []; a.append(a); b = copy.deepcopy(a); show('listcycle', (b is a, b[0] is b))
d = {}; d['me'] = d; e = copy.deepcopy(d); show('dictcycle', (e is d, e['me'] is e))
x = Cyc(); x.self = x; y = copy.deepcopy(x); show('instcycle', (y is x, y.self is y))
sh = [1]; o = copy.deepcopy([sh, sh, {'k': sh}]); show('shared', (o[0] is o[1], o[2]['k'] is o[0], o[0] is sh))
s = Sl(); s.x = [1]; t = copy.deepcopy(s); show('slots', (t.x, t.x is s.x, hasattr(t, 'y')))
s2 = SlD(); s2.x = 1; s2.y = [2]; t2 = copy.deepcopy(s2); show('slotsub', (t2.x, t2.y, t2.y is s2.y))
dc = copy.deepcopy(DC()); show('__deepcopy__', dc.v)
show('__copy__', copy.copy(CC()))
l = LS([1, [2]]); l.z = [3]; m = copy.deepcopy(l); show('listsub', (type(m).__name__, m, m.z, m[1] is l[1], m.z is l.z))
ds = DS(a=[1]); n = copy.deepcopy(ds); show('dictsub', (type(n).__name__, n, n['a'] is ds['a']))
st = copy.deepcopy(St()); show('setstate', (st.a, st.restored, hasattr(st, 'skip')))
rd = copy.deepcopy(Red([1])); show('reduce', (type(rd).__name__, rd.n))
lr = copy.deepcopy(ListRed()); show('listitems', (type(lr).__name__, lr.l))
dr = copy.deepcopy(DictRed()); show('dictitems', (type(dr).__name__, dr))
tp = (1, 'a', (2, 3)); show('tuple-atomic', copy.deepcopy(tp) is tp)
tm = ([1], 2); tc = copy.deepcopy(tm); show('tuple-mut', (tc is tm, tc[0] is tm[0], tc))
fs = frozenset([1, 2]); show('frozenset', (copy.deepcopy(fs), copy.deepcopy(fs) == fs))
ss = {1, 2, 3}; sc = copy.deepcopy(ss); show('set', (sc == ss, sc is ss))
ba = bytearray(b'ab'); bc = copy.deepcopy(ba); show('bytearray', (bc, bc is ba))
for v in (None, 1, 2.5, 'str', True, int, len, P, range(3)):
    show('atomic', copy.deepcopy(v) is v)
memo = {}; z = [P()]; z2 = copy.deepcopy(z, memo); show('memo', (id(z) in memo, memo[id(z)] is z2))
show('copy-inst', (copy.copy(r) is r, copy.copy(r).a is r.a))
show('copy-list', (copy.copy([1, [2]]), copy.copy(LS([1])).__class__.__name__))
show('copy-slots', (copy.copy(s).x is s.x))
show('copy-dict', copy.copy({'a': 1}))
try:
    copy.replace(1)
except TypeError as ex:
    show('replace', 'TypeError')
