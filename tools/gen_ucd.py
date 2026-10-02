#!/usr/bin/env python3
"""Generate the Unicode tables for gpython's unicodedata module.

Everything is taken from this machine's CPython unicodedata module, so the
tables agree with the Unicode version the reference interpreter ships (16.0.0
as of CPython 3.14).  The output is one gzip-compressed, base64-encoded blob of
newline-tagged sections, decoded lazily at first use by stdlib/unicodedata.

Usage: python3 tools/gen_ucd.py stdlib/unicodedata/tables.go
"""
import unicodedata as u
import gzip
import base64
import sys
import io


def ranges(f, key):
    """Collapse a per-code-point function into (start, end, value) ranges."""
    out = []
    prev = None
    start = None
    for c in range(0x110000):
        v = f(chr(c))
        if key(v):
            if v != prev:
                if prev is not None:
                    out.append((start, c - 1, prev))
                start, prev = c, v
        else:
            if prev is not None:
                out.append((start, c - 1, prev))
                prev = None
    if prev is not None:
        out.append((start, 0x10FFFF, prev))
    return out


# Ranges whose names are built from the code point rather than stored.  The
# hex width is the minimum number of digits CPython prints.
ALGORITHMIC = [
    (0x3400, 0x4DBF, "CJK UNIFIED IDEOGRAPH-", 4),
    (0x4E00, 0x9FFF, "CJK UNIFIED IDEOGRAPH-", 4),
    (0x20000, 0x2A6DF, "CJK UNIFIED IDEOGRAPH-", 5),
    (0x2A700, 0x2EBEF, "CJK UNIFIED IDEOGRAPH-", 5),
    (0x2EBF0, 0x2EE5D, "CJK UNIFIED IDEOGRAPH-", 5),
    (0x30000, 0x3134A, "CJK UNIFIED IDEOGRAPH-", 5),
    (0x31350, 0x323AF, "CJK UNIFIED IDEOGRAPH-", 5),
    (0xAC00, 0xD7A3, "HANGUL SYLLABLE ", 0),
    (0x17000, 0x187F7, "TANGUT IDEOGRAPH-", 5),
    (0x187F8, 0x18CD5, "TANGUT IDEOGRAPH-", 5),
    (0x18D00, 0x18D08, "TANGUT IDEOGRAPH-", 5),
]


def is_algorithmic(cp):
    return any(lo <= cp <= hi for lo, hi, _, _ in ALGORITHMIC)


def sec_cat():
    return "".join("%X:%X:%s;" % (a, b, v)
                   for a, b, v in ranges(u.category, lambda v: True))


def sec_bidi():
    return "".join("%X:%X:%s;" % (a, b, v)
                   for a, b, v in ranges(u.bidirectional, lambda v: v != ""))


def sec_ccc():
    return "".join("%X:%X:%d;" % (a, b, v)
                   for a, b, v in ranges(u.combining, lambda v: v != 0))


def sec_ea():
    return "".join("%X:%X:%s;" % (a, b, v)
                   for a, b, v in ranges(u.east_asian_width, lambda v: True))


def sec_mir():
    return "".join("%X:%X;" % (a, b)
                   for a, b, _ in ranges(lambda ch: 1 if u.mirrored(ch) else 0,
                                        lambda v: v == 1))


def sec_num():
    out = []
    for c in range(0x110000):
        ch = chr(c)
        d, g, n = u.decimal(ch, None), u.digit(ch, None), u.numeric(ch, None)
        if d is None and g is None and n is None:
            continue
        out.append("%X:%s:%s:%s;" % (
            c,
            "" if d is None else d,
            "" if g is None else g,
            "" if n is None else n))
    return "".join(out)


def sec_decomp():
    out = []
    for c in range(0x110000):
        d = u.decomposition(chr(c))
        if d:
            out.append("%X:%s;" % (c, d))
    return "".join(out)


