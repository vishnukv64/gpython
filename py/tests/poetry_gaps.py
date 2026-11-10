import urllib.parse as u
for s in (b'', b'a', b'a\nb\r\nc\rd\n', b'\n\n', b'a\r\n\r\nb'):
    print(s.splitlines(), s.splitlines(True), s.splitlines(keepends=True))
a = u.urlsplit('https://h/p?x#f'); b = u.urlsplit('https://h/p?x#f')
print(a == b, a != b, a == tuple(a), a == 5, hash(a) == hash(b), hash(a) == hash(tuple(a)), a == u.urlsplit('https://h/q'), {a: 1}[b])
print(u.urlparse('http://a/b;c?d#e') == u.urlparse('http://a/b;c?d#e'), a._replace(query='') == b._replace(query=''))
