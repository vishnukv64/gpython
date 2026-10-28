// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package zipfile provides the implementation of python's 'zipfile' module.
//
// The reading half is implemented on Go's archive/zip, which reads the same
// format: is_zipfile, ZipFile with namelist, infolist, read, open, extract and
// extractall all work, and a member's bytes come out unchanged.  Writing
// ("w"/"x") goes through archive/zip's Writer onto a path or onto any object
// with a write() method: write() and writestr() with a name.  Append mode and
// the low-level ZipInfo objects are not implemented - those paths raise
// NotImplementedError rather than produce a bad archive.
package zipfile

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
	zipio "github.com/vishnukv64/gpython/stdlib/io"
)

const module_doc = `Read and write ZIP-format archive files.

This implementation reads archives with Go's archive/zip; ZipFile, is_zipfile,
namelist, infolist, read, open, extract and extractall are implemented for
reading, and write(), writestr() for writing a new archive.`

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

	// Write mode: w is the archive being built, sink the file-like object it
	// goes to, and own the os.File to close when the archive was given as a
	// path.
	w           *zip.Writer
	sink        py.Object
	own         *os.File
	compression uint16
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
	externalAttr uint32
	method       uint16
	modified     time.Time
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

// pyWriter adapts a Python file-like object (anything with write()) to io.Writer.
type pyWriter struct{ obj py.Object }

