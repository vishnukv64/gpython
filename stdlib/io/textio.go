// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package io

// textIOWrapperSrc is io.TextIOWrapper over any binary stream (BytesIO, BZ2File,
// GzipFile, a socket file...).  It decodes incrementally with the codecs
// module, translates newlines as the newline argument says, and defers
// everything else to the wrapped buffer.  Files opened with open() do not come
// through here.
const textIOWrapperSrc = `
class TextIOWrapper(TextIOBase):
    """Character and line based layer over a BufferedIOBase object, buffer."""
    def __init__(self, buffer, encoding=None, errors=None, newline=None, line_buffering=False, write_through=False):
        if newline not in (None, '', '\n', '\r', '\r\n'):
            raise ValueError("illegal newline value: %s" % (newline,))
        if encoding is None:
            encoding = 'utf-8'
        elif encoding == 'locale':
            encoding = 'utf-8'
        if errors is None:
            errors = 'strict'
        self._buffer = buffer
        self._encoding = encoding
        self._errors = errors
        self._newline = newline
        self._line_buffering = line_buffering
        self._write_through = write_through
        self._carry = b''
        ''.encode(encoding, errors)
        self._text = ''
        self._pending_cr = False
        self._eof = False
        self._detached = False

    @property
    def buffer(self): return self._buffer
    @property
    def encoding(self): return self._encoding
    @property
    def errors(self): return self._errors
    @property
    def line_buffering(self): return self._line_buffering
    @property
    def write_through(self): return self._write_through
    @property
    def newlines(self): return None
    @property
    def name(self): return self._buffer.name
    @property
    def closed(self): return self._buffer.closed

    def readable(self): return self._buffer.readable()
    def writable(self): return self._buffer.writable()
    def seekable(self): return self._buffer.seekable()
    def isatty(self): return self._buffer.isatty()
    def fileno(self): return self._buffer.fileno()
    def flush(self):
        self._check_closed()
        self._buffer.flush()
    def close(self):
        if self._buffer is not None and not self.closed:
            try:
                self.flush()
            finally:
                self._buffer.close()
    def detach(self):
        self.flush()
        b, self._buffer = self._buffer, None
        return b
    def _check_closed(self):
        if self._buffer is None or self._buffer.closed:
            raise ValueError("I/O operation on closed file.")

    def _decode(self, b, final=False):
        "Decode b, holding back a character cut off at the end of the chunk."
        data = self._carry + b
        self._carry = b''
        try:
            return data.decode(self._encoding, self._errors)
        except UnicodeDecodeError:
            if not final:
                for k in (1, 2, 3):
                    try:
                        out = data[:-k].decode(self._encoding, self._errors)
                    except UnicodeDecodeError:
                        continue
                    self._carry = data[-k:]
                    return out
            raise

    def _translate(self, s):
        if self._newline is None:
            if self._pending_cr:
                s = '\r' + s
                self._pending_cr = False
            if s.endswith('\r') and not self._eof:
                s = s[:-1]
                self._pending_cr = True
            s = s.replace('\r\n', '\n').replace('\r', '\n')
        return s

    def _fill(self):
        "Read one more chunk into the text buffer; False at end of input."
        if self._eof:
            return False
        chunk = self._buffer.read1(8192) if hasattr(self._buffer, 'read1') else self._buffer.read(8192)
        if chunk:
            self._text += self._translate(self._decode(chunk))
            return True
        self._eof = True
        self._text += self._translate(self._decode(b'', True))
        if self._pending_cr:
            self._pending_cr = False
            self._text += '\n'
        return False

    def read(self, size=-1):
        self._check_closed()
        if size is None:
            size = -1
        if size < 0:
            while self._fill():
                pass
            r, self._text = self._text, ''
            return r
        while len(self._text) < size and self._fill():
            pass
        r, self._text = self._text[:size], self._text[size:]
        return r

    def readline(self, size=-1):
        self._check_closed()
        if size is None:
            size = -1
        nl = '\n' if self._newline in (None, '\n') else self._newline
        start = 0
        while True:
            i = self._text.find(nl, start) if nl else -1
            if self._newline == '':
                j = self._text.find('\r', start)
                k = self._text.find('\n', start)
                cands = [x for x in (j, k) if x >= 0]
                if cands:
                    i = min(cands)
                    end = i + 1
                    if self._text[i] == '\r':
                        if end == len(self._text) and not self._eof:
                            if self._fill():
                                continue
                        if self._text[end:end + 1] == '\n':
                            end += 1
                    i = end - 1
                    nl = self._text[i:i + 1]
                    end = i + 1
                    if size >= 0 and end > size:
                        end = size
                    r, self._text = self._text[:end], self._text[end:]
                    return r
                i = -1
            if i >= 0:
                end = i + len(nl)
                if size >= 0 and end > size:
                    end = size
                r, self._text = self._text[:end], self._text[end:]
                return r
            if size >= 0 and len(self._text) >= size:
                r, self._text = self._text[:size], self._text[size:]
                return r
            start = max(0, len(self._text) - len(nl) + 1)
            if not self._fill():
                r, self._text = self._text, ''
                return r

    def readlines(self, hint=-1):
        out = []
        total = 0
        for line in self:
            out.append(line)
            total += len(line)
            if hint is not None and 0 < hint <= total:
                break
        return out
    def __iter__(self): return self
    def __next__(self):
        line = self.readline()
        if not line:
            raise StopIteration
        return line

    def write(self, s):
        self._check_closed()
        if not isinstance(s, str):
            raise TypeError("write() argument must be str, not %s" % type(s).__name__)
        n = len(s)
        if self._newline is None:
            import os
            if os.linesep != '\n':
                s = s.replace('\n', os.linesep)
        elif self._newline not in ('', '\n'):
            s = s.replace('\n', self._newline)
        self._buffer.write(s.encode(self._encoding, self._errors))
        if self._line_buffering and ('\n' in s or '\r' in s):
            self.flush()
        return n
    def writelines(self, lines):
        for l in lines:
            self.write(l)
    def truncate(self, pos=None):
        self.flush()
        return self._buffer.truncate(pos)
    def seek(self, offset, whence=0):
        self._check_closed()
        self.flush()
        self._text = ''
        self._eof = False
        self._pending_cr = False
        self._carry = b''
        return self._buffer.seek(offset, whence)
    def tell(self):
        self._check_closed()
        self.flush()
        return self._buffer.tell() - len(self._text.encode(self._encoding))
    def __repr__(self):
        return "<_io.TextIOWrapper encoding=%r>" % (self._encoding,)
`
