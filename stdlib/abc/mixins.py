# Concrete methods of the collections.abc classes, as in CPython's
# _collections_abc.py.  The classes themselves are Go types; _install copies
# these onto them once per process.

class _MappingView:
    def __init__(self, mapping): self._mapping = mapping
    def __len__(self): return len(self._mapping)
    def __repr__(self): return type(self).__name__ + '(' + repr(self._mapping) + ')'

class _KeysView(_MappingView, KeysView):
    def __contains__(self, key): return key in self._mapping
    def __iter__(self): yield from self._mapping

class _ItemsView(_MappingView, ItemsView):
    def __contains__(self, item):
        key, value = item
        try: v = self._mapping[key]
        except KeyError: return False
        return v is value or v == value
    def __iter__(self):
        for key in self._mapping: yield (key, self._mapping[key])

class _ValuesView(_MappingView, ValuesView):
    def __contains__(self, value):
        for key in self._mapping:
            v = self._mapping[key]
            if v is value or v == value: return True
        return False
    def __iter__(self):
        for key in self._mapping: yield self._mapping[key]

_KeysView.__name__ = 'KeysView'
_ItemsView.__name__ = 'ItemsView'
_ValuesView.__name__ = 'ValuesView'

_marker = object()

def _get(self, key, default=None):
    try: return self[key]
    except KeyError: return default
def _contains(self, key):
    try: self[key]
    except KeyError: return False
    return True
def _keys(self): return _KeysView(self)
def _items(self): return _ItemsView(self)
def _values(self): return _ValuesView(self)
def _meq(self, other):
    if not isinstance(other, Mapping): return NotImplemented
    return dict(self.items()) == dict(other.items())

def _mpop(self, key, default=_marker):
    try: value = self[key]
    except KeyError:
        if default is _marker: raise
        return default
    del self[key]
    return value
def _popitem(self):
    try: key = next(iter(self))
    except StopIteration: raise KeyError from None
    value = self[key]
    del self[key]
    return key, value
def _mclear(self):
    try:
        while True: self.popitem()
    except KeyError: pass
def _update(self, other=(), /, **kwds):
    if isinstance(other, Mapping):
        for key in other: self[key] = other[key]
    elif hasattr(other, "keys"):
        for key in other.keys(): self[key] = other[key]
    else:
        for key, value in other: self[key] = value
    for key, value in kwds.items(): self[key] = value
def _setdefault(self, key, default=None):
    try: return self[key]
    except KeyError: self[key] = default
    return default

def _scontains(self, value):
    for v in self:
        if v is value or v == value: return True
    return False
def _siter(self):
    i = 0
    try:
        while True:
            yield self[i]
            i += 1
    except IndexError: return
def _sreversed(self):
    for i in reversed(range(len(self))): yield self[i]
def _index(self, value, start=0, stop=None):
    if start is not None and start < 0: start = max(len(self) + start, 0)
    if stop is not None and stop < 0: stop += len(self)
    i = start
    while stop is None or i < stop:
        try:
            v = self[i]
            if v is value or v == value: return i
        except IndexError: break
        i += 1
    raise ValueError
def _count(self, value): return sum(1 for v in self if v is value or v == value)

def _append(self, value): self.insert(len(self), value)
def _sclear(self):
    try:
        while True: self.pop()
    except IndexError: pass
def _reverse(self):
    n = len(self)
    for i in range(n // 2): self[i], self[n - i - 1] = self[n - i - 1], self[i]
def _extend(self, values):
    if values is self: values = list(values)
    for v in values: self.append(v)
def _spop(self, index=-1):
    v = self[index]
    del self[index]
    return v
def _remove(self, value): del self[self.index(value)]
def _iadd(self, values):
    self.extend(values)
    return self

def _isdisjoint(self, other):
    for value in other:
        if value in self: return False
    return True
def _from_iterable(cls, it): return cls(it)
def _le(self, other):
    if not isinstance(other, Set): return NotImplemented
    if len(self) > len(other): return False
    for elem in self:
        if elem not in other: return False
    return True
def _lt(self, other):
    if not isinstance(other, Set): return NotImplemented
    return len(self) < len(other) and self.__le__(other)
def _gt(self, other):
    if not isinstance(other, Set): return NotImplemented
    return len(self) > len(other) and self.__ge__(other)
def _ge(self, other):
    if not isinstance(other, Set): return NotImplemented
    if len(self) < len(other): return False
    for elem in other:
        if elem not in self: return False
    return True
def _seq(self, other):
    if not isinstance(other, Set): return NotImplemented
    return len(self) == len(other) and self.__le__(other)
def _and(self, other):
    if not isinstance(other, Iterable): return NotImplemented
    return self._from_iterable(value for value in other if value in self)
def _or(self, other):
    if not isinstance(other, Iterable): return NotImplemented
    return self._from_iterable([e for s in (self, other) for e in s])
def _sub(self, other):
    if not isinstance(other, Set):
        if not isinstance(other, Iterable): return NotImplemented
        other = self._from_iterable(other)
    return self._from_iterable(value for value in self if value not in other)
def _xor(self, other):
    if not isinstance(other, Set):
        if not isinstance(other, Iterable): return NotImplemented
        other = self._from_iterable(other)
    return (self - other) | (other - self)

def _spop_set(self):
    it = iter(self)
    try: value = next(it)
    except StopIteration: raise KeyError from None
    self.discard(value)
    return value
def _sremove(self, value):
    if value not in self: raise KeyError(value)
    self.discard(value)
def _setclear(self):
    try:
        while True: self.pop()
    except KeyError: pass
def _ior(self, it):
    for value in it: self.add(value)
    return self
def _iand(self, it):
    for value in (self - it): self.discard(value)
    return self
def _isub(self, it):
    if it is self: self.clear()
    else:
        for value in it: self.discard(value)
    return self

_install([
    (Mapping, (("get", _get), ("__contains__", _contains), ("keys", _keys), ("items", _items), ("values", _values), ("__eq__", _meq), ("__hash__", None),)),
    (MutableMapping, (("pop", _mpop), ("popitem", _popitem), ("clear", _mclear), ("update", _update), ("setdefault", _setdefault),)),
    (Sequence, (("__contains__", _scontains), ("__iter__", _siter), ("__reversed__", _sreversed), ("index", _index), ("count", _count),)),
    (MutableSequence, (("append", _append), ("clear", _sclear), ("reverse", _reverse), ("extend", _extend), ("pop", _spop), ("remove", _remove), ("__iadd__", _iadd),)),
    (Set, (("isdisjoint", _isdisjoint), ("_from_iterable", classmethod(_from_iterable)), ("__le__", _le), ("__lt__", _lt), ("__gt__", _gt), ("__ge__", _ge), ("__eq__", _seq), ("__and__", _and), ("__rand__", _and), ("__or__", _or), ("__ror__", _or), ("__sub__", _sub), ("__xor__", _xor), ("__rxor__", _xor), ("__hash__", None),)),
    (MutableSet, (("pop", _spop_set), ("remove", _sremove), ("clear", _setclear), ("__ior__", _ior), ("__iand__", _iand), ("__isub__", _isub),)),
])
