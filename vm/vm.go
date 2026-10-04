// Copyright 2018 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Python virtual machine
package vm

import (
	"github.com/vishnukv64/gpython/py"
)

//go:generate stringer -type=vmStatus,OpCode -output stringer.go

// VM status code
type vmStatus byte

// VM Status code for main loop (reason for stack unwind)
const (
	whyNot       vmStatus = iota // No error
	whyException                 // Exception occurred
	whyReturn                    // 'return' statement
	whyBreak                     // 'break' statement
	whyContinue                  // 'continue' statement
	whyYield                     // 'yield' operator
	whySilenced                  // Exception silenced by 'with'
)

// Virtual machine state
type Vm struct {
	// Current frame
	frame *py.Frame
	// Whether ext should be added to the next arg
	extended bool
	// 16 bit extension for argument for next opcode
	ext int32
	// Return value
	retval py.Object
	// VM Status code for main loop
	why vmStatus
	// Current Pending exception type, value and traceback
	curexc py.ExceptionInfo
	// Previous exception type, value and traceback
	exc py.ExceptionInfo
	// excStack remembers the exception being handled by each ENCLOSING
	// except block, so that leaving an inner handler restores the outer one.
	//
	// There was no such record: POP_EXCEPT only unwound the value stack, so
	// after a nested handler finished, sys.exc_info() still reported the INNER
	// exception - and outside every handler it never cleared at all.  CPython
	// restores the previous state on handler exit and reports (None, None,
	// None) once no handler is active.
	excStack []py.ExceptionInfo
	// VM access to state / modules
	context py.Context
}
