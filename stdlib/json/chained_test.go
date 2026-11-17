package json_test

import (
	"testing"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/pytest"
)

// TestJsonChained loads a document with json and yaml and walks into it, the
// way real code uses a parsed document: obj["a"]["b"]["c"], obj.get("a").get("b"),
// and the mutating forms.  The structures a loader returns are ordinary dicts
// and lists, so these compose - and if that ever stops being true, this fails.
func TestJsonChained(t *testing.T) {
	pytest.RunScript(t, "./testdata/test_chained.py")
}

// A dict that contains itself is a ValueError, as a cyclic list is. It used to
// recurse until the Go stack overflowed, which no recover can catch.
func TestJsonCircularDictIsAnError(t *testing.T) {
	ctx := py.NewContext(py.DefaultContextOpts())
	defer ctx.Close()
	src := `
import json
x = {}
x['a'] = x
try:
    json.dumps(x)
    result = "no error"
except ValueError as e:
    result = str(e)
`
	module, code := pytest.CompileSrc(t, ctx, src, "cyclic")
	if _, err := ctx.RunCode(code, module.Globals, module.Globals, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := module.Globals.Get("result"); got != py.String("Circular reference detected") {
		t.Fatalf("result = %v, want Circular reference detected", got)
	}
}
