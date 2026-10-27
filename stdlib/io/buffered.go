package io

import (
	"github.com/vishnukv64/gpython/py"
)

// BufferedReader / BufferedWriter wrap any raw stream object (a python-level
// RawIOBase subclass, as gzip and tarfile use them).
// ponytail: read-ahead is one chunk and no BufferedRandom; the raw object does
// the real I/O. Upgrade if a caller needs peek() beyond the buffered chunk.

type buffered struct {
	writer bool
	raw    py.Object
	buf    []byte // reader: unread bytes; writer: pending bytes
	size   int
	closed bool
}

func (b *buffered) Type() *py.Type {
	if b.writer {
		return BufferedWriterType
	}
	return BufferedReaderType
}

func rawCall(raw py.Object, name string, args ...py.Object) (py.Object, error) {
	fn, err := py.GetAttrString(raw, name)
	if err != nil {
		return nil, err
	}
	return py.Call(fn, py.Tuple(args), py.NewStringDict())
}

func newBufferedType(name string) *py.Type {
	return py.NewTypeX("io."+name, "Buffered stream over a raw stream.", func(_ *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		var raw py.Object
		var size py.Object = py.Int(8192)
		if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:"+name, []string{"raw", "buffer_size"}, &raw, &size); err != nil {
			return nil, err
		}
		n, err := py.IndexInt(size)
		if err != nil {
			return nil, err
		}
		return &buffered{writer: name == "BufferedWriter", raw: raw, size: n}, nil
	}, nil)
}

var (
	BufferedReaderType = newBufferedType("BufferedReader")
	BufferedWriterType = newBufferedType("BufferedWriter")
)

func (b *buffered) bytesOf(o py.Object) ([]byte, error) {
	if o == py.None {
		return nil, nil
	}
	v, err := py.BytesFromObject(o)
	return []byte(v), err
}

// fill reads at least one chunk from raw into buf; false at EOF.
func (b *buffered) fill() (bool, error) {
	r, err := rawCall(b.raw, "read", py.Int(max(b.size, 1)))
	if err != nil {
		return false, err
	}
	data, err := b.bytesOf(r)
	if err != nil || len(data) == 0 {
		return false, err
	}
	b.buf = append(b.buf, data...)
	return true, nil
}

func (b *buffered) read(n int) (py.Object, error) {
	for n < 0 || len(b.buf) < n {
		ok, err := b.fill()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
	}
	if n < 0 || n > len(b.buf) {
		n = len(b.buf)
	}
	out := py.Bytes(append([]byte(nil), b.buf[:n]...))
	b.buf = b.buf[n:]
	return out, nil
}

func (b *buffered) flush() error {
	if len(b.buf) > 0 {
		data := py.Bytes(b.buf)
		b.buf = nil
		if _, err := rawCall(b.raw, "write", data); err != nil {
			return err
		}
	}
	if fn, err := py.GetAttrString(b.raw, "flush"); err == nil {
		_, err = py.Call(fn, nil, py.NewStringDict())
		return err
	}
	return nil
}

func (b *buffered) doClose() error {
	if b.closed {
		return nil
	}
	if b.writer {
		if err := b.flush(); err != nil {
			return err
		}
	}
	b.closed = true
	_, err := rawCall(b.raw, "close")
	return err
}

