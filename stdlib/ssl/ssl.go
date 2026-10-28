// Copyright 2024 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ssl provides the native half of python's 'ssl' module (registered as
// _ssl; the public ssl.py builds on it).
//
// It is real client-side TLS on crypto/tls: SSLContext.wrap_socket handshakes,
// verifies the chain against the CAs the context loaded, and returns an
// SSLSocket.  What is NOT provided raises NotImplementedError rather than
// pretending: server-side sockets and the BIO interface.
package ssl

import (
	"github.com/vishnukv64/gpython/py"
)

// Constants beyond the CERT_*/PROTOCOL_* block; values match CPython/OpenSSL.
const (
	OP_NO_COMPRESSION           = 0x00020000
	OP_NO_TICKET                = 0x00004000
	OP_NO_TLSv1_1               = 0x10000000
	OP_NO_TLSv1_2               = 0x08000000
	OP_NO_TLSv1_3               = 0x20000000
	OP_ALL                      = 0x80000050
	OP_CIPHER_SERVER_PREFERENCE = 0x00400000
)

const module_doc = `Native support for Secure Sockets Layer (SSL/TLS), backed by crypto/tls.

Client connections only: SSLContext.wrap_socket(server_side=True), the BIO
interface (MemoryBIO / wrap_bio) and server-side certificates raise
NotImplementedError.  The public module is ssl.py, which builds on this one.
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
