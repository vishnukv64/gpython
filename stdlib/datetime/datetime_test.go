package datetime_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestDatetime(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
