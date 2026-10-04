"""Percent-formatting: star width and precision, and string precision."""

# A '*' takes the width from the argument list, which is what pip's option
# formatter writes.
print(repr("%*s%s" % (4, "", "hi")))
print(repr("%-*s|" % (4, "a")))
print(repr("%*s" % (-4, "a")))
print(repr("%*d" % (6, 42)))
print(repr("%*d" % (-6, 42)))
print(repr("%0*d" % (5, 42)))

# The precision can come from an argument too.
print(repr("%.*f" % (2, 3.14159)))
print(repr("%*.*f" % (10, 3, 3.14159)))
print(repr("%.*e" % (2, 12345.6789)))

# A negative precision is precision ZERO, not "no precision" - which the
# float and string verbs both show.
print(repr("%.*f" % (-2, 3.14159)))
print(repr("%.*s" % (-2, "abcdef")))
print(repr("%.*f" % (2, 3.14159)))

# Precision truncates a string, written literally as well as through '*'.
print(repr("%.2s" % "abcdef"))
print(repr("%.0s" % "abcdef"))
print(repr("%.2r" % "abcdef"))
print(repr("%10.3s|" % "abcdef"))

# The ordinary forms still work.
print(repr("%s %r %d %x %o" % ("a", "b", 10, 255, 8)))
print(repr("%.2f" % 3.14159))
print(repr("%(k)s" % {"k": "v"}))
print(repr("%5.2f|" % 3.14159))
print(repr("%-5.2f|" % 3.14159))

doc = "finished"
