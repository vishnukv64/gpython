"""except* (PEP 654): splitting, several clauses, reraise, nesting, errors."""


def show(label, f):
    try:
        f()
        print(label, "-> no exception")
    except BaseException as e:
        subs = [repr(x) for x in getattr(e, "exceptions", ())]
        print(label, "->", type(e).__name__, repr(e), subs, repr(e.__context__))


def split_two():
    try:
        raise ExceptionGroup("g", [ValueError(1), TypeError(2), KeyError(3)])
    except* ValueError as e:
        print("V", repr(e))
    except* (TypeError, KeyError) as e:
        print("T/K", repr(e))


show("split_two", split_two)


def nested():
    try:
        raise ExceptionGroup("o", [ValueError(1), ExceptionGroup("i", [TypeError(2), ValueError(3)])])
    except* ValueError as e:
        print("V", repr(e))
    except* TypeError as e:
        print("T", repr(e))


show("nested", nested)


def naked():
    try:
        raise ValueError(1)
    except* ValueError as e:
        print("wrapped", repr(e), e.exceptions)


show("naked", naked)


def naked_nomatch():
    try:
        raise ValueError(1)
    except* TypeError:
        print("never")


show("naked_nomatch", naked_nomatch)


def bare_raise_naked():
    try:
        raise ValueError(1)
    except* ValueError:
        raise


show("bare_raise_naked", bare_raise_naked)


def unmatched_rest():
    try:
        raise ExceptionGroup("g", [ValueError(1), TypeError(3)])
    except* ValueError:
        pass


show("unmatched_rest", unmatched_rest)


def raise_new_only():
    try:
        raise ExceptionGroup("g", [ValueError(1)])
    except* ValueError:
        raise KeyError(2)


show("raise_new_only", raise_new_only)


def raise_new_with_rest():
    try:
        raise ExceptionGroup("g", [ValueError(1), TypeError(3)])
    except* ValueError:
        raise KeyError(2)


show("raise_new_with_rest", raise_new_with_rest)


def bare_reraise_group():
    try:
        raise ExceptionGroup("g", [ValueError(1), TypeError(3)])
    except* ValueError:
        raise
    except* TypeError:
        print("T handled")


show("bare_reraise_group", bare_reraise_group)


def reraise_named():
    try:
        raise ExceptionGroup("g", [ValueError(1), TypeError(3)])
    except* ValueError as e:
        raise e


show("reraise_named", reraise_named)


def both_raise():
    try:
        raise ExceptionGroup("g", [ValueError(1), TypeError(3)])
    except* ValueError:
        raise KeyError(5)
    except* TypeError:
        raise


show("both_raise", both_raise)


def else_finally(raise_it):
    log = []
    try:
        try:
            if raise_it:
                raise ExceptionGroup("g", [ValueError(1)])
        except* ValueError:
            log.append("handler")
        else:
            log.append("else")
        finally:
            log.append("finally")
    finally:
        print(log)


else_finally(True)
else_finally(False)


try:
    raise ExceptionGroup("g", [ValueError(1)])
except* ValueError as e:
    pass
try:
    e
except NameError as ne:
    print("NameError", ne)


def loop_inside():
    try:
        raise ExceptionGroup("g", [ValueError(1)])
    except* ValueError:
        for i in range(5):
            if i == 2:
                break
        print("loop ended at", i)


loop_inside()


def exc_info_inside():
    import sys
    try:
        raise ExceptionGroup("g", [ValueError(1), TypeError(2)])
    except* ValueError:
        print(repr(sys.exc_info()[1]))
    except* TypeError:
        print(repr(sys.exc_info()[1]))


try:
    exc_info_inside()
except ExceptionGroup:
    pass


def bad_type(t):
    try:
        raise ValueError(1)
    except* t:
        pass


for t in (int, ExceptionGroup, (ValueError, BaseExceptionGroup)):
    show("bad_type", lambda: bad_type(t))

print("PEP 758 tuple without parens")
try:
    raise KeyError(1)
except KeyError, ValueError:
    print("caught")
try:
    raise ExceptionGroup("g", [KeyError(1)])
except* KeyError, ValueError:
    print("caught*")

for src in (
    "try:\n pass\nexcept* A:\n pass\nexcept B:\n pass\n",
    "try:\n pass\nexcept A:\n pass\nexcept* B:\n pass\n",
    "try:\n pass\nexcept*:\n pass\n",
    "def f():\n try:\n  pass\n except* A:\n  return 1\n",
    "for i in x:\n try:\n  pass\n except* A:\n  break\n",
    "for i in x:\n try:\n  pass\n except* A:\n  continue\n",
    "try:\n pass\nexcept* A:\n for i in x:\n  break\n",
    "def f():\n try:\n  pass\n except* A:\n  def g():\n   return 1\n",
):
    try:
        compile(src, "<s>", "exec")
        print("ok")
    except SyntaxError as e:
        print("SyntaxError", e.msg, e.lineno)
