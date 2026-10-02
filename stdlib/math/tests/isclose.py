# math.isclose, exactly as CPython defines it.
import math

assert math.isclose(1, 1)
assert not math.isclose(1.0, 1.0000001)
assert not math.isclose(1.0, 1.1)
assert math.isclose(1, 1.5, rel_tol=0, abs_tol=0.5)
assert not math.isclose(1, 1.6, rel_tol=0, abs_tol=0.5)
assert math.isclose(1e10, 1e10 + 1.0, rel_tol=1e-9)  # within 1 part in 1e9
assert math.isclose(float("inf"), float("inf"))
assert not math.isclose(float("inf"), float("-inf"))
assert not math.isclose(float("nan"), float("nan"))
assert math.isclose(True, 1)

try:
    math.isclose(1, 1, rel_tol=-1)
    raise AssertionError("expected ValueError")
except ValueError as e:
    assert str(e) == "tolerances must be non-negative", str(e)

try:
    math.isclose("a", 1)
    raise AssertionError("expected TypeError")
except TypeError:
    pass

try:
    math.isclose(1, 1, "positional")
    raise AssertionError("expected TypeError for positional rel_tol")
except TypeError:
    pass

doc = "finished"
