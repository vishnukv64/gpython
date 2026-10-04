def f(v):
    match v:
        case [x, *rest]:
            return (x, rest)
        case _:
            return "no"

print("4 elems:", f([1,2,3,4]))
print("2 elems:", f([1,2]))
print("1 elem:", f([1]))
print("string:", f("ab"))
print("empty:", f([]))

doc = "finished"
