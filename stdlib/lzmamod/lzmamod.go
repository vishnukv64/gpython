// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package lzmamod is the _lzma extension module under CPython's lzma.py.
//
// It is built on github.com/ulikunitz/xz, so it supports FORMAT_XZ and
// FORMAT_ALONE (and FORMAT_AUTO between them), with CHECK_NONE, CHECK_CRC32,
// CHECK_CRC64 and CHECK_SHA256.  FORMAT_RAW, the delta and branch (BCJ)
// filters, and filter chains other than a single LZMA2 (xz) or LZMA1 (alone)
// filter raise LZMAError.  Compression presets and filter options are
// accepted but do not tune the encoder; the output is valid but is not
// byte-identical to liblzma's.
package lzmamod

import (
	"bytes"
	"errors"
	"io"

	"github.com/ulikunitz/xz"
	"github.com/ulikunitz/xz/lzma"
	"github.com/vishnukv64/gpython/py"
)

var xzMagic = []byte{0xfd, '7', 'z', 'X', 'Z', 0}

const (
	formatAuto  = 0
	formatXZ    = 1
	formatAlone = 2
)

// decode decodes the first stream in data.  It returns the output, how many
// bytes of data the stream used and whether the stream was complete.
// Incomplete input is not an error: the caller waits for more.  A non-empty
// msg is a corrupt-data error.
func decode(data []byte, format int) (out []byte, used int, complete bool, msg string) {
	if format == formatAuto {
		switch {
		case len(data) < len(xzMagic) && bytes.HasPrefix(xzMagic, data):
			return nil, 0, false, ""
		case bytes.HasPrefix(data, xzMagic):
			format = formatXZ
		default:
			format = formatAlone
		}
	}
	br := bytes.NewReader(data)
	var r io.Reader
	var err error
	if format == formatXZ {
		r, err = xz.ReaderConfig{SingleStream: true}.NewReader(br)
	} else {
		r, err = lzma.NewReader(br)
	}
	if err == nil {
		out, err = io.ReadAll(r)
	}
	used = len(data) - br.Len()
	switch {
	case err == nil:
		return out, used, format == formatAlone || footerAt(data, used), ""
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return out, 0, false, ""
	case err.Error() == "xz: unexpected data after stream":
		// the single-stream probe consumed the first trailing byte
		return out, used - 1, format == formatAlone || footerAt(data, used-1), ""
	}
	return nil, 0, false, err.Error()
}

// footerAt reports whether an xz stream footer ends at data[end]: the library
// takes a stream cut off at a block boundary for a finished one.
func footerAt(data []byte, end int) bool {
	return end >= 24 && end%4 == 0 && data[end-2] == 'Y' && data[end-1] == 'Z'
}

// encoder is an incremental compressor.
type encoder struct {
	buf bytes.Buffer
	w   io.WriteCloser
}

var encoderType = py.NewTypeX("_lzma.encoder", "Go compressor handle.", nil, nil)

func (*encoder) Type() *py.Type { return encoderType }

func (e *encoder) take() py.Object {
	b := bytes.Clone(e.buf.Bytes())
	e.buf.Reset()
	return py.Bytes(b)
}

func newEncoder(format, check int) (*encoder, error) {
	e := &encoder{}
	var err error
	if format == formatAlone {
		e.w, err = lzma.WriterConfig{EOSMarker: true}.NewWriter(&e.buf)
	} else {
		c := xz.WriterConfig{CheckSum: xz.CRC64}
		switch check {
		case -1, 4:
		case 0:
			c.NoCheckSum = true
		case 1:
			c.CheckSum = xz.CRC32
		case 10:
			c.CheckSum = xz.SHA256
		}
		e.w, err = c.NewWriter(&e.buf)
	}
	return e, err
}