func init() {
	for _, t := range []*py.Type{BufferedReaderType, BufferedWriterType} {
		self := func(o py.Object) *buffered { return o.(*buffered) }
		set := func(name string, fn func(b *buffered, args py.Tuple) (py.Object, error)) {
			t.Dict.Set(name, py.MustNewMethod(name, func(o py.Object, args py.Tuple) (py.Object, error) {
				b := self(o)
				if b.closed && name != "close" && name != "flush" {
					return nil, py.ExceptionNewf(py.ValueError, "I/O operation on closed file.")
				}
				return fn(b, args)
			}, 0, name))
		}
		set("close", func(b *buffered, _ py.Tuple) (py.Object, error) { return py.None, b.doClose() })
		set("flush", func(b *buffered, _ py.Tuple) (py.Object, error) {
			if b.writer && !b.closed {
				return py.None, b.flush()
			}
			return py.None, nil
		})
		set("readable", func(b *buffered, _ py.Tuple) (py.Object, error) { return py.NewBool(!b.writer), nil })
		set("writable", func(b *buffered, _ py.Tuple) (py.Object, error) { return py.NewBool(b.writer), nil })
		set("seekable", func(b *buffered, _ py.Tuple) (py.Object, error) { return rawCall(b.raw, "seekable") })
		set("fileno", func(b *buffered, _ py.Tuple) (py.Object, error) { return rawCall(b.raw, "fileno") })
		set("isatty", func(b *buffered, _ py.Tuple) (py.Object, error) { return py.False, nil })
		set("detach", func(b *buffered, _ py.Tuple) (py.Object, error) { b.closed = true; return b.raw, nil })
		t.Dict.Set("raw", &py.Property{Fget: func(o py.Object) (py.Object, error) { return o.(*buffered).raw, nil }})
		t.Dict.Set("closed", &py.Property{Fget: func(o py.Object) (py.Object, error) { return py.NewBool(o.(*buffered).closed), nil }})
		t.Dict.Set("name", &py.Property{Fget: func(o py.Object) (py.Object, error) { return py.GetAttrString(o.(*buffered).raw, "name") }})
		t.Dict.Set("__enter__", py.MustNewMethod("__enter__", func(o py.Object) (py.Object, error) { return o, nil }, 0, ""))
		t.Dict.Set("__exit__", py.MustNewMethod("__exit__", func(o py.Object, args py.Tuple) (py.Object, error) {
			return py.False, self(o).doClose()
		}, 0, ""))
	}
	w := BufferedWriterType.Dict
	w.Set("write", py.MustNewMethod("write", func(o py.Object, args py.Tuple) (py.Object, error) {
		b := o.(*buffered)
		if b.closed {
			return nil, py.ExceptionNewf(py.ValueError, "write to closed file")
		}
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "write() takes exactly one argument")
		}
		data, err := b.bytesOf(args[0])
		if err != nil {
			return nil, err
		}
		b.buf = append(b.buf, data...)
		if len(b.buf) >= b.size {
			if err := b.flush(); err != nil {
				return nil, err
			}
		}
		return py.Int(len(data)), nil
	}, 0, "write(b) -> int"))

	r := BufferedReaderType.Dict
	intArg := func(args py.Tuple) (int, error) {
		if len(args) == 0 || args[0] == py.None {
			return -1, nil
		}
		return py.IndexInt(args[0])
	}
	r.Set("read", py.MustNewMethod("read", func(o py.Object, args py.Tuple) (py.Object, error) {
		b := o.(*buffered)
		n, err := intArg(args)
		if err != nil {
			return nil, err
		}
		return b.read(n)
	}, 0, "read(size=-1) -> bytes"))
	r.Set("read1", py.MustNewMethod("read1", func(o py.Object, args py.Tuple) (py.Object, error) {
		b := o.(*buffered)
		n, err := intArg(args)
		if err != nil {
			return nil, err
		}
		if len(b.buf) == 0 {
			if _, err := b.fill(); err != nil {
				return nil, err
			}
		}
		if n < 0 || n > len(b.buf) {
			n = len(b.buf)
		}
		return b.read(n)
	}, 0, "read1(size=-1) -> bytes"))
	r.Set("peek", py.MustNewMethod("peek", func(o py.Object, args py.Tuple) (py.Object, error) {
		b := o.(*buffered)
		if len(b.buf) == 0 {
			if _, err := b.fill(); err != nil {
				return nil, err
			}
		}
		return py.Bytes(append([]byte(nil), b.buf...)), nil
	}, 0, "peek(size=0) -> bytes"))
	r.Set("readline", py.MustNewMethod("readline", func(o py.Object, args py.Tuple) (py.Object, error) {
		b := o.(*buffered)
		limit, err := intArg(args)
		if err != nil {
			return nil, err
		}
		for i := 0; ; i++ {
			for ; i < len(b.buf); i++ {
				if b.buf[i] == '\n' || (limit >= 0 && i+1 >= limit) {
					return b.read(i + 1)
				}
			}
			if ok, err := b.fill(); err != nil || !ok {
				if err != nil {
					return nil, err
				}
				return b.read(-1)
			}
		}
	}, 0, "readline(size=-1) -> bytes"))
	r.Set("tell", py.MustNewMethod("tell", func(o py.Object, args py.Tuple) (py.Object, error) {
		b := o.(*buffered)
		pos, err := rawCall(b.raw, "tell")
		if err != nil {
			return nil, err
		}
		p, err := py.IndexInt(pos)
		return py.Int(p - len(b.buf)), err
	}, 0, "tell() -> int"))
	r.Set("seek", py.MustNewMethod("seek", func(o py.Object, args py.Tuple) (py.Object, error) {
		b := o.(*buffered)
		if len(args) == 2 {
			if w, _ := py.IndexInt(args[1]); w == 1 { // relative: account for read-ahead
				off, err := py.IndexInt(args[0])
				if err != nil {
					return nil, err
				}
				args = py.Tuple{py.Int(off - len(b.buf)), py.Int(1)}
			}
		}
		b.buf = nil
		fn, err := py.GetAttrString(b.raw, "seek")
		if err != nil {
			return nil, err
		}
		return py.Call(fn, args, py.NewStringDict())
	}, 0, "seek(offset, whence=0) -> int"))
	r.Set("__iter__", py.MustNewMethod("__iter__", func(o py.Object) (py.Object, error) { return o, nil }, 0, ""))
	r.Set("__next__", py.MustNewMethod("__next__", func(o py.Object) (py.Object, error) {
		line, err := rawCallOn(o, "readline")
		if err == nil {
			if bs, _ := line.(py.Bytes); len(bs) == 0 {
				return nil, py.ExceptionNewf(py.StopIteration, "")
			}
		}
		return line, err
	}, 0, ""))
}

func rawCallOn(o py.Object, name string) (py.Object, error) { return rawCall(o, name) }
