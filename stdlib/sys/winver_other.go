//go:build !windows && !gpython_simwin

package sys

import "github.com/vishnukv64/gpython/py"

func winExtras() []py.DictEntry { return nil }
