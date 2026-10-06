//go:build !windows

// stdlib/mmap: memory-mapped files.
//
// The mapping itself is syscall.Mmap, which exists only on Unix; on Windows the
// package registers nothing, so "import mmap" is a ModuleNotFoundError there
// rather than a broken build.  There is no portable mremap, so resize()
// refuses, and every method on a closed object raises ValueError "mmap closed
// or invalid" - both measured against CPython 3.14 on darwin.
package mmap

import (
	"bytes"
	"fmt"
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

const (
	ACCESS_READ    = 1
	ACCESS_WRITE   = 2
	ACCESS_COPY    = 3
	ACCESS_DEFAULT = 0
)

const moduleDoc = `An interface to memory-mapped files.`

// Mmap is a memory-mapped file object.  data is the mapped memory itself -
// shared with any memoryview made from this object.  close() marks the
// Python object invalid; the actual munmap is deferred to the last view's
// finalizer (wired by py.NewMemoryView through MemoryViewCloseFunc), because
// CPython raises BufferError from close() while a buffer is exported, which
// callers that expect close() to succeed (pip's cachecontrol) would trip on.
type Mmap struct {
	data     []byte // the mapped pages
	size     int64  // size of the mapped object; file size, or length for anonymous
	pos      int64  // current file position
	access   int
	readonly bool
	closed   bool
	// exported is the number of memoryview exports holding data.  While it is
	// > 0, close() must NOT munmap: the view owns the pages and its finalizer
	// (see py.MemoryView.DelayedClose) fires the unmap when the last one goes.
	exported int
}

var MmapType = py.NewTypeX("mmap.mmap", moduleDoc, mmapNew, nil)

func (m *Mmap) Type() *py.Type { return MmapType }

var (
	_ py.I__len__            = (*Mmap)(nil)
	_ py.I__getitem__        = (*Mmap)(nil)
	_ py.I__setitem__        = (*Mmap)(nil)
	_ py.I__iter__           = (*Mmap)(nil)
	_ py.I__contains__       = (*Mmap)(nil)
	_ py.I__eq__             = (*Mmap)(nil)
	_ py.I__ne__             = (*Mmap)(nil)
	_ py.I__enter__          = (*Mmap)(nil)
	_ py.I__exit__           = (*Mmap)(nil)
	_ py.I__repr__           = (*Mmap)(nil)
	_ py.MemoryViewBytesLike = (*Mmap)(nil)
)

// errnoError renders a syscall failure as CPython does: "[Errno 9] Bad file
// descriptor".  Go's errno text is the same strerror text with its first
// letter lowercased, so capitalising it restores CPython's wording.
func errnoError(err error) error {
	en, ok := err.(syscall.Errno)
	if !ok {
		return py.ExceptionNewf(py.OSError, "%s", err.Error())
	}
	msg := en.Error()
	if msg != "" && msg[0] >= 'a' && msg[0] <= 'z' {
		msg = string(msg[0]-'a'+'A') + msg[1:]
	}
	return py.ExceptionNewf(py.OSError, "[Errno %d] %s", int(en), msg)
}

// errClosed is the error every method raises once the map is closed.
func errClosed() error {
	return py.ExceptionNewf(py.ValueError, "mmap closed or invalid")
}

func (m *Mmap) checkOpen() error {
	if m.closed {
		return errClosed()
	}
	return nil
}

// --- the py.MemoryViewBytesLike interface: how memoryview(obj) sees us ---

func (m *Mmap) MemoryViewBytes() []byte  { return m.data }
func (m *Mmap) MemoryViewReadOnly() bool { return m.readonly }
func (m *Mmap) MemoryViewCloseFunc() func() error {
	// Taking the closeFunc IS the export: a view that holds it has the pages,
	// so the export is counted now and released by the view's unmap call.
	m.exported++
	return func() error {
		m.exported--
		if m.exported == 0 && m.closed {
			return m.unmap()
		}
		return nil
	}
}

func (m *Mmap) unmap() error {
	if m.data == nil {
		return nil
	}
	d, err := m.data, error(nil)
	m.data = nil
	err = syscall.Munmap(d)
	return err
}

// mmap.mmap(fileno, length, flags=MAP_SHARED, prot=PROT_READ|PROT_WRITE,
//
//	access=ACCESS_DEFAULT, offset=0)
func mmapNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var filenoObj, lengthObj py.Object
	var flagsObj py.Object = py.Int(syscall.MAP_SHARED)
	var protObj py.Object = py.Int(syscall.PROT_READ | syscall.PROT_WRITE)
	var accessObj py.Object = py.Int(ACCESS_DEFAULT)
	var offsetObj py.Object = py.Int(0)
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|OOOO:mmap.mmap",
		[]string{"fileno", "length", "flags", "prot", "access", "offset"},
		&filenoObj, &lengthObj, &flagsObj, &protObj, &accessObj, &offsetObj); err != nil {
		return nil, err
	}
	fileno, err := py.IndexInt(filenoObj)
	if err != nil {
		return nil, err
	}
	length, err := py.IndexInt(lengthObj)
	if err != nil {
		return nil, err
	}
	access, err := py.IndexInt(accessObj)
	if err != nil {
		return nil, err
	}
	offset, err := py.MakeGoInt64(offsetObj)
	if err != nil {
		return nil, err
	}

	// access and (flags, prot) are two spellings of the same request; CPython
	// on POSIX lets either drive.  flags/prot are accepted but access wins,
	// because that is the spelling pip's cachecontrol uses.
	var prot, mapFlags int
	readonly := false
	switch access {
	case ACCESS_DEFAULT, ACCESS_WRITE:
		prot = syscall.PROT_READ | syscall.PROT_WRITE
		mapFlags = syscall.MAP_SHARED
	case ACCESS_READ:
		prot = syscall.PROT_READ
		mapFlags = syscall.MAP_SHARED
		readonly = true
	case ACCESS_COPY:
		prot = syscall.PROT_READ | syscall.PROT_WRITE
		mapFlags = syscall.MAP_PRIVATE
	default:
		return nil, py.ExceptionNewf(py.ValueError, "mmap invalid access parameter.")
	}

	// Only -1 means anonymous, as in CPython; any other negative descriptor is
	// left to Fstat to reject.
	anonymous := fileno == -1
	if anonymous {
		mapFlags |= syscall.MAP_ANON
	}

	if length < 0 {
		return nil, py.ExceptionNewf(py.OverflowError, "memory mapped length must be positive")
	}

	var size int64
	if anonymous {
		size = int64(length)
		if size == 0 {
			return nil, py.ExceptionNewf(py.OSError, "[Errno 22] Invalid argument")
		}
	} else {
		var st syscall.Stat_t
		if err := syscall.Fstat(fileno, &st); err != nil {
			return nil, errnoError(err)
		}
		if length == 0 {
			if st.Size == 0 || offset >= st.Size {
				return nil, py.ExceptionNewf(py.ValueError, "cannot mmap an empty file")
			}
			length = int(st.Size - offset)
		}
		size = int64(length)
		if offset+size > st.Size {
			size = st.Size - offset
		}
		if size <= 0 {
			size = int64(length)
		}
	}

	var data []byte
	if anonymous {
		data, err = syscall.Mmap(-1, 0, int(size), prot, mapFlags)
	} else {
		data, err = syscall.Mmap(fileno, offset, int(size), prot, mapFlags)
	}
	if err != nil {
		return nil, py.ExceptionNewf(py.OSError, "[Errno %d] %s", err.(syscall.Errno), err.Error())
	}
	if anonymous {
		size = int64(length)
	} else {
		var st syscall.Stat_t
		_ = syscall.Fstat(fileno, &st)
		if length == 0 {
			size = st.Size - offset
		}
	}
	_ = flagsObj
	_ = protObj
	return &Mmap{data: data, size: size, access: access, readonly: readonly}, nil
}

