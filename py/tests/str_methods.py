print("zfill:", repr("42".zfill(5)), repr("-42".zfill(5)), repr("+42".zfill(5)), repr("abc".zfill(2)), repr("abc".zfill(0)))
print("center:", repr("ab".center(6, "-")), repr("ab".center(5)), repr("ab".center(1)), repr("ab".center(7, "*")))
print("ljust:", repr("ab".ljust(5, "*")), repr("ab".ljust(1)), repr("ab".ljust(4)))
print("expandtabs:", repr("a\tb".expandtabs(4)), repr("ab\tc".expandtabs(4)), repr("a\tb".expandtabs(0)), repr("\t".expandtabs(4)))
print("removeprefix:", repr("foobar".removeprefix("foo")), repr("foobar".removeprefix("zz")), repr("foobar".removeprefix("")))
print("removesuffix:", repr("foobar".removesuffix("bar")), repr("foobar".removesuffix("zz")))
print("rfind:", "abcabc".rfind("b"), "abcabc".rfind("b", 0, 3), "abc".rfind("z"), "abc".rfind(""))
print("index:", "abcabc".index("b"), "abcabc".index("b", 2))
print("rindex:", "abcabc".rindex("b"))
for name, fn in [("index", lambda: "abc".index("z")), ("rindex", lambda: "abc".rindex("z"))]:
    try:
        fn()
    except ValueError as e:
        print(name, "err:", e)
print("partition:", "a=b".partition("="), "a=b".partition("z"), "".partition("x"), "=x".partition("="))
print("rpartition:", "a=b=c".rpartition("="), "a=b".rpartition("z"), "x=".rpartition("="))
print("casefold:", repr("STRASSE".casefold()), repr("\u00df".casefold()), repr("AbC".casefold()), repr("\u0130".casefold()))
print("swapcase:", repr("AbC".swapcase()), repr("abc".swapcase()), repr("aBc".swapcase()))
print("maketrans:", str.maketrans("ab", "xy"), str.maketrans({"a": "x"}), str.maketrans("ab", "xy", "a"))
print("translate:", repr("abc".translate({97: "X"})), repr("abc".translate({97: None})), repr("abc".translate({97: 65})))
print("translate table:", "abc".translate(str.maketrans("ab", "xy")))
try:
    "ab".center(6, "xy")
except TypeError as e:
    print("fillchar err:", e)
try:
    "a".partition("")
except ValueError as e:
    print("sep err:", e)

doc = "finished"
