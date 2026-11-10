def f(c):
    match c:
        case b"*":
            return "star"
        case b"a" | b"b":
            return "ab"
        case "s":
            return "str"
        case {b"k": v}:
            return ("map", v)
    return "none"
print(f(b"*"), f(b"b"), f("s"), f(b"z"), f({b"k": 3}))
