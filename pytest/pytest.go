// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pytest

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vishnukv64/gpython/compile"
	"github.com/vishnukv64/gpython/py"

	_ "github.com/vishnukv64/gpython/stdlib"
)

var RegenTestData = flag.Bool("regen", false, "Regenerate golden files from current testdata.")

// containsString reports whether the list of objects holds the given string.
func containsString(items []py.Object, want string) bool {
	for _, item := range items {
		if s, ok := item.(py.String); ok && string(s) == want {
			return true
		}
	}
	return false
}

// gContext is the shared context the tests run their scripts in.  Its SysArgs
// is the test binary's own name rather than empty, so that a script reading
// sys.argv[0] sees an ordinary string and not an IndexError - a program may
// index argv[0] unconditionally, and CPython always provides one.
var gContext = py.NewContext(func() py.ContextOpts {
	opts := py.DefaultContextOpts()
	opts.SysArgs = []string{"gpython-test"}
	return opts
}())

// Compile the program in the file prog to code in the module that is returned
func compileProgram(t testing.TB, prog string) (*py.Module, *py.Code) {
	f, err := os.Open(prog)
	if err != nil {
		t.Fatalf("%s: Open failed: %v", prog, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("%s: Close failed: %v", prog, err)
		}
	}()

	str, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("%s: ReadAll failed: %v", prog, err)
	}
	return CompileSrc(t, gContext, string(str), prog)
}

func CompileSrc(t testing.TB, ctx py.Context, pySrc string, prog string) (*py.Module, *py.Code) {
	code, err := compile.Compile(string(pySrc), prog, py.ExecMode, 0, true)
	if err != nil {
		t.Fatalf("%s: Compile failed: %v", prog, err)
	}

	module, err := ctx.Store().NewModule(ctx, &py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:     py.MainModuleName,
			FileDesc: prog,
		},
	})
	if err != nil {
		t.Fatalf("%s: NewModule failed: %v", prog, err)
	}

	// The directory of the script being run is its first search path, which is
	// what lets it import the helper modules sitting next to it.  This is the
	// same rule CPython applies to "python script.py".
	sysMod := ctx.Store().MustGetModule("sys")
	paths, ok := sysMod.Globals.GetOrNil("path").(*py.List)
	if ok && !containsString(paths.Items, path.Dir(prog)) {
		paths.Items = append([]py.Object{py.String(path.Dir(prog))}, paths.Items...)
	}

	return module, code
}

// Run the code in the module
func run(t testing.TB, module *py.Module, code *py.Code) {
	_, err := gContext.RunCode(code, module.Globals, module.Globals, nil)
	if err != nil {
		if wantErrObj, ok := module.Globals.Get("err"); ok {
			gotExc, ok := err.(py.ExceptionInfo)
			if !ok {
				t.Fatalf("got err is not ExceptionInfo: %#v", err)
			}
			if gotExc.Value.Type() != wantErrObj.Type() {
				t.Fatalf("Want exception %v got %v", wantErrObj, gotExc.Value)
			}
			// t.Logf("matched exception")
			return
		} else {
			py.TracebackDump(err)
			t.Fatalf("Run failed: %v at %q", err, module.Globals.GetOrNil("doc"))
		}
	}

	// t.Logf("%s: Return = %v", prog, res)
	if doc, ok := module.Globals.Get("doc"); ok {
		if docStr, ok := doc.(py.String); ok {
			if string(docStr) != "finished" {
				t.Fatalf("Didn't finish at %q", docStr)
			}
		} else {
			t.Fatalf("Set doc variable to non string: %#v", doc)
		}
	} else {
		t.Fatalf("Didn't set doc variable at all")
	}
}

