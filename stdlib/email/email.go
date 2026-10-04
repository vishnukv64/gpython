// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package email

import (
	"github.com/vishnukv64/gpython/py"
)

const email_doc = `email - package for parsing, handling, and generating email messages.

The submodules implemented here are email.errors (the exception and defect
hierarchy), email.utils (address and date helpers) and email.parser (an RFC
5322 parser building email.message.Message objects).`

func init() {
	globals := py.NewStringDict()
	// __path__ marks this as a package, which is what lets a "from email
	// import errors" reach the natively registered submodule.
	path := py.NewListFromItems(nil)
	globals.Set("__path__", path)
	globals.Set("__all__", py.NewListFromItems([]py.Object{
		py.String("errors"), py.String("utils"), py.String("parser"),
		py.String("message"), py.String("header"),
	}))
	// CPython defines message_from_* in email/__init__.py rather than in
	// email.parser, and callers use both spellings; they are registered here
	// and in email.parser so "email.message_from_string" works.
	globals.Set("message_from_string", py.MustNewMethod("message_from_string", messageFromString, 0,
		"Parse a message from a string."))
	globals.Set("message_from_bytes", py.MustNewMethod("message_from_bytes", messageFromBytes, 0,
		"Parse a message from bytes."))
	globals.Set("message_from_file", py.MustNewMethod("message_from_file", messageFromFile, 0,
		"Parse a message from an open text file."))
	globals.Set("message_from_binary_file", py.MustNewMethod("message_from_binary_file", messageFromBinaryFile, 0,
		"Parse a message from an open binary file."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "email",
			Doc:  email_doc,
		},
		Globals: globals,
	})
}
