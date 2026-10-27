// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package multiprocessing holds the end-to-end tests of multiprocessing and
// concurrent.futures.ProcessPoolExecutor.  Workers are real gpython processes
// (the "spawn" start method), so the tests build the gpython binary once and
// run each testdata script through it, comparing stdout with the golden file
// CPython 3.14 produced for the same script.
package multiprocessing

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var regen = flag.Bool("regen", false, "regenerate golden files")

var gpython string

func TestMain(m *testing.M) {
	flag.Parse()
	tmp, err := os.MkdirTemp("", "gpython-mp-")
	if err != nil {
		panic(err)
	}
	gpython = filepath.Join(tmp, "gpython")
	cmd := exec.Command("go", "build", "-o", gpython, "github.com/vishnukv64/gpython")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	code := 1
	if err := cmd.Run(); err != nil {
		os.Stderr.WriteString("building gpython: " + err.Error() + "\n")
	} else {
		code = m.Run()
	}
	os.RemoveAll(tmp)
	os.Exit(code)
}

func TestScripts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("multiprocessing is POSIX-only here: _multiprocessing.SemLock raises NotImplementedError on windows")
	}
	scripts, err := filepath.Glob("testdata/*.py")
	if err != nil || len(scripts) == 0 {
		t.Fatalf("no scripts: %v", err)
	}
	for _, script := range scripts {
		name := script[:len(script)-len(".py")]
		t.Run(filepath.Base(name), func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(gpython, script)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%v\nstdout:\n%s\nstderr:\n%s", err, stdout.Bytes(), stderr.Bytes())
			}
			golden := name + "_golden.txt"
			if *regen {
				if err := os.WriteFile(golden, stdout.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(stdout.Bytes(), want) {
				t.Fatalf("stdout differs\ngot:\n%s\nwant:\n%s\nstderr:\n%s", stdout.Bytes(), want, stderr.Bytes())
			}
		})
	}
}