// readBytes advances the position; n<0 means "the rest".
func (m *Mmap) readBytes(n int64) []byte {
	if m.pos >= m.size {
		return []byte{}
	}
	if n < 0 || m.pos+n > m.size {
		n = m.size - m.pos
	}
	out := make([]byte, n)
	copy(out, m.data[m.pos:m.pos+n])
	m.pos += n
	return out
}

func init() {
	globals := py.NewStringDict()

	globals.Set("mmap", MmapType)

	methods := []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"read", func(self py.Object, args py.Tuple) (py.Object, error) {
			m := self.(*Mmap)
			if err := m.checkOpen(); err != nil {
				return nil, err
			}
			size := py.Object(py.Int(-1))
			if err := py.UnpackTuple(args, py.StringDict{}, "read", 0, 1, &size); err != nil {
				return nil, err
			}
			n, err := py.MakeGoInt64(size)
			if err != nil {
				return nil, err
			}
			return py.Bytes(m.readBytes(n)), nil
		}, "read([n]) -> bytes, advancing the position."},
		{"read_byte", func(self py.Object, args py.Tuple) (py.Object, error) {
			m := self.(*Mmap)
			if err := m.checkOpen(); err != nil {
				return nil, err
			}
			if m.pos >= m.size {
				return nil, py.ExceptionNewf(py.ValueError, "read byte out of range")
			}
			b := m.data[m.pos]
			m.pos++
			return py.Int(b), nil
		}, "read_byte() -> the byte at the position, as an int."},
		{"readline", func(self py.Object, args py.Tuple) (py.Object, error) {
			m := self.(*Mmap)
			if err := m.checkOpen(); err != nil {
				return nil, err
			}
			if m.pos >= m.size {
				return py.Bytes(nil), nil
			}
			idx := bytes.IndexByte(m.data[m.pos:int(m.size)], '\n')
			if idx < 0 {
				return py.Bytes(m.readBytes(-1)), nil
			}
			return py.Bytes(m.readBytes(int64(idx) + 1)), nil
		}, "readline() -> one line, newline included, position past it."},
		{"seek", func(self py.Object, args py.Tuple) (py.Object, error) {
			m := self.(*Mmap)
			if err := m.checkOpen(); err != nil {
				return nil, err
			}
			var posObj py.Object
			var whenceObj py.Object = py.Int(0)
			if err := py.UnpackTuple(args, py.StringDict{}, "seek", 1, 2, &posObj, &whenceObj); err != nil {
				return nil, err
			}
			pos, err := py.MakeGoInt64(posObj)
			if err != nil {
				return nil, err
			}
			whence, err := py.IndexInt(whenceObj)
			if err != nil {
				return nil, err
			}
			base := int64(0)
			switch whence {
			case 0:
				base = 0
			case 1:
				base = m.pos
			case 2:
				base = m.size
			default:
				return nil, py.ExceptionNewf(py.ValueError, "unknown seek type")
			}
			npos := base + pos
			if npos < 0 || npos > m.size {
				return nil, py.ExceptionNewf(py.ValueError, "seek out of range")
			}
			m.pos = npos
			return py.None, nil
		}, "seek(pos[, whence]) -> set the position."},
		{"tell", func(self py.Object, args py.Tuple) (py.Object, error) {
			m := self.(*Mmap)
			if err := m.checkOpen(); err != nil {
				return nil, err
			}
			return py.Int(m.pos), nil
		}, "tell() -> the current position."},
		{"size", func(self py.Object, args py.Tuple) (py.Object, error) {
			m := self.(*Mmap)
			if err := m.checkOpen(); err != nil {
				return nil, err
			}
			return py.Int(m.size), nil
		}, "size() -> the length of the underlying file."},
		// flush(): the write-through case (MAP_SHARED) is always coherent, and
		// there is no msync here, so it exists only as a successful no-op.
		{"flush", func(self py.Object, args py.Tuple) (py.Object, error) {
			m := self.(*Mmap)
			if err := m.checkOpen(); err != nil {
				return nil, err
			}
			return py.None, nil
		}, "flush() -> no-op: MAP_SHARED changes go straight through."},
		{"close", func(self py.Object, args py.Tuple) (py.Object, error) {
			m := self.(*Mmap)
			if m.closed {
				return py.None, nil
			}
			m.closed = true
			// An exported view keeps the pages: munmap fires from its finalizer.
			if m.exported == 0 {
				if err := m.unmap(); err != nil {
					return nil, py.ExceptionNewf(py.OSError, "%s", err.Error())
				}
			}
			return py.None, nil
		}, "close() -> close the mmap.  Further access raises ValueError."},
		// resize(): no mremap on darwin, and CPython raises SystemError for the
		// same reason; refusing is the honest answer.
		{"resize", func(self py.Object, args py.Tuple) (py.Object, error) {
			m := self.(*Mmap)
			if err := m.checkOpen(); err != nil {
				return nil, err
			}
			// The readonly refusal fires BEFORE the platform one: CPython checks
			// writability first, then mremap availability (the golden captures
			// TypeError from a readonly map and SystemError from a writable one).
			if m.readonly || m.access == ACCESS_COPY {
				return nil, py.ExceptionNewf(py.TypeError, "mmap can't resize a readonly or copy-on-write memory map.")
			}
			// CPython spells this with a double dash in the message.
			return nil, py.ExceptionNewf(py.SystemError, "mmap: resizing not available--no mremap()")
		}, "resize(n) -> NOT SUPPORTED: no mremap() on this platform."},
		{"find", func(self py.Object, args py.Tuple) (py.Object, error) {
			return mFind(self, args, false)
		}, "find(sub[, start[, end]]) -> lowest index of sub, or -1."},
		{"rfind", func(self py.Object, args py.Tuple) (py.Object, error) {
			return mFind(self, args, true)
		}, "rfind(sub[, start[, end]]) -> highest index of sub, or -1."},
	}

	for _, md := range methods {
		MmapType.Dict.Set(md.name, py.MustNewMethod(md.name, md.fn, 0, md.doc))
	}

	// Constants come from the kernel via syscall, the way errno does, so they
	// can never drift from the platform they are used on.
	globals.Set("ACCESS_READ", py.Int(ACCESS_READ))
	globals.Set("ACCESS_WRITE", py.Int(ACCESS_WRITE))
	globals.Set("ACCESS_COPY", py.Int(ACCESS_COPY))
	globals.Set("ACCESS_DEFAULT", py.Int(ACCESS_DEFAULT))
	globals.Set("MAP_SHARED", py.Int(syscall.MAP_SHARED))
	globals.Set("MAP_PRIVATE", py.Int(syscall.MAP_PRIVATE))
	globals.Set("MAP_ANONYMOUS", py.Int(syscall.MAP_ANON))
	globals.Set("MAP_ANON", py.Int(syscall.MAP_ANON))
	globals.Set("PROT_READ", py.Int(syscall.PROT_READ))
	globals.Set("PROT_WRITE", py.Int(syscall.PROT_WRITE))
	globals.Set("PROT_EXEC", py.Int(syscall.PROT_EXEC))
	globals.Set("PAGESIZE", py.Int(syscall.Getpagesize()))
	globals.Set("ALLOCATIONGRANULARITY", py.Int(syscall.Getpagesize()))
	globals.Set("__doc__", py.String(moduleDoc))

	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "mmap", Doc: moduleDoc},
		Globals: globals,
	})
}

