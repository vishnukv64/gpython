// Copyright 2022 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package errno provides the implementation of python's 'errno' module:
// the standard system error numbers and their messages.
//
// The numbers are taken from the host in Python's own source, where they are
// #defined per platform; the values here are the ones in the Python source
// listing, which are what Python code compares against.  The operating
// system wrappers in the standard library use them when they turn a failing
// syscall into an OSError.
package errno

import (
	"syscall"

	"github.com/vishnukv64/gpython/py"
)

const module_doc = `This module makes available standard errno system symbols.

The value of each symbol is the corresponding integer value,
which is the same as the value of the C errno constant.`

// errors is the table of name -> errno, taken from the host's own syscall
// package rather than transcribed.  That way the numbers and the messages are
// the ones this machine actually reports, and a name that does not exist on
// the platform cannot be invented.
type errnoEntry struct {
	name   string
	number syscall.Errno
}

var errors = []errnoEntry{
	{"EPERM", syscall.EPERM},
	{"ENOENT", syscall.ENOENT},
	{"ESRCH", syscall.ESRCH},
	{"EINTR", syscall.EINTR},
	{"EIO", syscall.EIO},
	{"ENXIO", syscall.ENXIO},
	{"E2BIG", syscall.E2BIG},
	{"ENOEXEC", syscall.ENOEXEC},
	{"EBADF", syscall.EBADF},
	{"ECHILD", syscall.ECHILD},
	{"EDEADLK", syscall.EDEADLK},
	{"ENOMEM", syscall.ENOMEM},
	{"EACCES", syscall.EACCES},
	{"EFAULT", syscall.EFAULT},
	{"ENOTBLK", syscall.ENOTBLK},
	{"EBUSY", syscall.EBUSY},
	{"EEXIST", syscall.EEXIST},
	{"EXDEV", syscall.EXDEV},
	{"ENODEV", syscall.ENODEV},
	{"ENOTDIR", syscall.ENOTDIR},
	{"EISDIR", syscall.EISDIR},
	{"EINVAL", syscall.EINVAL},
	{"ENFILE", syscall.ENFILE},
	{"EMFILE", syscall.EMFILE},
	{"ENOTTY", syscall.ENOTTY},
	{"ETXTBSY", syscall.ETXTBSY},
	{"EFBIG", syscall.EFBIG},
	{"ENOSPC", syscall.ENOSPC},
	{"ESPIPE", syscall.ESPIPE},
	{"EROFS", syscall.EROFS},
	{"EMLINK", syscall.EMLINK},
	{"EPIPE", syscall.EPIPE},
	{"EDOM", syscall.EDOM},
	{"ERANGE", syscall.ERANGE},
	{"EAGAIN", syscall.EAGAIN},
	{"EWOULDBLOCK", syscall.EWOULDBLOCK},
	{"EINPROGRESS", syscall.EINPROGRESS},
	{"EALREADY", syscall.EALREADY},
	{"ENOTSOCK", syscall.ENOTSOCK},
	{"EDESTADDRREQ", syscall.EDESTADDRREQ},
	{"EMSGSIZE", syscall.EMSGSIZE},
	{"EPROTOTYPE", syscall.EPROTOTYPE},
	{"ENOPROTOOPT", syscall.ENOPROTOOPT},
	{"EPROTONOSUPPORT", syscall.EPROTONOSUPPORT},
	{"ESOCKTNOSUPPORT", syscall.ESOCKTNOSUPPORT},
	{"ENOTSUP", syscall.ENOTSUP},
	{"EOPNOTSUPP", syscall.EOPNOTSUPP},
	{"EPFNOSUPPORT", syscall.EPFNOSUPPORT},
	{"EAFNOSUPPORT", syscall.EAFNOSUPPORT},
	{"EADDRINUSE", syscall.EADDRINUSE},
	{"EADDRNOTAVAIL", syscall.EADDRNOTAVAIL},
	{"ENETDOWN", syscall.ENETDOWN},
	{"ENETUNREACH", syscall.ENETUNREACH},
	{"ENETRESET", syscall.ENETRESET},
	{"ECONNABORTED", syscall.ECONNABORTED},
	{"ECONNRESET", syscall.ECONNRESET},
	{"ENOBUFS", syscall.ENOBUFS},
	{"EISCONN", syscall.EISCONN},
	{"ENOTCONN", syscall.ENOTCONN},
	{"ESHUTDOWN", syscall.ESHUTDOWN},
	{"ETOOMANYREFS", syscall.ETOOMANYREFS},
	{"ETIMEDOUT", syscall.ETIMEDOUT},
	{"ECONNREFUSED", syscall.ECONNREFUSED},
	{"ELOOP", syscall.ELOOP},
	{"ENAMETOOLONG", syscall.ENAMETOOLONG},
	{"EHOSTDOWN", syscall.EHOSTDOWN},
	{"EHOSTUNREACH", syscall.EHOSTUNREACH},
	{"ENOTEMPTY", syscall.ENOTEMPTY},
	{"EPROCLIM", syscall.EPROCLIM},
	{"EUSERS", syscall.EUSERS},
	{"EDQUOT", syscall.EDQUOT},
	{"ESTALE", syscall.ESTALE},
	{"EREMOTE", syscall.EREMOTE},
	{"ENOSTR", syscall.ENOSTR},
	{"ETIME", syscall.ETIME},
	{"ENOSR", syscall.ENOSR},
	{"ENOMSG", syscall.ENOMSG},
	{"EBADMSG", syscall.EBADMSG},
	{"EIDRM", syscall.EIDRM},
	{"ENOLCK", syscall.ENOLCK},
	{"ENOSYS", syscall.ENOSYS},
	{"EILSEQ", syscall.EILSEQ},
	{"EOVERFLOW", syscall.EOVERFLOW},
	{"ECANCELED", syscall.ECANCELED},
	{"EBADEXEC", syscall.EBADEXEC},
	{"EBADARCH", syscall.EBADARCH},
	{"ESHLIBVERS", syscall.ESHLIBVERS},
	{"EBADMACHO", syscall.EBADMACHO},
	{"EMULTIHOP", syscall.EMULTIHOP},
	{"ENODATA", syscall.ENODATA},
	{"ENOLINK", syscall.ENOLINK},
	{"EPROTO", syscall.EPROTO},
	{"ENOTRECOVERABLE", syscall.ENOTRECOVERABLE},
	{"EOWNERDEAD", syscall.EOWNERDEAD},
	{"ENOATTR", syscall.ENOATTR},
}

