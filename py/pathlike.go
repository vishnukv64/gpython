// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

// os.PathLike is a protocol class: CPython's __subclasshook__ makes
// isinstance(x, os.PathLike) true for anything with __fspath__.  Library code
// (zipapp, shutil, tempfile) branches on it, so it is declared here through
// the same structural-check hook the abstract base classes use.
func init() {
	ABCHooks = append(ABCHooks, func(obj Object, class *Type) bool {
		return class.Name == "os.PathLike" && obj.Type().Lookup("__fspath__") != nil
	})
}
