// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile writes a file inside dir, creating parent directories.
func writeFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile %q: %v", path, err)
	}
}

// newImportTestContext builds a context whose sys.path holds a temporary
// directory laid out as a small package tree:
//
//	plain.py        VALUE = 1
//	pkg/__init__.py NAME = "pkg"; from .sub import helper
//	pkg/sub.py      def helper(): return "sub"
//	pkg2/__init__.py (empty)
//	pkg2/mod.py     X = 2
func newImportTestContext(t *testing.T) Context {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "plain.py", "VALUE = 1\n")
	writeFile(t, root, "pkg/__init__.py", "NAME = \"pkg\"\nfrom .sub import helper\n")
	writeFile(t, root, "pkg/sub.py", "def helper():\n    return \"sub\"\n")
	writeFile(t, root, "pkg2/__init__.py", "")
	writeFile(t, root, "pkg2/mod.py", "X = 2\n")

	opts := DefaultContextOpts()
	opts.SysPaths = append([]string{root}, opts.SysPaths...)
	return NewContext(opts)
}

// lookup returns the named global of an imported module.
func lookup(t *testing.T, ctx Context, module, name string) Object {
	t.Helper()
	mod, err := ctx.GetModule(module)
	if err != nil {
		t.Fatalf("GetModule(%q): %v", module, err)
	}
	obj, ok := mod.Globals[name]
	if !ok {
		t.Fatalf("%s has no global %q", module, name)
	}
	return obj
}

func TestImportModule(t *testing.T) {
	ctx := newImportTestContext(t)
	defer ctx.Close()

	if err := Import(ctx, "plain"); err != nil {
		t.Fatalf("import plain: %v", err)
	}
	if got := lookup(t, ctx, "plain", "VALUE"); got != Int(1) {
		t.Errorf("plain.VALUE = %v, want 1", got)
	}
}

func TestImportPackageRunsRelativeImport(t *testing.T) {
	ctx := newImportTestContext(t)
	defer ctx.Close()

	// pkg/__init__.py does "from .sub import helper", so a successful import
	// proves both package loading and PEP 328 relative imports.
	if err := Import(ctx, "pkg"); err != nil {
		t.Fatalf("import pkg: %v", err)
	}
	if got := lookup(t, ctx, "pkg", "NAME"); got != String("pkg") {
		t.Errorf("pkg.NAME = %v, want 'pkg'", got)
	}
	helper := lookup(t, ctx, "pkg", "helper")
	res, err := Call(helper, Tuple{}, nil)
	if err != nil {
		t.Fatalf("pkg.helper(): %v", err)
	}
	if res != String("sub") {
		t.Errorf("pkg.helper() = %v, want 'sub'", res)
	}
}

func TestImportSubmoduleBindsParentAttribute(t *testing.T) {
	ctx := newImportTestContext(t)
	defer ctx.Close()

	if err := Import(ctx, "pkg2.mod"); err != nil {
		t.Fatalf("import pkg2.mod: %v", err)
	}
	// The submodule must be reachable through its parent, and registered in
	// its own right.
	if _, err := ctx.GetModule("pkg2.mod"); err != nil {
		t.Fatalf("pkg2.mod not registered: %v", err)
	}
	if got := lookup(t, ctx, "pkg2", "mod"); got == nil {
		t.Error("pkg2.mod is not bound as an attribute of pkg2")
	}
	if got := lookup(t, ctx, "pkg2.mod", "X"); got != Int(2) {
		t.Errorf("pkg2.mod.X = %v, want 2", got)
	}
}

func TestImportPlainDottedReturnsTopLevelPackage(t *testing.T) {
	ctx := newImportTestContext(t)
	defer ctx.Close()

	// "import a.b" binds the top level package, not the submodule.
	obj, err := ImportModuleLevelObject(ctx, "pkg2.mod", nil, nil, Tuple{}, 0)
	if err != nil {
		t.Fatalf("import pkg2.mod: %v", err)
	}
	mod, ok := obj.(*Module)
	if !ok {
		t.Fatalf("import pkg2.mod returned %T, want *Module", obj)
	}
	if got := string(mod.Globals["__name__"].(String)); got != "pkg2" {
		t.Errorf("import pkg2.mod bound %q, want \"pkg2\"", got)
	}
}

func TestImportFromlistBindsSubmodule(t *testing.T) {
	ctx := newImportTestContext(t)
	defer ctx.Close()

	// "from pkg2 import mod" must import and bind the submodule.
	obj, err := ImportModuleLevelObject(ctx, "pkg2", nil, nil, Tuple{String("mod")}, 0)
	if err != nil {
		t.Fatalf("from pkg2 import mod: %v", err)
	}
	mod, ok := obj.(*Module)
	if !ok {
		t.Fatalf("from pkg2 import mod returned %T, want *Module", obj)
	}
	if got := string(mod.Globals["__name__"].(String)); got != "pkg2" {
		t.Errorf("from pkg2 import mod bound %q, want \"pkg2\"", got)
	}
	if got := lookup(t, ctx, "pkg2", "mod"); got == nil {
		t.Error("mod was not bound onto pkg2")
	}
}

func TestImportRelativeLevel(t *testing.T) {
	ctx := newImportTestContext(t)
	defer ctx.Close()

	if err := Import(ctx, "pkg"); err != nil {
		t.Fatalf("import pkg: %v", err)
	}

	// Inside pkg, "from .sub import helper" is a level 1 import of "sub" with
	// "helper" in the fromlist, so it resolves to pkg.sub and binds helper.
	globals := StringDict{"__package__": String("pkg")}
	obj, err := ImportModuleLevelObject(ctx, "sub", globals, nil, Tuple{String("helper")}, 1)
	if err != nil {
		t.Fatalf("from .sub import helper (inside pkg): %v", err)
	}
	mod, ok := obj.(*Module)
	if !ok {
		t.Fatalf("relative import returned %T, want *Module", obj)
	}
	if got := string(mod.Globals["__name__"].(String)); got != "pkg.sub" {
		t.Errorf("relative import resolved to %q, want \"pkg.sub\"", got)
	}

	// "from . import sub" passes an empty module name and "sub" in the
	// fromlist, which yields the package itself with the submodule bound onto
	// it (this is what the IMPORT_FROM opcode then reads).
	obj, err = ImportModuleLevelObject(ctx, "", globals, nil, Tuple{String("sub")}, 1)
	if err != nil {
		t.Fatalf("from . import sub (inside pkg): %v", err)
	}
	pkgMod, ok := obj.(*Module)
	if !ok {
		t.Fatalf("relative import returned %T, want *Module", obj)
	}
	if got := string(pkgMod.Globals["__name__"].(String)); got != "pkg" {
		t.Errorf("from . import sub resolved to %q, want \"pkg\"", got)
	}
	if _, ok := pkgMod.Globals["sub"].(*Module); !ok {
		t.Errorf("from . import sub did not bind the submodule onto pkg")
	}
}

func TestImportMissingModuleRaises(t *testing.T) {
	ctx := newImportTestContext(t)
	defer ctx.Close()

	err := Import(ctx, "definitely_not_a_module")
	if err == nil {
		t.Fatal("importing a missing module should fail")
	}
	if !IsException(ImportError, err) {
		t.Errorf("got %T (%v), want ImportError", err, err)
	}
}
