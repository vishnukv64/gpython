// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ssl provides the implementation of python's 'ssl' module.
//
// This interpreter cannot do TLS: there is no OpenSSL binding, no certificate
// store and no handshake.  This module is therefore deliberately honest - it
// provides the NAMES and CONSTANTS that code imports and uses at import time
// (SSLContext, SSLSocket, the SSLException hierarchy, the CERT_* and PROTOCOL_*
// and OP_* constants, HAS_SNI, OPENSSL_VERSION, create_default_context,
// wrap_socket), and EVERY function or method that would perform a handshake or
// open a TLS connection raises NotImplementedError naming the limit.  Nothing
// here fakes a working connection: a silent fake would be far worse than a
// clear error, because code would believe it was talking TLS when it was not.
package ssl

import (
	"github.com/vishnukv64/gpython/py"
)

const module_doc = `TLS/SSL wrapper for socket objects.

THIS INTERPRETER CANNOT DO TLS.  The names and constants in this module exist
so that code which imports them loads, and every operation that would perform
a TLS handshake raises NotImplementedError with a message saying so.  Do not
use this module to attempt a secure connection: it will not establish one.
`

// ---------------------------------------------------------------------------
// Exceptions
// ---------------------------------------------------------------------------

var (
	// SSLError is the base class, a subtype of OSError as in CPython.
	SSLErrorType                 = py.OSError.NewType("ssl.SSLError", "An SSL error occurred.", nil, nil)
	SSLZeroReturnErrorType       = SSLErrorType.NewType("ssl.SSLZeroReturnError", "TLS/SSL connection has been closed (EOF).", nil, nil)
	SSLWantReadErrorType         = SSLErrorType.NewType("ssl.SSLWantReadError", "The operation did not complete (read).", nil, nil)
	SSLWantWriteErrorType        = SSLErrorType.NewType("ssl.SSLWantWriteError", "The operation did not complete (write).", nil, nil)
	SSLSyscallErrorType          = SSLErrorType.NewType("ssl.SSLSyscallError", "System error when attempting SSL operation.", nil, nil)
	SSLEOFErrorType              = SSLErrorType.NewType("ssl.SSLEOFError", "TLS/SSL connection has been closed (EOF).", nil, nil)
	SSLCertVerificationErrorType = SSLErrorType.NewType("ssl.SSLCertVerificationError",
		"A certificate could not be verified.", nil, nil)
)

// sslErrorType is the alias CPython keeps as ssl.SSLError's old name.
var sslErrorType = SSLErrorType

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

// These match CPython's values so code that compares against them behaves.
const (
	CERT_NONE     = 0
	CERT_OPTIONAL = 1
	CERT_REQUIRED = 2

	PROTOCOL_TLS        = 2
	PROTOCOL_TLS_CLIENT = 16
	PROTOCOL_TLS_SERVER = 17
	PROTOCOL_SSLv23     = PROTOCOL_TLS
	PROTOCOL_TLSv1      = 3
	PROTOCOL_TLSv1_1    = 4
	PROTOCOL_TLSv1_2    = 5

	OP_NO_SSLv2 = 0x00000000
	OP_NO_SSLv3 = 0x02000000
	OP_NO_TLSv1 = 0x04000000

	VERIFY_DEFAULT        = 0
	VERIFY_CRL_CHECK_LEAF = 4

	SSL_ERROR_NONE = 0
)

// notImplementedError builds the honest error every TLS-performing path raises.
func notImplementedError(what string) error {
	return py.ExceptionNewf(py.NotImplementedError,
		"ssl.%s: this interpreter cannot perform TLS - there is no OpenSSL binding "+
			"and no certificate store, so no handshake can be made.  The ssl module "+
			"provides only the names and constants needed to import; it does not "+
			"establish a secure connection.", what)
}

// ---------------------------------------------------------------------------
// SSLContext
// ---------------------------------------------------------------------------

var SSLContextType = py.NewTypeX("ssl.SSLContext",
	"An SSL context holds configuration data and certificates.\n\n"+
		"THIS INTERPRETER CANNOT DO TLS: a context can be constructed and configured, "+
		"but wrap_socket and everything that would handshake raise NotImplementedError.",
	sslContextNew, nil)

type sslContext struct {
	protocol      int
	verifyMode    int
	checkHostname py.Object
	Dict          py.StringDict
}