// find the python files in the directory passed in
func findFiles(t testing.TB, testDir string) (names []string) {
	files, err := os.ReadDir(testDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, f := range files {
		name := f.Name()
		if !strings.HasPrefix(name, "lib") && strings.HasSuffix(name, ".py") {
			names = append(names, name)
		}
	}
	return names
}

// runScriptTask compiles and runs one script in its OWN module, honouring the
// two conventions every test script under py/tests and vm/tests follows:
//
//	err = SomeException   the script is EXPECTED to raise that exception
//	doc = "finished"       the script reached its last statement
//
// The module is built and run in two steps rather than through py.RunFile
// because RunFile returns nil on failure - so the globals holding "err" and
// "doc" would be unreachable exactly when they are needed.
func runScriptTask(ctx py.Context, pyFile string) error {
	out, err := ctx.ResolveAndCompile(pyFile, py.CompileOpts{})
	if err != nil {
		return fmt.Errorf("could not run target script %q: %w", pyFile, err)
	}

	module, err := ctx.Store().NewModule(ctx, &py.ModuleImpl{
		Info: py.ModuleInfo{
			Name:     py.MainModuleName,
			FileDesc: pyFile,
		},
		Code: out.Code,
	})
	if err != nil {
		return fmt.Errorf("could not run target script %q: %w", pyFile, err)
	}

	_, runErr := ctx.RunCode(out.Code, module.Globals, module.Globals, nil)

	if runErr != nil {
		wantErrObj, ok := module.Globals.Get("err")
		if !ok {
			return fmt.Errorf("could not run target script %q: %w", pyFile, runErr)
		}
		gotExc, ok := runErr.(py.ExceptionInfo)
		if !ok {
			return fmt.Errorf("could not run target script %q: got %#v, which is not ExceptionInfo", pyFile, runErr)
		}
		if gotExc.Value.Type() != wantErrObj.Type() {
			return fmt.Errorf("could not run target script %q: want exception %v, got %v", pyFile, wantErrObj, gotExc.Value)
		}
		return nil
	}

	if doc, ok := module.Globals.Get("doc"); ok {
		docStr, ok := doc.(py.String)
		if !ok {
			return fmt.Errorf("could not run target script %q: doc is not a string: %#v", pyFile, doc)
		}
		if string(docStr) != "finished" {
			return fmt.Errorf("could not run target script %q: did not finish, stopped at %q", pyFile, docStr)
		}
	}

	return nil
}

// addScriptDirToSysPath puts the directory holding a script at the FRONT of
// sys.path, which is what lets it import the helper modules sitting beside it -
// the rule CPython applies to "python script.py".  An entry already present is
// left alone rather than duplicated.
func addScriptDirToSysPath(sys *py.Module, prog string) {
	dir := filepath.Dir(prog)
	paths, ok := sys.Globals.GetOrNil("path").(*py.List)
	if !ok {
		return
	}
	for _, item := range paths.Items {
		if s, ok := item.(py.String); ok && string(s) == dir {
			return
		}
	}
	paths.Items = append([]py.Object{py.String(dir)}, paths.Items...)
}

// RunTests runs the tests in the directory passed in
//
// A script that HAS a <name>_golden.txt is compared against it; one without is
// only run.  Comparing previously happened in no caller of this function at all,
// so the goldens under py/tests/ were INERT - present, plausible, and enforcing
// nothing.
func RunTests(t *testing.T, testDir string) {
	for _, name := range findFiles(t, testDir) {
		name := name
		t.Run(name, func(t *testing.T) {
			runFileWithGolden(t, path.Join(testDir, name))
		})
	}
}

// runFileWithGolden runs one script, comparing its output when a golden exists.
//
// Each script gets a context of its own, so state one script leaves behind - a
// logger it configured, a module it replaced - is not inherited by the next.
func runFileWithGolden(t *testing.T, pyFile string) {
	// ABSOLUTE paths, because a script may chdir (logging.handlers' test does,
	// to a temp directory) and the golden is read after it has run.  A relative
	// path resolved correctly before the script and then failed after it with
	// "could not read golden output testdata/test_golden.txt" for a file that
	// plainly exists.
	absPy, err := filepath.Abs(pyFile)
	if err != nil {
		t.Fatalf("%s: Abs failed: %v", pyFile, err)
	}

	task := &Task{PyFile: absPy}
	gold := absPy[:len(absPy)-len(filepath.Ext(absPy))] + "_golden.txt"
	if _, err := os.Stat(gold); err == nil {
		task.GoldFile = gold
	} else {
		// Not every script in py/tests has a golden; without one there is
		// nothing to compare, so it is merely run.
		task.AllowMissingGolden = true
		task.GoldFile = gold
	}
	RunTestTasks(t, []*Task{task})
}

// RunBenchmarks runs the benchmarks in the directory passed in
func RunBenchmarks(b *testing.B, testDir string) {
	for _, name := range findFiles(b, testDir) {
		module, code := compileProgram(b, path.Join(testDir, name))
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				run(b, module, code)
			}
		})
	}
}

// RunScript runs the provided path to a script.
// RunScript captures the stdout and stderr while executing the script
// and compares it to a golden file, blocking until completion.
//
//	RunScript("./testdata/foo.py")
//
// will compare the output with "./testdata/foo_golden.txt".
func RunScript(t *testing.T, fname string) {

	RunTestTasks(t, []*Task{
		{
			PyFile: fname,
		},
	})
}

