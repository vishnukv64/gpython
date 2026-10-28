// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bz2mod is the _bz2 extension module under CPython's bz2.py.
//
// Only decompression exists: Go's standard library has a bzip2 decoder and no
// encoder, so BZ2Compressor raises NotImplementedError.  bz2.decompress, the
// BZ2File read mode and BZ2Decompressor work; bz2.compress and writing do not.
package bz2mod

import (
	"bytes"
	"compress/bzip2"
	"errors"
	"io"

	"github.com/vishnukv64/gpython/py"
)

// decode decodes the first bzip2 stream in data.  It returns the output, how
// many bytes of data the stream used and whether the stream was complete.
// Incomplete input is not an error here: the caller waits for more.
func decode(data []byte) (out []byte, used int, complete bool, err error) {
	r := bytes.NewReader(data)
	out, err = io.ReadAll(bzip2.NewReader(r))
	switch {
	case err == nil:
		return out, len(data), true, nil
	case errors.Is(err, io.ErrUnexpectedEOF):
		return out, 0, false, nil
	}
	// Bytes after the stream: Go reads on into a next stream and fails on them.
	// Find where this stream ended (the 48-bit end-of-stream magic, then a
	// 32-bit CRC, padded to a byte) and decode just that much.
	if end := streamEnd(data); end > 0 && end < len(data) {
		if out, e := io.ReadAll(bzip2.NewReader(bytes.NewReader(data[:end]))); e == nil {
			return out, end, true, nil
		}
	}
	return out, 0, false, err
}

// streamEnd tries each bit offset where the end-of-stream magic occurs and
// returns the byte length of the stream that ends there, or 0.
func streamEnd(data []byte) int {
	const magic = 0x177245385090
	var window uint64
	for i := 0; i < len(data)*8; i++ {
		window = (window<<1 | uint64(data[i/8]>>(7-uint(i%8))&1)) & (1<<48 - 1)
		if i >= 47 && window == magic {
			end := (i + 1 + 32 + 7) / 8
			if end <= len(data) {
				if _, e := io.ReadAll(bzip2.NewReader(bytes.NewReader(data[:end]))); e == nil {
					return end
				}
			}
		}
	}
	return 0
}

const src = `
class BZ2Compressor:
    "Create a compressor object for compressing data incrementally."
    def __init__(self, compresslevel=9, /):
        raise NotImplementedError("bz2 compression is not available: gpython has only a bzip2 decoder")
    def compress(self, data, /):
        raise NotImplementedError("bz2 compression is not available")
    def flush(self, /):
        raise NotImplementedError("bz2 compression is not available")

class BZ2Decompressor:
    """Create a decompressor object for decompressing data incrementally.

    For one-shot decompression, use the decompress() function instead."""
    def __init__(self, /):
        self._buf = b''
        self._given = 0
        self._eof = False
        self._unused = b''
        self._pending = b''

    @property
    def eof(self):
        "True if the end-of-stream marker has been reached."
        return self._eof and not self._pending

    @property
    def unused_data(self):
        "Data found after the end of the compressed stream."
        return self._unused

    @property
    def needs_input(self):
        "True if more input is needed before more decompressed data can be produced."
        return not self._eof and not self._pending

    def decompress(self, data, max_length=-1):
        """Decompress *data*, returning uncompressed data as bytes.

        If *max_length* is nonnegative, returns at most *max_length* bytes of
        decompressed data."""
        if self.eof:
            raise EOFError("End of stream already reached")
        data = bytes(data)
        if data and not self._eof:
            self._buf += data
            out, used, complete = _decode(self._buf)
            if complete:
                self._eof = True
                self._unused = self._buf[used:]
                self._buf = b''
            self._pending += out[self._given:]
            self._given = 0 if complete else len(out)
            if complete:
                self._given = 0
        if max_length is None or max_length < 0:
            res, self._pending = self._pending, b''
        else:
            res, self._pending = self._pending[:max_length], self._pending[max_length:]
        return res

    def __reduce__(self):
        raise TypeError("cannot pickle '_bz2.BZ2Decompressor' object")
`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_bz2", Doc: "Implementation module for bz2 (decompression only)."},
		Methods: []*py.Method{
			py.MustNewMethod("_decode", func(self py.Object, args py.Tuple) (py.Object, error) {
				out, used, complete, err := decode([]byte(args[0].(py.Bytes)))
				if err != nil {
					return nil, py.ExceptionNewf(py.OSError, "Invalid data stream")
				}
				return py.Tuple{py.Bytes(out), py.Int(used), py.Bool(complete)}, nil
			}, 0, ""),
		},
		CodeSrc: src,
	})
}