// mFind is find and rfind; they differ only in search direction.
func mFind(self py.Object, args py.Tuple, reverse bool) (py.Object, error) {
	m := self.(*Mmap)
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	var subObj py.Object
	var startObj py.Object = py.None
	var endObj py.Object = py.None
	if err := py.UnpackTuple(args, py.StringDict{}, "find", 1, 3, &subObj, &startObj, &endObj); err != nil {
		return nil, err
	}
	sub, err := subBytesOf("find", subObj)
	if err != nil {
		return nil, err
	}
	haystack := m.data[:m.size]
	lo, hi := 0, len(haystack)
	if startObj != py.None {
		lo, err = py.IndexInt(startObj)
		if err != nil {
			return nil, err
		}
		if lo < 0 {
			lo += len(haystack)
		}
	}
	if endObj != py.None {
		hi, err = py.IndexInt(endObj)
		if err != nil {
			return nil, err
		}
		if hi < 0 {
			hi += len(haystack)
		}
	}
	if lo < 0 {
		lo = 0
	}
	if hi > len(haystack) {
		hi = len(haystack)
	}
	if lo > hi {
		lo, hi = hi, lo
		if !reverse {
			// CPython returns -1 for an empty window in the forward direction.
			return py.Int(-1), nil
		}
	}
	window := haystack[lo:hi]
	var idx int
	if reverse {
		idx = bytes.LastIndex(window, sub)
	} else {
		idx = bytes.Index(window, sub)
	}
	if idx < 0 {
		return py.Int(-1), nil
	}
	return py.Int(lo + idx), nil
}

