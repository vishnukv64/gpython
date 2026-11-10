# Lookarounds: a lookbehind after consuming text (answered by backtracking),
# lookbehinds over non-ASCII text and \u escapes, {n} widths, and the
# PatternError class name.
import re
cases = [
 (r"\d+(?!%)", ["12%", "123", "12%34", "99% 100", "1%2%3", "", "%%"]),
 (r"x(?<!a)y.*", ["xy", "axy", "xyz", "ax yxyq", "xxy-1"]),
 (r"(?<=(a|bc))d", ["ad", "bcd", "cd", "xbcdad"]),
 (r"a(?(1)b|c)", ["ab", "ac"]),
 (r"(\w+?)(?<!s)\b", ["cats dogs bird", "less mass x"]),
 (r"(?<![\d.])\d+(?![\d.])", ["1.5 22 3.", "a 7 b", "10.0.1 8"]),
 (r"(?:(?!ab).)+", ["xxabyyab", "abab", "zzz"]),
]
for pat, texts in cases:
    try:
        p = re.compile(pat)
    except Exception as e:
        print(pat, "compile", type(e).__name__); continue
    for t in texts:
        print(pat, repr(t), [(m.span(), m.group()) for m in p.finditer(t)], p.sub("#", t))
import re
for pat in [r"\w+(?<!s)\b", r"(\w+)(?<!s)\b", r"\w+?(?<!s)\b", r"(\w+?)(?<!s)\b", r"\w+(?<!s)", r"\w+(?<!s) ", r"(\w+?)(?<!s)(?=\W|$)", r"\w(?<!s)\b", r"ab(?<!b)\b", r"a\w*?(?<=t)\b"]:
    for t in ["cats dogs bird", "a_bat cat"]:
        print(pat, repr(t), [m.span() for m in re.finditer(pat, t)])
for pat, t in [(r"(?<!é)x", "éx ax"), (r"(?<=é)x", "éx ax"), (r"(?<=ab)c", "abc ébc"), (r"(?<!\w)\d+", "a1 22 é3"),
               (r"(?<=a{3})b", "aab aaab"), (r"(?<=(?:ab){2})c", "abc ababc"), (r"(?<=\d{2}é)x", "12éx 1éx"),
               (r"(?<!é)", "é"), (r"(?<=[éa])x", "éx ax bx"), (r"(?<=\u00e9)x", "éx"), (r"(?<!😀)z", "😀z az")]:
    print(pat, repr(t), [m.span() for m in re.finditer(pat, t)], re.sub(pat, "#", t))
import re
for pat, t in [(r"(?<=\x41)b", "Ab ab"), (r"(?<=\\é)x", "\\éx éx"), (r"(?<!\.)\d", "a.1 2")]:
    print(pat, [m.span() for m in re.finditer(pat, t)])
try:
    re.compile("(?<=a+)b")
except re.error as e:
    print(type(e).__name__, e)
