package vm_test

import (
	"testing"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/pytest"
)

// Hot-loop cost of the interrupt check: a tight bytecode loop and a call-heavy
// recursion, each run to completion on one context per benchmark.
const benchLoopSrc = `
total = 0
for i in range(1000000):
    total += i
`

const benchCallSrc = `
def fib(n):
    if n < 2:
        return n
    return fib(n - 1) + fib(n - 2)

fib(24)
`

func benchRun(b *testing.B, src string) {
	ctx := py.NewContext(py.DefaultContextOpts())
	defer ctx.Close()
	module, code := pytest.CompileSrc(b, ctx, src, "bench")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ctx.RunCode(code, module.Globals, module.Globals, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoop(b *testing.B)  { benchRun(b, benchLoopSrc) }
func BenchmarkCalls(b *testing.B) { benchRun(b, benchCallSrc) }
