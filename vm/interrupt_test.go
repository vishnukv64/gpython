package vm_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/pytest"
)

// A host's Request stops a non-terminating loop with KeyboardInterrupt.
func TestInterruptStopsInfiniteLoop(t *testing.T) {
	ctx := py.NewContext(py.DefaultContextOpts())
	defer ctx.Close()
	module, code := pytest.CompileSrc(t, ctx, "while True:\n    pass\n", "spin")

	go func() {
		time.Sleep(50 * time.Millisecond)
		ctx.Interrupt().Request()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := ctx.RunCode(code, module.Globals, module.Globals, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "KeyboardInterrupt") {
			t.Fatalf("want KeyboardInterrupt, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("interrupt did not stop the loop")
	}
}

// A run that is not interrupted is unaffected, and a request made while nothing
// runs does not stop the next run.
func TestInterruptNotLeakedAcrossRuns(t *testing.T) {
	ctx := py.NewContext(py.DefaultContextOpts())
	defer ctx.Close()
	module, code := pytest.CompileSrc(t, ctx, "total = 0\nfor i in range(1000):\n    total += i\n", "sum")

	ctx.Interrupt().Request() // idle: must not carry into the next run
	if _, err := ctx.RunCode(code, module.Globals, module.Globals, nil); err != nil {
		t.Fatalf("late request leaked into next run: %v", err)
	}
	if got, _ := module.Globals.Get("total"); got != py.Int(499500) {
		t.Fatalf("total = %v, want 499500", got)
	}
}

// Runaway recursion raises a catchable RecursionError at the limit instead of
// overflowing the Go stack, and sys.setrecursionlimit moves the limit.
func TestRecursionErrorCatchable(t *testing.T) {
	ctx := py.NewContext(py.DefaultContextOpts())
	defer ctx.Close()
	src := `
import sys
def f(n):
    return f(n + 1)
caught = None
try:
    f(0)
except RecursionError as e:
    caught = str(e)
limit = sys.getrecursionlimit()
sys.setrecursionlimit(50)
def g(n):
    return g(n + 1)
try:
    g(0)
except RecursionError:
    pass
sys.setrecursionlimit(limit)
`
	module, code := pytest.CompileSrc(t, ctx, src, "recur")
	if _, err := ctx.RunCode(code, module.Globals, module.Globals, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := module.Globals.Get("caught"); got != py.String("maximum recursion depth exceeded") {
		t.Fatalf("caught = %v", got)
	}
	if got, _ := module.Globals.Get("limit"); got != py.Int(1000) {
		t.Fatalf("limit = %v, want 1000", got)
	}
}

// time.sleep returns when the host interrupts, instead of holding the run's
// goroutine for the whole sleep.
func TestInterruptWakesSleep(t *testing.T) {
	ctx := py.NewContext(py.DefaultContextOpts())
	defer ctx.Close()
	module, code := pytest.CompileSrc(t, ctx, "import time\ntime.sleep(60)\n", "sleeper")

	go func() {
		time.Sleep(50 * time.Millisecond)
		ctx.Interrupt().Request()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := ctx.RunCode(code, module.Globals, module.Globals, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "KeyboardInterrupt") {
			t.Fatalf("want KeyboardInterrupt, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("interrupt did not wake time.sleep")
	}
}

// Stop ends a loop that swallows KeyboardInterrupt, and a thread the snippet
// left running: a plain Request is consumed by the first handler that sees it.
func TestInterruptStopCannotBeSwallowed(t *testing.T) {
	ctx := py.NewContext(py.DefaultContextOpts())
	defer ctx.Close()
	src := `
import _thread
def spin():
    while True:
        try:
            while True:
                pass
        except BaseException:
            pass
_thread.start_new_thread(spin, ())
spin()
`
	module, code := pytest.CompileSrc(t, ctx, src, "swallow")
	go func() {
		time.Sleep(50 * time.Millisecond)
		ctx.Interrupt().Stop()
	}()
	done := make(chan error, 1)
	go func() {
		_, err := ctx.RunCode(code, module.Globals, module.Globals, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "KeyboardInterrupt") {
			t.Fatalf("want KeyboardInterrupt, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not end a loop that swallows KeyboardInterrupt")
	}
}
