// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Gpython binary

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/repl"
	"github.com/vishnukv64/gpython/repl/cli"
	"github.com/vishnukv64/gpython/stdlib/atexit"

	_ "github.com/vishnukv64/gpython/stdlib"
)

var (
	cpuprofile = flag.String("cpuprofile", "", "Write cpu profile to file")
	runC       = flag.String("c", "", "Program passed in as string")
	runM       = flag.String("m", "", "Run library module as a script")
	showVer    = flag.Bool("version", false, "Print the version and exit")
)

// syntaxError prints the syntax
func syntaxError() {
	fmt.Fprintf(os.Stderr, `GPython

A python implementation in Go

Full options:
`)
	flag.PrintDefaults()
}

func main() {
	// Python code allocates many short-lived objects, so Go's default GOGC=100
	// collects too often: 200 measured 7-11% faster on call-heavy code for a
	// modest heap increase (400 doubled peak RSS for little more).  An explicit
	// GOGC in the environment still wins.
	// ponytail: fixed ratio; a memory-limit-aware policy if large heaps matter.
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(200)
	}
	flag.Usage = syntaxError
	// Stop at the first non-flag argument, the way CPython does.
	//
	// Go's flag package keeps parsing after a positional argument, so every
	// flag AFTER the script name was eaten by gpython instead of reaching the
	// program: "gpython -m pip --version" printed gpython's own version, and
	// "gpython -m pip --help" printed gpython's own help.  pip has both flags,
	// so it could never be invoked at all.  Parse only the leading flags -
	// everything from the first non-flag word onward belongs to the program.
	args, split := stripCPythonOptions(os.Args[1:])
	if err := flag.CommandLine.Parse(args[:split]); err != nil {
		return
	}
	if *showVer {
		fmt.Printf("Gpython %s (%s, %s)\n", version, commit, date)
		return
	}
	// flag.Args() holds only what Parse itself was given, so the arguments
	// after gpython's region - the program's own - are appended back.
	xmain(append(flag.Args(), args[split:]...))
}

// stripCPythonOptions drops the leading CPython options gpython has no use
// for (-I -E -s -S -B -u -O -q -b -d -R -v -P, -W arg, -X arg), which pip,
// venv and ensurepip pass to the interpreter, and returns the arguments with
// the index where gpython's own option region ends.  The region stops at the
// first positional word, as CPython's does, and after -c/-m's program.
func stripCPythonOptions(args []string) ([]string, int) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || len(a) == 0 || a[0] != '-' {
			// "-" means stdin, which is a positional argument too.
			return append(out, args[i:]...), len(out)
		}
		// A cluster of ignorable flags ending in c or m ("-Ic", "-EsSc", "-Im"
		// - findpython runs every interpreter it finds with -Ic) is those
		// flags plus -c or -m.
		if n := len(a); n > 2 && a[1] != '-' && (a[n-1] == 'c' || a[n-1] == 'm') && strings.Trim(a[1:n-1], "IEsSBuOqbdRvP") == "" {
			a = "-" + a[n-1:]
			args[i] = a
		}
		// "-c" and "-m" take the PROGRAM as their value, so their flag
		// ends gpython's own option region: everything after it - the
		// program's arguments, including its own "--version" - belongs to
		// the program.
		if a == "-c" || a == "-m" {
			out = append(out, args[i:]...)
			if i+2 <= len(args) {
				return out, len(out) - len(args) + i + 2
			}
			return out, len(out) - len(args) + i + 1
		}
		if a == "-W" || a == "-X" {
			i++
			continue
		}
		if strings.HasPrefix(a, "-W") || strings.HasPrefix(a, "-X") || strings.Trim(a[1:], "IEsSBuOqbdRvP") == "" && len(a) > 1 && a[1] != '-' {
			continue
		}
		// A flag that takes a separate value must not have that value
		// mistaken for the first positional argument.
		out = append(out, a)
		if a == "-cpuprofile" && i+1 < len(args) {
			i++
			out = append(out, args[i])
		}
	}
	return out, len(out)
}

