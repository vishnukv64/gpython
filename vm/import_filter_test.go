package vm_test

import (
	"fmt"
	"testing"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/pytest"
)

// The host refuses network modules through ContextOpts.ImportFilter. The
// snippet tries each import twice: once plainly, then after clearing
// sys.meta_path and dropping sys.modules entries, the bypass that defeats a
// meta_path finder. The refusal must hold both times; allowed modules must not
// change.
const importFilterSrc = `
import sys

def attempt(src):
    try:
        exec(src, {})
    except ImportError as e:
        return "refused" if "not allowed" in str(e) else "other:" + str(e)
    return "allowed"

cases = [
    "import urllib",
    "import urllib.request",
    "from urllib import request",
    "__import__('socket')",
    "import importlib; importlib.import_module('ssl')",
    "import json",
]
plain = [attempt(c) for c in cases]

sys.meta_path.clear()
for name in ["urllib", "urllib.request", "socket", "ssl"]:
    sys.modules.pop(name, None)
tampered = [attempt(c) for c in cases]

result = "|".join(plain) + "#" + "|".join(tampered)
`

func TestImportFilterRefusesDottedAndTamperedImports(t *testing.T) {
	denied := map[string]bool{"urllib": true, "socket": true, "ssl": true}
	opts := py.DefaultContextOpts()
	opts.ImportFilter = func(name string) error {
		if denied[name] {
			return fmt.Errorf("import of %q is not allowed in this context", name)
		}
		return nil
	}
	ctx := py.NewContext(opts)
	defer ctx.Close()

	module, code := pytest.CompileSrc(t, ctx, importFilterSrc, "importfilter")
	if _, err := ctx.RunCode(code, module.Globals, module.Globals, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := module.Globals.Get("result")
	want := py.String("refused|refused|refused|refused|refused|allowed#refused|refused|refused|refused|refused|allowed")
	if got != want {
		t.Fatalf("result = %v, want %v", got, want)
	}
}

// A context without a filter is unaffected: the same names import normally.
func TestImportFilterNilAllowsEverything(t *testing.T) {
	ctx := py.NewContext(py.DefaultContextOpts())
	defer ctx.Close()
	module, code := pytest.CompileSrc(t, ctx, "import urllib.request\nresult = 'ok'\n", "nofilter")
	if _, err := ctx.RunCode(code, module.Globals, module.Globals, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := module.Globals.Get("result"); got != py.String("ok") {
		t.Fatalf("result = %v, want ok", got)
	}
}

// A host restricts a module in ContextOpts.ModuleLoaded. The restriction must
// reach the fresh instance a snippet gets by deleting the name from sys.modules
// and importing it again, and the one importlib.reload returns.
func TestModuleLoadedSeesEveryInstance(t *testing.T) {
	opts := py.DefaultContextOpts()
	opts.ModuleLoaded = func(m *py.Module) error {
		if m.ModuleImpl.Info.Name == "os" {
			m.Globals.Set("mkdir", py.String("guarded"))
		}
		return nil
	}
	ctx := py.NewContext(opts)
	defer ctx.Close()

	src := `
import sys, importlib, os
first = os.mkdir
del sys.modules['os']
import os
second = os.mkdir
third = importlib.reload(os).mkdir
result = "|".join([first, second, third])
`
	module, code := pytest.CompileSrc(t, ctx, src, "moduleloaded")
	if _, err := ctx.RunCode(code, module.Globals, module.Globals, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := module.Globals.Get("result"); got != py.String("guarded|guarded|guarded") {
		t.Fatalf("result = %v, want every instance guarded", got)
	}
}

// A module loaded from a .py file is reported once its body has run, so the
// host can restrict the names the body defined.
func TestModuleLoadedSeesFileBackedModuleBody(t *testing.T) {
	sawGlob := false
	opts := py.DefaultContextOpts()
	opts.ModuleLoaded = func(m *py.Module) error {
		if m.ModuleImpl.Info.Name == "glob" && m.Globals.Has("glob") {
			sawGlob = true
		}
		return nil
	}
	ctx := py.NewContext(opts)
	defer ctx.Close()
	module, code := pytest.CompileSrc(t, ctx, "import glob
", "filebacked")
	if _, err := ctx.RunCode(code, module.Globals, module.Globals, nil); err != nil {
		t.Fatal(err)
	}
	if !sawGlob {
		t.Fatal("ModuleLoaded never saw glob with its body's names defined")
	}
}
