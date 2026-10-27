//go:build gpython_simwin && !windows

package sys

import "github.com/vishnukv64/gpython/py"

func winExtras() []py.DictEntry {
	return []py.DictEntry{{Key: "getwindowsversion", Value: py.MustNewMethod("getwindowsversion", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Tuple{py.Int(10), py.Int(0), py.Int(19045), py.Int(2), py.String("")}, nil
	}, 0, "Simulated Windows version.")}}
}
