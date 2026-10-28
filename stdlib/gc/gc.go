// Package gc provides a minimal 'gc' module. Go's collector owns memory, so
// the Python-level API only tracks the enabled flag and thresholds.
package gc

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/weakref"
)

var enabled atomic.Bool
var threshold = [3]int{700, 10, 10}

func init() {
	enabled.Store(true)
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "gc", Doc: "Interface to the garbage collector (minimal: Go's GC manages memory)."},
		Methods: []*py.Method{
			py.MustNewMethod("collect", func(self py.Object, args py.Tuple) (py.Object, error) {
				runtime.GC()
				// Finalizers run on their own goroutine; give them a moment to queue.
				time.Sleep(5 * time.Millisecond)
				weakref.RunPending()
				// ponytail: Go has no cycle-unreachable count; always 0.
				return py.Int(0), nil
			}, 0, "collect(generation=2) -> number of unreachable objects found (always 0)."),
			py.MustNewMethod("enable", func(self py.Object) (py.Object, error) { enabled.Store(true); return py.None, nil }, 0, "enable() -> None"),
			py.MustNewMethod("disable", func(self py.Object) (py.Object, error) { enabled.Store(false); return py.None, nil }, 0, "disable() -> None"),
			py.MustNewMethod("isenabled", func(self py.Object) (py.Object, error) { return py.NewBool(enabled.Load()), nil }, 0, "isenabled() -> bool"),
			py.MustNewMethod("get_count", func(self py.Object) (py.Object, error) { return py.Tuple{py.Int(0), py.Int(0), py.Int(0)}, nil }, 0, "get_count() -> (0, 0, 0)"),
			py.MustNewMethod("get_threshold", func(self py.Object) (py.Object, error) {
				return py.Tuple{py.Int(threshold[0]), py.Int(threshold[1]), py.Int(threshold[2])}, nil
			}, 0, "get_threshold() -> (t0, t1, t2)"),
			py.MustNewMethod("set_threshold", func(self py.Object, args py.Tuple) (py.Object, error) {
				for i := 0; i < len(args) && i < 3; i++ {
					n, err := py.IndexInt(args[i])
					if err != nil {
						return nil, err
					}
					threshold[i] = n
				}
				return py.None, nil
			}, 0, "set_threshold(t0[, t1[, t2]]) -> None"),
			py.MustNewMethod("get_objects", func(self py.Object, args py.Tuple) (py.Object, error) { return py.NewList(), nil }, 0, "get_objects() -> [] (not tracked)"),
		},
	})
}
