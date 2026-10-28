// Copyright 2026 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dismod provides gpython's own dis.  It lives here, not in
// stdlib/pylib/lib, which holds CPython's Lib verbatim.
//
// CPython's dis.py reads CPython's bytecode, which gpython does not have, so
// this dis reads gpython's: opname and opmap come from the VM's own opcode
// table (vm.OpCode), and dis.py decodes Code.Code the way the interpreter
// loop does (an opcode byte, then a 16-bit argument from HAVE_ARGUMENT up,
// widened by EXTENDED_ARG).
package dismod

import (
	_ "embed"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/vm"
)

//go:embed dis.py
var disSrc string

func init() {
	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "dis", FileDesc: "dis.py", Doc: "Disassembler for gpython bytecode."},
		CodeSrc: disSrc,
		PreSetGlobals: func(g py.StringDict) {
			g.Set("__spec__", py.NewModuleSpec("dis", py.String("dis.py"), false))
			ops := py.NewList()
			for i := 0; i < 256; i++ {
				op := vm.OpCode(i)
				name := op.String()
				if op == vm.STORE_NAME {
					name = "STORE_NAME" // shares its number with HAVE_ARGUMENT
				}
				if len(name) >= 6 && name[:6] == "OpCode" {
					continue // no such opcode
				}
				ops.Append(py.Tuple{py.String(name), py.Int(i)})
			}
			g.Set("_vm_ops", ops)
			g.Set("HAVE_ARGUMENT", py.Int(int(vm.HAVE_ARGUMENT)))
			g.Set("EXTENDED_ARG", py.Int(int(vm.EXTENDED_ARG)))
		},
	})
}
