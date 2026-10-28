package ssl

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vishnukv64/gpython/py"
	"github.com/vishnukv64/gpython/stdlib/socket"
)

func notImplementedError(what string) error {
	return py.ExceptionNewf(py.NotImplementedError, "ssl: %s is not supported by this interpreter", what)
}

// ---------------------------------------------------------------------------
// SSLContext
// ---------------------------------------------------------------------------

var SSLContextType = py.NewTypeX("_ssl._SSLContext",
	"An SSL context holds configuration data and certificates (client side only).",
	sslContextNew, nil)

type sslContext struct {
	protocol      int
	verifyMode    int
	checkHostname bool
	options       int64 // OP_ALL needs bit 31, which overflows int on 32-bit
	minVersion    int
	maxVersion    int
	system        bool // trust the platform's roots
	roots         *x509.CertPool
	certs         []tls.Certificate
	alpn          []string
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
	c := &sslContext{protocol: p, Dict: py.NewStringDict(), options: OP_ALL | OP_NO_SSLv2 | OP_NO_SSLv3, minVersion: -2, maxVersion: -1}
	if p == PROTOCOL_TLS_CLIENT {
		c.verifyMode = CERT_REQUIRED
		c.checkHostname = true
	}
	return c, nil
}

func (c *sslContext) pool() *x509.CertPool {
	if c.roots == nil {
		c.roots = x509.NewCertPool()
	}
	return c.roots
}

func (c *sslContext) loadSystem() error {
	sp, err := x509.SystemCertPool()
	if err != nil {
		return py.ExceptionNewf(SSLErrorType, "cannot load the system certificate store: %v", err)
	}
	c.system = true
	if c.roots == nil {
		c.roots = sp
	}
	return nil
}

// fsPath is os.fspath for str and bytes paths and objects with __fspath__.
func fsPath(o py.Object) (string, error) {
	switch v := o.(type) {
	case py.String:
		return string(v), nil
	case py.Bytes:
		return string(v), nil
	}
	m, err := py.GetAttrString(o, "__fspath__")
	if err != nil {
		return "", py.ExceptionNewf(py.TypeError, "expected str, bytes or os.PathLike object, not %s", o.Type().Name)
	}
	r, err := py.Call(m, py.Tuple{}, py.StringDict{})
	if err != nil {
		return "", err
	}
	return fsPath(r)
}

func sslErr(format string, a ...interface{}) error {
	return py.ExceptionNewf(SSLErrorType, format, a...)
}

// addPEM adds every certificate in data; it reports whether there was one.
func (c *sslContext) addPEM(data []byte) bool {
	n := 0
	for {
		var blk *pem.Block
		blk, data = pem.Decode(data)
		if blk == nil {
			break
		}
		if blk.Type != "CERTIFICATE" {
			continue
		}
		if cert, err := x509.ParseCertificate(blk.Bytes); err == nil {
			c.pool().AddCert(cert)
			n++
		}
	}
	return n > 0
}

func loadVerifyLocations(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := self.(*sslContext)
	var cafile, capath, cadata py.Object = py.None, py.None, py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "|OOO:load_verify_locations",
		[]string{"cafile", "capath", "cadata"}, &cafile, &capath, &cadata); err != nil {
		return nil, err
	}
	if cafile == py.None && capath == py.None && cadata == py.None {
		return nil, py.ExceptionNewf(py.TypeError, "cafile, capath and cadata cannot be all omitted")
	}
	if cafile != py.None {
		f, err := fsPath(cafile)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, py.ExceptionNewf(py.OSError, "[Errno 2] No such file or directory: %q", f)
		}
		if !c.addPEM(data) {
			return nil, sslErr("[SSL] PEM lib (_ssl.c:0)")
		}
	}
	if capath != py.None {
		d, err := fsPath(capath)
		if err != nil {
			return nil, err
		}
		files, _ := filepath.Glob(filepath.Join(d, "*"))
		for _, f := range files {
			if data, err := os.ReadFile(f); err == nil {
				c.addPEM(data)
			}
		}
	}
	switch d := cadata.(type) {
	case py.String:
		if !c.addPEM([]byte(d)) {
			return nil, sslErr("no start line: cadata does not contain a certificate")
		}
	case py.Bytes:
		cert, err := x509.ParseCertificate([]byte(d))
		if err != nil {
			return nil, sslErr("not enough data: cadata does not contain a certificate")
		}
		c.pool().AddCert(cert)
	}
	return py.None, nil
}

