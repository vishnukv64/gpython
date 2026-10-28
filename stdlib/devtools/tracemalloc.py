raise ImportError(
    "tracemalloc is not available in gpython: it records the Python source "
    "location of every allocation, and Go's garbage-collected heap does not "
    "expose allocation sites; zeroed statistics would make leak checks pass "
    "vacuously, so the module is declined (use runtime/pprof via the host "
    "program, or sys.getsizeof for single objects)"
)
