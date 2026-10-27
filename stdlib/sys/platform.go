package sys

import "runtime"

// platform is sys.platform, as CPython spells it: "win32" on every Windows
// (32- and 64-bit alike), "linux", "darwin".  The BSDs get CPython's
// "freebsd14"-style names only if the release were cheap to read; it is not,
// so they keep GOOS.  A package-level initialiser so that packages whose init
// reads sys.platform never see it unset.
var platform = platformName(runtime.GOOS)

func platformName(goos string) string {
	if goos == "windows" || simWin {
		return "win32"
	}
	return goos
}