func loadCertChain(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := self.(*sslContext)
	var certfile, keyfile, password py.Object = nil, py.None, py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OO:load_cert_chain",
		[]string{"certfile", "keyfile", "password"}, &certfile, &keyfile, &password); err != nil {
		return nil, err
	}
	cf, err := fsPath(certfile)
	if err != nil {
		return nil, err
	}
	kf := cf
	if keyfile != py.None {
		if kf, err = fsPath(keyfile); err != nil {
			return nil, err
		}
	}
	pair, err := tls.LoadX509KeyPair(cf, kf)
	if err != nil {
		return nil, sslErr("%v", err)
	}
	c.certs = append(c.certs, pair)
	return py.None, nil
}

func intProp(get func(*sslContext) int, set func(*sslContext, int) error) *py.Property {
	return &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.Int(get(self.(*sslContext))), nil },
		Fset: func(self py.Object, v py.Object) error {
			n, err := py.IndexInt(v)
			if err != nil {
				return err
			}
			return set(self.(*sslContext), n)
		},
	}
}

// ---------------------------------------------------------------------------
// SSLSocket
// ---------------------------------------------------------------------------

var SSLSocketType = socket.SocketType.NewType("ssl.SSLSocket",
	"An SSL socket: a socket whose traffic goes through TLS.",
	func(metatype *py.Type, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
		return nil, py.ExceptionNewf(py.TypeError,
			"SSLSocket does not have a public constructor. Instances are returned by SSLContext.wrap_socket().")
	}, nil)

// tlsConn is the TLS connection plus what SSLSocket reports about its origin.
type tlsConn struct {
	*tls.Conn
	ctx      *sslContext
	hostname py.Object
}

func tlsOf(self py.Object) (*tlsConn, error) {
	if c, ok := socket.ConnOf(self); ok {
		if tc, ok := c.(*tlsConn); ok {
			return tc, nil
		}
	}
	return nil, py.ExceptionNewf(py.ValueError, "Underlying socket has been detached.")
}

func (c *sslContext) config(host string) *tls.Config {
	cfg := &tls.Config{
		ServerName:   host,
		RootCAs:      c.roots,
		Certificates: c.certs,
		NextProtos:   c.alpn,
		MinVersion:   tls.VersionTLS12,
	}
	if c.roots == nil && !c.system {
		cfg.RootCAs = x509.NewCertPool() // CPython: no CAs loaded, nothing verifies
	}
	if v := goVersion(c.minVersion); v != 0 && v > cfg.MinVersion {
		cfg.MinVersion = v
	}
	cfg.MaxVersion = goVersion(c.maxVersion)
	switch {
	case c.verifyMode == CERT_NONE:
		cfg.InsecureSkipVerify = true
	case !c.checkHostname:
		// Verify the chain but not the name (CERT_REQUIRED, check_hostname off).
		cfg.InsecureSkipVerify = true
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			opts := x509.VerifyOptions{Roots: cfg.RootCAs, Intermediates: x509.NewCertPool()}
			for _, ic := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(ic)
			}
			_, err := cs.PeerCertificates[0].Verify(opts)
			return err
		}
	}
	return cfg
}

func goVersion(v int) uint16 {
	switch v {
	case 769:
		return tls.VersionTLS10
	case 770:
		return tls.VersionTLS11
	case 771:
		return tls.VersionTLS12
	case 772:
		return tls.VersionTLS13
	}
	return 0
}

