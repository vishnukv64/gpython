package zlib

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"

	"github.com/vishnukv64/gpython/py"
)

// newWriter picks the container from wbits as zlib does: negative is a raw
// DEFLATE stream, 25..31 (16+) is gzip, otherwise the zlib wrapper.
func newWriter(w io.Writer, wbits, level int) (io.WriteCloser, error) {
	switch {
	case wbits < 0:
		return flate.NewWriter(w, level)
	case wbits >= 16:
		return gzip.NewWriterLevel(w, level)
	}
	return zlib.NewWriterLevel(w, level)
}

// newReader is the inverse of newWriter; 32+ auto-detects gzip or zlib.
func newReader(r *bytes.Reader, wbits int) (io.Reader, error) {
	if wbits >= 32 {
		if h, err := r.ReadByte(); err == nil {
			_ = r.UnreadByte()
			if h == 0x1f {
				wbits = 16
			} else {
				wbits = 15
			}
		}
	}
	switch {
	case wbits < 0:
		return flate.NewReader(r), nil
	case wbits >= 16:
		g, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		g.Multistream(false)
		return g, nil
	}
	return zlib.NewReader(r)
}

// decompressor backs both zlib.decompressobj and zlib._ZlibDecompressor.
//
// ponytail: Go's inflaters cannot resume after running out of input, so each
// feed re-inflates everything fed so far and keeps only the new tail. That is
// O(n^2) in the number of feeds; swap for a goroutine-fed pipe if large
// streams matter.
type decompressor struct {
	wbits    int
	in       []byte
	produced int
	pending  []byte
	eof      bool
	unused   []byte
	raw      bool // _ZlibDecompressor: max_length defaults to -1, EOFError after eof
	// tail is unconsumed_tail: output held back by max_length is already
	// inflated here, but a caller loops "while unconsumed_tail", so a one-byte
	// token stands for the input zlib would not have consumed yet.  The token
	// is dropped from the front of the next decompress() data, so feeding the
	// tail back, as the zlib docs ask, continues the stream.
	tail []byte
	Dict py.StringDict
}

var (
	decompressorType = py.NewTypeX("zlib._ZlibDecompressor", "Create a decompressor object for decompressing data incrementally.",
		func(_ *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			wbits := py.Object(py.Int(maxWBits))
			zdict := py.Object(py.None)
			if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:_ZlibDecompressor", []string{"wbits", "zdict"}, &wbits, &zdict); err != nil {
				return nil, err
			}
			w, err := py.IndexInt(wbits)
			if err != nil {
				return nil, err
			}
			return &decompressor{wbits: w, raw: true, Dict: py.NewStringDict()}, nil
		}, nil)
)

func (d *decompressor) Type() *py.Type {
	if d.raw {
		return decompressorType
	}
	return decompressObjType
}
func (d *decompressor) GetDict() py.StringDict { return d.Dict }

func (d *decompressor) feed(data []byte) error {
	if d.eof {
		d.unused = append(d.unused, data...)
		return nil
	}
	if len(data) == 0 && len(d.in) > 0 {
		return nil
	}
	d.in = append(d.in, data...)
	r := bytes.NewReader(d.in)
	dec, err := newReader(r, d.wbits)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil // header not complete yet
		}
		return py.ExceptionNewf(errorType, "Error -3 while decompressing data: %s", err)
	}
	out, err := io.ReadAll(dec)
	switch {
	case err == nil:
		d.eof = true
		d.unused = append([]byte(nil), d.in[len(d.in)-r.Len():]...)
		d.in = nil
	case errors.Is(err, io.ErrUnexpectedEOF):
	default:
		return py.ExceptionNewf(errorType, "Error -3 while decompressing data: %s", err)
	}
	d.pending = append(d.pending, out[d.produced:]...)
	d.produced = len(out)
	return nil
}

func (d *decompressor) take(max int) py.Bytes {
	n := len(d.pending)
	if max > 0 && max < n {
		n = max
	}
	out := py.Bytes(append([]byte(nil), d.pending[:n]...))
	d.pending = d.pending[n:]
	return out
}

func init() {
	for _, t := range []*py.Type{decompressObjType, decompressorType} {
		t.Dict.Set("decompress", py.MustNewMethod("decompress", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			d := self.(*decompressor)
			var data py.Object
			var maxLength py.Object = py.Int(0)
			if d.raw {
				maxLength = py.Int(-1)
			}
			if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:decompress", []string{"data", "max_length"}, &data, &maxLength); err != nil {
				return nil, err
			}
			b, err := py.BytesFromObject(data)
			if err != nil {
				return nil, err
			}
			max, err := py.IndexInt(maxLength)
			if err != nil {
				return nil, err
			}
			if d.raw && d.eof && len(d.pending) == 0 {
				return nil, py.ExceptionNewf(py.EOFError, "End of stream already reached")
			}
			if len(d.tail) > 0 && bytes.HasPrefix(b, d.tail) {
				b = b[len(d.tail):]
			}
			d.tail = nil
			if err := d.feed(b); err != nil {
				return nil, err
			}
			out := d.take(max)
			if !d.raw && len(d.pending) > 0 {
				d.tail = []byte{0}
			}
			return out, nil
		}, 0, "Return a bytes object containing the decompressed data."))
		t.Dict.Set("unused_data", &py.Property{Fget: func(self py.Object) (py.Object, error) {
			return py.Bytes(self.(*decompressor).unused), nil
		}})
		t.Dict.Set("eof", &py.Property{Fget: func(self py.Object) (py.Object, error) {
			d := self.(*decompressor)
			return py.NewBool(d.eof && len(d.pending) == 0), nil
		}})
	}
	decompressorType.Dict.Set("needs_input", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		d := self.(*decompressor)
		return py.NewBool(!d.eof && len(d.pending) == 0), nil
	}})
	decompressObjType.Dict.Set("unconsumed_tail", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Bytes(self.(*decompressor).tail), nil
	}})
}
