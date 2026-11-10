import rapidfuzz
from rapidfuzz import fuzz, utils
from rapidfuzz.distance import Levenshtein as L

print(rapidfuzz.__version__)

strs = ["", "a", "b", "ab", "abc", "abd", "kitten", "sitting", "lewenstein", "levenshtein",
        "Hello, World", "hello world", "héllo", "hello", "日本語", "日本人", "😀x", "😁x",
        "install", "instal", "add", "new", "poetry", "poerty", "a" * 40, "a" * 39 + "b",
        "the quick brown fox", "quick the brown fox"]
seqs = [[], [1, 2, 3], [1, 3], ["a", "bc", "d"], ["a", "bc", "e"], (1, -1, 2), [-1], ["ab", ("t", 1)],
        b"", b"abc", b"abd", bytearray(b"abc"), ["a", "b"], "ab"]
weights = [None, (1, 1, 1), (1, 1, 2), (2, 3, 4), (1, 2, 3), (3, 1, 2), (0, 1, 1), (1, 1, 0)]
fns = [L.distance, L.similarity, L.normalized_distance, L.normalized_similarity]

def show(f, *a, **k):
    d = sorted((n, getattr(v, "__name__", "fn") if callable(v) else v) for n, v in k.items())
    try:
        r = f(*a, **k)
        print(f.__name__, a, d, repr(r), type(r).__name__)
    except Exception as e:
        print(f.__name__, a, d, "EXC", type(e).__name__)

for f in fns:
    for a in strs:
        for b in strs[::3]:
            show(f, a, b)
    for w in weights:
        for a, b in [("kitten", "sitting"), ("", "abc"), ("abc", ""), ("lewenstein", "levenshtein"), ("日本語", "日本人")]:
            show(f, a, b, weights=w)
    for c in (None, 0, 1, 2, 3, 0.2, 0.5, 0.85, 1.0):
        for a, b in [("kitten", "sitting"), ("lewenstein", "levenshtein"), ("", ""), ("abc", "abc")]:
            show(f, a, b, score_cutoff=c)
            show(f, a, b, score_cutoff=c, weights=(1, 1, 2))
    show(f, "ab", "AB", processor=str.lower)
    show(f, "  Ab!", "ab", processor=utils.default_process)
    show(f, ["lewenstein"], ["levenshtein"], processor=lambda s: s[0])
    show(f, "kitten", "sitting", score_hint=2)
    show(f, None, "a")
    show(f, "a", None)
    show(f, float("nan"), "a")
    show(f, 1, 2)
    show(f, "a")
    show(f, "a", "b", weights=(1, 2))
    show(f, "a", "b", weights=(-1, 1, 1))
    show(f, "a", ["x", [1]])
    for a in seqs:
        for b in seqs:
            show(f, a, b)

for a in strs:
    for b in strs[::2]:
        show(fuzz.ratio, a, b)
for c in (None, 0, 50, 80, 90, 100):
    for a, b in [("this is a test", "this is a test!"), ("abc", "abd"), ("", ""), ("a", "b")]:
        show(fuzz.ratio, a, b, score_cutoff=c)
show(fuzz.ratio, None, "a")
show(fuzz.ratio, "A b", "a B", processor=utils.default_process)
show(fuzz.ratio, [1, 2], [1, 3])
show(fuzz.ratio, "a")

for s in ["", "  ", " Hello, World! ", "a_b-c", "Héllo", "ÀÉÎ", "x²y", "٣٤", "日本語 text", "😀 x", "A\tB\nC", "İstanbul", "ß", "ǅ", "a\u0301b", "\u00a0nb\u00a0", "ΑΣ", "Σ", "ⅷ"]:
    show(utils.default_process, s)
show(utils.default_process, 1)
