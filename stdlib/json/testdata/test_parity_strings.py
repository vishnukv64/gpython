# CPython 3.12 parity: str.center fill order and dict.pop KeyError text.
print("ab".center(5, '.'))
print("a".center(4, '*'))
print("x".center(4, '.'))
print("abc".center(8, '.'))
d = {"a": 1}
try:
    d.pop("z")
except KeyError as e:
    print("KeyError", str(e))
