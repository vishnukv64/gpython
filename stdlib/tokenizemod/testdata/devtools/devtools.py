import sys, os, tabnanny, pyclbr, inspect, io
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
def show(label, f):
    try: print(label, f())
    except BaseException as e: print(label, "ERR", type(e).__name__, e)
d = os.path.dirname(os.path.abspath(__file__))
def cap(fn, *a):
    old = sys.stdout; sys.stdout = io.StringIO()
    try: fn(*a); return sys.stdout.getvalue().replace(d, "D")
    finally: sys.stdout = old
tabnanny.verbose = 1
show("tab-good", lambda: cap(tabnanny.check, os.path.join(d, "good.py")))
show("tab-bad", lambda: cap(tabnanny.check, os.path.join(d, "bad_tab.py")))
show("tab-nr", lambda: tabnanny.NannyNag(3, "m", "l").get_msg())
show("readmodule", lambda: sorted((k, type(v).__name__, v.lineno) for k, v in pyclbr.readmodule("mod_a").items()))
def tree(t, ind=0):
    out = []
    for k, v in sorted(t.items()):
        out.append("%s%s %s %s %s" % (" " * ind, k, type(v).__name__, v.lineno, getattr(v, 'end_lineno', None)))
        out += tree(v.children, ind + 2) if hasattr(v, 'children') else []
    return out
show("readmodule_ex", lambda: tree(pyclbr.readmodule_ex("mod_a")))
show("supers", lambda: [(c.name, [getattr(s, 'name', s) for s in c.super]) for c in pyclbr.readmodule("mod_a").values() if hasattr(c, 'super')])
import mod_a
show("getsource-func", lambda: inspect.getsource(mod_a.func))
show("getsource-class", lambda: inspect.getsource(mod_a.Derived))
show("getsourcelines", lambda: inspect.getsourcelines(mod_a.Base.Inner))
show("getblock", lambda: inspect.getblock(["def f():\n", "    return 1\n", "x = 2\n"]))
show("getcomments", lambda: inspect.getcomments(mod_a.func))
show("cleandoc", lambda: inspect.cleandoc("  a\n    b\n  c"))