func (w pyWriter) Write(p []byte) (int, error) {
	fn, err := py.GetAttrString(w.obj, "write")
	if err != nil {
		return 0, err
	}
	if _, err := py.Call(fn, py.Tuple{py.Bytes(append([]byte(nil), p...))}, py.StringDict{}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func zipFileNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var file py.Object
	var mode py.Object = py.String("r")
	var compression py.Object = py.Int(0)
	var allowZip64, compresslevel, strictTimestamps, metadataEncoding py.Object = py.True, py.None, py.True, py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OiOOOO:__init__",
		[]string{"file", "mode", "compression", "allowZip64", "compresslevel", "strict_timestamps", "metadata_encoding"},
		&file, &mode, &compression, &allowZip64, &compresslevel, &strictTimestamps, &metadataEncoding); err != nil {
		return nil, err
	}
	m, _ := py.StrAsString(mode)
	if m == "" {
		m = "r"
	}
	if m == "w" || m == "x" {
		return zipWriterNew(file, m, uint16(compression.(py.Int)))
	}
	if !strings.HasPrefix(m, "r") {
		return nil, py.ExceptionNewf(py.NotImplementedError,
			"zipfile.ZipFile: append mode is not implemented")
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

func zipWriterNew(file py.Object, mode string, compression uint16) (py.Object, error) {
	if compression != zip.Store && compression != zip.Deflate {
		return nil, py.ExceptionNewf(py.NotImplementedError, "zipfile: only ZIP_STORED and ZIP_DEFLATED are implemented")
	}
	z := &zipFile{Dict: py.NewStringDict(), compression: compression}
	var out io.Writer
	if _, isFile := file.(py.String); isFile || hasFSPath(file) {
		path, err := py.StrAsString(file)
		if hasFSPath(file) {
			res, _, ferr := py.TypeCall0(file, "__fspath__")
			if ferr != nil {
				return nil, ferr
			}
			path, err = py.StrAsString(res)
		}
		if err != nil {
			return nil, err
		}
		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if mode == "x" {
			flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
		}
		f, err := os.OpenFile(path, flags, 0o666)
		if err != nil {
			return nil, py.OSErrorFrom(err, path)
		}
		z.own, z.path, z.filename, out = f, path, path, f
	} else {
		z.sink, out = file, pyWriter{file}
	}
	z.w = zip.NewWriter(out)
	return z, nil
}

func hasFSPath(o py.Object) bool {
	_, isStr := o.(py.String)
	return !isStr && o.Type().Lookup("__fspath__") != nil
}

func zipInfoOf(f *zip.File) *zipInfo {
	return &zipInfo{
		name:         f.Name,
		fileSize:     int64(f.UncompressedSize64),
		compressSize: int64(f.CompressedSize64),
		crc:          f.CRC32,
		mode:         f.Mode(),
		externalAttr: f.ExternalAttrs,
		method:       f.Method,
		modified:     f.Modified,
		Dict:         py.NewStringDict(),
	}
}

func zipFileNamelist(self py.Object, args py.Tuple) (py.Object, error) {
	z := self.(*zipFile)
	if z.r == nil {
		return nil, py.ExceptionNewf(py.ValueError, "ZipFile was opened for writing; reading members is not implemented")
	}
	items := make([]py.Object, len(z.r.File))
	for i, f := range z.r.File {
		items[i] = py.String(f.Name)
	}
	return py.NewListFromItems(items), nil
}

func zipFileInfolist(self py.Object, args py.Tuple) (py.Object, error) {
	z := self.(*zipFile)
	if z.r == nil {
		return nil, py.ExceptionNewf(py.ValueError, "ZipFile was opened for writing; reading members is not implemented")
	}
	items := make([]py.Object, len(z.r.File))
	for i, f := range z.r.File {
		items[i] = zipInfoOf(f)
	}
	return py.NewListFromItems(items), nil
}

func zipFileRead(self py.Object, args py.Tuple) (py.Object, error) {
	z := self.(*zipFile)
	if z.r == nil {
		return nil, py.ExceptionNewf(py.ValueError, "ZipFile was opened for writing; reading members is not implemented")
	}
	if len(args) != 1 {
		return nil, py.ExceptionNewf(py.TypeError, "read() takes exactly one argument")
	}
	name, err := memberName(args[0])
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

// memberName is the archive name a member argument stands for: a str, or a
// ZipInfo as read(), open() and getinfo() callers pass.
func memberName(o py.Object) (string, error) {
	if zi, ok := o.(*zipInfo); ok {
		return zi.name, nil
	}
	return py.StrAsString(o)
}

// zipFileOpen is ZipFile.open for reading: the member's bytes behind a
// BytesIO, which is a readable binary file with the same read() contract.
// Archives are read whole by archive/zip anyway.
func zipFileOpen(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var name, mode, pwd py.Object
	mode = py.String("r")
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:open", []string{"name", "mode", "pwd"}, &name, &mode, &pwd); err != nil {
		return nil, err
	}
	if m, _ := py.StrAsString(mode); m != "r" {
		return nil, py.ExceptionNewf(py.NotImplementedError, "ZipFile.open supports only mode 'r'")
	}
	data, err := zipFileRead(self, py.Tuple{name})
	if err != nil {
		return nil, err
	}
	return py.Call(zipio.BytesIOType, py.Tuple{data}, py.StringDict{})
}

func zipFileExtract(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	z := self.(*zipFile)
	if z.r == nil {
		return nil, py.ExceptionNewf(py.ValueError, "ZipFile was opened for writing; reading members is not implemented")
	}
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
	if z.r == nil {
		return nil, py.ExceptionNewf(py.ValueError, "ZipFile was opened for writing; reading members is not implemented")
	}
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

func (z *zipFile) writing() error {
	if z.w == nil {
		return py.ExceptionNewf(py.ValueError, "write() requires mode 'w' or 'x'")
	}
	return nil
}

// arcname normalises a member name the way CPython's ZipInfo does: forward
// slashes, no drive or leading separator.
func arcname(name string, isDir bool) string {
	name = filepath.ToSlash(name)
	if v := filepath.VolumeName(name); v != "" {
		name = name[len(filepath.ToSlash(v)):]
	}
	name = strings.TrimLeft(name, "/")
	if isDir && !strings.HasSuffix(name, "/") {
		name += "/"
	}
	return name
}

func (z *zipFile) add(name string, mode os.FileMode, mtime time.Time, data []byte, compress py.Object) error {
	method := z.compression
	if c, ok := compress.(py.Int); ok {
		method = uint16(c)
	}
	if method != zip.Store && method != zip.Deflate {
		return py.ExceptionNewf(py.NotImplementedError, "zipfile: only ZIP_STORED and ZIP_DEFLATED are implemented")
	}
	h := &zip.FileHeader{Name: name, Method: method, Modified: mtime}
	h.SetMode(mode)
	if strings.HasSuffix(name, "/") {
		h.Method = zip.Store
	}
	w, err := z.w.CreateHeader(h)
	if err != nil {
		return py.ExceptionNewf(py.OSError, "%s", err)
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return nil
}

func zipFileWrite(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	z := self.(*zipFile)
	if err := z.writing(); err != nil {
		return nil, err
	}
	var filename py.Object
	var arc, compress py.Object = py.None, py.None
	var level py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OOO:write", []string{"filename", "arcname", "compress_type", "compresslevel"},
		&filename, &arc, &compress, &level); err != nil {
		return nil, err
	}
	path, err := fsString(filename)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, py.OSErrorFrom(err, path)
	}
	name := path
	if arc != py.None {
		if name, err = py.StrAsString(arc); err != nil {
			return nil, err
		}
	}
	name = arcname(name, st.IsDir())
	var data []byte
	if !st.IsDir() {
		if data, err = os.ReadFile(path); err != nil {
			return nil, py.OSErrorFrom(err, path)
		}
	}
	if err := z.add(name, st.Mode(), st.ModTime(), data, compress); err != nil {
		return nil, err
	}
	return py.None, nil
}

func zipFileWritestr(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	z := self.(*zipFile)
	if err := z.writing(); err != nil {
		return nil, err
	}
	var info, data py.Object
	var compress, level py.Object = py.None, py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "OO|OO:writestr", []string{"zinfo_or_arcname", "data", "compress_type", "compresslevel"},
		&info, &data, &compress, &level); err != nil {
		return nil, err
	}
	name, ok := info.(py.String)
	if !ok {
		return nil, py.ExceptionNewf(py.NotImplementedError, "zipfile: writestr() takes an archive name; ZipInfo is not implemented")
	}
	var b []byte
	switch d := data.(type) {
	case py.String:
		b = []byte(d)
	case py.Bytes:
		b = []byte(d)
	default:
		return nil, py.ExceptionNewf(py.TypeError, "writestr() data must be str or bytes, not %s", data.Type().Name)
	}
	if err := z.add(arcname(string(name), false), 0o600, time.Now(), b, compress); err != nil {
		return nil, err
	}
	return py.None, nil
}

// fsString is os.fspath for a str or os.PathLike.
func fsString(o py.Object) (string, error) {
	if hasFSPath(o) {
		res, _, err := py.TypeCall0(o, "__fspath__")
		if err != nil {
			return "", err
		}
		o = res
	}
	return py.StrAsString(o)
}

func zipFileClose(self py.Object, args py.Tuple) (py.Object, error) {
	z := self.(*zipFile)
	if z.w != nil {
		w := z.w
		z.w = nil
		if err := w.Close(); err != nil {
			return nil, err
		}
		if z.own != nil {
			if err := z.own.Close(); err != nil {
				return nil, py.OSErrorFrom(err, z.path)
			}
			z.own = nil
		}
	}
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
	if _, err := zipFileClose(self, nil); err != nil {
		return nil, err
	}
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
	if z.r == nil {
		return nil, py.ExceptionNewf(py.ValueError, "ZipFile was opened for writing; reading members is not implemented")
	}
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
	ZipInfoType.Dict.Set("orig_filename", ZipInfoType.Dict.GetOrNil("filename"))
	ZipInfoType.Dict.Set("external_attr", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*zipInfo).externalAttr), nil
	}})
	ZipInfoType.Dict.Set("compress_type", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*zipInfo).method), nil
	}})
	ZipInfoType.Dict.Set("date_time", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		t := self.(*zipInfo).modified
		return py.Tuple{py.Int(t.Year()), py.Int(int(t.Month())), py.Int(t.Day()), py.Int(t.Hour()), py.Int(t.Minute()), py.Int(t.Second())}, nil
	}})
	ZipInfoType.Dict.Set("is_dir", py.MustNewMethod("is_dir", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewBool(strings.HasSuffix(self.(*zipInfo).name, "/")), nil
	}, 0, "Return True if this archive member is a directory."))
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
		{"open", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return zipFileOpen(self, args, kw)
		}, "Return a file-like object for a member."},
		{"extract", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return zipFileExtract(self, args, kw)
		}, "Extract a member from the archive to the current working directory."},
		{"extractall", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return zipFileExtractall(self, args, kw)
		}, "Extract all members from the archive to the current working directory."},
		{"write", zipFileWrite, "Put the bytes from filename into the archive under the name arcname."},
		{"writestr", zipFileWritestr, "Write a file into the archive; the contents are data."},
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
