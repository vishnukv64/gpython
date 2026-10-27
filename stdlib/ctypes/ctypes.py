"""Fixed-size C types over a buffer.  See ctypes.go for what is and is not here."""
import struct as _struct

_LP64 = _struct.calcsize("P") == 8


class _Meta(type):
    # `T * n` makes an array type of n T.
    def __mul__(cls, n):
        if not isinstance(n, int) or n < 0:
            raise ValueError("Array length must be >= 0, not %r" % (n,))
        key = (cls, n)
        t = _array_cache.get(key)
        if t is None:
            t = _array_cache[key] = _Meta(
                "%s_Array_%d" % (cls.__name__, n), (Array,),
                {"_type_": cls, "_length_": n, "_size_": cls._size_ * n})
        return t

    __rmul__ = __mul__


_array_cache = {}


class _CData(metaclass=_Meta):
    _size_ = 0

    @classmethod
    def from_buffer(cls, source, offset=0):
        mv = memoryview(source)
        if mv.readonly:
            raise TypeError("underlying buffer is not writable")
        if len(mv) - offset < cls._size_:
            raise ValueError("Buffer size too small (%d instead of at least %d bytes)"
                             % (len(mv), cls._size_ + offset))
        self = cls.__new__(cls)
        self._buf = mv[offset:offset + cls._size_]
        return self

    @classmethod
    def from_buffer_copy(cls, source, offset=0):
        data = bytes(memoryview(source))[offset:offset + cls._size_]
        if len(data) < cls._size_:
            raise ValueError("Buffer size too small")
        return cls.from_buffer(bytearray(data))

    def _alloc(self):
        self._buf = memoryview(bytearray(self._size_))

    def _get_bytes(self, off, n):
        return bytes(self._buf[off:off + n])

    def _put_bytes(self, off, data):
        buf = self._buf
        for i, b in enumerate(data):
            buf[off + i] = b


class _SimpleCData(_CData):
    _fmt_ = None
    _default_ = 0

    def __init__(self, *args):
        if not hasattr(self, "_buf"):
            self._alloc()
        self.value = args[0] if args else self._default_

    @property
    def value(self):
        return self._load()

    @value.setter
    def value(self, v):
        self._store(v)

    def _load(self):
        return _struct.unpack(self._fmt_, self._get_bytes(0, self._size_))[0]

    def _store(self, v):
        if self._fmt_ in "fd":
            v = float(v)
        elif self._fmt_ == "?":
            v = bool(v)
        elif not isinstance(v, int):
            raise TypeError("an integer is required" if self._fmt_ not in "fd" else "a float is required")
        elif v.bit_length() > 8 * self._size_:
            v &= (1 << (8 * self._size_)) - 1
            if self._fmt_.islower() and v >> (8 * self._size_ - 1):
                v -= 1 << (8 * self._size_)
        self._put_bytes(0, _struct.pack(self._fmt_, v))

    def __repr__(self):
        return "%s(%r)" % (type(self).__name__, self.value)


def _simple(name, fmt, default=0):
    return _Meta(name, (_SimpleCData,), {"_fmt_": fmt, "_size_": _struct.calcsize(fmt), "_default_": default})


c_byte = _simple("c_byte", "b")
c_ubyte = _simple("c_ubyte", "B")
c_short = _simple("c_short", "h")
c_ushort = _simple("c_ushort", "H")
c_int = _simple("c_int", "i")
c_uint = _simple("c_uint", "I")
c_long = _simple("c_long", "q" if _LP64 else "i")
c_ulong = _simple("c_ulong", "Q" if _LP64 else "I")
c_longlong = _simple("c_longlong", "q")
c_ulonglong = _simple("c_ulonglong", "Q")
c_float = _simple("c_float", "f", 0.0)
c_double = _simple("c_double", "d", 0.0)
c_bool = _simple("c_bool", "?", False)
c_size_t = c_ulong
c_ssize_t = c_long
c_int8, c_uint8 = c_byte, c_ubyte
c_int16, c_uint16 = c_short, c_ushort
c_int32, c_uint32 = c_int, c_uint
c_int64, c_uint64 = c_longlong, c_ulonglong


class c_char(_SimpleCData):
    _fmt_ = "c"
    _size_ = 1
    _default_ = b"\0"

    def _store(self, v):
        if isinstance(v, int):
            v = bytes([v])
        if not isinstance(v, (bytes, bytearray)) or len(v) != 1:
            raise TypeError("one character bytes, bytearray or integer expected")
        self._put_bytes(0, v)


class c_wchar(_SimpleCData):
    _fmt_ = "I"
    _size_ = 4
    _default_ = "\0"

    def _load(self):
        return chr(_struct.unpack("I", self._get_bytes(0, 4))[0])

    def _store(self, v):
        if not isinstance(v, str) or len(v) != 1:
            raise TypeError("one character unicode string expected")
        self._put_bytes(0, _struct.pack("I", ord(v)))


