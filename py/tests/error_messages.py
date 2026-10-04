"""Two error messages that name what actually went wrong."""

# A property with no setter: the message names the property AND the class, so a
# reader knows which @x.setter is missing.
class Circle:
    @property
    def radius(self):
        return 1


try:
    Circle().radius = 5
except AttributeError as e:
    print("no setter:", e)

# A positional-only parameter passed by keyword: the message says the argument
# EXISTS but may not be passed by name, and lists every such parameter - not
# "unexpected keyword argument", which is misleading when the name is expected.
def divide(a, b, /):
    return a / b


try:
    divide(a=1, b=2)
except TypeError as e:
    print("posonly:", e)

try:
    divide(1, b=2)
except TypeError as e:
    print("posonly one:", e)

# The ordinary case keeps its own message.
try:
    divide(1, 2, nope=3)
except TypeError as e:
    print("unexpected:", e)

doc = "finished"
