import re
print("neg:", bool(re.search(r"(?<!x)y", "ay")), bool(re.search(r"(?<!x)y", "xy")))
print("pos:", bool(re.search(r"(?<=x)y", "xy")), bool(re.search(r"(?<=x)y", "ay")))
print("word:", bool(re.search(r"(?<!\w)abc", " abc")), bool(re.search(r"(?<!\w)abc", "xabc")))
print("findall:", re.findall(r"(?<!\w)[0-9]+", "a 12 b34 c"))
print("escaped:", re.findall(r'(?<![\\w])"', 'a" \" b"'))
print("span:", re.search(r"(?<!x)y", "ay").span())
print("start:", bool(re.search(r"(?<!x)y", "y")))
print("group:", re.search(r"(?<!x)(a)", "ya").group(1))
print("pos0:", bool(re.search(r"(?<!x)y", "y")), bool(re.search(r"(?<=x)y", "y")))
print("pos0 findall:", re.findall(r"(?<!x)y", "y y"))

# Mid-pattern: a closing quote NOT preceded by a backslash, at the end of a
# lazy body.  This is rich's string pattern idiom.
DQ = chr(34)
BS = chr(92)
pat = DQ + '.*?(?<!' + BS + BS + ')' + DQ
print("mid:", re.findall(pat, 'a "x" b "y" c'))
print("mid esc:", re.findall(pat, 'a "x' + BS + BS + '" b'))
print("mid none:", re.findall(pat, "no quotes here"))

# A leading lookbehind combined with alternation and a group.
print("alt:", re.findall(r"(?<!a|b)(c|d)", "ac bc xc xd"))

doc = "finished"
