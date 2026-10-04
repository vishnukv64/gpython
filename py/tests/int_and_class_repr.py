"""int() error wording, and a class repr qualified with its module."""

try:
    int(None)
except TypeError as e:
    print("int(None):", e)
try:
    int([])
except TypeError as e:
    print("int([]):", e)

# A class repr names the module it was defined in, which is how a reader tells
# two classes of the same name apart.  A builtin has no module and prints bare.
class MyError(Exception):
    pass


print("defined here:", MyError)
print("builtin:", int)
print("str:", str)

doc = "finished"
