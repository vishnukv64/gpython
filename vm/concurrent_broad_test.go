package vm_test

import (
	"sync"
	"testing"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/pytest"
	_ "github.com/vishnukv64/gpython/stdlib"
)

const broad = `
import json, re, datetime, decimal, pathlib, collections, contextlib, enum, dataclasses, typing, logging, argparse, textwrap, itertools, functools, statistics
@contextlib.contextmanager
def cm():
    yield 1
@cm()
def f(): return 2
@dataclasses.dataclass
class P:
    x: int = 1
class E(enum.Enum):
    A = 1
result = json.dumps([f(), P().x, E.A.value, str(decimal.Decimal('1.1')+1), statistics.mean([1,2,3]), textwrap.dedent(' a'), re.sub('a','b','aa'), str(datetime.date(2020,1,1)), collections.Counter('aab')['a'], list(itertools.islice(itertools.count(), 3)), functools.reduce(lambda a,b:a+b,[1,2,3])])
`

// Sixteen fresh contexts on their own goroutines define classes, enums,
// dataclasses and named tuples at once. Under -race this caught a drain readying
// another goroutine's half-built type, and a plain map written by every enum.
func TestConcurrentBroad(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer py.EnterGoroutine()()
			ctx := py.NewContext(py.DefaultContextOpts())
			defer ctx.Close()
			m, code := pytest.CompileSrc(t, ctx, broad, "x")
			if _, err := ctx.RunCode(code, m.Globals, m.Globals, nil); err != nil {
				t.Error(err)
				return
			}
			if r, _ := m.Globals.Get("result"); r == nil {
				t.Error("no result")
			}
		}()
	}
	wg.Wait()
}