// subBytesOf turns the "what to find" argument into []byte, with the CPython
// message for a wrong type (the call is a buffer-protocol lookup).
func subBytesOf(name string, obj py.Object) ([]byte, error) {
	switch v := obj.(type) {
	case py.Bytes:
		return []byte(v), nil
	case *py.ByteArray:
		b, err := v.M__bytes__()
		if err != nil {
			return nil, err
		}
		return []byte(b.(py.Bytes)), nil
	}
	return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not '%s'", obj.Type().Name)
}

func (m *Mmap) M__len__() (py.Object, error) {
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	return py.Int(m.size), nil
}

// mmapIndex resolves one subscript with CPython's message shape.
func mmapIndex(m *Mmap, key py.Object) (int, error) {
	i, err := py.Index(key)
	if err != nil {
		return 0, py.ExceptionNewf(py.TypeError, "mmap indices must be integers or slices, not %s", key.Type().Name)
	}
	n := int(i)
	if i < 0 {
		n += int(m.size)
	}
	if n < 0 || n >= int(m.size) {
		return 0, py.ExceptionNewf(py.IndexError, "mmap index out of range")
	}
	return n, nil
}

func (m *Mmap) M__getitem__(key py.Object) (py.Object, error) {
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	if sl, ok := key.(*py.Slice); ok {
		start, _, step, slicelength, err := sl.GetIndices(int(m.size))
		if err != nil {
			return nil, err
		}
		out := make([]byte, slicelength)
		i := start
		for k := 0; k < slicelength; k++ {
			out[k] = m.data[i]
			i += step
		}
		// A slice of an mmap is BYTES (a copy), not an mmap - CPython spells
		// that out, and bytes are what a caller can hold past close().
		return py.Bytes(out), nil
	}
	n, err := mmapIndex(m, key)
	if err != nil {
		return nil, err
	}
	return py.Int(m.data[n]), nil
}

