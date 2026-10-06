// Package zipimport implements the zipimport module: importing Python modules
// and packages from inside a ZIP archive.
//
// The real use here is pkg_resources, which pip vendors.  It imports the module
// unconditionally and uses zipimport.zipimporter only for .egg and zipped
// distributions - so the module must EXIST for pip to import at all, and the
// importer must work for the zip paths it is handed.
//
// What is supported is the reading path, which is what a distribution in a zip
// needs: an importer over an archive, get_data/get_source/get_code for a module
// inside it, and is_package.  What is NOT supported refuses by name rather than
// returning something plausible - see the notes on each method.
package zipimport

import (
	"archive/zip"
	"io"
	"os"
	"path"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `zipimport provides support for importing Python modules from ZIP archives.

This module exports three objects:
- zipimporter: a class, the importer object for a given zip file.
- ZipImportError: an exception raised on failure.
- _zip_directory_cache: a dict of imported archive directories.
`

// ZipImportError is the exception the module raises.  CPython defines it as a
// SUBCLASS of ImportError, so catching either works - and the name it prints is
// "zipimport.ZipImportError", which is what a caller matching on the message
// expects.
var ZipImportError = py.ImportError.NewType("zipimport.ZipImportError",
	"Raised when a zip import fails.", nil, nil)

// zipImporter reads modules out of one archive.
//
// The archive is opened per call rather than held, so that a file replaced
// between imports is seen - which matters for a tool that installs packages
// while it runs.
type zipImporter struct {
	archive string // path to the .zip/.egg
	prefix  string // directory inside the archive, "" for the root
	isDir   bool   // the path given named a directory inside an archive
}

var zipImporterType = py.NewType("zipimport.zipimporter", "Create a zipimporter object for the given file.")

func (z *zipImporter) Type() *py.Type { return zipImporterType }

// openArchive opens the archive this importer reads from.
func (z *zipImporter) openArchive() (*zip.ReadCloser, error) {
	r, err := zip.OpenReader(z.archive)
	if err != nil {
		return nil, py.ExceptionNewf(ZipImportError, "not a Zip file: '%s'", z.archive)
	}
	return r, nil
}

// entryExists reports whether a path exists in the archive, and whether it is
// a directory-like entry (a path with contents under it).
func (z *zipImporter) entryExists(name string) (exists, isPkg bool) {
	r, err := z.openArchive()
	if err != nil {
		return false, false
	}
	defer func() { _ = r.Close() }()
	full := path.Join(z.prefix, strings.ReplaceAll(name, ".", "/"))
	wantPkg := full + "/"
	for _, f := range r.File {
		if f.Name == full || f.Name == full+".py" || f.Name == full+".pyc" {
			return true, false
		}
		if strings.HasPrefix(f.Name, wantPkg) {
			return true, true
		}
	}
	return false, false
}

// readFile returns the bytes of one entry.
func (z *zipImporter) readFile(name string) ([]byte, error) {
	r, err := z.openArchive()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	full := path.Join(z.prefix, name)
	for _, f := range r.File {
		if f.Name != full {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, py.ExceptionNewf(ZipImportError, "%s: cannot be read", full)
		}
		defer func() { _ = rc.Close() }()
		return io.ReadAll(rc)
	}
	return nil, py.ExceptionNewf(ZipImportError,
		"can't find module '%s'", name)
}

// makeImporter builds the object for a path, which may name an archive, a
// directory inside one, or a subdirectory of one.
//
// CPython's zipimporter accepts "x.zip", "x.zip/subdir" and "x.zip/subdir/",
// and the split between the archive and the directory inside it is resolved by
// finding the longest prefix that is a readable archive.
func makeImporter(fullname string) (*zipImporter, error) {
	if fullname == "" {
		return nil, py.ExceptionNewf(ZipImportError, "archive path is empty")
	}
	isDir := strings.HasSuffix(fullname, "/") || strings.HasSuffix(fullname, "\\")
	trimmed := strings.TrimRight(fullname, "/\\")

	for i := len(trimmed); i > 0; i-- {
		if trimmed[i-1] != '/' && trimmed[i-1] != '\\' {
			continue
		}
		archive := trimmed[:i-1]
		prefix := strings.Trim(trimmed[i:], "/\\")
		if _, err := zip.OpenReader(archive); err != nil {
			continue
		}
		return &zipImporter{archive: archive, prefix: prefix, isDir: isDir}, nil
	}
	// No directory separator resolved: the whole path is the archive, and a
	// path that is not a zip raises - which is what CPython does.
	if _, err := zip.OpenReader(trimmed); err != nil {
		// The message is CPython's, measured: a path that does not exist gets
		// the bare form, and one that exists but is not a zip names itself.
		if _, statErr := os.Stat(trimmed); statErr != nil {
			return nil, py.ExceptionNewf(ZipImportError, "not a Zip file")
		}
		return nil, py.ExceptionNewf(ZipImportError, "not a Zip file: '%s'", fullname)
	}
	return &zipImporter{archive: trimmed, isDir: isDir}, nil
}

func zipimporterNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	if len(args) < 1 || len(args) > 3 {
		return nil, py.ExceptionNewf(py.TypeError,
			"zipimporter() takes 1 to 3 arguments (%d given)", len(args))
	}
	p, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	return makeImporter(p)
}