class Array(_CData):
    _type_ = None
    _length_ = 0

    def __init__(self, *args):
        if not hasattr(self, "_buf"):
            self._alloc()
        if len(args) > self._length_:
            raise IndexError("too many initializers")
        for i, a in enumerate(args):
            self[i] = a

    def __len__(self):
        return self._length_

    def _norm(self, i):
        if i < 0:
            i += self._length_
        if not 0 <= i < self._length_:
            raise IndexError("invalid index")
        return i

    def _elem(self, i):
        t = self._type_
        e = t.from_buffer(self._buf, i * t._size_)
        return e

    def __getitem__(self, i):
        if isinstance(i, slice):
            return [self[j] for j in range(*i.indices(self._length_))]
        e = self._elem(self._norm(i))
        if isinstance(e, _SimpleCData):
            return e.value
        return e

    def __setitem__(self, i, v):
        if isinstance(i, slice):
            idx = range(*i.indices(self._length_))
            vals = list(v)
            if len(vals) != len(idx):
                raise ValueError("Can only assign sequence of same size")
            for j, x in zip(idx, vals):
                self[j] = x
            return
        e = self._elem(self._norm(i))
        if isinstance(e, _SimpleCData):
            e.value = v
        else:
            _copy_into(e, v)

    def __iter__(self):
        for i in range(self._length_):
            yield self[i]

    def __repr__(self):
        return "<%s object>" % type(self).__name__

    @property
    def raw(self):
        return self._get_bytes(0, self._size_)

    @raw.setter
    def raw(self, data):
        if len(data) > self._size_:
            raise ValueError("byte string too long")
        self._put_bytes(0, data)

    @property
    def value(self):
        if self._type_ is c_char:
            data = self.raw
            n = data.find(b"\0")
            return data if n < 0 else data[:n]
        if self._type_ is c_wchar:
            s = "".join(self)
            n = s.find("\0")
            return s if n < 0 else s[:n]
        raise AttributeError("value")

    @value.setter
    def value(self, v):
        if self._type_ is c_char:
            if len(v) > self._length_:
                raise ValueError("byte string too long")
            self._put_bytes(0, v + b"\0" * (self._length_ - len(v)) if len(v) < self._length_ else v)
        elif self._type_ is c_wchar:
            if len(v) > self._length_:
                raise ValueError("string too long")
            for i, ch in enumerate(v + "\0" * (self._length_ - len(v))):
                self[i] = ch
        else:
            raise AttributeError("value")


def _copy_into(dst, src):
    if type(src) is not type(dst) and not isinstance(src, type(dst)):
        raise TypeError("incompatible types, %s instance instead of %s instance"
                        % (type(src).__name__, type(dst).__name__))
    dst._put_bytes(0, src._get_bytes(0, src._size_))


class _Field:
    def __init__(self, name, ctype, offset):
        self.name, self.ctype, self.offset = name, ctype, offset

    def __get__(self, obj, owner=None):
        if obj is None:
            return self
        e = self.ctype.from_buffer(obj._buf, self.offset)
        return e.value if isinstance(e, _SimpleCData) else e

    def __set__(self, obj, v):
        e = self.ctype.from_buffer(obj._buf, self.offset)
        if isinstance(e, _SimpleCData):
            e.value = v
        else:
            _copy_into(e, v)


class _StructMeta(_Meta):
    def __new__(mcs, name, bases, ns):
        cls = super().__new__(mcs, name, bases, ns)
        fields = ns.get("_fields_")
        if fields is not None:
            off = 0
            for f in fields:
                fname, ftype = f[0], f[1]
                al = min(ftype._size_, 8) or 1
                off = (off + al - 1) // al * al
                setattr(cls, fname, _Field(fname, ftype, off))
                off += ftype._size_
            cls._size_ = (off + 7) // 8 * 8 if off else 0
        return cls


class Structure(_CData, metaclass=_StructMeta):
    _fields_ = ()

    def __init__(self, *args, **kwargs):
        if not hasattr(self, "_buf"):
            self._alloc()
        for (f, v) in zip(self._fields_, args):
            setattr(self, f[0], v)
        for k, v in kwargs.items():
            setattr(self, k, v)


def sizeof(obj):
    if isinstance(obj, _Meta):
        return obj._size_
    if isinstance(obj, _CData):
        return obj._size_
    raise TypeError("this type has no size")


class _Addr:
    def __init__(self, obj):
        self.obj = obj


def addressof(obj):
    if not isinstance(obj, _CData):
        raise TypeError("invalid type")
    return _Addr(obj)


def memset(dest, c, count):
    d = dest.obj if isinstance(dest, _Addr) else dest
    d._put_bytes(0, bytes([c]) * count)
    return dest


def memmove(dest, src, count):
    d = dest.obj if isinstance(dest, _Addr) else dest
    s = src.obj if isinstance(src, _Addr) else src
    data = s._get_bytes(0, count) if isinstance(s, _CData) else bytes(s)[:count]
    d._put_bytes(0, data)
    return dest


class _Pointer:
    """pointer(obj): only item 0 is addressable, which is all copy() needs."""

    def __init__(self, obj):
        self.contents = obj

    def __getitem__(self, i):
        if i != 0:
            raise IndexError("pointer arithmetic is not supported")
        return self.contents

    def __setitem__(self, i, v):
        if i != 0:
            raise IndexError("pointer arithmetic is not supported")
        _copy_into(self.contents, v)


def pointer(obj):
    if not isinstance(obj, _CData):
        raise TypeError("_type_ must have storage info")
    return _Pointer(obj)
