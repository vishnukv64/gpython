// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Gpython binary

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	"github.com/go-python/gpython/py"
	"github.com/go-python/gpython/repl"
	"github.com/go-python/gpython/repl/cli"

	_ "github.com/go-python/gpython/stdlib"
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
	flag.Usage = syntaxError
	flag.Parse()
	if *showVer {
		fmt.Printf("Gpython %s (%s, %s)\n", version, commit, date)
		return
	}
	xmain(flag.Args())
}

func xmain(args []string) {
	var err error

	switch {
	case *runM != "":
		err = runModule(append([]string{*runM}, args...))
	case *runC != "":
		err = runCode(*runC, args)
	case len(args) == 0:
		err = runREPL(args)
	default:
		err = runScript(args)
	}

	if err != nil {
		if py.IsException(py.SystemExit, err) {
			handleSystemExit(err.(py.ExceptionInfo).Value.(*py.Exception))
		}
		py.TracebackDump(err)
		os.Exit(1)
	}
}

// runREPL starts the interactive interpreter.
func runREPL(args []string) error {
	opts := py.DefaultContextOpts()
	opts.SysArgs = args
	ctx := py.NewContext(opts)
	defer ctx.Close()

	fmt.Printf("Python 3.4.0 (%s, %s)\n", commit, date)
	fmt.Printf("[Gpython %s]\n", version)
	fmt.Printf("- os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("- go version: %s\n", runtime.Version())

	return cli.RunREPL(repl.New(ctx))
}

// runScript executes a file, with the file's directory as the first search
// path so that it can import modules sitting next to it.
func runScript(args []string) error {
	opts := py.DefaultContextOpts()
	opts.SysPaths = append([]string{filepath.Dir(args[0])}, opts.SysPaths...)
	opts.SysArgs = args
	ctx := py.NewContext(opts)
	defer ctx.Close()

	if err := startCPUProfile(); err != nil {
		return err
	}
	defer pprof.StopCPUProfile()

	_, err := py.RunFile(ctx, args[0], py.CompileOpts{}, nil)
	return err
}

// runCode executes a program given on the command line with -c.
func runCode(src string, args []string) error {
	opts := py.DefaultContextOpts()
	opts.SysPaths = append([]string{"."}, opts.SysPaths...)
	opts.SysArgs = args
	ctx := py.NewContext(opts)
	defer ctx.Close()

	_, err := py.RunSrc(ctx, src, "<string>", "__main__")
	return err
}

// runModule executes a module or package with -m, the way CPython runs
// "python -m pkg" or "python -m pkg.mod".
func runModule(args []string) error {
	name := args[0]
	opts := py.DefaultContextOpts()
	opts.SysPaths = append([]string{"."}, opts.SysPaths...)
	opts.SysArgs = args
	ctx := py.NewContext(opts)
	defer ctx.Close()

	path, isPkg, err := py.ResolveModulePath(ctx, name)
	if err != nil {
		return py.ExceptionNewf(py.ImportError, "No module named %q", name)
	}

	if !isPkg {
		_, err = py.RunFile(ctx, path, py.CompileOpts{}, "__main__")
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
	_, err = py.RunFile(ctx, mainPath, py.CompileOpts{}, "__main__")
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

func handleSystemExit(exc *py.Exception) {
	args := exc.Args.(py.Tuple)
	if len(args) == 0 {
		os.Exit(0)
	} else if len(args) == 1 {
		if code, ok := args[0].(py.Int); ok {
			c, err := code.GoInt()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(c)
		}
		msg, err := py.ReprAsString(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
		os.Exit(1)
	} else {
		msg, err := py.ReprAsString(args)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
		os.Exit(1)
	}
}
