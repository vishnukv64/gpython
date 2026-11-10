import re, time, sys
def t(label, fn):
    s = time.perf_counter(); r = fn(); print(f"{label:32} {time.perf_counter() - s:9.4f}  {r}", flush=True)
text = " ".join(f"user{i}@host{i % 50}.com junk{i}" for i in range(2_000))
pat = re.compile(r"(\w+)@(\w+)\.com")
t("re compile", lambda: len([re.compile(r"(\w+)@(\w+)\.com%d" % i) for i in range(50)]))
t("findall 2k (\\w+)@", lambda: len(pat.findall(text)))
t("search literal", lambda: re.search("junk1999", text) is not None)
t("sub \\d+ 2k", lambda: len(re.sub(r"\d+", "#", text)))
t("findall [a-z]+ 2k", lambda: len(re.findall(r"[a-z]+", text)))
t("match anchored x20k", lambda: sum(1 for i in range(20_000) if re.match(r"\d+", str(i))))
a = [i * 7 % 1000 for i in range(200_000)]
t("listcomp 200k", lambda: len([i * 7 % 1000 for i in range(200_000)]))
t("sort 200k", lambda: sorted(a)[0])
t("sorted key lambda 200k", lambda: sorted(a, key=lambda v: -v)[0])
t("slice [::3]", lambda: sum(a[::3]))
t("filter comp", lambda: len([x for x in a if x % 2]))
