from fractions import Fraction
import math

# Construction from int, float, str, Fraction, and numerator/denominator.
assert Fraction(3, 4) == Fraction("3/4")
assert Fraction("3/4") + Fraction(1, 4) == 1
assert Fraction("3/4") + Fraction(1, 4) == Fraction(1, 1)
assert Fraction(0.5) == Fraction(1, 2)
assert Fraction("1.5") == Fraction(3, 2)
assert Fraction("  -7/3  ") == Fraction(-7, 3)
assert Fraction("1e3") == Fraction(1000, 1)
assert Fraction(Fraction(3, 4)) == Fraction(3, 4)
assert Fraction(2, 3) == Fraction(4, 6)

# numerator / denominator, and the positive-denominator normalization.
assert Fraction(3, -9).numerator == -1
assert Fraction(3, -9).denominator == 3
assert Fraction(-7, 3).numerator == -7
assert Fraction(-7, 3).denominator == 3

# repr and str: repr is the exact form, str is not.
assert repr(Fraction(-7, 3)) == "Fraction(-7, 3)", repr(Fraction(-7, 3))
assert str(Fraction(-7, 3)) == "-7/3", str(Fraction(-7, 3))
assert str(Fraction(2, 1)) == "2", str(Fraction(2, 1))
assert repr(Fraction(2, 1)) == "Fraction(2, 1)"

# hash: equal to hash(int) when the denominator is 1, and to hash(float) when
# the value is exactly representable (the float hash is the interpreter's).
assert hash(Fraction(3, 1)) == hash(3)
assert hash(Fraction(-3, 1)) == hash(-3)

# limit_denominator.
assert Fraction(7, 3).limit_denominator(10) == Fraction(7, 3)
assert Fraction("3.14159").limit_denominator(10) == Fraction(22, 7)
assert Fraction("1/3").limit_denominator(100) == Fraction(1, 3)

# Arithmetic stays exact.
assert Fraction(2, 3) ** 2 == Fraction(4, 9)
assert Fraction(2, 3) ** -1 == Fraction(3, 2)
assert Fraction(2, 3) + 1 == Fraction(5, 3)
assert 1 + Fraction(2, 3) == Fraction(5, 3)
assert Fraction(1, 2) * Fraction(2, 3) == Fraction(1, 3)
assert Fraction(1, 2) / Fraction(1, 4) == 2
assert Fraction(1, 2) // Fraction(1, 3) == 1
assert Fraction(7, 2) % 3 == Fraction(1, 2)
assert divmod(Fraction(7, 2), 3) == (1, Fraction(1, 2))
assert 2 ** Fraction(-1) == Fraction(1, 2)
# A fractional power of a perfect square stays exact, of anything else
# falls back to a float, as in CPython.
assert Fraction(4, 9) ** Fraction(1, 2) == 0.6666666666666666
assert Fraction(2, 3) ** Fraction(1, 2) != Fraction(2, 3)

# Mixed with a float the result is a float, as in CPython.
assert Fraction(1, 2) + 0.5 == 1.0
assert float(Fraction(1, 3)) == 0.3333333333333333

# Comparisons.
assert Fraction(1, 2) == 0.5
assert Fraction(1, 2) < 0.6
assert Fraction(2, 3) > 0.6
assert Fraction(1, 2) <= Fraction(1, 2)
assert Fraction(2, 3) != Fraction(1, 3)

# math.floor / ceil / trunc, and round.
assert math.floor(Fraction(-7, 3)) == -3
assert math.ceil(Fraction(-7, 3)) == -2
assert math.trunc(Fraction(-7, 3)) == -2
assert round(Fraction(7, 3)) == 2
assert round(Fraction(5, 2)) == 2
assert round(Fraction(3, 2)) == 2
assert round(Fraction(7, 3), 1) == Fraction(23, 10)

# as_integer_ratio and is_integer.
assert Fraction(1, 2).as_integer_ratio() == (1, 2)
assert Fraction(5, 1).is_integer()
assert not Fraction(1, 2).is_integer()

# A bad literal raises ValueError.
try:
    Fraction("abc")
except ValueError:
    pass
else:
    raise AssertionError("Fraction('abc') did not raise ValueError")

print("fractions ok")