func xmain(args []string) {
	var err error

	switch {
	case *runM != "":
		err = runModule(append([]string{*runM}, args...))
	case *runC != "":
		err = runCode(*runC, args, "-c")
	case len(args) == 0:
		err = runREPL(args)
	default:
		err = runScript(args)
	}

	if err != nil {
		// The error was already reported and atexit has run (finish); a
		// SystemExit only picks the status.
		if py.IsException(py.SystemExit, err) {
			handleSystemExit(err.(py.ExceptionInfo).Value.(*py.Exception))
		}
		// An uncaught KeyboardInterrupt ends the process as if by SIGINT, as
		// CPython does on POSIX: the shell sees status 130.
		if py.IsException(py.KeyboardInterrupt, err) && runtime.GOOS != "windows" {
			os.Exit(130)
		}
		os.Exit(1)
	}
}

// finish ends a run in CPython's order, while the interpreter can still
// import and execute: an uncaught exception's traceback (or a SystemExit's
// message) is printed, then the atexit handlers run, then the context closes.
// Running the handlers after Close made any import in one fail with "Context
// closed" (IPython's exit-time "import gc").
func finish(ctx py.Context, err error) {
	if err != nil {
		if py.IsException(py.SystemExit, err) {
			printSystemExit(err.(py.ExceptionInfo).Value.(*py.Exception))
		} else {
			py.TracebackDump(err)
		}
	}
	atexit.RunExitFuncs()
	ctx.Close()
}

