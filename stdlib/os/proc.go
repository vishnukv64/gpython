// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os

import (
	"crypto/rand"
	"os"
	"runtime"

	"github.com/vishnukv64/gpython/py"
)

// procMethods are the os functions multiprocessing builds on.  The ones that
// need POSIX file descriptors and wait4 live in proc_unix.go.
func procMethods() []*py.Method {
	return append([]*py.Method{
		py.MustNewMethod("urandom", func(self py.Object, args py.Tuple) (py.Object, error) {
			n, err := oneInt(args, "urandom")
			if err != nil {
				return nil, err
			}
			if n < 0 {
				return nil, py.ExceptionNewf(py.ValueError, "negative argument not allowed")
			}
			b := make([]byte, n)
			if _, err := rand.Read(b); err != nil {
				return nil, py.OSErrorFrom(err, "")
			}
			return py.Bytes(b), nil
		}, 0, "Return n random bytes suitable for cryptographic use."),
		py.MustNewMethod("cpu_count", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(runtime.NumCPU()), nil
		}, 0, "Return the number of logical CPUs in the system."),
		py.MustNewMethod("process_cpu_count", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(runtime.NumCPU()), nil
		}, 0, "Return the number of logical CPUs usable by the process."),
		py.MustNewMethod("getppid", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Int(os.Getppid()), nil
		}, 0, "Return the parent's process id."),
	}, unixMethods()...)
}

// oneInt parses a single integer argument.
func oneInt(args py.Tuple, name string) (int, error) {
	if len(args) != 1 {
		return 0, py.ExceptionNewf(py.TypeError, "%s() takes exactly one argument (%d given)", name, len(args))
	}
	return py.IndexInt(args[0])
}
