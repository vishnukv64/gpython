// Package devtools holds the gpython-specific Python that CPython's
// developer tooling (pdb, ...) needs and that cannot be taken from CPython's
// Lib unchanged.
package devtools

import (
	_ "embed"

	"github.com/vishnukv64/gpython/py"
)

//go:embed pyrepl_utils.py
var pyreplUtilsSrc string

//go:embed tracemalloc.py
var tracemallocSrc string

func init() {
	// tracemalloc is declined: importing it raises ImportError with the reason.
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "tracemalloc", FileDesc: "tracemalloc.py", Doc: "Declined: see the ImportError."},
		CodeSrc: tracemallocSrc,
	})
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_pyrepl", Doc: "Stand-in package: only utils exists."},
	})
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "_pyrepl.utils", FileDesc: "_pyrepl/utils.py", Doc: "Stand-in for CPython's _pyrepl.utils."},
		CodeSrc: pyreplUtilsSrc,
	})
}
