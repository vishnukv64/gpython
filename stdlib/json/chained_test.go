package json_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// TestJsonChained loads a document with json and yaml and walks into it, the
// way real code uses a parsed document: obj["a"]["b"]["c"], obj.get("a").get("b"),
// and the mutating forms.  The structures a loader returns are ordinary dicts
// and lists, so these compose - and if that ever stops being true, this fails.
func TestJsonChained(t *testing.T) {
	pytest.RunScript(t, "./testdata/test_chained.py")
}
