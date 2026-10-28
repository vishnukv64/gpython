// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathlib

import (
	"testing"

	"github.com/vishnukv64/gpython/py"
)

// WindowsPath's Go Base is Path, so a Base-chain walk alone parsed every
// Windows Path with the POSIX flavour ("C:\x" had no root and as_uri()
// called it relative).
func TestConcreteClassesPickTheirFlavour(t *testing.T) {
	for _, c := range []struct {
		t    *py.Type
		want *py.Type
	}{
		{WindowsPathType, PureWindowsPathType},
		{PosixPathType, PurePosixPathType},
	} {
		if !isKindOf(c.t, c.want) {
			t.Errorf("isKindOf(%s, %s) = false", c.t.Name, c.want.Name)
		}
	}
	if isKindOf(WindowsPathType, PurePosixPathType) || isKindOf(PosixPathType, PureWindowsPathType) {
		t.Errorf("a concrete class claims the other flavour")
	}
}
