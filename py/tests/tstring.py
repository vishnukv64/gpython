from string.templatelib import Template, Interpolation, convert
x=5; y="hi"
t = t"a{x}b{y!r:>{x}}c{x=}"
print(repr(t)); print(t.strings, t.values)
for i in t.interpolations: print(repr(i), i.value, i.expression, i.conversion, i.format_spec)
print(list(t))
print(repr(t"{ x }"), repr(t""), repr(t"abc"), repr(t"{x}{y}"))
print(repr(t"a" t"b{x}"), repr(rt"\n{x}"), repr(t"\n{x}"))
print(repr(t"a{x}" + t"b{y}"))
try: t + "s"
except TypeError as e: print(e)
print(repr(Template("a", Interpolation(1, "x"), "b")))
print(repr(Template()), repr(Template("a","b")), repr(Template(Interpolation(1))))
print(type(t), type(t).__module__, Interpolation.__match_args__, Template.__match_args__ if hasattr(Template,'__match_args__') else None)
print(t == t, t == t"a", hash(t) is not None)
try: Template(1)
except TypeError as e: print(e)
try: Interpolation(1, "x", "q")
except Exception as e: print(type(e).__name__, e)
try: str(t) ; print(str(t))
except Exception as e: print(e)
match Interpolation(3,"e","r","f"):
    case Interpolation(v, e, c, f): print(v,e,c,f)
print(convert("a", "r"), convert("a", None))
print(f"{t.strings!r}")
for src in ['t"a" "b"','f"a" t"b"','t"a" b"x"','t"{x!z}"', 'tb"x"', 'ft"x"','t"{}"']:
    try: compile(src,"<s>","eval"); print("ok",src)
    except SyntaxError as e: print("SE",src,e.msg)
print(repr(t"{x:{y}}"), repr(t"{x!s}"), repr(t"{x = }"), repr(t"{x=:>3}"),repr(t"{{}}"))
import copy, pickle
print(repr(copy.copy(t)), pickle.loads(pickle.dumps(t)) == t if 0 else "")
print(repr(t.__class__.__name__), Interpolation.__name__, Interpolation.__module__)
