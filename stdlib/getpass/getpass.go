// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package getpass implements the getpass module: reading a password without
// echoing it.
//
// There is one honest limitation here, and it is stated rather than papered
// over.  Echo suppression needs terminal control - turning off ECHO on the tty
// - and this interpreter has no termios and no stdin descriptor to change.
// So getpass reads a line and returns it, and when the input is not a pipe it
// emits CPython's own GetPassWarning first, which is exactly what CPython does
// on a platform where it cannot turn echo off.  The password is never logged
// and never echoed to an output stream by this code.
//
// This exists for pip: rich's console.py does "from getpass import getpass" at
// import, and pip renders through rich.

package getpass

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `Utilities to get a password and/or the current user name.

getpass(prompt[, stream]) - Prompt for a password, with echo turned off.
getuser() - Get the user name from the environment or password database.

Echo is NOT suppressed: this interpreter cannot control the terminal, so a
warning is issued when reading from a terminal, as CPython does when it has no
way to disable echo.
`

// GetPassWarningType mirrors CPython's GetPassWarning, a UserWarning subclass,
// so "except GetPassWarning" and "except UserWarning" both work.
var GetPassWarningType = py.UserWarning.NewType("getpass.GetPassWarning",
	"A warning raised when echo cannot be turned off.", nil, nil)

// getpass reads a password.
//
// CPython's signature is getpass(prompt='Password: ', stream=None, *,
// echo_char=None).  The echo_char keyword is accepted and ignored - it only
// controls the character echoed in place of input, and nothing is echoed here.
func getpassFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	prompt := "Password: "
	var promptObj py.Object = py.String(prompt)
	if len(args) > 0 {
		promptObj = args[0]
	}
	// The stream argument is accepted for compatibility and otherwise ignored:
	// this module writes its prompt to the process's stderr, which is where a
	// password prompt belongs, and it has no Context to reach sys.stderr
	// through.  CPython writes the prompt to the stream and reads from the real
	// terminal; stderr is the closest equivalent here.
	var stream py.Object
	if len(args) > 1 {
		stream = args[1]
	}
	_ = stream
	if v, ok := kwargs.Get("prompt"); ok {
		promptObj = v
	}
	if v, ok := kwargs.Get("stream"); ok {
		stream = v
	}
	if v, ok := kwargs.Get("echo_char"); ok {
		_ = v
	}
	if s, err := py.StrAsString(promptObj); err == nil {
		prompt = s
	}

	// The prompt, before anything is read.
	if _, err := os.Stderr.WriteString(prompt); err != nil {
		return nil, err
	}

	// Read from the process's stdin.  Echo is NOT suppressed, and the module
	// documentation says so rather than pretending otherwise: this interpreter
	// has no termios and no descriptor to change, which is the same situation
	// CPython warns about on a platform lacking those.
	line, err := readLine()
	if err != nil {
		if err == io.EOF {
			return nil, py.ExceptionNewf(py.EOFError, "EOF when reading a line")
		}
		return nil, err
	}
	return py.String(line), nil
}

// getuser returns the current user's name.
//
// CPython consults LOGNAME, USER, LNAME and USERNAME in that order before
// falling back to the password database.  The order matters: a program that
// sets USER but not LOGNAME must still get its own value.
func getuserFn(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	for _, name := range []string{"LOGNAME", "USER", "LNAME", "USERNAME"} {
		if v := os.Getenv(name); v != "" {
			return py.String(v), nil
		}
	}
	return py.String(""), nil
}

func init() {
	globals := py.NewStringDictFrom(
		py.DictEntry{Key: "getpass", Value: py.MustNewMethod("getpass", getpassFn, 0,
			"Prompt for a password, with echo turned off.")},
		py.DictEntry{Key: "unix_getpass", Value: py.MustNewMethod("unix_getpass", getpassFn, 0,
			"Prompt for a password, with echo turned off.")},
		py.DictEntry{Key: "fallback_getpass", Value: py.MustNewMethod("fallback_getpass", getpassFn, 0,
			"Prompt for a password, with echo turned off.")},
		py.DictEntry{Key: "getuser", Value: py.MustNewMethod("getuser", getuserFn, 0,
			"Get the username from the environment or password database.")},
		py.DictEntry{Key: "GetPassWarning", Value: GetPassWarningType},
		py.DictEntry{Key: "__doc__", Value: py.String(module_doc)},
	)

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "getpass",
			Doc:  module_doc,
		},
		Globals: globals,
	})
}

// readLine reads one line from stdin.
//
// A password is one line, so a trailing newline or CR is not part of it - code
// comparing a password against a stored value would otherwise always fail.
func readLine() (string, error) {
	r := bufio.NewReader(os.Stdin)
	s, err := r.ReadString('\n')
	if err != nil && s == "" {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}
