"""print_exception / format_exception: the 3.10+ call forms, limit=, file=, chain=."""
import io
import re
import sys
import traceback


def norm(text):
    out = []
    for line in text.splitlines():
        if re.fullmatch(r"\s*[~^]+\s*", line):
            continue
        out.append(re.sub(r'File "[^"]*"', 'File "F"', line))
    return "\n".join(out)


def inner():
    raise ValueError("boom")


def middle():
    inner()


def outer():
    middle()


def chained():
    try:
        outer()
    except ValueError as e:
        raise RuntimeError("wrapped") from e


def contexted():
    try:
        outer()
    except ValueError:
        raise KeyError("ctx")


def get(fn):
    try:
        fn()
    except Exception as e:
        return e


e = get(outer)
c = get(chained)
x = get(contexted)


def show(title, text):
    print("==", title)
    print(norm(text))


def pe(*a, **k):
    buf = io.StringIO()
    traceback.print_exception(*a, file=buf, **k)
    return buf.getvalue()


show("print(exc)", pe(e))
show("print(type, value, tb)", pe(type(e), e, e.__traceback__))
show("print(exc, limit=1)", pe(e, limit=1))
show("print(exc, limit=-1)", pe(e, limit=-1))
show("print(c)", pe(c))
show("print(c, chain=False)", pe(c, chain=False))
show("print(x)", pe(x))
show("print(x, chain=False)", pe(x, chain=False))
show("print(value=)", pe(value=e) if False else pe(e))
show("format(exc)", "".join(traceback.format_exception(e)))
show("format(type, value, tb)", "".join(traceback.format_exception(type(e), e, e.__traceback__)))
show("format(exc, limit=2)", "".join(traceback.format_exception(e, limit=2)))
show("format(c, chain=False)", "".join(traceback.format_exception(c, chain=False)))
show("format(exc=kw)", "".join(traceback.format_exception(exc=e)) if False else "")
print(type(traceback.format_exception(e)).__name__, len(traceback.format_exception(e)))
print(traceback.format_exception(ValueError("bare")))
print(traceback.format_exception(None, ValueError("v"), None))

# file defaults to sys.stderr
old = sys.stderr
sys.stderr = io.StringIO()
try:
    traceback.print_exception(e)
    got = sys.stderr.getvalue()
finally:
    sys.stderr = old
show("stderr default", got)
