package sys

import (
	"sync"

	"github.com/vishnukv64/gpython/py"
)

// sys.monitoring (PEP 669) is present for its tool-id registry and constants,
// which is what bdb.py reads at import time, but gpython generates none of its
// events: a call that would enable one raises, so nothing silently waits for
// events that never come.  bdb's default backend is sys.settrace, which works.

var monitoringEvents = []struct {
	name string
	bit  int
}{{"PY_START", 1}, {"PY_RESUME", 2}, {"PY_RETURN", 4}, {"PY_YIELD", 8}, {"CALL", 16}, {"LINE", 32},
	{"INSTRUCTION", 64}, {"JUMP", 128}, {"BRANCH_LEFT", 256}, {"BRANCH_RIGHT", 512}, {"STOP_ITERATION", 1024},
	{"RAISE", 2048}, {"EXCEPTION_HANDLED", 4096}, {"PY_UNWIND", 8192}, {"PY_THROW", 16384}, {"RERAISE", 32768},
	{"C_RETURN", 65536}, {"C_RAISE", 131072}, {"BRANCH", 262144}, {"NO_EVENTS", 0}}

var monitoring struct {
	sync.Mutex
	tools [6]py.Object // name of the tool using each id, or nil
}

func toolID(o py.Object) (int, error) {
	i, ok := o.(py.Int)
	if !ok {
		return 0, py.ExceptionNewf(py.TypeError, "tool_id must be an int, not %s", o.Type().Name)
	}
	if i < 0 || int(i) >= len(monitoring.tools) {
		return 0, py.ExceptionNewf(py.ValueError, "invalid tool %d (must be between 0 and 5)", int(i))
	}
	return int(i), nil
}

func monitoringModule() py.Object {
	events := py.NewStringDict()
	for _, e := range monitoringEvents {
		events.Set(e.name, py.Int(e.bit))
	}
	evObj := &py.Module{ModuleImpl: &py.ModuleImpl{Info: py.ModuleInfo{Name: "sys.monitoring.events"}}, Globals: events}

	noEvents := func(what string) error {
		return py.ExceptionNewf(py.RuntimeError, "%s: gpython does not generate sys.monitoring events; use sys.settrace or sys.setprofile", what)
	}
	m := func(name string, f func(args py.Tuple) (py.Object, error)) py.Object {
		return py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) { return f(args) }, 0, "")
	}
	g := py.NewStringDict()
	g.Set("events", evObj)
	g.Set("DEBUGGER_ID", py.Int(0))
	g.Set("COVERAGE_ID", py.Int(1))
	g.Set("PROFILER_ID", py.Int(2))
	g.Set("OPTIMIZER_ID", py.Int(5))
	g.Set("DISABLE", py.NewStringDict())
	g.Set("MISSING", py.NewStringDict())
	g.Set("use_tool_id", m("use_tool_id", func(a py.Tuple) (py.Object, error) {
		if len(a) != 2 {
			return nil, py.ExceptionNewf(py.TypeError, "use_tool_id() takes 2 arguments (%d given)", len(a))
		}
		id, err := toolID(a[0])
		if err != nil {
			return nil, err
		}
		monitoring.Lock()
		defer monitoring.Unlock()
		if monitoring.tools[id] != nil {
			return nil, py.ExceptionNewf(py.ValueError, "tool %d is already in use", id)
		}
		monitoring.tools[id] = a[1]
		return py.None, nil
	}))
	g.Set("free_tool_id", m("free_tool_id", func(a py.Tuple) (py.Object, error) {
		id, err := toolID(a[0])
		if err != nil {
			return nil, err
		}
		monitoring.Lock()
		monitoring.tools[id] = nil
		monitoring.Unlock()
		return py.None, nil
	}))
	g.Set("clear_tool_id", m("clear_tool_id", func(a py.Tuple) (py.Object, error) {
		_, err := toolID(a[0])
		return py.None, err
	}))
	g.Set("get_tool", m("get_tool", func(a py.Tuple) (py.Object, error) {
		id, err := toolID(a[0])
		if err != nil {
			return nil, err
		}
		monitoring.Lock()
		defer monitoring.Unlock()
		if t := monitoring.tools[id]; t != nil {
			return t, nil
		}
		return py.None, nil
	}))
	g.Set("get_events", m("get_events", func(a py.Tuple) (py.Object, error) { return py.Int(0), nil }))
	g.Set("get_local_events", m("get_local_events", func(a py.Tuple) (py.Object, error) { return py.Int(0), nil }))
	g.Set("register_callback", m("register_callback", func(a py.Tuple) (py.Object, error) {
		if len(a) == 3 && a[2] != py.None {
			return nil, noEvents("register_callback")
		}
		return py.None, nil
	}))
	g.Set("restart_events", m("restart_events", func(a py.Tuple) (py.Object, error) { return py.None, nil }))
	setEvents := func(name string, evIdx int) py.Object {
		return m(name, func(a py.Tuple) (py.Object, error) {
			if len(a) > evIdx {
				if n, ok := a[evIdx].(py.Int); !ok || n != 0 {
					return nil, noEvents(name)
				}
			}
			return py.None, nil
		})
	}
	g.Set("set_events", setEvents("set_events", 1))
	g.Set("set_local_events", setEvents("set_local_events", 2))
	return &py.Module{ModuleImpl: &py.ModuleImpl{Info: py.ModuleInfo{Name: "sys.monitoring", Doc: "Event monitoring (PEP 669): tool registry only."}}, Globals: g}
}
