// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package py

func init() {
	BytesType.Dict.Set("splitlines", MustNewMethod("splitlines", func(self Object, args Tuple, kwargs StringDict) (Object, error) {
		var keep Object = False
		if err := ParseTupleAndKeywords(args, kwargs, "|O:splitlines", []string{"keepends"}, &keep); err != nil {
			return nil, err
		}
		k, err := MakeBool(keep)
		if err != nil {
			return nil, err
		}
		b := self.(Bytes)
		var out []Object
		start := 0
		for i := 0; i < len(b); i++ {
			if b[i] != '\n' && b[i] != '\r' {
				continue
			}
			end := i
			if b[i] == '\r' && i+1 < len(b) && b[i+1] == '\n' {
				i++
			}
			if k == True {
				end = i + 1
			}
			out = append(out, Bytes(append([]byte(nil), b[start:end]...)))
			start = i + 1
		}
		if start < len(b) {
			out = append(out, Bytes(append([]byte(nil), b[start:]...)))
		}
		return NewListFromItems(out), nil
	}, 0, "Return a list of the lines in the bytes, breaking at \n, \r and \r\n."))
}
