// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Prefixes is where the running interpreter lives, as CPython's getpath
// computes it: sys.executable, sys._base_executable, sys.prefix and
// sys.base_prefix.  Inside a virtual environment (PEP 405) Prefix is the
// environment and BasePrefix the interpreter it was made from.
type Prefixes struct {
	Executable     string
	BaseExecutable string
	Prefix         string
	BasePrefix     string
	Venv           bool // a pyvenv.cfg was found
	SystemSite     bool // include-system-site-packages = true
}

// SitePackages is the purelib directory under prefix, in CPython's layout.
func SitePackages(prefix string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(prefix, "Lib", "site-packages")
	}
	return filepath.Join(prefix, "lib", "python3.14", "site-packages")
}

var (
	prefixesOnce sync.Once
	prefixes     Prefixes
)

// GetPrefixes locates the interpreter once and caches the answer.
func GetPrefixes() Prefixes {
	prefixesOnce.Do(func() { prefixes = findPrefixes() })
	return prefixes
}

// invokedPath is the path the interpreter was started by.  os.Executable
// resolves symlinks on Linux, which would hide a venv's bin/python link, so
// argv[0] is preferred and os.Executable is the fallback.
func invokedPath() string {
	if a := os.Args[0]; a != "" {
		if !strings.ContainsRune(a, filepath.Separator) {
			if p, err := exec.LookPath(a); err == nil {
				a = p
			}
		}
		if abs, err := filepath.Abs(a); err == nil {
			if st, err := os.Stat(abs); err == nil && !st.IsDir() {
				// Resolve the directory but never the file: like CPython,
				// /tmp -> /private/tmp is followed, a venv's python link is not.
				if d, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
					abs = filepath.Join(d, filepath.Base(abs))
				}
				return abs
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return ""
}

// prefixOf is the installation prefix of an executable's directory: the
// parent of "bin" (or "Scripts" is never a base), else the directory itself.
func prefixOf(dir string) string {
	if runtime.GOOS != "windows" && filepath.Base(dir) == "bin" {
		return filepath.Dir(dir)
	}
	return dir
}

func findPrefixes() Prefixes {
	exe := invokedPath()
	p := Prefixes{Executable: exe, BaseExecutable: exe}
	if exe == "" {
		return p
	}
	dir := filepath.Dir(exe)
	p.Prefix = prefixOf(dir)
	p.BasePrefix = p.Prefix
	// pyvenv.cfg beside the executable, or one directory up.
	var cfg []byte
	var cfgDir string
	for _, d := range []string{dir, filepath.Dir(dir)} {
		if b, err := os.ReadFile(filepath.Join(d, "pyvenv.cfg")); err == nil {
			cfg, cfgDir = b, d
			break
		}
	}
	if cfg == nil {
		return p
	}
	p.Venv = true
	p.Prefix = cfgDir
	p.BasePrefix = cfgDir
	var home, base string
	for _, line := range strings.Split(string(cfg), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "home":
			home = v
		case "executable":
			base = v
		case "include-system-site-packages":
			p.SystemSite = strings.EqualFold(v, "true")
		}
	}
	if home != "" {
		p.BasePrefix = prefixOf(home)
		p.BaseExecutable = filepath.Join(home, filepath.Base(exe))
	}
	if base != "" {
		p.BaseExecutable = base
	}
	if _, err := os.Stat(p.BaseExecutable); err != nil {
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			p.BaseExecutable = r
		}
	}
	return p
}

// ParsePth is site.addpackage for every "*.pth" file in a site directory, in
// name order.  A line that is blank or a comment is skipped; one starting with
// "import" is returned in imports to be executed; any other names a directory,
// relative to the site directory, which is returned in paths when it exists.
// Editable installs and "uv run --with" overlays rely on this.
func ParsePth(sitedir string) (paths, imports []string) {
	ents, err := os.ReadDir(sitedir)
	if err != nil {
		return nil, nil
	}
	for _, e := range ents { // ReadDir sorts by name, as site.py does
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") || !strings.HasSuffix(n, ".pth") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(sitedir, n))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimRight(line, "\r")
			switch {
			case line == "" || line[0] == '#':
			case strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "import\t"):
				imports = append(imports, line)
			default:
				d := strings.TrimRight(line, " \t")
				if !filepath.IsAbs(d) {
					d = filepath.Join(sitedir, d)
				}
				if st, err := os.Stat(d); err == nil && st.IsDir() {
					paths = append(paths, d)
				}
			}
		}
	}
	return paths, imports
}

// ExecPthLine runs one "import ..." line of a .pth file, reporting a failure
// on stderr and carrying on, as site.py does.
func ExecPthLine(ctx Context, line string) {
	code, err := Compile(line, "<pth>", ExecMode, 0, true)
	if err == nil {
		g := NewStringDict()
		g.Set("__name__", String("__pth__"))
		_, err = ctx.RunCode(code, g, g, nil)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing .pth line %q:\n", line)
		TracebackDump(err)
	}
}

// RunPthImports executes the "import" lines of the .pth files in the prefix's
// site-packages, once sys exists (site.py runs them at startup).
func RunPthImports(ctx Context) {
	for _, s := range sitePackagesDirs() {
		_, imports := ParsePth(s)
		for _, line := range imports {
			ExecPthLine(ctx, line)
		}
	}
}

// sitePackagesDirs are the existing site-packages directories startup adds.
func sitePackagesDirs() []string {
	pf := GetPrefixes()
	sites := []string{SitePackages(pf.Prefix)}
	if pf.Venv && pf.SystemSite {
		sites = append(sites, SitePackages(pf.BasePrefix))
	}
	var out []string
	for _, s := range sites {
		if st, err := os.Stat(s); err == nil && st.IsDir() {
			out = append(out, s)
		}
	}
	return out
}