const src = `
CHECK_NONE = 0
CHECK_CRC32 = 1
CHECK_CRC64 = 4
CHECK_SHA256 = 10
CHECK_ID_MAX = 15
CHECK_UNKNOWN = 16
FILTER_LZMA1 = 4611686018427387905
FILTER_LZMA2 = 33
FILTER_DELTA = 3
FILTER_X86 = 4
FILTER_IA64 = 6
FILTER_ARM = 7
FILTER_ARMTHUMB = 8
FILTER_POWERPC = 5
FILTER_SPARC = 9
FORMAT_AUTO = 0
FORMAT_XZ = 1
FORMAT_ALONE = 2
FORMAT_RAW = 3
MF_HC3 = 3
MF_HC4 = 4
MF_BT2 = 18
MF_BT3 = 19
MF_BT4 = 20
MODE_FAST = 1
MODE_NORMAL = 2
PRESET_DEFAULT = 6
PRESET_EXTREME = 2147483648

class LZMAError(Exception):
    "Call to liblzma failed."

def is_check_supported(check_id, /):
    "Test whether the given integrity check is supported."
    return check_id in (0, 1, 4, 10)

_NAMES = {3: 'delta', 4: 'x86', 5: 'powerpc', 6: 'ia64', 7: 'arm', 8: 'armthumb', 9: 'sparc'}

def _check_filters(filters, format):
    if not filters:
        raise ValueError("Must specify filters for FORMAT_RAW")
    want = FILTER_LZMA1 if format == FORMAT_ALONE else FILTER_LZMA2
    for f in filters:
        try:
            fid = f["id"]
        except (KeyError, TypeError):
            raise ValueError("Filter specifier must be a dict or dict-like object")
        if fid not in _NAMES and fid not in (FILTER_LZMA1, FILTER_LZMA2):
            raise LZMAError("Invalid or unsupported options")
    if len(filters) != 1 or filters[0]["id"] != want:
        raise LZMAError("Unsupported filter chain: gpython supports only a single %s filter for this format"
                        % ("LZMA1" if want == FILTER_LZMA1 else "LZMA2"))

def _check_preset(preset):
    if preset is not None and (preset & ~PRESET_EXTREME) > 9:
        raise LZMAError("Invalid or unsupported options")

class LZMACompressor:
    """Create a compressor object for compressing data incrementally.

    format specifies the container format to use for the output. This can
    be FORMAT_XZ (default) or FORMAT_ALONE; FORMAT_RAW is not supported.

    check specifies the integrity check to use (FORMAT_XZ only).
    preset is accepted but does not tune the encoder."""
    def __init__(self, format=FORMAT_XZ, check=-1, preset=None, filters=None):
        if format not in (FORMAT_XZ, FORMAT_ALONE, FORMAT_RAW):
            raise LZMAError("Invalid or unsupported options")
        if format == FORMAT_RAW:
            raise LZMAError("FORMAT_RAW is not supported by gpython")
        if check == -1 and format == FORMAT_XZ:
            check = CHECK_CRC64
        if check not in (-1, 0, 1, 4, 10) and not (0 <= check <= CHECK_ID_MAX):
            raise LZMAError("Invalid or unsupported options")
        if format != FORMAT_XZ and check not in (-1, CHECK_NONE):
            raise ValueError("Integrity checks are only supported by FORMAT_XZ")
        if preset is not None and filters is not None:
            raise ValueError("Cannot specify both preset and filter chain")
        if filters is not None:
            _check_filters(filters, format)
        _check_preset(preset)
        if check not in (-1, 0, 1, 4, 10):
            raise LZMAError("Unsupported integrity check")
        self._h = _enc_new(format, check)
        self._flushed = False

    def compress(self, data, /):
        "Provide data to the compressor object. Returns a chunk of compressed data if possible, or b'' otherwise."
        if self._flushed:
            raise ValueError("Compressor has been flushed")
        return _enc_write(self._h, bytes(data))

    def flush(self, /):
        "Finish the compression process. Returns the compressed data left in internal buffers."
        if self._flushed:
            raise ValueError("Repeated call to flush()")
        self._flushed = True
        return _enc_finish(self._h)

    def __reduce__(self):
        raise TypeError("cannot pickle '_lzma.LZMACompressor' object")

class LZMADecompressor:
    """Create a decompressor object for decompressing data incrementally.

    format specifies the container format of the input stream: FORMAT_AUTO
    (default), FORMAT_XZ or FORMAT_ALONE.  FORMAT_RAW is not supported.
    memlimit is accepted and ignored."""
    def __init__(self, format=FORMAT_AUTO, memlimit=None, filters=None):
        if format not in (FORMAT_AUTO, FORMAT_XZ, FORMAT_ALONE, FORMAT_RAW):
            raise ValueError("Invalid container format: %s" % format)
        if format == FORMAT_RAW:
            if memlimit is not None:
                raise ValueError("Cannot specify memory limit with FORMAT_RAW")
            if filters is None:
                raise ValueError("Must specify filters for FORMAT_RAW")
            raise LZMAError("FORMAT_RAW is not supported by gpython")
        if filters is not None:
            raise ValueError("Cannot specify filters except with FORMAT_RAW")
        self._format = format
        self._buf = b''
        self._given = 0
        self._eof = False
        self._unused = b''
        self._pending = b''
        self._check = CHECK_UNKNOWN

    def _set_check(self):
        b = self._buf
        if b[:6] == b'\xfd7zXZ\x00' and self._format != FORMAT_ALONE:
            if len(b) >= 8:
                self._check = b[7] & 15
        elif self._format == FORMAT_ALONE or len(b) >= 6 or b[:len(b)] != b'\xfd7zXZ\x00'[:len(b)]:
            self._check = CHECK_NONE

    @property
    def check(self):
        "ID of the integrity check used by the input stream."
        return self._check

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
            raise EOFError("Already at end of stream")
        data = bytes(data)
        if self._eof:
            self._unused += data
        elif data:
            # ponytail: re-decodes the buffered stream on every call, so a
            # stream fed in n chunks costs O(n^2); a goroutine+pipe fixes it.
            self._buf += data
            self._set_check()
            out, used, complete, err = _decode(self._buf, self._format)
            if err:
                raise LZMAError(err)
            self._pending += out[self._given:]
            if complete:
                self._eof = True
                self._unused = self._buf[used:]
                self._buf = b''
                self._given = 0
            else:
                self._given = len(out)
        if max_length is None or max_length < 0:
            res, self._pending = self._pending, b''
        else:
            res, self._pending = self._pending[:max_length], self._pending[max_length:]
        return res

    def __reduce__(self):
        raise TypeError("cannot pickle '_lzma.LZMADecompressor' object")

def _encode_filter_properties(filter):
    fid = filter["id"]
    if fid == FILTER_LZMA2:
        size = filter.get("dict_size", 1 << 23)
        for b in range(41):
            if ((2 | (b & 1)) << (b // 2 + 11)) >= size or b == 40:
                return bytes([b])
    if fid == FILTER_LZMA1:
        lc, lp, pb = filter.get("lc", 3), filter.get("lp", 0), filter.get("pb", 2)
        return bytes([lc + lp * 9 + pb * 45]) + (filter.get("dict_size", 1 << 23)).to_bytes(4, 'little')
    raise LZMAError("Invalid or unsupported options")

def _decode_filter_properties(filter_id, encoded_props, /):
    if filter_id == FILTER_LZMA2 and len(encoded_props) == 1 and encoded_props[0] <= 40:
        b = encoded_props[0]
        size = 0xFFFFFFFF if b == 40 else (2 | (b & 1)) << (b // 2 + 11)
        return {"id": filter_id, "dict_size": size}
    if filter_id == FILTER_LZMA1 and len(encoded_props) == 5 and encoded_props[0] < 225:
        p = encoded_props[0]
        return {"id": filter_id, "lc": p % 9, "lp": p // 9 % 5, "pb": p // 45,
                "dict_size": int.from_bytes(encoded_props[1:], 'little')}
    raise LZMAError("Invalid or unsupported options")
`

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_lzma", Doc: "Implementation module for lzma (xz and lzma-alone)."},
		Methods: []*py.Method{
			py.MustNewMethod("_decode", func(self py.Object, args py.Tuple) (py.Object, error) {
				out, used, complete, msg := decode([]byte(args[0].(py.Bytes)), int(args[1].(py.Int)))
				return py.Tuple{py.Bytes(out), py.Int(used), py.Bool(complete), py.String(msg)}, nil
			}, 0, ""),
			py.MustNewMethod("_enc_new", func(self py.Object, args py.Tuple) (py.Object, error) {
				e, err := newEncoder(int(args[0].(py.Int)), int(args[1].(py.Int)))
				if err != nil {
					return nil, py.ExceptionNewf(py.ValueError, "%v", err)
				}
				return e, nil
			}, 0, ""),
			py.MustNewMethod("_enc_write", func(self py.Object, args py.Tuple) (py.Object, error) {
				e := args[0].(*encoder)
				if _, err := e.w.Write([]byte(args[1].(py.Bytes))); err != nil {
					return nil, py.ExceptionNewf(py.ValueError, "%v", err)
				}
				return e.take(), nil
			}, 0, ""),
			py.MustNewMethod("_enc_finish", func(self py.Object, args py.Tuple) (py.Object, error) {
				e := args[0].(*encoder)
				if err := e.w.Close(); err != nil {
					return nil, py.ExceptionNewf(py.ValueError, "%v", err)
				}
				return e.take(), nil
			}, 0, ""),
		},
		CodeSrc: src,
	})
}