func (m *Mmap) M__setitem__(key, value py.Object) (py.Object, error) {
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	if m.readonly {
		return nil, py.ExceptionNewf(py.TypeError, "mmap can't modify a readonly memory map.")
	}
	if sl, ok := key.(*py.Slice); ok {
		var data []byte
		switch v := value.(type) {
		case py.Bytes:
			data = []byte(v)
		case *py.ByteArray:
			b, berr := v.M__bytes__()
			if berr != nil {
				return nil, berr
			}
			data = []byte(b.(py.Bytes))
		default:
			return nil, py.ExceptionNewf(py.TypeError, "a bytes-like object is required, not '%s'", value.Type().Name)
		}
		start, _, step, slicelength, err := sl.GetIndices(int(m.size))
		if err != nil {
			return nil, err
		}
		if len(data) != slicelength {
			return nil, py.ExceptionNewf(py.ValueError, "mmap slice assignment is wrong size")
		}
		i := start
		for k := 0; k < slicelength; k++ {
			m.data[i] = data[k]
			i += step
		}
		return py.None, nil
	}
	n, err := mmapIndex(m, key)
	if err != nil {
		return nil, err
	}
	v, err := py.MakeGoInt(value)
	if err != nil {
		return nil, py.ExceptionNewf(py.TypeError, "mmap item value must be an int")
	}
	if v < 0 || v > 255 {
		return nil, py.ExceptionNewf(py.ValueError, "mmap: byte assignment out of range")
	}
	m.data[n] = byte(v)
	return py.None, nil
}

