// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package entities provides python's 'html.entities' submodule: the HTML5
// named character reference table and the older HTML4 tables.
package entities

import (
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `This module defines four dictionaries, html5, name2codepoint,
codepoint2name, and entitydefs.`

func init() {
	globals := py.NewStringDict()

	html5 := py.NewStringDictSized(len(html5Data))
	for k, v := range html5Data {
		html5.Set(k, py.String(v))
	}
	globals.Set("html5", html5)

	name2codepoint := py.NewStringDictSized(len(name2codepointData))
	for k, v := range name2codepointData {
		name2codepoint.Set(k, py.Int(v))
	}
	globals.Set("name2codepoint", name2codepoint)

	codepoint2name := py.NewStringDictSized(len(codepoint2nameData))
	for k, v := range codepoint2nameData {
		// codepoint2name is keyed by code point, so the key is an int; setItem
		// handles the encoding and lookup of a non-string key.
		if _, err := codepoint2name.M__setitem__(py.Int(k), py.String(v)); err != nil {
			panic(err)
		}
	}
	globals.Set("codepoint2name", codepoint2name)

	entitydefs := py.NewStringDictSized(len(entitydefsData))
	for k, v := range entitydefsData {
		entitydefs.Set(k, py.String(v))
	}
	globals.Set("entitydefs", entitydefs)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "html.entities",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}
