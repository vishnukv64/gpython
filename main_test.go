// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var regen = flag.Bool("regen", false, "regenerate golden files")

func TestGPython(t *testing.T) {

	tmp, err := os.MkdirTemp("", "go-python-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	exe := filepath.Join(tmp, "out.exe")
	cmd := exec.Command("go", "build", "-o", exe, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		t.Fatalf("failed to compile embedding example: %+v", err)
	}

	got, err := exec.Command(exe, "testdata/hello.py").CombinedOutput()
	if err != nil {
		t.Fatalf("could not run gpython:\n%s\nerr: %+v", got, err)
	}

	const fname = "testdata/hello_golden.txt"

	flag.Parse()
	if *regen {
		err = os.WriteFile(fname, got, 0644)
		if err != nil {
			t.Fatalf("could not write golden file: %+v", err)
		}
	}

	want, err := os.ReadFile(fname)
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if err != nil {
		t.Fatalf("could not read golden file: %+v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stdout differ:\ngot:\n%s\nwant:\n%s\n", got, want)
	}

	// -m on a package runs its __main__.py, and argv[0] names that file (as in
	// CPython), not the package's __init__.py: pip's usage line is built from it.
	pkg := filepath.Join(tmp, "runme")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{
		"__init__.py": "",
		"__main__.py": "import os, sys\nprint(os.path.basename(sys.argv[0]), sys.argv[1:], __name__)\n",
	} {
		if err := os.WriteFile(filepath.Join(pkg, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd = exec.Command(exe, "-m", "runme", "a")
	cmd.Dir = tmp
	got, err = cmd.CombinedOutput()
	if want := "__main__.py ['a'] __main__\n"; err != nil || string(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))) != want {
		t.Fatalf("-m package: got %q (err %v), want %q", got, err, want)
	}
}

func TestRunFile(t *testing.T) {
	xmain([]string{"./testdata/hello.py"})
}
