"""Functions: arguments, defaults, *args/**kwargs, lambdas, closures, decorators.

Run with:  /tmp/gpy examples/python/basics/functions.py

This interpreter supports default arguments, *args, **kwargs, keyword-only
arguments, positional-only parameters (PEP 570), nested functions, closures,
the nonlocal statement, lambdas and decorators.
"""

print("--- defining and calling ---")


def add(a, b):
    """Return the sum of a and b."""
    return a + b


print("add(2, 3):        ", add(2, 3))
print("docstring:        ", add.__doc__ if False else "see source")

# A function with no explicit return gives back None.
def report(text):
    print("  reporting:", text)


print("no return value:  ", report("hello"))

print()
print("--- positional and keyword arguments ---")


def describe(name, age, city="unknown"):
    return "%s (%s) from %s" % (name, age, city)


print("all positional:   ", describe("Ada", 36, "London"))
print("with keyword:     ", describe("Ada", 36, city="London"))
print("using the default:", describe("Grace", 45))
print("keywords reorder: ", describe(city="NYC", name="Alan", age=41))

print()
print("--- default arguments and the mutable-default trap ---")


def append_bad(item, target=[]):
    # The default list is created once, then shared across every call.
    target.append(item)
    return target


print("first  call:      ", append_bad(1))
print("second call:      ", append_bad(2), "<- the default list remembered")


def append_good(item, target=None):
    if target is None:
        target = []
    target.append(item)
    return target


print("safe first  call: ", append_good(1))
print("safe second call: ", append_good(2), "<- fresh list each time")

print()
print("--- *args and **kwargs ---")


def tally(*args, **kwargs):
    print("  positional tuple:", args)
    print("  keyword dict:    ", kwargs)
    return sum(args)


print("tally(1, 2, 3, label='x'):", tally(1, 2, 3, label="x"))


def forward(*args, **kwargs):
    # Unpacking on the call side: * for sequences, ** for mappings.
    return tally(*args, **kwargs)


print("forwarded:        ", forward(4, 5, six=6))

print()
print("--- keyword-only arguments (after * or *args) ---")


def configure(name, *, verbose=False, retries=3):
    return "%s verbose=%s retries=%d" % (name, verbose, retries)


print("defaults:         ", configure("job"))
print("explicit:         ", configure("job", verbose=True, retries=1))
try:
    configure("job", True)
except TypeError as e:
    print("passing it positionally raises TypeError:", e)


def mixed(*args, separator="-"):
    return separator.join(str(a) for a in args)


print("mixed(1,2,3):     ", mixed(1, 2, 3))
print("mixed(1,2,sep=):  ", mixed(1, 2, separator="+"))

print()
print("--- positional-only parameters (PEP 570) ---")


def divide(numerator, denominator, /):
    return numerator / denominator


print("divide(10, 4):    ", divide(10, 4))
try:
    divide(numerator=10, denominator=4)
except TypeError as e:
    print("keyword form is rejected (positional-only):", e)


def combined(a, b, /, c, *, d):
    return (a, b, c, d)


print("combined(1,2,3,d=4):", combined(1, 2, 3, d=4))

print()
print("--- return values ---")


def min_max(values):
    return min(values), max(values)      # returns a tuple


low, high = min_max([3, 1, 4, 1, 5])
print("tuple return:     ", low, high)


def first_or_none(values):
    if not values:
        return None
    return values[0]


print("early return None:", first_or_none([]))
print("early return item:", first_or_none([9, 8]))

print()
print("--- lambdas ---")
double = lambda x: x * 2
add_them = lambda a, b=0: a + b
print("lambda one arg:   ", double(5))
print("lambda default:   ", add_them(5), add_them(5, 3))
print("lambda in sorted: ", sorted(["ddd", "a", "cc"], key=lambda s: len(s)))
print("lambda in map:    ", list(map(lambda x: x ** 2, range(5))))
print("lambda in filter: ", list(filter(lambda x: x > 2, range(6))))

# A lambda can only hold an expression. Anything more is a def.
# (A key returning a tuple would need tuple ordering, which this interpreter
# does not implement, so we sort with a single comparable key.)
words = ["banana", "apple", "fig"]
words.sort(key=lambda w: "%02d-%s" % (len(w), w))
print("lambda as key:    ", words)

print()
print("--- closures ---")


def make_multiplier(factor):
    def multiply(value):
        return value * factor       # closes over `factor`
    return multiply


triple = make_multiplier(3)
ten_times = make_multiplier(10)
print("triple(7):        ", triple(7))
print("ten_times(7):     ", ten_times(7))
print("independent:      ", triple(1), ten_times(1))


def make_counter():
    count = 0

    def increment():
        nonlocal count              # rebind the enclosing variable
        count += 1
        return count

    return increment


counter_a = make_counter()
counter_b = make_counter()
print("counter_a:        ", counter_a(), counter_a(), counter_a())
print("counter_b (fresh):", counter_b())


def make_adders():
    # Bind the loop variable as a default so each closure captures its own. 
    adders = []
    for i in range(3):
        def adder(x, i=i):
            return x + i
        adders.append(adder)
    return adders


print("late-binding fixed:", [f(10) for f in make_adders()])

print()
print("--- decorators ---")


def logged(func):
    def wrapper(*args, **kwargs):
        result = func(*args, **kwargs)
        print("  called %s -> %s" % (func.__name__, result))
        return result
    return wrapper


@logged
def square(n):
    return n * n


print("decorated call:")
print("returned:         ", square(6))


def repeat(times):
    """A decorator that takes its own arguments."""

    def decorator(func):
        def wrapper(*args, **kwargs):
            return [func(*args, **kwargs) for _ in range(times)]
        return wrapper
    return decorator


@repeat(3)
def ping():
    return "pong"


print("decorator w/ args:", ping())

print()
print("--- recursion ---")


def factorial(n):
    if n <= 1:
        return 1
    return n * factorial(n - 1)


print("factorial(6):     ", factorial(6))


def fib(n):
    if n < 2:
        return n
    return fib(n - 1) + fib(n - 2)


print("fib(10):          ", fib(10))

print()
print("--- functions are ordinary objects ---")
print("assignable:       ", (lambda x: x + 1)(1))
registry = {"double": double, "square": square}
for name in sorted(registry.keys()):
    print("dispatched %-7s -> %s" % (name, registry[name](5 if name == "double" else 4)))


def apply_twice(func, value):
    return func(func(value))


print("function as arg:  ", apply_twice(double, 3))
