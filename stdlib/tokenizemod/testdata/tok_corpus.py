# Golden: tokenize over a corpus of tricky sources and real stdlib files.
# The _golden.txt is CPython 3.14's output for this same script.
import tokenize, io, sys, os, hashlib
def dump(label, f):
    print("==", label)
    try:
        for t in f():
            print(tokenize.tok_name[t.type], repr(t.string), t.start, t.end, repr(t.line))
    except BaseException as e:
        print("ERR", type(e).__name__, e.args)
def gen(src):
    return lambda: tokenize.generate_tokens(io.StringIO(src, newline='').readline)
def genb(src):
    return lambda: tokenize.tokenize(io.BytesIO(src).readline)
CORPUS = ["x\r\n", "x = 1\r\n\r\ny\r", "$ ? !", 'f"{x:}"', 'f"a{{b}}c"', 'f"""a\nb{x}\nc"""\n', 'x = """a\nb""" + 1\n',
 "x = 1 + \\\n  2\n", "(\n", "x = (1,\n", "if x:\n\ty\n  z\n", "if 1:\n        a\n\tb\n", "'abc\n", '"""abc', "a \\ b", "1if x else 2", "0777", "x = 1_", "def f(): pass", "f'{x!r:>{w}}'", "  # c", "x # c", "\\\n", "'a\\\nb'\n",
 "x = (1,\n2,\n3", "x = (1,\n2,\n3\n\n", "x = 1 + \\\n", "x = 1 + \\", "a \\ b\n", "if 1:\n  a\\\n b\n", "(\n)\n]", "(]", "a = 'abc\n", "a = '''abc\ndef", "'\\\n",
 "x = 0x", "x = 0o9", "x = 1e", "x = 1.e+", "x = 00", "x = 09", "x = 0_0", "x = 1__0", "1.5j", "0b12", "0xg", ".5e3j", "1.", "0e0", "0_7", "1_000.000_1e1_0",
 "é = 1", "a€ = 1", "a\x01", "\x00", "\x0cx", "x\ty", "if 1:\n x\n\ty\n", "\n\n  \n# c\n", "x\n  \n\n  y\n", "a ... b .. c", "a <> b != c ->d := e **= f //= g >>= h <<= i @= j", "!=", "a!b",
 "f'{x=}'", "f'{x = }'", "f'{x:{y}z}'", "f'{{'", "f'}'", "f'{'", "f'{x'", "f'''{\nx\n}'''", "rf'\\{x}'", "f'\\N{BULLET}'", "f'\\{x}'", "f'{x!r}'", "f'{a!=b}'", "f'{a:=5}'", "f'{(a:=5)}'", "f'{lambda x: 1}'", "f'{f\"{x}\"}'", "f'{\"a\" \"b\"}'", "f'a' 'b'", "t'{x}'", "rt'{x}'", "Tr'a'", "bf'a'", "f'{x:%Y-%m}'", "f'{x:\n}'", "f'''{x:\na}'''", "f'{#c\n}'", "f'{\nx}'", "f'a\nb'", "f'abc", "f'''abc", "f'{x}}'", "f'{x:{{}}}'", "f'{x:{y:{z}}}'", "f\"{x['a']}\"", "f'{x:{y:{z:{w}}}}'",
 "async def f(): await x", "print(1) # x\n# y\n", "if 1:\n    # c\n    x\n# d\n  # e\ny\n", "def f():\n    pass\n    \n  \nx\n",
 "\"abc\\\"def\"", "'''a'''b", "\"\"", "''''''", "u'x' b'y' Rb'z' bR'q' ur'x'",
 "if 1:\n  if 2:\n    x\n  y\nz", "class A:\n\tdef f(self):\n\t\treturn 1\n", "x = [\n  1,\n  2,\n]\n", "x = {\n  'a': f'{1}',\n}\n# end", "def f(a,\n      b):\n    return a  # c\n\n\n", "s = 'é' + \"日本\" + f'ü{x}ñ'\n", "x = '''é\n日本''' ; y", "\tx\n", " x\ny", "x\n y\n", "a = 1 \\\n  + 2 \\\n  + 3\n", "if True:\n    x = 1\n\n    # comment\n\ny = 2\n", "\xa0x", "x = 5 \\\n", "'\\N{DIGIT ONE}'", "f'{x}' f'{y}'\n", "f'{x!s:^{w}.{p}}'", "f'{a[\"b\"]}'", "f'{ {1:2}[1] }'", "f'{x:{\"}\"}}'", "t'{x!r}' t\"\"\"a{b}\n\"\"\"", "f'\\\n{x}'", "f'''a\\\nb'''", "f'{x:\\\n}'",
 "x = 1\n\x0c\ny = 2\n", "if 1:\n\x0c  x\n", "0 if 1else 2", "1 if 1 else 2 or 3and 4", "1_a", "1.2.3", "1..real", "x.5", "1e5j", "1jj", "1__", "0b", "0o", "0b_1", "0x_f", "0xf_", "0_", "00_", "0e", "0.e", "1ee1",
]
def main():
    for i, s in enumerate(CORPUS):
        dump("str%d %r" % (i, s), gen(s))
    for i, s in enumerate(CORPUS):
        try: b = s.encode()
        except Exception: continue
        dump("bytes%d %r" % (i, s), genb(b))
    for enc_src in [b"# -*- coding: latin-1 -*-\nx = '\xe9'\n", b"\xef\xbb\xbfx = 1\n", b"# coding: ascii\nx = 'a'\n", b"# vim: set fileencoding=utf-8 :\ny=1", b"x = 1\n# coding: latin-1\n", b"# coding: bogus\nx=1\n", b"\xff\xfe", b"# coding: latin-1\r\nx = '\xe9'\r\n"]:
        dump("enc %r" % enc_src, genb(enc_src))
    for s in CORPUS:
        try:
            toks = list(tokenize.generate_tokens(io.StringIO(s, newline='').readline))
        except BaseException:
            continue
        try:
            print("UNTOK", repr(s), repr(tokenize.untokenize(toks)), repr(tokenize.untokenize([(t.type, t.string) for t in toks])))
        except BaseException as e:
            print("UNTOK ERR", repr(s), type(e).__name__, e.args)
    for eb in [b"# -*- coding: latin-1 -*-\nx = 1\n", b"\xef\xbb\xbfx=1", b"# coding: bogus\n", b"x\n# coding: utf-8\n", b"\xff\n"]:
        try:
            print("DETECT", repr(eb), tokenize.detect_encoding(io.BytesIO(eb).readline))
        except BaseException as e:
            print("DETECT ERR", repr(eb), type(e).__name__, e.args)
    corpus = os.path.join(os.path.dirname(os.path.abspath(__file__)), "corpus")
    for name in sorted(os.listdir(corpus)):
        data = open(os.path.join(corpus, name), 'rb').read()
        h = hashlib.sha256()
        n = 0
        try:
            for t in tokenize.tokenize(io.BytesIO(data).readline):
                h.update(repr((t.type, t.string, t.start, t.end, t.line)).encode()); n += 1
            print("FILE", name, n, h.hexdigest())
        except BaseException as e:
            print("FILE ERR", name, type(e).__name__, e.args)
        try:
            u = tokenize.untokenize(tokenize.tokenize(io.BytesIO(data).readline))
            print("FILE-UNTOK", name, hashlib.sha256(u).hexdigest())
        except BaseException as e:
            print("FILE-UNTOK ERR", name, type(e).__name__, e.args)
main()
