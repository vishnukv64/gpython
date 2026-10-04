"""Division of a non-number names the operation and BOTH operands."""

for label, fn in [
    ("str / int", lambda: "a" / 1),
    ("int / str", lambda: 1 / "a"),
    ("list / int", lambda: [] / 1),
    ("int / None", lambda: 1 / None),
]:
    try:
        fn()
    except TypeError as e:
        print(label, "->", e)

# Other operators were already right, and stay right.
try:
    "a" - 1
except TypeError as e:
    print("sub ->", e)

doc = "finished"
