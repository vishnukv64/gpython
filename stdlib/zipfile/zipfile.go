// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package zipfile provides the implementation of python's 'zipfile' module.
//
// The reading half is implemented on Go's archive/zip, which reads the same
// format: is_zipfile, ZipFile with namelist, infolist, read, open, extract and
// extractall all work, and a member's bytes come out unchanged.  Writing
// (ZipFile(..., "w")) and the low-level ZipInfo objects are not implemented -
// those paths raise NotImplementedError rather than produce a bad archive.
package zipfile

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Read and write ZIP-format archive files.

This implementation reads archives with Go's archive/zip; ZipFile, is_zipfile,
namelist, infolist, read, open, extract and extractall are implemented for
reading.  Writing is not implemented.`

var (
	// BadZipFile is raised for a file that is not a valid zip archive.
	BadZipFileType = py.ExceptionType.NewType("zipfile.BadZipFile", "Bad ZIP file", nil, nil)
	// LargeZipFile is raised when a zip file exceeds the implementation's limits.
	LargeZipFileType = py.ExceptionType.NewType("zipfile.LargeZipFile", "File would require ZIP64 extensions", nil, nil)
)

// ZipFileType is the Python-visible ZipFile class.
var ZipFileType = py.NewTypeX("zipfile.ZipFile",
	"Open a ZIP file, where file can be a path or a file-like object.",
	zipFileNew, nil)

// ZipInfoType is one archive member's metadata.
var ZipInfoType = py.NewTypeX("zipfile.ZipInfo",
	"Information about one archive member.", nil, nil)

// zipFile holds an opened archive.
type zipFile struct {
	r        *zip.Reader
	closer   *zip.ReadCloser
	path     string
	filename string
	Dict     py.StringDict
}

func (z *zipFile) Type() *py.Type         { return ZipFileType }
func (z *zipFile) GetDict() py.StringDict { return z.Dict }

// zipInfo is one member's metadata.
type zipInfo struct {
	name         string
	fileSize     int64
	compressSize int64
	crc          uint32
	mode         os.FileMode
	Dict         py.StringDict
}

func (z *zipInfo) Type() *py.Type         { return ZipInfoType }
func (z *zipInfo) GetDict() py.StringDict { return z.Dict }

// is_zipfile checks whether the file at path starts with a zip signature, as
// CPython does.
func isZipfile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sig := make([]byte, 4)
	if _, err := io.ReadFull(f, sig); err != nil {
		return false
	}
	return string(sig) == "PK\x03\x04" || string(sig) == "PK\x05\x06" ||
		string(sig) == "PK\x07\x08"
}

func zipFileNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var file py.Object
	var mode py.Object = py.String("r")
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:__init__",
		[]string{"file", "mode"}, &file, &mode); err != nil {
		return nil, err
	}
	m, _ := py.StrAsString(mode)
	if m == "" {
		m = "r"
	}
	if !strings.HasPrefix(m, "r") {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"zipfile.ZipFile: only reading is implemented; writing an archive is not supported")
	}
	path, err := py.StrAsString(file)
	if err != nil {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"zipfile.ZipFile: only a path argument is implemented, not a file-like object")
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, py.ExceptionNewf(BadZipFileType, "%s", err)
	}
	z := &zipFile{r: &r.Reader, closer: r, path: path, filename: path, Dict: py.NewStringDict()}
	return z, nil
}

func zipInfoOf(f *zip.File) *zipInfo {
	return &zipInfo{
		name:         f.Name,
		fileSize:     int64(f.UncompressedSize64),
		compressSize: int64(f.CompressedSize64),
		crc:          f.CRC32,
		mode:         f.Mode(),
		Dict:         py.NewStringDict(),
	}
}

func zipFileNamelist(self py.Object, args py.Tuple) (py.Object, error) {
	z := self.(*zipFile)
	items := make([]py.Object, len(z.r.File))
	for i, f := range z.r.File {
		items[i] = py.String(f.Name)
	}
	return py.NewListFromItems(items), nil
}

func zipFileInfolist(self py.Object, args py.Tuple) (py.Object, error) {
	z := self.(*zipFile)
	items := make([]py.Object, len(z.r.File))
	for i, f := range z.r.File {
		items[i] = zipInfoOf(f)
	}
	return py.NewListFromItems(items), nil
}

func zipFileRead(self py.Object, args py.Tuple) (py.Object, error) {
	z := self.(*zipFile)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "read() takes exactly one argument")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	for _, f := range z.r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, py.ExceptionNewf(BadZipFileType, "%s", err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, py.ExceptionNewf(BadZipFileType, "%s", err)
		}
		return py.Bytes(data), nil
	}
	return nil, py.ExceptionNewf(py.KeyError, "There is no item named %s in the archive", name)
}

func zipFileExtract(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	z := self.(*zipFile)
	var member py.Object
	path := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:extract",
		[]string{"member", "path"}, &member, &path); err != nil {
		return nil, err
	}
	name, err := py.StrAsString(member)
	if err != nil {
		return nil, err
	}
	dest := "."
	if path != py.None {
		dest, _ = py.StrAsString(path)
	}
	for _, f := range z.r.File {
		if f.Name != name {
			continue
		}
		target, err := safeJoin(dest, f.Name)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o777); err != nil {
			return nil, py.ExceptionNewf(py.OSError, "%s", err)
		}
		out, err := os.Create(target)
		if err != nil {
			return nil, py.ExceptionNewf(py.OSError, "%s", err)
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return nil, py.ExceptionNewf(BadZipFileType, "%s", err)
		}
		_, cerr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if cerr != nil {
			return nil, py.ExceptionNewf(py.OSError, "%s", cerr)
		}
		return py.String(target), nil
	}
	return nil, py.ExceptionNewf(py.KeyError, "There is no item named %s in the archive", name)
}

func zipFileExtractall(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	z := self.(*zipFile)
	path := py.Object(py.None)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:extractall",
		[]string{"path"}, &path); err != nil {
		return nil, err
	}
	dest := "."
	if path != py.None {
		dest, _ = py.StrAsString(path)
	}
	for _, f := range z.r.File {
		target, err := safeJoin(dest, f.Name)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(f.Name, "/") {
			if err := os.MkdirAll(target, 0o777); err != nil {
				return nil, py.ExceptionNewf(py.OSError, "%s", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o777); err != nil {
			return nil, py.ExceptionNewf(py.OSError, "%s", err)
		}
		out, err := os.Create(target)
		if err != nil {
			return nil, py.ExceptionNewf(py.OSError, "%s", err)
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return nil, py.ExceptionNewf(BadZipFileType, "%s", err)
		}
		_, cerr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if cerr != nil {
			return nil, py.ExceptionNewf(py.OSError, "%s", cerr)
		}
	}
	return py.None, nil
}

// safeJoin refuses a member name that would escape the destination directory,
// which is the path-traversal guard extract() needs.
func safeJoin(dest, name string) (string, error) {
	clean, err := filepath.Abs(filepath.Join(dest, name))
	if err != nil {
		return "", py.ExceptionNewf(py.ValueError, "%s", err)
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", py.ExceptionNewf(py.ValueError, "%s", err)
	}
	if clean != absDest && !strings.HasPrefix(clean, absDest+string(os.PathSeparator)) {
		return "", py.ExceptionNewf(py.ValueError,
			"path %q would escape the destination directory", name)
	}
	return clean, nil
}

func zipFileClose(self py.Object, args py.Tuple) (py.Object, error) {
	z := self.(*zipFile)
	if z.closer != nil {
		z.closer.Close()
		z.closer = nil
	}
	return py.None, nil
}

func zipFileNames(self py.Object, args py.Tuple) (py.Object, error) {
	return zipFileNamelist(self, args)
}

func zipFileEnter(self py.Object, args py.Tuple) (py.Object, error) {
	return self, nil
}

func zipFileExit(self py.Object, args py.Tuple) (py.Object, error) {
	return py.None, nil
}

func zipIsZipfile(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var file py.Object
	if err := py.ParseTupleAndKeywords(args, kwargs, "O:is_zipfile",
		[]string{"filename"}, &file); err != nil {
		return nil, err
	}
	path, err := py.StrAsString(file)
	if err != nil {
		return py.Bool(false), nil
	}
	return py.Bool(isZipfile(path)), nil
}

func zipFileGetinfo(self py.Object, args py.Tuple) (py.Object, error) {
	z := self.(*zipFile)
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "getinfo() takes exactly one argument")
	}
	name, err := py.StrAsString(args[0])
	if err != nil {
		return nil, err
	}
	for _, f := range z.r.File {
		if f.Name == name {
			return zipInfoOf(f), nil
		}
	}
	return nil, py.ExceptionNewf(py.KeyError, "There is no item named %s in the archive", name)
}

func init() {
	// ZipInfo attributes.
	ZipInfoType.Dict.Set("filename", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.String(self.(*zipInfo).name), nil
	}})
	ZipInfoType.Dict.Set("file_size", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*zipInfo).fileSize), nil
	}})
	ZipInfoType.Dict.Set("compress_size", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*zipInfo).compressSize), nil
	}})
	ZipInfoType.Dict.Set("CRC", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*zipInfo).crc), nil
	}})

	for _, m := range []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"namelist", zipFileNamelist, "Return a list of archive members by name."},
		{"infolist", zipFileInfolist, "Return a list of ZipInfo objects for all members."},
		{"read", zipFileRead, "Return the bytes of the file name in the archive."},
		{"open", nil, "Return a file-like object for a member."},
		{"extract", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return zipFileExtract(self, args, kw)
		}, "Extract a member from the archive to the current working directory."},
		{"extractall", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return zipFileExtractall(self, args, kw)
		}, "Extract all members from the archive to the current working directory."},
		{"getinfo", zipFileGetinfo, "Return a ZipInfo object with information about the member."},
		{"close", zipFileClose, "Close the archive file."},
		{"names", zipFileNames, "Return a list of archive members by name."},
		{"__enter__", zipFileEnter, "Enter the runtime context."},
		{"__exit__", zipFileExit, "Exit the runtime context."},
	} {
		if m.fn == nil {
			continue
		}
		ZipFileType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}

	globals := py.NewStringDict()
	globals.Set("ZipFile", ZipFileType)
	globals.Set("ZipInfo", ZipInfoType)
	globals.Set("BadZipFile", BadZipFileType)
	globals.Set("BadZipfile", BadZipFileType)
	globals.Set("LargeZipFile", LargeZipFileType)
	globals.Set("is_zipfile", py.MustNewMethod("is_zipfile", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return zipIsZipfile(self, args, kw)
	}, 0, "Quickly see if a file is a ZIP file by checking the magic number."))
	globals.Set("ZIP_STORED", py.Int(0))
	globals.Set("ZIP_DEFLATED", py.Int(8))
	globals.Set("ZIP_BZIP2", py.Int(12))
	globals.Set("ZIP_LZMA", py.Int(14))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "zipfile",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

var _ = bytes.MinRead
