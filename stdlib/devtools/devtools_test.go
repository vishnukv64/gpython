package devtools_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

// Each script's golden is CPython 3.14's output for the same script.
func TestDevTools(t *testing.T) {
	for _, script := range []string{"settrace_gold", "settrace2_gold", "trace_gold", "prof_gold", "pdb_gold", "modulefinder_gold", "dis_gold", "faulthandler_gold", "import_fail_gold", "declined_gold"} {
		t.Run(script, func(t *testing.T) { pytest.RunScript(t, "./testdata/"+script+".py") })
	}
}
