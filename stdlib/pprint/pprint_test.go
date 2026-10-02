package pprint_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestPprint(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
