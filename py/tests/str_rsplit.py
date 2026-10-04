print(repr("a b c".rsplit()))
print(repr("a b c".rsplit(None, 1)))
print(repr("a,b,c".rsplit(",", 1)))
print(repr("a,b,c".rsplit(",", 0)))
print(repr("abc".rsplit()))
print(repr("".rsplit()))
print(repr("  a  b  ".rsplit()))
print(repr("a,b,c".rsplit(",", -1)))
print(repr("a,,b".rsplit(",")))
print(repr("a,b,c,d".rsplit(",", 2)))

doc = "finished"
