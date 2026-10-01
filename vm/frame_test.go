// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vm_test

import (
	"testing"

	"github.com/vishnukv64/gpython/compile"
	"github.com/vishnukv64/gpython/py"
)

// frameProbe is a Python-callable that records how deep the executing frame
// chain is when it is called, and what the caller's globals look like.
type frameProbe struct {
	depths  []int
	names   []string
	globals []py.StringDict
	ctx     py.Context
}

var probeType = py.NewType("frameprobe", "test helper")

func (p *frameProbe) Type() *py.Type { return probeType }

func (p *frameProbe) M__call__(args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	f := p.ctx.Store().CurrentFrame()
	depth := 0
	for at := f; at != nil; at = at.Back {
		depth++
	}
	p.depths = append(p.depths, depth)

	// Walk out to the caller the way inspect.currentframe().f_back does.
	if f != nil && f.Back != nil {
		p.globals = append(p.globals, f.Back.Globals)
		if name, ok := f.Back.Globals["__name__"]; ok {
			if s, ok := name.(py.String); ok {
				p.names = append(p.names, string(s))
			}
		}
	}
	return py.None, nil
}

// runProgram runs src in a fresh context with a frameProbe bound to "probe".
func runProgram(t *testing.T, src string) (*frameProbe, py.Context, error) {
	t.Helper()
	ctx := py.NewContext(py.DefaultContextOpts())

	code, err := compile.Compile(src, "<test>", py.ExecMode, 0, true)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	globals := py.NewStringDict()
	probe := &frameProbe{ctx: ctx}
	globals["probe"] = probe
	globals["__name__"] = py.String("probe_module")
	if _, err := ctx.RunCode(code, globals, globals, nil); err != nil {
		return probe, ctx, err
	}
	return probe, ctx, nil
}

// TestFrameChainBalances is the point of the frame back-pointer: the chain
// must be exactly as deep as the calls are nested, and must be empty again
// once the program has finished.
func TestFrameChainBalances(t *testing.T) {
	src := `
def inner():
    probe()
def outer():
    inner()
def top():
    outer()
top()
`
	probe, ctx, err := runProgram(t, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	defer ctx.Close()

	if len(probe.depths) != 1 {
		t.Fatalf("probe called %d times, want 1", len(probe.depths))
	}
	// inner <- outer <- top <- module body.  probe itself is a Go callable
	// and adds no frame, so the chain is four deep.
	if got, want := probe.depths[0], 4; got != want {
		t.Errorf("frame depth inside inner() = %d, want %d", got, want)
	}
	// The caller of probe is inner, whose globals are the module's.
	if len(probe.names) != 1 || probe.names[0] != "probe_module" {
		t.Errorf("caller globals __name__ = %v, want [\"probe_module\"]", probe.names)
	}
	if f := ctx.Store().CurrentFrame(); f != nil {
		t.Errorf("frame stack not empty after the program finished: depth still non-zero at %v", f.Code.Name)
	}
}

// TestFrameChainUnwindsOnException makes sure a frame is popped when the
// call exits by raising rather than returning.
func TestFrameChainUnwindsOnException(t *testing.T) {
	src := `
def boom():
    probe()
    raise ValueError("bang")
def wrapper():
    try:
        boom()
    except ValueError:
        pass
wrapper()
probe()
`
	probe, ctx, err := runProgram(t, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	defer ctx.Close()

	if len(probe.depths) != 2 {
		t.Fatalf("probe called %d times, want 2", len(probe.depths))
	}
	// boom <- wrapper <- module body.
	if got, want := probe.depths[0], 3; got != want {
		t.Errorf("depth inside boom() = %d, want %d", got, want)
	}
	// After boom raised and the handler ran, the second call is back to the
	// module body alone: the frames from the failed call were popped.
	if got, want := probe.depths[1], 1; got != want {
		t.Errorf("depth after the exception = %d, want %d (frames leaked)", got, want)
	}
	if f := ctx.Store().CurrentFrame(); f != nil {
		t.Errorf("frame stack not empty at the end")
	}
}

// TestFrameChainUnwindsOnGenerator covers the other way a frame stops
// executing without finishing: a yield.
func TestFrameChainUnwindsOnGenerator(t *testing.T) {
	src := `
def gen():
    probe()
    yield 1
    probe()
    yield 2
g = gen()
next(g)
probe()
next(g)
`
	probe, ctx, err := runProgram(t, src)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	defer ctx.Close()

	if len(probe.depths) != 3 {
		t.Fatalf("probe called %d times, want 3", len(probe.depths))
	}
	// Inside the generator body called from the module body: gen + module.
	if got, want := probe.depths[0], 2; got != want {
		t.Errorf("depth inside gen() = %d, want %d", got, want)
	}
	// Between yields the interpreter is back in the module body only.
	if got, want := probe.depths[1], 1; got != want {
		t.Errorf("depth while suspended = %d, want %d (generator frame leaked)", got, want)
	}
	// Resumed: gen + module again.
	if got, want := probe.depths[2], 2; got != want {
		t.Errorf("depth after resume = %d, want %d", got, want)
	}
	if f := ctx.Store().CurrentFrame(); f != nil {
		t.Errorf("frame stack not empty at the end")
	}
}
