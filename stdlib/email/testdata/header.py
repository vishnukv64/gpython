# email.header behaviour, checked against CPython's own output.
#
# Only the parts this build implements are exercised: decoding, str(), the
# pip sanitise_header() round trip, equality, and the RFC 2047 encoders for the
# codecs gpython provides (us-ascii, utf-8, latin-1/iso-8859-1).  Charsets whose
# codec gpython lacks (big5, koi8-r, iso-2022-jp, ...) raise NotImplementedError
# and are covered by the Go test instead of this golden.
from email.header import Header, decode_header, make_header


def show(label, fn):
    try:
        print(label, "=>", repr(fn()))
    except Exception as e:
        print(label, "!!", type(e).__name__, e)


# --- str() and construction -------------------------------------------------
show("str utf-8", lambda: str(Header("caf\xe9", "utf-8")))
show("str latin-1", lambda: str(Header("caf\xe9", "latin-1")))
show("str iso-8859-1", lambda: str(Header("caf\xe9", "iso-8859-1")))
show("str plain", lambda: str(Header("hello")))
show("str empty", lambda: str(Header()))
show("str empty string", lambda: str(Header("")))
show("str None", lambda: str(Header(None)))
show("str bytes utf-8", lambda: str(Header(b"caf\xc3\xa9", "utf-8")))
show("str whitespace only", lambda: str(Header("   ")))
show("str leading ws", lambda: str(Header("  x")))

# --- append -----------------------------------------------------------------
def appended():
    h = Header()
    h.append("a")
    h.append("b", "utf-8")
    h.append("c")
    return h


def appended_bytes():
    h = Header()
    h.append(b"caf\xc3\xa9", "utf-8")
    return h


def appended_mixed():
    h = Header("plain")
    h.append("caf\xe9", "utf-8")
    h.append("tail")
    return h


show("append", lambda: str(appended()))
show("append bytes", lambda: str(appended_bytes()))
show("append mixed str", lambda: str(appended_mixed()))
show("append mixed encode", lambda: appended_mixed().encode())
show("append mixed decode", lambda: decode_header(appended_mixed()))

# --- equality ---------------------------------------------------------------
show("eq str true", lambda: Header("caf\xe9", "utf-8") == "caf\xe9")
show("eq str false", lambda: Header("a") == "b")
show("eq header true", lambda: Header("a") == Header("a"))
show("eq header false", lambda: Header("a") == Header("b"))
show("eq int", lambda: Header("a") == 1)
show("ne int", lambda: Header("a") != 1)
show("ne str", lambda: Header("a") != "a")
show("str eq header", lambda: "a" == Header("a"))

# --- decode_header ----------------------------------------------------------
show("dh plain", lambda: decode_header("the text"))
show("dh empty", lambda: decode_header(""))
show("dh q utf-8", lambda: decode_header("=?utf-8?q?caf=C3=A9?="))
show("dh b utf-8", lambda: decode_header("=?utf-8?b?Y2Fmw6k=?="))
show("dh b no padding", lambda: decode_header("=?utf-8?b?Y2Fmw6k?="))
show("dh b empty", lambda: decode_header("=?utf-8?b??="))
show("dh q empty", lambda: decode_header("=?utf-8?q??="))
show("dh q latin-1", lambda: decode_header("=?iso-8859-1?q?caf=E9?="))
show("dh q underscore", lambda: decode_header("=?utf-8?q?a_b?="))
show("dh q bad escape", lambda: decode_header("=?utf-8?q?a=zz?="))
show("dh b invalid", lambda: decode_header("=?utf-8?b?!!!!?="))
show("dh case insensitive", lambda: decode_header("=?UTF-8?Q?caf=C3=A9?="))
show("dh unknown-8bit", lambda: decode_header("=?unknown-8bit?q?caf=C3=A9?="))
show("dh us-ascii", lambda: decode_header("=?us-ascii?q?hello?="))
show("dh mixed around", lambda: decode_header("a =?utf-8?q?caf=C3=A9?= b"))
show("dh two words", lambda: decode_header("=?utf-8?q?caf=C3=A9?= =?utf-8?q?x?="))
show("dh three runs", lambda: decode_header("x =?utf-8?q?a?= y =?utf-8?q?b?= z"))
show("dh whitespace run", lambda: decode_header("=?utf-8?q?a?=   =?utf-8?q?b?="))
show("dh newline run", lambda: decode_header("=?utf-8?q?a?= \t\n =?utf-8?q?b?="))
show("dh multiline", lambda: decode_header("=?utf-8?q?a?=\n =?utf-8?q?b?="))
show("dh leading ws", lambda: decode_header("  =?utf-8?q?a?="))
show("dh newline in text", lambda: decode_header("a\nb"))

