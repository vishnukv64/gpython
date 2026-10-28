# Declined items: not CPython goldens (CPython has these); they pin the decision.
import sys
for i in range(2):
    try:
        import tracemalloc
    except ImportError as e:
        print("tracemalloc:", str(e).split(":")[0], "tracemalloc" in sys.modules)
import faulthandler
print([n for n in ("dump_traceback_later", "cancel_dump_traceback_later", "register", "unregister") if hasattr(faulthandler, n)])
