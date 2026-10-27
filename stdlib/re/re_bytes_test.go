package re_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestBytes(t *testing.T) {
	pytest.RunScript(t, "./testdata/bytes.py")
}