# a Header decodes straight from its chunks
show("dh Header utf-8", lambda: decode_header(Header("caf\xe9", "utf-8")))
show("dh Header plain", lambda: decode_header(Header("hello")))
show("dh Header latin-1", lambda: decode_header(Header("caf\xe9", "latin-1")))
show("dh Header iso-8859-1", lambda: decode_header(Header("caf\xe9", "iso-8859-1")))
show("dh Header bytes", lambda: decode_header(Header(b"caf\xc3\xa9", "utf-8")))

# --- make_header ------------------------------------------------------------
show("mh roundtrip", lambda: str(make_header(decode_header("=?utf-8?q?caf=C3=A9?="))))
show("mh plain", lambda: str(make_header([(b"the text", None)])))
show("mh utf-8", lambda: str(make_header([("caf\xe9", "utf-8")])))
show("mh latin1", lambda: str(make_header([(b"caf\xc3\xa9", "latin1")])))
show("mh unknown-8bit", lambda: str(make_header([(b"caf\xc3\xa9", "unknown-8bit")])))
show("mh None then utf-8", lambda: str(make_header([("abc", None), ("def", "utf-8")])))
show("mh utf-8 then None", lambda: str(make_header([("abc", "utf-8"), ("def", None)])))
show("mh named", lambda: make_header([(b"a", "utf-8")], header_name="Subject").encode())
show(
    "mh maxlinelen",
    lambda: make_header(
        decode_header("=?utf-8?q?caf=C3=A9?=" * 5), maxlinelen=30
    ).encode(),
)

# --- encode -----------------------------------------------------------------
show("encode plain", lambda: Header("x").encode())
show("encode utf-8", lambda: Header("caf\xe9", "utf-8").encode())
show("encode latin-1", lambda: Header("caf\xe9", "latin-1").encode())
show("encode ascii fallback", lambda: Header("caf\xe9").encode())
show("encode splitchars", lambda: Header("a; b, c d\te").encode())
show("encode newline", lambda: Header("line1\nline2").encode())


def embedded_header():
    # type(e).__name__ is not printed: gpython names every native exception
    # type with its module ("email.errors.HeaderParseError"), a repo-wide
    # convention shared with urllib.error, json and socket.  The class and the
    # message are what this test can hold both interpreters to.
    from email.errors import HeaderParseError

    try:
        Header("a\nb: c").encode()
    except HeaderParseError as e:
        return (isinstance(e, HeaderParseError), str(e))
    return "no error"


show("encode embedded header", embedded_header)
show("encode long", lambda: Header("long " * 20).encode())
show("encode long crlf", lambda: Header("long " * 20).encode(linesep="\r\n"))
show("encode header name", lambda: Header("Hello " * 30, header_name="Subject").encode())
show("encode maxlinelen 0", lambda: Header("a " * 80).encode(maxlinelen=0))
show("encode fold comma", lambda: Header("a, b; c\td e").encode(maxlinelen=8))
show("encode fold words", lambda: Header("one two three four five six").encode(maxlinelen=12))
show("encode fold semicolons", lambda: Header("a; b; c; d; e; f; g; h; i; j").encode(maxlinelen=10))
show("encode fold tabs", lambda: Header("a\tb\tc\td\te\tf").encode(maxlinelen=10))
show("encode leading ws", lambda: Header("  hello world").encode())
show("encode parens", lambda: Header("(x)").encode())
show("encode utf-8 long", lambda: Header("caf\xe9" * 20, "utf-8").encode())
show("encode utf-8 folded", lambda: Header("x" * 10 + "caf\xe9" * 10, "utf-8").encode(maxlinelen=30))
show("encode many", lambda: appended().encode())

# --- the pip sanitise_header() round trip -----------------------------------
def sanitise_header(h):
    if isinstance(h, Header):
        chunks = []
        for bytes, encoding in decode_header(h):
            if encoding == "unknown-8bit":
                try:
                    encoding = "utf-8"
                    bytes.decode(encoding)
                except UnicodeDecodeError:
                    encoding = "latin1"
            chunks.append((bytes, encoding))
        return str(make_header(chunks))
    return str(h)


show("pip plain", lambda: sanitise_header(Header("hello")))
show("pip encoded word", lambda: sanitise_header("=?utf-8?q?caf=C3=A9?="))
show("pip Header utf-8", lambda: sanitise_header(Header("=?utf-8?q?caf=C3=A9?=", "utf-8")))
show("pip unknown-8bit", lambda: sanitise_header(Header(b"\xff", "unknown-8bit")))
show("pip unknown-8bit utf-8", lambda: sanitise_header(Header(b"caf\xc3\xa9", "unknown-8bit")))

doc = "finished"