func handshakeErr(err error) error {
	var ce *tls.CertificateVerificationError
	var ua x509.UnknownAuthorityError
	var hn x509.HostnameError
	var ci x509.CertificateInvalidError
	if errors.As(err, &ce) || errors.As(err, &ua) || errors.As(err, &hn) || errors.As(err, &ci) {
		msg := "certificate verify failed"
		switch {
		case errors.As(err, &hn):
			msg = "Hostname mismatch, certificate is not valid for '" + hn.Host + "'."
		case errors.As(err, &ua):
			msg = "unable to get local issuer certificate"
		case errors.As(err, &ci) && ci.Reason == x509.Expired:
			msg = "certificate has expired"
		}
		e := py.ExceptionNewf(SSLCertVerificationErrorType, "[SSL: CERTIFICATE_VERIFY_FAILED] certificate verify failed: %s (_ssl.c:1000)", msg)
		return e
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return py.ExceptionNewf(py.TimeoutError, "_ssl.c:1000: The handshake operation timed out")
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return py.ExceptionNewf(py.TimeoutError, "_ssl.c:1000: The handshake operation timed out")
	}
	return py.ExceptionNewf(SSLErrorType, "[SSL] %v (_ssl.c:1000)", err)
}

func wrapSocket(self py.Object, args py.Tuple, kwargs py.StringDict) (py.Object, error) {
	c := self.(*sslContext)
	var sock py.Object
	var serverSide, handshake, suppress py.Object = py.False, py.True, py.True
	var hostname, session py.Object = py.None, py.None
	if err := py.ParseTupleAndKeywords(args, kwargs, "O|OOOOO:wrap_socket",
		[]string{"sock", "server_side", "do_handshake_on_connect", "suppress_ragged_eofs", "server_hostname", "session"},
		&sock, &serverSide, &handshake, &suppress, &hostname, &session); err != nil {
		return nil, err
	}
	if truthy(serverSide) {
		return nil, notImplementedError("server-side wrap_socket")
	}
	host := ""
	if hostname != py.None {
		h, ok := hostname.(py.String)
		if !ok {
			return nil, py.ExceptionNewf(py.TypeError, "server_hostname must be str or None")
		}
		host = strings.TrimSuffix(string(h), ".")
	} else if c.checkHostname {
		return nil, py.ExceptionNewf(py.ValueError, "check_hostname requires server_hostname")
	}
	doHS := truthy(handshake)
	return socket.Rewrap(sock, SSLSocketType, func(raw net.Conn, deadline time.Time) (net.Conn, error) {
		cfg := c.config(host)
		if host == "" && !cfg.InsecureSkipVerify {
			cfg.InsecureSkipVerify = true // no name to check against
		}
		tc := &tlsConn{Conn: tls.Client(raw, cfg), ctx: c, hostname: hostname}
		if doHS {
			_ = raw.SetDeadline(deadline)
			err := tc.Handshake()
			_ = raw.SetDeadline(time.Time{})
			if err != nil {
				return nil, handshakeErr(err)
			}
		}
		return tc, nil
	})
}

func certDict(cert *x509.Certificate) py.Object {
	name := func(n [](interface{ String() string })) {}
	_ = name
	rdn := func(vals map[string][]string, order []string) py.Object {
		out := py.Tuple{}
		for _, k := range order {
			for _, v := range vals[k] {
				out = append(out, py.Tuple{py.Tuple{py.String(k), py.String(v)}})
			}
		}
		return out
	}
	subj := map[string][]string{"countryName": cert.Subject.Country, "stateOrProvinceName": cert.Subject.Province,
		"localityName": cert.Subject.Locality, "organizationName": cert.Subject.Organization,
		"organizationalUnitName": cert.Subject.OrganizationalUnit, "commonName": {cert.Subject.CommonName}}
	iss := map[string][]string{"countryName": cert.Issuer.Country, "stateOrProvinceName": cert.Issuer.Province,
		"localityName": cert.Issuer.Locality, "organizationName": cert.Issuer.Organization,
		"organizationalUnitName": cert.Issuer.OrganizationalUnit, "commonName": {cert.Issuer.CommonName}}
	order := []string{"countryName", "stateOrProvinceName", "localityName", "organizationName", "organizationalUnitName", "commonName"}
	san := py.Tuple{}
	for _, d := range cert.DNSNames {
		san = append(san, py.Tuple{py.String("DNS"), py.String(d)})
	}
	for _, ip := range cert.IPAddresses {
		san = append(san, py.Tuple{py.String("IP Address"), py.String(ip.String())})
	}
	const f = "Jan _2 15:04:05 2006 GMT"
	d := py.NewStringDict()
	d.Set("subject", rdn(subj, order))
	d.Set("issuer", rdn(iss, order))
	d.Set("version", py.Int(cert.Version))
	d.Set("serialNumber", py.String(strings.ToUpper(cert.SerialNumber.Text(16))))
	d.Set("notBefore", py.String(cert.NotBefore.UTC().Format(f)))
	d.Set("notAfter", py.String(cert.NotAfter.UTC().Format(f)))
	if len(san) > 0 {
		d.Set("subjectAltName", san)
	}
	return d
}

