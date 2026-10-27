import re
p = re.compile(rb"(a)(b)?")
print(p.findall(b"ab a"), re.findall(rb"\w+", b"foo bar"))
print(re.split(rb"(,)", b"a,b,c"), re.split(rb",", b"a,b"))
print(re.sub(rb"a(b)", rb"<\1>", b"xab"), re.subn(rb"a", b"-", b"aaa"))
print(re.sub(rb"a", lambda m: b"[" + m.group() + b"]", b"banana"))
m = re.match(rb"(?P<x>a)(b)?", b"ac")
print(m.group(), m.group(1), m.group("x"), m.groups(), m.groups(b"d"), m.groupdict(), m.string)
print(m.group(1, 2))
for f in (lambda: re.search(rb"a", "a"), lambda: re.search("a", b"a"),
          lambda: re.sub(rb"a", "b", b"a"), lambda: re.sub("a", b"b", "a"),
          lambda: p.findall("ab")):
    try:
        f()
    except TypeError as e:
        print("TypeError", e)
print(re.findall(rb".", b"\xff\x00"), re.sub(rb"\xff", b"\xfe", b"\xff"))