def sec_comp():
    """The canonical composition pairs.

    A pair composes exactly when the NFC of its canonical decomposition is the
    original character, so asking CPython that question derives the table,
    composition exclusions included, rather than transcribing them by hand.
    """
    out = []
    for c in range(0x110000):
        d = u.decomposition(chr(c))
        if not d or d.startswith("<"):
            continue
        parts = d.split()
        if len(parts) != 2:
            continue
        a, b = int(parts[0], 16), int(parts[1], 16)
        if u.normalize("NFC", chr(a) + chr(b)) == chr(c):
            out.append("%X,%X:%X;" % (a, b, c))
    return "".join(out)


def sec_names():
    out = []
    for c in range(0x110000):
        if is_algorithmic(c):
            continue
        n = u.name(chr(c), None)
        if n:
            out.append("%X:%s;" % (c, n))
    return "".join(out)


def main(path):
    sections = [
        ("cat", sec_cat()),
        ("bidi", sec_bidi()),
        ("ccc", sec_ccc()),
        ("ea", sec_ea()),
        ("mir", sec_mir()),
        ("num", sec_num()),
        ("decomp", sec_decomp()),
        ("comp", sec_comp()),
        ("names", sec_names()),
    ]

    buf = io.StringIO()
    for name, body in sections:
        # The body must end with a newline: the reader finds a section by its
        # "\x01name" line, so without the terminator the next header would be
        # glued onto the end of this body and every later section would be lost.
        buf.write("\x01%s\n" % name)
        buf.write(body)
        buf.write("\n")
    raw = buf.getvalue().encode()
    comp = gzip.compress(raw, 9)

    print("unidata_version:", u.unidata_version, file=sys.stderr)
    for name, body in sections:
        print("  section %-7s raw %8d  gz %7d"
              % (name, len(body), len(gzip.compress(body.encode(), 9))),
              file=sys.stderr)
    print("total raw", len(raw), "gz", len(comp), file=sys.stderr)

    b64 = base64.b64encode(comp).decode()
    lines = [b64[i:i + 100] for i in range(0, len(b64), 100)]

    go = io.StringIO()
    go.write("package unicodedata\n\n")
    go.write("// Code generated from CPython's unicodedata (Unicode %s) by\n"
             "// tools/gen_ucd.py.  DO NOT EDIT.\n\n" % u.unidata_version)
    go.write("// The blob is a gzip stream of newline tagged sections, each holding\n")
    go.write("// semicolon separated records.  It is decoded on first use by buildTables.\n\n")
    go.write("import (\n\t\"bytes\"\n\t\"compress/gzip\"\n\t\"encoding/base64\"\n")
    go.write("\t\"io\"\n\t\"strings\"\n)\n\n")
    go.write('const unidataVersion = "%s"\n\n' % u.unidata_version)
    go.write("// algorithmicNames lists the ranges whose character names are built\n")
    go.write("// from the code point rather than stored, as CPython does.\n")
    go.write("var algorithmicNames = []struct {\n")
    go.write("\tlo, hi    rune\n\tprefix    string\n\thexDigits int\n}{\n")
    for lo, hi, prefix, hd in ALGORITHMIC:
        go.write('\t{0x%X, 0x%X, "%s", %d},\n' % (lo, hi, prefix, hd))
    go.write("}\n\n")

    # base64 is ASCII with no backquotes, so a raw literal avoids a megabyte of
    # Go string escapes.
    go.write("// ucdBlobB64 is the gzip stream, base64 encoded.\n")
    go.write("const ucdBlobB64 = `\n")
    for ln in lines:
        go.write(ln + "\n")
    go.write("`\n\n")
    go.write("""// decodeBlob decompresses the embedded tables.
func decodeBlob() (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(ucdBlobB64), ""))
	if err != nil {
		return "", err
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
""")

    with open(path, "w") as f:
        f.write(go.getvalue())
    print("wrote", path, "bytes:", len(go.getvalue()), file=sys.stderr)


if __name__ == "__main__":
    main(sys.argv[1])
