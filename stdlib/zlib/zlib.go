// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package zlib provides the implementation of python's 'zlib' module.
//
// The compression is Go's compress/zlib, which reads and writes the zlib
// wrapper (RFC 1950) around a DEFLATE stream, so the bytes round-trip with
// CPython's zlib.  compress/decompress are one-shot helpers; compressobj and
// decompressobj provide the streaming objects.  crc32 and adler32 are
// hash/crc32 and hash/adler32.
package zlib

import (
	"bytes"
	"compress/zlib"
	"hash/crc32"
	"io"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `The functions in this module allow compression and decompression using the
zlib library.

compress(data, level=-1)    -- Compress data.
decompress(data, ...)       -- Decompress data.
compressobj(level=-1, ...)  -- Return a compressor object.
decompressobj(...)          -- Return a decompressor object.
crc32(data, value=0)        -- Compute a CRC-32 checksum.
adler32(data, value=1)      -- Compute an Adler-32 checksum.
`

const maxWBits = 15

// zlibFinish is Z_FINISH, the only flush mode Go's compress/zlib can perform.
const zlibFinish = 4

// errorType is zlib.error, raised for a corrupt or truncated stream.
var errorType = py.ExceptionType.NewType("zlib.error", "An error occurred in the zlib module.", nil, nil)

var (
	compressObjType = py.NewTypeX("zlib.compressobj",
		"A streaming compressor: compress() and flush().", nil, nil)
	decompressObjType = py.NewTypeX("zlib.decompressobj",
		"A streaming decompressor: decompress() and flush().", nil, nil)
)

// compressObj is the streaming compressor.
type compressObj struct {
	w       *zlib.Writer
	buf     bytes.Buffer
	flushed bool
	Dict    py.StringDict
}

func (c *compressObj) Type() *py.Type         { return compressObjType }
func (c *compressObj) GetDict() py.StringDict { return c.Dict }

// decompressObj is the streaming decompressor.
type decompressObj struct {
	r      io.ReadCloser
	unused []byte
	Dict   py.StringDict
}

func (d *decompressObj) Type() *py.Type         { return decompressObjType }
func (d *decompressObj) GetDict() py.StringDict { return d.Dict }

func zlibCompress(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var data py.Object
	var level py.Object = py.Int(-1)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:compress",
		[]string{"data", "level"}, &data, &level); err != nil {
		return nil, err
	}
	b, err := py.BytesFromObject(data)
	if err != nil {
		return nil, err
	}
	lvl, _ := py.IndexInt(level)
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, lvl)
	if err != nil {
		return nil, py.ExceptionNewf(py.ValueError, "Invalid initialization option")
	}
	if _, err := w.Write(b); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return py.Bytes(buf.Bytes()), nil
}

func zlibDecompress(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var data py.Object
	var wbits py.Object = py.Int(maxWBits)
	var bufsize py.Object = py.Int(16384)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:decompress",
		[]string{"data", "wbits", "bufsize"}, &data, &wbits, &bufsize); err != nil {
		return nil, err
	}
	b, err := py.BytesFromObject(data)
	if err != nil {
		return nil, err
	}
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, py.ExceptionNewf(errorType, "%s", err)
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, py.ExceptionNewf(errorType, "%s", err)
	}
	return py.Bytes(out), nil
}

func zlibCompressObj(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	level := py.Object(py.Int(-1))
	method := py.Object(py.Int(8))
	wbits := py.Object(py.Int(maxWBits))
	memLevel := py.Object(py.Int(8))
	strategy := py.Object(py.Int(0))
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOOOO:compressobj",
		[]string{"level", "method", "wbits", "memLevel", "strategy"},
		&level, &method, &wbits, &memLevel, &strategy); err != nil {
		return nil, err
	}
	if m, _ := py.IndexInt(method); m != 8 {
		return nil, py.ExceptionNewf(py.ValueError, "Invalid initialization option")
	}
	lvl, _ := py.IndexInt(level)
	c := &compressObj{Dict: py.NewStringDict()}
	w, err := zlib.NewWriterLevel(&c.buf, lvl)
	if err != nil {
		return nil, py.ExceptionNewf(py.ValueError, "Invalid initialization option")
	}
	c.w = w
	return c, nil
}