// RunTestTasks runs each given task in a newly created py.Context concurrently.
// If a fatal error is encountered, the given testing.T is signaled.
func RunTestTasks(t *testing.T, tasks []*Task) {
	onCompleted := make(chan *Task)

	numTasks := len(tasks)
	for ti := 0; ti < numTasks; ti++ {
		task := tasks[ti]
		go func() {
			err := task.run()
			task.Err = err
			onCompleted <- task
		}()
	}

	tasks = tasks[:0]
	for ti := 0; ti < numTasks; ti++ {
		task := <-onCompleted
		if task.Err != nil {
			t.Error(task.Err)
		}
		tasks = append(tasks, task)
	}
}

var (
	taskCounter int32
)

type Task struct {
	num      int32                      // Assigned when this task is run
	ID       string                     // unique key identifying this task.  If empty, autogenerated from the basename of PyFile
	PyFile   string                     // If set, this file pathname is executed in a newly created ctx
	PyTask   func(ctx py.Context) error // If set, a new created ctx is created and this blocks until completion
	GoldFile string                     // Filename containing the "gold standard" stdout+stderr.  If empty, autogenerated from PyFile or ID
	// AllowMissingGolden makes a MISSING golden mean "nothing to compare"
	// rather than an error.  The directory runner needs it, because not every
	// script in py/tests has one; RunScript does not set it, so a named script
	// that lost its golden still fails.
	AllowMissingGolden bool
	Err                error // Non-nil if a fatal error is encountered with this task
}

func (task *Task) run() error {
	fileBase := ""

	opts := py.DefaultContextOpts()
	if task.PyFile != "" {
		opts.SysArgs = []string{task.PyFile}
		if task.ID == "" {
			ext := filepath.Ext(task.PyFile)
			fileBase = task.PyFile[0 : len(task.PyFile)-len(ext)]
		}
	}

	task.num = atomic.AddInt32(&taskCounter, 1)
	if task.ID == "" {
		if fileBase == "" {
			task.ID = fmt.Sprintf("task-%04d", atomic.AddInt32(&taskCounter, 1))
		} else {
			task.ID = strings.TrimPrefix(fileBase, "./")
		}
	}

	if task.GoldFile == "" {
		task.GoldFile = fileBase + "_golden.txt"
	}

	ctx := py.NewContext(opts)
	defer ctx.Close()

	sys := ctx.Store().MustGetModule("sys")
	tmp, err := os.MkdirTemp("", "gpython-pytest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	out, err := os.Create(filepath.Join(tmp, "combined"))
	if err != nil {
		return fmt.Errorf("could not create stdout+stderr output file: %w", err)
	}
	defer out.Close()

	sys.Globals.Set("stdout", &py.File{File: out, FileMode: py.FileWrite})
	sys.Globals.Set("stderr", &py.File{File: out, FileMode: py.FileWrite})

	if task.PyFile != "" {
		// The directory of the script is its first search path, which is what
		// lets it import the helpers sitting next to it (py/tests' scripts all
		// import libtest).  CompileSrc applies the same rule; without it here a
		// script run through this task path could not find them.
		addScriptDirToSysPath(sys, task.PyFile)
		if err := runScriptTask(ctx, task.PyFile); err != nil {
			return err
		}
	}

	if task.PyTask != nil {
		err := task.PyTask(ctx)
		if err != nil {
			return fmt.Errorf("PyTask %q failed: %w", task.ID, err)
		}
	}

	// Close the ctx explicitly as it may legitimately generate output
	ctx.Close()
	<-ctx.Done()

	err = out.Close()
	if err != nil {
		return fmt.Errorf("could not close output file: %w", err)
	}

	got, err := os.ReadFile(out.Name())
	if err != nil {
		return fmt.Errorf("could not read script output file: %w", err)
	}

	if *RegenTestData {
		err := os.WriteFile(task.GoldFile, got, 0644)
		if err != nil {
			return fmt.Errorf("could not write golden output %q: %w", task.GoldFile, err)
		}
	}

	want, err := os.ReadFile(task.GoldFile)
	if err != nil {
		if task.AllowMissingGolden {
			return nil
		}
		return fmt.Errorf("could not read golden output %q: %w", task.GoldFile, err)
	}

	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	diff := cmp.Diff(string(want), string(got))
	if !bytes.Equal(got, want) {
		// The actual output is kept OUTSIDE the source tree, in the temp
		// directory, not beside the script: writing "<script>.txt" into the tree
		// left stray files after every run, and could overwrite a real
		// hand-written .txt already sitting there.  (The task's own temp dir is
		// removed on return, so the dump goes to the shared one.)
		gotPath := filepath.Join(os.TempDir(), filepath.Base(fileBase)+".got.txt")
		if werr := os.WriteFile(gotPath, got, 0644); werr != nil {
			gotPath = "(could not be written)"
		}
		return fmt.Errorf("output differ: -- (-ref +got)\n%s\n--- got written to %s", diff, gotPath)
	}

	return nil
}