func (c *sslContext) Type() *py.Type         { return SSLContextType }
func (c *sslContext) GetDict() py.StringDict { return c.Dict }

func sslContextNew(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var protocol py.Object = py.Int(PROTOCOL_TLS)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:__init__",
		[]string{"protocol"}, &protocol); err != nil {
		return nil, err
	}
	p, err := py.IndexInt(protocol)
	if err != nil {
		return nil, err
	}
	return &sslContext{protocol: p, Dict: py.NewStringDict()}, nil
}

func tlsNotImplemented(self py.Object, args py.Tuple) (py.Object, error) {
	return nil, notImplementedError("SSLContext.wrap_socket")
}

func sslContextLoadVerify(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	_ = self.(*sslContext)
	var cafile py.Object = py.None
	var capath py.Object = py.None
	var cadata py.Object = py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOO:load_verify_locations",
		[]string{"cafile", "capath", "cadata"}, &cafile, &capath, &cadata); err != nil {
		return nil, err
	}
	// Reading a CA bundle is harmless and useful metadata, but there is no
	// TLS engine to verify against, so this records nothing beyond accepting
	// the call.  It does NOT claim to have loaded a store.
	return py.None, nil
}

// ---------------------------------------------------------------------------
// SSLSocket
// ---------------------------------------------------------------------------

var SSLSocketType = py.NewTypeX("ssl.SSLSocket",
	"An SSL socket.\n\nTHIS INTERPRETER CANNOT DO TLS: constructing one is refused, "+
		"because a socket with no TLS engine would be a fake.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return nil, notImplementedError("SSLSocket")
	}, nil)

// ---------------------------------------------------------------------------
// Functions
// ---------------------------------------------------------------------------

func sslCreateDefaultContext(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	var purpose py.Object = py.Int(PROTOCOL_TLS_CLIENT)
	if err := py.ParseTupleAndKeywords(args, kwargs, "|O:create_default_context",
		[]string{"purpose"}, &purpose); err != nil {
		return nil, err
	}
	p, err := py.IndexInt(purpose)
	if err != nil {
		return nil, err
	}
	// A default context is a configuration object; it can be built and its
	// settings recorded, but using it to make a connection raises.  The
	// defaults match CPython: a client context requires verification and
	// checks the host name.
	c := &sslContext{protocol: p, Dict: py.NewStringDict()}
	if p == PROTOCOL_TLS_CLIENT {
		c.verifyMode = CERT_REQUIRED
		c.checkHostname = py.True
	}
	return c, nil
}

func sslWrapSocket(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	return nil, notImplementedError("wrap_socket")
}

func sslGetDefaultVerifyPaths(self py.Object, args py.Tuple) (py.Object, error) {
	// This reports where a store WOULD be looked for; it does not read one.
	return py.Tuple{py.None, py.None, py.NewListFromItems(nil)}, nil
}

