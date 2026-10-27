package asyncio_test

import (
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestAsyncio(t *testing.T) {
	pytest.RunTests(t, "./testdata")
}
