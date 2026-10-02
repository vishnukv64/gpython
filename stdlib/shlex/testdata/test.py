import shlex

# POSIX quoting.
assert shlex.split('a "b c" d') == ["a", "b c", "d"]
assert shlex.split("a 'b c' d") == ["a", "b c", "d"]
assert shlex.split(r"a\ b") == ["a b"]
assert shlex.split('a "" b') == ["a", "", "b"]

# comments=False disables comment handling, which is the split() default.
assert shlex.split("a #comment\nb", comments=True) == ["a", "b"]
assert shlex.split("a #comment\nb") == ["a", "#comment", "b"]

# quote and join.
assert shlex.quote("a b") == "'a b'", shlex.quote("a b")
assert shlex.quote("") == "''"
assert shlex.quote("a'b") == "'a'\"'\"'b'", shlex.quote("a'b")
assert shlex.quote("abc") == "abc"
assert shlex.join(["a b", "c"]) == "'a b' c"

# posix=False keeps the quotes in the token.
assert shlex.split('a "b c" d', posix=False) == ["a", '"b c"', "d"]
lex = shlex.shlex('a "b c" d', posix=False)
assert list(lex) == ["a", '"b c"', "d"]
lex = shlex.shlex("'a'", posix=False)
assert list(lex) == ["'a'"]

# A backslash escapes the space in posix mode; the shlex() default is
# posix=False, so the backslash is literal there.
lex = shlex.shlex(r"a\ b", posix=True)
assert list(lex) == ["a b"]
lex = shlex.shlex(r"a\ b")
assert list(lex) == ["a", "\\", "b"]
lex = shlex.shlex(r"a\ b", posix=False)
assert list(lex) == ["a", "\\", "b"]

# punctuation_chars groups runs of punctuation into their own token.
lex = shlex.shlex("a;b|c", punctuation_chars=";|")
lex.whitespace_split = True
assert list(lex) == ["a", ";", "b", "|", "c"]
assert lex.punctuation_chars == ";|"

# commenters can be changed on an instance.
lex = shlex.shlex("x #comment")
lex.commenters = "#"
assert list(lex) == ["x"]
lex = shlex.shlex("a b # c\n d")
lex.commenters = "#"
assert list(lex) == ["a", "b", "d"]

# get_token and push_token.
lex = shlex.shlex("a b")
assert lex.get_token() == "a"
assert lex.get_token() == "b"
lex.push_token("xx")
assert lex.get_token() == "xx"

# whitespace_split=True makes each whitespace-separated word one token, and
# keeps "1+2" whole; the default splits the punctuation out.
lex = shlex.shlex("1+2")
lex.whitespace_split = True
assert lex.whitespace_split
assert list(lex) == ["1+2"]
assert list(shlex.shlex("1+2")) == ["1", "+", "2"]

# An apostrophe inside a word is not a quote in posix mode.
lex = shlex.shlex("it's")
assert list(lex) == ["it's"]

# An unclosed quote raises ValueError.
try:
    list(shlex.shlex("a 'b"))
except ValueError:
    pass
else:
    raise AssertionError("unclosed quote did not raise ValueError")

print("shlex ok")
