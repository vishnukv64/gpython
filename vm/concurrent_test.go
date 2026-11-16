package vm_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/pytest"
)

// Many contexts run at once in one process: each has its own globals and its
// own sys.stdout, and all of them import the same stdlib modules. Under -race
// this proves the process-global state they share (module caches, init-once
// tables) is synchronised.
const concurrentSrc = `
import sys, io, re, json, collections
sys.stdout = io.StringIO()
d = collections.OrderedDict()
total = 0
for i in range(2000):
    d[str(i)] = i
    total += len(re.findall(r"\d", str(i)))
out = json.dumps({"n": len(d), "digits": total})
print(out)
result = sys.stdout.getvalue()
`

func TestManyContextsConcurrent(t *testing.T) {
	const workers = 8
	const rounds = 3
	want := `{"n": 2000, "digits": 6890}` + "\n"
	for r := 0; r < rounds; r++ {
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Each worker is a Python thread; unregistered goroutines share id 1.
				defer py.EnterGoroutine()()
				ctx := py.NewContext(py.DefaultContextOpts())
				defer ctx.Close()
				module, code := pytest.CompileSrc(t, ctx, concurrentSrc, "conc")
				if _, err := ctx.RunCode(code, module.Globals, module.Globals, nil); err != nil {
					errs <- err
					return
				}
				got, _ := module.Globals.Get("result")
				if got != py.String(want) {
					errs <- fmt.Errorf("result = %q, want %q", got, want)
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
	}
}
