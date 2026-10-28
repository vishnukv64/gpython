import cmath, itertools, sys
_w = []
def print(*a):
    _w.append(' '.join(map(str, a)))
inf, nan = float('inf'), float('nan')
vals = [0.0, -0.0, 0.5, -0.5, 1.0, -1.0, 2.0, -2.0, 3.5, 1e-5, -1e-5, 1e5, -1e5, 1e300, -1e300, 5e-324, 2.2e-308, 1.7e308, inf, -inf, nan]
zs = [complex(a, b) for a in vals for b in vals]
def rep(x):
    return "(%r, %r)" % (x.real, x.imag)
def close(a, b):
    # libm last-digit differences are allowed for finite results; everything else (signs, inf, nan) must match
    return cmath.isclose(a, b, rel_tol=1e-12, abs_tol=1e-300) or (a != a and b != b)
for name in "acos acosh asin asinh atan atanh cos cosh exp log log10 sin sinh sqrt tan tanh phase polar isfinite isinf isnan".split():
    f = getattr(cmath, name)
    for z in zs:
        try:
            r = f(z)
        except (ValueError, OverflowError) as e:
            r = type(e).__name__
        if isinstance(r, complex):
            # print rounded to 12 digits, keeping signs of zeros, infs, nans
            def q(x):
                return repr(x) if x != x or x in (inf, -inf) or x == 0 else "%.10e" % x
            r = "(%s, %s)" % (q(r.real), q(r.imag))
        elif isinstance(r, tuple):
            r = repr(tuple(("%.10e" % v if v == v and abs(v) != inf and v else repr(v)) for v in r))
        elif isinstance(r, float):
            r = "%.10e" % r if r == r and abs(r) != inf and r else repr(r)
        print(name, rep(z), r)
for r_, p_ in itertools.product(vals, vals):
    try: z = cmath.rect(r_, p_)
    except (ValueError, OverflowError) as e: print("rect", r_, p_, type(e).__name__); continue
    q = lambda x: repr(x) if x != x or x in (inf, -inf) or x == 0 else "%.10e" % x
    print("rect", r_, p_, q(z.real), q(z.imag))
for z, b in [(10+2j, 10), (8, 2), (1j, 1j), (-1, 2), (0, 2), (2, 0), (3+4j, -1j), (inf, 2), (1e308+1e308j, 3)]:
    try: r = cmath.log(z, b); print("log2", z, b, "%.10e %.10e" % (r.real, r.imag))
    except (ValueError, ZeroDivisionError, OverflowError) as e: print("log2", z, b, type(e).__name__)
print(cmath.pi, cmath.e, cmath.tau, cmath.inf, cmath.infj, cmath.nan, cmath.nanj)
print(cmath.isclose(1+1j, 1+1.0000000001j), cmath.isclose(1j, 1j+1e-12, abs_tol=1e-10), cmath.isclose(inf, inf), cmath.isclose(nan, nan), cmath.isclose(complex(inf, 0), 1))
for f in (lambda: cmath.isclose(1, 1, rel_tol=-1), lambda: cmath.sqrt("a"), lambda: cmath.sqrt(), lambda: cmath.sqrt(None), lambda: cmath.exp(1000), lambda: cmath.log(0), lambda: cmath.atanh(1), lambda: cmath.cosh(1000+1j)):
    try: print(f())
    except Exception as e: print(type(e).__name__)
class C:
    def __complex__(self): return 3+4j
class F:
    def __float__(self): return 2.0
class I:
    def __index__(self): return 7
print(cmath.phase(C()), cmath.polar(F()), cmath.sqrt(I()), cmath.sqrt(True), cmath.exp(2), cmath.sqrt(-4))
sys.stdout.write(chr(10).join(_w) + chr(10))
