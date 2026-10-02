// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vishnukv64/gpython/py"
)

// kw builds a keyword-argument mapping from alternating name/value pairs.
func kw(pairs ...string) py.StringDict {
	d := py.NewStringDict()
	for i := 0; i+1 < len(pairs); i += 2 {
		d.Set(pairs[i], py.String(pairs[i+1]))
	}
	return d
}

// TestWhichFindsAnExecutable checks the PATH search against a command that is
// definitely on PATH, and a directory that must never be a result.
func TestWhichFindsAnExecutable(t *testing.T) {
	// A binary the platform certainly has, in a directory this test creates
	// and puts on PATH, so the answer does not depend on the host's PATH.
	dir := t.TempDir()
	exe := filepath.Join(dir, "gpy-test-prog")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "gpy-test-plain")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "gpy-test-dir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// The explicit path is searched, and the executable file there is found.
	got, err := which(nil, nil, kw("cmd", "gpy-test-prog", "path", dir))
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := got.(py.String); !ok || string(s) != exe {
		t.Errorf("which('gpy-test-prog', path=%q) = %v, want %q", dir, got, exe)
	}

	// A non-executable file is not found: the mode bits are consulted.
	got, err = which(nil, nil, kw("cmd", "gpy-test-plain", "path", dir))
	if err != nil {
		t.Fatal(err)
	}
	if got != py.None {
		t.Errorf("which found a non-executable file: %v", got)
	}

	// A directory is never a command, even with every bit set.
	got, err = which(nil, nil, kw("cmd", "gpy-test-dir", "path", dir))
	if err != nil {
		t.Fatal(err)
	}
	if got != py.None {
		t.Errorf("which returned a directory: %v", got)
	}

	// A command that is not there answers None, not an error.
	got, err = which(nil, nil, kw("cmd", "gpy-no-such-command-xyzzy"))
	if err != nil {
		t.Fatal(err)
	}
	if got != py.None {
		t.Errorf("which found a command that does not exist: %v", got)
	}
}

// TestRmtreeRefusesSymlinkToDirectory is the case the Python test cannot set
// up: this interpreter's os has no symlink.  Following the link would delete
// the tree it points at, which is never what the caller asked for.
func TestRmtreeRefusesSymlinkToDirectory(t *testing.T) {
	d := t.TempDir()
	real := filepath.Join(d, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(real, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(d, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("this platform cannot create a symlink: %v", err)
	}

	if _, err := rmtree(nil, py.Tuple{py.String(link)}, py.NewStringDict()); err == nil {
		t.Fatal("rmtree followed a symlink to a directory instead of refusing it")
	}
	// The target tree is untouched and the link is still a link.
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("rmtree deleted through the symlink: %v", err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("rmtree removed the symlink it was told to refuse")
	}
}

// TestRmtreeRemovesCreatedTree is the ordinary case: a tree this test built is
// gone afterwards, with every file in it.
func TestRmtreeRemovesCreatedTree(t *testing.T) {
	d := t.TempDir()
	tree := filepath.Join(d, "tree")
	nested := filepath.Join(tree, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "deep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rmtree(nil, py.Tuple{py.String(tree)}, py.NewStringDict()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Fatalf("rmtree left the tree behind: %v", err)
	}
}

// TestDiskUsageAnswersRealNumbers checks the figures are self-consistent.
func TestDiskUsageAnswersRealNumbers(t *testing.T) {
	d := t.TempDir()
	got, err := diskUsage(nil, py.Tuple{py.String(d)}, py.NewStringDict())
	if err != nil {
		t.Fatal(err)
	}
	u, ok := got.(*usage)
	if !ok {
		t.Fatalf("disk_usage returned %T", got)
	}
	if u.total <= 0 {
		t.Errorf("total = %d, want > 0", u.total)
	}
	if u.used < 0 || u.free < 0 {
		t.Errorf("used = %d, free = %d, want non-negative", u.used, u.free)
	}
	if u.total != u.used+u.free {
		t.Errorf("total %d != used %d + free %d", u.total, u.used, u.free)
	}
}