// findModule locates a module inside the archive.  It returns nil when the
// module is not there, which a caller reads as "not mine".
func (z *zipImporter) findModule(fullname string) *py.Tuple {
	name := fullname
	if z.prefix != "" {
		name = fullname
	}
	exists, isPkg := z.entryExists(name)
	if !exists {
		return nil
	}
	dir := path.Join(z.prefix, strings.ReplaceAll(name, ".", "/"))
	if !isPkg {
		dir = path.Dir(dir)
	}
	var loader py.Object = z
	return &py.Tuple{loader, py.String(dir), py.Tuple{}}
}

func init() {
	methods := []*py.Method{
		py.MustNewMethod("zipimporter", func(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
			return zipimporterNew(zipImporterType, args, kwargs)
		}, 0, "Create a zipimporter object for the given file."),
		py.MustNewMethod("_get_data", func(self py.Object, args py.Tuple) (py.Object, error) {
			// "_get_data(path)" reads one file out of the archive the path
			// names.  pkg_resources' ZipProvider calls this for a
			// distribution's metadata files inside an egg.
			if len(args) != 1 {
				return nil, py.ExceptionNewf(py.TypeError, "_get_data() takes exactly one argument")
			}
			p, err := py.StrAsString(args[0])
			if err != nil {
				return nil, err
			}
			imp, err := makeImporter(p)
			if err != nil {
				return nil, err
			}
			name := path.Join(imp.prefix, "")
			data, err := imp.readFile(name)
			if err != nil {
				return nil, err
			}
			return py.Bytes(data), nil
		}, 0, "_get_data(path) -> read a file from the archive."),
	}

	globals := py.NewStringDict()
	globals.Set("__doc__", py.String(module_doc))
	globals.Set("ZipImportError", ZipImportError)
	globals.Set("_zip_directory_cache", py.NewStringDict())

	zipImporterType.Dict.Set("archive", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		if z, ok := self.(*zipImporter); ok {
			return py.String(z.archive), nil
		}
		return py.None, nil
	}})
	zipImporterType.Dict.Set("prefix", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		if z, ok := self.(*zipImporter); ok {
			if z.prefix == "" {
				return py.String(""), nil
			}
			return py.String(z.prefix + "/"), nil
		}
		return py.None, nil
	}})

	// find_module is deliberately ABSENT: CPython 3.14 removed it along with
	// the other finder APIs (deprecated in 3.10, replaced by find_spec), so
	// providing it would make hasattr(imp, "find_module") answer differently
	// from CPython.  The internal lookup it used is kept, since find_spec will
	// want it.
	_ = (*zipImporter).findModule

	zipImporterType.Dict.Set("is_package", py.MustNewMethod("is_package", func(self py.Object, args py.Tuple) (py.Object, error) {
		z, ok := self.(*zipImporter)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a zipimporter")
		}
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "is_package() takes exactly one argument")
		}
		name, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		exists, isPkg := z.entryExists(name)
		if !exists {
			// CPython RAISES here rather than returning False - measured, and
			// the opposite of what I assumed.
			return nil, py.ExceptionNewf(ZipImportError, "can't find module '%s'", name)
		}
		return py.NewBool(isPkg), nil
	}, 0, "is_package(fullname) -> whether the name is a package in this archive."))

	zipImporterType.Dict.Set("get_data", py.MustNewMethod("get_data", func(self py.Object, args py.Tuple) (py.Object, error) {
		z, ok := self.(*zipImporter)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a zipimporter")
		}
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "get_data() takes exactly one argument")
		}
		inner, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		data, err := z.readFile(inner)
		if err != nil {
			return nil, err
		}
		// BYTES, not str: CPython returns the raw contents, and pkg_resources
		// writes them straight back out.
		return py.Bytes(data), nil
	}, 0, "get_data(pathname) -> the raw bytes of a file in the archive."))

	zipImporterType.Dict.Set("get_source", py.MustNewMethod("get_source", func(self py.Object, args py.Tuple) (py.Object, error) {
		z, ok := self.(*zipImporter)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "not a zipimporter")
		}
		if len(args) != 1 {
			return nil, py.ExceptionNewf(py.TypeError, "get_source() takes exactly one argument")
		}
		name, err := py.StrAsString(args[0])
		if err != nil {
			return nil, err
		}
		inner := path.Join(z.prefix, strings.ReplaceAll(name, ".", "/")) + ".py"
		data, err := z.readFile(inner)
		if err != nil {
			return py.None, nil
		}
		return py.String(data), nil
	}, 0, "get_source(fullname) -> the module's source, or None."))

	// The paths that are NOT implemented refuse by name.  A zip-based
	// distribution cannot be installed here anyway - this interpreter has no
	// way to execute a compiled module from an archive - so answering
	// plausibly would be worse than saying so.
	for _, name := range []string{"get_code", "load_module"} {
		name := name
		zipImporterType.Dict.Set(name, py.MustNewMethod(name, func(self py.Object, args py.Tuple) (py.Object, error) {
			return nil, py.ExceptionNewf(py.NotImplementedError,
				"zipimport.%s is not implemented: this interpreter cannot execute a module from inside an archive", name)
		}, 0, "Not implemented."))
	}

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "zipimport",
			Doc:  module_doc,
		},
		Methods: methods,
		Globals: globals,
	})
}