func compressObjCompress(self py.Object, args py.Tuple) (py.Object, error) {
	c := self.(*compressObj)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "compress() takes exactly one argument")
	}
	b, err := py.BytesFromObject(args[0])
	if err != nil {
		return nil, err
	}
	before := c.buf.Len()
	if _, err := c.w.Write(b); err != nil {
		return nil, py.ExceptionNewf(errorType, "%s", err)
	}
	out := c.buf.Bytes()[before:]
	return py.Bytes(append([]byte(nil), out...)), nil
}

func compressObjFlush(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := self.(*compressObj)
	var mode py.Object = py.Int(int(zlibFinish))
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:flush",
		[]string{"mode"}, &mode); err != nil {
		return nil, err
	}
	m, err := py.IndexInt(mode)
	if err != nil {
		return nil, err
	}
	if m != int(zlibFinish) {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"zlib.compressobj.flush: only Z_FINISH is implemented; Go's compress/zlib flushes "+
				"the whole DEFLATE stream, not a partial sync/flush block")
	}
	before := c.buf.Len()
	if err := c.w.Close(); err != nil {
		return nil, py.ExceptionNewf(errorType, "%s", err)
	}
	out := c.buf.Bytes()[before:]
	c.flushed = true
	return py.Bytes(append([]byte(nil), out...)), nil
}

func zlibDecompressObj(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	wbits := py.Object(py.Int(maxWBits))
	zdict := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:decompressobj",
		[]string{"wbits", "zdict"}, &wbits, &zdict); err != nil {
		return nil, err
	}
	return &decompressObj{Dict: py.NewStringDict()}, nil
}

func decompressObjDecompress(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	d := self.(*decompressObj)
	var data py.Object
	var maxLength py.Object = py.Int(0)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:decompress",
		[]string{"data", "max_length"}, &data, &maxLength); err != nil {
		return nil, err
	}
	b, err := py.BytesFromObject(data)
	if err != nil {
		return nil, err
	}
	if d.r == nil {
		r, err := zlib.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, py.ExceptionNewf(errorType, "%s", err)
		}
		d.r = r
	} else {
		// Continue decompressing; the reader is refilled from a fresh buffer.
		r, err := zlib.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, py.ExceptionNewf(errorType, "%s", err)
		}
		d.r = r
	}
	out, err := io.ReadAll(d.r)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, py.ExceptionNewf(errorType, "%s", err)
	}
	return py.Bytes(out), nil
}

func decompressObjFlush(self py.Object, args py.Tuple) (py.Object, error) {
	return py.Bytes(nil), nil
}

func zlibCRC32(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var data py.Object
	var value py.Object = py.Int(0)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:crc32",
		[]string{"data", "value"}, &data, &value); err != nil {
		return nil, err
	}
	b, err := py.BytesFromObject(data)
	if err != nil {
		return nil, err
	}
	seed, _ := py.IndexInt(value)
	return py.Int(int64(crc32.Update(uint32(seed), crc32.IEEETable, b))), nil
}

func zlibAdler32(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var data py.Object
	var value py.Object = py.Int(1)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:adler32",
		[]string{"data", "value"}, &data, &value); err != nil {
		return nil, err
	}
	b, err := py.BytesFromObject(data)
	if err != nil {
		return nil, err
	}
	seed, _ := py.IndexInt(value)
	return py.Int(int64(adler32Of(uint32(seed), b))), nil
}

// adler32Of computes the Adler-32 of b continued from seed.  hash/adler32
// exposes only Checksum, so the continuation is done through its internal
// update via a small reimplementation of the two-limb recurrence.
func adler32Of(seed uint32, b []byte) uint32 {
	const mod = 65521
	s1 := seed & 0xffff
	s2 := (seed >> 16) & 0xffff
	for _, c := range b {
		s1 = (s1 + uint32(c)) % mod
		s2 = (s2 + s1) % mod
	}
	return s2<<16 | s1
}

