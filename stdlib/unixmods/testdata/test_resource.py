import resource, os
print(resource.RLIMIT_CPU, resource.RLIMIT_NOFILE != resource.RLIMIT_CPU, resource.RUSAGE_SELF, resource.RUSAGE_CHILDREN)
s, h = resource.getrlimit(resource.RLIMIT_NOFILE)
print(isinstance(s, int), s <= h or h == resource.RLIM_INFINITY)
resource.setrlimit(resource.RLIMIT_NOFILE, (s, h))
print(resource.getrlimit(resource.RLIMIT_NOFILE) == (s, h))
lo = min(s, 256) if s >= 0 else 256
resource.setrlimit(resource.RLIMIT_NOFILE, (lo, h))
print(resource.getrlimit(resource.RLIMIT_NOFILE)[0] == lo)
resource.setrlimit(resource.RLIMIT_NOFILE, (s, h))
u = resource.getrusage(resource.RUSAGE_SELF)
print(type(u).__name__, len(u), isinstance(u.ru_utime, float), u.ru_maxrss > 0, u[0] == u.ru_utime, resource.struct_rusage.__module__)
print(resource.getpagesize() == os.sysconf('SC_PAGE_SIZE') if hasattr(os, 'sysconf') else resource.getpagesize() > 0)
print(resource.error is OSError)
for f in (lambda: resource.getrlimit(999), lambda: resource.getrlimit(-1), lambda: resource.getrusage(99), lambda: resource.setrlimit(resource.RLIMIT_CPU, (5, 1)), lambda: resource.setrlimit(resource.RLIMIT_CPU, 5), lambda: resource.getrlimit("a")):
    try: f()
    except Exception as e: print(type(e).__name__, e)
