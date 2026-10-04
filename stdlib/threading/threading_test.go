package threading_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestThreading(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
