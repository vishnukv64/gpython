package base64_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestBase64(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}