// runREPL starts the interactive interpreter.
func runREPL(args []string) (err error) {
	opts := py.DefaultContextOpts()
	// CPython's sys.argv[0] is "" in the interactive interpreter, and never
	// absent - a program may index it unconditionally.
	opts.SysArgs = append([]string{""}, args...)
	ctx := py.NewContext(opts)
	defer func() { finish(ctx, err) }()

	fmt.Printf("Python 3.14.7 (%s, %s)\n", commit, date)
	fmt.Printf("[Gpython %s]\n", version)
	fmt.Printf("- os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("- go version: %s\n", runtime.Version())

	return cli.RunREPL(repl.New(ctx))
}

// runScript executes a file, with the file's directory as the first search
// path so that it can import modules sitting next to it.
func runScript(args []string) (err error) {
	if args[0] == "-" { // the program is read from standard input
		src, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		return runCode(string(src), args[1:], "-")
	}
	opts := py.DefaultContextOpts()
	opts.SysPaths = append([]string{filepath.Dir(args[0])}, opts.SysPaths...)
	opts.SysArgs = args
	ctx := py.NewContext(opts)
	defer func() { finish(ctx, err) }()
	importSiteCustomize(ctx)

	if err := startCPUProfile(); err != nil {
		return err
	}
	defer pprof.StopCPUProfile()

	_, err = py.RunFile(ctx, args[0], py.CompileOpts{}, nil)
	return err
}

// runCode executes a program given on the command line with -c.
func runCode(src string, args []string, argv0 string) (err error) {
	opts := py.DefaultContextOpts()
	opts.SysPaths = append([]string{"."}, opts.SysPaths...)
	// sys.argv[0] is "-c" under -c, exactly as CPython has it.  Passing only
	// the program's own arguments left argv EMPTY, and pip reads argv[0] to
	// name the program in its usage line: "pip --help" died with
	// "IndexError: list index out of range" inside optparse's get_prog_name.
	opts.SysArgs = append([]string{argv0}, args...)
	ctx := py.NewContext(opts)
	defer func() { finish(ctx, err) }()
	importSiteCustomize(ctx)

	name := "<string>"
	if argv0 == "-" {
		name = "<stdin>"
	}
	_, err = py.RunSrc(ctx, src, name, "__main__")
	return err
}

// runModule executes a module or package with -m, the way CPython runs
// "python -m pkg" or "python -m pkg.mod".
func runModule(args []string) (err error) {
	name := args[0]
	opts := py.DefaultContextOpts()
	opts.SysPaths = append([]string{"."}, opts.SysPaths...)
	// sys.argv[0] is set AFTER the module is located, because -m puts the
	// RESOLVED FILE PATH there and not the module name: CPython runs
	// "python -m m" with argv[0] == "/abs/path/m.py".  Setting it to the bare
	// name made the two disagree, which is visible in any program that prints
	// its own invocation - pip names itself from argv[0] in its usage line.
	opts.SysArgs = args
	ctx := py.NewContext(opts)
	defer func() { finish(ctx, err) }()
	importSiteCustomize(ctx)

	path, isPkg, err := py.ResolveModulePath(ctx, name)
	if err != nil {
		return py.ExceptionNewf(py.ImportError, "No module named %q", name)
	}
	// Resolved: argv[0] is the path of the file that runs, with the program's
	// own arguments after it.  sys.argv is rewritten on the live sys module,
	// because the context has already been built and its sys.argv is what the
	// program reads.  CPython puts an ABSOLUTE path there.
	setArgv0 := func(runPath string) {
		if abs, err := filepath.Abs(runPath); err == nil {
			runPath = abs
		}
		if sysMod, err := ctx.GetModule("sys"); err == nil && sysMod != nil {
			sysMod.Globals.Set("argv", py.NewListFromStrings(append([]string{runPath}, args[1:]...)))
		}
	}

	if !isPkg {
		setArgv0(path)
		// Run as __main__ but with a spec naming the real module, which is
		// what CPython does and what pip's __main__.py reads.
		_, err = py.RunFileAsNamed(ctx, path, py.CompileOpts{}, "__main__", name)
		return err
	}

	// A package: run its __main__ submodule, as CPython does.  The package has
	// to be imported first so that its __path__ is available for locating it.
	if err := py.Import(ctx, name); err != nil {
		return err
	}
	mainName := name + ".__main__"
	mainPath, _, err := py.ResolveModulePath(ctx, mainName)
	if err != nil {
		return py.ExceptionNewf(py.ImportError, "No module named %q", mainName)
	}
	// For a package the file that runs is __main__.py, not __init__.py:
	// pip's usage line is built from argv[0].
	setArgv0(mainPath)
	// __name__ must be "__main__" - that is the whole point of -m, and pip's
	// __main__.py guards on it: with the package name there, the guard was
	// false and "gpython -m pip" printed nothing and exited 0.  The SPEC names
	// the real module, which is a separate thing and is what pip reads to
	// decide whether it is running from a wheel.
	_, err = py.RunFileAsNamed(ctx, mainPath, py.CompileOpts{}, "__main__", mainName)
	return err
}

// startCPUProfile begins CPU profiling when -cpuprofile was given.
func startCPUProfile() error {
	if *cpuprofile == "" {
		return nil
	}
	f, err := os.Create(*cpuprofile)
	if err != nil {
		return err
	}
	return pprof.StartCPUProfile(f)
}

// systemExitCode is the exit status a SystemExit asks for, as CPython's
// handle_system_exit: none or None is 0, an int is itself, anything else is
// printed to stderr (by printSystemExit) and gives 1.
func systemExitCode(exc *py.Exception) int {
	args, _ := exc.Args.(py.Tuple)
	if len(args) == 0 || (len(args) == 1 && args[0] == py.None) {
		return 0
	}
	if len(args) == 1 {
		if code, ok := args[0].(py.Int); ok {
			if c, err := code.GoInt(); err == nil {
				return c
			}
		}
	}
	return 1
}

// printSystemExit writes a SystemExit's non-integer argument to stderr, as
// CPython does before running atexit handlers.
func printSystemExit(exc *py.Exception) {
	args, _ := exc.Args.(py.Tuple)
	var msg py.Object = args
	if len(args) == 1 {
		if _, isInt := args[0].(py.Int); isInt || args[0] == py.None {
			return
		}
		msg = args[0]
	} else if len(args) == 0 {
		return
	}
	if s, ok := msg.(py.String); ok {
		fmt.Fprintln(os.Stderr, string(s))
		return
	}
	if r, err := py.ReprAsString(msg); err == nil {
		fmt.Fprintln(os.Stderr, r)
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
}

func handleSystemExit(exc *py.Exception) {
	os.Exit(systemExitCode(exc))
}

// importSiteCustomize runs a sitecustomize module if one is on sys.path, as
// CPython's startup does; pip's build isolation relies on it to set up
// sys.path in the child.  A missing module is normal and ignored.
func importSiteCustomize(ctx py.Context) {
	py.RunPthImports(ctx)
	if err := py.Import(ctx, "sitecustomize"); err != nil && !py.IsException(py.ImportError, err) {
		py.TracebackDump(err)
	}
}