func init() {
	globals := py.StringDict{}

	// The names are module attributes so that "errno.EPIPE" works; the
	// message table is what errno.errorcode and errno.strerror use.
	errorcode := py.NewStringDict()
	strerror := map[int]string{}

	// The errorcode mapping is filled through the dict's own setitem so that
	// its keys use the same encoding a lookup from Python will produce; a
	// plain Go string key would never be found by an int key.
	for _, e := range errors {
		number := int(e.number)
		globals[e.name] = py.Int(number)
		if _, ok := strerror[number]; !ok {
			strerror[number] = e.number.Error()
		}
		if _, err := errorcode.M__setitem__(py.Int(number), py.String(e.name)); err != nil {
			panic(err)
		}
	}

	globals["errorcode"] = errorcode
	globals["strerror"] = py.MustNewMethod("strerror", func(self py.Object, args py.Tuple) (py.Object, error) {
		var code py.Object
		if err := py.UnpackTuple(args, nil, "strerror", 1, 1, &code); err != nil {
			return nil, err
		}
		n, err := py.IndexInt(code)
		if err != nil {
			return nil, err
		}
		if message, ok := strerror[n]; ok {
			return py.String(message), nil
		}
		return nil, py.ExceptionNewf(py.ValueError, "Unknown error code %d", n)
	}, 0, "strerror(code) -> the message for an errno.")

	// E* constants cover the Linux and BSD names Python exposes; the table
	// above is deliberately the superset, because a program that imports
	// errno on one platform often references another platform's constant.

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "errno",
			Doc:  moduleDoc(),
		},
		Globals: globals,
	})
}

func moduleDoc() string { return module_doc }