func init() {
	// SSLContext.
	SSLContextType.Dict.Set("protocol", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*sslContext).protocol), nil
	}})
	SSLContextType.Dict.Set("verify_mode", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.Int(self.(*sslContext).verifyMode), nil
		},
		Fset: func(self py.Object, v py.Object) error {
			n, err := py.IndexInt(v)
			if err != nil {
				return err
			}
			self.(*sslContext).verifyMode = n
			return nil
		},
	})
	SSLContextType.Dict.Set("check_hostname", &py.Property{
		Fget: func(self py.Object) (py.Object, error) {
			return py.Bool(truthy(self.(*sslContext).checkHostname)), nil
		},
		Fset: func(self py.Object, v py.Object) error {
			self.(*sslContext).checkHostname = v
			return nil
		},
	})
	for _, m := range []struct {
		name string
		fn   interface{}
		doc  string
	}{
		{"wrap_socket", tlsNotImplemented, "Wrap a socket in TLS.  Not implemented: no TLS engine."},
		{"load_verify_locations", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return sslContextLoadVerify(self, args, kw)
		}, "Load CA certificates.  No TLS engine exists, so nothing is verified."},
		{"set_ciphers", func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.None, nil
		}, "Set the cipher list.  Accepted and ignored: no TLS engine."},
	} {
		SSLContextType.Dict.Set(m.name, py.MustNewMethod(m.name, m.fn, 0, m.doc))
	}

	globals := py.NewStringDict()
	for name, t := range map[string]*py.Type{
		"SSLError":                 SSLErrorType,
		"SSLZeroReturnError":       SSLZeroReturnErrorType,
		"SSLWantReadError":         SSLWantReadErrorType,
		"SSLWantWriteError":        SSLWantWriteErrorType,
		"SSLSyscallError":          SSLSyscallErrorType,
		"SSLEOFError":              SSLEOFErrorType,
		"SSLCertVerificationError": SSLCertVerificationErrorType,
		"SSLContext":               SSLContextType,
		"SSLSocket":                SSLSocketType,
	} {
		globals.Set(name, t)
	}
	_ = sslErrorType

	globals.Set("CERT_NONE", py.Int(CERT_NONE))
	globals.Set("CERT_OPTIONAL", py.Int(CERT_OPTIONAL))
	globals.Set("CERT_REQUIRED", py.Int(CERT_REQUIRED))
	globals.Set("PROTOCOL_TLS", py.Int(PROTOCOL_TLS))
	globals.Set("PROTOCOL_TLS_CLIENT", py.Int(PROTOCOL_TLS_CLIENT))
	globals.Set("PROTOCOL_TLS_SERVER", py.Int(PROTOCOL_TLS_SERVER))
	globals.Set("PROTOCOL_SSLv23", py.Int(PROTOCOL_SSLv23))
	globals.Set("PROTOCOL_TLSv1", py.Int(PROTOCOL_TLSv1))
	globals.Set("PROTOCOL_TLSv1_1", py.Int(PROTOCOL_TLSv1_1))
	globals.Set("PROTOCOL_TLSv1_2", py.Int(PROTOCOL_TLSv1_2))
	globals.Set("OP_NO_SSLv2", py.Int(OP_NO_SSLv2))
	globals.Set("OP_NO_SSLv3", py.Int(OP_NO_SSLv3))
	globals.Set("OP_NO_TLSv1", py.Int(OP_NO_TLSv1))
	globals.Set("OPTIONS_ALL", py.Int(0))
	globals.Set("HAS_SNI", py.Bool(true))
	globals.Set("HAS_ALPN", py.Bool(false))
	globals.Set("HAS_TLSv1_3", py.Bool(false))
	globals.Set("OPENSSL_VERSION", py.String("Gpython has no OpenSSL"))
	globals.Set("OPENSSL_VERSION_INFO", py.Tuple{py.Int(0), py.Int(0), py.Int(0), py.Int(0), py.Int(0)})
	globals.Set("OPENSSL_VERSION_NUMBER", py.Int(0))
	globals.Set("SSL_ERROR_NONE", py.Int(SSL_ERROR_NONE))
	globals.Set("VERIFY_DEFAULT", py.Int(VERIFY_DEFAULT))
	globals.Set("VERIFY_CRL_CHECK_LEAF", py.Int(VERIFY_CRL_CHECK_LEAF))

	globals.Set("create_default_context", py.MustNewMethod("create_default_context",
		func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
			return sslCreateDefaultContext(self, args, kw)
		}, 0, "Create an SSLContext.  It cannot make a connection: no TLS engine."))
	globals.Set("wrap_socket", py.MustNewMethod("wrap_socket", sslWrapSocket, 0,
		"Wrap a socket.  Not implemented: no TLS engine."))
	globals.Set("get_default_verify_paths", py.MustNewMethod("get_default_verify_paths",
		sslGetDefaultVerifyPaths, 0, "Report where a CA store would be looked for (none is read)."))
	globals.Set("RAND_add", py.MustNewMethod("RAND_add", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "Mix entropy into the PRNG.  Accepted and ignored: no TLS engine."))
	globals.Set("RAND_bytes", py.MustNewMethod("RAND_bytes", func(self py.Object, args py.Tuple) (py.Object, error) {
		return nil, notImplementedError("RAND_bytes")
	}, 0, "Return random bytes.  Not implemented: no TLS RNG."))
	globals.Set("get_server_certificate", py.MustNewMethod("get_server_certificate",
		func(self py.Object, args py.Tuple) (py.Object, error) {
			return nil, notImplementedError("get_server_certificate")
		}, 0, "Retrieve a server certificate.  Not implemented: no TLS engine."))

	py.RegisterModule(&py.ModuleImpl{
		Info: py.ModuleInfo{
			Name: "ssl",
			Doc:  module_doc,
		},
		Globals: globals,
	})
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