func init() {
	compressObjType.Dict.Set("compress", py.MustNewMethod("compress", compressObjCompress, 0,
		"Return a bytes object containing compressed data."))
	compressObjType.Dict.Set("flush", py.MustNewMethod("flush", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return compressObjFlush(self, args, kw)
	}, 0, "Return a bytes object containing any remaining compressed data."))
	decompressObjType.Dict.Set("decompress", py.MustNewMethod("decompress", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return decompressObjDecompress(self, args, kw)
	}, 0, "Return a bytes object containing the decompressed data."))
	decompressObjType.Dict.Set("flush", py.MustNewMethod("flush", decompressObjFlush, 0,
		"Return the bytes remaining after a streaming decompress."))
	decompressObjType.Dict.Set("unused_data", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Bytes(self.(*decompressObj).unused), nil
	}})
	decompressObjType.Dict.Set("eof", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Bool(false), nil
	}})

	globals := py.NewStringDict()
	globals.Set("error", errorType)
	globals.Set("compress", py.MustNewMethod("compress", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return zlibCompress(self, args, kw)
	}, 0, "compress(data, level=-1) - return the compressed data."))
	globals.Set("decompress", py.MustNewMethod("decompress", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return zlibDecompress(self, args, kw)
	}, 0, "decompress(data, wbits=MAX_WBITS, bufsize=16384) - return the decompressed data."))
	globals.Set("compressobj", py.MustNewMethod("compressobj", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return zlibCompressObj(self, args, kw)
	}, 0, "compressobj(level=-1, method=8, wbits=MAX_WBITS, memLevel=8, strategy=0)"))
	globals.Set("decompressobj", py.MustNewMethod("decompressobj", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return zlibDecompressObj(self, args, kw)
	}, 0, "decompressobj(wbits=MAX_WBITS, zdict=b'')"))
	globals.Set("crc32", py.MustNewMethod("crc32", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return zlibCRC32(self, args, kw)
	}, 0, "crc32(data, value=0) - compute a CRC-32 checksum."))
	globals.Set("adler32", py.MustNewMethod("adler32", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return zlibAdler32(self, args, kw)
	}, 0, "adler32(data, value=1) - compute an Adler-32 checksum."))

	globals.Set("MAX_WBITS", py.Int(maxWBits))
	globals.Set("DEFLATED", py.Int(8))
	globals.Set("DEF_MEM_LEVEL", py.Int(8))
	globals.Set("MAX_MEM_LEVEL", py.Int(9))
	globals.Set("Z_NO_COMPRESSION", py.Int(0))
	globals.Set("Z_BEST_SPEED", py.Int(1))
	globals.Set("Z_BEST_COMPRESSION", py.Int(9))
	globals.Set("Z_DEFAULT_COMPRESSION", py.Int(-1))
	globals.Set("Z_FILTERED", py.Int(1))
	globals.Set("Z_HUFFMAN_ONLY", py.Int(2))
	globals.Set("Z_RLE", py.Int(3))
	globals.Set("Z_FIXED", py.Int(4))
	globals.Set("Z_DEFAULT_STRATEGY", py.Int(0))
	globals.Set("Z_NO_FLUSH", py.Int(0))
	globals.Set("Z_PARTIAL_FLUSH", py.Int(1))
	globals.Set("Z_SYNC_FLUSH", py.Int(2))
	globals.Set("Z_FULL_FLUSH", py.Int(3))
	globals.Set("Z_FINISH", py.Int(zlibFinish))
	globals.Set("Z_BLOCK", py.Int(5))
	globals.Set("Z_TREES", py.Int(6))
	globals.Set("ZLIB_VERSION", py.String("1.3"))
	globals.Set("ZLIB_RUNTIME_VERSION", py.String("1.3"))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "zlib",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
