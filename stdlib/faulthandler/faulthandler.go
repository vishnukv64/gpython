// Package faulthandler is gpython's faulthandler.
//
// dump_traceback walks the calling goroutine's Python frame chain and writes it
// in CPython's format.  enable/disable/is_enabled keep CPython's state and
// validate the file, but gpython installs no fatal-signal handler: a Go
// runtime crash prints Go's own goroutine dump, and nothing here can add the
// Python frames to it.  Not provided, because they need to read another
// goroutine's frames or install signal handlers: dump_traceback_later,
// cancel_dump_traceback_later, register, unregister.  all_threads=True dumps
// the calling goroutine only.
package faulthandler

import (
	"fmt"
	"strings"
	"sync"

	"github.com/vishnukv64/gpython/py"
)

const maxFrames = 100 // CPython's MAX_FRAME_DEPTH

var state struct {
	sync.Mutex
	enabled bool
	file    py.Object
}

// target resolves the file argument to a writer: sys.stderr for None, os.write
// for an int descriptor, otherwise the object's write() followed by flush().
// CPython insists on a real descriptor (fileno()); this accepts any file-like
// object too, so a StringIO can capture a dump.
func target(self py.Object, file py.Object) (py.Object, func(string) error, error) {
	mod, _ := self.(*py.Module)
	if file == nil || file == py.None {
		if mod == nil {
			return nil, nil, py.ExceptionNewf(py.RuntimeError, "sys.stderr is None")
		}
		sys, err := mod.Context.GetModule("sys")
		if err != nil {
			return nil, nil, err
		}
		if file = sys.Globals.GetOrNil("stderr"); file == nil || file == py.None {
			return nil, nil, py.ExceptionNewf(py.RuntimeError, "sys.stderr is None")
		}
	}
	if i, ok := file.(py.Int); ok {
		if i < 0 || mod == nil {
			return nil, nil, py.ExceptionNewf(py.ValueError, "file is not a valid file descriptor")
		}
		return file, func(text string) error {
			osMod, err := mod.Context.GetModule("os")
			if err != nil {
				return err
			}
			write, err := py.GetAttrString(osMod, "write")
			if err != nil {
				return err
			}
			_, err = py.Call(write, py.Tuple{i, py.Bytes(text)}, py.StringDict{})
			return err
		}, nil
	}
	write, err := py.GetAttrString(file, "write")
	if err != nil {
		return nil, nil, err
	}
	return file, func(text string) error {
		if _, err := py.Call(write, py.Tuple{py.String(text)}, py.StringDict{}); err != nil {
			return err
		}
		if flush, err := py.GetAttrString(file, "flush"); err == nil {
			_, err = py.Call(flush, py.Tuple{}, py.StringDict{})
			return err
		}
		return nil
	}, nil
}

func parseFile(self py.Object, args py.Tuple, kwargs py.StringDict, name string) (py.Object, func(string) error, bool, error) {
	var file, all py.Object = py.None, py.True
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:"+name, []string{"file", "all_threads"}, &file, &all); err != nil {
		return nil, nil, false, err
	}
	a, err := py.ObjectIsTrue(all)
	if err != nil {
		return nil, nil, false, err
	}
	f, w, err := target(self, file)
	return f, w, a, err
}

// traceback is the calling goroutine's Python stack in CPython's format.
func traceback(allThreads bool) string {
	var b strings.Builder
	if allThreads {
		fmt.Fprintf(&b, "Current thread 0x%016x (most recent call first):\n", py.GoroutineID())
	} else {
		b.WriteString("Stack (most recent call first):\n")
	}
	n := 0
	for f := py.CurrentFrame(); f != nil; f = f.Back {
		if n == maxFrames {
			b.WriteString("  ...\n")
			break
		}
		fmt.Fprintf(&b, "  File \"%s\", line %d in %s\n", f.Code.Filename, f.CurrentLine(), f.Code.Name)
		n++
	}
	if n == 0 {
		b.WriteString("  <no Python frame>\n")
	}
	return b.String()
}

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "faulthandler", Doc: "faulthandler module: dump_traceback, and the enable/disable state."},
		Methods: []*py.Method{
			py.MustNewMethod("dump_traceback", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				_, write, all, err := parseFile(self, args, kwargs, "dump_traceback")
				if err != nil {
					return nil, err
				}
				if err := write(traceback(all)); err != nil {
					return nil, err
				}
				return py.None, nil
			}, 0, "dump_traceback(file=sys.stderr, all_threads=True): dump the traceback of the current goroutine into file (all_threads is accepted; only the current goroutine is dumped)"),
			py.MustNewMethod("enable", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
				f, _, _, err := parseFile(self, args, kwargs, "enable")
				if err != nil {
					return nil, err
				}
				state.Lock()
				state.enabled, state.file = true, f
				state.Unlock()
				return py.None, nil
			}, 0, "enable(file=sys.stderr, all_threads=True): record that fault dumping is on (gpython installs no fatal-signal handler)"),
			py.MustNewMethod("disable", func(self py.Object, args py.Tuple) (py.Object, error) {
				state.Lock()
				was := state.enabled
				state.enabled, state.file = false, nil
				state.Unlock()
				return py.NewBool(was), nil
			}, 0, "disable(): turn fault dumping off; returns whether it was on"),
			py.MustNewMethod("is_enabled", func(self py.Object, args py.Tuple) (py.Object, error) {
				state.Lock()
				defer state.Unlock()
				return py.NewBool(state.enabled), nil
			}, 0, "is_enabled() -> bool"),
		},
	})
}
