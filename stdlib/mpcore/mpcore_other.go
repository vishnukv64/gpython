//go:build windows || plan9 || js || wasip1

// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mpcore

import "github.com/vishnukv64/gpython/py"

func init() {
	// multiprocessing needs POSIX descriptors and flock; here constructing a
	// SemLock says so plainly.
	t := py.NewTypeX("_multiprocessing.SemLock", "Semaphore/Mutex type", func(*py.Type, py.Tuple, py.StringDict) (py.Object, error) {
		return nil, py.ExceptionNewf(py.NotImplementedError, "multiprocessing is not supported on this platform")
	}, nil)
	t.Dict.Set("SEM_VALUE_MAX", py.Int(32767))
	register(py.NewStringDictFrom(
		py.DictEntry{Key: "SemLock", Value: t},
		py.DictEntry{Key: "SEM_VALUE_MAX", Value: py.Int(32767)},
	))
}
