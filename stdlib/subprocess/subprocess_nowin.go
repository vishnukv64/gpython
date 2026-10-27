//go:build !windows && !gpython_simwin

package subprocess

import "github.com/vishnukv64/gpython/py"

func platformEntries() []py.DictEntry { return nil }
