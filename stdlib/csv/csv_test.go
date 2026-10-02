package csv_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestCsv(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
