import re
t = "héllo wörld ñandú"
m = re.search(r"(w\w+) (\w+)", t)
print(m.start(), m.end(), m.span(1), m.span(2), m.group(1), m.group(2))
print([(x.start(), x.span(), x.group()) for x in re.finditer(r"\w+", t)])
print(re.findall(r"x*", "axxb"))
print(re.findall(r"x*", "éxxé"))
print(re.sub(r"(\w)(\w*)", r"\2\1", t))
print(re.sub(r"x*", "-", "axxb"))
print(re.split(r"(ö)", t))
print(re.split(r"\s*", "é é"))
print(re.findall(r"(?<!é)l", t))
