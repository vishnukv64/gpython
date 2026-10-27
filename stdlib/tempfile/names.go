// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tempfile

import (
	"crypto/rand"

	"github.com/vishnukv64/gpython/py"
)

const nameChars = "abcdefghijklmnopqrstuvwxyz0123456789_"

// RandomNameSequence is tempfile._RandomNameSequence: an iterator of 8-char
// random names, which multiprocessing uses to name its semaphores.
type RandomNameSequence struct{}

var RandomNameSequenceType = py.NewTypeX("tempfile._RandomNameSequence", "An instance of _RandomNameSequence generates an endless sequence of unpredictable strings.", func(*py.Type, py.Tuple, py.StringDict) (py.Object, error) {
	return &RandomNameSequence{}, nil
}, nil)

func (*RandomNameSequence) Type() *py.Type { return RandomNameSequenceType }

func (r *RandomNameSequence) M__iter__() (py.Object, error) { return r, nil }

func (*RandomNameSequence) M__next__() (py.Object, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, py.OSErrorFrom(err, "")
	}
	for i, c := range b {
		b[i] = nameChars[int(c)%len(nameChars)]
	}
	return py.String(b[:]), nil
}
