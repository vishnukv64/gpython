import fnmatch

# Case folding: fnmatch lowercases both sides, fnmatchcase does not.  On a
# case-sensitive filesystem normcase is the identity, so both keep case; the
# two calls still agree because the pattern and the name have the same case.
assert fnmatch.fnmatch("a/b", "a*")
assert fnmatch.fnmatch("A.TXT", "*.TXT")
assert fnmatch.fnmatchcase("A.TXT", "*.TXT")
assert not fnmatch.fnmatch("a.txt", "*.TXT")
assert fnmatch.fnmatch("a.TXT", "*.TXT")

# ? and character classes.
assert fnmatch.fnmatch("x", "[!a]")
assert not fnmatch.fnmatch("a", "[!a]")
assert fnmatch.fnmatch("x", "?")
assert fnmatch.fnmatch("a", "[abc]")
assert fnmatch.fnmatch("c", "[a-c]")
assert not fnmatch.fnmatch("d", "[a-c]")
assert fnmatch.fnmatch("abc", "a?c")

# A "[" with no close is a literal.
assert fnmatch.fnmatch("a[", "a[")

# translate returns CPython's regex text verbatim.
assert fnmatch.translate("*.TXT") == "(?s:.*\\.TXT)\\z", fnmatch.translate("*.TXT")
assert fnmatch.translate("a[!bc]d") == "(?s:a[^bc]d)\\z", fnmatch.translate("a[!bc]d")
assert fnmatch.translate("[a-z]?") == "(?s:[a-z].)\\z", fnmatch.translate("[a-z]?")
assert fnmatch.translate("x") == "(?s:x)\\z", fnmatch.translate("x")
assert fnmatch.translate("") == "(?s:)\\z", fnmatch.translate("")
assert fnmatch.translate("*") == "(?s:.*)\\z", fnmatch.translate("*")
assert fnmatch.translate("**") == "(?s:.*)\\z", fnmatch.translate("**")
assert fnmatch.translate("a?b") == "(?s:a.b)\\z", fnmatch.translate("a?b")
assert fnmatch.translate("[-a]") == "(?s:[\\-a])\\z", fnmatch.translate("[-a]")
assert fnmatch.translate("[]a]") == "(?s:[]a])\\z", fnmatch.translate("[]a]")
assert fnmatch.translate("a*b") == "(?s:a.*b)\\z", fnmatch.translate("a*b")
assert fnmatch.translate("[!]") == "(?s:\\[!\\])\\z", fnmatch.translate("[!]")
assert fnmatch.translate("[z-a]") == "(?s:(?!))\\z", fnmatch.translate("[z-a]")

# A class range that is not a valid pattern never matches.
assert not fnmatch.fnmatchcase("x", "[z-a]")

# filter and filterfalse keep the original elements.
assert fnmatch.filter(["a.txt", "b.TXT", "c.py", "A.txt"], "*.TXT") == ["b.TXT"]
assert fnmatch.filterfalse(["a.txt", "b.TXT", "c.py"], "*.TXT") == ["a.txt", "c.py"]

# A "*" matches across a path separator, unlike glob.
assert fnmatch.fnmatch("a/b/c.txt", "*.txt")
assert fnmatch.fnmatchcase("a/b", "a*")

print("fnmatch ok")
