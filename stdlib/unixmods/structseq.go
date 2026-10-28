// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || darwin

package unixmods

// structSeqSrc defines _structseq, the Python-level stand-in for CPython's
// PyStructSequence: a tuple subclass with named fields, CPython's repr, a
// constructor that takes one sequence, and pickling by value.
const structSeqSrc = `
import operator as _operator
def _structseq(name, mod, fields, doc=None):
    n = len(fields)
    def __new__(cls, seq):
        seq = tuple(seq)
        if len(seq) != n:
            raise TypeError("%s.%s() takes a %d-sequence (%d-sequence given)" % (mod, name, n, len(seq)))
        return tuple.__new__(cls, seq)
    def __repr__(self):
        return "%s.%s(%s)" % (mod, name, ", ".join("%s=%r" % (f, v) for f, v in zip(fields, self)))
    def __reduce__(self):
        return (type(self), (tuple(self),))
    d = {'__new__': __new__, '__repr__': __repr__, '__reduce__': __reduce__, '__slots__': (),
         '__module__': mod, '__match_args__': tuple(fields), '__doc__': doc,
         'n_fields': n, 'n_sequence_fields': n, 'n_unnamed_fields': 0}
    for i, f in enumerate(fields):
        d[f] = property(_operator.itemgetter(i))
    return type(name, (tuple,), d)
`
