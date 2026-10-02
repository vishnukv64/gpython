// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package os_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vishnukv64/gpython/pytest"
)

func TestOs(t *testing.T) {
	pytest.RunScript(t, "./testdata/test.py")
}

// TestWalk builds the tree walk.py walks.  It is built here rather than in
// Python because the symlink has to exist for the followlinks assertions to
// mean anything, and creating one needs os.symlink.
func TestWalk(t *testing.T) {
	root := t.TempDir()
	mustMkdir := func(p string) {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite := func(p string) {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustMkdir(filepath.Join(root, "a", "b"))
	mustWrite(filepath.Join(root, "top.txt"))
	mustWrite(filepath.Join(root, "a", "f1.txt"))
	if err := os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "link_to_a")); err != nil {
		t.Fatal(err)
	}

	os.Setenv("GPY_WALK_ROOT", root)
	defer os.Unsetenv("GPY_WALK_ROOT")

	pytest.RunScript(t, "./testdata/walk.py")
}
