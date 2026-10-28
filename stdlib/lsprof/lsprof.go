// Package lsprof is gpython's _lsprof, the engine under cProfile.
//
// It is built on sys.setprofile rather than on a C hook, so it sees what a
// profile function sees: the call and return of Python frames.  CPython's
// also times builtin and C calls (the "{built-in method ...}" rows of a
// cProfile report); gpython has no c_call events, so those rows are absent and
// their time is counted in the Python function that called them.  Otherwise
// the accounting is lsprof's: callcount, reccallcount, totaltime (outermost
// activation only), inlinetime (time outside sub-calls), per-caller subentries.
package lsprof

import (
	"sync"
	"time"

	"github.com/vishnukv64/gpython/py"
)

var epoch = time.Now()

type entry struct {
	code      py.Object
	callcount int
	reccount  int
	tt, it    float64
	recursion int
	subs      map[*entry]*entry // keyed by callee entry; value holds the subentry stats
	order     []*entry          // callees in first-seen order
}

type context struct {
	e    *entry
	t0   float64
	subt float64
	prev *context
}

// Profiler is _lsprof.Profiler.
type Profiler struct {
	cls  *py.Type
	Dict py.StringDict

	mu       sync.Mutex
	timer    py.Object
	timeunit float64
	subcalls bool
	entries  map[py.Object]*entry
	order    []*entry
	current  *context
	callback *callback
}

var ProfilerType = py.NewTypeX("_lsprof.Profiler", "Profiler(timer=None, timeunit=None, subcalls=True, builtins=True)\n\n    Builds a profiler object using the specified timer function.", profilerNew, nil)

func (p *Profiler) Type() *py.Type {
	if p.cls != nil {
		return p.cls
	}
	return ProfilerType
}
func (p *Profiler) GetDict() py.StringDict { return p.Dict }

func profilerNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var timer, timeunit py.Object = py.None, py.None
	var subcalls, builtins py.Object = py.True, py.True
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOOO:Profiler", []string{"timer", "timeunit", "subcalls", "builtins"},
		&timer, &timeunit, &subcalls, &builtins); err != nil {
		return nil, err
	}
	p := &Profiler{cls: metatype, Dict: py.NewStringDict(), timeunit: 1, entries: map[py.Object]*entry{}}
	if timer != py.None {
		p.timer = timer
	}
	if timeunit != py.None {
		f, err := py.FloatAsFloat64(timeunit)
		if err != nil {
			return nil, err
		}
		p.timeunit = f
	}
	sub, err := py.ObjectIsTrue(subcalls)
	if err != nil {
		return nil, err
	}
	p.subcalls = sub
	return p, nil
}

// now is the profiler's clock in the units getstats reports (seconds).
func (p *Profiler) now() (float64, error) {
	if p.timer == nil {
		return time.Since(epoch).Seconds(), nil
	}
	v, err := py.Call(p.timer, py.Tuple{}, py.StringDict{})
	if err != nil {
		return 0, err
	}
	f, err := py.FloatAsFloat64(v)
	if err != nil {
		return 0, err
	}
	return f * p.timeunit, nil
}

func (p *Profiler) entryFor(code py.Object) *entry {
	e := p.entries[code]
	if e == nil {
		e = &entry{code: code}
		p.entries[code] = e
		p.order = append(p.order, e)
	}
	return e
}

// callback is the object installed with sys.setprofile.
type callback struct{ p *Profiler }

var callbackType = py.NewTypeX("_lsprof.callback", "", nil, nil)

func (c *callback) Type() *py.Type { return callbackType }

func (c *callback) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) != 3 {
		return py.None, nil
	}
	frame, ok := args[0].(*py.Frame)
	event, _ := args[1].(py.String)
	if !ok {
		return py.None, nil
	}
	p := c.p
	p.mu.Lock()
	defer p.mu.Unlock()
	t, err := p.now()
	if err != nil {
		return nil, err
	}
	switch event {
	case "call":
		e := p.entryFor(frame.Code)
		e.recursion++
		if p.current != nil && p.subcalls {
			p.current.e.sub(e).recursion++
		}
		p.current = &context{e: e, t0: t, prev: p.current}
	case "return":
		p.stop(t)
	}
	return py.None, nil
}

// sub is the subentry for callee under e, created on first use.
func (e *entry) sub(callee *entry) *entry {
	if e.subs == nil {
		e.subs = map[*entry]*entry{}
	}
	s := e.subs[callee]
	if s == nil {
		s = &entry{code: callee.code}
		e.subs[callee] = s
		e.order = append(e.order, callee)
	}
	return s
}

