//go:build !gpython_simwin

package sys

import "testing"

func TestPlatformName(t *testing.T) {
	for goos, want := range map[string]string{
		"windows": "win32", "linux": "linux", "darwin": "darwin", "freebsd": "freebsd",
	} {
		if got := platformName(goos); got != want {
			t.Errorf("platformName(%q) = %q, want %q", goos, got, want)
		}
	}
}