// M__iter__ yields 1-byte BYTES items, not ints - the oracle run showed
// CPython producing [b'h', b'e', b'l'], and that is what the code matches.
type MmapIterator struct {
	m   *Mmap
	pos int
}

var MmapIteratorType = py.NewType("mmap_iterator", "Iterator over the bytes of a memory-mapped file.")

func (it *MmapIterator) Type() *py.Type { return MmapIteratorType }

func (m *Mmap) M__iter__() (py.Object, error) {
	return &MmapIterator{m: m}, nil
}

func (it *MmapIterator) M__iter__() (py.Object, error) { return it, nil }

func (it *MmapIterator) M__next__() (py.Object, error) {
	m := it.m
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	if int64(it.pos) >= m.size {
		return nil, py.ExceptionNewf(py.StopIteration, "")
	}
	b := m.data[it.pos]
	it.pos++
	return py.Bytes([]byte{b}), nil
}

var (
	_ py.I__iter__ = (*MmapIterator)(nil)
	_ py.I__next__ = (*MmapIterator)(nil)
)

// M__contains__ follows CPython, whose mmap defines no __contains__: "in"
// falls back to iteration, and iteration yields 1-byte bytes.  So only a
// 1-byte bytes equal to some byte is found - "104 in m" and a multi-byte
// needle like b"lo w" are both False, measured on CPython 3.14.
func (m *Mmap) M__contains__(item py.Object) (py.Object, error) {
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	b, ok := item.(py.Bytes)
	if !ok || len(b) != 1 {
		return py.False, nil
	}
	return py.NewBool(bytes.IndexByte(m.data[:m.size], b[0]) >= 0), nil
}

// mmapEquals compares against bytes, another mmap, or a memoryview - what the
// caller can reasonably hold an mmap up against.
func mmapEquals(a *Mmap, other py.Object) (bool, bool) {
	switch o := other.(type) {
	case *Mmap:
		return bytes.Equal(a.data[:a.size], o.data[:o.size]), true
	case py.Bytes:
		return bytes.Equal(a.data[:a.size], []byte(o)), true
	case *py.ByteArray:
		b, _ := o.M__bytes__()
		return bytes.Equal(a.data[:a.size], []byte(b.(py.Bytes))), true
	}
	return false, false
}

func (m *Mmap) M__eq__(other py.Object) (py.Object, error) {
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	if eq, ok := mmapEquals(m, other); ok {
		return py.NewBool(eq), nil
	}
	return py.NotImplemented, nil
}

func (m *Mmap) M__ne__(other py.Object) (py.Object, error) {
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	if eq, ok := mmapEquals(m, other); ok {
		return py.NewBool(!eq), nil
	}
	return py.NotImplemented, nil
}

func (m *Mmap) M__enter__() (py.Object, error) {
	if err := m.checkOpen(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Mmap) M__exit__(excType, excValue, tb py.Object) (py.Object, error) {
	if m.closed {
		return py.False, nil
	}
	m.closed = true
	if m.exported == 0 {
		_ = m.unmap()
	}
	return py.False, nil
}

func (m *Mmap) M__repr__() (py.Object, error) {
	return py.String(fmt.Sprintf("<mmap.mmap closed=%t, access=%s, length=%d, pos=%d, offset=0>", m.closed, accessName(m.access), m.size, m.pos)), nil
}

func accessName(a int) string {
	switch a {
	case ACCESS_READ:
		return "ACCESS_READ"
	case ACCESS_WRITE:
		return "ACCESS_WRITE"
	case ACCESS_COPY:
		return "ACCESS_COPY"
	}
	return "ACCESS_DEFAULT"
}