func init() {
	d := SSLContextType.Dict
	d.Set("protocol", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(self.(*sslContext).protocol), nil
	}})
	d.Set("verify_mode", intProp(func(c *sslContext) int { return c.verifyMode },
		func(c *sslContext, n int) error {
			if n < CERT_NONE || n > CERT_REQUIRED {
				return py.ExceptionNewf(py.ValueError, "invalid value for verify_mode")
			}
			if n == CERT_NONE && c.checkHostname {
				return py.ExceptionNewf(py.ValueError, "Cannot set verify_mode to CERT_NONE when check_hostname is enabled.")
			}
			c.verifyMode = n
			return nil
		}))
	d.Set("check_hostname", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.Bool(self.(*sslContext).checkHostname), nil },
		Fset: func(self py.Object, v py.Object) error {
			c := self.(*sslContext)
			on := truthy(v)
			if on && c.verifyMode == CERT_NONE {
				c.verifyMode = CERT_REQUIRED
			}
			c.checkHostname = on
			return nil
		},
	})
	d.Set("options", &py.Property{
		Fget: func(self py.Object) (py.Object, error) { return py.Int(self.(*sslContext).options), nil },
		Fset: func(self py.Object, v py.Object) error {
			n, err := py.MakeGoInt64(v)
			if err != nil {
				return err
			}
			self.(*sslContext).options = n
			return nil
		},
	})
	d.Set("minimum_version", intProp(func(c *sslContext) int { return c.minVersion }, func(c *sslContext, n int) error { c.minVersion = n; return nil }))
	d.Set("maximum_version", intProp(func(c *sslContext) int { return c.maxVersion }, func(c *sslContext, n int) error { c.maxVersion = n; return nil }))
	d.Set("verify_flags", intProp(func(c *sslContext) int { return 0 }, func(c *sslContext, n int) error { return nil }))
	d.Set("post_handshake_auth", &py.Property{
		Fget: func(py.Object) (py.Object, error) { return py.False, nil },
		Fset: func(py.Object, py.Object) error { return nil }})
	d.Set("keylog_filename", &py.Property{
		Fget: func(py.Object) (py.Object, error) { return py.None, nil },
		Fset: func(py.Object, py.Object) error { return nil }})
	d.Set("hostname_checks_common_name", &py.Property{
		Fget: func(py.Object) (py.Object, error) { return py.False, nil },
		Fset: func(py.Object, py.Object) error { return nil }})
	d.Set("wrap_socket", py.MustNewMethod("wrap_socket", wrapSocket, 0, "Wrap a connected socket in TLS (client side)."))
	d.Set("load_verify_locations", py.MustNewMethod("load_verify_locations", loadVerifyLocations, 0, "Load CA certificates."))
	d.Set("load_cert_chain", py.MustNewMethod("load_cert_chain", loadCertChain, 0, "Load a client certificate chain and key."))
	d.Set("load_default_certs", py.MustNewMethod("load_default_certs", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, self.(*sslContext).loadSystem()
	}, 0, "Trust the platform's CA store."))
	d.Set("set_default_verify_paths", py.MustNewMethod("set_default_verify_paths", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, self.(*sslContext).loadSystem()
	}, 0, "Trust the platform's CA store."))
	d.Set("set_ciphers", py.MustNewMethod("set_ciphers", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil // ponytail: Go picks its own safe suites; no per-suite control
	}, 0, "Accepted and ignored: crypto/tls chooses the cipher suites."))
	d.Set("set_alpn_protocols", py.MustNewMethod("set_alpn_protocols", func(self py.Object, args py.Tuple) (py.Object, error) {
		var l py.Object
		if err := py.UnpackTuple(args, py.StringDict{}, "set_alpn_protocols", 1, 1, &l); err != nil {
			return nil, err
		}
		var out []string
		if err := py.Iterate(l, func(v py.Object) bool {
			if str, ok := v.(py.String); ok {
				out = append(out, string(str))
			}
			return false
		}); err != nil {
			return nil, err
		}
		self.(*sslContext).alpn = out
		return py.None, nil
	}, 0, "Set the ALPN protocols offered."))
	d.Set("cert_store_stats", py.MustNewMethod("cert_store_stats", func(self py.Object, args py.Tuple) (py.Object, error) {
		sd := py.NewStringDict()
		sd.Set("x509", py.Int(0))
		sd.Set("crl", py.Int(0))
		sd.Set("x509_ca", py.Int(0))
		return sd, nil
	}, 0, "Certificate store counts (not tracked)."))
	d.Set("get_ca_certs", py.MustNewMethod("get_ca_certs", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.NewListFromItems(nil), nil
	}, 0, "Loaded CA certificates (not enumerated)."))
	d.Set("wrap_bio", py.MustNewMethod("wrap_bio", func(self py.Object, args py.Tuple, kw py.StringDict) (py.Object, error) {
		return &sslObject{}, nil
	}, 0, "Return an SSLObject; it cannot handshake."))

	// SSLSocket.
	sd := SSLSocketType.Dict
	sd.Set("version", py.MustNewMethod("version", func(self py.Object, args py.Tuple) (py.Object, error) {
		tc, err := tlsOf(self)
		if err != nil {
			return nil, err
		}
		return py.String(tls.VersionName(tc.ConnectionState().Version)), nil
	}, 0, "Return the negotiated TLS version."))
	sd.Set("cipher", py.MustNewMethod("cipher", func(self py.Object, args py.Tuple) (py.Object, error) {
		tc, err := tlsOf(self)
		if err != nil {
			return nil, err
		}
		cs := tc.ConnectionState()
		return py.Tuple{py.String(tls.CipherSuiteName(cs.CipherSuite)), py.String(tls.VersionName(cs.Version)), py.Int(0)}, nil
	}, 0, "Return (name, protocol, secret_bits)."))
	sd.Set("selected_alpn_protocol", py.MustNewMethod("selected_alpn_protocol", func(self py.Object, args py.Tuple) (py.Object, error) {
		tc, err := tlsOf(self)
		if err != nil {
			return nil, err
		}
		if p := tc.ConnectionState().NegotiatedProtocol; p != "" {
			return py.String(p), nil
		}
		return py.None, nil
	}, 0, "Return the ALPN protocol, or None."))
	sd.Set("compression", py.MustNewMethod("compression", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "TLS compression is never negotiated."))
	sd.Set("pending", py.MustNewMethod("pending", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.Int(0), nil
	}, 0, "Bytes buffered inside the TLS layer (not tracked)."))
	sd.Set("do_handshake", py.MustNewMethod("do_handshake", func(self py.Object, args py.Tuple) (py.Object, error) {
		tc, err := tlsOf(self)
		if err != nil {
			return nil, err
		}
		if err := tc.Handshake(); err != nil {
			return nil, handshakeErr(err)
		}
		return py.None, nil
	}, 0, "Perform the handshake."))
	sd.Set("getpeercert", py.MustNewMethod("getpeercert", func(self py.Object, args py.Tuple) (py.Object, error) {
		tc, err := tlsOf(self)
		if err != nil {
			return nil, err
		}
		binary := len(args) > 0 && truthy(args[0])
		pc := tc.ConnectionState().PeerCertificates
		if len(pc) == 0 {
			return py.None, nil
		}
		if binary {
			return py.Bytes(pc[0].Raw), nil
		}
		return certDict(pc[0]), nil
	}, 0, "Return the peer certificate (dict, or DER bytes when binary_form)."))
	sd.Set("read", socket.SocketType.Dict.GetOrNil("recv"))
	sd.Set("write", socket.SocketType.Dict.GetOrNil("send"))
	sd.Set("context", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		tc, err := tlsOf(self)
		if err != nil {
			return nil, err
		}
		return tc.ctx, nil
	}})
	sd.Set("server_hostname", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		tc, err := tlsOf(self)
		if err != nil {
			return nil, err
		}
		return tc.hostname, nil
	}})
}

