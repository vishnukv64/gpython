"""math: the C maths library.

Run with:  /tmp/gpy examples/stdlib/math_demo.py

Everything here works on floats (or ints widened to floats) and returns a
float, except factorial() and the integer-ish helpers.

Interpreter note: math.gcd(), math.comb(), math.isqrt(), math.lcm() and
math.isclose() are NOT present -- only the libm-shaped functions are exposed.
See examples/KNOWN_ISSUES.md.
"""

import math

print("--- constants ---")
print("math.pi:", math.pi)
print("math.e: ", math.e)
print("tau is absent:", not hasattr(math, "tau"))
print("inf:    ", math.inf if hasattr(math, "inf") else float("inf"))
print("nan:    ", math.nan if hasattr(math, "nan") else float("nan"))

print()
print("--- powers and roots ---")
print("sqrt(2):        ", math.sqrt(2))
print("sqrt(16):       ", math.sqrt(16))
print("pow(2, 10):     ", math.pow(2, 10))
print("exp(1):         ", math.exp(1))
print("expm1(tiny):    ", math.expm1(1e-10))
print("hypot(3, 4):    ", math.hypot(3, 4))
print("factorial(10):  ", math.factorial(10))
print("ldexp(0.5, 4):  ", math.ldexp(0.5, 4))

print()
print("--- logarithms ---")
print("log(e):         ", math.log(math.e))
print("log(8, 2):      ", math.log(8, 2))
print("log10(1000):    ", math.log10(1000))
print("log2(1024):     ", math.log2(1024))
print("log1p(1e-10):   ", math.log1p(1e-10))

print()
print("--- rounding-family ---")
print("floor(2.7):     ", math.floor(2.7))
print("ceil(2.1):      ", math.ceil(2.1))
print("trunc(-2.7):    ", math.trunc(-2.7))
print("fabs(-3.5):     ", math.fabs(-3.5))
print("fmod(7, 3):     ", math.fmod(7, 3))
print("copysign(3,-1): ", math.copysign(3, -1))
print("modf(3.7):      ", math.modf(3.7), "(fractional, integral)")

print()
print("--- trigonometry ---")
print("sin(pi/2):      ", math.sin(math.pi / 2))
print("cos(0):         ", math.cos(0))
print("tan(pi/4):      ", math.tan(math.pi / 4))
print("asin(1):        ", math.asin(1))
print("atan2(1, 1):    ", math.atan2(1, 1))
print("degrees(pi):    ", math.degrees(math.pi))
print("radians(180):   ", math.radians(180))
print("hyperbolic sinh(1):", math.sinh(1))

print()
print("--- classification predicates ---")
print("isnan(nan):     ", math.isnan(float("nan")))
print("isinf(inf):     ", math.isinf(float("inf")))
print("isfinite(1.0):  ", math.isfinite(1.0))
print("isfinite(inf):  ", math.isfinite(float("inf")))

print()
print("--- compensated summation ---")
plain = 0.0
for _ in range(10):
    plain = plain + 0.1
print("naive sum of ten 0.1:  ", plain)
print("math.fsum of ten 0.1:  ", math.fsum([0.1] * 10))
print("(fsum tracks the rounding error and compensates)")

print()
print("--- decomposition ---")
print("frexp(8.0):  ", math.frexp(8.0), "(mantissa, exponent)")
print("ldexp(0.5, 4):", math.ldexp(0.5, 4), "(the inverse)")
print("gamma(5):    ", math.gamma(5), "  == factorial(4)")
print("lgamma(5):   ", math.lgamma(5))
print("erf(1):      ", math.erf(1))
print("erfc(1):     ", math.erfc(1))

print()
print("--- a worked example: a statistics helper ---")


class Stats:
    def __init__(self):
        self.values = []

    def add(self, value):
        self.values.append(value)
        return self

    def count(self):
        return len(self.values)

    def total(self):
        return math.fsum(self.values)

    def mean(self):
        return self.total() / self.count()

    def variance(self):
        mean = self.mean()
        # fsum keeps the squared deviations from drifting.
        return math.fsum([(value - mean) ** 2 for value in self.values]) / self.count()

    def stddev(self):
        return math.sqrt(self.variance())

    def summary(self):
        return ("n=%d  sum=%.4f  mean=%.4f  var=%.4f  sd=%.4f"
                % (self.count(), self.total(), self.mean(), self.variance(), self.stddev()))


stats = Stats()
for value in [2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0]:
    stats.add(value)
print("values:", [2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0])
print("mean is exactly 5:", stats.mean() == 5.0)
print(stats.summary())

print()
print("--- floating point limits ---")
print("sqrt of a negative raises ValueError:")
try:
    math.sqrt(-1)
except ValueError as err:
    print("  ", type(err).__name__, err)
print("log of zero raises ValueError:")
try:
    math.log(0)
except ValueError as err:
    print("  ", type(err).__name__, err)
print("factorial of a negative raises ValueError:")
try:
    math.factorial(-1)
except ValueError as err:
    print("  ", type(err).__name__, err)

print()
print("--- integer helpers that are NOT here ---")
for name in ["gcd", "lcm", "comb", "perm", "isqrt", "isclose", "prod", "nextafter"]:
    print("  math.%-10s %s" % (name, hasattr(math, name)))
