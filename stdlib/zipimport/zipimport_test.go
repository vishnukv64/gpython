package zipimport_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestZipimport(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
