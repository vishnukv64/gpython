//go:build windows

package sys

import (
	"golang.org/x/sys/windows"

	"github.com/vishnukv64/gpython/py"
)

// winExtras: attributes only a win32 sys has.  getwindowsversion() is a plain
// tuple (major, minor, build, platform, service_pack); CPython's result is a
// struct sequence, but multiprocessing only slices it.
func winExtras() []py.DictEntry {
	return []py.DictEntry{{Key: "getwindowsversion", Value: py.MustNewMethod("getwindowsversion", func(self py.Object, args py.Tuple) (py.Object, error) {
		v := windows.RtlGetVersion()
		return py.Tuple{py.Int(v.MajorVersion), py.Int(v.MinorVersion), py.Int(v.BuildNumber), py.Int(v.PlatformId), py.String("")}, nil
	}, 0, "Return the running Windows version as (major, minor, build, platform, service_pack).")}}
}
