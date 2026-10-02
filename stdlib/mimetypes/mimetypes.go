// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mimetypes provides the implementation of python's 'mimetypes'
// module.
//
// Only the pure-python, table-driven half is implemented: the inittab mapping
// of common types is built in, and guess_type/guess_extension/guess_all_
// extensions answer from it.  The on-disk databases (mime.types, the Windows
// registry) are not read, so init() and the read_* functions exist but do not
// consult anything external - which is what urllib3's use of guess_type needs.
package mimetypes

import (
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Guess the MIME type of a file.

This module is a small, self-contained implementation: the inittab mapping of
common extensions to types is built in, and the on-disk databases are not read.
`

// encodings maps the special suffix encodings to their content-type suffix.
var encodingsMap = map[string]string{
	"gzip":     "gzip",
	"bzip2":    "x-bzip2",
	"compress": "x-compress",
	"xz":       "x-xz",
	"br":       "brotli",
}

// suffixMap maps a filename suffix to the content-type (and, when the suffix
// is a compression suffix, the suffix itself).
var suffixMap = map[string]string{
	".gz":   "gzip",
	".tgz":  "gzip",
	".bz2":  "x-bzip2",
	".tbz2": "x-bzip2",
	".Z":    "x-compress",
	".xz":   "x-xz",
	".br":   "brotli",
	".zip":  "x-zip",
	".tar":  "x-tar",
}

// inittab is the built-in extension -> type table, matching CPython's common
// entries.
var inittab = map[string]string{
	".txt":         "text/plain",
	".text":        "text/plain",
	".c":           "text/x-csrc",
	".h":           "text/x-chdr",
	".py":          "text/x-python",
	".js":          "text/javascript",
	".json":        "application/json",
	".html":        "text/html",
	".htm":         "text/html",
	".css":         "text/css",
	".xml":         "text/xml",
	".csv":         "text/csv",
	".md":          "text/markdown",
	".rst":         "text/x-rst",
	".yaml":        "text/yaml",
	".yml":         "text/yaml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".gif":         "image/gif",
	".svg":         "image/svg+xml",
	".webp":        "image/webp",
	".ico":         "image/vnd.microsoft.icon",
	".bmp":         "image/bmp",
	".tiff":        "image/tiff",
	".pdf":         "application/pdf",
	".zip":         "application/zip",
	".gz":          "application/gzip",
	".tar":         "application/x-tar",
	".mp3":         "audio/mpeg",
	".wav":         "audio/x-wav",
	".ogg":         "audio/ogg",
	".mp4":         "video/mp4",
	".mpeg":        "video/mpeg",
	".avi":         "video/x-msvideo",
	".bin":         "application/octet-stream",
	".exe":         "application/x-msdos-program",
	".sh":          "application/x-sh",
	".rtf":         "application/rtf",
	".doc":         "application/msword",
	".xls":         "application/vnd.ms-excel",
	".ppt":         "application/vnd.ms-powerpoint",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".ttf":         "font/ttf",
	".otf":         "font/otf",
	".wasm":        "application/wasm",
	".webmanifest": "application/manifest+json",
}

// guessType implements guess_type(url, strict=True), returning
// (type, encoding) with either possibly None.
func guessType(url string, strict bool) (py.Object, py.Object) {
	// Strip a query/fragment the way CPython does, and take the last path
	// component.
	base := url
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
		base = base[i+1:]
	}
	ext := strings.ToLower(extOf(base))
	var encoding py.Object = py.None
	if enc, ok := suffixMap[ext]; ok || ext == ".Z" {
		encoding = py.String(enc)
		// Strip the compression suffix and re-look-up.
		base = base[:len(base)-len(ext)]
		ext = strings.ToLower(extOf(base))
	}
	if t, ok := inittab[ext]; ok {
		return py.String(t), encoding
	}
	if encoding != py.None {
		return py.String("application/octet-stream"), encoding
	}
	return py.None, py.None
}

// extOf returns the extension of a name, including the leading dot.
func extOf(name string) string {
	i := strings.LastIndex(name, ".")
	if i <= 0 {
		return ""
	}
	return name[i:]
}

func mimetypesGuessType(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var url py.Object
	var strict py.Object = py.True
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:guess_type",
		[]string{"url", "strict"}, &url, &strict); err != nil {
		return nil, err
	}
	s, err := py.StrAsString(url)
	if err != nil {
		return nil, err
	}
	t, enc := guessType(s, truthy(strict))
	return py.Tuple{t, enc}, nil
}

func mimetypesGuessExtension(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var typ py.Object
	var strict py.Object = py.True
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:guess_extension",
		[]string{"type", "strict"}, &typ, &strict); err != nil {
		return nil, err
	}
	t, err := py.StrAsString(typ)
	if err != nil {
		return nil, err
	}
	// A reverse lookup over the built-in table.
	var best string
	for ext, et := range inittab {
		if et == t && (best == "" || ext < best) {
			best = ext
		}
	}
	if best == "" {
		return py.None, nil
	}
	return py.String(best), nil
}

func mimetypesGuessAllExtensions(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var typ py.Object
	var strict py.Object = py.True
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|O:guess_all_extensions",
		[]string{"type", "strict"}, &typ, &strict); err != nil {
		return nil, err
	}
	t, err := py.StrAsString(typ)
	if err != nil {
		return nil, err
	}
	var exts []py.Object
	for ext, et := range inittab {
		if et == t {
			exts = append(exts, py.String(ext))
		}
	}
	return py.NewListFromItems(exts), nil
}

func truthy(v py.Object) bool {
	if v == nil || v == py.None {
		return false
	}
	b, err := py.ObjectIsTrue(v)
	if err != nil {
		return false
	}
	return b
}

func init() {
	typesMap := py.NewStringDict()
	for ext, t := range inittab {
		typesMap.Set(ext, py.String(t))
	}
	suffixMapPy := py.NewStringDict()
	for suffix, enc := range suffixMap {
		suffixMapPy.Set(suffix, py.String(enc))
	}
	encodingsPy := py.NewStringDict()
	for enc, val := range encodingsMap {
		encodingsPy.Set(enc, py.String(val))
	}

	globals := py.NewStringDict()
	globals.Set("types_map", typesMap)
	globals.Set("suffix_map", suffixMapPy)
	globals.Set("encodings_map", encodingsPy)
	globals.Set("common_types", typesMap)
	globals.Set("guess_type", py.MustNewMethod("guess_type", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return mimetypesGuessType(self, args, kw)
	}, 0, "Guess the type of a file based on its URL."))
	globals.Set("guess_extension", py.MustNewMethod("guess_extension", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return mimetypesGuessExtension(self, args, kw)
	}, 0, "Guess the extension for a file based on its MIME type."))
	globals.Set("guess_all_extensions", py.MustNewMethod("guess_all_extensions", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return mimetypesGuessAllExtensions(self, args, kw)
	}, 0, "Guess the extensions for a file based on its MIME type."))
	// The database functions exist so callers can call them, but no on-disk
	// database is read.
	globals.Set("init", py.MustNewMethod("init", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return py.None, nil
	}, 0, "Load the on-disk databases.  No database is read by this implementation."))
	globals.Set("read_mime_types", py.MustNewMethod("read_mime_types", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return py.None, nil
	}, 0, "Load the type map from a given file.  Not implemented: no database is read."))
	globals.Set("add_type", py.MustNewMethod("add_type", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		var typ, ext py.Object = nil, nil
		if err := py.ParseTupleAndKeywords(args, kw, "s|s:add_type", []string{"type", "ext"}, &typ, &ext); err != nil {
			return nil, err
		}
		return py.None, nil
	}, 0, "Add a mapping from a MIME type to an extension or list of extensions."))
	globals.Set("inited", py.False)
	globals.Set("_db", py.None)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "mimetypes",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