// stop closes the innermost context at time t.
func (p *Profiler) stop(t float64) {
	ctx := p.current
	if ctx == nil {
		return // a return from a frame entered before enable()
	}
	tt := t - ctx.t0
	it := tt - ctx.subt
	if ctx.prev != nil {
		ctx.prev.subt += tt
	}
	p.current = ctx.prev
	e := ctx.e
	if e.recursion--; e.recursion == 0 {
		e.tt += tt
	} else {
		e.reccount++
	}
	e.it += it
	e.callcount++
	if p.subcalls && ctx.prev != nil {
		s := ctx.prev.e.sub(e)
		if s.recursion--; s.recursion == 0 {
			s.tt += tt
		} else {
			s.reccount++
		}
		s.it += it
		s.callcount++
	}
}

// flush closes every context still open, as lsprof does on disable.
func (p *Profiler) flush() {
	if p.current == nil {
		return
	}
	t, err := p.now()
	if err != nil {
		t = 0
	}
	for p.current != nil {
		p.stop(t)
	}
}

var (
	entryType    = py.NewTypeX("_lsprof.profiler_entry", "", nil, nil)
	subentryType = py.NewTypeX("_lsprof.profiler_subentry", "", nil, nil)
)

// Stat is a profiler_entry / profiler_subentry: an object with the attributes
// code, callcount, reccallcount, totaltime, inlinetime (and calls).
type Stat struct {
	typ  *py.Type
	Dict py.StringDict
}

func (s *Stat) Type() *py.Type         { return s.typ }
func (s *Stat) GetDict() py.StringDict { return s.Dict }

func newStat(typ *py.Type, e *entry) *Stat {
	d := py.NewStringDict()
	d.Set("code", e.code)
	d.Set("callcount", py.Int(e.callcount))
	d.Set("reccallcount", py.Int(e.reccount))
	d.Set("totaltime", py.Float(e.tt))
	d.Set("inlinetime", py.Float(e.it))
	return &Stat{typ: typ, Dict: d}
}

func init() {
	ProfilerType.Dict.Set("enable", py.MustNewMethod("enable", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		p := self.(*Profiler)
		var subcalls, builtins py.Object = py.True, py.True
		if err := py.ParseTupleAndKeywords(args, kwargs, "|OO:enable", []string{"subcalls", "builtins"}, &subcalls, &builtins); err != nil {
			return nil, err
		}
		sub, err := py.ObjectIsTrue(subcalls)
		if err != nil {
			return nil, err
		}
		p.mu.Lock()
		p.subcalls = sub
		p.callback = &callback{p: p}
		cb := p.callback
		p.mu.Unlock()
		py.SetProfile(cb)
		return py.None, nil
	}, 0, "enable(subcalls=True, builtins=True)\n\nStart collecting profiling information."))

	ProfilerType.Dict.Set("disable", py.MustNewMethod("disable", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*Profiler)
		p.mu.Lock()
		defer p.mu.Unlock()
		if cb := p.callback; cb != nil {
			if cur, ok := py.ProfileFunc().(*callback); ok && cur == cb {
				py.SetProfile(nil)
			}
			p.callback = nil
		}
		p.flush()
		return py.None, nil
	}, 0, "disable()\n\nStop collecting profiling information."))

	ProfilerType.Dict.Set("clear", py.MustNewMethod("clear", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*Profiler)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.flush()
		p.entries, p.order = map[py.Object]*entry{}, nil
		return py.None, nil
	}, 0, "clear()\n\nClear all profiling information collected so far."))

	ProfilerType.Dict.Set("getstats", py.MustNewMethod("getstats", func(self py.Object, args py.Tuple) (py.Object, error) {
		p := self.(*Profiler)
		p.mu.Lock()
		defer p.mu.Unlock()
		// cProfile's snapshot_stats disables first, which closes open
		// contexts; getstats on a live profiler reports only closed calls.
		out := py.NewList()
		for _, e := range p.order {
			st := newStat(entryType, e)
			var calls py.Object = py.None
			if len(e.order) > 0 {
				l := py.NewList()
				for _, callee := range e.order {
					l.Append(newStat(subentryType, e.subs[callee]))
				}
				calls = l
			}
			st.Dict.Set("calls", calls)
			out.Append(st)
		}
		return out, nil
	}, 0, "getstats() -> list of profiler_entry objects\n\nReturn all information collected by the profiler."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{Name: "_lsprof", Doc: "Fast profiler"},
		PreSetGlobals: func(g py.StringDict) {
			g.Set("Profiler", ProfilerType)
			g.Set("profiler_entry", entryType)
			g.Set("profiler_subentry", subentryType)
		},
	})
}
