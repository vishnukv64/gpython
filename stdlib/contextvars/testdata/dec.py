from decimal import Decimal, getcontext, localcontext, ROUND_HALF_EVEN, ROUND_DOWN, InvalidOperation
print(Decimal("1.1") + Decimal("2.2"), Decimal("1.1") * 3, Decimal(1) / Decimal(7))
print(Decimal("2").sqrt(), Decimal("10") ** 3, Decimal("-7") // 2, Decimal("-7") % 3)
print(Decimal("3.14159").quantize(Decimal("0.01")), Decimal("2.675").quantize(Decimal("0.01"), rounding=ROUND_DOWN))
print(getcontext().prec)
with localcontext() as c:
    c.prec = 5
    print(Decimal(1) / Decimal(3), getcontext().prec)
print(getcontext().prec, Decimal(1) / Decimal(3))
with localcontext(prec=3):
    print(Decimal("123456") + 0)
print(Decimal("1e3"), repr(Decimal("1.50")), Decimal("1.50") == Decimal("1.5"), hash(Decimal("1.5")) == hash(1.5))
print(Decimal("NaN").is_nan(), Decimal("-Inf"), float(Decimal("0.1")), int(Decimal("9.9")))
print(format(Decimal("1234.5678"), ",.2f"), "{:e}".format(Decimal("12345")))
try:
    Decimal("abc")
except InvalidOperation:
    print("InvalidOperation")
print(sorted([Decimal("2"), Decimal("1.5"), Decimal("-1")]), Decimal("1.5").as_tuple())
print(Decimal(0.1))
