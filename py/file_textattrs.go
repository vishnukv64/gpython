// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

// The io.TextIOWrapper attributes of a text stream.  Terminal libraries (cleo,
// rich, click) read sys.stdout.encoding to decide whether to draw UTF-8.  A
// File does its own UTF-8 coding and never translates errors, so the values
// are fixed; a binary file has none of them.
func init() {
	textAttr := func(name, doc string, val Object) {
		FileType.Dict.Set(name, &Property{
			Fget: func(self Object) (Object, error) {
				if f, ok := self.(*File); ok && f.Can(FileBinary) {
					return nil, ExceptionNewf(AttributeError, "'file' object has no attribute '%s'", name)
				}
				return val, nil
			},
			Doc: doc,
		})
	}
	textAttr("encoding", "Encoding of the text stream.", String("utf-8"))
	textAttr("errors", "Error handler of the text stream.", String("strict"))
	textAttr("line_buffering", "Whether the stream flushes on newline.", False)
	// reconfigure accepts the TextIOWrapper keywords; the stream's coding is fixed.
	FileType.Dict.Set("reconfigure", MustNewMethod("reconfigure", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		return None, nil
	}, 0, "reconfigure(*, encoding=None, errors=None, ...) -> None; accepted and ignored."))
}