// sslObject is what wrap_bio returns.  There is no BIO transport, so it
// cannot handshake; its existence lets "hasattr" probes (pip's truststore)
// learn that the peer-chain API is absent and fall back.
type sslObject struct{}

var SSLObjectType = py.NewType("ssl.SSLObject", "TLS over memory buffers (not supported here).")

func (*sslObject) Type() *py.Type { return SSLObjectType }

// MemoryBIO is a byte pipe: write() appends, read() drains.
type memoryBIO struct {
	buf []byte
	eof bool
}

var MemoryBIOType = py.NewTypeX("_ssl.MemoryBIO", "Memory buffer for TLS data.",
	func(*py.Type, py.Tuple, py.StringDict) (py.Object, error) { return &memoryBIO{}, nil }, nil)

func (*memoryBIO) Type() *py.Type { return MemoryBIOType }

func init() {
	MemoryBIOType.Dict.Set("write", py.MustNewMethod("write", func(self py.Object, args py.Tuple) (py.Object, error) {
		b, ok := args[0].(py.Bytes)
		if len(args) != 1 || !ok {
			return nil, py.ExceptionNewf(py.TypeError, "write() argument must be a bytes-like object")
		}
		m := self.(*memoryBIO)
		m.buf = append(m.buf, b...)
		return py.Int(len(b)), nil
	}, 0, "Append data."))
	MemoryBIOType.Dict.Set("read", py.MustNewMethod("read", func(self py.Object, args py.Tuple) (py.Object, error) {
		m := self.(*memoryBIO)
		n := len(m.buf)
		if len(args) > 0 {
			if v, err := py.IndexInt(args[0]); err == nil && v >= 0 && v < n {
				n = v
			}
		}
		out := py.Bytes(append([]byte(nil), m.buf[:n]...))
		m.buf = m.buf[n:]
		return out, nil
	}, 0, "Drain up to n bytes."))
	MemoryBIOType.Dict.Set("write_eof", py.MustNewMethod("write_eof", func(self py.Object, args py.Tuple) (py.Object, error) {
		self.(*memoryBIO).eof = true
		return py.None, nil
	}, 0, "Mark end of input."))
	MemoryBIOType.Dict.Set("pending", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		return py.Int(len(self.(*memoryBIO).buf)), nil
	}})
	MemoryBIOType.Dict.Set("eof", &py.Property{Fget: func(self py.Object) (py.Object, error) {
		m := self.(*memoryBIO)
		return py.Bool(m.eof && len(m.buf) == 0), nil
	}})

	globals := py.NewStringDict()
	for name, t := range map[string]*py.Type{
		"SSLError": SSLErrorType, "SSLZeroReturnError": SSLZeroReturnErrorType,
		"SSLWantReadError": SSLWantReadErrorType, "SSLWantWriteError": SSLWantWriteErrorType,
		"SSLSyscallError": SSLSyscallErrorType, "SSLEOFError": SSLEOFErrorType,
		"SSLCertVerificationError": SSLCertVerificationErrorType,
		"SSLContext":               SSLContextType, "_SSLContext": SSLContextType, "SSLSocket": SSLSocketType,
		"SSLObject": SSLObjectType, "MemoryBIO": MemoryBIOType,
	} {
		globals.Set(name, t)
	}
	for name, v := range map[string]int64{
		"CERT_NONE": CERT_NONE, "CERT_OPTIONAL": CERT_OPTIONAL, "CERT_REQUIRED": CERT_REQUIRED,
		"PROTOCOL_TLS": PROTOCOL_TLS, "PROTOCOL_TLS_CLIENT": PROTOCOL_TLS_CLIENT,
		"PROTOCOL_TLS_SERVER": PROTOCOL_TLS_SERVER, "PROTOCOL_SSLv23": PROTOCOL_SSLv23,
		"PROTOCOL_TLSv1": PROTOCOL_TLSv1, "PROTOCOL_TLSv1_1": PROTOCOL_TLSv1_1, "PROTOCOL_TLSv1_2": PROTOCOL_TLSv1_2,
		"OP_NO_SSLv2": OP_NO_SSLv2, "OP_NO_SSLv3": OP_NO_SSLv3, "OP_NO_TLSv1": OP_NO_TLSv1,
		"OP_NO_TLSv1_1": OP_NO_TLSv1_1, "OP_NO_TLSv1_2": OP_NO_TLSv1_2, "OP_NO_TLSv1_3": OP_NO_TLSv1_3,
		"OP_NO_COMPRESSION": OP_NO_COMPRESSION, "OP_NO_TICKET": OP_NO_TICKET, "OP_ALL": OP_ALL,
		"OP_CIPHER_SERVER_PREFERENCE": OP_CIPHER_SERVER_PREFERENCE,
		"OPTIONS_ALL":                 0, "VERIFY_DEFAULT": VERIFY_DEFAULT, "VERIFY_CRL_CHECK_LEAF": VERIFY_CRL_CHECK_LEAF,
		"VERIFY_X509_STRICT": 0x20, "VERIFY_X509_PARTIAL_CHAIN": 0x80000, "VERIFY_X509_TRUSTED_FIRST": 0x8000,
		"SSL_ERROR_NONE": SSL_ERROR_NONE, "SSL_ERROR_SSL": 1, "SSL_ERROR_WANT_READ": 2, "SSL_ERROR_WANT_WRITE": 3,
		"SSL_ERROR_SYSCALL": 5, "SSL_ERROR_ZERO_RETURN": 6, "SSL_ERROR_EOF": 8,
		"OPENSSL_VERSION_NUMBER": 0x30000000,
	} {
		globals.Set(name, py.Int(v))
	}
	globals.Set("HAS_SNI", py.True)
	globals.Set("HAS_ALPN", py.True)
	globals.Set("HAS_TLSv1_3", py.True)
	globals.Set("HAS_NEVER_CHECK_COMMON_NAME", py.True)
	globals.Set("HAS_ECDH", py.True)
	globals.Set("HAS_NPN", py.False)
	for _, n := range []string{"HAS_SSLv2", "HAS_SSLv3", "HAS_TLSv1", "HAS_TLSv1_1"} {
		globals.Set(n, py.False)
	}
	globals.Set("HAS_TLSv1_2", py.True)
	globals.Set("OPENSSL_VERSION", py.String("Go crypto/tls"))
	globals.Set("OPENSSL_VERSION_INFO", py.Tuple{py.Int(3), py.Int(0), py.Int(0), py.Int(0), py.Int(0)})
	globals.Set("get_default_verify_paths", py.MustNewMethod("get_default_verify_paths",
		func(self py.Object, args py.Tuple) (py.Object, error) {
			return py.Tuple{py.None, py.None, py.NewListFromItems(nil)}, nil
		}, 0, "Where a CA store would be looked for (the platform store is used directly)."))
	globals.Set("RAND_add", py.MustNewMethod("RAND_add", func(self py.Object, args py.Tuple) (py.Object, error) {
		return py.None, nil
	}, 0, "Accepted and ignored: crypto/rand needs no seeding."))

	py.RegisterModule(&py.ModuleImpl{
		Info:    py.ModuleInfo{Name: "_ssl", Doc: module_doc},
		Globals: globals,
	})
}

func truthy(v py.Object) bool {
	if v == nil || v == py.None {
		return false
	}
	b, err := py.ObjectIsTrue(v)
	return err == nil && b
}
